package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

// TestAnthropicConvertMessages_ThinkingBlockSerialization 回归测试：
// thinking 块必须以 "thinking" 字段而不是 "text" 字段输出，
// 否则 DeepSeek-v4 等 Anthropic 兼容端会 400: "missing field thinking"。
func TestAnthropicConvertMessages_ThinkingBlockSerialization(t *testing.T) {
	p := NewAnthropicProvider("test", "", "key", "claude-opus-4-7", 16000, true, "medium")

	tc := ToolCall{ID: "call_1", Type: "function"}
	tc.Function.Name = "list_assets"
	tc.Function.Arguments = `{}`

	_, msgs := p.convertMessages([]Message{
		{Role: RoleUser, Content: "list assets"},
		{
			Role:      RoleAssistant,
			Thinking:  "let me think",
			Content:   "ok",
			ToolCalls: []ToolCall{tc},
		},
	})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	data, err := json.Marshal(msgs[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blocks, ok := raw["content"].([]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("expected content block array, got %#v", raw["content"])
	}
	first, ok := blocks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first block to be object, got %#v", blocks[0])
	}
	if first["type"] != "thinking" {
		t.Fatalf("first block type = %v, want \"thinking\"", first["type"])
	}
	if first["thinking"] != "let me think" {
		t.Fatalf("first block thinking = %v, want \"let me think\"", first["thinking"])
	}
	if _, hasText := first["text"]; hasText {
		t.Fatalf("thinking block must not contain \"text\" field, got %#v", first)
	}
}

// TestAnthropicConvertMessages_ThinkingSignatureRoundtrip 验证 thinking 块的 signature
// 在 Message → 出站 JSON 这一段被原样回传。真 Anthropic 在 reasoning + tool_use 多轮
// 会话中没有签名会 400，所以必须确保签名不被丢弃。
func TestAnthropicConvertMessages_ThinkingSignatureRoundtrip(t *testing.T) {
	p := NewAnthropicProvider("test", "", "key", "claude-opus-4-7", 16000, true, "medium")

	tc := ToolCall{ID: "call_1", Type: "function"}
	tc.Function.Name = "list_assets"
	tc.Function.Arguments = `{}`

	_, msgs := p.convertMessages([]Message{
		{Role: RoleUser, Content: "go"},
		{
			Role:              RoleAssistant,
			Thinking:          "let me think",
			ThinkingSignature: "sig-abc-123",
			ToolCalls:         []ToolCall{tc},
		},
	})

	data, err := json.Marshal(msgs[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	first := raw["content"].([]any)[0].(map[string]any)
	if first["signature"] != "sig-abc-123" {
		t.Fatalf("expected signature \"sig-abc-123\" on thinking block, got %#v", first)
	}

	// 没有签名时（DeepSeek 兼容端）omitempty 应当略掉 signature 字段，避免空字符串混淆后端。
	_, msgs2 := p.convertMessages([]Message{
		{Role: RoleUser, Content: "go"},
		{Role: RoleAssistant, Thinking: "x", ToolCalls: []ToolCall{tc}},
	})
	data2, _ := json.Marshal(msgs2[1])
	var raw2 map[string]any
	_ = json.Unmarshal(data2, &raw2)
	first2 := raw2["content"].([]any)[0].(map[string]any)
	if _, has := first2["signature"]; has {
		t.Fatalf("expected signature field to be omitted when empty, got %#v", first2)
	}
}

// TestAnthropicReadStream_CapturesSignature 验证流式解析把 signature_delta 累积起来，
// 并在 thinking 块结束时随 thinking_done 事件下发，供上层写入 Message.ThinkingSignature。
func TestAnthropicReadStream_CapturesSignature(t *testing.T) {
	// 模拟一个真 Anthropic 风格的 SSE 流：thinking 块 + signature_delta + tool_use + 结束。
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think "}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hard"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"xyz"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test", srv.URL, "key", "claude-opus-4-7", 1000, true, "medium")
	ch, err := p.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var sawDone bool
	var capturedSig string
	var thinkingBuf strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "thinking":
			thinkingBuf.WriteString(ev.Content)
		case "thinking_done":
			capturedSig = ev.ThinkingSignature
		case "done":
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatalf("stream did not complete with done")
	}
	if thinkingBuf.String() != "think hard" {
		t.Fatalf("thinking buf = %q, want \"think hard\"", thinkingBuf.String())
	}
	if capturedSig != "sig-xyz" {
		t.Fatalf("thinking_done signature = %q, want \"sig-xyz\"", capturedSig)
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
