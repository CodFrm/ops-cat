package aiagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/ai"
)

// handleCaptureEmitter is a thread-safe captureEmitter for handle tests.
// (The plain captureEmitter from bridge_test.go isn't safe under -race
// because handle tests have multiple goroutines emitting concurrently.)
type handleCaptureEmitter struct {
	mu     sync.Mutex
	convID int64
	events []ai.StreamEvent
}

func (h *handleCaptureEmitter) Emit(convID int64, ev ai.StreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.convID = convID
	h.events = append(h.events, ev)
}

func (h *handleCaptureEmitter) snapshot() []ai.StreamEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ai.StreamEvent, len(h.events))
	copy(out, h.events)
	return out
}

// setupHandle constructs an agent + conv + runner + ConvHandle. The bridge
// is wired into both runner.OnEvent (for runner events) and a goroutine
// driving conv.Watch (for change events).
func setupHandle(t *testing.T, prov provider.Provider) (*ConvHandle, *handleCaptureEmitter, context.CancelFunc) {
	a := agent.New(prov)
	conv := agent.NewConversation()
	em := &handleCaptureEmitter{}
	r := a.Runner(conv)

	bridge := newBridge(1, em)
	r.OnEvent(agent.AnyEvent, bridge.OnRunnerEvent)

	bgCtx, cancel := context.WithCancel(context.Background())
	go func() {
		for ch := range conv.Watch() {
			bridge.OnConvChange(bgCtx, ch)
		}
	}()

	h := NewConvHandle(1, conv, r)
	h.AttachBridge(bridge)
	return h, em, cancel
}

func TestConvHandle_SendNewTurnWhenNoActive(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "hello"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	h, em, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	err := h.Send(context.Background(), "raw text", "expanded body")
	require.NoError(t, err)

	types := streamTypes(em.snapshot())
	assert.Contains(t, types, "content")
	assert.Contains(t, types, "done")
}

// TestConvHandle_SendNewTurnWithMention_NoQueueConsumedBatch 复现 bug：
// 带 @-mention 时 raw != llmBody，display=raw 被挂到 user message 上做历史回放，
// 但 bridge 不应该把这次 fresh Send 当 steer-drain emit queue_consumed_batch——
// 否则前端会重复 push user 气泡 + 把当前 streaming asst 钉成空气泡。
func TestConvHandle_SendNewTurnWithMention_NoQueueConsumedBatch(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "ack"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	h, em, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	raw := "@local-docker 检查容器"
	llmBody := "## Mention context: local-docker (192.168.x.x)\n\n" + raw
	require.NoError(t, h.Send(context.Background(), raw, llmBody))

	// 等 done 出来再断言事件序列，避免读到流式 half-state。
	waitFor(t, func() bool {
		for _, e := range em.snapshot() {
			if e.Type == "done" {
				return true
			}
		}
		return false
	})

	for _, e := range em.snapshot() {
		assert.NotEqual(t, "queue_consumed_batch", e.Type,
			"Send-new-turn 带 mention 时绝不应该 emit queue_consumed_batch")
	}
}

// TestConvHandle_Edit_NoQueueConsumedBatch Edit 同样是前端已经本地 push 的
// fresh user-append，bridge 必须跳过——否则编辑后会多出一条重复 user 气泡。
func TestConvHandle_Edit_NoQueueConsumedBatch(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "v1"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "v2"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	h, em, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	require.NoError(t, h.Send(context.Background(), "u1", "u1"))

	// 等第一个 turn 走完再 edit，否则两个 turn 的事件混在一起断言不稳。
	waitFor(t, func() bool {
		count := 0
		for _, e := range em.snapshot() {
			if e.Type == "done" {
				count++
			}
		}
		return count >= 1
	})

	// Edit 带 mention 风格的 raw != llmBody。
	require.NoError(t, h.Edit(context.Background(), 0, "@asset edited", "## ctx\n\n@asset edited"))

	waitFor(t, func() bool {
		count := 0
		for _, e := range em.snapshot() {
			if e.Type == "done" {
				count++
			}
		}
		return count >= 2
	})

	for _, e := range em.snapshot() {
		assert.NotEqual(t, "queue_consumed_batch", e.Type,
			"Edit 是 fresh user-append，bridge 不该 emit queue_consumed_batch")
	}
}

func TestConvHandle_Cancel(t *testing.T) {
	prov := providertest.New().QueueStreamFunc(neverEndingStream())
	h, em, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	go func() { _ = h.Send(context.Background(), "hi", "hi") }()
	waitFor(t, func() bool { return len(em.snapshot()) > 0 })

	require.NoError(t, h.Cancel("user"))

	waitFor(t, func() bool {
		if h.conv.Len() == 0 {
			return false
		}
		last, err := h.conv.MessageAt(h.conv.Len() - 1)
		if err != nil {
			return false
		}
		return last.PartialReason == agent.PartialCancelled
	})
}

func TestConvHandle_Edit(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "old"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "new"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	h, _, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	require.NoError(t, h.Send(context.Background(), "u1", "u1"))
	require.NoError(t, h.Edit(context.Background(), 0, "u1-edit", "u1-edit"))

	last, err := h.conv.MessageAt(h.conv.Len() - 1)
	require.NoError(t, err)
	require.Equal(t, agent.RoleAssistant, last.Role)
	tb, ok := last.Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "new", tb.Text)
}

func TestConvHandle_Regenerate(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "v1"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "v2"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	h, _, cancel := setupHandle(t, prov)
	defer cancel()
	defer func() { _ = h.Close() }()

	require.NoError(t, h.Send(context.Background(), "u", "u"))
	// conv = [user("u"), assistant("v1")]; regenerate truncates assistant idx=1 and resends.
	require.NoError(t, h.Regenerate(context.Background(), 1))

	last, err := h.conv.MessageAt(h.conv.Len() - 1)
	require.NoError(t, err)
	tb, ok := last.Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "v2", tb.Text)
}

// helpers
func streamTypes(evs []ai.StreamEvent) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor timed out")
}

func neverEndingStream() func(ctx context.Context) <-chan provider.StreamChunk {
	return func(ctx context.Context) <-chan provider.StreamChunk {
		ch := make(chan provider.StreamChunk, 8)
		go func() {
			defer close(ch)
			ch <- provider.StreamChunk{ContentDelta: "drip"}
			<-ctx.Done()
		}()
		return ch
	}
}
