package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// ToolExecutor 执行 tool 调用的接口
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args string) (string, error)
}

// Agent AI 代理，管理对话循环和 tool 调度
type Agent struct {
	provider Provider
	executor ToolExecutor
	tools    []Tool
}

// NewAgent 创建 Agent
func NewAgent(provider Provider, executor ToolExecutor) *Agent {
	return &Agent{
		provider: provider,
		executor: executor,
		tools:    ToOpenAITools(AllToolDefs()),
	}
}

// Chat 发起对话，处理 tool 调用循环，通过回调流式返回内容
func (a *Agent) Chat(ctx context.Context, messages []Message, onEvent func(StreamEvent)) error {
	// Chat 结束后关闭 executor 持有的资源（如缓存的 SSH 连接）
	if closer, ok := a.executor.(io.Closer); ok {
		defer closer.Close()
	}

	const maxRounds = 10 // 防止无限循环

	for round := 0; round < maxRounds; round++ {
		ch, err := a.provider.Chat(ctx, messages, a.tools)
		if err != nil {
			return fmt.Errorf("provider chat 失败: %w", err)
		}

		var contentBuf string
		var toolCalls []ToolCall
		hasToolCall := false

		for event := range ch {
			switch event.Type {
			case "content":
				contentBuf += event.Content
				onEvent(event)
			case "tool_start", "tool_result":
				onEvent(event)
			case "tool_call":
				toolCalls = event.ToolCalls
				hasToolCall = true
				onEvent(event)
			case "error":
				onEvent(event)
				return fmt.Errorf("provider 错误: %s", event.Error)
			case "done":
				// 不立即转发 done，可能还有 tool 调用
			}
		}

		// 没有 tool 调用，对话结束
		if !hasToolCall {
			onEvent(StreamEvent{Type: "done"})
			return nil
		}

		// 将 assistant 的回复（含 tool_calls）加入消息
		assistantMsg := Message{
			Role:      RoleAssistant,
			Content:   contentBuf,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMsg)

		// 执行每个 tool 调用（Local CLI 模式下 executor 为 nil，不执行）
		if a.executor == nil {
			onEvent(StreamEvent{Type: "done"})
			return nil
		}
		for _, tc := range toolCalls {
			result, err := a.executor.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %s", err.Error())
			}
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
		// 继续下一轮对话
	}

	onEvent(StreamEvent{Type: "done"})
	return nil
}

// DefaultToolExecutor 默认工具执行器，通过统一注册表调度，缓存 SSH 连接供同一次 Chat 复用
type DefaultToolExecutor struct {
	handlers map[string]ToolHandlerFunc
	sshCache *SSHClientCache
}

func NewDefaultToolExecutor() *DefaultToolExecutor {
	handlers := make(map[string]ToolHandlerFunc)
	for _, def := range AllToolDefs() {
		handlers[def.Name] = def.Handler
	}
	return &DefaultToolExecutor{
		handlers: handlers,
		sshCache: NewSSHClientCache(),
	}
}

// Close 关闭所有缓存的 SSH 连接
func (e *DefaultToolExecutor) Close() error {
	return e.sshCache.Close()
}

func (e *DefaultToolExecutor) Execute(ctx context.Context, name string, argsJSON string) (string, error) {
	handler, ok := e.handlers[name]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	// 注入 SSH 缓存，run_command 会自动使用
	ctx = WithSSHCache(ctx, e.sshCache)
	return handler(ctx, args)
}
