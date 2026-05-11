package ai

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// auditWriter 由 rawTool 在每次 handler 返回后异步写审计日志。
// 包级变量便于测试时替换（参考 audit_test.go）。
var auditWriter AuditWriter = NewDefaultAuditWriter()

// === 共用 helpers ===
//
// 这些 helper 不是新的"工具抽象层"，仅用于减少 cago 原生 *tool.RawTool 字面量构造时的重复样板。
// 1) paramSpec / makeSchema：把若干参数 spec 压成 agent.Schema（其内部的 Properties 是 json.RawMessage）；
// 2) textResult：把现有 handler 的 (string, error) 包成 *agent.ToolResultBlock。
//
// 每个工具仍以 *tool.RawTool 值的形式定义，handler 直接调用现有 handleXxx，
// 没有 ToolDef / AllToolDefs / ToOpenAITools 这一层。

type paramSpec struct {
	name     string
	typ      string // "string" | "number" | "boolean" | "object" | "array"
	required bool
	desc     string
}

func makeSchema(params ...paramSpec) agent.Schema {
	props := map[string]any{}
	var required []string
	for _, p := range params {
		props[p.name] = map[string]any{
			"type":        p.typ,
			"description": p.desc,
		}
		if p.required {
			required = append(required, p.name)
		}
	}
	raw, _ := json.Marshal(props)
	return agent.Schema{
		Type:       "object",
		Properties: raw,
		Required:   required,
	}
}

// textResult 把现有 handler 返回的 (string, error) 包成 cago 的 *agent.ToolResultBlock。
// error 走 IsError=true 路径让模型自己消化，不上抛给 runner（cago 的 PostToolUse 钩子语义）。
//
//nolint:nilerr // 故意把 err 折进 IsError=true block，外层返回 nil 让 cago 不视为失败。
func textResult(out string, err error) (*agent.ToolResultBlock, error) {
	if err != nil {
		return &agent.ToolResultBlock{
			Content: []agent.ContentBlock{&agent.TextBlock{Text: err.Error()}},
			IsError: true,
		}, nil
	}
	return &agent.ToolResultBlock{
		Content: []agent.ContentBlock{&agent.TextBlock{Text: out}},
	}, nil
}

// rawTool 把 (string, error) handler 包成 *tool.RawTool。serial=true 时整轮串行。
//
// 在 handler 外层包了两件事，等价于旧 AuditingExecutor 的行为：
//  1. 注入 *CheckResult 占位指针，handler 自己做权限检查时通过 setCheckResult 填充；
//  2. handler 返回后异步写审计日志，携带原始 ctx 里的 session/conversation 信息 + 决策结果。
//
// 旧 AuditingExecutor 与本 wrapper 并存——分别给 conversation_runner.go 与 M5 的新 Runner 使用，
// 互不影响；M6 切换完成后旧 executor 整体下线。
func rawTool(name, desc string, schema agent.Schema, serial bool, handler func(context.Context, map[string]any) (string, error)) tool.Tool {
	return &tool.RawTool{
		NameStr:   name,
		DescStr:   desc,
		SchemaVal: schema,
		IsSerial:  serial,
		Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
			decision := &CheckResult{}
			callCtx := withCheckResult(ctx, decision)

			result, err := handler(callCtx, in)

			// fire-and-forget 异步写审计；原始 ctx 携带 session/conversation 上下文。
			argsJSON, _ := json.Marshal(in)
			go auditWriter.WriteToolCall(ctx, ToolCallInfo{
				ToolName: name,
				ArgsJSON: string(argsJSON),
				Result:   result,
				Error:    err,
				Decision: decision,
			})

			return textResult(result, err)
		},
	}
}

// CagoTools 返回所有 cago 原生工具实例。
//
// 与现有 AllToolDefs() 的差异：
//   - 不再返回 ToolDef + 经 ToOpenAITools 二次转换；
//   - 每个工具是 *tool.RawTool 字面量，Schema/Handler/Serial 直接挂在上面；
//   - 删除了 spawn_agent / batch_command（由 cago dispatch_subagent + 模型多次 run_command 覆盖）。
//
// M5 切换 runner 后 AllToolDefs() / ToOpenAITools 整体下线。
func CagoTools() []tool.Tool {
	// 24 = 8 asset + 4 exec + 4 data + 7 kafka + 1 ext，常量预分配避免多次扩容
	tools := make([]tool.Tool, 0, 24)
	tools = append(tools, assetCagoTools()...)
	tools = append(tools, execCagoTools()...)
	tools = append(tools, dataCagoTools()...)
	tools = append(tools, kafkaCagoTools()...)
	tools = append(tools, extCagoTools()...)
	return tools
}
