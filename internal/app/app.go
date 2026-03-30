package app

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"sync"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/sftp_svc"
	"github.com/opskat/opskat/internal/service/ssh_svc"
	"github.com/opskat/opskat/internal/sshpool"
	"github.com/opskat/opskat/pkg/extension"

	"github.com/cago-frame/cago/pkg/i18n"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// SkillContent 内嵌的 skill/plugin 文件内容（由 main.go 通过 go:embed 注入）
type SkillContent struct {
	SkillMD               string
	CommandsMD            string
	InitMD                string
	PluginJSON            string
	MarketplaceJSON       string
	PluginMarketplaceJSON string // 插件内嵌的 marketplace.json（自引用，用于插件缓存目录）
}

// SSHConnectEvent SSH 异步连接进度事件
type SSHConnectEvent struct {
	Type        string   `json:"type"`                  // "progress" | "connected" | "error" | "auth_challenge" | "host_key_verify"
	Step        string   `json:"step,omitempty"`        // 当前阶段: "resolve" | "connect" | "auth" | "shell"
	Message     string   `json:"message,omitempty"`     // type=progress 时的进度消息
	SessionID   string   `json:"sessionId,omitempty"`   // type=connected 时返回的会话ID
	Error       string   `json:"error,omitempty"`       // type=error 时的错误信息
	AuthFailed  bool     `json:"authFailed,omitempty"`  // type=error 时是否为认证失败
	ChallengeID string   `json:"challengeId,omitempty"` // type=auth_challenge 时的质询ID
	Prompts     []string `json:"prompts,omitempty"`     // type=auth_challenge 时的提示列表
	Echo        []bool   `json:"echo,omitempty"`        // type=auth_challenge 时是否回显
	// host_key_verify 事件
	HostKeyVerifyID string                `json:"hostKeyVerifyId,omitempty"` // 校验请求ID
	HostKeyEvent    *ssh_svc.HostKeyEvent `json:"hostKeyEvent,omitempty"`    // 主机密钥事件
}

// App Wails应用主结构体，替代controller层
type App struct {
	ctx                     context.Context
	lang                    string
	skillContent            SkillContent
	sshManager              *ssh_svc.Manager
	sftpService             *sftp_svc.Service
	forwardManager          *ForwardManager
	aiAgent                 *ai.Agent
	githubAuthCancel        context.CancelFunc
	permissionChan          chan ai.PermissionResponse // 前端权限响应 channel（CLI 工具用）
	pendingAIApprovals      sync.Map                   // map[string]chan ai.ApprovalResponse（AI 审批用）
	pendingOpsctlApprovals  sync.Map                   // map[string]chan ai.ApprovalResponse（opsctl 审批用）
	approvalServer          *approval.Server           // opsctl 审批 Unix socket 服务
	sshPool                 *sshpool.Pool              // opsctl SSH 连接池
	sshProxyServer          *sshpool.Server            // SSH 连接池 Unix socket 服务
	shutdownCh              chan struct{}              // 关闭信号，cleanup 时 close 以解除所有阻塞等待
	pendingAuthResponses    sync.Map                   // map[string]chan []string（keyboard-interactive 认证响应用）
	pendingHostKeyResponses sync.Map                   // map[string]chan ssh_svc.HostKeyAction（主机密钥校验响应用）
	pendingConnections      sync.Map                   // map[string]context.CancelFunc（异步连接取消用）
	mu                      sync.Mutex                 // 保护 connCounter
	connCounter             int64                      // 连接ID计数器
	currentConversationID   int64                      // 当前活跃会话ID
	runners                 sync.Map                   // map[int64]*ai.ConversationRunner
	extManager              *extension.Manager
	extBridge               *extension.Bridge
}

// NewApp 创建App实例
func NewApp(skill SkillContent) *App {
	mgr := ssh_svc.NewManager()
	a := &App{
		lang:           "zh-cn",
		skillContent:   skill,
		sshManager:     mgr,
		sftpService:    sftp_svc.NewService(mgr),
		permissionChan: make(chan ai.PermissionResponse, 1),
		shutdownCh:     make(chan struct{}),
	}
	a.forwardManager = NewForwardManager(&appPoolDialer{sshManager: mgr})
	return a
}

// Startup Wails启动回调
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// 生成 socket 认证 token（每次启动刷新）
	dataDir := bootstrap.AppDataDir()
	authToken, err := bootstrap.GenerateAuthToken(dataDir)
	if err != nil {
		log.Printf("Failed to generate auth token: %v", err)
	}

	a.startApprovalServer(authToken)
	a.startSSHPoolServer(authToken)
	a.startAutoUpdateCheck()
	a.InitAIProvider()
	a.emitSystemStatus()

	// Initialize extension system
	extDir := filepath.Join(dataDir, "extensions")
	a.extBridge = extension.NewBridge()
	a.extManager = extension.NewManager(extDir, func(extName string) extension.HostProvider {
		return extension.NewDefaultHostProvider(extension.DefaultHostConfig{
			Logger:       zap.L(),
			Credentials:  &appCredentialGetter{},
			AssetConfigs: &appAssetConfigGetter{},
			FileDialogs:  &appFileDialogOpener{ctx: a.ctx},
			KV:           &appKVStore{extName: extName},
			ActionEvents: &appActionEventHandler{ctx: a.ctx, extName: extName},
			TunnelDialer: &appTunnelDialer{app: a},
		})
	}, zap.L())

	if _, err := a.extManager.Scan(ctx); err != nil {
		zap.L().Error("scan extensions failed", zap.Error(err))
	}

	// Register loaded extensions into bridge
	for _, ext := range a.extManager.ListExtensions() {
		a.extBridge.Register(ext)
	}

	// Wire exec_tool handler
	ai.SetExecToolExecutor(a.extBridge)

	// Start hot-reload watcher
	if err := a.extManager.Watch(ctx, a.extBridge, func() {
		wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)
	}); err != nil {
		zap.L().Warn("extension watcher failed", zap.Error(err))
	}
}

// Cleanup 关闭审批服务等资源
func (a *App) Cleanup() {
	// 先发送关闭信号，解除所有阻塞等待（审批、权限确认等），避免 wg.Wait 死锁
	close(a.shutdownCh)

	if a.sshProxyServer != nil {
		a.sshProxyServer.Stop()
	}
	if a.sshPool != nil {
		a.sshPool.Close()
	}
	if a.approvalServer != nil {
		a.approvalServer.Stop()
	}
	if a.extManager != nil {
		a.extManager.Close(context.Background())
	}
}

// SetLanguage 前端调用，同步语言设置到后端
func (a *App) SetLanguage(lang string) {
	a.lang = lang
}

// GetLanguage 返回当前语言
func (a *App) GetLanguage() string {
	return a.lang
}

// langCtx 返回带语言设置的context，每个绑定方法内部调用
func (a *App) langCtx() context.Context {
	ctx := i18n.WithLanguage(a.ctx, a.lang)
	ctx = ai.WithPolicyLang(ctx, a.lang)
	return ctx
}

// activateWindow 激活应用窗口到前台（审批弹窗时调用）
func (a *App) activateWindow() {
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// OnSecondInstanceLaunch 第二个实例启动时激活当前窗口
func (a *App) OnSecondInstanceLaunch() {
	a.activateWindow()
}

// --- SSH 凭证解析（被多个文件共用的辅助方法）---

// appPoolDialer 实现 sshpool.PoolDialer，使用 credential_resolver 解析凭据
type appPoolDialer struct {
	sshManager *ssh_svc.Manager
}

func (d *appPoolDialer) DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error) {
	sshCfg, password, key, jumpHosts, err := credential_resolver.Default().ResolveSSHConnectConfig(ctx, assetID)
	if err != nil {
		return nil, nil, err
	}

	cfg := ssh_svc.ConnectConfig{
		Host:              sshCfg.Host,
		Port:              sshCfg.Port,
		Username:          sshCfg.Username,
		AuthType:          sshCfg.AuthType,
		Password:          password,
		Key:               key,
		PrivateKeys:       sshCfg.PrivateKeys,
		AssetID:           assetID,
		Proxy:             sshCfg.Proxy,
		JumpHosts:         jumpHosts,
		HostKeyVerifyFunc: ssh_svc.AutoTrustFirstRejectChangeVerifyFunc(),
	}

	return d.sshManager.Dial(cfg)
}

// resolveSSHCredentials 从 SSHConfig 解析凭据（委托给 credential_resolver）
func (a *App) resolveSSHCredentials(sshCfg *asset_entity.SSHConfig) (password, key string) {
	p, k, err := credential_resolver.Default().ResolveSSHCredentials(a.langCtx(), sshCfg)
	if err != nil {
		logger.Default().Warn("resolve SSH credentials", zap.Error(err))
	}
	return p, k
}

// decryptProxyPassword 解密代理配置中的密码（委托给 credential_resolver）
func (a *App) decryptProxyPassword(proxy *asset_entity.ProxyConfig) *asset_entity.ProxyConfig {
	return credential_resolver.Default().DecryptProxyPassword(proxy)
}

// resolveJumpHosts 递归解析跳板机链（委托给 credential_resolver，含凭据解密）
func (a *App) resolveJumpHosts(jumpHostID int64, maxDepth int) ([]ssh_svc.JumpHostEntry, error) {
	return credential_resolver.Default().ResolveJumpHosts(a.langCtx(), jumpHostID, maxDepth)
}
