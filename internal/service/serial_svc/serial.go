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

// Session 表示一个活跃的串口终端会话
type Session struct {
	ID       string
	AssetID  int64
	port     serial.Port
	writeMu  sync.Mutex
	mu       sync.Mutex
	closed   bool
	onData   func(data []byte)      // 终端输出回调
	onClosed func(sessionID string) // 会话关闭回调

	// AI 命令执行辅助：当 cmdOutputCh 非 nil 时，readOutput 会同时向此 channel 发送数据副本
	cmdOutputCh chan []byte
}

// Write 向串口写入数据（用户输入）
func (s *Session) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writeLocked(data)
}

func (s *Session) writeLocked(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session is closed")
	}
	_, err := s.port.Write(data)
	return err
}

// Resize 调整终端尺寸（串口无实际作用，保留接口兼容）
func (s *Session) Resize(cols, rows int) error {
	return nil // no-op for serial
}

// Close 关闭串口会话
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if err := s.port.Close(); err != nil && !isPortClosedError(err) {
		logger.Default().Warn("close serial port", zap.String("sessionID", s.ID), zap.Error(err))
	}
	if s.onClosed != nil {
		go s.onClosed(s.ID)
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
		return "", fmt.Errorf("session is closed")
	}
	// 设置命令输出收集 channel
	ch := make(chan []byte, 256)
	s.cmdOutputCh = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.cmdOutputCh = nil
		s.mu.Unlock()
	}()

	// 写入命令 + 回车
	if err := s.writeLocked([]byte(command + "\r\n")); err != nil {
		return "", fmt.Errorf("write command: %w", err)
	}

	var output []byte
	silenceTimer := time.NewTimer(silenceTimeout)
	maxTimer := time.NewTimer(maxTimeout)
	defer silenceTimer.Stop()
	defer maxTimer.Stop()

	for {
		select {
		case data := <-ch:
			output = append(output, data...)
			// 收到数据后重置静默计时器
			if !silenceTimer.Stop() {
				select {
				case <-silenceTimer.C:
				default:
				}
			}
			silenceTimer.Reset(silenceTimeout)
		case <-silenceTimer.C:
			// 输出静默，认为命令执行完毕
			return string(output), nil
		case <-maxTimer.C:
			// 超过最大等待时间
			return string(output), nil
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
func (m *Manager) Connect(cfg ConnectConfig) (string, error) {
	// 构建 serial 模式
	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		StopBits: toStopBits(cfg.StopBits),
		Parity:   toParity(cfg.Parity),
	}

	// 默认值
	if mode.BaudRate == 0 {
		mode.BaudRate = 115200
	}
	if mode.DataBits == 0 {
		mode.DataBits = 8
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

	// 设置流控制
	switch cfg.FlowControl {
	case "hardware":
		if err := port.SetDTR(true); err != nil {
			logger.Default().Warn("set DTR", zap.Error(err))
		}
		if err := port.SetRTS(true); err != nil {
			logger.Default().Warn("set RTS", zap.Error(err))
		}
	case "software":
		if closeErr := port.Close(); closeErr != nil && !isPortClosedError(closeErr) {
			logger.Default().Warn("close serial port after unsupported flow control", zap.Error(closeErr))
		}
		return "", fmt.Errorf("software flow control (XON/XOFF) is not supported; use 'hardware' or 'none'")
	case "", "none":
		// no flow control
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

	return sessionID, nil
}

// SetCallbacks 设置会话的数据和关闭回调（在 Connect 返回 sessionId 后调用）。
// 回调设置完成后才启动读取 goroutine，避免 SetCallbacks 调用前的数据丢失。
func (m *Manager) SetCallbacks(sessionID string, onData func(data []byte), onClosed func(sessionID string)) {
	sess, ok := m.GetSession(sessionID)
	if !ok {
		return
	}
	sess.mu.Lock()
	sess.onData = onData
	sess.onClosed = onClosed
	sess.mu.Unlock()

	// 回调就绪后才启动读取，确保首屏输出不会因回调未挂载而丢失
	go m.readOutput(sess)
}

// readOutput 持续从串口读取数据并回调。
// 使用 10ms ticker 合并输出，减少高频 EventsEmit 调用导致前端事件队列阻塞。
// cmdOutputCh（AI 命令执行）仍按每个 chunk 即时转发，确保 ExecCommand 能及时收到数据。
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
			data := make([]byte, n)
			copy(data, buf[:n])
			pending.Write(data)
			// AI 命令执行：即时转发每个 chunk（ExecCommand 依赖 channel 实时性）
			sess.mu.Lock()
			cmdCh := sess.cmdOutputCh
			sess.mu.Unlock()
			if cmdCh != nil {
				select {
				case cmdCh <- data:
				default: // 避免阻塞
				}
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

// toStopBits 转换停止位字符串到 serial.StopBits
func toStopBits(s string) serial.StopBits {
	switch s {
	case "1.5":
		return serial.OnePointFiveStopBits
	case "2":
		return serial.TwoStopBits
	default:
		return serial.OneStopBit
	}
}

// toParity 转换校验位字符串到 serial.Parity
func toParity(s string) serial.Parity {
	switch s {
	case "odd":
		return serial.OddParity
	case "even":
		return serial.EvenParity
	case "mark":
		return serial.MarkParity
	case "space":
		return serial.SpaceParity
	default:
		return serial.NoParity
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
