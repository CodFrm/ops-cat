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

	gw := NewApprovalGateway(opts.Emitter, opts.Resolver)
	gw.SetActivateFunc(opts.Activate)

	sc := newSidecar()
	turnState := &PerTurnState{}

	policyHook := makePolicyHook(sc, gw, opts.CheckPerm)
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

	store := opts.Store
	if store == nil {
		store = NewGormStore()
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

	return &System{
		cs:        cs,
		sess:      sess,
		convID:    opts.ConvID,
		emitter:   opts.Emitter,
		turnState: turnState,
		sidecar:   sc,
		bridge:    newBridge(opts.Emitter),
		rounds:    rounds,
	}, nil
}

// Stream sends a prompt for the next turn, draining cago events through the
// bridge to Wails and applying the retry policy. Updates per-turn state
// (open tabs, mentions, ext skills) before opening the stream. The per-turn
// rounds budget is reset by the SessionStart hook installed in NewSystem.
//
// Returns when the stream completes or all retry attempts fail.
func (s *System) Stream(ctx context.Context, prompt string, aiCtx ai.AIContext, ext map[string]string) error {
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

// Steer injects a mid-cycle user message. Mirrors legacy QueueAIMessage by
// emitting "queue_consumed" so the frontend can clear its pending banner.
// displayContent is what the UI showed the user (may differ from body when
// mentions are expanded server-side).
func (s *System) Steer(ctx context.Context, body, displayContent string) error {
	s.emitter.Emit(s.convID, ai.StreamEvent{Type: "queue_consumed", Content: displayContent})
	return s.sess.Steer(ctx, body, agent.AsUser(), agent.Persist(true))
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
