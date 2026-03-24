package approval

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// ApprovalRequest is sent from opsctl to the desktop app.
type ApprovalRequest struct {
	Type        string     `json:"type"` // "exec"|"cp"|"create"|"update"|"plan"
	AssetID     int64      `json:"asset_id,omitempty"`
	AssetName   string     `json:"asset_name,omitempty"`
	Command     string     `json:"command,omitempty"`
	Detail      string     `json:"detail"`
	SessionID   string     `json:"session_id,omitempty"`  // 统一 session 标识（审批 session 或 plan session）
	PlanItems   []PlanItem `json:"plan_items,omitempty"`  // type="plan" 时使用
	Description string     `json:"description,omitempty"` // 计划描述
}

// PlanItem 计划中的单条操作
type PlanItem struct {
	Type      string `json:"type"` // "exec", "cp", "create", "update"
	AssetID   int64  `json:"asset_id"`
	AssetName string `json:"asset_name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Command   string `json:"command"`
	Detail    string `json:"detail"`
}

// ApprovalResponse is sent from the desktop app back to opsctl.
type ApprovalResponse struct {
	Approved       bool   `json:"approved"`
	Reason         string `json:"reason,omitempty"`
	SessionID      string `json:"session_id,omitempty"`      // plan 审批返回 / session 标识
	ApproveSession bool   `json:"approve_session,omitempty"` // 用户选择了"允许当前会话"
}

// SocketPath returns the approval socket path for the given data directory.
func SocketPath(dataDir string) string {
	return filepath.Join(dataDir, "approval.sock")
}

// --- Server ---

// ApprovalHandler processes an approval request and returns a response.
type ApprovalHandler func(req ApprovalRequest) ApprovalResponse

// Server listens on a Unix socket for approval requests from opsctl.
type Server struct {
	handler  ApprovalHandler
	listener net.Listener
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewServer creates a new approval server.
func NewServer(handler ApprovalHandler) *Server {
	return &Server{
		handler: handler,
		done:    make(chan struct{}),
	}
}

// Start begins listening on the Unix socket at socketPath.
// Removes stale socket file if it exists.
func (s *Server) Start(socketPath string) error {
	// Clean up stale socket
	if _, err := os.Stat(socketPath); err == nil {
		// Try to connect - if successful, another instance is running
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			if err := conn.Close(); err != nil {
				logger.Default().Warn("close connection check", zap.Error(err))
			}
			return fmt.Errorf("another instance is already listening on %s", socketPath)
		}
		// Stale socket, remove it
		if err := os.Remove(socketPath); err != nil {
			logger.Default().Warn("remove stale socket", zap.String("path", socketPath), zap.Error(err))
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	s.listener = listener

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop closes the listener, removes the socket file, and waits for goroutines.
func (s *Server) Stop() {
	close(s.done)
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			logger.Default().Warn("close listener", zap.Error(err))
		}
	}
	s.wg.Wait()
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Default().Warn("close client connection", zap.Error(err))
		}
	}()

	var req ApprovalRequest
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&req); err != nil {
		resp := ApprovalResponse{Approved: false, Reason: "invalid request"}
		if err := json.NewEncoder(conn).Encode(resp); err != nil {
			logger.Default().Warn("encode error response", zap.Error(err))
		}
		return
	}

	resp := s.handler(req)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		logger.Default().Warn("encode approval response", zap.Error(err))
	}
}

// --- Client ---

// SendNotification sends a data-change notification to the desktop app (fire-and-forget).
// resource indicates what changed, e.g. "asset".
func SendNotification(socketPath string, resource string) {
	if _, err := RequestApproval(socketPath, ApprovalRequest{
		Type:   "notify",
		Detail: resource,
	}); err != nil {
		logger.Default().Warn("send notification", zap.String("resource", resource), zap.Error(err))
	}
}

// RequestApproval connects to the Unix socket and sends an approval request.
// Blocks until a response is received.
func RequestApproval(socketPath string, req ApprovalRequest) (ApprovalResponse, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return ApprovalResponse{}, fmt.Errorf("cannot connect to desktop app (is it running?): %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Default().Warn("close request connection", zap.Error(err))
		}
	}()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return ApprovalResponse{}, fmt.Errorf("send request: %w", err)
	}

	var resp ApprovalResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return ApprovalResponse{}, fmt.Errorf("read response: %w", err)
	}

	return resp, nil
}
