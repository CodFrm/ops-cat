package aiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

const maxToolResultLen = 32 * 1024

// opsToolAdapter wraps an internal/ai.ToolDef into cago's agent.Tool. Per-call
// context injection replays the v1 pattern: SSHPool / SSHCache / MongoCache /
// KafkaService / PolicyChecker are pushed via ai.WithXxx context helpers so
// the underlying handlers can pull them at handler entry without parameter
// plumbing.
type opsToolAdapter struct {
	def  ai.ToolDef
	deps *Deps
}

// NewOpsTool wraps a single ai.ToolDef as an agent.Tool. Exposed in case
// callers want to filter / extend the tool set; production wiring goes through
// OpsTools(deps).
func NewOpsTool(def ai.ToolDef, deps *Deps) agent.Tool {
	return &opsToolAdapter{def: def, deps: deps}
}

func (t *opsToolAdapter) Name() string        { return t.def.Name }
func (t *opsToolAdapter) Description() string { return t.def.Description }

func (t *opsToolAdapter) Schema() agent.Schema {
	props := map[string]any{}
	var required []string
	for _, p := range t.def.Params {
		props[p.Name] = map[string]any{
			"type":        string(p.Type),
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	propsJSON, _ := json.Marshal(props)
	return agent.Schema{
		Type:        "object",
		Properties:  propsJSON,
		Required:    required,
		Description: t.def.Description,
	}
}

func (t *opsToolAdapter) Call(ctx context.Context, input map[string]any) (*agent.ToolResultBlock, error) {
	if input == nil {
		input = map[string]any{}
	}
	ctx = injectDeps(ctx, t.deps)
	out, err := t.def.Handler(ctx, input)
	if err != nil {
		// 工具内部错误回成 ToolResultBlock(IsError=true)，让模型看到错误信息并自适应，
		// 而不是直接走 cago HookError 路径中断 turn。这与 v1 wrapToolDef 一致。
		return &agent.ToolResultBlock{
			IsError: true,
			Content: []agent.ContentBlock{agent.TextBlock{Text: fmt.Sprintf("Tool execution error: %s", err.Error())}},
		}, nil
	}
	return &agent.ToolResultBlock{
		Content: []agent.ContentBlock{agent.TextBlock{Text: truncateToolResult(out)}},
	}, nil
}

// OpsTools 把 ai.AllToolDefs() 全部包成 agent.Tool 列表。deps 注入到 ctx 让
// 单个工具 handler 拿到 ssh/mongo/kafka 缓存与 policy checker。
func OpsTools(deps *Deps) []agent.Tool {
	defs := ai.AllToolDefs()
	out := make([]agent.Tool, 0, len(defs))
	for _, d := range defs {
		out = append(out, NewOpsTool(d, deps))
	}
	return out
}

// injectDeps 把 deps 各组件以 ctx-keyed 形态注入，让 ai.* 工具 handler 在
// 不改签名的前提下取到。空 deps 视作空注入；测试可传 &Deps{} 让 handler 走
// 自身的 nil 兜底分支。
func injectDeps(ctx context.Context, deps *Deps) context.Context {
	if deps == nil {
		return ctx
	}
	if deps.SSHCache != nil {
		ctx = ai.WithSSHCache(ctx, deps.SSHCache)
	}
	if deps.SSHPool != nil {
		ctx = ai.WithSSHPool(ctx, deps.SSHPool)
	}
	if deps.MongoCache != nil {
		ctx = ai.WithMongoDBCache(ctx, deps.MongoCache)
	}
	if deps.KafkaService != nil {
		ctx = ai.WithKafkaService(ctx, deps.KafkaService)
	}
	if deps.PolicyChecker != nil {
		ctx = ai.WithPolicyChecker(ctx, deps.PolicyChecker)
	}
	return ctx
}

// truncateToolResult 在工具结果超过 maxToolResultLen 时截到 2KB 并加截断提示。
// 与 v1 wrapToolDef 行为一致：尾部消息引导模型用更精细的过滤器再来一次，避免
// 模型为了拿全量结果反复重试同一工具。
func truncateToolResult(s string) string {
	if len(s) <= maxToolResultLen {
		return s
	}
	return s[:2048] + fmt.Sprintf(
		"\n\n--- Output truncated ---\nOutput too large (%d bytes, exceeds %d byte limit). Use more precise filters, pipe through | head or | grep, or split the query.",
		len(s), maxToolResultLen)
}
