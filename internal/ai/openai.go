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
	apiBase         string
	apiKey          string
	model           string
	name            string
	maxOutputTokens int
}

// NewOpenAIProvider 创建 OpenAI 兼容 provider
func NewOpenAIProvider(name, apiBase, apiKey, model string, maxOutputTokens int) *OpenAIProvider {
	return &OpenAIProvider{
		name:            name,
		apiBase:         strings.TrimRight(apiBase, "/"),
		apiKey:          apiKey,
		model:           model,
		maxOutputTokens: maxOutputTokens,
	}
}

func (p *OpenAIProvider) Name() string { return p.name }

// openAIRequest OpenAI API 请求体
type openAIRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	Stream    bool      `json:"stream"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// openAIStreamChunk SSE 流式响应 chunk
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
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
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error) {
	reqBody := openAIRequest{
		Model:     p.model,
		Messages:  messages,
		Stream:    true,
		MaxTokens: p.maxOutputTokens,
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
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(errBody))
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

	// 累积 tool calls（流式中 tool_calls 是分片的）
	toolCallMap := make(map[int]*ToolCall)

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

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// 文本内容
		if delta.Content != "" {
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

	// 如果 scanner 结束时还有未发送的 tool calls
	if len(toolCallMap) > 0 {
		var calls []ToolCall
		for i := 0; i < len(toolCallMap); i++ {
			if tc, ok := toolCallMap[i]; ok {
				calls = append(calls, *tc)
			}
		}
		if len(calls) > 0 {
			// 检查是否已经发过了
			ch <- StreamEvent{Type: "tool_call", ToolCalls: calls}
		}
	}

	ch <- StreamEvent{Type: "done"}
}
