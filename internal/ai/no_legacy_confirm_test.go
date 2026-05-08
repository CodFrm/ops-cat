package ai

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"go.uber.org/mock/gomock"
)

// 这一组测试看住"in-handler 旧防御已删除"的契约。
//
// 历史：每个 ops handler（run_command / exec_sql / exec_redis / exec_mongo / exec_k8s）
// 内部曾经有一段防御：
//
//	if checker := GetPolicyChecker(ctx); checker != nil {
//	    result := checker.Check(ctx, assetID, command)
//	    if result.Decision != Allow { return result.Message, nil }
//	}
//
// cago 迁移之后审批 gate 由 PreToolUse aiagent.policyHook 统一处理（走 gw.RequestSingle，
// emitter 闭包绑定的是当前会话 convID）。in-handler 那段会再走 legacy makeCommandConfirmFunc，
// 用户那边表现成"双卡"（参考 internal/aiagent/stream_integration_test.go
// TestSystem_NeedConfirm_EmitsExactlyOneApprovalRequest）。
//
// 这里给每个 handler 注入一个会计数 confirm 调用次数的 PolicyChecker。如果有人把那段
// 防御加回来，confirm 计数会从 0 变 ≥1，测试挂掉。
//
// handler 本身会在策略 check 之后 / 之前去查 asset 或 dial 真实依赖；测试让 asset 查询
// 返回错误，handler 早退，结果错误信息我们不关心 —— 只关心 confirm 计数。

// noLegacyChecker 是一只只为计数 confirm 调用次数的 *CommandPolicyChecker。返回 deny 让
// 行为在"加回 in-handler check"分支里 path 收敛到早退，避免 handler 跑到下一阶段产生噪声。
func noLegacyChecker() (*CommandPolicyChecker, *atomic.Int32) {
	var calls atomic.Int32
	checker := NewCommandPolicyChecker(func(_ context.Context, _ string, _ []ApprovalItem, _ string) ApprovalResponse {
		calls.Add(1)
		return ApprovalResponse{Decision: "deny"}
	})
	return checker, &calls
}

// TestHandleRunCommand_NoLegacyConfirm 看住 run_command 的 in-handler check 已删除。
func TestHandleRunCommand_NoLegacyConfirm(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	mockAsset.EXPECT().Find(gomock.Any(), int64(99)).
		Return(nil, errors.New("asset not found")).AnyTimes()

	checker, calls := noLegacyChecker()
	ctx = WithPolicyChecker(ctx, checker)

	_, _ = handleRunCommand(ctx, map[string]any{
		"asset_id": float64(99),
		"command":  "ls",
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("legacy confirmFunc 被调用了 %d 次 —— in-handler check 又被加回 handleRunCommand"+
			"，会触发 PreToolUse + in-handler 双卡回归", got)
	}
}

// TestHandleExecSQL_NoLegacyConfirm 看住 exec_sql 的 in-handler check 已删除。
func TestHandleExecSQL_NoLegacyConfirm(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	mockAsset.EXPECT().Find(gomock.Any(), int64(99)).
		Return(nil, errors.New("asset not found")).AnyTimes()

	checker, calls := noLegacyChecker()
	ctx = WithPolicyChecker(ctx, checker)

	_, _ = handleExecSQL(ctx, map[string]any{
		"asset_id": float64(99),
		"sql":      "select 1",
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("legacy confirmFunc 被调用了 %d 次 —— in-handler check 又被加回 handleExecSQL", got)
	}
}

// TestHandleExecRedis_NoLegacyConfirm 看住 exec_redis 的 in-handler check 已删除。
func TestHandleExecRedis_NoLegacyConfirm(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	mockAsset.EXPECT().Find(gomock.Any(), int64(99)).
		Return(nil, errors.New("asset not found")).AnyTimes()

	checker, calls := noLegacyChecker()
	ctx = WithPolicyChecker(ctx, checker)

	_, _ = handleExecRedis(ctx, map[string]any{
		"asset_id": float64(99),
		"command":  "GET k",
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("legacy confirmFunc 被调用了 %d 次 —— in-handler check 又被加回 handleExecRedis", got)
	}
}

// TestHandleExecMongo_NoLegacyConfirm 看住 exec_mongo 的 in-handler check 已删除。
func TestHandleExecMongo_NoLegacyConfirm(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	mockAsset.EXPECT().Find(gomock.Any(), int64(99)).
		Return(nil, errors.New("asset not found")).AnyTimes()

	checker, calls := noLegacyChecker()
	ctx = WithPolicyChecker(ctx, checker)

	_, _ = handleExecMongo(ctx, map[string]any{
		"asset_id":  float64(99),
		"operation": "find",
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("legacy confirmFunc 被调用了 %d 次 —— in-handler check 又被加回 handleExecMongo", got)
	}
}

// TestHandleExecK8s_NoLegacyConfirm 看住 exec_k8s 的 in-handler check 已删除。
func TestHandleExecK8s_NoLegacyConfirm(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	mockAsset.EXPECT().Find(gomock.Any(), int64(99)).
		Return(nil, errors.New("asset not found")).AnyTimes()

	checker, calls := noLegacyChecker()
	ctx = WithPolicyChecker(ctx, checker)

	_, _ = handleExecK8s(ctx, map[string]any{
		"asset_id": float64(99),
		"command":  "get pods",
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("legacy confirmFunc 被调用了 %d 次 —— in-handler check 又被加回 handleExecK8s", got)
	}
}

// TestCheckKafkaToolPermission_NoOp 钉死 kafka stub 的 no-op 契约。
//
// 历史：checkKafkaToolPermission 曾经走 GetPolicyChecker(ctx).CheckForAsset 做策略 + confirm，
// 与 PreToolUse policyHook 重叠，造成双卡。现在改成无条件返回 (Allow, true)，调用方那 7 处
// `if result, ok := ...; !ok` 写法不变，但永远走 ok=true 分支。
//
// 这条测试同时覆盖 "ctx 里有 PolicyChecker（带 confirmFunc）也不该被触发" —— 是另一种回归
// 形式（有人把 GetPolicyChecker 那段挂回来）。
func TestCheckKafkaToolPermission_NoOp(t *testing.T) {
	checker, calls := noLegacyChecker()

	// 干净 ctx：函数应当 Allow + true，不接触任何依赖。
	result, ok := checkKafkaToolPermission(context.Background(), 1, "topic.delete *")
	if result.Decision != Allow {
		t.Errorf("clean ctx: decision = %v, want Allow", result.Decision)
	}
	if !ok {
		t.Errorf("clean ctx: ok = false, want true（callsites 写法是 if !ok { return result.Message }）")
	}

	// 即使把 PolicyChecker 塞进 ctx，stub 也不该 reach 它。
	ctx := WithPolicyChecker(context.Background(), checker)
	result, ok = checkKafkaToolPermission(ctx, 1, "topic.delete *")
	if result.Decision != Allow || !ok {
		t.Errorf("with PolicyChecker: decision=%v ok=%v, want Allow+true", result.Decision, ok)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("kafka stub 调用了 confirmFunc %d 次 —— in-handler 防御又被挂回 checkKafkaToolPermission", got)
	}
}
