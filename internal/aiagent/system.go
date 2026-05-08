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

	"github.com/opskat/opskat/internal/ai"
)

// SystemOptions wires together the dependencies for one *coding.System.
// One System per active Conversation.
type SystemOptions struct {
	Provider      provider.Provider
	Model         string // 空字符串 = backend 默认；通常应填 AIProvider.Model，否则 cago 发出去的 req.Model 为空
	Cwd           string
	ConvID        int64
	Lang          string
	Deps          *Deps
	Emitter       EventEmitter
	PolicyChecker PolicyChecker
	AuditWriter   ai.AuditWriter // nil → ai.NewDefaultAuditWriter()
	Resolver      PendingResolver
	Activate      func() // window activation; may be nil
	MaxRounds     int    // 0 → 50
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

	policyHook := makePolicyHook(opts.Deps, sc, gw, opts.PolicyChecker)
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
			agent.PreToolUse("", policyHook),
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

	sess := cs.Agent().Session(
		agent.WithStore(NewGormStore()),
		agent.WithID(fmt.Sprintf("conv_%d", opts.ConvID)),
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
	ctx = WithConvID(ctx, s.convID)

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
