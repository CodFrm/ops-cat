package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"ops-cat/internal/ai"
	"ops-cat/internal/approval"
	"ops-cat/internal/bootstrap"
	"ops-cat/internal/connpool"
	"ops-cat/internal/embedded"
	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/model/entity/audit_entity"
	"ops-cat/internal/model/entity/conversation_entity"
	"ops-cat/internal/model/entity/group_entity"
	"ops-cat/internal/model/entity/plan_entity"
	"ops-cat/internal/model/entity/credential_entity"
	"ops-cat/internal/repository/asset_repo"
	"ops-cat/internal/repository/audit_repo"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/repository/plan_repo"
	"ops-cat/internal/service/asset_svc"
	"ops-cat/internal/service/backup_svc"
	"ops-cat/internal/service/conversation_svc"
	"ops-cat/internal/service/credential_mgr_svc"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/internal/service/import_svc"
	"ops-cat/internal/service/sftp_svc"
	"ops-cat/internal/service/ssh_svc"
	"ops-cat/internal/service/update_svc"
	"ops-cat/internal/sshpool"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/i18n"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

//go:embed skill/SKILL.md
var skillMDContent string

//go:embed skill/references/commands.md
var skillCommandsMDContent string

//go:embed skill/references/ops-init.md
var skillOpsInitMDContent string

// ConfirmResponse 命令确认响应
type ConfirmResponse struct {
	Behavior string // "allow" | "allowAll" | "deny"
}

// SSHConnectEvent SSH 异步连接进度事件
type SSHConnectEvent struct {
	Type        string   `json:"type"`                  // "progress" | "connected" | "error" | "auth_challenge"
	Step        string   `json:"step,omitempty"`        // 当前阶段: "resolve" | "connect" | "auth" | "shell"
	Message     string   `json:"message,omitempty"`     // type=progress 时的进度消息
	SessionID   string   `json:"sessionId,omitempty"`   // type=connected 时返回的会话ID
	Error       string   `json:"error,omitempty"`       // type=error 时的错误信息
	AuthFailed  bool     `json:"authFailed,omitempty"`  // type=error 时是否为认证失败
	ChallengeID string   `json:"challengeId,omitempty"` // type=auth_challenge 时的质询ID
	Prompts     []string `json:"prompts,omitempty"`     // type=auth_challenge 时的提示列表
	Echo        []bool   `json:"echo,omitempty"`        // type=auth_challenge 时是否回显
}

// App Wails应用主结构体，替代controller层
type App struct {
	ctx                   context.Context
	lang                  string
	sshManager            *ssh_svc.Manager
	sftpService           *sftp_svc.Service
	forwardManager        *ForwardManager
	aiAgent               *ai.Agent
	aiProvider            ai.Provider // 保留 provider 引用，用于权限回调注入
	githubAuthCancel      context.CancelFunc
	permissionChan        chan ai.PermissionResponse // 前端权限响应 channel（CLI 工具用）
	pendingConfirms       sync.Map                   // map[string]chan ConfirmResponse（run_command 确认用）
	pendingApprovals      sync.Map                   // map[string]chan bool（opsctl 审批用）
	approvalServer        *approval.Server           // opsctl 审批 Unix socket 服务
	approvedSessions      sync.Map                   // map[string]bool（已批准的 session）
	sshPool               *sshpool.Pool              // opsctl SSH 连接池
	sshProxyServer        *sshpool.Server            // SSH 连接池 Unix socket 服务
	pendingAuthResponses  sync.Map                   // map[string]chan []string（keyboard-interactive 认证响应用）
	pendingConnections    sync.Map                   // map[string]context.CancelFunc（异步连接取消用）
	mu                    sync.Mutex                 // 保护 connCounter
	connCounter           int64                      // 连接ID计数器
	currentConversationID int64                      // 当前活跃会话ID
	aiProviderType        string                     // 当前 provider 类型
	aiModel               string                     // 当前模型
}

// NewApp 创建App实例
func NewApp() *App {
	mgr := ssh_svc.NewManager()
	a := &App{
		lang:           "zh-cn",
		sshManager:     mgr,
		sftpService:    sftp_svc.NewService(mgr),
		permissionChan: make(chan ai.PermissionResponse, 1),
	}
	a.forwardManager = NewForwardManager(a)
	return a
}

// SetAIProvider 设置 AI provider 并创建 agent
func (a *App) SetAIProvider(providerType, apiBase, apiKey, model string) error {
	a.aiProviderType = providerType
	a.aiModel = model

	// 创建共用的命令权限检查器
	checker := ai.NewCommandPolicyChecker(a.makeCommandConfirmFunc())

	var provider ai.Provider
	switch providerType {
	case "openai":
		provider = ai.NewOpenAIProvider("OpenAI Compatible", apiBase, apiKey, model)
	case "local_cli":
		// apiBase 作为 CLI 路径，model 作为 CLI 类型
		cliProvider := ai.NewLocalCLIProvider("Local CLI", apiBase, model)
		// 注入权限确认回调：转发到前端，等待用户响应
		cliProvider.OnPermissionRequest = func(req ai.PermissionRequest) ai.PermissionResponse {
			wailsRuntime.EventsEmit(a.ctx, "ai:permission", req)
			return <-a.permissionChan
		}
		// 注入 session 重置回调：清理 approvedSessions
		cliProvider.OnSessionReset = func(sessionID string) {
			a.approvedSessions.Delete(sessionID)
		}
		// 设置工作目录
		if a.currentConversationID > 0 {
			conv, err := conversation_svc.Conversation().Get(a.langCtx(), a.currentConversationID)
			if err == nil && conv.WorkDir != "" {
				cliProvider.SetWorkDir(conv.WorkDir)
			}
		}
		a.aiProvider = cliProvider
		a.aiAgent = ai.NewAgent(cliProvider, nil, checker)
		return nil
	default:
		provider = ai.NewOpenAIProvider(providerType, apiBase, apiKey, model)
	}
	a.aiAgent = ai.NewAgent(provider, ai.NewAuditingExecutor(ai.NewDefaultToolExecutor()), checker)
	return nil
}

// startup Wails启动回调
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startApprovalServer()
	a.startSSHPoolServer()
}

// startApprovalServer 启动 opsctl 审批 Unix socket 服务
func (a *App) startApprovalServer() {
	handler := func(req approval.ApprovalRequest) approval.ApprovalResponse {
		// 数据变更通知：opsctl 通知前端刷新
		if req.Type == "notify" {
			wailsRuntime.EventsEmit(a.ctx, "data:changed", map[string]any{
				"resource": req.Detail,
			})
			return approval.ApprovalResponse{Approved: true}
		}

		// 计划审批
		if req.Type == "plan" {
			return a.handlePlanApproval(req)
		}

		// session 自动放行
		if req.SessionID != "" {
			if _, ok := a.approvedSessions.Load(req.SessionID); ok {
				return approval.ApprovalResponse{Approved: true}
			}
		}

		// 单条审批
		confirmID := fmt.Sprintf("opsctl_%d", time.Now().UnixNano())

		wailsRuntime.EventsEmit(a.ctx, "opsctl:approval", map[string]any{
			"confirm_id": confirmID,
			"type":       req.Type,
			"asset_id":   req.AssetID,
			"asset_name": req.AssetName,
			"command":    req.Command,
			"detail":     req.Detail,
			"session_id": req.SessionID,
		})

		ch := make(chan bool, 1)
		a.pendingApprovals.Store(confirmID, ch)
		defer a.pendingApprovals.Delete(confirmID)

		select {
		case approved := <-ch:
			if approved {
				return approval.ApprovalResponse{Approved: true}
			}
			return approval.ApprovalResponse{Approved: false, Reason: "user denied"}
		case <-a.ctx.Done():
			return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
		}
	}

	srv := approval.NewServer(handler)
	sockPath := approval.SocketPath(bootstrap.AppDataDir())
	if err := srv.Start(sockPath); err != nil {
		log.Printf("Approval server failed to start: %v", err)
		return
	}
	a.approvalServer = srv
}

// startSSHPoolServer 启动 SSH 连接池 proxy 服务
func (a *App) startSSHPoolServer() {
	dialer := &appPoolDialer{app: a}
	a.sshPool = sshpool.NewPool(dialer, 5*time.Minute)
	a.sshProxyServer = sshpool.NewServer(a.sshPool)
	sockPath := sshpool.SocketPath(bootstrap.AppDataDir())
	if err := a.sshProxyServer.Start(sockPath); err != nil {
		log.Printf("SSH pool server failed to start: %v", err)
		return
	}
}

// appPoolDialer 实现 sshpool.PoolDialer，复用 app 的凭据解析和跳板机逻辑
type appPoolDialer struct {
	app *App
}

func (d *appPoolDialer) DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error) {
	asset, err := asset_svc.Asset().Get(d.app.langCtx(), assetID)
	if err != nil {
		return nil, nil, fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return nil, nil, fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, nil, err
	}

	// 解析凭证
	password, key := d.app.resolveSSHCredentials(sshCfg)

	cfg := ssh_svc.ConnectConfig{
		Host:        sshCfg.Host,
		Port:        sshCfg.Port,
		Username:    sshCfg.Username,
		AuthType:    sshCfg.AuthType,
		Password:    password,
		Key:         key,
		PrivateKeys: sshCfg.PrivateKeys,
		AssetID:     assetID,
		Proxy:       sshCfg.Proxy,
	}

	// 解析跳板机链（含凭据）
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err := d.app.resolveJumpHostsWithCredentials(sshCfg.JumpHostID, 5)
		if err != nil {
			return nil, nil, fmt.Errorf("解析跳板机失败: %w", err)
		}
		cfg.JumpHosts = jumpHosts
	}

	return d.app.sshManager.Dial(cfg)
}

// resolveSSHCredentials 从 SSHConfig 解析凭据（统一凭证优先，兼容内联密码和本地密钥文件）
func (a *App) resolveSSHCredentials(sshCfg *asset_entity.SSHConfig) (password, key string) {
	ctx := a.langCtx()
	// 优先使用统一凭证
	if sshCfg.CredentialID > 0 {
		cred, err := credential_mgr_svc.Get(ctx, sshCfg.CredentialID)
		if err == nil {
			switch cred.Type {
			case credential_entity.TypePassword:
				decrypted, err := credential_svc.Default().Decrypt(cred.Password)
				if err == nil {
					return decrypted, ""
				}
			case credential_entity.TypeSSHKey:
				privKey, err := credential_mgr_svc.GetDecryptedPrivateKey(ctx, sshCfg.CredentialID)
				if err == nil {
					return "", privKey
				}
			}
		}
	}
	// 向后兼容：内联密码
	if sshCfg.AuthType == "password" && sshCfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(sshCfg.Password)
		if err == nil {
			return decrypted, ""
		}
	}
	return "", ""
}

// decryptProxyPassword 解密代理配置中的密码，返回新的 ProxyConfig（不修改原始对象）
func (a *App) decryptProxyPassword(proxy *asset_entity.ProxyConfig) *asset_entity.ProxyConfig {
	if proxy == nil || proxy.Password == "" {
		return proxy
	}
	decrypted, err := credential_svc.Default().Decrypt(proxy.Password)
	if err != nil {
		// 解密失败，可能是明文（测试连接场景），原样返回
		return proxy
	}
	cp := *proxy
	cp.Password = decrypted
	return &cp
}

// resolveJumpHostsWithCredentials 递归解析跳板机链（含凭据解密）
func (a *App) resolveJumpHostsWithCredentials(jumpHostID int64, maxDepth int) ([]ssh_svc.JumpHostEntry, error) {
	if maxDepth <= 0 {
		return nil, fmt.Errorf("跳板机链过深，可能存在循环引用")
	}

	jumpAsset, err := asset_svc.Asset().Get(a.langCtx(), jumpHostID)
	if err != nil {
		return nil, fmt.Errorf("跳板机资产不存在(ID=%d): %w", jumpHostID, err)
	}
	jumpCfg, err := jumpAsset.GetSSHConfig()
	if err != nil {
		return nil, err
	}

	// 解析跳板机凭证
	password, key := a.resolveSSHCredentials(jumpCfg)

	entry := ssh_svc.JumpHostEntry{
		Host:     jumpCfg.Host,
		Port:     jumpCfg.Port,
		Username: jumpCfg.Username,
		AuthType: jumpCfg.AuthType,
		Password: password,
		Key:      key,
	}

	if jumpCfg.JumpHostID > 0 {
		parentHosts, err := a.resolveJumpHostsWithCredentials(jumpCfg.JumpHostID, maxDepth-1)
		if err != nil {
			return nil, err
		}
		return append(parentHosts, entry), nil
	}

	return []ssh_svc.JumpHostEntry{entry}, nil
}

// GetSSHPoolConnections 返回连接池中的活跃连接信息（供前端展示）
func (a *App) GetSSHPoolConnections() []sshpool.PoolEntryInfo {
	if a.sshPool == nil {
		return nil
	}
	return a.sshPool.List()
}

// handlePlanApproval 处理批量计划审批
func (a *App) handlePlanApproval(req approval.ApprovalRequest) approval.ApprovalResponse {
	ctx := a.langCtx()
	sessionID := req.SessionID

	// 写入 DB
	session := &plan_entity.PlanSession{
		ID:          sessionID,
		Description: req.Description,
		Status:      plan_entity.PlanStatusPending,
		Createtime:  time.Now().Unix(),
	}
	if err := plan_repo.Plan().CreateSession(ctx, session); err != nil {
		return approval.ApprovalResponse{Approved: false, Reason: "failed to create plan session"}
	}

	var items []*plan_entity.PlanItem
	for i, pi := range req.PlanItems {
		items = append(items, &plan_entity.PlanItem{
			PlanSessionID: sessionID,
			ItemIndex:     i,
			ToolName:      pi.Type,
			AssetID:       pi.AssetID,
			AssetName:     pi.AssetName,
			Command:       pi.Command,
			Detail:        pi.Detail,
		})
	}
	if err := plan_repo.Plan().CreateItems(ctx, items); err != nil {
		return approval.ApprovalResponse{Approved: false, Reason: "failed to create plan items"}
	}

	// 构建前端事件数据
	eventItems := make([]map[string]any, 0, len(req.PlanItems))
	for _, pi := range req.PlanItems {
		eventItems = append(eventItems, map[string]any{
			"type":       pi.Type,
			"asset_id":   pi.AssetID,
			"asset_name": pi.AssetName,
			"command":    pi.Command,
			"detail":     pi.Detail,
		})
	}

	wailsRuntime.EventsEmit(a.ctx, "opsctl:plan-approval", map[string]any{
		"session_id":  sessionID,
		"description": req.Description,
		"items":       eventItems,
	})

	// 等待前端响应
	ch := make(chan bool, 1)
	a.pendingApprovals.Store(sessionID, ch)
	defer a.pendingApprovals.Delete(sessionID)

	select {
	case approved := <-ch:
		if approved {
			_ = plan_repo.Plan().UpdateSessionStatus(ctx, sessionID, plan_entity.PlanStatusApproved)
			return approval.ApprovalResponse{Approved: true, SessionID: sessionID}
		}
		_ = plan_repo.Plan().UpdateSessionStatus(ctx, sessionID, plan_entity.PlanStatusRejected)
		return approval.ApprovalResponse{Approved: false, Reason: "user denied", SessionID: sessionID}
	case <-a.ctx.Done():
		_ = plan_repo.Plan().UpdateSessionStatus(ctx, sessionID, plan_entity.PlanStatusRejected)
		return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
	}
}

// RespondOpsctlApproval 前端响应 opsctl 审批请求
func (a *App) RespondOpsctlApproval(confirmID string, approved bool) {
	if v, ok := a.pendingApprovals.Load(confirmID); ok {
		ch := v.(chan bool)
		select {
		case ch <- approved:
		default:
		}
	}
}

// RespondPlanApproval 前端响应计划审批请求
func (a *App) RespondPlanApproval(sessionID string, approved bool) {
	a.RespondOpsctlApproval(sessionID, approved)
}

// RespondOpsctlApprovalSession 前端响应审批并可选允许整个 session
func (a *App) RespondOpsctlApprovalSession(confirmID string, approved bool, approveSession bool, sessionID string) {
	if approved && approveSession && sessionID != "" {
		a.approvedSessions.Store(sessionID, true)
	}
	a.RespondOpsctlApproval(confirmID, approved)
}

// cleanup 关闭审批服务等资源
func (a *App) cleanup() {
	if a.sshProxyServer != nil {
		a.sshProxyServer.Stop()
	}
	if a.sshPool != nil {
		a.sshPool.Close()
	}
	if a.approvalServer != nil {
		a.approvalServer.Stop()
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
	return i18n.WithLanguage(a.ctx, a.lang)
}

// --- 资产操作 ---

// GetAsset 获取资产详情
func (a *App) GetAsset(id int64) (*asset_entity.Asset, error) {
	return asset_svc.Asset().Get(a.langCtx(), id)
}

// ListAssets 列出资产
func (a *App) ListAssets(assetType string, groupID int64) ([]*asset_entity.Asset, error) {
	return asset_svc.Asset().List(a.langCtx(), assetType, groupID)
}

// CreateAsset 创建资产
func (a *App) CreateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Create(a.langCtx(), asset)
}

// UpdateAsset 更新资产
func (a *App) UpdateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Update(a.langCtx(), asset)
}

// DeleteAsset 删除资产
func (a *App) DeleteAsset(id int64) error {
	return asset_svc.Asset().Delete(a.langCtx(), id)
}

// MoveAsset 移动资产排序（up/down/top）
func (a *App) MoveAsset(id int64, direction string) error {
	ctx := a.langCtx()
	asset, err := asset_repo.Asset().Find(ctx, id)
	if err != nil {
		return err
	}
	// 获取同组所有资产（已按 sort_order ASC, id ASC 排序）
	siblings, err := asset_repo.Asset().List(ctx, asset_repo.ListOptions{GroupID: asset.GroupID, ExactGroupID: true})
	if err != nil {
		return err
	}
	return moveItem(ctx, id, direction, siblings,
		func(item *asset_entity.Asset) int64 { return item.ID },
		func(item *asset_entity.Asset) int { return item.SortOrder },
		func(itemID int64, order int) error {
			return asset_repo.Asset().UpdateSortOrder(ctx, itemID, order)
		},
	)
}

// MoveGroup 移动分组排序（up/down/top）
func (a *App) MoveGroup(id int64, direction string) error {
	ctx := a.langCtx()
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}
	// 获取同级分组
	allGroups, err := group_repo.Group().List(ctx)
	if err != nil {
		return err
	}
	var siblings []*group_entity.Group
	for _, g := range allGroups {
		if g.ParentID == group.ParentID {
			siblings = append(siblings, g)
		}
	}
	return moveItem(ctx, id, direction, siblings,
		func(item *group_entity.Group) int64 { return item.ID },
		func(item *group_entity.Group) int { return item.SortOrder },
		func(itemID int64, order int) error {
			return group_repo.Group().UpdateSortOrder(ctx, itemID, order)
		},
	)
}

// moveItem 通用排序移动逻辑
func moveItem[T any](ctx context.Context, id int64, direction string, items []T,
	getID func(T) int64, getOrder func(T) int, updateOrder func(int64, int) error,
) error {
	idx := -1
	for i, item := range items {
		if getID(item) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("item not found")
	}

	switch direction {
	case "up":
		if idx == 0 {
			return nil
		}
		// 交换当前项和上一项的 sort_order
		prevOrder := getOrder(items[idx-1])
		curOrder := getOrder(items[idx])
		if prevOrder == curOrder {
			curOrder = prevOrder + 1
		}
		if err := updateOrder(getID(items[idx]), prevOrder); err != nil {
			return err
		}
		return updateOrder(getID(items[idx-1]), curOrder)
	case "down":
		if idx == len(items)-1 {
			return nil
		}
		nextOrder := getOrder(items[idx+1])
		curOrder := getOrder(items[idx])
		if nextOrder == curOrder {
			nextOrder = curOrder + 1
		}
		if err := updateOrder(getID(items[idx]), nextOrder); err != nil {
			return err
		}
		return updateOrder(getID(items[idx+1]), curOrder)
	case "top":
		if idx == 0 {
			return nil
		}
		// 将目标项的 sort_order 设为比第一项更小
		firstOrder := getOrder(items[0])
		return updateOrder(id, firstOrder-1)
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}
}

// --- 分组操作 ---

// ListGroups 列出所有分组
func (a *App) ListGroups() ([]*group_entity.Group, error) {
	return group_repo.Group().List(a.langCtx())
}

// GetGroup 获取单个分组详情
func (a *App) GetGroup(id int64) (*group_entity.Group, error) {
	return group_repo.Group().Find(a.langCtx(), id)
}

// CreateGroup 创建分组
func (a *App) CreateGroup(group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	return group_repo.Group().Create(a.langCtx(), group)
}

// UpdateGroup 更新分组
func (a *App) UpdateGroup(group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	return group_repo.Group().Update(a.langCtx(), group)
}

// DeleteGroup 删除分组
// deleteAssets: true 删除分组下的资产，false 移动到未分组
func (a *App) DeleteGroup(id int64, deleteAssets bool) error {
	ctx := a.langCtx()
	// 获取分组信息，用于将子分组挂到父分组
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}
	// 子分组挂到被删分组的父级
	if err := group_repo.Group().ReparentChildren(ctx, id, group.ParentID); err != nil {
		return err
	}
	// 处理分组下的资产
	if deleteAssets {
		if err := asset_repo.Asset().DeleteByGroupID(ctx, id); err != nil {
			return err
		}
	} else {
		if err := asset_repo.Asset().MoveToGroup(ctx, id, 0); err != nil {
			return err
		}
	}
	return group_repo.Group().Delete(ctx, id)
}

// --- SSH 操作 ---

// SSHConnectRequest 前端 SSH 连接请求
type SSHConnectRequest struct {
	AssetID  int64  `json:"assetId"`
	Password string `json:"password"`
	Key      string `json:"key"`
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
}

// ConnectSSH 连接 SSH 服务器，返回会话 ID
func (a *App) ConnectSSH(req SSHConnectRequest) (string, error) {
	// 获取资产信息
	asset, err := asset_svc.Asset().Get(a.langCtx(), req.AssetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return "", fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return "", err
	}

	// 解析存储的凭证
	storedPassword, storedKey := a.resolveSSHCredentials(sshCfg)
	password := req.Password
	key := req.Key
	if password == "" {
		password = storedPassword
	}
	if key == "" {
		key = storedKey
	}

	connectCfg := ssh_svc.ConnectConfig{
		Host:        sshCfg.Host,
		Port:        sshCfg.Port,
		Username:    sshCfg.Username,
		AuthType:    sshCfg.AuthType,
		Password:    password,
		Key:         key,
		PrivateKeys: sshCfg.PrivateKeys,
		AssetID:     req.AssetID,
		Cols:        req.Cols,
		Rows:        req.Rows,
		Proxy:       a.decryptProxyPassword(sshCfg.Proxy),
		OnData: func(sid string, data []byte) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
		},
		OnClosed: func(sid string) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
		},
	}

	// 解析跳板机链（递归，最大深度 5）
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err := a.resolveJumpHosts(sshCfg.JumpHostID, 5)
		if err != nil {
			return "", fmt.Errorf("解析跳板机失败: %w", err)
		}
		connectCfg.JumpHosts = jumpHosts
	}

	sessionID, err := a.sshManager.Connect(connectCfg)
	if err != nil {
		if isSSHAuthError(err) {
			return "", fmt.Errorf("AUTH_FAILED:%s", err.Error())
		}
		return "", err
	}
	return sessionID, nil
}

// isSSHAuthError 判断是否为 SSH 认证失败错误
func isSSHAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain")
}

// ConnectSSHAsync 异步连接 SSH 服务器，立即返回 connectionId，通过事件推送进度
func (a *App) ConnectSSHAsync(req SSHConnectRequest) (string, error) {
	// 前置校验（同步）
	asset, err := asset_svc.Asset().Get(a.langCtx(), req.AssetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return "", fmt.Errorf("资产不是SSH类型")
	}

	// 生成 connectionId
	a.mu.Lock()
	a.connCounter++
	connectionId := fmt.Sprintf("conn-%d", a.connCounter)
	a.mu.Unlock()

	// 创建可取消的 context
	connCtx, cancel := context.WithCancel(a.ctx)
	a.pendingConnections.Store(connectionId, cancel)

	eventName := "ssh:connect:" + connectionId

	emitEvent := func(event SSHConnectEvent) {
		wailsRuntime.EventsEmit(a.ctx, eventName, event)
	}

	go func() {
		defer func() {
			a.pendingConnections.Delete(connectionId)
		}()

		emitEvent(SSHConnectEvent{Type: "progress", Step: "resolve", Message: "正在解析凭证..."})

		sshCfg, err := asset.GetSSHConfig()
		if err != nil {
			emitEvent(SSHConnectEvent{Type: "error", Error: err.Error()})
			return
		}

		// 检查是否已取消
		if connCtx.Err() != nil {
			return
		}

		// 解析凭证
		storedPassword, storedKey := a.resolveSSHCredentials(sshCfg)
		password := req.Password
		key := req.Key
		if password == "" {
			password = storedPassword
		}
		if key == "" {
			key = storedKey
		}

		connectCfg := ssh_svc.ConnectConfig{
			Host:        sshCfg.Host,
			Port:        sshCfg.Port,
			Username:    sshCfg.Username,
			AuthType:    sshCfg.AuthType,
			Password:    password,
			Key:         key,
			PrivateKeys: sshCfg.PrivateKeys,
			AssetID:     req.AssetID,
			Cols:        req.Cols,
			Rows:        req.Rows,
			Proxy:       a.decryptProxyPassword(sshCfg.Proxy),
			OnData: func(sid string, data []byte) {
				wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
			},
			OnClosed: func(sid string) {
				wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
			},
			OnProgress: func(step, message string) {
				emitEvent(SSHConnectEvent{Type: "progress", Step: step, Message: message})
			},
			OnAuthChallenge: func(prompts []string, echo []bool) ([]string, error) {
				challengeID := fmt.Sprintf("auth_%s_%d", connectionId, time.Now().UnixNano())
				emitEvent(SSHConnectEvent{
					Type:        "auth_challenge",
					ChallengeID: challengeID,
					Prompts:     prompts,
					Echo:        echo,
				})

				ch := make(chan []string, 1)
				a.pendingAuthResponses.Store(challengeID, ch)
				defer a.pendingAuthResponses.Delete(challengeID)

				select {
				case answers := <-ch:
					return answers, nil
				case <-connCtx.Done():
					return nil, fmt.Errorf("连接已取消")
				}
			},
		}

		// 解析跳板机链
		if sshCfg.JumpHostID > 0 {
			emitEvent(SSHConnectEvent{Type: "progress", Step: "resolve", Message: "正在解析跳板机链..."})
			jumpHosts, err := a.resolveJumpHosts(sshCfg.JumpHostID, 5)
			if err != nil {
				emitEvent(SSHConnectEvent{Type: "error", Error: fmt.Sprintf("解析跳板机失败: %s", err.Error())})
				return
			}
			connectCfg.JumpHosts = jumpHosts
		}

		// 检查是否已取消
		if connCtx.Err() != nil {
			return
		}

		sessionID, err := a.sshManager.Connect(connectCfg)
		if err != nil {
			emitEvent(SSHConnectEvent{
				Type:       "error",
				Error:      err.Error(),
				AuthFailed: isSSHAuthError(err),
			})
			return
		}

		emitEvent(SSHConnectEvent{Type: "connected", SessionID: sessionID})
	}()

	return connectionId, nil
}

// RespondAuthChallenge 前端响应 keyboard-interactive 认证质询
func (a *App) RespondAuthChallenge(challengeID string, answers []string) {
	if v, ok := a.pendingAuthResponses.Load(challengeID); ok {
		ch := v.(chan []string)
		select {
		case ch <- answers:
		default:
		}
	}
}

// CancelSSHConnect 取消异步 SSH 连接
func (a *App) CancelSSHConnect(connectionId string) {
	if v, ok := a.pendingConnections.Load(connectionId); ok {
		cancel := v.(context.CancelFunc)
		cancel()
	}
}

// UpdateAssetPassword 更新资产的保存密码
func (a *App) UpdateAssetPassword(assetID int64, password string) error {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return err
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return err
	}
	encrypted, err := credential_svc.Default().Encrypt(password)
	if err != nil {
		return err
	}
	sshCfg.Password = encrypted
	if err := asset.SetSSHConfig(sshCfg); err != nil {
		return err
	}
	return asset_svc.Asset().Update(a.langCtx(), asset)
}

// resolveJumpHosts 递归解析跳板机链，返回从第一跳到最后一跳的顺序
func (a *App) resolveJumpHosts(jumpHostID int64, maxDepth int) ([]ssh_svc.JumpHostEntry, error) {
	if maxDepth <= 0 {
		return nil, fmt.Errorf("跳板机链过深，可能存在循环引用")
	}

	jumpAsset, err := asset_svc.Asset().Get(a.langCtx(), jumpHostID)
	if err != nil {
		return nil, fmt.Errorf("跳板机资产不存在(ID=%d): %w", jumpHostID, err)
	}
	jumpCfg, err := jumpAsset.GetSSHConfig()
	if err != nil {
		return nil, err
	}

	entry := ssh_svc.JumpHostEntry{
		Host:     jumpCfg.Host,
		Port:     jumpCfg.Port,
		Username: jumpCfg.Username,
		AuthType: jumpCfg.AuthType,
	}

	// 如果跳板机自身也有跳板机，递归解析
	if jumpCfg.JumpHostID > 0 {
		parentHosts, err := a.resolveJumpHosts(jumpCfg.JumpHostID, maxDepth-1)
		if err != nil {
			return nil, err
		}
		// 父级跳板机在前，当前在后
		return append(parentHosts, entry), nil
	}

	return []ssh_svc.JumpHostEntry{entry}, nil
}

// TestSSHConnection 测试 SSH 连接（不创建终端会话）
// configJSON: SSHConfig JSON，plainPassword: 明文密码（前端表单直接传入）
func (a *App) TestSSHConnection(configJSON string, plainPassword string) error {
	var sshCfg asset_entity.SSHConfig
	if err := json.Unmarshal([]byte(configJSON), &sshCfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}

	storedPassword, key := a.resolveSSHCredentials(&sshCfg)
	password := plainPassword
	if password == "" {
		password = storedPassword
	}

	connectCfg := ssh_svc.ConnectConfig{
		Host:        sshCfg.Host,
		Port:        sshCfg.Port,
		Username:    sshCfg.Username,
		AuthType:    sshCfg.AuthType,
		Password:    password,
		Key:         key,
		PrivateKeys: sshCfg.PrivateKeys,
		Proxy:       sshCfg.Proxy,
	}

	// 解析跳板机
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err := a.resolveJumpHosts(sshCfg.JumpHostID, 5)
		if err != nil {
			return fmt.Errorf("解析跳板机失败: %w", err)
		}
		connectCfg.JumpHosts = jumpHosts
	}

	return a.sshManager.TestConnection(connectCfg)
}

// TestDatabaseConnection 测试数据库连接
// configJSON: DatabaseConfig JSON，plainPassword: 明文密码
func (a *App) TestDatabaseConnection(configJSON string, plainPassword string) error {
	var cfg asset_entity.DatabaseConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	if plainPassword != "" {
		cfg.Password = plainPassword
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	db, tunnel, err := connpool.DialDatabase(ctx, &cfg, a.sshPool)
	if err != nil {
		return err
	}
	defer db.Close()
	if tunnel != nil {
		defer tunnel.Close()
	}
	return nil
}

// TestRedisConnection 测试 Redis 连接
// configJSON: RedisConfig JSON，plainPassword: 明文密码
func (a *App) TestRedisConnection(configJSON string, plainPassword string) error {
	var cfg asset_entity.RedisConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	if plainPassword != "" {
		cfg.Password = plainPassword
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialRedis(ctx, &cfg, a.sshPool)
	if err != nil {
		return err
	}
	defer client.Close()
	if tunnel != nil {
		defer tunnel.Close()
	}
	return nil
}

// ExecuteSQL 在指定数据库资产上执行 SQL 查询
func (a *App) ExecuteSQL(assetID int64, sqlText string, database string) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsDatabase() {
		return "", fmt.Errorf("资产不是数据库类型")
	}
	cfg, err := asset.GetDatabaseConfig()
	if err != nil {
		return "", fmt.Errorf("获取数据库配置失败: %w", err)
	}
	if database != "" {
		cfg.Database = database
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	db, tunnel, err := connpool.DialDatabase(ctx, cfg, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.Close()
	if tunnel != nil {
		defer tunnel.Close()
	}

	return ai.ExecuteSQL(ctx, db, sqlText)
}

// ExecuteRedis 在指定 Redis 资产上执行命令
func (a *App) ExecuteRedis(assetID int64, command string) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsRedis() {
		return "", fmt.Errorf("资产不是 Redis 类型")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("获取 Redis 配置失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialRedis(ctx, cfg, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 Redis 失败: %w", err)
	}
	defer client.Close()
	if tunnel != nil {
		defer tunnel.Close()
	}

	return ai.ExecuteRedis(ctx, client, command)
}

// WriteSSH 向 SSH 终端写入数据（base64 编码）
func (a *App) WriteSSH(sessionID string, dataB64 string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return fmt.Errorf("解码数据失败: %w", err)
	}
	return sess.Write(data)
}

// ResizeSSH 调整终端尺寸
func (a *App) ResizeSSH(sessionID string, cols int, rows int) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	return sess.Resize(cols, rows)
}

// SplitSSH 在已有会话的连接上创建新会话（分割窗格复用连接）
func (a *App) SplitSSH(existingSessionID string, cols, rows int) (string, error) {
	return a.sshManager.NewSessionFrom(existingSessionID, cols, rows,
		func(sid string, data []byte) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
		},
		func(sid string) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
		},
	)
}

// DisconnectSSH 断开 SSH 连接
func (a *App) DisconnectSSH(sessionID string) {
	a.sshManager.Disconnect(sessionID)
}

// --- SFTP 文件传输 ---

// SFTPGetwd 获取远程工作目录（用户 home）
func (a *App) SFTPGetwd(sessionID string) (string, error) {
	return a.sftpService.Getwd(sessionID)
}

// SFTPListDir 列出远程目录内容
func (a *App) SFTPListDir(sessionID, dirPath string) ([]sftp_svc.FileEntry, error) {
	return a.sftpService.ListDir(sessionID, dirPath)
}

// SFTPUpload 上传文件：弹出本地文件选择 → 上传到 remotePath
func (a *App) SFTPUpload(sessionID, remotePath string) (string, error) {
	localPath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件",
	})
	if err != nil {
		return "", fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil // 用户取消
	}

	// 如果 remotePath 以 / 结尾，则拼接本地文件名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localPath)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Upload(a.ctx, transferID, sessionID, localPath, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPUploadDir 上传目录：弹出本地目录选择 → 上传到 remotePath
func (a *App) SFTPUploadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件夹",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// remotePath 拼接本地目录名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localDir)
	} else {
		remotePath += "/" + filepath.Base(localDir)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.UploadDir(a.ctx, transferID, sessionID, localDir, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownload 下载文件：remotePath → 弹出本地保存对话框
func (a *App) SFTPDownload(sessionID, remotePath string) (string, error) {
	// 以远程文件名作为默认文件名
	defaultName := filepath.Base(remotePath)
	localPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "保存到本地",
	})
	if err != nil {
		return "", fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Download(a.ctx, transferID, sessionID, remotePath, localPath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownloadDir 下载目录：remotePath → 弹出本地目录选择
func (a *App) SFTPDownloadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择保存目录",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// 本地目录 + 远程目录名
	localDir = filepath.Join(localDir, filepath.Base(remotePath))

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.DownloadDir(a.ctx, transferID, sessionID, remotePath, localDir, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPUploadFile 直接上传本地文件或目录（不弹对话框，用于拖拽上传）
func (a *App) SFTPUploadFile(sessionID, localPath, remotePath string) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}

	transferID := a.sftpService.GenerateTransferID()
	emitProgress := func(p sftp_svc.TransferProgress) {
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
	}
	emitDone := func(err error) {
		if err != nil {
			emitProgress(sftp_svc.TransferProgress{TransferID: transferID, Status: "error", Error: err.Error()})
			return
		}
		emitProgress(sftp_svc.TransferProgress{TransferID: transferID, Status: "done"})
	}

	if info.IsDir() {
		dirRemotePath := remotePath
		if strings.HasSuffix(dirRemotePath, "/") {
			dirRemotePath += filepath.Base(localPath)
		} else {
			dirRemotePath += "/" + filepath.Base(localPath)
		}
		go func() {
			emitDone(a.sftpService.UploadDir(a.ctx, transferID, sessionID, localPath, dirRemotePath, emitProgress))
		}()
	} else {
		fileRemotePath := remotePath
		if strings.HasSuffix(fileRemotePath, "/") {
			fileRemotePath += filepath.Base(localPath)
		}
		go func() {
			emitDone(a.sftpService.Upload(a.ctx, transferID, sessionID, localPath, fileRemotePath, emitProgress))
		}()
	}

	return transferID, nil
}

// SFTPCancelTransfer 取消传输
func (a *App) SFTPCancelTransfer(transferID string) {
	a.sftpService.Cancel(transferID)
}

// --- 本地 SSH 密钥发现 ---

// LocalSSHKeyInfo 本地 SSH 密钥信息
type LocalSSHKeyInfo struct {
	Path        string `json:"path"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

// SFTPDelete 删除远程文件或目录
func (a *App) SFTPDelete(sessionID, remotePath string, isDir bool) error {
	if isDir {
		return a.sftpService.RemoveDir(sessionID, remotePath)
	}
	return a.sftpService.Remove(sessionID, remotePath)
}

// ListLocalSSHKeys 扫描 ~/.ssh 目录，返回有效的私钥列表
func (a *App) ListLocalSSHKeys() ([]LocalSSHKeyInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %w", err)
	}
	sshDir := filepath.Join(homeDir, ".ssh")

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		// ~/.ssh 不存在时返回空列表
		if os.IsNotExist(err) {
			return []LocalSSHKeyInfo{}, nil
		}
		return nil, fmt.Errorf("读取 .ssh 目录失败: %w", err)
	}

	// 需要跳过的文件
	skipFiles := map[string]bool{
		"known_hosts":     true,
		"known_hosts.old": true,
		"config":          true,
		"authorized_keys": true,
		"environment":     true,
	}

	var keys []LocalSSHKeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过公钥、已知文件和隐藏文件
		if strings.HasSuffix(name, ".pub") || skipFiles[name] || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".sock") {
			continue
		}

		fullPath := filepath.Join(sshDir, name)
		info, err := parseLocalSSHKey(fullPath)
		if err != nil {
			continue // 不是有效私钥，跳过
		}
		keys = append(keys, *info)
	}

	if keys == nil {
		keys = []LocalSSHKeyInfo{}
	}
	return keys, nil
}

// SelectSSHKeyFile 打开文件选择框选择密钥文件，默认定位到 ~/.ssh
func (a *App) SelectSSHKeyFile() (*LocalSSHKeyInfo, error) {
	homeDir, _ := os.UserHomeDir()
	defaultDir := filepath.Join(homeDir, ".ssh")

	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:            "选择 SSH 私钥文件",
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil // 用户取消
	}

	info, err := parseLocalSSHKey(filePath)
	if err != nil {
		return nil, fmt.Errorf("所选文件不是有效的 SSH 私钥: %w", err)
	}
	return info, nil
}

// parseLocalSSHKey 解析本地私钥文件，返回密钥信息
func parseLocalSSHKey(path string) (*LocalSSHKeyInfo, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from user file dialog
	if err != nil {
		return nil, err
	}
	// 快速检查：私钥文件通常以 "-----BEGIN" 开头或是 OpenSSH 格式
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, err
	}

	pubKey := signer.PublicKey()
	fingerprint := ssh.FingerprintSHA256(pubKey)
	keyType := pubKey.Type()

	return &LocalSSHKeyInfo{
		Path:        path,
		KeyType:     keyType,
		Fingerprint: fingerprint,
	}, nil
}

// --- AI 操作 ---

// ConversationDisplayMessage 返回给前端的会话消息（用于恢复显示）
type ConversationDisplayMessage struct {
	Role    string                             `json:"role"`
	Content string                             `json:"content"`
	Blocks  []conversation_entity.ContentBlock `json:"blocks"`
}

// CreateConversation 创建新会话
func (a *App) CreateConversation() (*conversation_entity.Conversation, error) {
	if a.aiAgent == nil {
		return nil, fmt.Errorf("请先配置 AI Provider")
	}

	ctx := a.langCtx()
	conv := &conversation_entity.Conversation{
		Title:        "新对话",
		ProviderType: a.aiProviderType,
		Model:        a.aiModel,
	}

	// 本地 CLI 模式创建工作目录
	if a.aiProviderType == "local_cli" {
		workDir := filepath.Join(bootstrap.AppDataDir(), "workspaces", fmt.Sprintf("conv-%d", time.Now().UnixMilli()))
		conv.WorkDir = workDir
	}

	if err := conversation_svc.Conversation().Create(ctx, conv); err != nil {
		return nil, err
	}

	// 如果有工作目录，更新路径为带 ID 的稳定路径
	if conv.WorkDir != "" {
		stableDir := filepath.Join(bootstrap.AppDataDir(), "workspaces", fmt.Sprintf("%d", conv.ID))
		if err := os.Rename(conv.WorkDir, stableDir); err == nil {
			conv.WorkDir = stableDir
			_ = conversation_svc.Conversation().Update(ctx, conv)
		}
	}

	// 切换到新会话
	a.switchToConversation(conv)

	return conv, nil
}

// ListConversations 获取会话列表
func (a *App) ListConversations() ([]*conversation_entity.Conversation, error) {
	return conversation_svc.Conversation().List(a.langCtx())
}

// SwitchConversation 切换到指定会话，返回显示消息
func (a *App) SwitchConversation(id int64) ([]ConversationDisplayMessage, error) {
	ctx := a.langCtx()
	conv, err := conversation_svc.Conversation().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}

	a.switchToConversation(conv)

	// 加载消息用于前端显示
	msgs, err := conversation_svc.Conversation().LoadMessages(ctx, id)
	if err != nil {
		return nil, err
	}

	var displayMsgs []ConversationDisplayMessage
	for _, msg := range msgs {
		blocks, _ := msg.GetBlocks()
		displayMsgs = append(displayMsgs, ConversationDisplayMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Blocks:  blocks,
		})
	}
	return displayMsgs, nil
}

// switchToConversation 内部切换会话逻辑
func (a *App) switchToConversation(conv *conversation_entity.Conversation) {
	a.currentConversationID = conv.ID

	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		// 恢复 CLI session
		info, err := conv.GetSessionInfo()
		if err == nil && info.SessionID != "" {
			p.SetSessionID(info.SessionID)
		} else {
			p.SetSessionID("")
		}

		// 切换工作目录
		if conv.WorkDir != "" {
			p.SetWorkDir(conv.WorkDir)
		}
	}
}

// DeleteConversation 删除会话
func (a *App) DeleteConversation(id int64) error {
	err := conversation_svc.Conversation().Delete(a.langCtx(), id)
	if err != nil {
		return err
	}
	// 如果删的是当前会话，清空当前会话ID
	if a.currentConversationID == id {
		a.currentConversationID = 0
	}
	return nil
}

// SendAIMessage 发送 AI 消息，通过 Wails Events 流式返回
// convID 指定目标会话，支持多会话并发发送
func (a *App) SendAIMessage(convID int64, messages []ai.Message) error {
	if a.aiAgent == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}

	ctx := a.langCtx()

	// 自动创建会话（首次发消息时）
	if convID == 0 {
		conv, err := a.CreateConversation()
		if err != nil {
			return fmt.Errorf("创建会话失败: %w", err)
		}
		convID = conv.ID
	}

	// 更新会话标题（如果仍是默认标题"新对话"）
	if conv, err := conversation_svc.Conversation().Get(ctx, convID); err == nil && conv.Title == "新对话" {
		for _, msg := range messages {
			if msg.Role == ai.RoleUser {
				title := string(msg.Content)
				if len([]rune(title)) > 50 {
					title = string([]rune(title)[:50])
				}
				conv.Title = title
				_ = conversation_svc.Conversation().Update(ctx, conv)
				break
			}
		}
	}

	eventName := fmt.Sprintf("ai:event:%d", convID)

	// 添加系统提示
	fullMessages := make([]ai.Message, 0, 1+len(messages))
	fullMessages = append(fullMessages, ai.Message{
		Role:    ai.RoleSystem,
		Content: "You are the Ops Cat AI assistant, helping users manage IT assets. You can list assets, view details, add assets, and run commands on SSH servers. Respond in the same language the user uses.",
	})
	fullMessages = append(fullMessages, messages...)

	go func() {
		// 注入审计上下文
		chatCtx := ai.WithAuditSource(a.ctx, "ai")
		chatCtx = ai.WithConversationID(chatCtx, convID)

		err := a.aiAgent.Chat(chatCtx, fullMessages, func(event ai.StreamEvent) {
			wailsRuntime.EventsEmit(a.ctx, eventName, event)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:  "error",
				Error: err.Error(),
			})
		}

		// 消息完成后持久化
		a.persistConversationState(convID, messages)
	}()

	return nil
}

// persistConversationState 持久化会话状态（消息+session）
func (a *App) persistConversationState(convID int64, messages []ai.Message) {
	ctx := a.langCtx()

	// 保存 local CLI session ID
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		conv, err := conversation_svc.Conversation().Get(ctx, convID)
		if err == nil {
			sessionID := p.GetSessionID()
			_ = conv.SetSessionInfo(&conversation_entity.SessionInfo{
				SessionID: sessionID,
			})
			conv.Updatetime = time.Now().Unix()
			_ = conversation_svc.Conversation().Update(ctx, conv)
		}
	}
}

// SaveConversationMessages 前端调用，保存显示消息到数据库
// convID 指定目标会话，支持多会话独立保存
func (a *App) SaveConversationMessages(convID int64, displayMsgs []ConversationDisplayMessage) error {
	if convID == 0 {
		return nil
	}
	ctx := a.langCtx()
	var msgs []*conversation_entity.Message
	for i, dm := range displayMsgs {
		msg := &conversation_entity.Message{
			ConversationID: convID,
			Role:           dm.Role,
			Content:        dm.Content,
			SortOrder:      i,
			Createtime:     time.Now().Unix(),
		}
		_ = msg.SetBlocks(dm.Blocks)
		msgs = append(msgs, msg)
	}
	return conversation_svc.Conversation().SaveMessages(ctx, convID, msgs)
}

// GetCurrentConversationID 获取当前会话ID
func (a *App) GetCurrentConversationID() int64 {
	return a.currentConversationID
}

// DetectLocalCLIs 检测本地 AI CLI 工具
func (a *App) DetectLocalCLIs() []ai.CLIInfo {
	return ai.DetectLocalCLIs()
}

// RespondPermission 前端响应权限确认请求（CLI 工具用）
func (a *App) RespondPermission(behavior, message string) {
	resp := ai.PermissionResponse{Behavior: behavior, Message: message}
	// Codex 工具确认走 confirmCh
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		if srv := p.GetCodexServer(); srv != nil {
			srv.RespondConfirm(resp)
			return
		}
	}
	select {
	case a.permissionChan <- resp:
	default:
	}
}

// makeCommandConfirmFunc 创建命令确认回调，向 AI 聊天流发送 tool_confirm 事件并阻塞等待
func (a *App) makeCommandConfirmFunc() ai.CommandConfirmFunc {
	return func(assetName, command string) (bool, bool) {
		convID := a.currentConversationID
		confirmID := fmt.Sprintf("cmd_%d_%d", convID, time.Now().UnixNano())
		eventName := fmt.Sprintf("ai:event:%d", convID)

		// 向 AI 聊天流发送 tool_confirm 事件
		wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
			Type:      "tool_confirm",
			ToolName:  "run_command",
			ToolInput: fmt.Sprintf("[%s] $ %s", assetName, command),
			ConfirmID: confirmID,
		})

		// 阻塞等待前端响应
		ch := make(chan ConfirmResponse, 1)
		a.pendingConfirms.Store(confirmID, ch)
		defer a.pendingConfirms.Delete(confirmID)

		select {
		case resp := <-ch:
			// 发送确认结果事件更新 UI 状态
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "tool_confirm_result",
				ConfirmID: confirmID,
				Content:   resp.Behavior,
			})
			return resp.Behavior != "deny", resp.Behavior == "allowAll"
		case <-a.ctx.Done():
			return false, false
		}
	}
}

// RespondCommandConfirm 前端响应 run_command 确认请求
func (a *App) RespondCommandConfirm(confirmID, behavior string) {
	// 先检查普通命令确认（有明确的 confirmID 匹配）
	if v, ok := a.pendingConfirms.Load(confirmID); ok {
		ch := v.(chan ConfirmResponse)
		select {
		case ch <- ConfirmResponse{Behavior: behavior}:
		default:
		}
		return
	}
	// 否则转发到 Codex 工具确认
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		if srv := p.GetCodexServer(); srv != nil {
			srv.RespondConfirm(ai.PermissionResponse{Behavior: behavior})
		}
	}
}

// ResetAISession 重置 AI 会话（创建新会话）
func (a *App) ResetAISession() {
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		p.ResetSession()
	}
	a.currentConversationID = 0
}

// --- 凭证操作 ---

// EncryptPassword 加密密码，返回加密后的字符串（用于前端保存资产配置）
func (a *App) EncryptPassword(plaintext string) (string, error) {
	return credential_svc.Default().Encrypt(plaintext)
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

// ExportData 导出所有资产和分组为 JSON
func (a *App) ExportData() (string, error) {
	data, err := backup_svc.Export(a.langCtx())
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

// ExportToFile 导出备份到文件，password 为空则不加密
func (a *App) ExportToFile(password string) error {
	data, err := backup_svc.Export(a.langCtx())
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
		defaultName = fmt.Sprintf("ops-cat-backup-%s.encrypted.json", time.Now().Format("20060102"))
	} else {
		output = jsonData
		defaultName = fmt.Sprintf("ops-cat-backup-%s.json", time.Now().Format("20060102"))
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
	FilePath  string `json:"filePath"`
	Encrypted bool   `json:"encrypted"`
}

// SelectImportFile 选择备份文件并检测是否加密
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

	return &ImportFileInfo{
		FilePath:  filePath,
		Encrypted: backup_svc.IsEncryptedBackup(fileData),
	}, nil
}

// ExecuteImportFile 执行文件导入
func (a *App) ExecuteImportFile(filePath, password string) error {
	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from previous file dialog selection
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data)
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
func (a *App) ExportToGist(password, token, gistID string) (*backup_svc.GistInfo, error) {
	data, err := backup_svc.Export(a.langCtx())
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

// --- 密钥管理 ---

// ListCredentials 列出所有凭证
func (a *App) ListCredentials() ([]*credential_entity.Credential, error) {
	return credential_mgr_svc.List(a.langCtx())
}

// ListCredentialsByType 按类型列出凭证
func (a *App) ListCredentialsByType(credType string) ([]*credential_entity.Credential, error) {
	return credential_mgr_svc.ListByType(a.langCtx(), credType)
}

// CreatePasswordCredential 创建密码凭证
func (a *App) CreatePasswordCredential(name, username, password, description string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.CreatePassword(a.langCtx(), credential_mgr_svc.CreatePasswordRequest{
		Name:        name,
		Username:    username,
		Password:    password,
		Description: description,
	})
}

// GenerateSSHKey 生成新的 SSH 密钥对
func (a *App) GenerateSSHKey(name, comment, keyType string, keySize int) (*credential_entity.Credential, error) {
	return credential_mgr_svc.GenerateSSHKey(a.langCtx(), credential_mgr_svc.GenerateKeyRequest{
		Name:    name,
		Comment: comment,
		KeyType: keyType,
		KeySize: keySize,
	})
}

// ImportSSHKeyFile 通过文件选择框导入 SSH 密钥
func (a *App) ImportSSHKeyFile(name, comment string) (*credential_entity.Credential, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 SSH 私钥文件",
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}
	return credential_mgr_svc.ImportSSHKeyFromFile(a.langCtx(), name, comment, filePath)
}

// ImportSSHKeyPEM 通过粘贴 PEM 内容导入 SSH 密钥
func (a *App) ImportSSHKeyPEM(name, comment, pemData string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.ImportSSHKeyFromPEM(a.langCtx(), name, comment, pemData)
}

// UpdateCredential 更新凭证
func (a *App) UpdateCredential(id int64, name, comment, description, username string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.Update(a.langCtx(), credential_mgr_svc.UpdateRequest{
		ID:          id,
		Name:        name,
		Comment:     comment,
		Description: description,
		Username:    username,
	})
}

// UpdateCredentialPassword 更新密码凭证的密码
func (a *App) UpdateCredentialPassword(id int64, password string) error {
	return credential_mgr_svc.UpdatePassword(a.langCtx(), id, password)
}

// GetCredentialUsage 获取引用此凭证的资产名称列表
func (a *App) GetCredentialUsage(id int64) ([]string, error) {
	assets, err := asset_repo.Asset().FindByCredentialID(a.langCtx(), id)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(assets))
	for i, asset := range assets {
		names[i] = asset.Name
	}
	return names, nil
}

// DeleteCredential 删除凭证
func (a *App) DeleteCredential(id int64) error {
	return credential_mgr_svc.Delete(a.langCtx(), id)
}

// GetCredentialPublicKey 获取 SSH 密钥凭证的公钥（用于复制）
func (a *App) GetCredentialPublicKey(id int64) (string, error) {
	cred, err := credential_mgr_svc.Get(a.langCtx(), id)
	if err != nil {
		return "", err
	}
	return cred.PublicKey, nil
}

// ImportFromGist 从 Gist 导入备份
func (a *App) ImportFromGist(gistID, password, token string) error {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data)
}

// GetDataDir 返回应用数据目录
func (a *App) GetDataDir() string {
	return bootstrap.AppDataDir()
}

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
func skillMDWithDataDir() string {
	dataDir := bootstrap.AppDataDir()
	insertion := "## Data Directory\n\n" + dataDir + "\n\n"
	return strings.Replace(skillMDContent, "## Global Flags", insertion+"## Global Flags", 1)
}

// installSkillTo 将 Skill 文件安装到指定目录
func installSkillTo(skillDir string) error {
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMDWithDataDir()), 0644); err != nil {
		return fmt.Errorf("write SKILL.md failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "commands.md"), []byte(skillCommandsMDContent), 0644); err != nil {
		return fmt.Errorf("write commands.md failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "ops-init.md"), []byte(skillOpsInitMDContent), 0644); err != nil {
		return fmt.Errorf("write ops-init.md failed: %w", err)
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
		if err := installSkillTo(skillDir); err != nil {
			return fmt.Errorf("install %s skill failed: %w", def.Name, err)
		}
	}

	// 清理旧的 skill 文件
	oldSkillPath := filepath.Join(home, ".claude", "commands", "ops-cat.md")
	_ = os.Remove(oldSkillPath)

	return nil
}

// GetSkillPreview 获取 Skill 文件内容预览
func (a *App) GetSkillPreview() string {
	return "--- SKILL.md ---\n\n" + skillMDWithDataDir() +
		"\n\n--- references/commands.md ---\n\n" + skillCommandsMDContent +
		"\n\n--- references/ops-init.md ---\n\n" + skillOpsInitMDContent
}

// --- 审计日志 ---

// AuditLogListResult 审计日志列表结果
type AuditLogListResult struct {
	Items []*audit_entity.AuditLog `json:"items"`
	Total int64                    `json:"total"`
}

// ListAuditLogs 查询审计日志
func (a *App) ListAuditLogs(source string, assetID int64, offset, limit int) (*AuditLogListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	items, total, err := audit_repo.Audit().List(a.langCtx(), audit_repo.ListOptions{
		Source:  source,
		AssetID: assetID,
		Offset:  offset,
		Limit:   limit,
	})
	if err != nil {
		return nil, err
	}
	return &AuditLogListResult{Items: items, Total: total}, nil
}

// --- 更新 ---

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
			if err := installSkillTo(s.Path); err != nil {
				wailsRuntime.EventsEmit(a.ctx, "update:skill-error", err.Error())
			}
		}
	}

	return nil
}
