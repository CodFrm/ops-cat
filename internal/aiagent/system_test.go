package aiagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"

	"github.com/opskat/opskat/internal/ai"
)

func newSmokeSystem(t *testing.T, em EventEmitter) *System {
	t.Helper()
	mock := providertest.New()
	sys, err := NewSystem(context.Background(), SystemOptions{
		Provider: mock,
		Cwd:      t.TempDir(),
		ConvID:   7,
		Lang:     "en",
		Deps:     &Deps{},
		Emitter:  em,
		// CheckPerm 留空 → 默认 ai.CheckPermission；本 smoke 测不实际触发工具调用，
		// hook 不会被命中，所以走默认即可。
		// Store 注入 in-memory，避免 gormStore 默认走 conversation_svc → 真 DB。
		Store: agent.NewMemoryStore(),
	})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	t.Cleanup(func() { _ = sys.Close(context.Background()) })
	return sys
}

func TestSystem_NewClose_Smoke(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))
	if sys == nil {
		t.Fatal("NewSystem returned nil System")
	}
	if err := sys.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Idempotency: Close again is a no-op (cago coding.System uses sync.Once).
	if err := sys.Close(context.Background()); err != nil {
		t.Fatalf("Close (second): %v", err)
	}
}

// captureEmitter records every event so tests can assert on emission order.
type captureEmitter struct {
	mu     sync.Mutex
	events []ai.StreamEvent
}

func (c *captureEmitter) Emit(_ int64, ev ai.StreamEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *captureEmitter) snapshot() []ai.StreamEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ai.StreamEvent, len(c.events))
	copy(out, c.events)
	return out
}

// Steer 走 cago Session.FollowUp（safe-point drain）：不再同步 emit queue_consumed，
// 改由 bridge 在 cago 发出 EventUserPromptSubmit(Kind=FollowUp) 时合并 emit
// queue_consumed_batch。Session.FollowUp 不需要 active stream（没有时入队等下次
// Stream 启动消费），所以 Steer 在 smoke 场景下应当返回 nil 并把展示原文塞进
// displayContent FIFO。
func TestSystem_Steer_QueuesIntoDisplayFIFO(t *testing.T) {
	em := &captureEmitter{}
	sys := newSmokeSystem(t, em)

	if err := sys.Steer(context.Background(), "expanded body", "user-typed text"); err != nil {
		t.Fatalf("Steer should succeed via Session.FollowUp queue, got %v", err)
	}

	// 不应有任何同步 emit 出去的事件——前端只通过 bridge 在 cago drain 时收到通知。
	if got := em.snapshot(); len(got) != 0 {
		t.Fatalf("Steer must not synchronously emit; got %+v", got)
	}

	// 展示原文按 push 顺序进入 FIFO 头部。
	if got := sys.popPendingDisplay(); got != "user-typed text" {
		t.Fatalf("popPendingDisplay = %q, want %q", got, "user-typed text")
	}
}

// 多条 Steer 入队后，displayContent FIFO 必须按 push 顺序 pop（与 cago Session.followUp
// 的 FIFO 保持一致），bridge 翻译 EventUserPromptSubmit 时才能拿到正确的展示原文。
func TestSystem_PendingDisplay_FIFOOrder(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))

	sys.pushPendingDisplay("m1")
	sys.pushPendingDisplay("m2")
	sys.pushPendingDisplay("m3")

	for _, want := range []string{"m1", "m2", "m3", ""} {
		if got := sys.popPendingDisplay(); got != want {
			t.Fatalf("popPendingDisplay = %q, want %q", got, want)
		}
	}
}

// TestSystem_RunSlash_NonSlashPassesThrough covers the fall-through branch:
// any line that doesn't start with "/" must report IsSlash=false so the caller
// keeps the legacy send path.
func TestSystem_RunSlash_NonSlashPassesThrough(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))

	res, err := sys.RunSlash(context.Background(), "hello there")
	if err != nil {
		t.Fatalf("RunSlash: %v", err)
	}
	if res.IsSlash {
		t.Fatalf("non-slash line should not be marked IsSlash, got %+v", res)
	}
}

// TestSystem_RunSlash_BuiltinHelp invokes the always-registered /help builtin
// and asserts the SlashResult shape: IsSlash=true, Notice non-empty (UI text),
// Prompt empty (no follow-up user message).
func TestSystem_RunSlash_BuiltinHelp(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))

	res, err := sys.RunSlash(context.Background(), "/help")
	if err != nil {
		t.Fatalf("RunSlash(/help): %v", err)
	}
	if !res.IsSlash {
		t.Fatal("/help should be IsSlash=true")
	}
	if res.Prompt != "" {
		t.Errorf("/help should not produce a follow-up prompt, got %q", res.Prompt)
	}
	if !strings.Contains(res.Notice, "compact") || !strings.Contains(res.Notice, "help") {
		t.Errorf("/help notice should list builtins, got %q", res.Notice)
	}
}

// TestSystem_RunSlash_UnknownReturnsSentinel checks that callers can use
// errors.Is(err, ErrUnknownSlashCommand) to drive UI behavior. The constant is
// re-exported from cago precisely so callers don't need to import app/coding.
func TestSystem_RunSlash_UnknownReturnsSentinel(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))

	_, err := sys.RunSlash(context.Background(), "/nopesnotreal")
	if err == nil {
		t.Fatal("expected error for unknown slash command")
	}
	if !errors.Is(err, ErrUnknownSlashCommand) {
		t.Fatalf("expected ErrUnknownSlashCommand, got %v", err)
	}
}

// TestSystem_StopStream_NoActiveStreamSafe guards the lifecycle invariant: the
// frontend's "Stop" button can fire at any time, including before any Stream
// call. StopStream must not panic when streamCancel is nil.
func TestSystem_StopStream_NoActiveStreamSafe(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))
	sys.StopStream() // must not panic
}

// fakeStore is a minimal agent.Store that returns a preset SessionData on Load.
// Used to assert NewSystem rehydrates Session.History from prior persisted data
// — without this, app restart silently drops every conversation's context.
type fakeStore struct {
	id   string
	data agent.SessionData
}

func (f *fakeStore) Save(_ context.Context, _ string, _ agent.SessionData) error { return nil }
func (f *fakeStore) Load(_ context.Context, id string) (agent.SessionData, error) {
	if id != f.id {
		return agent.SessionData{}, nil
	}
	return f.data, nil
}
func (f *fakeStore) Delete(_ context.Context, _ string) error { return nil }

// TestSystem_RehydratesSessionHistoryFromStore is the OpsKat-side companion to
// the cago session_test.go fix: NewSystem must Load prior SessionData from the
// Store and seed it into the new Session, otherwise app restart (or any path
// that rebuilds *aiagent.System) starts from an empty history and the LLM has
// no memory of earlier turns in the conversation.
func TestSystem_RehydratesSessionHistoryFromStore(t *testing.T) {
	prior := agent.SessionData{
		Messages: []agent.Message{
			{Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "earlier turn", Persist: true},
			{Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "earlier reply", Persist: true},
		},
		State: agent.State{ThreadID: "thread-7"},
	}
	store := &fakeStore{id: "conv_7", data: prior}

	sys, err := NewSystem(context.Background(), SystemOptions{
		Provider: providertest.New(),
		Cwd:      t.TempDir(),
		ConvID:   7,
		Lang:     "en",
		Deps:     &Deps{},
		Emitter:  EmitterFunc(func(int64, ai.StreamEvent) {}),
		Store:    store,
	})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	t.Cleanup(func() { _ = sys.Close(context.Background()) })

	hist := sys.sess.History()
	if len(hist) != 2 {
		t.Fatalf("History len = %d, want 2 (rehydrated from store)\n  got: %+v", len(hist), hist)
	}
	if hist[0].Text != "earlier turn" || hist[1].Text != "earlier reply" {
		t.Errorf("History contents wrong: %+v", hist)
	}
	if got := sys.sess.State().ThreadID; got != "thread-7" {
		t.Errorf("State.ThreadID = %q, want \"thread-7\" (rehydrated)", got)
	}
}

// TestSystem_Close_ThenStopStreamSafe 守住关闭顺序 robustness：用户关 app 时
// AppShutdown → System.Close 已经把 streamCancel 重置为 nil；如果某条遗漏路径
// 还在调 StopStream（比如 emitter 上的"stop"按钮事件晚到一拍），不能 panic。
func TestSystem_Close_ThenStopStreamSafe(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))
	if err := sys.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	sys.StopStream() // 不能 panic
}

// TestSystem_CompactionEnabled 是 OpsKat 这层能做的唯一压缩回归：
// 老 compress.go 已删，新系统通过 coding.WithCompactionThreshold(80000) 委托给
// cago。要锁住"压缩开关没被关掉"——RunSlash("/compact") 是 cago 默认 builtin
// （cago/agents/app/coding/system.go:registerBuiltins），其内部调 sys.Compact，
// 如果 OpsKat 误传了 WithoutCompaction 会返回 ErrCompactionDisabled。
//
// 在内存 store + 空 history 下，cago 走"history 太短"分支返回
// `Notice: "No compaction needed (history too short)."`，err == nil。
// 这就证明 NewSystem 启用了 compactor。
func TestSystem_CompactionEnabled(t *testing.T) {
	sys := newSmokeSystem(t, EmitterFunc(func(int64, ai.StreamEvent) {}))
	res, err := sys.RunSlash(context.Background(), "/compact")
	if err != nil {
		t.Fatalf("RunSlash(/compact) returned err — compactor may have been disabled: %v", err)
	}
	if !res.IsSlash {
		t.Fatal("/compact should be IsSlash=true")
	}
	// 对 Notice 做包含断言而不是精确匹配——cago 改提示文本不应破坏我们的回归。
	if !strings.Contains(strings.ToLower(res.Notice), "compaction") &&
		!strings.Contains(strings.ToLower(res.Notice), "compact") {
		t.Errorf("/compact notice should mention compaction, got %q", res.Notice)
	}
}

// TestSystem_Stream_HappyPathEmitsContentAndDone wires NewSystem to a scripted
// mock provider, calls Stream once, and asserts that bridged events make it to
// the emitter (content + done). This is the integration the sub-tests can't
// cover individually: turnState is set, ConvID is plumbed via context, and the
// drain loop pipes through the bridge.
func TestSystem_Stream_HappyPathEmitsContentAndDone(t *testing.T) {
	mock := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "hello"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)

	em := &captureEmitter{}
	sys, err := NewSystem(context.Background(), SystemOptions{
		Provider: mock,
		Cwd:      t.TempDir(),
		ConvID:   42,
		Lang:     "en",
		Deps:     &Deps{},
		Emitter:  em,
		Store:    agent.NewMemoryStore(),
	})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	defer func() { _ = sys.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sys.Stream(ctx, "hi", ai.AIContext{}, nil); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawContent, sawDone bool
	for _, ev := range em.snapshot() {
		switch ev.Type {
		case "content":
			if ev.Content == "hello" {
				sawContent = true
			}
		case "done":
			sawDone = true
		}
	}
	if !sawContent {
		t.Error("Stream did not emit content")
	}
	if !sawDone {
		t.Error("Stream did not emit done")
	}
}
