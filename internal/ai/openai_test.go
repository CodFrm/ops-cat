package ai

import "testing"

func TestSupportsOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "o1 model", model: "o1-preview", want: true},
		{name: "o3 model", model: "o3-mini", want: true},
		{name: "o4 model", model: "o4-mini", want: true},
		{name: "gpt-5 model", model: "gpt-5.4", want: true},
		{name: "gpt-4o model", model: "gpt-4o", want: false},
		{name: "deepseek model", model: "deepseek-chat", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsOpenAIReasoningEffort(tt.model); got != tt.want {
				t.Fatalf("supportsOpenAIReasoningEffort(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestIsDeepSeekModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "deepseek-chat", model: "deepseek-chat", want: true},
		{name: "deepseek-v4-pro", model: "deepseek-v4-pro", want: true},
		{name: "DeepSeek-V4", model: "DeepSeek-V4", want: true},
		{name: "gpt-5", model: "gpt-5.4", want: false},
		{name: "o3-mini", model: "o3-mini", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeepSeekModel(tt.model); got != tt.want {
				t.Fatalf("isDeepSeekModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestNormalizeOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		want   string
	}{
		{name: "low", effort: "low", want: "low"},
		{name: "upper medium", effort: "MEDIUM", want: "medium"},
		{name: "trim high", effort: "  high ", want: "high"},
		{name: "xhigh", effort: "xhigh", want: "xhigh"},
		{name: "none", effort: "none", want: "none"},
		{name: "empty defaults to medium", effort: "", want: "medium"},
		{name: "unknown defaults to medium", effort: "ultra", want: "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIReasoningEffort(tt.effort); got != tt.want {
				t.Fatalf("normalizeOpenAIReasoningEffort(%q) = %q, want %q", tt.effort, got, tt.want)
			}
		})
	}
}
