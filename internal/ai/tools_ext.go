package ai

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// extTools 扩展派发：exec_tool。
// 派发逻辑（按 asset_id → 扩展类型 → 调 Plugin.CallTool）仍在 handleExecTool 中，
// 这里只把它包成 cago 原生 tool.Tool。Serial：扩展执行可能跨 SSH/远程，统一串行避免审计错位。
func extTools() []tool.Tool {
	return []tool.Tool{
		&tool.RawTool{
			NameStr: "exec_tool",
			DescStr: "Execute an extension tool. Use this to call tools provided by installed extensions.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"extension": {Type: "string", Description: `Extension name (e.g. "oss")`},
					"tool":      {Type: "string", Description: `Tool name (e.g. "list_buckets")`},
					"args":      {Type: "string", Description: "Tool arguments as JSON object"},
					"asset_id":  {Type: "number", Description: "Asset ID for policy checking"},
				},
				Required: []string{"extension", "tool", "args"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleExecTool(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
	}
}
