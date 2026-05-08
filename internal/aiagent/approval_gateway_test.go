package aiagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/ai"
)

func TestApprovalGateway_RequestEmitsAndBlocks(t *testing.T) {
	emitted := make(chan ai.StreamEvent, 2)
	em := EmitterFunc(func(_ int64, ev ai.StreamEvent) { emitted <- ev })
	resolve := make(chan ai.ApprovalResponse, 1)
	gw := NewApprovalGateway(em, func(confirmID string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		resolve <- ai.ApprovalResponse{Decision: "allow"}
	}()

	resp := gw.RequestSingle(context.Background(), 7, "exec",
		[]ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls"}}, "")

	if resp.Decision != "allow" {
		t.Fatalf("decision = %s", resp.Decision)
	}
	ev := <-emitted
	if ev.Type != "approval_request" {
		t.Fatalf("first event = %s", ev.Type)
	}
	ev2 := <-emitted
	if ev2.Type != "approval_result" || ev2.Content != "allow" {
		t.Fatalf("second event = %+v", ev2)
	}
}

func TestApprovalGateway_ContextCancelDenies(t *testing.T) {
	em := EmitterFunc(func(int64, ai.StreamEvent) {})
	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return make(chan ai.ApprovalResponse), func() {}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp := gw.RequestSingle(ctx, 1, "exec", nil, "")
	if resp.Decision != "deny" {
		t.Fatalf("expected deny on canceled ctx, got %s", resp.Decision)
	}
}

// TestApprovalGateway_RequestGrant_AllowReturnsEditedItems mirrors the policy-
// hook's "save user-edited grant pattern" expectation: when the user edits the
// command in the grant dialog, RequestGrant must surface the edited items, not
// the original.
func TestApprovalGateway_RequestGrant_AllowReturnsEditedItems(t *testing.T) {
	emitted := make(chan ai.StreamEvent, 2)
	em := EmitterFunc(func(_ int64, ev ai.StreamEvent) { emitted <- ev })
	resolve := make(chan ai.ApprovalResponse, 1)
	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		resolve <- ai.ApprovalResponse{
			Decision:    "allow",
			EditedItems: []ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls /var/*"}},
		}
	}()

	ok, final := gw.RequestGrant(context.Background(), 5,
		[]ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls"}}, "expand pattern")

	if !ok {
		t.Fatal("expected approved=true on allow decision")
	}
	if len(final) != 1 || final[0].Command != "ls /var/*" {
		t.Fatalf("expected user-edited pattern, got %+v", final)
	}
	ev := <-emitted
	if ev.Type != "approval_request" || ev.Kind != "grant" || ev.Description != "expand pattern" {
		t.Fatalf("first event = %+v", ev)
	}
}

// TestApprovalGateway_RequestGrant_DenyReturnsNoItems confirms the contract for
// the deny branch: approved=false, items=nil — even if the user supplies edits,
// the caller must not persist them.
func TestApprovalGateway_RequestGrant_DenyReturnsNoItems(t *testing.T) {
	em := EmitterFunc(func(int64, ai.StreamEvent) {})
	resolve := make(chan ai.ApprovalResponse, 1)
	resolve <- ai.ApprovalResponse{Decision: "deny"}
	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})

	ok, final := gw.RequestGrant(context.Background(), 1,
		[]ai.ApprovalItem{{Command: "ls"}}, "")
	if ok {
		t.Fatal("expected approved=false on deny")
	}
	if final != nil {
		t.Fatalf("deny must zero out items, got %+v", final)
	}
}

// TestApprovalGateway_RequestGrant_FallsBackToOriginalWhenNoEdits covers the
// branch where the user clicks Allow without editing — final must equal the
// caller-supplied items so saveGrantPatternFromResponse stores the policy-
// proposed pattern.
func TestApprovalGateway_RequestGrant_FallsBackToOriginalWhenNoEdits(t *testing.T) {
	em := EmitterFunc(func(int64, ai.StreamEvent) {})
	resolve := make(chan ai.ApprovalResponse, 1)
	resolve <- ai.ApprovalResponse{Decision: "allow"} // no EditedItems
	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})

	original := []ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "uname -a"}}
	ok, final := gw.RequestGrant(context.Background(), 1, original, "")
	if !ok {
		t.Fatal("expected approved=true")
	}
	if len(final) != 1 || final[0].Command != "uname -a" {
		t.Fatalf("expected fallback to original, got %+v", final)
	}
}

// TestApprovalGateway_ActivateFuncCalledBeforeEmit pins down the UX guarantee:
// the desktop window is brought to front *before* the approval_request event is
// emitted, so the user sees the dialog without manually focusing the app.
func TestApprovalGateway_ActivateFuncCalledBeforeEmit(t *testing.T) {
	var order []string
	var mu sync.Mutex
	em := EmitterFunc(func(_ int64, _ ai.StreamEvent) {
		mu.Lock()
		order = append(order, "emit")
		mu.Unlock()
	})
	resolve := make(chan ai.ApprovalResponse, 1)
	resolve <- ai.ApprovalResponse{Decision: "deny"}

	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})
	gw.SetActivateFunc(func() {
		mu.Lock()
		order = append(order, "activate")
		mu.Unlock()
	})

	gw.RequestSingle(context.Background(), 1, "exec",
		[]ai.ApprovalItem{{Command: "x"}}, "")

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 || order[0] != "activate" || order[1] != "emit" {
		t.Fatalf("expected activate→emit ordering, got %v", order)
	}
}
