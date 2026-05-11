package aiagent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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

// 本地工具命中 grants → RequestSingle 直接合成 allow，不发 wails 事件。
func TestApprovalGateway_LocalGrantShortCircuit(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{hasAllowed: true}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())

	resp := g.RequestSingle(context.Background(), "local_write",
		[]ai.ApprovalItem{{Type: "local_write", Command: "/tmp/x"}}, "")
	assert.Equal(t, "allow", resp.Decision)
	assert.Empty(t, em.events, "local grant should suppress all emits")
}

// local_bash 即使 grants 命中也不能短路 —— bash 永不放行。
func TestApprovalGateway_LocalBashIgnoresGrant(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{hasAllowed: true}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "allow"}

	resp := g.RequestSingle(context.Background(), "local_bash",
		[]ai.ApprovalItem{{Type: "local_bash", Command: "rm -rf /"}}, "")
	assert.Equal(t, "allow", resp.Decision)
	// 必须发出弹卡（说明 grants 短路没生效）。
	assert.Len(t, em.events, 2)
	assert.Equal(t, "approval_request", em.events[0].Type)
}

// 用户 allow → RequestSingle 返回 allow，两个事件按序发出。
func TestApprovalGateway_UserAllows(t *testing.T) {
	em := &apprCaptureEmitter{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, &fakeGrants{}, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "allow"}

	resp := g.RequestSingle(context.Background(), "exec",
		[]ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls"}}, "")
	assert.Equal(t, "allow", resp.Decision)

	assert.Len(t, em.events, 2)
	assert.Equal(t, "approval_request", em.events[0].Type)
	assert.Equal(t, "exec", em.events[0].Kind)
	assert.Equal(t, "approval_result", em.events[1].Type)
	assert.Equal(t, "allow", em.events[1].Content)
}

// 用户 deny → RequestSingle 返回 deny。
func TestApprovalGateway_UserDenies(t *testing.T) {
	em := &apprCaptureEmitter{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, &fakeGrants{}, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "deny"}

	resp := g.RequestSingle(context.Background(), "exec",
		[]ai.ApprovalItem{{Type: "exec", Command: "rm -rf /"}}, "")
	assert.Equal(t, "deny", resp.Decision)
}

// allowAll + local_write → 落 grants；bash 不落。
func TestApprovalGateway_AllowAllPersistsForLocalWrite(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "allowAll"}

	g.RequestSingle(context.Background(), "local_write",
		[]ai.ApprovalItem{{Type: "local_write", Command: "/tmp/x"}}, "")
	assert.Equal(t, []string{"conv_42:write"}, grants.saved)
}

func TestApprovalGateway_AllowAllSkipsBash(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "allowAll"}

	g.RequestSingle(context.Background(), "local_bash",
		[]ai.ApprovalItem{{Type: "local_bash", Command: "rm -rf /"}}, "")
	assert.Empty(t, grants.saved, "bash allowAll must not persist")
}

// 非 local_* 工具 allowAll 不落 grants（资产维度走另一条 grant 路径，由
// policy hook 自身负责）。
func TestApprovalGateway_AllowAllSkipsAssetKind(t *testing.T) {
	em := &apprCaptureEmitter{}
	grants := &fakeGrants{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, grants, res.Resolver())
	res.respCh <- ai.ApprovalResponse{Decision: "allowAll"}

	g.RequestSingle(context.Background(), "exec",
		[]ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls"}}, "")
	assert.Empty(t, grants.saved, "asset-kind allowAll handled elsewhere")
}

// ctx 取消时 RequestSingle 立即返回 deny。
func TestApprovalGateway_CtxCancelDenies(t *testing.T) {
	em := &apprCaptureEmitter{}
	res := newFakeResolver()
	g := NewApprovalGateway(42, em, &fakeGrants{}, res.Resolver())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan ai.ApprovalResponse, 1)
	go func() {
		done <- g.RequestSingle(ctx, "exec",
			[]ai.ApprovalItem{{Type: "exec", Command: "stuck"}}, "")
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case resp := <-done:
		assert.Equal(t, "deny", resp.Decision)
	case <-time.After(2 * time.Second):
		t.Fatal("RequestSingle did not return after ctx cancel")
	}
}
