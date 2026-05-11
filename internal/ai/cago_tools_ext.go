package ai

import "github.com/cago-frame/agents/tool"

// extCagoTools 扩展派发：exec_tool。
// 派发逻辑（按 asset_id → 扩展类型 → 调 Plugin.CallTool）仍在 handleExecTool 中，
// 这里只把它包成 cago 原生 tool.Tool。Serial：扩展执行可能跨 SSH/远程，统一串行避免审计错位。
func extCagoTools() []tool.Tool {
	return []tool.Tool{
		rawTool(
			"exec_tool",
			"Execute an extension tool. Use this to call tools provided by installed extensions.",
			makeSchema(
				paramSpec{name: "extension", typ: "string", required: true, desc: `Extension name (e.g. "oss")`},
				paramSpec{name: "tool", typ: "string", required: true, desc: `Tool name (e.g. "list_buckets")`},
				paramSpec{name: "args", typ: "string", required: true, desc: "Tool arguments as JSON object"},
				paramSpec{name: "asset_id", typ: "number", desc: "Asset ID for policy checking"},
			),
			true,
			handleExecTool,
		),
	}
}
