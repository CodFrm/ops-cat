package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/pkg/executil"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/external_edit_svc"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

type ExternalEditSettings = external_edit_svc.Settings
type ExternalEditSettingsInput = external_edit_svc.SettingsInput
type ExternalEditOpenRequest = external_edit_svc.OpenRequest
type ExternalEditSession = external_edit_svc.Session
type ExternalEditSaveResult = external_edit_svc.SaveResult
type ExternalEditCompareResult = external_edit_svc.CompareResult
type ExternalEditMergePrepareResult = external_edit_svc.MergePrepareResult
type ExternalEditMergeApplyRequest = external_edit_svc.MergeApplyRequest

func (a *App) initExternalEdit() {
	// Wails 绑定层只负责把运行时依赖接到 service：
	// 数据目录、配置持久化、SFTP 远端访问、资产查询、审计和桌面事件都从这里注入，
	// 具体状态机和文件处理规则全部下沉到 external_edit_svc，避免 IPC 层再分叉业务逻辑。
	svc, err := external_edit_svc.NewService(external_edit_svc.Options{
		DataDir:        bootstrap.AppDataDir(),
		ConfigProvider: bootstrap.GetConfig,
		ConfigSaver:    bootstrap.SaveConfig,
		Remote:         a.sftpService,
		FindSessions:   a.sshManager.ListActiveSessionIDsByAsset,
		Assets:         asset_repo.Asset(),
		Audit:          audit_repo.Audit(),
		Emit: func(event external_edit_svc.Event) {
			if a.ctx == nil {
				return
			}
			wailsRuntime.EventsEmit(a.ctx, "external-edit:event", event)
		},
		Launch: externalEditLauncher{},
	})
	if err != nil {
		logger.Default().Warn("init external edit service", zap.Error(err))
		return
	}
	if err := svc.Start(context.Background()); err != nil {
		logger.Default().Warn("start external edit service", zap.Error(err))
	}
	a.externalEditSvc = svc
}

type externalEditLauncher struct{}

func (externalEditLauncher) Launch(execPath string, args []string) error {
	cmd := exec.Command(execPath, args...) //nolint:gosec // path and args are validated by external_edit_svc
	executil.HideConsoleWindow(cmd)
	return cmd.Start()
}

func (a *App) externalEditService() (*external_edit_svc.Service, error) {
	if a.externalEditSvc == nil {
		return nil, fmt.Errorf("external edit service unavailable")
	}
	return a.externalEditSvc, nil
}

func (a *App) GetExternalEditSettings() (*ExternalEditSettings, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.GetSettings()
}

func (a *App) SaveExternalEditSettings(input ExternalEditSettingsInput) (*ExternalEditSettings, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.SaveSettings(input)
}

func (a *App) SelectExternalEditorExecutable() (string, error) {
	// 选择器只返回用户明确挑选的绝对路径，不在 IPC 层提前推断可用性；
	// 真正的可执行文件校验留给 service 统一处理，避免桌面端和测试端出现双重规则。
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: a.externalEditDialogTitle("editor"),
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Executable", Pattern: executablePattern()},
		},
	})
	if err != nil {
		return "", fmt.Errorf("打开文件对话框失败: %w", err)
	}
	return filePath, nil
}

func (a *App) SelectExternalEditWorkspaceRoot() (string, error) {
	// 工作区目录由前端选择，但最终是否落盘、是否需要补齐默认路径仍以后端配置逻辑为准。
	dirPath, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: a.externalEditDialogTitle("workspace"),
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	return dirPath, nil
}

func (a *App) OpenExternalEdit(req ExternalEditOpenRequest) (*ExternalEditSession, error) {
	// IPC 边界只转发“打开哪个远程文件”的意图；
	// 文本判定、会话复用、编码快照、审计和事件广播全部交由 service 串行裁决。
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Open(a.langCtx(), req)
}

func (a *App) ListExternalEditSessions() ([]*ExternalEditSession, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.ListSessions(), nil
}

func (a *App) SaveExternalEditSession(sessionID string) (*ExternalEditSaveResult, error) {
	// 保存和冲突处理都以 sessionID 作为唯一入口，前端不直接操纵远端文件内容，
	// 这样 desktop 事件、审计和状态恢复才能围绕同一份会话记录运转。
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Save(a.langCtx(), sessionID)
}

func (a *App) RefreshExternalEditSession(sessionID string) (*ExternalEditSession, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Refresh(sessionID)
}

func (a *App) ResolveExternalEditConflict(sessionID, resolution string) (*ExternalEditSaveResult, error) {
	// resolution 只是用户决策信号：overwrite / recreate / reread 的副作用和状态迁移全部封装在 service 内，
	// IPC 层不额外拼接分支，避免同一冲突在不同入口出现不一致结果。
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Resolve(a.langCtx(), sessionID, resolution)
}

func (a *App) CompareExternalEditSession(sessionID string) (*ExternalEditCompareResult, error) {
	// compare 只暴露“生成一个只读差异快照”的能力；
	// 具体的编码/BOM/round-trip 校验与远端身份确认仍由 service 串行裁决，避免前端擅自猜测文件状态。
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Compare(sessionID)
}

func (a *App) PrepareExternalEditMerge(sessionID string) (*ExternalEditMergePrepareResult, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.PrepareMerge(sessionID)
}

func (a *App) ApplyExternalEditMerge(req ExternalEditMergeApplyRequest) (*ExternalEditSaveResult, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.ApplyMerge(a.langCtx(), req)
}

func (a *App) RecoverExternalEditSession(sessionID string) (*ExternalEditSession, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Recover(sessionID)
}

func (a *App) ContinueExternalEditSession(sessionID string) (*ExternalEditSession, error) {
	svc, err := a.externalEditService()
	if err != nil {
		return nil, err
	}
	return svc.Continue(sessionID)
}

func (a *App) externalEditDialogTitle(kind string) string {
	isEnglish := strings.EqualFold(a.lang, "en")
	switch kind {
	case "workspace":
		if isEnglish {
			return "Choose External Edit Workspace"
		}
		return "选择外部编辑工作区"
	default:
		if isEnglish {
			return "Choose External Editor"
		}
		return "选择外部编辑器"
	}
}

func executablePattern() string {
	if runtime.GOOS == "windows" {
		return "*.exe"
	}
	return "*"
}
