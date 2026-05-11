package aiagent

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"
	"github.com/cago-frame/agents/provider"
)

// ManagerOptions bundles the dependencies a Manager needs to construct
// per-conversation Agents. Most fields are required; sensible defaults are
// applied for MaxRounds.
type ManagerOptions struct {
	Provider      provider.Provider
	System        string
	Tools         []agent.Tool
	MaxRounds     int
	RetryPolicy   agent.RetryPolicy
	Emitter       EventEmitter
	Resolver      PendingResolver
	LocalGrants   LocalGrantStore
	AuditWriter   AuditWriter
	PolicyChecker PolicyChecker
	Mention       MentionResolver
	TabOpener     TabOpener
}

// Manager is the desktop-app-wide coordinator. It owns the gormStore and
// holds shared adapters, but delegates per-conversation work to a fresh
// *agent.Agent constructed inside each Handle().
//
// Lifecycle: build via NewManager (no I/O), call Handle(ctx, convID) to get
// or create a ConvHandle, call Close() at app shutdown.
type Manager struct {
	opts     ManagerOptions
	store    agentstore.Store
	recorder *agentstore.Recorder

	mu      sync.Mutex
	handles map[int64]*ConvHandle
}

// NewManager constructs a Manager. Does no I/O; the gormStore lazy-binds to
// the global db.Default() at first use via the registered conversation_repo.
func NewManager(opts ManagerOptions) *Manager {
	if opts.MaxRounds == 0 {
		opts.MaxRounds = 50
	}
	s := NewGormStore()
	return &Manager{
		opts:     opts,
		store:    s,
		recorder: agentstore.NewRecorder(s),
		handles:  make(map[int64]*ConvHandle),
	}
}

// Handle returns the ConvHandle for convID, creating it (and loading the conv
// from the store) on first call. The returned handle is cached for subsequent
// calls and shut down when m.Close() runs.
func (m *Manager) Handle(ctx context.Context, convID int64) (*ConvHandle, error) {
	m.mu.Lock()
	if h, ok := m.handles[convID]; ok {
		m.mu.Unlock()
		return h, nil
	}
	m.mu.Unlock()

	sid := strconv.FormatInt(convID, 10)
	msgs, branch, err := m.store.LoadConversation(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("aiagent: load conv %d: %w", convID, err)
	}

	convOpts := []agent.ConvOption{agent.WithConvID(sid)}
	if branch.ParentConvID != "" {
		convOpts = append(convOpts, agent.WithBranchedFrom(agent.BranchInfo{
			ParentConvID: branch.ParentConvID,
			ParentIndex:  branch.ParentIndex,
		}))
	}
	conv := agent.LoadConversation(sid, msgs, convOpts...)

	// Per-conv components.
	gw := NewApprovalGateway(convID, m.opts.Emitter, m.opts.LocalGrants, m.opts.Resolver)
	rc := newRoundsCounter(m.opts.MaxRounds)

	// Build Agent for this conv. All hooks are closures over per-conv state.
	a := agent.New(m.opts.Provider, m.buildAgentOptions(gw, rc)...)
	r := a.Runner(conv)

	// Bridge: Runner.OnEvent + Conv.Watch.
	bridge := newBridge(convID, m.opts.Emitter)
	r.OnEvent(agent.AnyEvent, bridge.OnRunnerEvent)

	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	go func() {
		for ch := range conv.Watch() {
			bridge.OnConvChange(bridgeCtx, ch)
		}
	}()

	// Recorder: subscribes Conv.Watch → Store writes. Returns unsubscribe.
	unbind := m.recorder.Bind(conv)

	// Approval gateway Run goroutine.
	gwCtx, gwCancel := context.WithCancel(context.Background())
	go gw.Run(gwCtx)

	h := NewConvHandle(convID, conv, r)
	// LIFO order: teardown runs in reverse — bridgeCancel first (stop emit
	// translation), then gateway (deny in-flight pending), then unbind
	// (release the recorder watch).
	h.AddTeardown(func() { unbind() })
	h.AddTeardown(func() { gwCancel(); _ = gw.Close() })
	h.AddTeardown(func() { bridgeCancel() })

	m.mu.Lock()
	// Race guard: another goroutine may have created a handle between our
	// initial lock release and now. If so, tear down our work and return theirs.
	if existing, ok := m.handles[convID]; ok {
		m.mu.Unlock()
		_ = h.Close()
		return existing, nil
	}
	m.handles[convID] = h
	m.mu.Unlock()
	return h, nil
}

// Close shuts down all per-conv handles. Idempotent.
func (m *Manager) Close() error {
	m.mu.Lock()
	handles := m.handles
	m.handles = nil
	m.mu.Unlock()
	for _, h := range handles {
		_ = h.Close()
	}
	return nil
}

// buildAgentOptions assembles the cago Agent options. Called once per Handle()
// because the approval/rounds hooks are per-conv (gw and rc are per-conv).
//
// The PreToolUse matcher is empty string (= match all tools). Per-tool
// filtering happens inside the policy / approval hooks themselves; trying to
// pre-filter at the matcher level would force us to maintain a separate
// allow-list that would drift from PolicyChecker. Tiny per-call overhead is
// acceptable for ~10 active convs.
func (m *Manager) buildAgentOptions(gw *ApprovalGateway, rc *roundsCounter) []agent.Option {
	opts := []agent.Option{
		agent.System(m.opts.System),
		agent.Tools(m.opts.Tools...),
		agent.UserPromptSubmit(newMentionsHook(m.opts.Mention, m.opts.TabOpener)),
		agent.PreToolUse("", newPolicyHook(m.opts.PolicyChecker)),
		agent.PreToolUse("", rc.Hook()),
		agent.PreToolUse("", gw.Hook()),
		agent.PostToolUse("", newAuditHook(m.opts.AuditWriter)),
		agent.PostToolUse("dispatch_subagent", newSubagentDispatchHook(2000)),
		agent.OnRunnerStart(func(_ context.Context, _ *agent.Runner) error {
			rc.Reset()
			return nil
		}),
	}
	// Retry only if a policy is configured.
	if m.opts.RetryPolicy.MaxAttempts > 1 {
		opts = append(opts, agent.Retry(m.opts.RetryPolicy))
	}
	return opts
}
