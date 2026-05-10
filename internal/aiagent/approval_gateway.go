package aiagent

import (
	"context"
	"fmt"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/agent/approve"

	"github.com/opskat/opskat/internal/ai"
)

// LocalGrantStore is the session-level "always-allow" switch for cago built-in
// tools. Production impl in local_grants.go reads/writes local_tool_grant_repo.
// Definition moved here from the deleted v1 hook_policy.go.
type LocalGrantStore interface {
	Has(ctx context.Context, sessionID, toolName string) bool
	Save(ctx context.Context, sessionID, toolName string)
}

// PendingResolver hands out a one-shot response channel for a confirmID and a
// teardown func that the caller defers. Implemented by internal/app's
// pendingAIApprovals sync.Map. Same shape as v1 — preserved so the existing
// app-side wiring continues to work.
type PendingResolver func(confirmID string) (chan ai.ApprovalResponse, func())

// ApprovalGateway wraps a cago approve.Approver. One gateway instance per
// active conversation. Manager creates one when it builds a ConvHandle, runs
// Run(ctx) as a goroutine, and registers Hook() as the approver hook on the
// underlying *agent.Agent.
//
// Lifecycle:
//   - Hook() returns the cago PreToolUseHook that submits Pending entries
//   - Run(ctx) loops over Pending, routes each through grants → resolver → cago Approve/Deny
//   - Close() shuts down the approver (cago auto-denies any in-flight Pending)
type ApprovalGateway struct {
	convID   int64
	approver *approve.Approver
	emit     EventEmitter
	grants   LocalGrantStore
	resolver PendingResolver
	activate func() // optional UI-window activation; nil = no-op
}

func NewApprovalGateway(convID int64, em EventEmitter, grants LocalGrantStore, resolver PendingResolver) *ApprovalGateway {
	return &ApprovalGateway{
		convID:   convID,
		approver: approve.New(),
		emit:     em,
		grants:   grants,
		resolver: resolver,
	}
}

func (g *ApprovalGateway) SetActivateFunc(fn func()) { g.activate = fn }

// Approver returns the underlying cago Approver. Used in tests and by the
// Manager when wiring approve.Hook into the agent's PreToolUse hook chain.
func (g *ApprovalGateway) Approver() *approve.Approver { return g.approver }

// Hook returns the cago PreToolUse hook produced by approve.Hook(g.approver).
// Manager registers this with agent.PreToolUse(toolMatcher, gateway.Hook()).
func (g *ApprovalGateway) Hook() agent.PreToolUseHook { return approve.Hook(g.approver) }

// Run processes Pending entries from the approver until ctx is done or the
// approver is closed. Each Pending is either short-circuited via the local
// grants store, or routed through the resolver/Wails event pipeline.
//
// Run blocks; call as a goroutine: `go gw.Run(ctx)`.
func (g *ApprovalGateway) Run(ctx context.Context) {
	for p := range g.approver.Pending() {
		select {
		case <-ctx.Done():
			_ = g.approver.Deny(p.ID, "context canceled")
			return
		default:
		}
		g.handlePending(ctx, p)
	}
}

// Close shuts down the approver. Any in-flight Pending entries get auto-denied.
func (g *ApprovalGateway) Close() error { return g.approver.Close() }

func (g *ApprovalGateway) handlePending(ctx context.Context, p approve.Pending) {
	// Local grants short-circuit: if the user previously chose "always allow",
	// skip the user prompt and approve immediately.
	sessionID := fmt.Sprintf("conv_%d", g.convID)
	if g.grants != nil && g.grants.Has(ctx, sessionID, p.ToolName) {
		_ = g.approver.Approve(p.ID)
		return
	}

	// Build approval_request event. Use cago's Pending.ID as the confirmID so
	// the resolver and approver share keys.
	confirmID := p.ID
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(g.convID, ai.StreamEvent{
		Type:       "approval_request",
		Kind:       "single",
		ConfirmID:  confirmID,
		ToolName:   p.ToolName,
		ToolCallID: p.ToolUseID,
		// Items omitted: rich descriptions are produced by the v1 helper layer
		// and will be wired in Task 21 when the policy adapter materializes.
	})

	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emitResult(confirmID, resp.Decision)
		switch resp.Decision {
		case "approve":
			_ = g.approver.Approve(p.ID)
		case "always":
			if g.grants != nil {
				g.grants.Save(ctx, sessionID, p.ToolName)
			}
			_ = g.approver.Approve(p.ID)
		default: // "deny" or anything else
			_ = g.approver.Deny(p.ID, "user denied")
		}
	case <-ctx.Done():
		_ = g.approver.Deny(p.ID, "context canceled")
		g.emitResult(confirmID, "deny")
	}
}

func (g *ApprovalGateway) emitResult(confirmID, decision string) {
	g.emit.Emit(g.convID, ai.StreamEvent{
		Type:      "approval_result",
		ConfirmID: confirmID,
		Content:   decision,
	})
}
