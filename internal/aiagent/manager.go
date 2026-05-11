package aiagent

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"
	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/agents/provider"
)

// ManagerOptions bundles the dependencies a Manager needs to construct
// per-conversation Agents. Most fields are required; sensible defaults are
// applied for MaxRounds.
type ManagerOptions struct {
	Provider provider.Provider
	Model    string
	// System 通过 coding.AppendSystem 拼到 cago 默认 system prompt 之后；
	// cago 那一段已含工具列表 / cwd / date / project-context / skills，
	// 这里只补 OpsKat 场景特有的指引。
	System string
	// Tools 是 OpsKat 自家 ops 工具（list_assets / run_command 等）。通过
	// coding.WithExtraTools 追加在 cago 默认 10 件套（read/write/edit/bash/
	// bash_output/kill_shell/grep/find/ls/todo_write）+ dispatch_subagent 之后。
	Tools []agent.Tool
	// CwdResolver 按 convID 解析 coding session 的工作目录（per-conversation）。
	// 调用方实现（通常读 conversation_entity.WorkDir）；返回空串或 error 时
	// Handle 退回到 DefaultCwd。read/write/edit/bash/grep/find/ls 都相对于它。
	CwdResolver func(ctx context.Context, convID int64) (string, error)
	// DefaultCwd 是 CwdResolver 拿不到值时的兜底（通常 ~/.opskat）。
	DefaultCwd    string
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

	// 解析 per-conv 工作目录。CwdResolver 失败或返回空时退回 DefaultCwd ——
	// coding.New cwd 留空会按 os.Getwd 兜底，但桌面应用 Wails 启动目录不确定，
	// 显式给个 ~/.opskat 更可控。
	cwd := m.opts.DefaultCwd
	if m.opts.CwdResolver != nil {
		if resolved, err := m.opts.CwdResolver(ctx, convID); err != nil {
			// 解析失败不阻断 conv —— 退回默认目录即可，conv 自己仍能跑工具。
			// 这种 case 通常意味着 conv 实体读取出错，调用方应通过自己的 logger 报。
			_ = err
		} else if resolved != "" {
			cwd = resolved
		}
	}

	// Build Agent for this conv via cago coding.New —— 它会自动挂 10 件套
	// (read/write/edit/bash/bash_output/kill_shell/grep/find/ls/todo_write) +
	// dispatch_subagent + explore/plan/general-purpose 子 agent，并自动加载
	// ~/.claude 链上的 CLAUDE.md / skills / slash commands。OpsKat 自家的 ops
	// 工具通过 WithExtraTools 追加；per-conv hook 通过 WithAgentOpts 注入。
	sys, err := coding.New(ctx, m.opts.Provider, cwd, m.buildCodingOptions(gw, rc)...)
	if err != nil {
		return nil, fmt.Errorf("aiagent: build coding system for conv %d: %w", convID, err)
	}
	a := sys.Agent()
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
	// coding.System 只持有 parent bash JobManager —— Close 时 StopAll 后台进程。
	// 没有 ctx 需求（看 system.go:219 实现），传 background 即可。
	h.AddTeardown(func() { _ = sys.Close(context.Background()) })

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

// buildCodingOptions assembles the cago coding.System options. Called once per
// Handle() because the approval/rounds hooks are per-conv (gw and rc are per-conv).
//
// Mapping vs. agent.New 直挂:
//   - agent.Model        → coding.WithModel
//   - agent.System       → coding.AppendSystem（拼在 cago 默认 prompt 之后）
//   - agent.Tools        → coding.WithExtraTools（追加到 10 件套 + dispatch_subagent 之后）
//   - 其余 hook/Retry     → coding.WithAgentOpts（透传给父 agent 的 agent.Option）
//
// The PreToolUse matcher is empty string (= match all tools). Per-tool
// filtering happens inside the policy / approval hooks themselves; trying to
// pre-filter at the matcher level would force us to maintain a separate
// allow-list that would drift from PolicyChecker. Tiny per-call overhead is
// acceptable for ~10 active convs.
func (m *Manager) buildCodingOptions(gw *ApprovalGateway, rc *roundsCounter) []coding.Option {
	// decisionStore 是 per-Handle 的 policy → audit 旁路：PreHook 写决策，
	// PostHook Pop 给 AuditWriter。ToolUseID 是 turn-uniq 的，不必跨 handle 共享。
	decisionStore := newToolDecisionStore()
	agentOpts := []agent.Option{
		agent.UserPromptSubmit(newMentionsHook(m.opts.Mention, m.opts.TabOpener)),
		// policy hook 内部直接调用 gw.RequestSingle 处理 NeedConfirm 分支：
		// PreToolUse 链路上只有这一处审批入口，避免重复弹卡。
		agent.PreToolUse("", newPolicyHook(m.opts.PolicyChecker, gw, decisionStore)),
		agent.PreToolUse("", rc.Hook()),
		agent.PostToolUse("", newAuditHook(m.opts.AuditWriter, decisionStore)),
		agent.OnRunnerStart(func(_ context.Context, _ *agent.Runner) error {
			rc.Reset()
			return nil
		}),
	}
	if m.opts.RetryPolicy.MaxAttempts > 1 {
		agentOpts = append(agentOpts, agent.Retry(m.opts.RetryPolicy))
	}
	return []coding.Option{
		coding.WithModel(m.opts.Model),
		coding.AppendSystem(m.opts.System),
		coding.WithExtraTools(m.opts.Tools...),
		coding.WithAgentOpts(agentOpts...),
	}
}
