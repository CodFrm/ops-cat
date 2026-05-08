package aiagent

import "context"

type ctxKey int

const (
	keyConvID ctxKey = iota
	keyAgentRole
)

// WithConvID stamps the active conversation ID for hooks and the bridge.
func WithConvID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, keyConvID, id)
}

// WithAgentRole tags the ctx with the active sub-agent role (empty = top-level).
func WithAgentRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, keyAgentRole, role)
}

func getConvID(ctx context.Context) int64 {
	if v, ok := ctx.Value(keyConvID).(int64); ok {
		return v
	}
	return 0
}

func getAgentRole(ctx context.Context) string {
	if v, ok := ctx.Value(keyAgentRole).(string); ok {
		return v
	}
	return ""
}
