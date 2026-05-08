package aiagent

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/tool/subagent"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// SystemOptions wires together the dependencies for one *coding.System.
// One System per active Conversation.
type SystemOptions struct {
	Provider provider.Provider
	Model    string // 空字符串 = backend 默认；通常应填 AIProvider.Model，否则 cago 发出去的 req.Model 为空
	Cwd      string
	ConvID   int64
	Lang     string
	Deps     *Deps
	Emitter  EventEmitter
	// CheckPerm 注入静态策略检查；nil 时走 ai.CheckPermission（生产默认）。
	// 测试可传 fake 避免拉起 DB / asset 仓库。
	CheckPerm   CheckPermissionFunc
	AuditWriter ai.AuditWriter // nil → ai.NewDefaultAuditWriter()
	Resolver    PendingResolver
	Activate    func()      // window activation; may be nil
	MaxRounds   int         // 0 → 50
	Store       agent.Store // nil → NewGormStore() (production default)
	// LocalGrants 抽象 cago built-in 工具（write / edit）会话级"始终放行"开关。
	// nil → NewRepoLocalGrantStore()（生产默认，落 local_tool_grant_repo）。
	LocalGrants LocalGrantStore
}

// System is the OpsKat-facing handle around cago's coding.System. It owns the
// per-Conversation cago objects (parent agent, sub-agents, the persistent
// Session) plus OpsKat-side state (PerTurnState, sidecar, bridge, rounds
// counter).
//
// Lifecycle: NewSystem → Stream/Steer (any number of times) → Close.
// Close is idempotent.
type System struct {
	cs        *coding.System
	sess      *agent.Session
	convID    int64
	emitter   EventEmitter
	turnState *PerTurnState
	sidecar   *sidecar
	bridge    *bridge
	rounds    *roundsCounter

	mu           sync.Mutex
	streamCancel context.CancelFunc
	closeOnce    sync.Once
	closeErr     error

	// Steer 走 cago Session.FollowUp（FIFO，下一个 safe point 才 drain），cago 通过
	// EventUserPromptSubmit 通知"这条 follow-up 已被消费"，但只携带 LLM body 文本
	// （已混入 mention 上下文），拿不到给前端展示的原始用户输入。
	// 这里旁路一个 displayContent FIFO：Steer 入队时 push，bridge 收到 follow-up 类
	// EventUserPromptSubmit 时 pop 头部，emit 给前端的 queue_consumed_batch 事件。
	displayMu       sync.Mutex
	pendingDisplays []string

	// pendingMu 保护 pendingMentions / pendingUsage：由 SendAIMessage 入口写入
	// （mentions）与 event_bridge.EventUsage 写入（usage），由 gormStore.Save 在
	// cago internalObserver.persist 触发时 drain。这两个缓存的生命周期被
	// "Stream 入口 stash → cago 触发 persist → gormStore.Save drain" 这一对
	// 严格夹紧；读写都在 main goroutine（cago observer 是同步调用）。
	pendingMu       sync.Mutex
	pendingMentions []ai.MentionedAsset
	pendingUsage    map[string]*conversation_entity.TokenUsage // cago_id → usage
}

// NewSystem assembles the per-Conversation cago System.
//
// Hooks installed (matchers in parens):
//   - PreToolUse  ("")                 → policy hook (per-tool gating)
//   - PreToolUse  ("")                 → rounds hook (cap to MaxRounds)
//   - PostToolUse ("")                 → audit hook (drain sidecar → AuditWriter)
//   - PostToolUse ("dispatch_subagent")→ agent_end hook (emit truncated summary)
//   - UserPromptSubmit                 → prompt hook (open tabs + mentions + ext skills)
func NewSystem(ctx context.Context, opts SystemOptions) (*System, error) {
	if opts.Deps == nil {
		opts.Deps = &Deps{}
	}
	if opts.AuditWriter == nil {
		opts.AuditWriter = ai.NewDefaultAuditWriter()
	}
	if opts.MaxRounds == 0 {
		opts.MaxRounds = 50
	}
	if opts.LocalGrants == nil {
		opts.LocalGrants = NewRepoLocalGrantStore()
	}

	gw := NewApprovalGateway(opts.Emitter, opts.Resolver)
	gw.SetActivateFunc(opts.Activate)

	sc := newSidecar()
	turnState := &PerTurnState{}

	policyHook := makePolicyHook(sc, gw, opts.CheckPerm, opts.LocalGrants)
	auditHook := makeAuditHook(sc, opts.AuditWriter)
	rounds := newRoundsCounter(opts.MaxRounds)
	promptHook := makePromptHook(turnState)
	agentEndHook := MakeAgentEndHook(opts.Emitter)

	// OpsTools 包含全部 OpsKat 工具（exec_tool / run_command / batch_command / 等）。
	// 子 agent dispatch 通过 cago 的 dispatch_subagent + 下面的 subEntries 实现。
	parentTools := OpsTools(opts.Deps)
	subEntries := []subagent.Entry{
		OpsExplorerEntry(opts.Provider, opts.Deps, opts.Cwd, opts.Model),
		OpsBatchEntry(opts.Provider, opts.Deps, opts.Cwd, opts.Model),
		OpsReadOnlyEntry(opts.Provider, opts.Deps, opts.Cwd, opts.Model),
	}

	// Confine skill discovery to the conversation's cwd; do NOT scan ~/.claude
	// (those are the user's personal skills, not project-scoped).
	skillsDir := filepath.Join(opts.Cwd, ".claude", "skills")

	codingOpts := []coding.Option{
		coding.AppendSystem(StaticSystemPrompt(opts.Lang)),
		coding.WithExtraTools(parentTools...),
		coding.WithExtraSubagents(subEntries...),
		coding.WithSkillDirs(skillsDir),
		coding.WithCompactionThreshold(80000), // matches legacy heuristic
		coding.WithAgentOpts(
			agent.SessionStart(rounds.ResetHook()),
			// policyHook 阻塞等用户审批，可能远超 cago 默认 5min 单 hook 超时；用 Hooks
			// 直接传 Timeout=-1 关掉超时，让取消只受 stream ctx（用户 Stop / 关 app）控制。
			// 否则 5min 一到 ctx.Done 触发，gw.RequestSingle 静默 deny，UI 上甚至看不到
			// 审批卡（approval_request 紧跟着 approval_result deny，前端把 block 转成
			// error 立刻收起），最后模型直接收到「用户拒绝」结果。
			agent.Hooks(agent.Hook{
				Stage: agent.StagePreToolUse, Matcher: "", Fn: policyHook, Timeout: -1,
			}),
			agent.PreToolUse("", rounds.Hook()),
			agent.PostToolUse("", auditHook),
			agent.PostToolUse("dispatch_subagent", agentEndHook),
			agent.UserPromptSubmit(promptHook),
		),
	}
	if opts.Model != "" {
		codingOpts = append(codingOpts, coding.WithModel(opts.Model))
	}

	cs, err := coding.New(ctx, opts.Provider, opts.Cwd, codingOpts...)
	if err != nil {
		return nil, fmt.Errorf("aiagent.NewSystem: coding.New: %w", err)
	}

	// 先构造 sys（不带 sess），因为 store 需要 sys 实现 pendingMentionsProvider /
	// pendingUsageProvider，而 sess 需要 store。这样可以把 sys 作为两个 provider
	// 注入 NewGormStore，保持单一所有者。
	sys := &System{
		cs:        cs,
		convID:    opts.ConvID,
		emitter:   opts.Emitter,
		turnState: turnState,
		sidecar:   sc,
		rounds:    rounds,
	}

	store := opts.Store
	if store == nil {
		store = NewGormStore(sys, sys)
	}

	// 从 store 恢复历史：cago 的 Agent.Session() 不会自动 Load，必须显式
	// WithInitialHistory/State 才能让重建的 Session 看见之前的轮次。否则 app
	// 重启或 *aiagent.System 被 evict（resetAIAgentSystems / DeleteConversation
	// 重新创建）后，新 Session 历史为空，LLM 完全失忆。
	// Load 失败按 warn 处理，让用户至少能继续聊新一轮，而不是直接报错卡住。
	sessionID := fmt.Sprintf("conv_%d", opts.ConvID)
	prior, loadErr := store.Load(ctx, sessionID)
	if loadErr != nil {
		logger.Default().Warn("aiagent.NewSystem: store.Load failed; starting with empty history",
			zap.Int64("conv_id", opts.ConvID), zap.Error(loadErr))
		prior = agent.SessionData{}
	}

	sess := cs.Agent().Session(
		agent.WithStore(store),
		agent.WithID(sessionID),
		agent.WithInitialHistory(prior.Messages),
		agent.WithInitialState(prior.State),
	)
	sys.sess = sess

	// bridge 持回调拿 follow-up 展示原文：避免循环依赖（bridge 不引用 *System
	// 全部 API），且测试时可注入 fake popDisplay/usageStasher 单独覆盖 bridge
	// 行为。usage 这一项让 bridge 能在 EventUsage 时把 token usage stash 进
	// System pending 缓存。
	sys.bridge = newBridge(opts.Emitter, sys.popPendingDisplay, sys)
	return sys, nil
}

// Stream sends a prompt for the next turn, draining cago events through the
// bridge to Wails and applying the retry policy. Updates per-turn state
// (open tabs, mentions, ext skills) before opening the stream. The per-turn
// rounds budget is reset by the SessionStart hook installed in NewSystem.
//
// Returns when the stream completes or all retry attempts fail.
func (s *System) Stream(ctx context.Context, prompt string, aiCtx ai.AIContext, ext map[string]string) error {
	// 把当前轮的 mentions stash 进 System pending 缓存；下一次 cago Save 触发时
	// gormStore drain 出来绑到刚 upsert 的 user 行的 mentions 列。
	s.stashPendingMentions(aiCtx)
	s.turnState.Set(aiCtx, ext)
	// keyConvID 只给 aiagent 内部 hook 用；ai.WithConversationID / WithSessionID 给老路径
	// （batch_command + tool handler in-handler check + audit）兜底，不然它们 Get 出来全是 0/""，
	// 会回退到 App.currentConversationID 或 SaveGrantPattern 静默 no-op。grant session ID
	// 与 saveGrantPatternsFromResponse 内构造的 conv_<convID> 必须保持一致。
	ctx = WithConvID(ctx, s.convID)
	ctx = ai.WithConversationID(ctx, s.convID)
	ctx = ai.WithSessionID(ctx, fmt.Sprintf("conv_%d", s.convID))

	streamCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.streamCancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streamCancel = nil
		s.mu.Unlock()
		cancel()
	}()

	_, err := RunWithRetry(streamCtx, s.sess, prompt, s.emitter, s.convID,
		func(stream *agent.Stream) {
			for stream.Next() {
				s.bridge.translate(s.convID, stream.Event())
			}
		},
	)
	return err
}

// Steer 把用户消息入 cago 的 FollowUp 队列，等到当前轮 tool 批执行完、下一轮 LLM
// 调用前由 cago drainInjections 一次性 drain 出来。不打断当前 token stream，多条
// 排队消息会在前端 pendingQueue 里按 FIFO 累积，符合老 ConversationRunner 体验。
//
// displayContent 是前端要展示的原文（mention 未展开版本），cago 拿到的 body 可能已
// 经被 RenderMentionContext 拼接过；这里把 displayContent 推到 System 的 FIFO，由
// bridge 在收到对应 EventUserPromptSubmit (Kind=MessageKindFollowUp) 时 pop 出来
// 通过 queue_consumed_batch 通知前端。
//
// 函数名沿用旧的 Steer 是为了不破坏 App.QueueAIMessage 调用点；语义已经从 cago 的
// Steer（mid-cycle 立即注入）切换到 cago 的 FollowUp（safe point drain）。
func (s *System) Steer(ctx context.Context, body, displayContent string) error {
	s.pushPendingDisplay(displayContent)
	if err := s.sess.FollowUp(ctx, body, agent.AsUser(), agent.Persist(true)); err != nil {
		// FollowUp 入队失败时，回滚 displayContent FIFO，避免 bridge 后续 pop 出
		// 一条永远不会被 cago 消费的鬼影记录。
		s.popMatchingDisplay(displayContent)
		return err
	}
	return nil
}

// pushPendingDisplay 把一条排队消息的展示原文塞到 System 的 displayContent FIFO 末尾。
// bridge 在收到 cago follow-up 类 EventUserPromptSubmit 时 popPendingDisplay 头部。
func (s *System) pushPendingDisplay(content string) {
	s.displayMu.Lock()
	s.pendingDisplays = append(s.pendingDisplays, content)
	s.displayMu.Unlock()
}

// popPendingDisplay 弹出 FIFO 头部，没有时返回空串。bridge 在 EventUserPromptSubmit
// (Kind=FollowUp) 到来时调用一次。
func (s *System) popPendingDisplay() string {
	s.displayMu.Lock()
	defer s.displayMu.Unlock()
	if len(s.pendingDisplays) == 0 {
		return ""
	}
	out := s.pendingDisplays[0]
	s.pendingDisplays = s.pendingDisplays[1:]
	return out
}

// popMatchingDisplay 在 FollowUp 入队失败时回滚最后一次 push（必须是尾部对应项；
// 用 content 校验防止误删并发情况下别人 push 的项）。匹配不上就什么都不做。
func (s *System) popMatchingDisplay(content string) {
	s.displayMu.Lock()
	defer s.displayMu.Unlock()
	n := len(s.pendingDisplays)
	if n == 0 {
		return
	}
	if s.pendingDisplays[n-1] != content {
		return
	}
	s.pendingDisplays = s.pendingDisplays[:n-1]
}

// stashPendingMentions 把当前轮的 mentions 暂存；下一次 cago Save 触发时 gormStore
// 把它绑到刚出现的 user 行的 mentions 列。覆盖式语义：每次 Stream 入口 stash 一次。
func (s *System) stashPendingMentions(aiCtx ai.AIContext) {
	if len(aiCtx.MentionedAssets) == 0 {
		s.pendingMu.Lock()
		s.pendingMentions = nil
		s.pendingMu.Unlock()
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	s.pendingMentions = append(s.pendingMentions[:0], aiCtx.MentionedAssets...)
}

// popPendingMentions drain 出当前缓存（move 语义），由 gormStore.Save 调用。
func (s *System) popPendingMentions() []ai.MentionedAsset {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := s.pendingMentions
	s.pendingMentions = nil
	return out
}

// stashPendingUsage 把一条 EventUsage 的 token usage 关联到对应 cago_id 缓存起来；
// 下一次 cago Save 触发时由 gormStore.Save drain 到 conversation_messages.token_usage。
// 同 cago_id 多次 stash 取最新值（应当不会发生，但此语义安全）。
func (s *System) stashPendingUsage(cagoID string, u *conversation_entity.TokenUsage) {
	if cagoID == "" || u == nil {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pendingUsage == nil {
		s.pendingUsage = make(map[string]*conversation_entity.TokenUsage)
	}
	s.pendingUsage[cagoID] = u
}

// drainPendingUsage 拿走全部缓存（move 语义），由 gormStore.Save 调用。
func (s *System) drainPendingUsage() map[string]*conversation_entity.TokenUsage {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := s.pendingUsage
	s.pendingUsage = nil
	return out
}

// StopStream cancels the in-flight stream (if any). Safe to call from another
// goroutine while Stream is running.
func (s *System) StopStream() {
	s.mu.Lock()
	cancel := s.streamCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ErrUnknownSlashCommand is returned by RunSlash when the user types a
// /command that isn't registered. Re-exported from cago so callers can
// errors.Is against it without importing app/coding.
var ErrUnknownSlashCommand = coding.ErrUnknownSlashCommand

// SlashResult is the outcome of resolving a /slash line.
//
//   - For a non-slash line: IsSlash is false; both Prompt and Notice are empty.
//     Caller should fall through to the legacy send path.
//   - For an expanded template: Prompt holds the expanded text; caller should
//     feed it back through System.Stream as the next prompt.
//   - For a builtin: Prompt holds output.UserText (often empty), Notice holds
//     output.Notice (often non-empty). Frontend should render Notice as a
//     synthesized system message.
type SlashResult struct {
	IsSlash bool
	Prompt  string
	Notice  string
}

// RunSlash resolves a chat-input line through the slash registry. See
// SlashResult for the return semantics. Returns ErrUnknownSlashCommand on
// unrecognized /commands.
func (s *System) RunSlash(ctx context.Context, line string) (SlashResult, error) {
	reg := s.cs.SlashRegistry()
	if reg == nil {
		// Slash disabled at System construction — pass through as a normal line.
		return SlashResult{IsSlash: false}, nil
	}
	res, err := reg.Resolve(line)
	if err != nil {
		return SlashResult{}, err
	}
	if !res.IsSlash {
		return SlashResult{IsSlash: false}, nil
	}
	if res.IsBuiltin {
		out, runErr := res.Run(ctx, s.sess)
		if runErr != nil {
			return SlashResult{}, runErr
		}
		return SlashResult{
			IsSlash: true,
			Prompt:  out.UserText,
			Notice:  out.Notice,
		}, nil
	}
	// Template path: caller will feed res.Literal back into Stream.
	return SlashResult{
		IsSlash: true,
		Prompt:  res.Literal,
	}, nil
}

// Close releases sub-agents, the parent agent, the session store handle, and
// per-Conversation Deps (idempotent). Subsequent calls return the same error.
func (s *System) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.StopStream()
		var errs []error
		if s.sess != nil {
			if err := s.sess.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("session close: %w", err))
			}
		}
		if s.cs != nil {
			if err := s.cs.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("coding system close: %w", err))
			}
		}
		// Deps is owned by the App; not closed here.
		if len(errs) > 0 {
			s.closeErr = errs[0]
			for _, e := range errs[1:] {
				s.closeErr = fmt.Errorf("%w; %w", s.closeErr, e)
			}
		}
	})
	return s.closeErr
}
