package aiagent

import (
	"testing"

	"github.com/cago-frame/agents/provider/providertest"
	"github.com/cago-frame/agents/tool/subagent"
)

func TestOpsExplorerEntry_HasReadOnlyTools(t *testing.T) {
	mock := providertest.New()
	deps := &Deps{}

	entry := OpsExplorerEntry(mock, deps, "/", "")
	if entry.Type != "ops-explorer" {
		t.Fatalf("Type = %q, want ops-explorer", entry.Type)
	}
	for _, name := range []string{"add_asset", "update_asset", "add_group", "update_group", "upload_file", "download_file", "batch_command"} {
		if entryHasTool(entry, name) {
			t.Errorf("write/batch tool %q must be excluded from ops-explorer", name)
		}
	}
	for _, name := range []string{"list_assets", "get_asset", "run_command", "exec_sql", "request_permission"} {
		if !entryHasTool(entry, name) {
			t.Errorf("read tool %q missing from ops-explorer", name)
		}
	}
}

func TestOpsBatchEntry_HasBatchAndExecTools(t *testing.T) {
	mock := providertest.New()
	deps := &Deps{}

	entry := OpsBatchEntry(mock, deps, "/", "")
	if entry.Type != "ops-batch" {
		t.Fatalf("Type = %q, want ops-batch", entry.Type)
	}
	for _, name := range []string{"batch_command", "run_command", "exec_sql", "list_assets"} {
		if !entryHasTool(entry, name) {
			t.Errorf("tool %q missing from ops-batch", name)
		}
	}
	for _, name := range []string{"add_asset", "update_asset"} {
		if entryHasTool(entry, name) {
			t.Errorf("write tool %q must not be in ops-batch", name)
		}
	}
}

func TestOpsReadOnlyEntry_HasOnlyInventoryTools(t *testing.T) {
	mock := providertest.New()
	deps := &Deps{}

	entry := OpsReadOnlyEntry(mock, deps, "/", "")
	if entry.Type != "ops-readonly" {
		t.Fatalf("Type = %q, want ops-readonly", entry.Type)
	}
	for _, name := range []string{"list_assets", "get_asset", "list_groups", "get_group"} {
		if !entryHasTool(entry, name) {
			t.Errorf("inventory tool %q missing from ops-readonly", name)
		}
	}
	for _, name := range []string{"run_command", "exec_sql", "exec_redis", "request_permission"} {
		if entryHasTool(entry, name) {
			t.Errorf("execution tool %q must not be in ops-readonly", name)
		}
	}
}

// TestEntryHasTool_UnknownRoleReturnsFalse 守住 entryToolNames 没有 key 的兜底：
// 任何陌生 Type（比如未来插入的第七种 sub-agent，或 cago 默认 explore/plan/general
// 经 dispatch 拿到的 Entry）查询时不能 panic、必须返 false。
func TestEntryHasTool_UnknownRoleReturnsFalse(t *testing.T) {
	if entryHasTool(subagent.Entry{Type: "no-such-role"}, "list_assets") {
		t.Error("unknown role: got true, want false")
	}
}

// TestEntryHasTool_KnownRoleMissingTool 验证 slice 不命中分支——例如 ops-readonly
// 不应包含 run_command。前面 ReadOnly/Explorer/Batch 三条用例已经隐含覆盖，这里
// 显式断言 entryHasTool 返 false（独立于 entry composition 测试，未来 stamp 实现
// 改成 set/map 也保持 API 不变）。
func TestEntryHasTool_KnownRoleMissingTool(t *testing.T) {
	mock := providertest.New()
	entry := OpsReadOnlyEntry(mock, &Deps{}, "/", "")
	if entryHasTool(entry, "run_command") {
		t.Error("ops-readonly must not have run_command")
	}
}
