package aiagent

import (
	"slices"
	"sync"

	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/tool"
	"github.com/cago-frame/agents/tool/subagent"
)

const (
	roleExplorer = "ops-explorer"
	roleBatch    = "ops-batch"
	roleReadOnly = "ops-readonly"
)

// entryToolNames is a sidecar map keyed by Entry.Type holding the names of the
// tools each Entry was constructed with. cago's subagent.Entry doesn't expose
// the underlying tool list, so we cache it here for tests and runtime
// introspection (e.g., diagnostics). Keys are the OpsKat role strings.
var entryToolNames sync.Map // map[string][]string

// OpsExplorerEntry builds the read-leaning sub-agent for "investigate this".
// Tools: list/get assets+groups, exec_* read-leaning ops, request_permission.
// No write tools.
//
// model 空字符串 = 走 backend 默认（实际通常会拼成空 model 报错）；正常应传 SystemOptions.Model。
func OpsExplorerEntry(prov provider.Provider, deps *Deps, cwd, model string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets":          true,
		"get_asset":            true,
		"list_groups":          true,
		"get_group":            true,
		"run_command":          true,
		"exec_sql":             true,
		"exec_redis":           true,
		"exec_mongo":           true,
		"exec_k8s":             true,
		"kafka_cluster":        true,
		"kafka_topic":          true,
		"kafka_consumer_group": true,
		"kafka_acl":            true,
		"kafka_schema":         true,
		"kafka_connect":        true,
		"kafka_message":        true,
		"request_permission":   true,
	})
	stampToolNames(roleExplorer, tools)
	subOpts := []coding.SubagentOption{
		coding.SubagentWithType(roleExplorer),
		coding.SubagentWithDescription("Investigate OpsKat infra: list/inspect assets and run read-leaning ops commands. Reports findings; no writes."),
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-explorer sub-agent. Investigate efficiently and report findings concisely."),
	}
	if model != "" {
		subOpts = append(subOpts, coding.SubagentWithModel(model))
	}
	return coding.Explore(prov, cwd, subOpts...)
}

// OpsBatchEntry — for "execute the same thing across N assets" tasks.
func OpsBatchEntry(prov provider.Provider, deps *Deps, cwd, model string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets":        true,
		"get_asset":          true,
		"list_groups":        true,
		"run_command":        true,
		"exec_sql":           true,
		"exec_redis":         true,
		"exec_mongo":         true,
		"batch_command":      true,
		"request_permission": true,
	})
	stampToolNames(roleBatch, tools)
	subOpts := []coding.SubagentOption{
		coding.SubagentWithType(roleBatch),
		coding.SubagentWithDescription("Execute the same operation across many OpsKat assets in parallel and consolidate results."),
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-batch sub-agent. Coordinate parallel ops across multiple assets and consolidate results."),
	}
	if model != "" {
		subOpts = append(subOpts, coding.SubagentWithModel(model))
	}
	return coding.GeneralPurpose(prov, cwd, subOpts...)
}

// OpsReadOnlyEntry — strictly inventory inspection, no execution.
func OpsReadOnlyEntry(prov provider.Provider, deps *Deps, cwd, model string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets": true,
		"get_asset":   true,
		"list_groups": true,
		"get_group":   true,
	})
	stampToolNames(roleReadOnly, tools)
	subOpts := []coding.SubagentOption{
		coding.SubagentWithType(roleReadOnly),
		coding.SubagentWithDescription("Inspect OpsKat asset inventory: list/get assets and groups. No command execution."),
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-readonly sub-agent. Inspect inventory; do not execute commands."),
	}
	if model != "" {
		subOpts = append(subOpts, coding.SubagentWithModel(model))
	}
	return coding.Explore(prov, cwd, subOpts...)
}

func filterTools(in []tool.Tool, allow map[string]bool) []tool.Tool {
	out := make([]tool.Tool, 0, len(allow))
	for _, t := range in {
		if allow[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

func stampToolNames(role string, tools []tool.Tool) {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	entryToolNames.Store(role, names)
}

// entryHasTool reports whether the Entry was constructed with a tool named
// `name`. Reads the sidecar populated at construction time.
func entryHasTool(e subagent.Entry, name string) bool {
	v, ok := entryToolNames.Load(e.Type)
	if !ok {
		return false
	}
	return slices.Contains(v.([]string), name)
}
