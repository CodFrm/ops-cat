package ssh_svc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

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

	syncMu             sync.Mutex
	syncState          DirectorySyncState
	pendingDirChange   chan error
	pendingDirNonce    string
	pendingDirTarget   string
	parserRemainder    []byte
	syncToken          string
	promptNonce        string
	promptPendingNonce string
	shellPID           int
	syncDirty          bool
	syncProbeActive    bool
	probeShellStateFn  func(int) (shellProbeResult, error)
}

// DirectorySyncState 表示终端目录同步状态。
type DirectorySyncState struct {
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd,omitempty"`
	CwdKnown    bool   `json:"cwdKnown"`
	Shell       string `json:"shell,omitempty"`
	ShellType   string `json:"shellType,omitempty"`
	Supported   bool   `json:"supported"`
	PromptReady bool   `json:"promptReady"`
	PromptClean bool   `json:"promptClean"`
	Busy        bool   `json:"busy"`
	Status      string `json:"status"` // "initializing" | "ready" | "unsupported"
	LastError   string `json:"lastError,omitempty"`
}

const (
	shellTypeUnsupported = "unsupported"
	shellTypeBash        = "bash"
	shellTypeZsh         = "zsh"
	shellTypeKsh         = "ksh"
	shellTypeMksh        = "mksh"

	directorySyncInitializing = "initializing"
	directorySyncReady        = "ready"
	directorySyncUnsupported  = "unsupported"

	syncSequencePrefix          = "\x1b]1337;opskat:"
	syncSequenceTerm            = "\a"
	syncSequenceParserMaxBytes  = 8 * 1024
	syncSequenceTokenBytes      = 16
	directorySyncMarkerOverflow = "DIRSYNC_MARKER_OVERFLOW"
)

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
	s.failPendingDirectoryChange(fmt.Errorf("DIRSYNC_SESSION_CLOSED"))
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

// GetSyncState 返回目录同步状态快照。
func (s *Session) GetSyncState() DirectorySyncState {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncState
}

// ChangeDirectory 在终端提示符可用时切换目录，并等待 shell 确认结果。
func (s *Session) ChangeDirectory(targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("DIRSYNC_INVALID_TARGET")
	}

	resultCh := make(chan error, 1)
	command, err := s.prepareDirectoryChange(targetPath, resultCh)
	if err != nil {
		return err
	}

	if err := s.writeInternal([]byte(command)); err != nil {
		s.failPendingDirectoryChange(err)
		return err
	}
	s.ensureSyncProbe()

	select {
	case result := <-resultCh:
		return result
	case <-time.After(4 * time.Second):
		s.failPendingDirectoryChange(fmt.Errorf("DIRSYNC_TIMEOUT"))
		return fmt.Errorf("DIRSYNC_TIMEOUT")
	}
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

func (s *Session) initSyncState(shellPath, shellType string, supported bool) {
	state := DirectorySyncState{
		SessionID:   s.ID,
		Shell:       shellPath,
		ShellType:   shellType,
		Supported:   supported,
		PromptReady: false,
		PromptClean: true,
		Status:      directorySyncUnsupported,
	}
	if supported {
		state.Status = directorySyncInitializing
	}
	state.Busy = !state.PromptReady || !state.PromptClean

	s.syncMu.Lock()
	s.syncState = state
	s.syncDirty = supported
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) markUserInput(data []byte) {
	if len(data) == 0 {
		return
	}

	s.syncMu.Lock()
	if !s.syncState.Supported {
		s.syncMu.Unlock()
		return
	}

	hasNewline := bytes.ContainsAny(data, "\r\n")
	changed := false
	if s.syncState.PromptReady {
		if s.syncState.PromptClean {
			s.syncState.PromptClean = false
			changed = true
		}
		if hasNewline {
			s.syncState.PromptReady = false
			s.syncState.CwdKnown = false
			s.syncState.Cwd = ""
			s.syncState.Status = directorySyncInitializing
			s.syncDirty = true
			changed = true
		}
	}
	if changed {
		s.syncState.Busy = !s.syncState.PromptReady || !s.syncState.PromptClean
		state := s.syncState
		go s.emitSyncState(state)
	}
	s.syncMu.Unlock()
}

func (s *Session) notePrompt(cwd string) {
	s.syncMu.Lock()
	s.syncState.Cwd = strings.TrimRight(cwd, "\r\n")
	s.syncState.CwdKnown = s.syncState.Cwd != ""
	s.syncState.PromptReady = true
	s.syncState.PromptClean = true
	s.syncState.Busy = false
	s.syncState.Status = directorySyncReady
	s.syncState.LastError = ""
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) noteObservedCwd(cwd string) {
	cleaned := strings.TrimRight(cwd, "\r\n")
	if cleaned == "" {
		return
	}

	s.syncMu.Lock()
	s.syncState.Cwd = cleaned
	s.syncState.CwdKnown = true
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) prepareDirectoryChange(targetPath string, resultCh chan error) (string, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	switch {
	case !s.syncState.Supported:
		return "", fmt.Errorf("DIRSYNC_UNSUPPORTED")
	case !s.syncState.CwdKnown:
		return "", fmt.Errorf("DIRSYNC_CWD_UNKNOWN")
	case s.pendingDirChange != nil:
		return "", fmt.Errorf("DIRSYNC_PENDING")
	case !s.syncState.PromptReady || !s.syncState.PromptClean:
		return "", fmt.Errorf("DIRSYNC_BUSY")
	}

	nonce, err := generateSyncToken()
	if err != nil {
		return "", fmt.Errorf("DIRSYNC_NONCE_FAILED")
	}
	s.pendingDirChange = resultCh
	s.pendingDirNonce = nonce
	s.pendingDirTarget = targetPath
	s.syncState.PromptReady = false
	s.syncState.PromptClean = false
	s.syncState.CwdKnown = false
	s.syncState.Cwd = ""
	s.syncState.Busy = true
	s.syncState.Status = directorySyncInitializing
	s.syncState.LastError = ""
	s.syncDirty = true
	state := s.syncState

	go s.emitSyncState(state)
	return buildDirectoryChangeCommand(targetPath), nil
}

func (s *Session) finishDirectoryChange(err error, cwd string) {
	s.syncMu.Lock()
	ch := s.pendingDirChange
	s.pendingDirChange = nil
	s.pendingDirNonce = ""
	s.pendingDirTarget = ""
	if cwd != "" {
		s.syncState.Cwd = strings.TrimRight(cwd, "\r\n")
		s.syncState.CwdKnown = s.syncState.Cwd != ""
		s.syncState.PromptReady = true
		s.syncState.PromptClean = true
		s.syncState.Busy = false
		s.syncState.Status = directorySyncReady
	}
	if err != nil {
		s.syncState.LastError = err.Error()
	} else {
		s.syncState.LastError = ""
	}
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()

	if ch != nil {
		ch <- err
		close(ch)
	}
	s.emitSyncState(state)
}

func (s *Session) failPendingDirectoryChange(err error) {
	s.syncMu.Lock()
	ch := s.pendingDirChange
	s.pendingDirChange = nil
	s.pendingDirNonce = ""
	s.pendingDirTarget = ""
	if err != nil {
		s.syncState.LastError = err.Error()
	}
	state := s.syncState
	s.syncMu.Unlock()

	if ch != nil {
		ch <- err
		close(ch)
	}
	s.emitSyncState(state)
}

func (s *Session) emitSyncState(state DirectorySyncState) {
	if s.onSync == nil {
		return
	}
	s.onSync(s.ID, state)
}

func (s *Session) noteParserOverflow() {
	s.syncMu.Lock()
	if s.syncState.LastError == directorySyncMarkerOverflow {
		s.syncMu.Unlock()
		return
	}
	s.syncState.LastError = directorySyncMarkerOverflow
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

type shellProbeResult struct {
	cwd         string
	promptReady bool
}

func (s *Session) ensureSyncProbe() {
	s.syncMu.Lock()
	if s.syncProbeActive || !s.syncState.Supported || s.shellPID <= 0 || s.shared == nil || s.shared.client == nil {
		s.syncMu.Unlock()
		return
	}
	s.syncProbeActive = true
	s.syncMu.Unlock()

	go s.runSyncProbeLoop()
}

func (s *Session) runSyncProbeLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.syncMu.Lock()
		if s.closed || !s.syncState.Supported || s.shellPID <= 0 || s.shared == nil || s.shared.client == nil {
			s.syncProbeActive = false
			s.syncMu.Unlock()
			return
		}
		shouldProbe := s.syncDirty || s.pendingDirChange != nil
		pid := s.shellPID
		pending := s.pendingDirChange != nil
		pendingNonce := s.pendingDirNonce
		pendingTarget := s.pendingDirTarget
		s.syncMu.Unlock()

		if !shouldProbe {
			s.syncMu.Lock()
			s.syncProbeActive = false
			s.syncMu.Unlock()
			return
		}

		result, err := s.probeShellState(pid)
		if err == nil {
			if pending {
				s.finishPendingDirectoryChangeProbe(pendingNonce, pendingTarget, result.cwd)
			} else if result.cwd != "" {
				s.noteObservedCwd(result.cwd)
			}
		}

		<-ticker.C
	}
}

func (s *Session) finishPendingDirectoryChangeProbe(nonce, targetPath, cwd string) {
	s.syncMu.Lock()
	if s.pendingDirChange == nil || s.pendingDirNonce == "" || s.pendingDirNonce != nonce {
		s.syncMu.Unlock()
		return
	}
	s.syncMu.Unlock()

	if cwd == "" {
		return
	}
	if path.Clean(cwd) == path.Clean(targetPath) {
		s.finishDirectoryChange(nil, cwd)
	}
}

func (s *Session) probeShellState(shellPID int) (shellProbeResult, error) {
	if s.probeShellStateFn != nil {
		return s.probeShellStateFn(shellPID)
	}
	session, err := s.shared.client.NewSession()
	if err != nil {
		return shellProbeResult{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && closeErr != io.EOF {
			logger.Default().Warn("close shell probe session", zap.Error(closeErr))
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = io.Discard
	if err := session.Run(buildShellStateProbeCommand(shellPID)); err != nil {
		return shellProbeResult{}, err
	}
	return parseShellProbeOutput(out.Bytes())
}

func buildShellStateProbeCommand(shellPID int) string {
	return fmt.Sprintf(`sh -lc 'pid=%d
cwd=""
prompt=0
if kill -0 "$pid" 2>/dev/null; then
  cwd=$(readlink "/proc/$pid/cwd" 2>/dev/null || printf "")
  if [ -z "$cwd" ] && command -v pwdx >/dev/null 2>&1; then
    cwd=$(pwdx "$pid" 2>/dev/null | sed "s/^[^ ]* //")
  fi
  pgid=$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d " ")
  tpgid=$(ps -o tpgid= -p "$pid" 2>/dev/null | tr -d " ")
  tty_path=$(readlink "/proc/$pid/fd/0" 2>/dev/null || printf "")
  if [ -n "$tty_path" ]; then
    stty_state=$(stty -a < "$tty_path" 2>/dev/null || printf "")
    case "$stty_state" in
      *"-icanon"*"-echo"*)
        if [ -n "$pgid" ] && [ "$pgid" = "$tpgid" ]; then
          prompt=1
        fi
        ;;
    esac
  fi
fi
printf "cwd=%%s\0prompt=%%s\0" "$cwd" "$prompt"'`, shellPID)
}

func parseShellProbeOutput(raw []byte) (shellProbeResult, error) {
	result := shellProbeResult{}
	fields := bytes.Split(raw, []byte{0})
	for _, field := range fields {
		if len(field) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(field, []byte{'='})
		if !ok {
			return shellProbeResult{}, fmt.Errorf("invalid probe field")
		}
		switch string(key) {
		case "cwd":
			result.cwd = string(value)
		case "prompt":
			result.promptReady = string(value) == "1"
		}
	}
	return result, nil
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

	syncToken, err := generateSyncToken()
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after sync token failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("生成目录同步令牌失败: %w", err)
	}
	promptNonce, err := generateSyncToken()
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.Default().Warn("close session after prompt nonce failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("生成提示符校验令牌失败: %w", err)
	}

	sess := &Session{
		ID:          sessionID,
		AssetID:     assetID,
		shared:      shared,
		session:     session,
		stdin:       stdin,
		stdout:      stdout,
		onData:      func(data []byte) { onData(sessionID, data) },
		onClosed:    onClosed,
		syncToken:   syncToken,
		promptNonce: promptNonce,
	}
	if onSync != nil {
		sess.onSync = func(_ string, state DirectorySyncState) { onSync(sessionID, state) }
	}

	shellPath, shellType := detectRemoteShell(shared.client)
	supported := shellType == shellTypeBash || shellType == shellTypeZsh || shellType == shellTypeKsh || shellType == shellTypeMksh
	sess.initSyncState(shellPath, shellType, supported)

	if supported {
		if err := session.Start(buildInteractiveShellCommand(shellPath, shellType, syncToken, promptNonce)); err != nil {
			if closeErr := session.Close(); closeErr != nil {
				logger.Default().Warn("close session after wrapped shell start failure", zap.Error(closeErr))
			}
			return "", fmt.Errorf("启动终端失败: %w", err)
		}
	} else if err := session.Shell(); err != nil {
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

func detectRemoteShell(client *ssh.Client) (string, string) {
	session, err := client.NewSession()
	if err != nil {
		return "/bin/sh", shellTypeUnsupported
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && closeErr != io.EOF {
			logger.Default().Warn("close shell probe session", zap.Error(closeErr))
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = io.Discard
	if err := session.Run(`sh -lc 'printf "%s" "${SHELL:-/bin/sh}"'`); err != nil {
		return "/bin/sh", shellTypeUnsupported
	}

	shellPath := strings.TrimSpace(out.String())
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	return shellPath, normalizeShellType(shellPath)
}

func normalizeShellType(shellPath string) string {
	switch path.Base(shellPath) {
	case "bash":
		return shellTypeBash
	case "zsh":
		return shellTypeZsh
	case "ksh":
		return shellTypeKsh
	case "mksh":
		return shellTypeMksh
	default:
		return shellTypeUnsupported
	}
}

func generateSyncToken() (string, error) {
	buf := make([]byte, syncSequenceTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildInteractiveShellCommand(shellPath, shellType, syncToken, promptNonce string) string {
	switch shellType {
	case shellTypeBash:
		return fmt.Sprintf(`rc="$(mktemp "${TMPDIR:-/tmp}/opskat-bash-XXXXXX")" && cat >"$rc" <<'EOF'
if [ -f "$HOME/.bash_profile" ]; then
  . "$HOME/.bash_profile"
elif [ -f "$HOME/.bash_login" ]; then
  . "$HOME/.bash_login"
elif [ -f "$HOME/.profile" ]; then
  . "$HOME/.profile"
fi
[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"
opskat_next_prompt_nonce() {
  local opskat_now opskat_rand
  opskat_now=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  opskat_rand=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$opskat_rand" "$opskat_now"
}
opskat_prompt_proof() {
  local opskat_pwd opskat_current opskat_next
  opskat_current=${OPSKAT_PROMPT_NONCE:-}
  [ -n "$opskat_current" ] || return
  opskat_next=$(opskat_next_prompt_nonce)
  opskat_pwd=$(builtin pwd -P 2>/dev/null || builtin pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$opskat_current" "$opskat_next" "$opskat_pwd"
  OPSKAT_PROMPT_NONCE=$opskat_next
}
OPSKAT_PROMPT_NONCE=%s
PROMPT_COMMAND="opskat_prompt_proof${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
EOF
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s --rcfile "$rc" -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
	case shellTypeZsh:
		return fmt.Sprintf(`dir="$(mktemp -d "${TMPDIR:-/tmp}/opskat-zsh-XXXXXX")" && cat >"$dir/.zshenv" <<'EOF_ENV'
[[ -f "$HOME/.zshenv" ]] && source "$HOME/.zshenv"
EOF_ENV
cat >"$dir/.zshrc" <<'EOF_RC'
[[ -f "$HOME/.zprofile" ]] && source "$HOME/.zprofile"
[[ -f "$HOME/.zshrc" ]] && source "$HOME/.zshrc"
autoload -Uz add-zsh-hook
opskat_next_prompt_nonce() {
  local opskat_now opskat_rand
  opskat_now=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  opskat_rand=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$opskat_rand" "$opskat_now"
}
opskat_prompt_proof() {
  local opskat_pwd opskat_current opskat_next
  opskat_current=${OPSKAT_PROMPT_NONCE:-}
  [[ -n "$opskat_current" ]] || return
  opskat_next=$(opskat_next_prompt_nonce)
  opskat_pwd=$(pwd -P 2>/dev/null || pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$opskat_current" "$opskat_next" "$opskat_pwd"
  OPSKAT_PROMPT_NONCE=$opskat_next
}
OPSKAT_PROMPT_NONCE=%s
add-zsh-hook precmd opskat_prompt_proof
EOF_RC
export ZDOTDIR="$dir"
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
	case shellTypeKsh, shellTypeMksh:
		return fmt.Sprintf(`envfile="$(mktemp "${TMPDIR:-/tmp}/opskat-ksh-XXXXXX")" && cat >"$envfile" <<'EOF'
[ -f "$HOME/.profile" ] && . "$HOME/.profile"
opskat_next_prompt_nonce() {
  OPSKAT_NOW=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  OPSKAT_RAND=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$OPSKAT_RAND" "$OPSKAT_NOW"
}
opskat_prompt_proof() {
  OPSKAT_CURRENT=${OPSKAT_PROMPT_NONCE:-}
  [ -n "$OPSKAT_CURRENT" ] || return
  OPSKAT_NEXT=$(opskat_next_prompt_nonce)
  OPSKAT_PWD=$(pwd -P 2>/dev/null || pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$OPSKAT_CURRENT" "$OPSKAT_NEXT" "$OPSKAT_PWD"
  OPSKAT_PROMPT_NONCE=$OPSKAT_NEXT
}
OPSKAT_PROMPT_NONCE=%s
PS1='$(opskat_prompt_proof)'"$PS1"
EOF
export ENV="$envfile"
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
	default:
		return ""
	}
}

func buildDirectoryChangeCommand(targetPath string) string {
	return fmt.Sprintf("builtin cd -- %s\r", shellQuote(targetPath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
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

func (s *Session) filterOutput(chunk []byte) []byte {
	data := chunk
	if len(s.parserRemainder) > 0 {
		data = append(append([]byte(nil), s.parserRemainder...), chunk...)
		s.parserRemainder = nil
	}

	prefix := []byte(syncSequencePrefix)
	out := make([]byte, 0, len(data))

	for len(data) > 0 {
		idx := bytes.Index(data, prefix)
		if idx < 0 {
			break
		}
		out = append(out, data[:idx]...)
		remainder := data[idx+len(prefix):]
		end := bytes.IndexByte(remainder, syncSequenceTerm[0])
		if end < 0 {
			tail := append([]byte(nil), data[idx:]...)
			if len(tail) > syncSequenceParserMaxBytes {
				s.noteParserOverflow()
				out = append(out, tail...)
				return out
			}
			s.parserRemainder = tail
			return out
		}
		rawEnd := idx + len(prefix) + end + 1
		raw := data[idx:rawEnd]
		if !s.handleSyncPayload(string(remainder[:end])) {
			out = append(out, raw...)
		}
		data = data[rawEnd:]
	}

	if len(data) == 0 {
		return out
	}

	if keep := trailingPrefixLength(data, prefix); keep > 0 {
		out = append(out, data[:len(data)-keep]...)
		s.parserRemainder = append([]byte(nil), data[len(data)-keep:]...)
		return out
	}

	out = append(out, data...)
	return out
}

func trailingPrefixLength(data, prefix []byte) int {
	maxSize := len(prefix) - 1
	if maxSize > len(data) {
		maxSize = len(data)
	}
	for size := maxSize; size > 0; size-- {
		if bytes.Equal(data[len(data)-size:], prefix[:size]) {
			return size
		}
	}
	return 0
}

func (s *Session) handleSyncPayload(payload string) bool {
	token, body, ok := strings.Cut(payload, ":")
	if !ok || token == "" || token != s.syncToken {
		return false
	}

	switch {
	case strings.HasPrefix(body, "init:pid:"):
		pidText := strings.TrimPrefix(body, "init:pid:")
		pid, err := strconv.Atoi(strings.TrimSpace(pidText))
		if err != nil || pid <= 0 {
			return false
		}
		s.syncMu.Lock()
		if s.shellPID != 0 {
			s.syncMu.Unlock()
			return false
		}
		s.shellPID = pid
		s.syncDirty = true
		s.syncMu.Unlock()
		s.ensureSyncProbe()
		return true
	case strings.HasPrefix(body, "prompt:"):
		remainder := strings.TrimPrefix(body, "prompt:")
		currentNonce, nextPayload, ok := strings.Cut(remainder, ":")
		if !ok || currentNonce == "" {
			return false
		}
		nextNonce, cwd, ok := strings.Cut(nextPayload, ":")
		if !ok || nextNonce == "" {
			return false
		}
		s.syncMu.Lock()
		promptNonce := s.promptNonce
		promptPendingNonce := s.promptPendingNonce
		shellPID := s.shellPID
		s.syncMu.Unlock()
		validCurrent := currentNonce == promptNonce || (promptPendingNonce != "" && currentNonce == promptPendingNonce)
		if promptNonce == "" || !validCurrent || shellPID <= 0 {
			return false
		}
		probe, err := s.probeShellState(shellPID)
		if err != nil || !probe.promptReady {
			s.syncMu.Lock()
			if currentNonce == s.promptNonce || (s.promptPendingNonce != "" && currentNonce == s.promptPendingNonce) {
				s.promptPendingNonce = nextNonce
			}
			s.syncMu.Unlock()
			return false
		}
		resolvedCwd := probe.cwd
		if resolvedCwd == "" {
			resolvedCwd = cwd
		}
		if resolvedCwd == "" {
			return false
		}
		s.syncMu.Lock()
		if currentNonce != s.promptNonce && (s.promptPendingNonce == "" || currentNonce != s.promptPendingNonce) {
			s.syncMu.Unlock()
			return false
		}
		s.promptNonce = nextNonce
		s.promptPendingNonce = ""
		s.syncMu.Unlock()
		s.notePrompt(resolvedCwd)
		return true
	}
	return false
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
		return DirectorySyncState{}, fmt.Errorf("会话不存在: %s", id)
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
