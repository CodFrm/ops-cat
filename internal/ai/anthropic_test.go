package ai

import "testing"

func TestNormalizeAnthropicReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		want   string
	}{
		{name: "low", effort: "low", want: "low"},
		{name: "medium upper", effort: "MEDIUM", want: "medium"},
		{name: "high with spaces", effort: "  high  ", want: "high"},
		{name: "xhigh", effort: "xhigh", want: "xhigh"},
		{name: "max anthropic-only", effort: "max", want: "max"},
		{name: "none", effort: "none", want: ""},
		{name: "empty", effort: "", want: ""},
		{name: "garbage falls back to medium", effort: "ultra", want: "medium"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAnthropicReasoningEffort(tt.effort); got != tt.want {
				t.Fatalf("normalizeAnthropicReasoningEffort(%q) = %q, want %q", tt.effort, got, tt.want)
			}
		})
	}
}

func TestAnthropicProviderReasoningConfig(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		effort         string
		wantThinking   bool
		wantThinkType  string
		wantEffortSent string
	}{
		{name: "disabled when not enabled", enabled: false, effort: "high", wantThinking: false},
		{name: "disabled when effort=none", enabled: true, effort: "none", wantThinking: false},
		{name: "adaptive + medium", enabled: true, effort: "medium", wantThinking: true, wantThinkType: "adaptive", wantEffortSent: "medium"},
		{name: "adaptive + max", enabled: true, effort: "max", wantThinking: true, wantThinkType: "adaptive", wantEffortSent: "max"},
		{name: "adaptive + xhigh", enabled: true, effort: "xhigh", wantThinking: true, wantThinkType: "adaptive", wantEffortSent: "xhigh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnthropicProvider("test", "", "key", "claude-opus-4-7", 16000, tt.enabled, tt.effort)
			thinking, outputCfg := p.reasoningConfig()

			if tt.wantThinking {
				if thinking == nil {
					t.Fatalf("expected thinking, got nil")
				}
				if thinking.Type != tt.wantThinkType {
					t.Fatalf("thinking.type = %q, want %q", thinking.Type, tt.wantThinkType)
				}
				if outputCfg == nil {
					t.Fatalf("expected output_config, got nil")
				}
				if outputCfg.Effort != tt.wantEffortSent {
					t.Fatalf("output_config.effort = %q, want %q", outputCfg.Effort, tt.wantEffortSent)
				}
			} else {
				if thinking != nil {
					t.Fatalf("expected no thinking, got %+v", thinking)
				}
				if outputCfg != nil {
					t.Fatalf("expected no output_config, got %+v", outputCfg)
				}
			}
		})
	}
}
