package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"
)

// AuditWriter persists one audit row per tool invocation. Production wiring
// (Task 21) constructs an adapter around audit_repo / opsctl audit pipeline;
// this hook is purely observational — does NOT modify the tool output.
//
// Errors from Write are SWALLOWED (audit failure must not break the agent).
// The implementation may log internally if desired.
type AuditWriter interface {
	Write(ctx context.Context, toolName, inputJSON, outputJSON string, isError bool) error
}

// newAuditHook returns a PostToolUseHook that serializes input + output to JSON
// and calls AuditWriter.Write. Output blocks are serialized via the same
// serializeBlocks helper used by gormStore (Task 9), so the audit JSON shape
// matches what's persisted in conversation_messages.
func newAuditHook(w AuditWriter) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		// Defensive: skip if Output is nil (shouldn't happen per cago contract,
		// but a malformed tool result would panic on .Content access).
		if in.Output == nil {
			return &agent.PostToolUseOutput{}, nil
		}
		inJSON, _ := json.Marshal(in.Input)
		outBlocks := serializeBlocks(in.Output.Content)
		outJSON, _ := json.Marshal(outBlocks)
		_ = w.Write(ctx, in.ToolName, string(inJSON), string(outJSON), in.Output.IsError)
		// Don't modify output; pure observer.
		return &agent.PostToolUseOutput{}, nil
	}
}
