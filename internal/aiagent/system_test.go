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

// TestSystem_Steer_EmitsQueueConsumedAndForwards verifies the contract documented
// on Steer: the UI sees "queue_consumed" with the user-visible text *before*
// sess.Steer is invoked, even when sess.Steer ultimately fails (no active
// stream). The frontend relies on this event to clear its pending banner.
func TestSystem_Steer_EmitsQueueConsumedAndForwards(t *testing.T) {
	em := &captureEmitter{}
	sys := newSmokeSystem(t, em)

	err := sys.Steer(context.Background(), "expanded body", "user-typed text")
	// No active stream → cago returns ErrNoActiveStream. We don't assert on
	// the exact sentinel (cago-internal) — just that Steer surfaces an error
	// rather than silently swallowing it.
	if err == nil {
		t.Fatal("Steer with no active stream should return an error")
	}
	if !errors.Is(err, agent.ErrNoActiveStream) {
		t.Fatalf("expected ErrNoActiveStream, got %v", err)
	}

	evs := em.snapshot()
	if len(evs) == 0 || evs[0].Type != "queue_consumed" {
		t.Fatalf("first emitted event must be queue_consumed, got %+v", evs)
	}
	if evs[0].Content != "user-typed text" {
		t.Errorf("queue_consumed.Content = %q, want display text", evs[0].Content)
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
