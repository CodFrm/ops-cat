package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opskat/opskat/internal/model/entity/forward_entity"
	"github.com/opskat/opskat/internal/repository/forward_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// SSHDialer SSH 连接接口，供 ForwardManager 使用
type SSHDialer interface {
	DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error)
}

// ForwardManager 管理端口转发的运行时状态
type ForwardManager struct {
	dialer  SSHDialer
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
	AssetName string           `json:"assetName"`
	Rules     []RuleWithStatus `json:"rules"`
	Status    string           `json:"status"` // "running" | "partial" | "error" | "stopped"
}

// RuleWithStatus 规则 + 运行状态
type RuleWithStatus struct {
	forward_entity.ForwardRule
	Status string `json:"status"` // "running" | "error" | "stopped"
	Error  string `json:"error,omitempty"`
}

func NewForwardManager(dialer SSHDialer) *ForwardManager {
	return &ForwardManager{
		dialer:  dialer,
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
			logger.Ctx(ctx).Error("rule failed", zap.Int64("ruleID", rule.ID), zap.Error(startErr))
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
		if err := fc.client.Close(); err != nil {
			logger.Default().Warn("close SSH client", zap.Int64("assetID", assetID), zap.Error(err))
		}
		for _, c := range fc.closers {
			if err := c.Close(); err != nil {
				logger.Default().Warn("close closer", zap.Int64("assetID", assetID), zap.Error(err))
			}
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

	client, closers, err := m.dialer.DialAsset(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
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
		if err := fc.client.Close(); err != nil {
			logger.Default().Warn("close SSH client", zap.Int64("assetID", assetID), zap.Error(err))
		}
		for _, c := range fc.closers {
			if err := c.Close(); err != nil {
				logger.Default().Warn("close closer", zap.Int64("assetID", assetID), zap.Error(err))
			}
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
	case "dynamic":
		return startDynamicForward(ctx, client, rule)
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
		if err := listener.Close(); err != nil {
			logger.Ctx(ctx).Warn("close listener", zap.Error(err))
		}
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
					if closeErr := conn.Close(); closeErr != nil {
						logger.Ctx(ctx).Warn("close conn", zap.Error(closeErr))
					}
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
		if err := listener.Close(); err != nil {
			logger.Ctx(ctx).Warn("close listener", zap.Error(err))
		}
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
					if closeErr := conn.Close(); closeErr != nil {
						logger.Ctx(ctx).Warn("close conn", zap.Error(closeErr))
					}
					return
				}
				pipeConns(conn, lconn)
			}()
		}
	}()

	return nil
}

// startDynamicForward SOCKS5 代理：本地监听，通过 SSH 隧道代理所有连接
func startDynamicForward(ctx context.Context, client *ssh.Client, rule *forward_entity.ForwardRule) error {
	addr := fmt.Sprintf("%s:%d", rule.LocalHost, rule.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		if err := listener.Close(); err != nil {
			logger.Ctx(ctx).Warn("close listener", zap.Error(err))
		}
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleSOCKS5(conn, client)
		}
	}()

	return nil
}

// handleSOCKS5 处理一个 SOCKS5 CONNECT 请求
func handleSOCKS5(conn net.Conn, client *ssh.Client) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Default().Warn("close SOCKS5 conn", zap.Error(err))
		}
	}()

	// 1. 握手：读取客户端支持的认证方法
	buf := make([]byte, 258)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 { // SOCKS5
		return
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return
	}
	// 回复：无需认证
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		logger.Default().Warn("SOCKS5 auth reply", zap.Error(err))
		return
	}

	// 2. 读取连接请求
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[1] != 0x01 { // 只支持 CONNECT
		if _, err := conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			logger.Default().Warn("SOCKS5 write command-not-supported", zap.Error(err))
		}
		return
	}

	var host string
	switch buf[3] { // address type
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 0x03: // Domain
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return
		}
		host = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		host = net.IP(buf[:16]).String()
	default:
		if _, err := conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			logger.Default().Warn("SOCKS5 write addr-type-not-supported", zap.Error(err))
		}
		return
	}

	// 读端口（2 字节大端）
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	// 3. 通过 SSH 隧道连接目标
	rconn, err := client.Dial("tcp", target)
	if err != nil {
		if _, err := conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			logger.Default().Warn("SOCKS5 write connection-refused", zap.Error(err))
		}
		return
	}

	// 4. 回复成功
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		logger.Default().Warn("SOCKS5 write success reply", zap.Error(err))
		return
	}

	// 5. 双向转发
	pipeConns(conn, rconn)
}

func pipeConns(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		if _, err := io.Copy(dst, src); err != nil {
			logger.Default().Warn("pipe copy", zap.Error(err))
		}
		if err := dst.Close(); err != nil {
			logger.Default().Warn("pipe close", zap.Error(err))
		}
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
		if err := a.forwardManager.StartConfig(ctx, id); err != nil {
			logger.Ctx(ctx).Error("restart forward config after update", zap.Int64("id", id), zap.Error(err))
		}
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
		rules, err := forward_repo.Forward().ListRulesByConfigID(ctx, c.ID)
		if err != nil {
			logger.Ctx(ctx).Warn("list forward rules by config ID", zap.Error(err), zap.Int64("configID", c.ID))
		}

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
