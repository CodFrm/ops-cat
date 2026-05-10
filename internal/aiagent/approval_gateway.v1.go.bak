package aiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/ai"
)

// PendingResolver hands out a one-shot response channel for a confirmID and a
// teardown func that the caller defers. Implemented by internal/app's
// pendingAIApprovals sync.Map.
type PendingResolver func(confirmID string) (chan ai.ApprovalResponse, func())

// ApprovalGateway bridges the cago hook layer (which calls RequestSingle/Grant)
// to the Wails event layer (EventEmitter) and the response channel layer
// (PendingResolver). It encapsulates window activation, event emission, and
// cancellation handling.
type ApprovalGateway struct {
	emit     EventEmitter
	resolver PendingResolver
	activate func() // window activation hook; nil = no-op
}

// NewApprovalGateway constructs a gateway. activate may be nil when no window
// activation is desired (tests / headless).
func NewApprovalGateway(em EventEmitter, resolver PendingResolver) *ApprovalGateway {
	return &ApprovalGateway{emit: em, resolver: resolver}
}

// SetActivateFunc registers an optional window-activation callback called
// immediately before emitting an approval_request event.
func (g *ApprovalGateway) SetActivateFunc(fn func()) { g.activate = fn }

// RequestSingle emits an approval_request and blocks until the user responds
// or ctx cancels. kind ∈ {"single","batch"}. agentRole is "" for top-level.
func (g *ApprovalGateway) RequestSingle(ctx context.Context, convID int64, kind string,
	items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse {

	confirmID := fmt.Sprintf("ai_%d_%d", convID, time.Now().UnixNano())
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type: "approval_request", Kind: kind, Items: items,
		ConfirmID: confirmID, AgentRole: agentRole,
	})

	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: resp.Decision,
		})
		return resp
	case <-ctx.Done():
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: "deny",
		})
		return ai.ApprovalResponse{Decision: "deny"}
	}
}

// RequestGrant emits an approval_request kind=grant and blocks for response.
// Returns (approved, finalPatterns) — finalPatterns is the user's edited list
// (may differ from input items.Command).
func (g *ApprovalGateway) RequestGrant(ctx context.Context, convID int64,
	items []ai.ApprovalItem, reason string) (bool, []ai.ApprovalItem) {

	sessionID := fmt.Sprintf("conv_%d", convID)
	confirmID := fmt.Sprintf("grant_%d_%d", convID, time.Now().UnixNano())
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type: "approval_request", Kind: "grant", Items: items,
		ConfirmID: confirmID, Description: reason, SessionID: sessionID,
	})

	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: resp.Decision,
		})
		if resp.Decision == "deny" {
			return false, nil
		}
		final := resp.EditedItems
		if len(final) == 0 {
			final = items
		}
		return true, final
	case <-ctx.Done():
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: "deny",
		})
		return false, nil
	}
}
