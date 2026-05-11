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
	bridge *eventBridge // optional; nil-safe (unit tests may omit).
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

// AttachBridge wires the per-conv event bridge so Send/Edit can prime
// SkipNextUserAppend before fresh user-message appends. Manager calls this
// inside Handle(); unit tests that don't need queue-consumed semantics may
// leave it unset (handle is nil-safe).
func (h *ConvHandle) AttachBridge(b *eventBridge) {
	h.bridge = b
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
// The display text rides on a DisplayTextBlock (Audience=ToUI|ToStore) when
// raw != llmBody, so the UI keeps the user's raw input ("@srv1 status") while
// BuildRequest strips it before the LLM call. On the Steer path the equivalent
// is WithSteerDisplay(raw); on the Send-new-turn path WithSendDisplay(raw); on
// the Edit / mid-turn append path buildUserMessage attaches it directly.
func (h *ConvHandle) Send(ctx context.Context, raw, llmBody string) error {
	// Steer 路径必须始终挂 WithSteerDisplay(raw)：bridge.OnConvChange 用 display
	// 字段区分「队列 drain 的 user msg」(需要 emit queue_consumed_batch 让前端 pop
	// 本地 pendingQueue) 和「新 turn 的首条 user msg」(前端已自己 push，bridge 不
	// 该再 echo)。如果 raw == llmBody 时不挂 display，display="" → bridge 把空串塞
	// 进 pending → frontend 过滤掉 → 本地 pendingQueue 永远不被消费，UI 上排队
	// 气泡停在 pending 状态。
	steerOpts := []agent.SteerOption{}
	if raw != "" {
		steerOpts = append(steerOpts, agent.WithSteerDisplay(raw))
	}
	err := h.runner.Steer(ctx, llmBody, steerOpts...)
	if err == nil {
		return nil
	}
	if !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		return err
	}

	// Send 路径（新 turn）：前端已经 push 了 user 气泡，所以 display 仅在 mention
	// 扩展导致 raw != llmBody 时挂上（让历史回放渲染 raw 而不是 expanded body）。
	// 因为这条 user-append 不该走 bridge 的 queue_consumed_batch 链路，必须先 prime
	// bridge 跳过——否则带 @ 提及时 display 非空，前端会重复 push user 气泡，并把
	// 当前 streaming asst placeholder 钉成空气泡。
	if h.bridge != nil {
		h.bridge.SkipNextUserAppend()
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
// finalized with PartialReason=canceled. Returns nil even if no turn is
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
	// 与 Send 新 turn 同理：前端 edit 框已经把新文本本地 push 上去了，下面这条
	// conv.Append 不该再 echo 回前端，否则会重复一条 user 气泡。
	if h.bridge != nil {
		h.bridge.SkipNextUserAppend()
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
// TextBlock and an optional DisplayTextBlock when raw differs from llmBody.
//
// 三路投影：
//   - LLM：TextBlock(llmBody) 走 BuildRequest（Audience=ToAll）。
//   - UI：raw != llmBody 时挂 DisplayTextBlock(raw) —— 前端历史回放渲染 raw。
//   - 储存：两块都被 Recorder 写入（Audience 都含 ToStore）。
//
// raw == llmBody 时只挂一个 TextBlock，UI 走 text fallback 渲染。
func buildUserMessage(raw, llmBody string) agent.Message {
	content := []agent.ContentBlock{agent.TextBlock{Text: llmBody}}
	if raw != "" && raw != llmBody {
		content = append(content, agent.DisplayTextBlock{Text: raw})
	}
	return agent.Message{Role: agent.RoleUser, Content: content}
}
