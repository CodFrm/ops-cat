package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Codex App Server JSON-RPC 2.0 适配器

// codexJSONRPC JSON-RPC 2.0 消息
type codexJSONRPC struct {
	Method string          `json:"method,omitempty"`
	ID     *int64          `json:"id,omitempty"`     // 请求时设置，通知时不设置
	Params json.RawMessage `json:"params,omitempty"` // 请求参数
	Result json.RawMessage `json:"result,omitempty"` // 响应结果
	Error  *codexRPCError  `json:"error,omitempty"`  // 错误
}

type codexRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CodexAppServer 管理与 codex app-server 的通信
type CodexAppServer struct {
	cliPath  string
	proc     *CLIProcess
	threadID string
	nextID   atomic.Int64
	mu       sync.Mutex

	// 响应等待
	pending   map[int64]chan codexJSONRPC
	pendingMu sync.Mutex

	// 通知事件分发
	notifyCh chan codexJSONRPC // 后台 reader 将通知事件发到这里
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewCodexAppServer 创建 Codex App Server 客户端
func NewCodexAppServer() *CodexAppServer {
	return &CodexAppServer{
		pending:  make(map[int64]chan codexJSONRPC),
		notifyCh: make(chan codexJSONRPC, 128),
	}
}

// Start 启动 codex app-server 进程并完成初始化握手
func (s *CodexAppServer) Start(ctx context.Context, cliPath, workDir string) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.cliPath = cliPath

	proc, err := StartCLIProcess(s.ctx, cliPath, []string{"app-server"}, workDir)
	if err != nil {
		return err
	}
	s.proc = proc

	// 启动后台 stdout reader（必须在 sendRequest 之前启动）
	go s.readLoop()

	// 初始化握手
	if err := s.initialize(); err != nil {
		stderrStr := proc.Stderr()
		s.Stop()
		if stderrStr != "" {
			return fmt.Errorf("Codex 初始化失败: %w\nstderr: %s", err, stderrStr)
		}
		return fmt.Errorf("Codex 初始化失败: %w", err)
	}

	return nil
}

// readLoop 后台持续读取 stdout，分发到 pending 响应或 notifyCh
func (s *CodexAppServer) readLoop() {
	lines := s.proc.ReadLines(s.ctx)
	for line := range lines {
		var msg codexJSONRPC
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// 有 ID 且无 method → 是对请求的响应
		if msg.ID != nil && msg.Method == "" {
			s.pendingMu.Lock()
			if ch, ok := s.pending[*msg.ID]; ok {
				ch <- msg
				delete(s.pending, *msg.ID)
			}
			s.pendingMu.Unlock()
			continue
		}

		// 否则是通知事件，发到 notifyCh
		select {
		case s.notifyCh <- msg:
		case <-s.ctx.Done():
			return
		}
	}
}

// initialize 发送 initialize 请求和 initialized 通知
func (s *CodexAppServer) initialize() error {
	version := getCLIVersion(s.cliPath)
	initParams := map[string]any{
		"clientInfo": map[string]any{
			"name":    "codex-cli",
			"version": version,
		},
	}
	_, err := s.sendRequest("initialize", initParams)
	if err != nil {
		return err
	}

	// 发送 initialized 通知（无 id）
	return s.sendNotification("initialized", nil)
}

// StartThread 开始新的对话线程
func (s *CodexAppServer) StartThread() error {
	result, err := s.sendRequest("thread/start", map[string]any{})
	if err != nil {
		return err
	}

	var resp struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return fmt.Errorf("解析 thread/start 响应失败: %w", err)
	}
	s.mu.Lock()
	s.threadID = resp.Thread.ID
	s.mu.Unlock()
	return nil
}

// SendTurn 发送用户消息开始一个 turn
func (s *CodexAppServer) SendTurn(ctx context.Context, text string, onEvent func(StreamEvent)) error {
	s.mu.Lock()
	threadID := s.threadID
	s.mu.Unlock()

	if threadID == "" {
		if err := s.StartThread(); err != nil {
			return err
		}
		s.mu.Lock()
		threadID = s.threadID
		s.mu.Unlock()
	}

	params := map[string]any{
		"threadId": threadID,
		"input":    []map[string]any{{"type": "text", "text": text}},
	}

	_, err := s.sendRequest("turn/start", params)
	if err != nil {
		return err
	}

	// 从 notifyCh 读取事件直到 turn 完成
	for {
		select {
		case msg := <-s.notifyCh:
			done := s.handleNotification(msg.Method, msg.Params, onEvent)
			if done {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// codexItem Codex item 通用结构（camelCase 类型名）
type codexItem struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Command  string          `json:"command"`          // commandExecution
	Path     string          `json:"path"`             // fileRead / fileWrite
	Output   string          `json:"output"`           // commandExecution completed
	Content  json.RawMessage `json:"content"`          // 可能是 string 或 array
	ExitCode *int            `json:"exitCode"`         // commandExecution completed
	Text     string          `json:"text"`             // agentMessage completed
}

// contentString 安全提取 content 字段为字符串
func (item *codexItem) contentString() string {
	if item.Content == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(item.Content, &s); err == nil {
		return s
	}
	return ""
}

// handleNotification 处理 Codex 通知事件
func (s *CodexAppServer) handleNotification(method string, params json.RawMessage, onEvent func(StreamEvent)) bool {
	switch method {
	// ── 文本流式输出 ──
	case "codex/event/agent_message_delta":
		var p struct {
			Msg struct {
				Delta string `json:"delta"`
			} `json:"msg"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.Msg.Delta != "" {
			onEvent(StreamEvent{Type: "content", Content: p.Msg.Delta})
		}

	// ── 命令执行 ──
	case "codex/event/exec_command_begin":
		var p struct {
			Msg struct {
				Command string `json:"command"`
			} `json:"msg"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.Msg.Command != "" {
			onEvent(StreamEvent{Type: "tool_start", ToolName: "Bash", ToolInput: p.Msg.Command})
		}

	case "codex/event/exec_command_end":
		var p struct {
			Msg struct {
				ExitCode int    `json:"exit_code"`
				Stdout   string `json:"stdout"`
				Stderr   string `json:"stderr"`
			} `json:"msg"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			result := p.Msg.Stdout
			if p.Msg.Stderr != "" {
				if result != "" {
					result += "\n"
				}
				result += p.Msg.Stderr
			}
			if p.Msg.ExitCode != 0 {
				result = fmt.Sprintf("exit code %d\n%s", p.Msg.ExitCode, result)
			}
			onEvent(StreamEvent{Type: "tool_result", ToolName: "Bash", Content: truncateOutput(result, 20)})
		}

	// ── item 事件（camelCase 类型名）──
	case "item/started":
		var p struct {
			Item codexItem `json:"item"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			s.handleItemStarted(&p.Item, onEvent)
		}

	case "item/completed":
		var p struct {
			Item codexItem `json:"item"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			s.handleItemCompleted(&p.Item, onEvent)
		}

	// ── turn 生命周期 ──
	case "turn/completed":
		return true

	case "turn/failed":
		var p struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			onEvent(StreamEvent{Type: "error", Error: p.Error})
		}
		return true

	// ── 静默忽略的事件 ──
	case "codex/event/agent_message_content_delta",
		"codex/event/agent_message",
		"codex/event/item_started",
		"codex/event/item_completed",
		"codex/event/token_count",
		"codex/event/task_started",
		"codex/event/task_complete",
		"codex/event/user_message",
		"codex/event/mcp_startup_complete",
		"item/agentMessage/delta",
		"thread/started",
		"thread/status/changed",
		"thread/tokenUsage/updated",
		"account/rateLimits/updated",
		"turn/started",
		"configWarning":
		// 忽略
	}

	return false
}

func (s *CodexAppServer) handleItemStarted(item *codexItem, onEvent func(StreamEvent)) {
	switch item.Type {
	case "commandExecution":
		if item.Command != "" {
			onEvent(StreamEvent{Type: "tool_start", ToolName: "Bash", ToolInput: item.Command})
		}
	case "fileRead":
		if item.Path != "" {
			onEvent(StreamEvent{Type: "tool_start", ToolName: "Read", ToolInput: item.Path})
		}
	case "fileWrite":
		if item.Path != "" {
			onEvent(StreamEvent{Type: "tool_start", ToolName: "Write", ToolInput: item.Path})
		}
	// agentMessage, userMessage, reasoning: 忽略
	}
}

func (s *CodexAppServer) handleItemCompleted(item *codexItem, onEvent func(StreamEvent)) {
	switch item.Type {
	case "commandExecution":
		result := item.Output
		if item.ExitCode != nil && *item.ExitCode != 0 {
			result = fmt.Sprintf("exit code %d\n%s", *item.ExitCode, result)
		}
		onEvent(StreamEvent{Type: "tool_result", ToolName: "Bash", Content: result})
	case "fileRead":
		onEvent(StreamEvent{Type: "tool_result", ToolName: "Read", Content: truncateOutput(item.contentString(), 20)})
	case "fileWrite":
		onEvent(StreamEvent{Type: "tool_result", ToolName: "Write", Content: item.Path})
	// agentMessage: 忽略，delta 已经发送过
	}
}

// truncateOutput 截断长输出
func truncateOutput(s string, maxLines int) string {
	lines := 0
	for i, ch := range s {
		if ch == '\n' {
			lines++
			if lines >= maxLines {
				remaining := 0
				for _, c := range s[i+1:] {
					if c == '\n' {
						remaining++
					}
				}
				if remaining > 0 {
					return s[:i] + fmt.Sprintf("\n... (%d more lines)", remaining)
				}
				return s
			}
		}
	}
	return s
}

// sendRequest 发送 JSON-RPC 请求并等待响应
func (s *CodexAppServer) sendRequest(method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	paramsData, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	ch := make(chan codexJSONRPC, 1)
	s.pendingMu.Lock()
	s.pending[id] = ch
	s.pendingMu.Unlock()

	msg := codexJSONRPC{
		Method: method,
		ID:     &id,
		Params: paramsData,
	}
	if err := s.proc.WriteJSON(msg); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("Codex RPC 错误: %s", resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(30 * time.Second):
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		stderrStr := s.proc.Stderr()
		if stderrStr != "" {
			return nil, fmt.Errorf("Codex 请求超时 (%s)\nstderr: %s", method, stderrStr)
		}
		return nil, fmt.Errorf("Codex 请求超时: %s", method)
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// sendNotification 发送 JSON-RPC 通知（无 id）
func (s *CodexAppServer) sendNotification(method string, params any) error {
	var paramsData json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsData = data
	}

	msg := codexJSONRPC{
		Method: method,
		Params: paramsData,
	}
	return s.proc.WriteJSON(msg)
}

// Stop 停止 app-server 进程
func (s *CodexAppServer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.proc != nil {
		s.proc.Stop()
	}
}
