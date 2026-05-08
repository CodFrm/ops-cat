package aiagent

import (
	"testing"

	"github.com/cago-frame/agents/provider/providertest"
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
