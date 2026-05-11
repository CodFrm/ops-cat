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
	Model         string
	System        string
	Tools         []agent.Tool
	Deps          *Deps // per-Manager 共用：SSH/Mongo/Kafka 缓存。Manager.Close 时关闭。
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
	m := &Manager{
		opts:    opts,
		handles: make(map[int64]*ConvHandle),
	}
	m.store = NewGormStore()
	m.recorder = agentstore.NewRecorder(m.store)
	return m
}

// CloseHandle removes and shuts down the handle for convID. No-op if not
// cached. Used by app_ai when a conversation is deleted.
func (m *Manager) CloseHandle(convID int64) error {
	m.mu.Lock()
	h, ok := m.handles[convID]
	delete(m.handles, convID)
	m.mu.Unlock()
	if ok {
		return h.Close()
	}
	return nil
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
	stored, branch, err := m.store.LoadConversation(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("aiagent: load conv %d: %w", convID, err)
	}
	// cago Store 用 on-wire StoredMessage 形态（type discriminator 嵌 data），
	// agent.LoadConversation 需要 in-memory typed Message —— 走 store.DecodeMessages
	// 还原典型形态。失败说明 DB 行里有不认识的 block 类型（version skew），直接 bail
	// 比塞个空 conv 让用户看到一片空白要好。
	msgs, err := agentstore.DecodeMessages(stored)
	if err != nil {
		return nil, fmt.Errorf("aiagent: decode conv %d messages: %w", convID, err)
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

	h := NewConvHandle(convID, conv, r)
	// 让 Send/Edit 能在写 fresh user-append 前 prime bridge 跳过它，避免
	// 重复 user 气泡 + 空 asst 占位（详见 bridge.skipNextUser 注释）。
	h.AttachBridge(bridge)
	// request_permission 工具走 GrantApprover → 复用 per-conv gateway 的
	// emitter/resolver 通道（弹审批卡 + 持久化 grants）。
	h.AttachGateway(gw)
	// LIFO order: teardown runs in reverse — bridgeCancel first (stop emit
	// translation), then unbind (release the recorder watch).
	// gateway 没有后台 goroutine 了：RequestSingle 是同步调用，ctx 由 hook 的
	// turn ctx 控制，turn 取消时它自己 select<-ctx.Done() 返回 deny。
	h.AddTeardown(func() { unbind() })
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

// Close shuts down all per-conv handles and releases Deps. Idempotent.
func (m *Manager) Close() error {
	m.mu.Lock()
	handles := m.handles
	m.handles = nil
	m.mu.Unlock()
	for _, h := range handles {
		_ = h.Close()
	}
	if m.opts.Deps != nil {
		_ = m.opts.Deps.Close()
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
	// decisionStore 是 per-Handle 的 policy → audit 旁路：PreHook 写决策，
	// PostHook Pop 给 AuditWriter。ToolUseID 是 turn-uniq 的，不必跨 handle 共享。
	decisionStore := newToolDecisionStore()
	opts := []agent.Option{
		agent.Model(m.opts.Model),
		agent.System(m.opts.System),
		agent.Tools(m.opts.Tools...),
		agent.UserPromptSubmit(newMentionsHook(m.opts.Mention, m.opts.TabOpener)),
		// policy hook 内部直接调用 gw.RequestSingle 处理 NeedConfirm 分支：
		// PreToolUse 链路上只有这一处审批入口，避免重复弹卡。
		agent.PreToolUse("", newPolicyHook(m.opts.PolicyChecker, gw, decisionStore)),
		agent.PreToolUse("", rc.Hook()),
		agent.PostToolUse("", newAuditHook(m.opts.AuditWriter, decisionStore)),
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
