package ssh_svc

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/pkg/dirsync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// sharedClient 封装 SSH 连接，支持引用计数共享
type sharedClient struct {
	client   *ssh.Client
	mu       sync.Mutex
	refCount int
	closers  []io.Closer // 跳板机 client 等额外资源
	closed   bool
}

func newSharedClient(client *ssh.Client, closers []io.Closer) *sharedClient {
	return &sharedClient{
		client:   client,
		refCount: 1,
		closers:  closers,
	}
}

func (sc *sharedClient) acquire() {
	sc.mu.Lock()
	sc.refCount++
	sc.mu.Unlock()
}

func (sc *sharedClient) release() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.refCount--
	if sc.refCount <= 0 && !sc.closed {
		sc.closed = true
		if err := sc.client.Close(); err != nil {
			logger.Default().Warn("close client", zap.Error(err))
		}
		for _, c := range sc.closers {
			if err := c.Close(); err != nil {
				logger.Default().Warn("close jump host resource", zap.Error(err))
			}
		}
	}
}

// Session 表示一个活跃的 SSH 终端会话
type Session struct {
	ID       string
	AssetID  int64
	shared   *sharedClient
	session  *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	mu       sync.Mutex
	closed   bool
	onData   func(data []byte)      // 终端输出回调
	onClosed func(sessionID string) // 会话关闭回调
	onSync   func(sessionID string, state DirectorySyncState)

	// shellPath / shellType are detected once at session creation by
	// detectRemoteShell. They are read by EnableSync to build the right
	// hook-installer payload. Empty / "unsupported" means lazy sync is
	// not available for this session.
	shellPath string
	shellType string

	syncMu             sync.Mutex
	syncState          DirectorySyncState
	pendingDirChange   chan error
	pendingDirNonce    string
	pendingDirTarget   string
	pendingDirExpected string
	parserRemainder    []byte
	syncToken          string
	promptNonce        string
	promptPendingNonce string
	shellPID           int
	syncDirty          bool
	syncBootstrapCh    chan struct{} // closed when EnableSync receives init:pid; nil when not bootstrapping
	syncProbeActive    bool
	probeShellStateFn  func(int) (shellProbeResult, error)
}

// Write 向终端写入数据（用户输入）
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("session is closed")
	}
	hasNewline := bytes.ContainsAny(data, "\r\n")
	s.markUserInput(data)
	_, err := s.stdin.Write(data)
	s.mu.Unlock()
	if err == nil && hasNewline {
		s.ensureSyncProbe()
	}
	return err
}

// Resize 调整终端尺寸
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session is closed")
	}
	return s.session.WindowChange(rows, cols)
}

// Close 关闭会话
func (s *Session) Close() {
	s.failPendingDirectoryChange(dirsync.Error(dirSyncErrSessionClosed))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if err := s.session.Close(); err != nil {
		logger.Default().Warn("close session", zap.String("sessionID", s.ID), zap.Error(err))
	}
	s.shared.release()
	if s.onClosed != nil {
		go s.onClosed(s.ID)
	}
}

// Client 返回底层 SSH Client（用于 SFTP 等）
func (s *Session) Client() *ssh.Client {
	return s.shared.client
}

// IsClosed 检查是否已关闭
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Session) writeInternal(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session is closed")
	}
	_, err := s.stdin.Write(data)
	return err
}

// Manager 管理所有 SSH 会话
type Manager struct {
	sessions sync.Map // map[string]*Session
	counter  int64
	mu       sync.Mutex
}

// NewManager 创建会话管理器
func NewManager() *Manager {
	return &Manager{}
}

// ConnectConfig SSH 连接配置
type ConnectConfig struct {
	Host          string
	Port          int
	Username      string
	AuthType      string // password | key | keyboard-interactive
	Password      string
	Key           string   // PEM 格式私钥（直接传入）
	KeyPassphrase string   // 私钥密码（用于加密的私钥）
	PrivateKeys   []string // 私钥文件路径列表
	AssetID       int64
	Cols          int
	Rows          int
	OnData        func(sessionID string, data []byte) // 终端输出回调
	OnClosed      func(sessionID string)              // 关闭回调
	OnSync        func(sessionID string, state DirectorySyncState)

	// 进度回调（异步连接用），step: resolve/connect/auth/shell
	OnProgress func(step, message string)
	// 键盘交互认证回调
	OnAuthChallenge func(prompts []string, echo []bool) ([]string, error)

	// 跳板机: 已解析的链式连接配置（从叶子到根）
	JumpHosts []JumpHostEntry
	// 代理
	Proxy *asset_entity.ProxyConfig

	// 主机密钥校验回调（nil 则跳过校验）
	HostKeyVerifyFunc HostKeyVerifyFunc
}

// JumpHostEntry 跳板机连接信息
type JumpHostEntry struct {
	Host       string
	Port       int
	Username   string
	AuthType   string
	Password   string
	Key        string
	Passphrase string
}

// emitProgress 安全调用进度回调
func emitProgress(cfg *ConnectConfig, step, message string) {
	if cfg.OnProgress != nil {
		cfg.OnProgress(step, message)
	}
}

// Dial 仅建立 SSH 连接（不创建 PTY/Session），用于连接池等场景
func (m *Manager) Dial(cfg ConnectConfig) (*ssh.Client, []io.Closer, error) {
	authMethods, err := buildAuthMethods(cfg.AuthType, cfg.Password, cfg.Key, cfg.KeyPassphrase, cfg.PrivateKeys, cfg.OnAuthChallenge)
	if err != nil {
		return nil, nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: MakeHostKeyCallback(cfg.Host, cfg.Port, cfg.HostKeyVerifyFunc),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return m.dial(cfg, sshConfig, addr)
}

// Connect 建立 SSH 连接并启动 PTY 会话
func (m *Manager) Connect(cfg ConnectConfig) (string, error) {
	// 构建目标认证方式
	authMethods, err := buildAuthMethods(cfg.AuthType, cfg.Password, cfg.Key, cfg.KeyPassphrase, cfg.PrivateKeys, cfg.OnAuthChallenge)
	if err != nil {
		return "", err
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: MakeHostKeyCallback(cfg.Host, cfg.Port, cfg.HostKeyVerifyFunc),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	emitProgress(&cfg, "connect", fmt.Sprintf("正在连接 %s...", addr))

	// 建立连接（可能经过代理和跳板机链）
	client, extraClosers, err := m.dial(cfg, sshConfig, addr)
	if err != nil {
		return "", err
	}

	shared := newSharedClient(client, extraClosers)

	emitProgress(&cfg, "shell", "正在启动终端...")

	sessionID, err := m.createSession(shared, cfg.AssetID, cfg.Cols, cfg.Rows, cfg.OnData, cfg.OnClosed, cfg.OnSync)
	if err != nil {
		shared.release()
		return "", err
	}

	return sessionID, nil
}

// createSession 在 sharedClient 上创建新的 SSH 会话（PTY + shell）
func (m *Manager) createSession(shared *sharedClient, assetID int64, cols, rows int,
	onData func(string, []byte), onClosed func(string), onSync func(string, DirectorySyncState)) (string, error) {

	session, err := shared.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}

	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if err := session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after PTY request failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("请求PTY失败: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after stdin pipe failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("获取stdin失败: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after stdout pipe failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("获取stdout失败: %w", err)
	}

	m.mu.Lock()
	m.counter++
	sessionID := fmt.Sprintf("ssh-%d", m.counter)
	m.mu.Unlock()

	shellPath, shellType := detectRemoteShell(shared.client)
	sess := &Session{
		ID:        sessionID,
		AssetID:   assetID,
		shared:    shared,
		session:   session,
		stdin:     stdin,
		stdout:    stdout,
		shellPath: shellPath,
		shellType: shellType,
		onData:    func(data []byte) { onData(sessionID, data) },
		onClosed:  onClosed,
	}
	if onSync != nil {
		sess.onSync = func(_ string, state DirectorySyncState) { onSync(sessionID, state) }
	}

	// Always start a native interactive shell so sshd emits "Last login" /
	// motd / banner. Sync hooks are installed on demand via Session.EnableSync
	// (added in Task 3); until then, sync stays off.
	sess.initSyncState(shellPath, shellType, false)

	if err := session.Shell(); err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after shell start failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("启动shell失败: %w", err)
	}

	m.sessions.Store(sessionID, sess)
	go m.readOutput(sess)

	return sessionID, nil
}

// NewSessionFrom 在已有会话的连接上创建新会话（用于分割窗格）
func (m *Manager) NewSessionFrom(existingSessionID string, cols, rows int,
	onData func(string, []byte), onClosed func(string), onSync func(string, DirectorySyncState)) (string, error) {

	existing, ok := m.GetSession(existingSessionID)
	if !ok {
		return "", fmt.Errorf("会话不存在: %s", existingSessionID)
	}
	if existing.IsClosed() {
		return "", fmt.Errorf("会话已关闭: %s", existingSessionID)
	}

	existing.shared.acquire()

	sessionID, err := m.createSession(existing.shared, existing.AssetID, cols, rows, onData, onClosed, onSync)
	if err != nil {
		existing.shared.release()
		return "", err
	}

	return sessionID, nil
}

// dial 建立到目标的网络连接，支持代理和跳板机链
func (m *Manager) dial(cfg ConnectConfig, sshConfig *ssh.ClientConfig, targetAddr string) (*ssh.Client, []io.Closer, error) {
	var closers []io.Closer

	// 情况1: 有跳板机链
	if len(cfg.JumpHosts) > 0 {
		return m.dialViaJumpHosts(cfg, sshConfig, targetAddr)
	}

	// 情况2: 有代理（无跳板机）
	if cfg.Proxy != nil {
		emitProgress(&cfg, "connect", fmt.Sprintf("正在通过代理 %s:%d 连接...", cfg.Proxy.Host, cfg.Proxy.Port))
		conn, err := dialViaProxy(cfg.Proxy, targetAddr)
		if err != nil {
			return nil, nil, err
		}
		closers = append(closers, conn)

		emitProgress(&cfg, "auth", "正在认证...")
		c, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, sshConfig)
		if err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				logger.Default().Warn("close proxy connection after handshake failure", zap.Error(closeErr))
			}
			return nil, nil, fmt.Errorf("SSH握手失败: %w", err)
		}
		return ssh.NewClient(c, chans, reqs), closers, nil
	}

	// 情况3: 直连
	emitProgress(&cfg, "auth", "正在认证...")
	client, err := ssh.Dial("tcp", targetAddr, sshConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	return client, nil, nil
}

// dialViaJumpHosts 通过跳板机链连接目标
func (m *Manager) dialViaJumpHosts(cfg ConnectConfig, targetConfig *ssh.ClientConfig, targetAddr string) (*ssh.Client, []io.Closer, error) {
	var closers []io.Closer

	// 连接第一个跳板机（可能通过代理）
	firstJump := cfg.JumpHosts[0]
	firstAddr := fmt.Sprintf("%s:%d", firstJump.Host, firstJump.Port)

	emitProgress(&cfg, "connect", fmt.Sprintf("正在连接跳板机 %s...", firstAddr))

	firstAuth, err := buildAuthMethods(firstJump.AuthType, firstJump.Password, firstJump.Key, firstJump.Passphrase, nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("跳板机认证配置失败: %w", err)
	}
	firstConfig := &ssh.ClientConfig{
		User:            firstJump.Username,
		Auth:            firstAuth,
		HostKeyCallback: MakeHostKeyCallback(firstJump.Host, firstJump.Port, cfg.HostKeyVerifyFunc),
		Timeout:         30 * time.Second,
	}

	var currentClient *ssh.Client

	if cfg.Proxy != nil {
		emitProgress(&cfg, "connect", fmt.Sprintf("正在通过代理 %s:%d 连接跳板机...", cfg.Proxy.Host, cfg.Proxy.Port))
		conn, err := dialViaProxy(cfg.Proxy, firstAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("通过代理连接跳板机失败: %w", err)
		}
		closers = append(closers, conn)

		c, chans, reqs, err := ssh.NewClientConn(conn, firstAddr, firstConfig)
		if err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				logger.Default().Warn("close proxy connection after jump host handshake failure", zap.Error(closeErr))
			}
			return nil, nil, fmt.Errorf("跳板机SSH握手失败: %w", err)
		}
		currentClient = ssh.NewClient(c, chans, reqs)
	} else {
		currentClient, err = ssh.Dial("tcp", firstAddr, firstConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("连接跳板机失败: %w", err)
		}
	}
	closers = append(closers, currentClient)

	// 连接中间跳板机
	for i := 1; i < len(cfg.JumpHosts); i++ {
		jump := cfg.JumpHosts[i]
		jumpAddr := fmt.Sprintf("%s:%d", jump.Host, jump.Port)

		emitProgress(&cfg, "connect", fmt.Sprintf("正在连接跳板机 %s...", jumpAddr))

		jumpAuth, err := buildAuthMethods(jump.AuthType, jump.Password, jump.Key, jump.Passphrase, nil, nil)
		if err != nil {
			for _, c := range closers {
				if closeErr := c.Close(); closeErr != nil {
					logger.Default().Warn("close jump host chain resource during auth config cleanup", zap.Error(closeErr))
				}
			}
			return nil, nil, fmt.Errorf("跳板机认证配置失败: %w", err)
		}
		jumpConfig := &ssh.ClientConfig{
			User:            jump.Username,
			Auth:            jumpAuth,
			HostKeyCallback: MakeHostKeyCallback(jump.Host, jump.Port, cfg.HostKeyVerifyFunc),
			Timeout:         30 * time.Second,
		}

		conn, err := currentClient.Dial("tcp", jumpAddr)
		if err != nil {
			for _, c := range closers {
				if closeErr := c.Close(); closeErr != nil {
					logger.Default().Warn("close jump host chain resource during dial cleanup", zap.Error(closeErr))
				}
			}
			return nil, nil, fmt.Errorf("通过跳板机连接下一跳失败: %w", err)
		}

		c, chans, reqs, err := ssh.NewClientConn(conn, jumpAddr, jumpConfig)
		if err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				logger.Default().Warn("close jump host connection after handshake failure", zap.Error(closeErr))
			}
			for _, c := range closers {
				if closeErr := c.Close(); closeErr != nil {
					logger.Default().Warn("close jump host chain resource during handshake cleanup", zap.Error(closeErr))
				}
			}
			return nil, nil, fmt.Errorf("跳板机SSH握手失败: %w", err)
		}
		currentClient = ssh.NewClient(c, chans, reqs)
		closers = append(closers, currentClient)
	}

	// 通过最后一个跳板机连接目标
	emitProgress(&cfg, "connect", fmt.Sprintf("正在通过跳板机连接目标 %s...", targetAddr))

	conn, err := currentClient.Dial("tcp", targetAddr)
	if err != nil {
		for _, c := range closers {
			if closeErr := c.Close(); closeErr != nil {
				logger.Default().Warn("close jump host chain resource during target dial cleanup", zap.Error(closeErr))
			}
		}
		return nil, nil, fmt.Errorf("通过跳板机连接目标失败: %w", err)
	}

	emitProgress(&cfg, "auth", "正在认证...")

	c, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, targetConfig)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Default().Warn("close target connection after handshake failure", zap.Error(closeErr))
		}
		for _, c := range closers {
			if closeErr := c.Close(); closeErr != nil {
				logger.Default().Warn("close jump host chain resource during target handshake cleanup", zap.Error(closeErr))
			}
		}
		return nil, nil, fmt.Errorf("目标SSH握手失败: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), closers, nil
}

// dialViaProxy 通过 SOCKS5 代理建立 TCP 连接
func dialViaProxy(proxyCfg *asset_entity.ProxyConfig, targetAddr string) (net.Conn, error) {
	if proxyCfg.Type != "" && proxyCfg.Type != "socks5" {
		return nil, fmt.Errorf("不支持的代理类型: %s", proxyCfg.Type)
	}

	proxyAddr := fmt.Sprintf("%s:%d", proxyCfg.Host, proxyCfg.Port)
	var auth *proxy.Auth
	if proxyCfg.Username != "" {
		auth = &proxy.Auth{
			User:     proxyCfg.Username,
			Password: proxyCfg.Password,
		}
	}
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("创建SOCKS代理失败: %w", err)
	}
	conn, err := dialer.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("通过SOCKS代理连接失败: %w", err)
	}
	return conn, nil
}

// buildAuthMethods 构建 SSH 认证方式
func buildAuthMethods(authType, password, key, keyPassphrase string, privateKeyPaths []string,
	onAuthChallenge func(prompts []string, echo []bool) ([]string, error)) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// keyboard-interactive 认证回调（用于 OTP/动态密码等场景）
	kbInteractive := func() ssh.AuthMethod {
		return ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			// 如果没有问题，返回空
			if len(questions) == 0 {
				return nil, nil
			}
			// 如果有回调，使用回调获取用户输入
			if onAuthChallenge != nil {
				return onAuthChallenge(questions, echos)
			}
			// 没有回调但有密码，尝试用密码回答第一个问题
			if password != "" {
				answers := make([]string, len(questions))
				answers[0] = password
				return answers, nil
			}
			return nil, fmt.Errorf("keyboard-interactive 认证需要用户输入")
		})
	}

	switch authType {
	case "password":
		methods = append(methods, ssh.Password(password))
		// 追加 keyboard-interactive 作为 fallback（许多服务器用 keyboard-interactive 替代 password）
		methods = append(methods, kbInteractive())
	case "key":
		// 优先使用直接传入的 key
		if key != "" {
			signer, err := parsePrivateKey([]byte(key), keyPassphrase)
			if err != nil {
				return nil, fmt.Errorf("解析密钥失败: %w", err)
			}
			methods = append(methods, ssh.PublicKeys(signer))
		}
		// 从文件路径读取私钥
		for _, path := range privateKeyPaths {
			data, err := os.ReadFile(path) //nolint:gosec // file path from user config
			if err != nil {
				return nil, fmt.Errorf("读取私钥文件 %s 失败: %w", path, err)
			}
			signer, err := parsePrivateKey(data, keyPassphrase)
			if err != nil {
				return nil, fmt.Errorf("解析私钥文件 %s 失败: %w", path, err)
			}
			methods = append(methods, ssh.PublicKeys(signer))
		}
		if len(methods) == 0 {
			return nil, fmt.Errorf("密钥认证方式需要提供私钥")
		}
	case "keyboard-interactive":
		methods = append(methods, kbInteractive())
	default:
		return nil, fmt.Errorf("不支持的认证方式: %s", authType)
	}

	return methods, nil
}

// parsePrivateKey 解析私钥，支持 passphrase
func parsePrivateKey(data []byte, passphrase string) (ssh.Signer, error) {
	// 先尝试无 passphrase 解析
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	// 如果失败且提供了 passphrase，尝试带 passphrase 解析
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("解析加密私钥失败（可能 passphrase 不正确）: %w", err)
		}
		return signer, nil
	}
	return nil, err
}

// readOutput 持续读取终端输出并回调
// 使用 timer 合并输出，减少高频 EventsEmit 调用导致前端事件队列阻塞
func (m *Manager) readOutput(sess *Session) {
	defer func() {
		if r := recover(); r != nil {
			logger.Default().Error("readOutput panic recovered",
				zap.String("sessionID", sess.ID),
				zap.Any("panic", r))
		}
		sess.Close()
		m.sessions.Delete(sess.ID)
	}()

	var pending bytes.Buffer
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if pending.Len() > 0 && sess.onData != nil {
			data := make([]byte, pending.Len())
			copy(data, pending.Bytes())
			pending.Reset()
			sess.onData(data)
		}
	}

	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 4)

	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := sess.stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				readCh <- readResult{data: data}
			}
			if err != nil {
				readCh <- readResult{err: err}
				return
			}
		}
	}()

	for {
		select {
		case r := <-readCh:
			if r.err != nil {
				if len(sess.parserRemainder) > 0 {
					pending.Write(sess.parserRemainder)
					sess.parserRemainder = nil
				}
				flush()
				return
			}
			filtered := sess.filterOutput(r.data)
			if len(filtered) > 0 {
				pending.Write(filtered)
			}
			if pending.Len() >= 32*1024 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// GetSession 获取会话
func (m *Manager) GetSession(id string) (*Session, bool) {
	v, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*Session), true
}

// GetSessionSyncState 获取会话目录同步状态。
func (m *Manager) GetSessionSyncState(id string) (DirectorySyncState, error) {
	sess, ok := m.GetSession(id)
	if !ok {
		return DirectorySyncState{}, dirsync.Error(dirsync.CodeSessionNotFound)
	}
	return sess.GetSyncState(), nil
}

// Disconnect 断开指定会话
func (m *Manager) Disconnect(id string) {
	if sess, ok := m.GetSession(id); ok {
		sess.Close()
		m.sessions.Delete(id)
	}
}

// DisconnectAll 断开所有会话
func (m *Manager) DisconnectAll() {
	m.sessions.Range(func(key, value any) bool {
		value.(*Session).Close()
		m.sessions.Delete(key)
		return true
	})
}

// TestConnection 测试 SSH 连接（仅验证连通性，不创建会话）
func (m *Manager) TestConnection(cfg ConnectConfig) error {
	authMethods, err := buildAuthMethods(cfg.AuthType, cfg.Password, cfg.Key, cfg.KeyPassphrase, cfg.PrivateKeys, cfg.OnAuthChallenge)
	if err != nil {
		return err
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: MakeHostKeyCallback(cfg.Host, cfg.Port, cfg.HostKeyVerifyFunc),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	client, closers, err := m.dial(cfg, sshConfig, addr)
	if err != nil {
		return err
	}
	if err := client.Close(); err != nil {
		logger.Default().Warn("close test connection client", zap.Error(err))
	}
	for _, c := range closers {
		if err := c.Close(); err != nil {
			logger.Default().Warn("close test connection resource", zap.Error(err))
		}
	}
	return nil
}

// ActiveSessions 返回活跃会话数
func (m *Manager) ActiveSessions() int {
	count := 0
	m.sessions.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
