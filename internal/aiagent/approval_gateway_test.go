package aiagent

import (
	"context"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/ai"
)

// apprCaptureEmitter is a local emitter for approval_gateway tests, separate
// from the captureEmitter in bridge_test.go (same package; avoids DuplicateDecl).
type apprCaptureEmitter struct {
	convID int64
	events []ai.StreamEvent
}

func (c *apprCaptureEmitter) Emit(convID int64, ev ai.StreamEvent) {
	c.convID = convID
	c.events = append(c.events, ev)
}

type fakeGrants struct {
	hasAllowed bool
	saved      []string
}

func (f *fakeGrants) Has(_ context.Context, _, _ string) bool {
	return f.hasAllowed
}
func (f *fakeGrants) Save(_ context.Context, sessionID, toolName string) {
	f.saved = append(f.saved, sessionID+":"+toolName)
}

// fakeResolver returns a synchronous channel that the test pre-fills.
type fakeResolver struct {
	respCh   chan ai.ApprovalResponse
	released chan struct{}
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{
		respCh:   make(chan ai.ApprovalResponse, 1),
		released: make(chan struct{}, 1),
	}
}

func (f *fakeResolver) Resolver() PendingResolver {
	return func(_ string) (chan ai.ApprovalResponse, func()) {
		return f.respCh, func() { f.released <- struct{}{} }
	}
}

func TestApprovalGateway_LocalGrantShortCircuit(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{hasAllowed: true}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	hook := g.Hook()
	out, err := hook(ctx, &agent.PreToolUseInput{
		ToolName: "Write", ToolUseID: "tu_1",
		Input: map[string]any{"path": "/tmp/x"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionApprove, out.Decision)
	// No emit (short-circuit), resolver not called.
	assert.Empty(t, em.events, "local grant should suppress all emits")

	_ = g.Close()
}

func TestApprovalGateway_UserApproves(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	// Pre-arm the response so the gateway sees "approve" once it emits the request.
	res.respCh <- ai.ApprovalResponse{Decision: "approve"}

	hook := g.Hook()
	out, err := hook(ctx, &agent.PreToolUseInput{
		ToolName: "ssh.exec", ToolUseID: "tu_2",
		Input: map[string]any{"cmd": "ls"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionApprove, out.Decision)

	// Two emits: approval_request + approval_result
	require.Len(t, em.events, 2)
	assert.Equal(t, "approval_request", em.events[0].Type)
	assert.Equal(t, "ssh.exec", em.events[0].ToolName)
	assert.Equal(t, "approval_result", em.events[1].Type)
	assert.Equal(t, "approve", em.events[1].Content)

	_ = g.Close()
}

func TestApprovalGateway_UserDenies(t *testing.T) {
	em := &apprCaptureEmitter{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, &fakeGrants{}, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	res.respCh <- ai.ApprovalResponse{Decision: "deny"}

	hook := g.Hook()
	out, err := hook(ctx, &agent.PreToolUseInput{
		ToolName: "rm.file", ToolUseID: "tu_3",
		Input: map[string]any{},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)

	_ = g.Close()
}

func TestApprovalGateway_AlwaysGrantPersists(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	res.respCh <- ai.ApprovalResponse{Decision: "always"}

	hook := g.Hook()
	out, err := hook(ctx, &agent.PreToolUseInput{
		ToolName: "Write", ToolUseID: "tu_4",
		Input: map[string]any{},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionApprove, out.Decision)
	assert.Equal(t, []string{"conv_42:Write"}, grants.saved, "always should persist via grants.Save")

	_ = g.Close()
}

func TestApprovalGateway_CtxCancelDeniesPending(t *testing.T) {
	em := &apprCaptureEmitter{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, &fakeGrants{}, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)

	hookDone := make(chan struct{})
	go func() {
		defer close(hookDone)
		hook := g.Hook()
		out, err := hook(context.Background(), &agent.PreToolUseInput{
			ToolName: "stuck", ToolUseID: "tu_stuck",
			Input: map[string]any{},
		})
		assert.NoError(t, err)
		assert.Equal(t, agent.DecisionDeny, out.Decision, "ctx cancel should deny pending")
	}()

	// Wait until the gateway is blocked on the resolver, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-hookDone:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("hook did not return after ctx cancel")
	}

	_ = g.Close()
}
