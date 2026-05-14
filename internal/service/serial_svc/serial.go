package serial_svc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.bug.st/serial"
	"go.uber.org/zap"
)

// CommandSession 是供 AI 串口命令执行使用的最小会话接口。
type CommandSession interface {
	ExecCommand(command string, silenceTimeout, maxTimeout time.Duration) (string, error)
}

// CommandManager 是供 AI 查找活跃串口会话使用的最小管理器接口。
type CommandManager interface {
	GetSessionByAssetID(assetID int64) (CommandSession, bool)
}

var (
	errSessionClosed   = errors.New("session is closed")
	errCommandTimedOut = errors.New("serial command timed out")
)

const callbackSetupGracePeriod = 5 * time.Second

type commandCapture struct {
	mu     sync.Mutex
	chunks [][]byte
	signal chan struct{}
}

func newCommandCapture() *commandCapture {
	return &commandCapture{signal: make(chan struct{}, 1)}
}

func (c *commandCapture) Append(data []byte) {
	chunk := make([]byte, len(data))
	copy(chunk, data)

	c.mu.Lock()
	c.chunks = append(c.chunks, chunk)
	c.mu.Unlock()

	select {
	case c.signal <- struct{}{}:
	default:
	}
}

func (c *commandCapture) Drain() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.chunks) == 0 {
		return nil
	}
	total := 0
	for _, chunk := range c.chunks {
		total += len(chunk)
	}
	out := make([]byte, 0, total)
	for _, chunk := range c.chunks {
		out = append(out, chunk...)
	}
	c.chunks = nil
	return out
}

// Session 表示一个活跃的串口终端会话
type Session struct {
	ID            string
	AssetID       int64
	port          serial.Port
	writeMu       sync.Mutex
	mu            sync.Mutex
	closed        bool
	readerStarted bool
	closedCh      chan struct{}
	readerReadyCh chan struct{}
	onData        func(data []byte)      // 终端输出回调
	onClosed      func(sessionID string) // 会话关闭回调

	// AI 命令执行辅助：当 cmdCapture 非 nil 时，readOutput 会同时向此收集器追加数据副本。
	cmdCapture *commandCapture
}

// Write 向串口写入数据（用户输入）
func (s *Session) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writeLocked(data)
}

func (s *Session) writeLocked(data []byte) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errSessionClosed
	}
	port := s.port
	s.mu.Unlock()
	_, err := port.Write(data)
	return err
}

// Resize 调整终端尺寸（串口无实际作用，保留接口兼容）
func (s *Session) Resize(cols, rows int) error {
	return nil // no-op for serial
}

func (s *Session) ensureClosedChLocked() chan struct{} {
	if s.closedCh == nil {
		s.closedCh = make(chan struct{})
	}
	return s.closedCh
}

func (s *Session) ensureReaderReadyChLocked() chan struct{} {
	if s.readerReadyCh == nil {
		s.readerReadyCh = make(chan struct{})
	}
	return s.readerReadyCh
}

func (s *Session) closeLocked() (serial.Port, func(string), string, bool) {
	if s.closed {
		return nil, nil, "", false
	}
	closedCh := s.ensureClosedChLocked()
	s.closed = true
	close(closedCh)
	return s.port, s.onClosed, s.ID, true
}

// Close 关闭串口会话
func (s *Session) Close() {
	s.mu.Lock()
	port, onClosed, sessionID, ok := s.closeLocked()
	s.mu.Unlock()
	if !ok {
		return
	}
	if err := port.Close(); err != nil && !isPortClosedError(err) {
		logger.Default().Warn("close serial port", zap.String("sessionID", sessionID), zap.Error(err))
	}
	if onClosed != nil {
		go onClosed(sessionID)
	}
}

// ExecCommand 向串口发送命令并收集输出。
// 适用于 AI 工具调用场景：写入命令后等待输出静默（silenceTimeout）或达到最大等待时间。
func (s *Session) ExecCommand(command string, silenceTimeout, maxTimeout time.Duration) (string, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", errSessionClosed
	}
	capture := newCommandCapture()
	s.cmdCapture = capture
	closedCh := s.ensureClosedChLocked()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.cmdCapture == capture {
			s.cmdCapture = nil
		}
		s.mu.Unlock()
	}()

	// 写入命令 + 回车
	if err := s.writeLocked([]byte(command + "\r\n")); err != nil {
		return "", fmt.Errorf("write command: %w", err)
	}

	var output []byte
	var silenceTimer *time.Timer
	var silenceTimerC <-chan time.Time
	maxTimer := time.NewTimer(maxTimeout)
	defer maxTimer.Stop()
	stopSilenceTimer := func() {
		if silenceTimer == nil {
			return
		}
		if !silenceTimer.Stop() {
			select {
			case <-silenceTimer.C:
			default:
			}
		}
	}
	defer stopSilenceTimer()
	resetSilenceTimer := func() {
		if silenceTimer == nil {
			silenceTimer = time.NewTimer(silenceTimeout)
			silenceTimerC = silenceTimer.C
			return
		}
		stopSilenceTimer()
		silenceTimer.Reset(silenceTimeout)
	}
	appendCapturedOutput := func() bool {
		data := capture.Drain()
		if len(data) == 0 {
			return false
		}
		output = append(output, data...)
		return true
	}

	for {
		select {
		case <-capture.signal:
			if !appendCapturedOutput() {
				continue
			}
			// 仅在收到首个输出后开始/重置静默计时，避免设备首包较慢时过早返回。
			resetSilenceTimer()
		case <-silenceTimerC:
			// 输出静默，认为命令执行完毕
			appendCapturedOutput()
			return string(output), nil
		case <-maxTimer.C:
			// 超过最大等待时间
			appendCapturedOutput()
			return string(output), fmt.Errorf("%w after %s", errCommandTimedOut, maxTimeout)
		case <-closedCh:
			appendCapturedOutput()
			return string(output), errSessionClosed
		}
	}
}

// IsClosed 检查是否已关闭
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// ConnectConfig 串口连接配置
type ConnectConfig struct {
	PortPath    string
	BaudRate    int
	DataBits    int
	StopBits    string
	Parity      string
	FlowControl string
	AssetID     int64
}

// SerialPortInfo 可用串口信息
type SerialPortInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	ProductID    string `json:"productId,omitempty"`
	VendorID     string `json:"vendorId,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
}

// Manager 管理所有串口会话
type Manager struct {
	sessions sync.Map // map[string]*Session
	counter  int64
	mu       sync.Mutex
}

// NewManager 创建串口会话管理器
func NewManager() *Manager {
	return &Manager{}
}

// ListPorts 列出系统可用串口
func (m *Manager) ListPorts() ([]SerialPortInfo, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("list serial ports: %w", err)
	}
	result := make([]SerialPortInfo, 0, len(ports))
	for _, p := range ports {
		result = append(result, SerialPortInfo{
			Name:        p,
			DisplayName: p,
		})
	}
	return result, nil
}

// Connect 打开串口连接，返回 sessionId。调用方通过 SetCallbacks 设置回调。
// 若未在宽限时间内设置回调，会话会自动关闭并清理，避免悬挂。
func (m *Manager) Connect(cfg ConnectConfig) (string, error) {
	mode, err := buildSerialMode(cfg)
	if err != nil {
		return "", err
	}

	port, err := serial.Open(cfg.PortPath, mode)
	if err != nil {
		return "", fmt.Errorf("open serial port %s: %w", cfg.PortPath, err)
	}

	// 设置读超时（避免阻塞读取 goroutine 永远不退出）
	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		if closeErr := port.Close(); closeErr != nil && !isPortClosedError(closeErr) {
			logger.Default().Warn("close serial port after read-timeout setup failure", zap.Error(closeErr))
		}
		return "", fmt.Errorf("set read timeout: %w", err)
	}

	// 流控制。"hardware" 走 enableHardwareFlowControl 直接 ioctl/DCB 打开 RTS/CTS，
	// 因为 go.bug.st/serial v1.6.4 不暴露这一项且 nativeOpen 会强制关闭它。
	switch cfg.FlowControl {
	case "", "none":
		// no flow control
	case "hardware":
		if err := enableHardwareFlowControl(port); err != nil {
			if closeErr := port.Close(); closeErr != nil && !isPortClosedError(closeErr) {
				logger.Default().Warn("close serial port after hw flow control failure", zap.Error(closeErr))
			}
			return "", fmt.Errorf("enable hardware flow control: %w", err)
		}
	case "software":
		if closeErr := port.Close(); closeErr != nil && !isPortClosedError(closeErr) {
			logger.Default().Warn("close serial port after unsupported flow control", zap.Error(closeErr))
		}
		return "", fmt.Errorf("software flow control (XON/XOFF) is not supported; use 'hardware' or 'none'")
	default:
		if closeErr := port.Close(); closeErr != nil && !isPortClosedError(closeErr) {
			logger.Default().Warn("close serial port after unsupported flow control", zap.Error(closeErr))
		}
		return "", fmt.Errorf("unsupported flow control mode: %q (supported: 'none', 'hardware')", cfg.FlowControl)
	}

	// 生成 session ID
	m.mu.Lock()
	m.counter++
	sessionID := fmt.Sprintf("serial-%d", m.counter)
	m.mu.Unlock()

	sess := &Session{
		ID:      sessionID,
		AssetID: cfg.AssetID,
		port:    port,
	}

	m.sessions.Store(sessionID, sess)
	m.watchCallbackSetup(sess, callbackSetupGracePeriod)

	return sessionID, nil
}

// SetCallbacks 设置会话的数据和关闭回调（在 Connect 返回 sessionId 后调用）。
// 回调设置完成后才启动读取 goroutine，避免 SetCallbacks 调用前的数据丢失。
func (m *Manager) SetCallbacks(sessionID string, onData func(data []byte), onClosed func(sessionID string)) {
	sess, ok := m.GetSession(sessionID)
	if !ok {
		return
	}
	startReader := false
	sess.mu.Lock()
	sess.onData = onData
	sess.onClosed = onClosed
	if !sess.readerStarted && !sess.closed {
		close(sess.ensureReaderReadyChLocked())
		sess.readerStarted = true
		startReader = true
	}
	sess.mu.Unlock()

	// 回调就绪后才启动读取，确保首屏输出不会因回调未挂载而丢失
	if startReader {
		go m.readOutput(sess)
	}
}

func (m *Manager) watchCallbackSetup(sess *Session, timeout time.Duration) {
	if timeout <= 0 {
		return
	}

	sess.mu.Lock()
	if sess.closed || sess.readerStarted {
		sess.mu.Unlock()
		return
	}
	readyCh := sess.ensureReaderReadyChLocked()
	closedCh := sess.ensureClosedChLocked()
	sessionID := sess.ID
	sess.mu.Unlock()

	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-readyCh:
			return
		case <-closedCh:
			return
		case <-timer.C:
			if m.closeSessionWithoutCallbacks(sessionID) {
				logger.Default().Warn(
					"close serial session without callbacks",
					zap.String("sessionID", sessionID),
					zap.Duration("timeout", timeout),
				)
			}
		}
	}()
}

func (m *Manager) closeSessionWithoutCallbacks(sessionID string) bool {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return false
	}

	sess := v.(*Session)
	sess.mu.Lock()
	if sess.closed || sess.readerStarted {
		sess.mu.Unlock()
		return false
	}
	port, onClosed, sessionID, shouldClose := sess.closeLocked()
	sess.mu.Unlock()
	if !shouldClose {
		return false
	}

	m.sessions.Delete(sessionID)
	if err := port.Close(); err != nil && !isPortClosedError(err) {
		logger.Default().Warn("close serial port after callback setup timeout", zap.String("sessionID", sessionID), zap.Error(err))
	}
	if onClosed != nil {
		go onClosed(sessionID)
	}
	return true
}

// readOutput 持续从串口读取数据并回调。
// 使用 10ms ticker 合并输出，减少高频 EventsEmit 调用导致前端事件队列阻塞。
// AI 命令执行期间通过 cmdCapture 即时收集每个 chunk，确保 ExecCommand 能及时收到数据。
func (m *Manager) readOutput(sess *Session) {
	defer func() {
		m.sessions.Delete(sess.ID)
		sess.Close()
	}()

	var pending bytes.Buffer
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if pending.Len() == 0 {
			return
		}
		sess.mu.Lock()
		handler := sess.onData
		sess.mu.Unlock()
		if handler != nil {
			data := make([]byte, pending.Len())
			copy(data, pending.Bytes())
			pending.Reset()
			handler(data)
		} else {
			pending.Reset()
		}
	}

	buf := make([]byte, 4096)
	for {
		if sess.IsClosed() {
			flush()
			return
		}
		n, err := sess.port.Read(buf)
		if n > 0 {
			// pending.Write 与 capture.Append 内部都会复制一份，所以这里不再额外 copy buf。
			chunk := buf[:n]
			pending.Write(chunk)
			// AI 命令执行：即时收集每个 chunk，避免高峰时丢失输出。
			sess.mu.Lock()
			capture := sess.cmdCapture
			sess.mu.Unlock()
			if capture != nil {
				capture.Append(chunk)
			}
			// 缓冲超过 32KB 时立即刷新，避免延迟过大
			if pending.Len() >= 32*1024 {
				flush()
			}
		}
		if err != nil {
			if err == io.EOF || isPortClosedError(err) || sess.IsClosed() {
				flush()
				return
			}
			logger.Default().Warn("serial read failed", zap.String("sessionID", sess.ID), zap.Error(err))
			flush()
			return
		}
		// n == 0: SetReadTimeout 超时，继续轮询
		select {
		case <-ticker.C:
			flush()
		default:
		}
	}
}

// GetSession 获取活跃会话
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	sess := v.(*Session)
	if sess.IsClosed() {
		m.sessions.Delete(sessionID)
		return nil, false
	}
	return sess, true
}

// GetSessionByAssetID 根据资产 ID 查找活跃的串口会话
func (m *Manager) GetSessionByAssetID(assetID int64) (CommandSession, bool) {
	var found CommandSession
	m.sessions.Range(func(_, value any) bool {
		sess := value.(*Session)
		if sess.AssetID == assetID && !sess.IsClosed() {
			found = sess
			return false
		}
		return true
	})
	return found, found != nil
}

// Disconnect 断开串口连接
func (m *Manager) Disconnect(sessionID string) {
	m.closeSession(sessionID)
}

// CloseAll 关闭所有活跃串口会话。
func (m *Manager) CloseAll() {
	var sessionIDs []string
	m.sessions.Range(func(key, _ any) bool {
		sessionIDs = append(sessionIDs, key.(string))
		return true
	})
	for _, sessionID := range sessionIDs {
		m.closeSession(sessionID)
	}
}

func buildSerialMode(cfg ConnectConfig) (*serial.Mode, error) {
	stopBits, err := parseStopBits(cfg.StopBits)
	if err != nil {
		return nil, err
	}
	parity, err := parseParity(cfg.Parity)
	if err != nil {
		return nil, err
	}
	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		StopBits: stopBits,
		Parity:   parity,
	}
	if mode.BaudRate == 0 {
		mode.BaudRate = 115200
	}
	if mode.DataBits == 0 {
		mode.DataBits = 8
	}
	return mode, nil
}

// parseStopBits 转换停止位字符串到 serial.StopBits
func parseStopBits(s string) (serial.StopBits, error) {
	switch s {
	case "", "1":
		return serial.OneStopBit, nil
	case "1.5":
		return serial.OnePointFiveStopBits, nil
	case "2":
		return serial.TwoStopBits, nil
	default:
		return 0, fmt.Errorf("unsupported stop bits %q (supported: \"1\", \"1.5\", \"2\")", s)
	}
}

// parseParity 转换校验位字符串到 serial.Parity
func parseParity(s string) (serial.Parity, error) {
	switch s {
	case "", "none":
		return serial.NoParity, nil
	case "odd":
		return serial.OddParity, nil
	case "even":
		return serial.EvenParity, nil
	case "mark":
		return serial.MarkParity, nil
	case "space":
		return serial.SpaceParity, nil
	default:
		return 0, fmt.Errorf("unsupported parity %q (supported: \"none\", \"odd\", \"even\", \"mark\", \"space\")", s)
	}
}

func (m *Manager) closeSession(sessionID string) {
	v, ok := m.sessions.LoadAndDelete(sessionID)
	if !ok {
		return
	}
	v.(*Session).Close()
}

func isPortClosedError(err error) bool {
	if err == nil {
		return false
	}
	var portErr *serial.PortError
	return errors.As(err, &portErr) && portErr.Code() == serial.PortClosed
}
