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

// BatchItem 批量执行中的单条操作
type BatchItem struct {
	Type      string `json:"type"` // "exec"|"sql"|"redis"
	AssetID   int64  `json:"asset_id"`
	AssetName string `json:"asset_name"`
	Command   string `json:"command"`
}

// ApprovalRequest is sent from opsctl to the desktop app.
type ApprovalRequest struct {
	Token       string      `json:"token,omitempty"` // 认证 token
	Type        string      `json:"type"`            // "exec"|"cp"|"create"|"update"|"grant"|"batch"|"ext_tool"
	AssetID     int64       `json:"asset_id,omitempty"`
	AssetName   string      `json:"asset_name,omitempty"`
	Command     string      `json:"command,omitempty"`
	Detail      string      `json:"detail"`
	SessionID   string      `json:"session_id,omitempty"`  // 统一 session 标识（审批 session 或 grant session）
	GrantItems  []GrantItem `json:"grant_items,omitempty"` // type="grant" 时使用
	BatchItems  []BatchItem `json:"batch_items,omitempty"` // type="batch" 时使用
	Description string      `json:"description,omitempty"` // 授权描述
	// ext_tool 扩展字段
	Extension string `json:"extension,omitempty"` // 扩展名，如 "oss"
	Tool      string `json:"tool,omitempty"`      // 工具名，如 "list_buckets"
	ArgsJSON  string `json:"args_json,omitempty"` // 工具参数 JSON
}

// GrantItem 授权中的单条操作
type GrantItem struct {
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
	Approved       bool        `json:"approved"`
	Reason         string      `json:"reason,omitempty"`
	SessionID      string      `json:"session_id,omitempty"`      // grant 审批返回 / session 标识
	ApproveGrant   bool        `json:"approve_grant,omitempty"`   // 用户选择了"记住并允许"
	MatchedPattern string      `json:"matched_pattern,omitempty"` // session 匹配时命中的规则模式
	EditedItems    []GrantItem `json:"edited_items,omitempty"`    // 用户编辑后的 grant items
	// ext_tool 响应
	Result string `json:"result,omitempty"` // 工具执行结果
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
	handler   ApprovalHandler
	listener  net.Listener
	done      chan struct{}
	wg        sync.WaitGroup
	authToken string // 认证 token，非空时校验
}

// NewServer creates a new approval server.
func NewServer(handler ApprovalHandler, authToken string) *Server {
	return &Server{
		handler:   handler,
		done:      make(chan struct{}),
		authToken: authToken,
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
	// 设置 socket 文件权限为 0600（仅所有者可访问）
	if err := os.Chmod(socketPath, 0600); err != nil {
		logger.Default().Warn("chmod socket", zap.String("path", socketPath), zap.Error(err))
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

	// 校验认证 token
	if s.authToken != "" && req.Token != s.authToken {
		resp := ApprovalResponse{Approved: false, Reason: "authentication failed"}
		if err := json.NewEncoder(conn).Encode(resp); err != nil {
			logger.Default().Warn("encode auth error response", zap.Error(err))
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
func SendNotification(socketPath, token, resource string) {
	if _, err := RequestApprovalWithToken(socketPath, token, ApprovalRequest{
		Type:   "notify",
		Detail: resource,
	}); err != nil {
		logger.Default().Warn("send notification", zap.String("resource", resource), zap.Error(err))
	}
}

// RequestApprovalWithToken connects to the Unix socket and sends an approval request with auth token.
func RequestApprovalWithToken(socketPath, token string, req ApprovalRequest) (ApprovalResponse, error) {
	req.Token = token
	return RequestApproval(socketPath, req)
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
