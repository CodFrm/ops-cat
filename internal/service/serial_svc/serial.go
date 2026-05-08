package serial_svc

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.bug.st/serial"
	"go.uber.org/zap"
)

// Session 表示一个活跃的串口终端会话
type Session struct {
	ID       string
	AssetID  int64
	port     serial.Port
	mu       sync.Mutex
	closed   bool
	onData   func(data []byte)      // 终端输出回调
	onClosed func(sessionID string) // 会话关闭回调
}

// Write 向串口写入数据（用户输入）
func (s *Session) Write(data []byte) error {
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
	if err := s.port.Close(); err != nil {
		logger.Default().Warn("close serial port", zap.String("sessionID", s.ID), zap.Error(err))
	}
	if s.onClosed != nil {
		go s.onClosed(s.ID)
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
		port.Close()
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

	// 启动读取 goroutine
	go m.readOutput(sess)

	return sessionID, nil
}

// SetCallbacks 设置会话的数据和关闭回调（在 Connect 返回 sessionId 后调用）
func (m *Manager) SetCallbacks(sessionID string, onData func(data []byte), onClosed func(sessionID string)) {
	sess, ok := m.GetSession(sessionID)
	if !ok {
		return
	}
	sess.mu.Lock()
	sess.onData = onData
	sess.onClosed = onClosed
	sess.mu.Unlock()
}

// readOutput 持续从串口读取数据并回调
func (m *Manager) readOutput(sess *Session) {
	buf := make([]byte, 4096)
	for {
		if sess.IsClosed() {
			return
		}
		n, err := sess.port.Read(buf)
		if n > 0 {
			sess.mu.Lock()
			handler := sess.onData
			sess.mu.Unlock()
			if handler != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				handler(data)
			}
		}
		if err != nil {
			if err == io.EOF || sess.IsClosed() {
				return
			}
			// 超时不算错误，继续读
			if sess.IsClosed() {
				return
			}
			continue
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

// Disconnect 断开串口连接
func (m *Manager) Disconnect(sessionID string) {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return
	}
	sess := v.(*Session)
	sess.Close()
	m.sessions.Delete(sessionID)
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
