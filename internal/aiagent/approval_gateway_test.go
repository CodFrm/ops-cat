package aiagent

import (
	"context"
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
