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

// AnthropicProvider Anthropic Messages API provider
type AnthropicProvider struct {
	apiBase         string
	apiKey          string
	model           string
	name            string
	maxOutputTokens int
}

// NewAnthropicProvider 创建 Anthropic provider
func NewAnthropicProvider(name, apiBase, apiKey, model string, maxOutputTokens int) *AnthropicProvider {
	if apiBase == "" {
		apiBase = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		name:            name,
		apiBase:         strings.TrimRight(apiBase, "/"),
		apiKey:          apiKey,
		model:           model,
		maxOutputTokens: maxOutputTokens,
	}
}

func (p *AnthropicProvider) Name() string { return p.name }

// --- Anthropic API 请求/响应类型 ---

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicRequest struct {
	Model     string                 `json:"model"`
	System    []anthropicSystemBlock `json:"system,omitempty"`
	Messages  []anthropicMessage     `json:"messages"`
	Tools     []anthropicTool        `json:"tools,omitempty"`
	MaxTokens int                    `json:"max_tokens"`
	Stream    bool                   `json:"stream"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Input        interface{}            `json:"input,omitempty"`
	ToolUseID    string                 `json:"tool_use_id,omitempty"`
	Content      string                 `json:"content,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	InputSchema  interface{}            `json:"input_schema"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// --- SSE 流式响应类型 ---

type anthropicSSEEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index"`
	ContentBlock *anthropicBlockStart  `json:"content_block,omitempty"`
	Delta        *anthropicDelta       `json:"delta,omitempty"`
	Error        *anthropicStreamError `json:"error,omitempty"`
}

type anthropicBlockStart struct {
	Type string `json:"type"` // "text" | "tool_use"
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"` // "text_delta" | "input_json_delta"
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// blockState 跟踪流式 content block 的状态
type blockState struct {
	blockType string // "text" | "tool_use"
	toolID    string
	toolName  string
	inputJSON strings.Builder
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error) {
	systemText, anthropicMsgs := p.convertMessages(messages)
	anthropicTools := p.convertTools(tools)

	// System prompt 使用 cache_control 启用缓存
	var systemBlocks []anthropicSystemBlock
	if systemText != "" {
		systemBlocks = []anthropicSystemBlock{
			{Type: "text", Text: systemText, CacheControl: &anthropicCacheControl{Type: "ephemeral"}},
		}
	}

	// 最后一个 tool 添加 cache_control，缓存整个 tool 定义
	if len(anthropicTools) > 0 {
		anthropicTools[len(anthropicTools)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}

	reqBody := anthropicRequest{
		Model:     p.model,
		System:    systemBlocks,
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
		MaxTokens: p.maxOutputTokens,
		Stream:    true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

// convertMessages 将内部 Message 转换为 Anthropic 格式
// system 消息提取为顶层字段，tool 结果合并到 user 消息中
func (p *AnthropicProvider) convertMessages(messages []Message) (string, []anthropicMessage) {
	var systemPrompt string
	var result []anthropicMessage

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			systemPrompt = msg.Content

		case RoleUser:
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: msg.Content,
			})

		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				var blocks []anthropicContentBlock
				if msg.Content != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					// 解析 arguments JSON 为 interface{} 供序列化
					var input interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
						input = tc.Function.Arguments
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}

		case RoleTool:
			// 多个连续 tool 结果合并到同一个 user 消息
			toolResult := anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}
			if len(result) > 0 {
				last := &result[len(result)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]anthropicContentBlock); ok {
						last.Content = append(blocks, toolResult)
						continue
					}
				}
			}
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{toolResult},
			})
		}
	}

	return systemPrompt, result
}

// convertTools 将 OpenAI 格式的工具定义转换为 Anthropic 格式
func (p *AnthropicProvider) convertTools(tools []Tool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		result = append(result, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return result
}

func (p *AnthropicProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer func() {
		if err := body.Close(); err != nil {
			logger.Default().Warn("close SSE stream body", zap.Error(err))
		}
	}()

	// 跟踪每个 content block 的状态
	blocks := make(map[int]*blockState)

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

		var event anthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock == nil {
				continue
			}
			bs := &blockState{blockType: event.ContentBlock.Type}
			if event.ContentBlock.Type == "tool_use" {
				bs.toolID = event.ContentBlock.ID
				bs.toolName = event.ContentBlock.Name
			}
			blocks[event.Index] = bs

		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			bs, ok := blocks[event.Index]
			if !ok {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					ch <- StreamEvent{Type: "content", Content: event.Delta.Text}
				}
			case "input_json_delta":
				bs.inputJSON.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			bs, ok := blocks[event.Index]
			if !ok {
				continue
			}
			if bs.blockType == "tool_use" {
				tc := ToolCall{ID: bs.toolID, Type: "function"}
				tc.Function.Name = bs.toolName
				tc.Function.Arguments = bs.inputJSON.String()
				ch <- StreamEvent{Type: "tool_call", ToolCalls: []ToolCall{tc}}
			}
			delete(blocks, event.Index)

		case "message_stop":
			ch <- StreamEvent{Type: "done"}
			return

		case "error":
			errMsg := "unknown error"
			if event.Error != nil {
				errMsg = event.Error.Message
			}
			ch <- StreamEvent{Type: "error", Error: errMsg}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Default().Warn("SSE stream scanner error", zap.Error(err))
	}

	// 如果流意外结束（没有 message_stop），仍发送 done
	ch <- StreamEvent{Type: "done"}
}
