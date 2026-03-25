package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/embedded"
	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/backup_svc"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/internal/service/import_svc"
	"github.com/opskat/opskat/internal/service/update_svc"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// --- GitHub Token ---

// SaveGitHubToken 加密保存 GitHub token
func (a *App) SaveGitHubToken(token, user string) error {
	cfg := bootstrap.GetConfig()
	if token == "" {
		cfg.GitHubToken = ""
		cfg.GitHubUser = ""
	} else {
		encrypted, err := credential_svc.Default().Encrypt(token)
		if err != nil {
			return fmt.Errorf("加密 GitHub Token 失败: %w", err)
		}
		cfg.GitHubToken = encrypted
		cfg.GitHubUser = user
	}
	return bootstrap.SaveConfig(cfg)
}

// GetGitHubToken 获取解密后的 GitHub token
func (a *App) GetGitHubToken() (string, error) {
	cfg := bootstrap.GetConfig()
	if cfg.GitHubToken == "" {
		return "", nil
	}
	return credential_svc.Default().Decrypt(cfg.GitHubToken)
}

// GetStoredGitHubUser 获取保存的 GitHub 用户名
func (a *App) GetStoredGitHubUser() string {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return ""
	}
	return cfg.GitHubUser
}

// ClearGitHubToken 清除保存的 GitHub token
func (a *App) ClearGitHubToken() error {
	return a.SaveGitHubToken("", "")
}

// --- 导入导出 ---

// PreviewTabbyConfig 预览 Tabby 配置（不写入数据库）
// 自动检测默认路径，找不到则弹出文件选择框
func (a *App) PreviewTabbyConfig() (*import_svc.PreviewResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.PreviewTabbyConfig(a.langCtx(), data)
}

// ImportTabbySelected 导入用户选中的 Tabby 连接
func (a *App) ImportTabbySelected(selectedIndexes []int, passphrase string, overwrite bool) (*import_svc.ImportResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.ImportTabbySelected(a.langCtx(), data, selectedIndexes, import_svc.ImportOptions{
		Passphrase: passphrase,
		Overwrite:  overwrite,
	})
}

// PreviewSSHConfig 预览 SSH Config 文件（不写入数据库）
// 自动检测 ~/.ssh/config，找不到则弹出文件选择框
func (a *App) PreviewSSHConfig() (*import_svc.PreviewResult, error) {
	data, err := a.readSSHConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.PreviewSSHConfig(a.langCtx(), data)
}

// ImportSSHConfigSelected 导入用户选中的 SSH Config 连接
func (a *App) ImportSSHConfigSelected(selectedIndexes []int, overwrite bool) (*import_svc.ImportResult, error) {
	data, err := a.readSSHConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.ImportSSHConfigSelected(a.langCtx(), data, selectedIndexes, import_svc.ImportOptions{
		Overwrite: overwrite,
	})
}

// readSSHConfig 读取 SSH Config 文件
func (a *App) readSSHConfig() ([]byte, error) {
	filePath := import_svc.DetectSSHConfigPath()
	if filePath == "" {
		var err error
		filePath, err = wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择 SSH Config 文件",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "All Files", Pattern: "*"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("打开文件对话框失败: %w", err)
		}
		if filePath == "" {
			return nil, nil
		}
	}
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog or known config path
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return data, nil
}

// readTabbyConfig 读取 Tabby 配置文件内容
func (a *App) readTabbyConfig() ([]byte, error) {
	filePath := detectTabbyConfigPath()
	if filePath == "" {
		var err error
		filePath, err = wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择 Tabby 配置文件",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
				{DisplayName: "All Files", Pattern: "*"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("打开文件对话框失败: %w", err)
		}
		if filePath == "" {
			return nil, nil
		}
	}
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog or known config path
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return data, nil
}

// detectTabbyConfigPath 检测 Tabby 配置文件默认路径
func detectTabbyConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(homeDir, "Library", "Application Support", "tabby", "config.yaml"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			candidates = []string{
				filepath.Join(appData, "Tabby", "config.yaml"),
			}
		}
	case "linux":
		candidates = []string{
			filepath.Join(homeDir, ".config", "tabby", "config.yaml"),
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil { //nolint:gosec // path is from known config locations
			return path
		}
	}
	return ""
}

// ExportData 导出所有资产和分组为 JSON（剪贴板用，不含凭据）
func (a *App) ExportData() (string, error) {
	opts := &backup_svc.ExportOptions{}
	data, err := backup_svc.Export(a.langCtx(), opts, nil)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// --- 备份操作 ---

// ExportToFile 导出备份到文件
func (a *App) ExportToFile(password string, opts backup_svc.ExportOptions) error {
	if opts.IncludeCredentials && password == "" {
		return fmt.Errorf("包含凭据时必须设置备份密码")
	}

	var crypto backup_svc.CredentialCrypto
	if opts.IncludeCredentials {
		crypto = credential_svc.Default()
	}

	data, err := backup_svc.Export(a.langCtx(), &opts, crypto)
	if err != nil {
		return err
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	var output []byte
	var defaultName string
	if password != "" {
		output, err = backup_svc.EncryptBackup(jsonData, password)
		if err != nil {
			return err
		}
		defaultName = fmt.Sprintf("opskat-backup-%s.encrypted.json", time.Now().Format("20060102"))
	} else {
		output = jsonData
		defaultName = fmt.Sprintf("opskat-backup-%s.json", time.Now().Format("20060102"))
	}

	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil
	}

	return os.WriteFile(filePath, output, 0644)
}

// ImportFileInfo 导入文件信息
type ImportFileInfo struct {
	FilePath  string                    `json:"filePath"`
	Encrypted bool                      `json:"encrypted"`
	Summary   *backup_svc.BackupSummary `json:"summary,omitempty"`
}

// SelectImportFile 选择备份文件并检测是否加密，返回概览信息
func (a *App) SelectImportFile() (*ImportFileInfo, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "导入备份",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}

	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	info := &ImportFileInfo{
		FilePath:  filePath,
		Encrypted: backup_svc.IsEncryptedBackup(fileData),
	}
	// 非加密备份可直接解析概览
	if !info.Encrypted {
		var data backup_svc.BackupData
		if err := json.Unmarshal(fileData, &data); err == nil {
			info.Summary = data.Summary()
		}
	}
	return info, nil
}

// PreviewImportFile 解密并预览备份文件概览
func (a *App) PreviewImportFile(filePath, password string) (*backup_svc.BackupSummary, error) {
	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from previous file dialog
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return nil, err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}
	summary := data.Summary()
	summary.Encrypted = backup_svc.IsEncryptedBackup(fileData)
	return summary, nil
}

// ExecuteImportFile 执行文件导入
func (a *App) ExecuteImportFile(filePath, password string, opts backup_svc.ImportOptions) (*backup_svc.ImportResult, error) {
	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from previous file dialog selection
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return nil, err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data, &opts, credential_svc.Default())
}

// --- GitHub 认证 ---

// StartGitHubDeviceFlow 发起 GitHub Device Flow 认证
func (a *App) StartGitHubDeviceFlow() (*backup_svc.DeviceFlowInfo, error) {
	return backup_svc.StartDeviceFlow()
}

// WaitGitHubDeviceAuth 等待用户完成 GitHub 授权，返回 access_token
func (a *App) WaitGitHubDeviceAuth(deviceCode string, interval int) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	a.githubAuthCancel = cancel
	defer func() {
		cancel()
		a.githubAuthCancel = nil
	}()
	return backup_svc.PollDeviceAuth(ctx, deviceCode, interval)
}

// CancelGitHubAuth 取消 GitHub 授权等待
func (a *App) CancelGitHubAuth() {
	if a.githubAuthCancel != nil {
		a.githubAuthCancel()
	}
}

// GetGitHubUser 获取 GitHub 用户信息
func (a *App) GetGitHubUser(token string) (*backup_svc.GitHubUser, error) {
	return backup_svc.GetGitHubUser(token)
}

// --- Gist 备份 ---

// ExportToGist 加密并上传备份到 Gist
func (a *App) ExportToGist(password, token, gistID string, opts backup_svc.ExportOptions) (*backup_svc.GistInfo, error) {
	var crypto backup_svc.CredentialCrypto
	if opts.IncludeCredentials {
		crypto = credential_svc.Default()
	}

	data, err := backup_svc.Export(a.langCtx(), &opts, crypto)
	if err != nil {
		return nil, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := backup_svc.EncryptBackup(jsonData, password)
	if err != nil {
		return nil, err
	}

	return backup_svc.CreateOrUpdateGist(token, gistID, encrypted)
}

// ListBackupGists 列出用户的备份 Gist
func (a *App) ListBackupGists(token string) ([]*backup_svc.GistInfo, error) {
	return backup_svc.ListBackupGists(token)
}

// PreviewGistBackup 预览 Gist 备份概览
func (a *App) PreviewGistBackup(gistID, password, token string) (*backup_svc.BackupSummary, error) {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return nil, err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return nil, err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}
	summary := data.Summary()
	summary.Encrypted = true
	return summary, nil
}

// ImportFromGist 从 Gist 导入备份
func (a *App) ImportFromGist(gistID, password, token string, opts backup_svc.ImportOptions) (*backup_svc.ImportResult, error) {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return nil, err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return nil, err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data, &opts, credential_svc.Default())
}

// GetDataDir 返回应用数据目录
func (a *App) GetDataDir() string {
	return bootstrap.AppDataDir()
}

// --- Opsctl 安装 ---

// OpsctlInfo opsctl CLI 检测结果
type OpsctlInfo struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Embedded  bool   `json:"embedded"` // 桌面端是否内嵌了 opsctl 二进制
}

// DetectOpsctl 检测 opsctl CLI 是否已安装
func (a *App) DetectOpsctl() OpsctlInfo {
	info := OpsctlInfo{
		Embedded: embedded.HasEmbeddedOpsctl(),
	}
	path, err := exec.LookPath("opsctl")
	if err != nil {
		return info
	}
	info.Installed = true
	info.Path = path
	out, err := exec.Command(path, "version").Output() //nolint:gosec // path is from exec.LookPath
	if err == nil {
		info.Version = strings.TrimSpace(string(out))
	}
	return info
}

// GetOpsctlInstallDir 返回默认安装目录
func (a *App) GetOpsctlInstallDir() string {
	return embedded.DefaultInstallDir()
}

// InstallOpsctl 将内嵌的 opsctl 二进制安装到指定目录
func (a *App) InstallOpsctl(targetDir string) (string, error) {
	if targetDir == "" {
		targetDir = embedded.DefaultInstallDir()
	}
	return embedded.InstallOpsctl(targetDir)
}

// --- Skills ---

// SkillTarget AI Skill 安装目标
type SkillTarget struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
}

// skillTargetDefs 支持的 Skill 安装目标，添加新 CLI 只需在此追加
var skillTargetDefs = []struct {
	Name   string // 显示名称
	SubDir string // home 目录下的子目录，如 ".claude"
}{
	{"Claude Code", ".claude"},
	{"Codex", ".codex"},
	{"OpenCode", ".opencode"},
}

// DetectSkills 检测所有 AI 工具的 Skill 安装状态
func (a *App) DetectSkills() []SkillTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	targets := make([]SkillTarget, 0, len(skillTargetDefs))
	for _, def := range skillTargetDefs {
		skillDir := filepath.Join(home, def.SubDir, "skills", "opsctl")
		installed := false
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err == nil {
			installed = true
		}
		targets = append(targets, SkillTarget{Name: def.Name, Installed: installed, Path: skillDir})
	}
	return targets
}

// skillMDWithDataDir 返回注入数据目录后的 SKILL.md 内容
func (a *App) skillMDWithDataDir() string {
	dataDir := bootstrap.AppDataDir()
	insertion := "## Data Directory\n\n" + dataDir + "\n\n"
	return strings.Replace(a.skillContent.SkillMD, "## Global Flags", insertion+"## Global Flags", 1)
}

// installSkillTo 将 Skill 文件安装到指定目录
func (a *App) installSkillTo(skillDir string) error {
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(a.skillMDWithDataDir()), 0644); err != nil {
		return fmt.Errorf("write SKILL.md failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "commands.md"), []byte(a.skillContent.CommandsMD), 0644); err != nil {
		return fmt.Errorf("write commands.md failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "ops-init.md"), []byte(a.skillContent.OpsInitMD), 0644); err != nil {
		return fmt.Errorf("write ops-init.md failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "init.md"), []byte(a.skillContent.InitMD), 0644); err != nil {
		return fmt.Errorf("write init.md failed: %w", err)
	}
	return nil
}

// InstallSkills 安装 Skill 文件到所有支持的 AI 工具
func (a *App) InstallSkills() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory failed: %w", err)
	}

	for _, def := range skillTargetDefs {
		skillDir := filepath.Join(home, def.SubDir, "skills", "opsctl")
		if err := a.installSkillTo(skillDir); err != nil {
			return fmt.Errorf("install %s skill failed: %w", def.Name, err)
		}
	}

	// 清理旧的 skill 文件
	oldSkillPath := filepath.Join(home, ".claude", "commands", "opskat.md")
	if err := os.Remove(oldSkillPath); err != nil && !os.IsNotExist(err) {
		logger.Default().Warn("remove old skill file", zap.String("path", oldSkillPath), zap.Error(err))
	}

	return nil
}

// GetSkillPreview 获取 Skill 文件内容预览
func (a *App) GetSkillPreview() string {
	return "--- SKILL.md ---\n\n" + a.skillMDWithDataDir() +
		"\n\n--- init.md ---\n\n" + a.skillContent.InitMD +
		"\n\n--- references/commands.md ---\n\n" + a.skillContent.CommandsMD +
		"\n\n--- references/ops-init.md ---\n\n" + a.skillContent.OpsInitMD
}

// --- 审计日志 ---

// AuditLogListResult 审计日志列表结果
type AuditLogListResult struct {
	Items []*audit_entity.AuditLog `json:"items"`
	Total int64                    `json:"total"`
}

// ListAuditLogs 查询审计日志
func (a *App) ListAuditLogs(source string, assetID int64, startTime, endTime int64, offset, limit int, sessionID string) (*AuditLogListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	items, total, err := audit_repo.Audit().List(a.langCtx(), audit_repo.ListOptions{
		Source:    source,
		AssetID:   assetID,
		SessionID: sessionID,
		StartTime: startTime,
		EndTime:   endTime,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return &AuditLogListResult{Items: items, Total: total}, nil
}

// ListAuditSessions 查询审计日志中的会话列表
func (a *App) ListAuditSessions(startTime int64) ([]audit_repo.SessionInfo, error) {
	return audit_repo.Audit().ListSessions(a.langCtx(), startTime)
}

// --- 更新 ---

// startAutoUpdateCheck 启动时自动检查更新（每天一次）
func (a *App) startAutoUpdateCheck() {
	go func() {
		// 延迟 5 秒，等前端就绪
		time.Sleep(5 * time.Second)

		cfg := bootstrap.GetConfig()
		if cfg == nil {
			return
		}
		now := time.Now().Unix()
		if now-cfg.LastUpdateCheck < 86400 {
			return
		}

		info, err := update_svc.CheckForUpdate(a.GetUpdateChannel())
		if err != nil {
			logger.Default().Warn("auto check update failed", zap.Error(err))
			return
		}

		cfg.LastUpdateCheck = now
		if err := bootstrap.SaveConfig(cfg); err != nil {
			logger.Default().Warn("save last update check time", zap.Error(err))
		}

		if info.HasUpdate {
			wailsRuntime.EventsEmit(a.ctx, "update:available", info)
		}
	}()
}

// GetAppVersion 返回当前应用版本
func (a *App) GetAppVersion() string {
	return configs.Version
}

// GetUpdateChannel 获取当前更新通道
func (a *App) GetUpdateChannel() string {
	cfg := bootstrap.GetConfig()
	if cfg == nil || cfg.UpdateChannel == "" {
		return update_svc.ChannelStable
	}
	return cfg.UpdateChannel
}

// SetUpdateChannel 设置更新通道
func (a *App) SetUpdateChannel(channel string) error {
	cfg := bootstrap.GetConfig()
	cfg.UpdateChannel = channel
	return bootstrap.SaveConfig(cfg)
}

// CheckForUpdate 检查是否有新版本
func (a *App) CheckForUpdate() (*update_svc.UpdateInfo, error) {
	return update_svc.CheckForUpdate(a.GetUpdateChannel())
}

// DownloadAndInstallUpdate 下载并安装更新
// 更新完成后需要用户重启应用
func (a *App) DownloadAndInstallUpdate() error {
	err := update_svc.DownloadAndUpdate(a.GetUpdateChannel(), func(downloaded, total int64) {
		wailsRuntime.EventsEmit(a.ctx, "update:progress", map[string]int64{
			"downloaded": downloaded,
			"total":      total,
		})
	})
	if err != nil {
		return err
	}

	// 更新后重新安装 opsctl（如果已安装）
	opsctlInfo := a.DetectOpsctl()
	if opsctlInfo.Installed && embedded.HasEmbeddedOpsctl() {
		installDir := filepath.Dir(opsctlInfo.Path)
		if _, err := embedded.InstallOpsctl(installDir); err != nil {
			// opsctl 更新失败不阻塞主更新
			wailsRuntime.EventsEmit(a.ctx, "update:opsctl-error", err.Error())
		}
	}

	// 更新后重新安装 Skills（如果已安装）
	skills := a.DetectSkills()
	for _, s := range skills {
		if s.Installed {
			if err := a.installSkillTo(s.Path); err != nil {
				wailsRuntime.EventsEmit(a.ctx, "update:skill-error", err.Error())
			}
		}
	}

	return nil
}
