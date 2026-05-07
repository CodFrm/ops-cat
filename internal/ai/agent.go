package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/opskat/opskat/internal/service/kafka_svc"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// needsReasoningContent 判断当前模型是否要求多轮 tool 调用 assistant 消息回传 reasoning_content。
// 目前只有 DeepSeek-v4 thinking 模式有此强制要求；其他 provider 保持原状以减少回归面。
func needsReasoningContent(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "deepseek-v4")
}

// ToolExecutor 执行 tool 调用的接口
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args string) (string, error)
}

// Agent AI 代理，管理对话循环和 tool 调度
type Agent struct {
	provider      Provider
	newExecutor   func() ToolExecutor // 每次 Chat 调用创建独立 executor，避免共享资源竞争
	tools         []Tool
	policyChecker *CommandPolicyChecker
	config        AgentConfig
}

// NewAgent 创建 Agent，newExecutor 工厂在每次 Chat 时生成独立 executor
func NewAgent(provider Provider, newExecutor func() ToolExecutor, checker *CommandPolicyChecker, config AgentConfig) *Agent {
	tools := config.Tools
	if tools == nil {
		tools = AllToolDefs()
	}
	return &Agent{
		provider:      provider,
		newExecutor:   newExecutor,
		tools:         ToOpenAITools(tools),
		policyChecker: checker,
		config:        config,
	}
}

// GetProvider 返回 Agent 的 Provider（供 Sub Agent 复用）
func (a *Agent) GetProvider() Provider {
	return a.provider
}

// GetPolicyChecker 返回 Agent 的 PolicyChecker（供 Sub Agent 复用）
func (a *Agent) GetPolicyChecker() *CommandPolicyChecker {
	return a.policyChecker
}

// Chat 发起对话，处理 tool 调用循环，通过回调流式返回内容
// getPendingMessages 可选，用于在工具调用后、下一轮 LLM 调用前注入排队的用户消息
func (a *Agent) Chat(ctx context.Context, messages []Message, onEvent func(StreamEvent), getPendingMessages func() []pendingMessage) error {
	// 每次 Chat 创建独立 executor，结束后关闭其持有的资源（如缓存的 SSH 连接）
	executor := a.newExecutor()
	if closer, ok := executor.(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				logger.Ctx(ctx).Warn("close tool executor", zap.Error(err))
			}
		}()
	}

	// 注入 PolicyChecker 到 context
	if a.policyChecker != nil {
		ctx = WithPolicyChecker(ctx, a.policyChecker)
	}

	maxRounds := a.config.effectiveMaxRounds()
	maxResultLen := a.config.effectiveMaxResultLen()

	for round := 0; round < maxRounds; round++ {
		// 检查是否需要压缩上下文
		if a.config.ContextWindow > 0 && needsCompression(messages, a.config.ContextWindow) {
			logger.Ctx(ctx).Info("compressing conversation context",
				zap.Int("messages", len(messages)),
				zap.Int("estimated_tokens", estimateTokens(messages)),
				zap.Int("context_window", a.config.ContextWindow))
			messages = compressMessages(ctx, a.provider, messages)
		}

		ch, err := a.provider.Chat(ctx, messages, a.tools)
		if err != nil {
			return fmt.Errorf("provider chat failed: %w", err)
		}

		var contentBuf string
		var thinkingBuf string
		var thinkingSignature string
		var toolCalls []ToolCall
		hasToolCall := false

		for event := range ch {
			switch event.Type {
			case "content":
				contentBuf += event.Content
				onEvent(event)
			case "thinking":
				thinkingBuf += event.Content
				onEvent(event)
			case "thinking_done":
				if event.ThinkingSignature != "" {
					thinkingSignature = event.ThinkingSignature
				}
				onEvent(event)
			case "tool_start", "tool_result", "approval_request", "approval_result":
				onEvent(event)
			case "usage":
				// 单次 Chat 调用可能在 agent 内部触发多轮 provider 调用（tool loop），
				// 每轮的 usage 都向前端转发，由前端累加到当前 assistant 消息。
				onEvent(event)
			case "tool_call":
				toolCalls = event.ToolCalls
				hasToolCall = true
				onEvent(event)
			case "error":
				onEvent(event)
				return fmt.Errorf("provider error: %s", event.Error)
			case "done":
				// 不立即转发 done，可能还有 tool 调用
			}
		}

		// 没有 tool 调用，对话结束（由 runner 发 done）
		if !hasToolCall {
			return nil
		}

		// 将 assistant 的回复（含 thinking + tool_calls）加入消息。
		// reasoning_content 仅 DeepSeek-v4 thinking 模式强制要求回传（否则多轮 400），
		// 其他 provider 保持原有行为，避免引入未知字段触发严格 OpenAI-compat 后端报错。
		assistantMsg := Message{
			Role:              RoleAssistant,
			Content:           contentBuf,
			Thinking:          thinkingBuf,
			ThinkingSignature: thinkingSignature,
			ToolCalls:         toolCalls,
		}
		if needsReasoningContent(a.provider.Model()) {
			assistantMsg.ReasoningContent = thinkingBuf
		}
		messages = append(messages, assistantMsg)

		// 并行执行工具调用
		type toolResult struct {
			content string
			callID  string
		}
		results := make([]toolResult, len(toolCalls))
		var mu sync.Mutex
		g, gCtx := errgroup.WithContext(ctx)

		for i, tc := range toolCalls {
			g.Go(func() error {
				// 通知前端工具开始执行；ToolCallID 用于前端跨 turn 还原 tool_calls 历史
				onEvent(StreamEvent{
					Type:       "tool_start",
					ToolName:   tc.Function.Name,
					ToolInput:  tc.Function.Arguments,
					ToolCallID: tc.ID,
				})

				result, execErr := executor.Execute(gCtx, tc.Function.Name, tc.Function.Arguments)
				if execErr != nil {
					result = fmt.Sprintf("Tool execution error: %s", execErr.Error())
				}
				if len(result) > maxResultLen {
					result = result[:2048] + fmt.Sprintf(
						"\n\n--- Output truncated ---\nOutput too large (%d bytes, exceeds %d byte limit). Use more precise filters, pipe through | head or | grep, or split the query.",
						len(result), maxResultLen)
				}

				// 通知前端工具执行完成
				onEvent(StreamEvent{
					Type:       "tool_result",
					ToolName:   tc.Function.Name,
					Content:    result,
					ToolCallID: tc.ID,
				})

				mu.Lock()
				results[i] = toolResult{content: result, callID: tc.ID}
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()

		for _, r := range results {
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    r.content,
				ToolCallID: r.callID,
			})
		}

		// 工具执行完毕后，检查排队的用户消息并注入。
		// queue_consumed 事件的 Content 必须是给前端展示的原始用户输入，
		// 不能携带 RenderMentionContext 注入到 LLM 的 raw 上下文。
		if getPendingMessages != nil {
			if pending := getPendingMessages(); len(pending) > 0 {
				for _, p := range pending {
					onEvent(StreamEvent{Type: "queue_consumed", Content: p.displayContent})
					messages = append(messages, p.msg)
				}
			}
		}
		// 继续下一轮对话
	}

	return fmt.Errorf("max rounds (%d) reached", maxRounds)
}

// DefaultToolExecutor 默认工具执行器，通过统一注册表调度，缓存 SSH 连接供同一次 Chat 复用
type DefaultToolExecutor struct {
	handlers     map[string]ToolHandlerFunc
	sshCache     *SSHClientCache
	sshPool      *sshpool.Pool // SSH 连接池，供 Redis/Database 隧道使用
	mongoDBCache *MongoDBClientCache
	kafkaService *kafka_svc.Service // 同一次 Chat 内复用 Kafka client，避免每次工具调用重新 dial+ping
}

func NewDefaultToolExecutor() *DefaultToolExecutor {
	handlers := make(map[string]ToolHandlerFunc)
	for _, def := range AllToolDefs() {
		handlers[def.Name] = def.Handler
	}
	pool := sshpool.NewPool(&AIPoolDialer{}, 5*time.Minute)
	return &DefaultToolExecutor{
		handlers:     handlers,
		sshCache:     NewSSHClientCache(),
		sshPool:      pool,
		mongoDBCache: NewMongoDBClientCache(),
		kafkaService: kafka_svc.New(pool),
	}
}

// Close 关闭所有缓存的连接
func (e *DefaultToolExecutor) Close() error {
	if e.kafkaService != nil {
		e.kafkaService.Close()
	}
	e.sshPool.Close()
	if err := e.mongoDBCache.Close(); err != nil {
		logger.Default().Warn("close MongoDB cache", zap.Error(err))
	}
	return e.sshCache.Close()
}

func (e *DefaultToolExecutor) Execute(ctx context.Context, name string, argsJSON string) (string, error) {
	handler, ok := e.handlers[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	// 注入 SSH 缓存，run_command 会自动使用
	ctx = WithSSHCache(ctx, e.sshCache)
	// 注入 SSH 连接池，仅在 context 中没有时设置（允许外部覆盖）
	if getSSHPool(ctx) == nil {
		ctx = WithSSHPool(ctx, e.sshPool)
	}
	// 注入 MongoDB 缓存，exec_mongo 会自动使用
	ctx = WithMongoDBCache(ctx, e.mongoDBCache)
	// 注入 Kafka service，kafka_* handler 会复用其内部 KafkaClientManager 的缓存
	ctx = WithKafkaService(ctx, e.kafkaService)
	return handler(ctx, args)
}
