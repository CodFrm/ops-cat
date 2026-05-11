package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	cagoProvider "github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/tool"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
)

// RunnerState 表示 Runner 的当前状态。
//
// 从老 conversation_runner.go 搬过来（那个文件在 M6 cutover 时删除）；
// 状态机本身没变，CagoRunner 复用 4 个值。
type RunnerState int

const (
	RunnerIdle RunnerState = iota
	RunnerRunning
	RunnerRetrying
	RunnerStopping
)

// CagoRunner 是基于 cago-frame/agents 的 ConversationRunner 替代。
//
// 与 ConversationRunner 的差异：
//   - 不再依赖自研 *Agent / *Provider / *ToolExecutor；
//   - 每次 Start 走 BuildCagoProvider → coding.New → agent.Runner.Send 一条路径；
//   - 重试由 cago 内部处理（EventRetry 透传给前端），外层不再做指数回退包装；
//   - QueueMessage 走 cago.Runner.Steer，把侧通道消息塞进当前 turn。
//
// 公共 API 形状与 ConversationRunner 对齐（Start/Stop/QueueMessage/State）以方便 M6 切换；
// Start 额外接收 systemPrompt 参数（per-call 动态提示词），不可放进构造器。
type CagoRunner struct {
	cfg CagoRunnerConfig

	mu        sync.Mutex
	state     RunnerState
	cancel    context.CancelFunc
	done      chan struct{}
	activeRun *agent.Runner // 当前 active runner，Steer 用；非 active 时为 nil
}

// CagoRunnerConfig 创建 CagoRunner 所需的依赖项。
//
//   - Provider：可选；测试场景用 providertest.Mock 直接注入。非 nil 时优先使用。
//   - ProviderEntity / APIKey：生产路径——传给 BuildCagoProvider 构造 cago provider；
//   - Cwd：coding.New 必填参数（read/write/bash 等工具的工作目录），通常为 ~/.opskat；
//   - PolicyChecker：注入到 ctx，handler 内部的 GetPolicyChecker(ctx) 仍是审批入口；
//   - Tools：通常是 CagoTools()，附加到 coding 默认工具集之后。
type CagoRunnerConfig struct {
	Provider       cagoProvider.Provider
	ProviderEntity *ai_provider_entity.AIProvider
	APIKey         string
	Cwd            string
	PolicyChecker  *CommandPolicyChecker
	Tools          []tool.Tool
}

// NewCagoRunner 创建一个新的 CagoRunner。
func NewCagoRunner(cfg CagoRunnerConfig) *CagoRunner {
	return &CagoRunner{
		cfg:   cfg,
		state: RunnerIdle,
	}
}

// State 返回当前状态。
func (r *CagoRunner) State() RunnerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Start 启动一次对话。
// systemPrompt 是 PromptBuilder 当下构建好的动态提示词，会通过 coding.AppendSystem 注入。
// messages 是 OpsKat 风格的完整历史 + 末尾新 user 消息。
func (r *CagoRunner) Start(ctx context.Context, systemPrompt string, messages []Message, onEvent func(StreamEvent)) error {
	r.mu.Lock()
	if r.state != RunnerIdle {
		r.mu.Unlock()
		return errors.New("runner is already active")
	}
	r.state = RunnerRunning
	chatCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.mu.Unlock()

	go r.run(chatCtx, systemPrompt, messages, onEvent)
	return nil
}

// Stop 取消当前生成并等待 goroutine 退出。
func (r *CagoRunner) Stop() {
	r.mu.Lock()
	if r.state == RunnerIdle {
		r.mu.Unlock()
		return
	}
	r.state = RunnerStopping
	if r.cancel != nil {
		r.cancel()
	}
	done := r.done
	r.mu.Unlock()

	if done != nil {
		<-done
	}
}

// QueueMessage 向正在运行的 turn 注入侧通道消息（cago Steer）。
// 如果当前没有 active turn，静默丢弃——与旧 ConversationRunner 的 pendingMsgs 清空策略对齐：
// 此时前端会通过 drainQueue 拿回未消费消息，由 SendAIMessage 重新发起。
func (r *CagoRunner) QueueMessage(displayContent string, msg Message) {
	r.mu.Lock()
	active := r.activeRun
	r.mu.Unlock()
	if active == nil {
		return
	}
	if displayContent == "" {
		displayContent = msg.Content
	}
	err := active.Steer(context.Background(), msg.Content, agent.WithSteerDisplay(displayContent))
	if err != nil && !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		logger.Default().Warn("cago Steer failed", zap.Error(err))
	}
}

// run 在 goroutine 内做实际工作。
func (r *CagoRunner) run(ctx context.Context, systemPrompt string, messages []Message, onEvent func(StreamEvent)) {
	defer func() {
		r.mu.Lock()
		r.state = RunnerIdle
		r.cancel = nil
		r.activeRun = nil
		r.mu.Unlock()
		close(r.done)
	}()

	// 1. policy checker 入 ctx，让 handler 内部审批仍走旧路径。
	if r.cfg.PolicyChecker != nil {
		ctx = WithPolicyChecker(ctx, r.cfg.PolicyChecker)
	}

	// 2. provider：测试可注入；否则走 BuildCagoProvider
	prov := r.cfg.Provider
	if prov == nil {
		built, err := BuildCagoProvider(r.cfg.ProviderEntity, r.cfg.APIKey)
		if err != nil {
			onEvent(StreamEvent{Type: "error", Error: fmt.Sprintf("build provider: %s", err.Error())})
			return
		}
		prov = built
	}

	// 3. coding.System（attach OpsKat 工具 + 动态 system prompt，关掉 ~/.claude 自动加载）
	model := ""
	if r.cfg.ProviderEntity != nil {
		model = r.cfg.ProviderEntity.Model
	}
	sys, err := buildCagoSystem(ctx, prov, r.cfg.Cwd, systemPrompt, model, r.cfg.Tools)
	if err != nil {
		onEvent(StreamEvent{Type: "error", Error: fmt.Sprintf("build coding system: %s", err.Error())})
		return
	}
	defer func() {
		if cerr := sys.Close(context.Background()); cerr != nil {
			logger.Default().Warn("close coding system", zap.Error(cerr))
		}
	}()

	// 4. 回放历史 + 切走 lastUserText
	history, lastUserText := splitForReplay(messages)
	cagoHistory := convertToCagoMessages(history)
	convID := fmt.Sprintf("opskat-conv-%d", GetConversationID(ctx))
	conv := agent.LoadConversation(convID, cagoHistory)

	// 5. parent agent runner
	parentAgent := sys.Agent()
	cagoRun := parentAgent.Runner(conv)
	defer func() { _ = cagoRun.Close() }()
	r.mu.Lock()
	r.activeRun = cagoRun
	r.mu.Unlock()

	// 6. Send 并把事件流翻译给前端
	events, err := cagoRun.Send(ctx, lastUserText)
	if err != nil {
		if ctx.Err() != nil {
			onEvent(StreamEvent{Type: "stopped"})
			return
		}
		onEvent(StreamEvent{Type: "error", Error: err.Error()})
		return
	}

	translator := NewEventTranslator()
	for ev := range events {
		translator.Translate(ev, onEvent)
	}
}

// buildCagoSystem 拼装 coding.System 构造 opts，复用 OpsKat 工具 + 提示词。
// 单独拆出来便于测试时单独验证 opts 组装。
func buildCagoSystem(ctx context.Context, prov cagoProvider.Provider, cwd, systemPrompt, model string, extraTools []tool.Tool) (*coding.System, error) {
	opts := []coding.Option{
		coding.WithoutContextFiles(),  // 不读 ~/.claude/CLAUDE.md 与仓库 CLAUDE.md 链
		coding.WithoutSkills(),        // 不扫 ~/.claude/skills
		coding.WithoutSlashCommands(), // 不挂 /compact /help（OpsKat 前端有自己的 UI）
	}
	if systemPrompt != "" {
		opts = append(opts, coding.AppendSystem(systemPrompt))
	}
	if model != "" {
		opts = append(opts, coding.WithModel(model))
	}
	if len(extraTools) > 0 {
		opts = append(opts, coding.WithExtraTools(extraTools...))
	}
	return coding.New(ctx, prov, cwd, opts...)
}
