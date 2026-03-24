package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"ops-cat/internal/model/entity/forward_entity"
	"ops-cat/internal/repository/forward_repo"
	"ops-cat/internal/service/asset_svc"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/internal/service/ssh_key_svc"
	"ops-cat/internal/service/ssh_svc"

	"golang.org/x/crypto/ssh"
)

// ForwardManager 管理端口转发的运行时状态
type ForwardManager struct {
	app     *App
	mu      sync.Mutex
	clients map[int64]*forwardClient  // assetID → SSH 连接
	running map[int64]*runningForward // ruleID → 运行状态
}

// forwardClient 每个资产对应的 SSH 连接
type forwardClient struct {
	client  *ssh.Client
	closers []io.Closer
	refs    int32
}

// runningForward 单条转发规则的运行状态
type runningForward struct {
	configID int64
	assetID  int64
	cancel   context.CancelFunc
	errMsg   string // 非空表示失败
}

// RuleStatus 返回给前端的规则运行状态
type RuleStatus struct {
	RuleID int64  `json:"ruleId"`
	Status string `json:"status"` // "running" | "error" | "stopped"
	Error  string `json:"error,omitempty"`
}

// ForwardConfigWithStatus 配置 + 规则 + 运行状态
type ForwardConfigWithStatus struct {
	forward_entity.ForwardConfig
	AssetName  string       `json:"assetName"`
	Rules      []RuleWithStatus `json:"rules"`
	Status     string       `json:"status"` // "running" | "partial" | "error" | "stopped"
}

// RuleWithStatus 规则 + 运行状态
type RuleWithStatus struct {
	forward_entity.ForwardRule
	Status string `json:"status"` // "running" | "error" | "stopped"
	Error  string `json:"error,omitempty"`
}

func NewForwardManager(app *App) *ForwardManager {
	return &ForwardManager{
		app:     app,
		clients: make(map[int64]*forwardClient),
		running: make(map[int64]*runningForward),
	}
}

// StartConfig 启动一个转发配置的所有规则
func (m *ForwardManager) StartConfig(ctx context.Context, configID int64) error {
	config, err := forward_repo.Forward().FindConfig(ctx, configID)
	if err != nil {
		return fmt.Errorf("配置不存在: %w", err)
	}
	rules, err := forward_repo.Forward().ListRulesByConfigID(ctx, configID)
	if err != nil {
		return fmt.Errorf("读取规则失败: %w", err)
	}
	if len(rules) == 0 {
		return fmt.Errorf("配置中没有转发规则")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 先停掉已有的（同一配置重新启动）
	m.stopConfigLocked(configID)

	// 获取或创建 SSH 连接
	fc, err := m.getOrDialLocked(ctx, config.AssetID)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}

	// 启动每条规则
	for _, rule := range rules {
		rctx, cancel := context.WithCancel(context.Background())
		rf := &runningForward{
			configID: configID,
			assetID:  config.AssetID,
			cancel:   cancel,
		}
		startErr := startForwardRule(rctx, fc.client, rule)
		if startErr != nil {
			rf.errMsg = startErr.Error()
			cancel()
			log.Printf("[Forward] rule %d failed: %s", rule.ID, startErr)
		}
		m.running[rule.ID] = rf
		atomic.AddInt32(&fc.refs, 1)
	}

	return nil
}

// StopConfig 停止一个转发配置的所有规则
func (m *ForwardManager) StopConfig(configID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopConfigLocked(configID)
}

func (m *ForwardManager) stopConfigLocked(configID int64) {
	for ruleID, rf := range m.running {
		if rf.configID == configID {
			rf.cancel()
			m.releaseClientLocked(rf.assetID)
			delete(m.running, ruleID)
		}
	}
}

// StopAll 停止所有转发
func (m *ForwardManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ruleID, rf := range m.running {
		rf.cancel()
		delete(m.running, ruleID)
	}
	for assetID, fc := range m.clients {
		fc.client.Close()
		for _, c := range fc.closers {
			c.Close()
		}
		delete(m.clients, assetID)
	}
}

// GetConfigStatus 获取指定配置的运行状态
func (m *ForwardManager) GetConfigStatus(configID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	running, errored, total := 0, 0, 0
	for _, rf := range m.running {
		if rf.configID == configID {
			total++
			if rf.errMsg != "" {
				errored++
			} else {
				running++
			}
		}
	}
	if total == 0 {
		return "stopped"
	}
	if errored == total {
		return "error"
	}
	if running == total {
		return "running"
	}
	return "partial"
}

// GetRuleStatus 获取单条规则的运行状态
func (m *ForwardManager) GetRuleStatus(ruleID int64) RuleStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	rf, ok := m.running[ruleID]
	if !ok {
		return RuleStatus{RuleID: ruleID, Status: "stopped"}
	}
	if rf.errMsg != "" {
		return RuleStatus{RuleID: ruleID, Status: "error", Error: rf.errMsg}
	}
	return RuleStatus{RuleID: ruleID, Status: "running"}
}

// IsConfigRunning 检查配置是否有运行中的规则
func (m *ForwardManager) IsConfigRunning(configID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rf := range m.running {
		if rf.configID == configID {
			return true
		}
	}
	return false
}

// --- SSH 连接管理 ---

func (m *ForwardManager) getOrDialLocked(ctx context.Context, assetID int64) (*forwardClient, error) {
	if fc, ok := m.clients[assetID]; ok {
		return fc, nil
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("资产不存在: %w", err)
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, err
	}

	// 解析凭证
	password, key := "", ""
	if sshCfg.AuthType == "password" && sshCfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(sshCfg.Password)
		if err == nil {
			password = decrypted
		}
	}
	if sshCfg.AuthType == "key" && sshCfg.KeySource == "managed" && sshCfg.KeyID > 0 {
		privKey, err := ssh_key_svc.GetPrivateKey(ctx, sshCfg.KeyID)
		if err == nil {
			key = privKey
		}
	}

	dialCfg := ssh_svc.ConnectConfig{
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
		jumpHosts, err := m.app.resolveJumpHostsWithCredentials(sshCfg.JumpHostID, 5)
		if err != nil {
			return nil, fmt.Errorf("解析跳板机失败: %w", err)
		}
		dialCfg.JumpHosts = jumpHosts
	}

	client, closers, err := m.app.sshManager.Dial(dialCfg)
	if err != nil {
		return nil, err
	}

	fc := &forwardClient{client: client, closers: closers}
	m.clients[assetID] = fc
	return fc, nil
}

func (m *ForwardManager) releaseClientLocked(assetID int64) {
	fc, ok := m.clients[assetID]
	if !ok {
		return
	}
	if atomic.AddInt32(&fc.refs, -1) <= 0 {
		fc.client.Close()
		for _, c := range fc.closers {
			c.Close()
		}
		delete(m.clients, assetID)
	}
}

// --- 端口转发启动 ---

func startForwardRule(ctx context.Context, client *ssh.Client, rule *forward_entity.ForwardRule) error {
	switch rule.Type {
	case "local":
		return startLocalForward(ctx, client, rule)
	case "remote":
		return startRemoteForward(ctx, client, rule)
	default:
		return fmt.Errorf("unsupported type: %s", rule.Type)
	}
}

func startLocalForward(ctx context.Context, client *ssh.Client, rule *forward_entity.ForwardRule) error {
	addr := fmt.Sprintf("%s:%d", rule.LocalHost, rule.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				remote := fmt.Sprintf("%s:%d", rule.RemoteHost, rule.RemotePort)
				rconn, err := client.Dial("tcp", remote)
				if err != nil {
					conn.Close()
					return
				}
				pipeConns(conn, rconn)
			}()
		}
	}()

	return nil
}

func startRemoteForward(ctx context.Context, client *ssh.Client, rule *forward_entity.ForwardRule) error {
	addr := fmt.Sprintf("%s:%d", rule.RemoteHost, rule.RemotePort)
	listener, err := client.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("remote listen %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				local := net.JoinHostPort(rule.LocalHost, fmt.Sprintf("%d", rule.LocalPort))
				lconn, err := net.Dial("tcp", local)
				if err != nil {
					conn.Close()
					return
				}
				pipeConns(conn, lconn)
			}()
		}
	}()

	return nil
}

func pipeConns(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
		dst.Close()
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
}

// --- Wails 绑定方法（挂在 App 上）---

// CreateForwardConfig 创建转发配置
func (a *App) CreateForwardConfig(name string, assetID int64, rules []forward_entity.ForwardRule) (*forward_entity.ForwardConfig, error) {
	ctx := a.langCtx()
	now := time.Now().Unix()
	config := &forward_entity.ForwardConfig{
		Name: name, AssetID: assetID,
		Createtime: now, Updatetime: now,
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := forward_repo.Forward().CreateConfig(ctx, config); err != nil {
		return nil, err
	}
	// 写入规则
	rulesPtrs := make([]*forward_entity.ForwardRule, len(rules))
	for i := range rules {
		rulesPtrs[i] = &rules[i]
	}
	if err := forward_repo.Forward().ReplaceRules(ctx, config.ID, rulesPtrs); err != nil {
		return nil, err
	}
	return config, nil
}

// UpdateForwardConfig 更新转发配置（如果正在运行，先停止再更新再启动）
func (a *App) UpdateForwardConfig(id int64, name string, assetID int64, rules []forward_entity.ForwardRule) (*forward_entity.ForwardConfig, error) {
	ctx := a.langCtx()
	config, err := forward_repo.Forward().FindConfig(ctx, id)
	if err != nil {
		return nil, err
	}

	wasRunning := a.forwardManager.IsConfigRunning(id)
	if wasRunning {
		a.forwardManager.StopConfig(id)
	}

	config.Name = name
	config.AssetID = assetID
	config.Updatetime = time.Now().Unix()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := forward_repo.Forward().UpdateConfig(ctx, config); err != nil {
		return nil, err
	}
	rulesPtrs := make([]*forward_entity.ForwardRule, len(rules))
	for i := range rules {
		rulesPtrs[i] = &rules[i]
	}
	if err := forward_repo.Forward().ReplaceRules(ctx, config.ID, rulesPtrs); err != nil {
		return nil, err
	}

	if wasRunning {
		_ = a.forwardManager.StartConfig(ctx, id)
	}

	return config, nil
}

// DeleteForwardConfig 删除转发配置
func (a *App) DeleteForwardConfig(id int64) error {
	a.forwardManager.StopConfig(id)
	ctx := a.langCtx()
	if err := forward_repo.Forward().DeleteRulesByConfigID(ctx, id); err != nil {
		return err
	}
	return forward_repo.Forward().DeleteConfig(ctx, id)
}

// ListForwardConfigs 列出所有转发配置（含规则和运行状态）
func (a *App) ListForwardConfigs() ([]ForwardConfigWithStatus, error) {
	ctx := a.langCtx()
	configs, err := forward_repo.Forward().ListConfigs(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ForwardConfigWithStatus, 0, len(configs))
	for _, c := range configs {
		rules, _ := forward_repo.Forward().ListRulesByConfigID(ctx, c.ID)

		// 获取资产名
		assetName := ""
		if asset, err := asset_svc.Asset().Get(ctx, c.AssetID); err == nil {
			assetName = asset.Name
		}

		rulesWithStatus := make([]RuleWithStatus, 0, len(rules))
		for _, r := range rules {
			rs := a.forwardManager.GetRuleStatus(r.ID)
			rulesWithStatus = append(rulesWithStatus, RuleWithStatus{
				ForwardRule: *r,
				Status:      rs.Status,
				Error:       rs.Error,
			})
		}

		result = append(result, ForwardConfigWithStatus{
			ForwardConfig: *c,
			AssetName:     assetName,
			Rules:         rulesWithStatus,
			Status:        a.forwardManager.GetConfigStatus(c.ID),
		})
	}
	return result, nil
}

// StartForwardConfig 启动转发配置
func (a *App) StartForwardConfig(id int64) error {
	return a.forwardManager.StartConfig(a.langCtx(), id)
}

// StopForwardConfig 停止转发配置
func (a *App) StopForwardConfig(id int64) {
	a.forwardManager.StopConfig(id)
}
