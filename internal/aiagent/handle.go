package aiagent

import (
	"context"
	"errors"

	"github.com/cago-frame/agents/agent"
)

// ConvHandle wraps a single conversation's cago *Conversation + *Runner. It is
// the unit of user-facing action: Send (unified entry), Cancel, Edit (truncate
// + replace last user msg + resend), Regenerate (truncate + resend).
//
// One ConvHandle per active conversation. Manager (Task 19) owns the registry.
type ConvHandle struct {
	convID int64
	conv   *agent.Conversation
	runner *agent.Runner
	closed bool

	// Wired by Manager.Handle. Close() unwinds them in reverse order.
	teardown []func()
}

// AddTeardown registers a cleanup func to be called by Close, in LIFO order.
// Manager uses this to attach the bridge goroutine cancel, the approval
// gateway cancel, and the recorder unbind.
func (h *ConvHandle) AddTeardown(fn func()) {
	h.teardown = append(h.teardown, fn)
}

// NewConvHandle constructs a ConvHandle. Manager wires the bridge / hooks /
// recorder before handing the handle to callers — this constructor does not
// own those concerns.
func NewConvHandle(convID int64, conv *agent.Conversation, runner *agent.Runner) *ConvHandle {
	return &ConvHandle{convID: convID, conv: conv, runner: runner}
}

// ConvID returns the conversation ID this handle manages.
func (h *ConvHandle) ConvID() int64 { return h.convID }

// Conv returns the underlying cago Conversation. Read-only callers (UI render)
// may use it; writers should go through Send / Edit / Regenerate.
func (h *ConvHandle) Conv() *agent.Conversation { return h.conv }

// Send is the unified user-message entry. raw is the UI display text; llmBody
// is the mention-expanded version sent to the LLM. The two are equal when no
// mention expansion happened.
//
// Routing:
//  1. Try Steer first — injects llmBody into the currently active turn.
//     The runner's auto-continue picks it up after the next safe point.
//  2. If no active turn (ErrSteerNoActiveTurn), start a new turn via Send.
//
// The display text is attached as a MetadataBlock{Key:"display", Value:raw}
// so the UI can render the user's raw input unchanged while the LLM sees the
// expanded form. (See Task 3 / Task 4 for the cago-side support.)
func (h *ConvHandle) Send(ctx context.Context, raw, llmBody string) error {
	steerOpts := []agent.SteerOption{}
	if raw != "" && raw != llmBody {
		steerOpts = append(steerOpts, agent.WithSteerDisplay(raw))
	}
	err := h.runner.Steer(ctx, llmBody, steerOpts...)
	if err == nil {
		return nil
	}
	if !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		return err
	}

	sendOpts := []agent.SendOption{}
	if raw != "" && raw != llmBody {
		sendOpts = append(sendOpts, agent.WithSendDisplay(raw))
	}
	events, err := h.runner.Send(ctx, llmBody, sendOpts...)
	if err != nil {
		return err
	}
	// Drain the iterator; OnEvent observers (bridge) handle the actual emits.
	for range events {
	}
	return nil
}

// Cancel cancels the currently active turn. The partial assistant message gets
// finalized with PartialReason=cancelled. Returns nil even if no turn is
// active (idempotent).
func (h *ConvHandle) Cancel(reason string) error {
	if reason == "" {
		reason = "user"
	}
	err := h.runner.Cancel(reason)
	if errors.Is(err, agent.ErrSteerNoActiveTurn) {
		return nil
	}
	return err
}

// Edit truncates the conversation at idx (dropping idx and everything after),
// appends a new user message with the given raw / llmBody, and runs a fresh
// turn via Resend. Used by the "edit a message and resend" UX.
func (h *ConvHandle) Edit(ctx context.Context, idx int, raw, llmBody string) error {
	if err := h.conv.Truncate(idx); err != nil {
		return err
	}
	h.conv.Append(buildUserMessage(raw, llmBody))
	return h.resend(ctx)
}

// Regenerate truncates the conversation at assistIdx (dropping the assistant
// message and any later messages) and runs a fresh turn via Resend on the
// preceding user message. Used by the "regenerate this answer" UX.
func (h *ConvHandle) Regenerate(ctx context.Context, assistIdx int) error {
	if err := h.conv.Truncate(assistIdx); err != nil {
		return err
	}
	return h.resend(ctx)
}

// Close releases the runner and runs all registered teardowns in LIFO order.
// Idempotent.
func (h *ConvHandle) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	for i := len(h.teardown) - 1; i >= 0; i-- {
		h.teardown[i]()
	}
	return h.runner.Close()
}

// resend triggers a fresh turn from the current conversation tail.
func (h *ConvHandle) resend(ctx context.Context) error {
	events, err := h.runner.Resend(ctx)
	if err != nil {
		return err
	}
	for range events {
	}
	return nil
}

// buildUserMessage constructs a user Message with the LLM body as the primary
// TextBlock and an optional display MetadataBlock when raw differs from llmBody.
func buildUserMessage(raw, llmBody string) agent.Message {
	content := []agent.ContentBlock{agent.TextBlock{Text: llmBody}}
	if raw != "" && raw != llmBody {
		content = append(content, agent.MetadataBlock{Key: "display", Value: raw})
	}
	return agent.Message{Role: agent.RoleUser, Content: content}
}
