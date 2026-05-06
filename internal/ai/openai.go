package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// OpenAIProvider OpenAI 兼容 API provider
type OpenAIProvider struct {
	apiBase          string
	apiKey           string
	model            string
	name             string
	maxOutputTokens  int
	reasoningEnabled bool
	reasoningEffort  string
}

// NewOpenAIProvider 创建 OpenAI 兼容 provider
func NewOpenAIProvider(name, apiBase, apiKey, model string, maxOutputTokens int, reasoningEnabled bool, reasoningEffort string) *OpenAIProvider {
	return &OpenAIProvider{
		name:             name,
		apiBase:          strings.TrimRight(apiBase, "/"),
		apiKey:           apiKey,
		model:            model,
		maxOutputTokens:  maxOutputTokens,
		reasoningEnabled: reasoningEnabled,
		reasoningEffort:  normalizeOpenAIReasoningEffort(reasoningEffort),
	}
}

func (p *OpenAIProvider) Name() string { return p.name }

func (p *OpenAIProvider) Model() string { return p.model }

// openAIRequest OpenAI API 请求体
type openAIRequest struct {
	Model           string               `json:"model"`
	Messages        []Message            `json:"messages"`
	Tools           []Tool               `json:"tools,omitempty"`
	Stream          bool                 `json:"stream"`
	StreamOptions   *openAIStreamOptions `json:"stream_options,omitempty"`
	MaxTokens       int                  `json:"max_tokens,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
	// DeepSeek thinking mode: {"type":"enabled"} or {"type":"disabled"}
	Thinking *deepSeekThinking `json:"thinking,omitempty"`
}

// deepSeekThinking DeepSeek 思考模式控制
type deepSeekThinking struct {
	Type string `json:"type"` // "enabled" | "disabled"
}

// openAIStreamOptions stream 专用选项，include_usage=true 后最后一个 chunk 会带 usage
type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIStreamChunk SSE 流式响应 chunk
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	// include_usage 时 API 在最后一个 chunk 返回 usage（choices 为空数组）
	Usage *openAIUsage `json:"usage,omitempty"`
}

type openAIUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func supportsOpenAIReasoningEffort(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "gpt-5")
}

func normalizeOpenAIReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(effort))
	case "none":
		return "none"
	default:
		return "medium"
	}
}

func isDeepSeekModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek")
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error) {
	reqBody := openAIRequest{
		Model:         p.model,
		Messages:      messages,
		Stream:        true,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
		MaxTokens:     p.maxOutputTokens,
	}
	if p.reasoningEnabled && supportsOpenAIReasoningEffort(p.model) {
		reqBody.ReasoningEffort = normalizeOpenAIReasoningEffort(p.reasoningEffort)
	}
	// DeepSeek thinking mode: needs explicit thinking.type field
	if isDeepSeekModel(p.model) {
		if p.reasoningEnabled && p.reasoningEffort != "none" {
			reqBody.Thinking = &deepSeekThinking{Type: "enabled"}
			reqBody.ReasoningEffort = normalizeOpenAIReasoningEffort(p.reasoningEffort)
		} else {
			reqBody.Thinking = &deepSeekThinking{Type: "disabled"}
		}
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				logger.Default().Warn("close HTTP response body", zap.Error(err))
			}
		}()
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logger.Default().Warn("read error response body", zap.Error(readErr))
		}
		return nil, &ProviderError{
			Err:        fmt.Errorf("API error %d: %s", resp.StatusCode, string(errBody)),
			RetryAfter: resp.Header.Get("Retry-After"),
			StatusCode: resp.StatusCode,
		}
	}

	ch := make(chan StreamEvent, 32)
	go p.readStream(ctx, resp.Body, ch)
	return ch, nil
}

func (p *OpenAIProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer func() {
		if err := body.Close(); err != nil {
			logger.Default().Warn("close SSE stream body", zap.Error(err))
		}
	}()

	toolCallMap := make(map[int]*ToolCall)
	thinkingActive := false
	// include_usage=true 时最后一个 chunk 的 choices 为空，usage 字段带最终统计
	var finalUsage *openAIUsage

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Error: "canceled"}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// 最后一个 usage-only chunk：choices 为空，usage 为最终值
		if chunk.Usage != nil {
			finalUsage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// 思考/推理内容（DeepSeek 等兼容 API）
		if delta.ReasoningContent != "" {
			if !thinkingActive {
				thinkingActive = true
			}
			ch <- StreamEvent{Type: "thinking", Content: delta.ReasoningContent}
		}

		// 文本内容 — 如果之前在思考，先发 thinking_done
		if delta.Content != "" {
			if thinkingActive {
				thinkingActive = false
				ch <- StreamEvent{Type: "thinking_done"}
			}
			ch <- StreamEvent{Type: "content", Content: delta.Content}
		}

		// 工具调用（流式累积）
		for _, tc := range delta.ToolCalls {
			existing, ok := toolCallMap[tc.Index]
			if !ok {
				existing = &ToolCall{ID: tc.ID, Type: "function"}
				existing.Function.Name = tc.Function.Name
				toolCallMap[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Function.Name = tc.Function.Name
			}
			existing.Function.Arguments += tc.Function.Arguments
		}

		// 完成时发送累积的 tool calls
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "tool_calls" {
			if thinkingActive {
				thinkingActive = false
				ch <- StreamEvent{Type: "thinking_done"}
			}
			var calls []ToolCall
			for i := 0; i < len(toolCallMap); i++ {
				if tc, ok := toolCallMap[i]; ok {
					calls = append(calls, *tc)
				}
			}
			if len(calls) > 0 {
				ch <- StreamEvent{Type: "tool_call", ToolCalls: calls}
			}
		}
	}

	// 流结束时如果仍在思考，发 thinking_done
	if thinkingActive {
		ch <- StreamEvent{Type: "thinking_done"}
	}

	// 如果 scanner 结束时还有未发送的 tool calls
	if len(toolCallMap) > 0 {
		var calls []ToolCall
		for i := 0; i < len(toolCallMap); i++ {
			if tc, ok := toolCallMap[i]; ok {
				calls = append(calls, *tc)
			}
		}
		if len(calls) > 0 {
			ch <- StreamEvent{Type: "tool_call", ToolCalls: calls}
		}
	}

	if finalUsage != nil {
		// OpenAI 的 prompt_tokens 包含 cached 部分；拆开便于与 Anthropic 对齐展示
		cached := finalUsage.PromptTokensDetails.CachedTokens
		input := max(finalUsage.PromptTokens-cached, 0)
		ch <- StreamEvent{Type: "usage", Usage: &Usage{
			InputTokens:     input,
			OutputTokens:    finalUsage.CompletionTokens,
			CacheReadTokens: cached,
		}}
	}

	ch <- StreamEvent{Type: "done"}
}
