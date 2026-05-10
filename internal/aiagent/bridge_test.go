package aiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
)

type captureEmitter struct {
	convID int64
	events []ai.StreamEvent
}

func (c *captureEmitter) Emit(convID int64, ev ai.StreamEvent) {
	c.convID = convID
	c.events = append(c.events, ev)
}

func TestBridge_TextDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hi"})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "content", em.events[0].Type)
	assert.Equal(t, "hi", em.events[0].Content)
	assert.Equal(t, int64(42), em.convID)
}

func TestBridge_ThinkingDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "let me think"})
	assert.Equal(t, "thinking", em.events[0].Type)
	assert.Equal(t, "let me think", em.events[0].Content)
}

func TestBridge_ErrorEmitsErrorOnly(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventError, Error: errors.New("boom")})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "error", em.events[0].Type)
	assert.Contains(t, em.events[0].Error, "boom")
}

func TestBridge_DoneEmitted(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "done", em.events[0].Type)
}

func TestBridge_RetryEvent(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventRetry,
		Retry: &agent.RetryEvent{
			Attempt: 2,
			Delay:   0,
			Cause:   errors.New("503"),
		},
	})
	assert.Equal(t, "retry", em.events[0].Type)
	assert.Contains(t, em.events[0].Content, "2/")
	assert.Contains(t, em.events[0].Error, "503")
}

func TestBridge_QueueConsumedBatch_AggregatesUserAppends(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	// Two user-message appends in a row (e.g., from Steer'd queue draining)
	userMsgA := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded A"},
		agent.MetadataBlock{Key: "display", Value: "raw A"},
	}}
	userMsgB := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded B"},
		agent.MetadataBlock{Key: "display", Value: "raw B"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &userMsgA})
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 1, Message: &userMsgB})

	// At this point nothing should have been emitted (still pending)
	assert.Len(t, em.events, 0, "user appends are buffered until a runner event arrives")

	// Next runner event triggers the flush BEFORE the event itself
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "ack"})

	// Expected: queue_consumed_batch with both raw displays, then content
	assert.Len(t, em.events, 2)
	assert.Equal(t, "queue_consumed_batch", em.events[0].Type)
	assert.Equal(t, []string{"raw A", "raw B"}, em.events[0].QueueContents)
	assert.Equal(t, "content", em.events[1].Type)
	assert.Equal(t, "ack", em.events[1].Content)
}

func TestBridge_QueueConsumedBatch_NonUserAppendIgnored(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	// Assistant message append shouldn't queue anything
	asstMsg := agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &asstMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hello"})

	// Expected: only "content", no batch flush
	assert.Len(t, em.events, 1)
	assert.Equal(t, "content", em.events[0].Type)
}

func TestBridge_QueueConsumedBatch_UserWithoutDisplayUsesEmptyString(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "no display block here"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &userMsg})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "x"})

	// Expected: batch with one empty string
	assert.Equal(t, "queue_consumed_batch", em.events[0].Type)
	assert.Equal(t, []string{""}, em.events[0].QueueContents)
}
