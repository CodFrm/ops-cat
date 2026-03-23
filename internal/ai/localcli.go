package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// PermissionRequest CLI 工具权限请求
type PermissionRequest struct {
	ToolName string         `json:"tool_name"`
	Input    map[string]any `json:"input"`
}

// PermissionResponse 权限响应
type PermissionResponse struct {
	Behavior string `json:"behavior"` // "allow" | "deny"
	Message  string `json:"message"`  // deny 原因
}

// LocalCLIProvider 本地 CLI provider（claude/codex）
type LocalCLIProvider struct {
	name      string
	cliPath   string // CLI 可执行文件路径
	cliType   string // "claude" 或 "codex"
	sessionID string // Claude session ID，跨调用保持
	mu        sync.Mutex

	// MCP 工作目录，包含 .mcp.json（Claude CLI 用）
	mcpWorkDir string

	// MCP Server URL（Codex 通过 -c 参数传入）
	mcpServerURL string

	// Codex app-server 实例
	codexServer *CodexAppServer

	// OnPermissionRequest 权限确认回调，由外部注入
	OnPermissionRequest func(req PermissionRequest) PermissionResponse
}

// SetMCPWorkDir 设置 MCP 配置所在的工作目录
func (p *LocalCLIProvider) SetMCPWorkDir(dir string) {
	p.mcpWorkDir = dir
}

// SetMCPServerURL 设置 MCP Server URL（Codex 通过 -c 参数传入）
func (p *LocalCLIProvider) SetMCPServerURL(url string) {
	p.mcpServerURL = url
}

// GetCodexServer 返回 Codex app-server 实例（用于转发确认响应）
func (p *LocalCLIProvider) GetCodexServer() *CodexAppServer {
	return p.codexServer
}

// NewLocalCLIProvider 创建本地 CLI provider
func NewLocalCLIProvider(name, cliPath, cliType string) *LocalCLIProvider {
	return &LocalCLIProvider{
		name:    name,
		cliPath: cliPath,
		cliType: cliType,
	}
}

func (p *LocalCLIProvider) Name() string { return p.name }

func (p *LocalCLIProvider) Chat(ctx context.Context, messages []Message, _ []Tool) (<-chan StreamEvent, error) {
	switch p.cliType {
	case "claude":
		return p.chatClaude(ctx, messages)
	case "codex":
		return p.chatCodex(ctx, messages)
	default:
		return nil, fmt.Errorf("不支持的 CLI 类型: %s", p.cliType)
	}
}

// chatClaude 使用 Claude CLI stream-json 模式
func (p *LocalCLIProvider) chatClaude(ctx context.Context, messages []Message) (<-chan StreamEvent, error) {
	// 提取最新 user 消息和 system prompt
	userMsg, systemPrompt := extractLastUserAndSystem(messages)
	if userMsg == "" {
		return nil, fmt.Errorf("没有用户消息")
	}

	// 构建 CLI 参数
	args := p.buildClaudeArgs(userMsg, systemPrompt)

	// 启动 CLI 进程（在 MCP 工作目录下，让 CLI 自动发现 .mcp.json）
	proc, err := StartCLIProcess(ctx, p.cliPath, args, p.mcpWorkDir)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer proc.Stop()

		parser := NewClaudeEventParser()
		lines := proc.ReadLines(ctx)

		for line := range lines {
			events, done := parser.ParseLine(line)
			for _, ev := range events {
				ch <- ev
			}
			if done {
				// 更新 sessionID 用于续话
				if parser.SessionID != "" {
					p.mu.Lock()
					p.sessionID = parser.SessionID
					p.mu.Unlock()
				}
				ch <- StreamEvent{Type: "done"}
				return
			}
		}

		// 进程结束但没收到 result 事件，检查是否有错误
		if parser.SessionID != "" {
			p.mu.Lock()
			p.sessionID = parser.SessionID
			p.mu.Unlock()
		}
		err := proc.Wait()
		if stderrStr := proc.Stderr(); stderrStr != "" {
			ch <- StreamEvent{Type: "error", Error: stderrStr}
		} else if err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Sprintf("CLI 进程退出: %s", err)}
		}
		ch <- StreamEvent{Type: "done"}
	}()

	return ch, nil
}

// buildClaudeArgs 构建 Claude CLI 参数
func (p *LocalCLIProvider) buildClaudeArgs(userMsg, systemPrompt string) []string {
	p.mu.Lock()
	sessionID := p.sessionID
	p.mu.Unlock()

	args := []string{
		"-p", userMsg,
		"--output-format", "stream-json",
	}

	if sessionID != "" {
		// 续话模式
		args = append(args, "-r", sessionID)
	} else {
		// 首次调用，添加系统提示
		if systemPrompt != "" {
			args = append(args, "--append-system-prompt", systemPrompt)
		}
		// 首次跳过权限（后续将替换为权限确认流程）
		args = append(args, "--dangerously-skip-permissions")
	}

	return args
}

// chatCodex 使用 Codex app-server 持久进程
func (p *LocalCLIProvider) chatCodex(ctx context.Context, messages []Message) (<-chan StreamEvent, error) {
	userMsg, _ := extractLastUserAndSystem(messages)
	if userMsg == "" {
		return nil, fmt.Errorf("没有用户消息")
	}

	// 懒启动 app-server
	if p.codexServer == nil {
		server := NewCodexAppServer()
		server.OnPermissionRequest = p.OnPermissionRequest
		if err := server.Start(ctx, p.cliPath, p.mcpWorkDir, p.mcpServerURL); err != nil {
			return nil, fmt.Errorf("启动 Codex app-server 失败: %w", err)
		}
		p.codexServer = server
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		err := p.codexServer.SendTurn(ctx, userMsg, func(ev StreamEvent) {
			ch <- ev
		})
		if err != nil {
			ch <- StreamEvent{Type: "error", Error: err.Error()}
		}
		ch <- StreamEvent{Type: "done"}
	}()

	return ch, nil
}

// extractLastUserAndSystem 提取最新的 user 消息和 system prompt
func extractLastUserAndSystem(messages []Message) (userMsg, systemPrompt string) {
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemPrompt = msg.Content
		}
	}
	// 从后往前找最新的 user 消息
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			userMsg = messages[i].Content
			break
		}
	}
	return
}

// GetSessionID 获取当前 Claude CLI sessionID
func (p *LocalCLIProvider) GetSessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessionID
}

// SetSessionID 恢复 Claude CLI sessionID（切换会话时使用）
func (p *LocalCLIProvider) SetSessionID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionID = id
}

// ResetSession 重置会话（用户清空聊天时调用）
func (p *LocalCLIProvider) ResetSession() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionID = ""
	if p.codexServer != nil {
		p.codexServer.Stop()
		p.codexServer = nil
	}
}

// messagesToPrompt 将消息列表转换为文本 prompt（Codex 回退用）
func messagesToPrompt(messages []Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			parts = append(parts, "[System]\n"+msg.Content)
		case RoleUser:
			parts = append(parts, msg.Content)
		case RoleAssistant:
			parts = append(parts, "[Assistant]\n"+msg.Content)
		case RoleTool:
			parts = append(parts, "[Tool Result]\n"+msg.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// DetectLocalCLIs 检测本地安装的 AI CLI 工具
func DetectLocalCLIs() []CLIInfo {
	var results []CLIInfo

	clis := []struct {
		name    string
		cliType string
		cmds    []string
	}{
		{"Claude Code", "claude", []string{"claude"}},
		{"Codex", "codex", []string{"codex"}},
	}

	for _, cli := range clis {
		for _, cmd := range cli.cmds {
			path, err := exec.LookPath(cmd)
			if err == nil {
				version := getCLIVersion(path)
				results = append(results, CLIInfo{
					Name:    cli.name,
					Type:    cli.cliType,
					Path:    path,
					Version: version,
				})
				break
			}
		}
	}

	return results
}

// CLIInfo 本地 CLI 信息
type CLIInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

func getCLIVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "unknown"
	}
	version := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(version, '\n'); idx > 0 {
		version = version[:idx]
	}
	return version
}

// CLIInfoJSON 序列化 CLIInfo 列表
func CLIInfoJSON(infos []CLIInfo) string {
	data, _ := json.Marshal(infos)
	return string(data)
}
