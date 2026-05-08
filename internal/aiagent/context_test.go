package aiagent

import (
	"context"
	"testing"
)

func TestConvIDAndAgentRole_RoundTrip(t *testing.T) {
	ctx := WithConvID(context.Background(), 42)
	ctx = WithAgentRole(ctx, "ops-explorer")
	if v := getConvID(ctx); v != 42 {
		t.Fatalf("convID = %d", v)
	}
	if v := getAgentRole(ctx); v != "ops-explorer" {
		t.Fatalf("agentRole = %q", v)
	}
}
