package aiagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"

	"github.com/opskat/opskat/internal/ai"
)

// namedTool is a minimal tool that returns a string immediately. The Name is
// configurable so a single fake can stand in for "run_command", "exec_sql", etc.
type namedTool struct {
	name string
}

func (t namedTool) Name() string            { return t.name }
func (t namedTool) Description() string     { return "noop test tool" }
func (t namedTool) Schema() json.RawMessage { return []byte(`{"type":"object","properties":{}}`) }
func (t namedTool) Serial() bool            { return false }
func (t namedTool) Call(_ context.Context, _ json.RawMessage) (any, error) {
	return "ok", nil
}

// (reuses fakeChecker from hook_policy_test.go)

// TestSystem_StreamCompletesAfterToolCall reproduces the user-reported bug:
// "AI agent gets stuck after executing a tool, doesn't continue the conversation."
//
// Sets up a cago Session with the same hook stack NewSystem installs, scripts a
// 2-round exchange (tool_call → final text), and asserts that the bridge emits
// the full sequence including "done" within a short timeout. If the run gets
// stuck after the tool result, the test times out instead of producing "done".
func TestSystem_StreamCompletesAfterToolCall(t *testing.T) {
	mock := providertest.New().
		// Round 1: model issues tool_call to a policy-gated tool (run_command).
		QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "call_1", Name: "run_command", ArgsDelta: `{"asset_id":1,"command":"ls"}`,
			}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		).
		// Round 2: model emits final text and stops.
		QueueStream(
			provider.StreamChunk{ContentDelta: "all done"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

	sc := newSidecar()
	auditWriter := &fakeAuditWriter{}
	auditHook := makeAuditHook(sc, auditWriter)
	rounds := newRoundsCounter(50)
	allow := func(_ context.Context, _ string, _ int64, _ string) ai.CheckResult {
		return ai.CheckResult{Decision: ai.Allow, DecisionSource: ai.SourcePolicyAllow}
	}
	policyHook := makePolicyHook(sc, nil, allow, nil)
	promptHook := makePromptHook(&PerTurnState{})

	a := agent.NewWithBackend(
		agent.NewBuiltinBackend(mock),
		agent.Tools(namedTool{name: "run_command"}),
		agent.SessionStart(rounds.ResetHook()),
		agent.PreToolUse("", policyHook),
		agent.PreToolUse("", rounds.Hook()),
		agent.PostToolUse("", auditHook),
		agent.UserPromptSubmit(promptHook),
	)
	sess := a.Session()

	rec := &recordEmitter{}
	br := newBridgeLegacy(rec, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RunWithRetry(ctx, sess, "do it", rec, 1, func(stream *agent.Stream) {
		for stream.Next() {
			br.translate(1, stream.Event())
		}
	})
	if err != nil {
		t.Fatalf("RunWithRetry: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	var sawDone, sawToolStart, sawToolResult, sawContent bool
	for _, ev := range rec.events {
		switch ev.Type {
		case "done":
			sawDone = true
		case "tool_start":
			sawToolStart = true
		case "tool_result":
			sawToolResult = true
		case "content":
			if ev.Content == "all done" {
				sawContent = true
			}
		}
	}
	if !sawToolStart {
		t.Errorf("no tool_start event in %d emitted events", len(rec.events))
	}
	if !sawToolResult {
		t.Errorf("no tool_result event in %d emitted events", len(rec.events))
	}
	if !sawContent {
		t.Errorf("no final content event — agent did not continue past tool result")
	}
	if !sawDone {
		t.Errorf("no done event — agent stuck after tool call")
	}
}

// TestSystem_NeedConfirm_EmitsExactlyOneApprovalRequest 是「双卡」用户回归。
//
// 历史：
//   - cago 迁移之后 PreToolUse policyHook 走 ai.CheckPermission + gw.RequestSingle，
//     emitter 闭包绑定的是当前会话 convID，弹卡 #1 正常显示。
//   - 但 ops 工具 handler 内部还有一段 in-handler 防御：if checker := GetPolicyChecker(ctx); ...
//     checker.Check(...)，会进 legacy makeCommandConfirmFunc。该路径之前因 ai.WithConversationID
//     不再注入 → convID 拿到 0 → 弹卡发到 ai:event:0 → 用户看不到。修复 ctx 注入之后这条
//     legacy 路径也变得可见 → 出现"双卡"。
//
// 当前结构：每个工具只在 PreToolUse 阶段做一次审批 gate，handler 内的 in-handler check
// 已经从 run_command / exec_sql / exec_redis / exec_mongo / exec_k8s / kafka_* 删掉。
//
// 这条测试驱动一次完整 NeedConfirm → 用户 allow → 工具运行 → 模型 wrap up 的流程，
// 断言整个 stream 里 approval_request / approval_result / tool_start / tool_result / done
// 各恰好 1 次。如果有人把 in-handler check 加回 handler，cago 又用真实 wrapToolDef
// 注入 ai.WithPolicyChecker，会触发第二次 confirmFunc → 第二个 approval_request →
// 该计数变成 2 → 测试失败。这条契约也兜住"policyHook 自己重复发卡"这种独立的回归。
func TestSystem_NeedConfirm_EmitsExactlyOneApprovalRequest(t *testing.T) {
	// lookupAssetName 默认查 asset_svc，需要真 DB；测试里 stub 掉。
	stubLookupAssetName(t, "asset-1")

	mock := providertest.New().
		QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "call_1", Name: "run_command", ArgsDelta: `{"asset_id":1,"command":"ls"}`,
			}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		).
		QueueStream(
			provider.StreamChunk{ContentDelta: "all done"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

	sc := newSidecar()
	auditWriter := &fakeAuditWriter{}
	auditHook := makeAuditHook(sc, auditWriter)
	rounds := newRoundsCounter(50)
	needConfirm := func(_ context.Context, _ string, _ int64, _ string) ai.CheckResult {
		return ai.CheckResult{Decision: ai.NeedConfirm}
	}

	rec := &recordEmitter{}

	// 自动 allow 的 resolver：channel 一开始就装一份 "allow"，gw.RequestSingle 一拉就返回。
	// 真实路径里 release() 是 sync.Map 删 confirmID 用，测试不需要副作用。
	resolver := PendingResolver(func(_ string) (chan ai.ApprovalResponse, func()) {
		ch := make(chan ai.ApprovalResponse, 1)
		ch <- ai.ApprovalResponse{Decision: "allow"}
		return ch, func() {}
	})
	gw := NewApprovalGateway(rec, resolver)
	policyHook := makePolicyHook(sc, gw, needConfirm, nil)
	promptHook := makePromptHook(&PerTurnState{})

	a := agent.NewWithBackend(
		agent.NewBuiltinBackend(mock),
		agent.Tools(namedTool{name: "run_command"}),
		agent.SessionStart(rounds.ResetHook()),
		// 与 NewSystem 注册方式保持一致：policyHook 关闭 cago 默认 5min 超时。
		agent.Hooks(agent.Hook{Stage: agent.StagePreToolUse, Fn: policyHook, Timeout: -1}),
		agent.PreToolUse("", rounds.Hook()),
		agent.PostToolUse("", auditHook),
		agent.UserPromptSubmit(promptHook),
	)
	sess := a.Session()

	br := newBridgeLegacy(rec, nil, nil)

	ctx, cancel := context.WithTimeout(WithConvID(context.Background(), 1), 5*time.Second)
	defer cancel()

	if _, err := RunWithRetry(ctx, sess, "do it", rec, 1, func(stream *agent.Stream) {
		for stream.Next() {
			br.translate(1, stream.Event())
		}
	}); err != nil {
		t.Fatalf("RunWithRetry: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	counts := map[string]int{}
	for _, ev := range rec.events {
		counts[ev.Type]++
	}
	if got := counts["approval_request"]; got != 1 {
		t.Errorf("approval_request = %d，want 1（>1 = 双卡回归，PreToolUse 之外又冒出一条审批源）", got)
	}
	if got := counts["approval_result"]; got != 1 {
		t.Errorf("approval_result = %d，want 1", got)
	}
	if got := counts["tool_start"]; got != 1 {
		t.Errorf("tool_start = %d，want 1", got)
	}
	if got := counts["tool_result"]; got != 1 {
		t.Errorf("tool_result = %d，want 1", got)
	}
	if got := counts["done"]; got != 1 {
		t.Errorf("done = %d，want 1（=0 说明 stream 没正常结束）", got)
	}
}

// TestRoundsHook_FreshCounterPerStream guards against the "stuck after a tool"
// bug: makeRoundsHook captures `var n int64` in a closure, and NewSystem
// installs that single closure on the parent agent for the Conversation's
// lifetime. Every System.Stream call reuses the same closure, so the counter
// accumulates across turns. Once it crosses MaxRounds, the next Stream's first
// PreToolUse returns StopRun — the bridge emits tool_start with no matching
// tool_result, and the UI shows the tool block "running" forever.
//
// Drives two consecutive RunWithRetry calls on the same Session with the same
// hook stack (cap=2) so the first stream uses up the budget and the second
// stream is the one at risk. Asserts the second stream still produces the full
// tool_start → tool_result → done sequence.
func TestRoundsHook_FreshCounterPerStream(t *testing.T) {
	mock := providertest.New().
		// Stream 1: two tool calls then final text. Uses 2 PreToolUse rounds.
		QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "s1_call_a", Name: "test_tool", ArgsDelta: `{}`,
			}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		).
		QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "s1_call_b", Name: "test_tool", ArgsDelta: `{}`,
			}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		).
		QueueStream(
			provider.StreamChunk{ContentDelta: "stream 1 done"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		).
		// Stream 2: one tool call then final text. With a fresh counter this
		// must complete normally; with the leaked counter it dies on the first
		// PreToolUse.
		QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "s2_call_a", Name: "test_tool", ArgsDelta: `{}`,
			}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		).
		QueueStream(
			provider.StreamChunk{ContentDelta: "stream 2 done"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

	sc := newSidecar()
	auditWriter := &fakeAuditWriter{}
	auditHook := makeAuditHook(sc, auditWriter)
	rounds := newRoundsCounter(2) // exactly enough for stream 1's two tools

	a := agent.NewWithBackend(
		agent.NewBuiltinBackend(mock),
		agent.Tools(namedTool{name: "test_tool"}),
		// SessionStart auto-reset is the fix: each Stream's SessionStart resets
		// the per-turn budget. Drop this line and stream 2 dies on its first
		// PreToolUse with the leaked counter.
		agent.SessionStart(rounds.ResetHook()),
		agent.PreToolUse("", rounds.Hook()),
		agent.PostToolUse("", auditHook),
	)
	sess := a.Session()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stream 1.
	rec1 := &recordEmitter{}
	br1 := newBridge(rec1, nil, nil)
	if _, err := RunWithRetry(ctx, sess, "do it", rec1, 1, func(stream *agent.Stream) {
		for stream.Next() {
			br1.translate(1, stream.Event())
		}
	}); err != nil {
		t.Fatalf("stream 1: %v", err)
	}

	// Stream 2 — the moment of truth.
	rec2 := &recordEmitter{}
	br2 := newBridge(rec2, nil, nil)
	if _, err := RunWithRetry(ctx, sess, "again", rec2, 1, func(stream *agent.Stream) {
		for stream.Next() {
			br2.translate(1, stream.Event())
		}
	}); err != nil {
		t.Fatalf("stream 2: %v", err)
	}

	rec2.mu.Lock()
	defer rec2.mu.Unlock()
	var sawToolStart, sawToolResult, sawFinalContent bool
	for _, ev := range rec2.events {
		switch ev.Type {
		case "tool_start":
			sawToolStart = true
		case "tool_result":
			sawToolResult = true
		case "content":
			if ev.Content == "stream 2 done" {
				sawFinalContent = true
			}
		}
	}
	if !sawToolStart {
		t.Errorf("stream 2: no tool_start event")
	}
	if !sawToolResult {
		t.Errorf("stream 2: tool_start without tool_result — UI shows tool block stuck \"running\"")
	}
	if !sawFinalContent {
		t.Errorf("stream 2: never reached the final assistant text — agent stuck after tool")
	}
}
