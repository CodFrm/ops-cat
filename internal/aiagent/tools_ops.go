package aiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cago-frame/agents/tool"

	"github.com/opskat/opskat/internal/ai"
)

const maxResultLen = 32 * 1024

// wrapToolDef adapts an OpsKat ai.ToolDef to cago's tool.Tool interface,
// preserving the existing handler signature, JSON schema, ctx-injected
// dependencies, and the 32KB result truncation with the helpful tail message.
func wrapToolDef(def ai.ToolDef, deps *Deps) tool.Tool {
	schema := buildSchema(def.Params)
	return &tool.RawTool{
		NameStr:   def.Name,
		DescStr:   def.Description,
		SchemaRaw: schema,
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var args map[string]any
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, fmt.Errorf("invalid args: %w", err)
				}
			} else {
				args = map[string]any{}
			}
			ctx = injectDeps(ctx, deps)
			result, err := def.Handler(ctx, args)
			if err != nil {
				return fmt.Sprintf("Tool execution error: %s", err.Error()), nil
			}
			return truncate(result), nil
		},
	}
}

func buildSchema(params []ai.ParamDef) json.RawMessage {
	properties := map[string]any{}
	var required []string
	for _, p := range params {
		properties[p.Name] = map[string]any{
			"type":        string(p.Type),
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	out := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	b, _ := json.Marshal(out)
	return b
}

func truncate(s string) string {
	if len(s) <= maxResultLen {
		return s
	}
	return s[:2048] + fmt.Sprintf(
		"\n\n--- Output truncated ---\nOutput too large (%d bytes, exceeds %d byte limit). Use more precise filters, pipe through | head or | grep, or split the query.",
		len(s), maxResultLen)
}

// injectDeps re-creates the context-keyed dependencies the legacy
// DefaultToolExecutor used to set: SSH cache, SSH pool, MongoDB cache, Kafka.
// Each Deps is per-Conversation, so calls inside one Conversation share caches.
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

// OpsTools returns the full set of OpsKat ops tools as cago tool.Tool values,
// bound to the given Deps. The legacy spawn_agent is excluded — sub-agent
// dispatch is now handled by cago's dispatch_subagent + custom Entries.
func OpsTools(deps *Deps) []tool.Tool {
	defs := ai.AllToolDefs()
	out := make([]tool.Tool, 0, len(defs))
	for _, d := range defs {
		if d.Name == "spawn_agent" {
			continue
		}
		out = append(out, wrapToolDef(d, deps))
	}
	return out
}
