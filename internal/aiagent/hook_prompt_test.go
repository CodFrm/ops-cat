package aiagent

import (
	"context"
	"strings"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

func TestPromptHook_InjectsTabsAndMentions(t *testing.T) {
	state := &PerTurnState{}
	state.Set(ai.AIContext{
		OpenTabs:        []ai.TabInfo{{Type: "ssh", AssetID: 9, AssetName: "edge-1"}},
		MentionedAssets: []ai.MentionedAsset{{AssetID: 9, Name: "edge-1", Type: "ssh", Host: "10.0.0.1"}},
	}, nil)

	hook := makePromptHook(state)
	out, err := hook(context.Background(), agent.HookInput{Stage: agent.StageUserPromptSubmit})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.AdditionalContext == "" {
		t.Fatal("expected AdditionalContext")
	}
	if !strings.Contains(out.AdditionalContext, "edge-1") {
		t.Errorf("missing tab name in: %s", out.AdditionalContext)
	}
	if !strings.Contains(out.AdditionalContext, "@edge-1") {
		t.Errorf("missing mention in: %s", out.AdditionalContext)
	}
}

func TestPromptHook_NoStateNoOp(t *testing.T) {
	state := &PerTurnState{}
	hook := makePromptHook(state)
	out, _ := hook(context.Background(), agent.HookInput{Stage: agent.StageUserPromptSubmit})
	if out != nil && out.AdditionalContext != "" {
		t.Fatalf("expected empty additionalContext, got %q", out.AdditionalContext)
	}
}
