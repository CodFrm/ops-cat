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
	apiBase          string
	apiKey           string
	model            string
	name             string
	maxOutputTokens  int
	reasoningEnabled bool
	reasoningEffort  string
}

// NewAnthropicProvider 创建 Anthropic provider
func NewAnthropicProvider(name, apiBase, apiKey, model string, maxOutputTokens int, reasoningEnabled bool, reasoningEffort string) *AnthropicProvider {
	if apiBase == "" {
		apiBase = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		name:             name,
		apiBase:          strings.TrimRight(apiBase, "/"),
		apiKey:           apiKey,
		model:            model,
		maxOutputTokens:  maxOutputTokens,
		reasoningEnabled: reasoningEnabled,
		reasoningEffort:  normalizeAnthropicReasoningEffort(reasoningEffort),
	}
}

// normalizeAnthropicReasoningEffort 归一化 effort，未识别值落回 medium。
func normalizeAnthropicReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh", "max":
		return strings.ToLower(strings.TrimSpace(effort))
	case "none", "":
		return ""
	default:
		return "medium"
	}
}

// reasoningConfig 决定是否下发 adaptive thinking 与 effort。
// 模型是否实际支持由 Anthropic API 自行裁定（老模型会返回错误）。
func (p *AnthropicProvider) reasoningConfig() (*anthropicThinking, *anthropicOutputConfig) {
	if !p.reasoningEnabled || p.reasoningEffort == "" {
		return nil, nil
	}
	return &anthropicThinking{Type: "adaptive"}, &anthropicOutputConfig{Effort: p.reasoningEffort}
}

func (p *AnthropicProvider) Name() string { return p.name }

func (p *AnthropicProvider) Model() string { return p.model }

// --- Anthropic API 请求/响应类型 ---

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicRequest struct {
	Model        string                 `json:"model"`
	System       []anthropicSystemBlock `json:"system,omitempty"`
	Messages     []anthropicMessage     `json:"messages"`
	Tools        []anthropicTool        `json:"tools,omitempty"`
	MaxTokens    int                    `json:"max_tokens"`
	Stream       bool                   `json:"stream"`
	Thinking     *anthropicThinking     `json:"thinking,omitempty"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

type anthropicThinking struct {
	Type string `json:"type"` // "adaptive" | "disabled"
}

type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"` // "low" | "medium" | "high" | "xhigh" | "max"
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
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Thinking / Signature: type=thinking 块的内容字段（不是 text）。
	// 真 Anthropic 在 adaptive thinking + tool_use 多轮会话中要求把 thinking 块
	// 连同 signature 原样回传，否则 400。DeepSeek-v4 的 Anthropic 兼容端
	// 不签名但仍然校验 thinking 字段名，缺字段会报 "missing field thinking"。
	Thinking     string                 `json:"thinking,omitempty"`
	Signature    string                 `json:"signature,omitempty"`
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
	// message_start 时 SSE 载荷形如 {"message": {"usage": {...}}}
	// message_delta 时直接在事件上有顶层 usage 字段
	Message *anthropicStreamMessage `json:"message,omitempty"`
	Usage   *anthropicUsage         `json:"usage,omitempty"`
}

type anthropicStreamMessage struct {
	Usage *anthropicUsage `json:"usage,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type anthropicBlockStart struct {
	Type string `json:"type"` // "text" | "tool_use"
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"` // "text_delta" | "input_json_delta" | "thinking_delta" | "signature_delta"
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// blockState 跟踪流式 content block 的状态
type blockState struct {
	blockType string // "text" | "tool_use" | "thinking"
	toolID    string
	toolName  string
	inputJSON strings.Builder
	// signature 仅 thinking 块使用：累积 signature_delta 事件，content_block_stop 时落到 thinking_done。
	signature strings.Builder
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

	thinking, outputCfg := p.reasoningConfig()
	reqBody := anthropicRequest{
		Model:        p.model,
		System:       systemBlocks,
		Messages:     anthropicMsgs,
		Tools:        anthropicTools,
		MaxTokens:    p.maxOutputTokens,
		Stream:       true,
		Thinking:     thinking,
		OutputConfig: outputCfg,
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
				logger.Ctx(ctx).Warn("close HTTP response body", zap.Error(err))
			}
		}()
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logger.Ctx(ctx).Warn("read error response body", zap.Error(readErr))
		}
		errMsg := strings.TrimSpace(string(errBody))
		if errMsg == "" {
			errMsg = http.StatusText(resp.StatusCode)
			if errMsg == "" {
				errMsg = "(empty response body)"
			}
		}
		return nil, &ProviderError{
			Err:        fmt.Errorf("API error %d: %s", resp.StatusCode, errMsg),
			RetryAfter: resp.Header.Get("Retry-After"),
			StatusCode: resp.StatusCode,
		}
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
			if len(msg.ToolCalls) > 0 || msg.Thinking != "" {
				var blocks []anthropicContentBlock
				if msg.Thinking != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type:      "thinking",
						Thinking:  msg.Thinking,
						Signature: msg.ThinkingSignature,
					})
				}
				if msg.Content != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
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
			logger.Ctx(ctx).Warn("close SSE stream body", zap.Error(err))
		}
	}()

	// 跟踪每个 content block 的状态
	blocks := make(map[int]*blockState)
	// Anthropic 的 usage 分布在 message_start（输入相关）和 message_delta（输出相关）两个事件，
	// 用累加器合并为一条 usage 事件，避免前端做两次合并。
	accumulated := Usage{}
	usageStarted := false

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
			logger.Ctx(ctx).Warn("anthropic SSE event unmarshal failed",
				zap.String("model", p.model),
				zap.String("raw", data),
				zap.Error(err))
			continue
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil && event.Message.Usage != nil {
				u := event.Message.Usage
				accumulated.InputTokens += u.InputTokens
				accumulated.CacheCreationTokens += u.CacheCreationInputTokens
				accumulated.CacheReadTokens += u.CacheReadInputTokens
				// message_start 里的 output_tokens 只是占位，正式值在 message_delta
				usageStarted = true
			}

		case "message_delta":
			if event.Usage != nil {
				accumulated.OutputTokens += event.Usage.OutputTokens
				usageStarted = true
			}

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
			case "thinking_delta":
				if event.Delta.Thinking != "" {
					ch <- StreamEvent{Type: "thinking", Content: event.Delta.Thinking}
				}
			case "signature_delta":
				// 不立即转发，累积到 content_block_stop 时随 thinking_done 一并下发，
				// 避免前端为 signature 引入额外事件类型。
				bs.signature.WriteString(event.Delta.Signature)
			case "input_json_delta":
				bs.inputJSON.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			bs, ok := blocks[event.Index]
			if !ok {
				continue
			}
			switch bs.blockType {
			case "tool_use":
				tc := ToolCall{ID: bs.toolID, Type: "function"}
				tc.Function.Name = bs.toolName
				tc.Function.Arguments = bs.inputJSON.String()
				ch <- StreamEvent{Type: "tool_call", ToolCalls: []ToolCall{tc}}
			case "thinking":
				ch <- StreamEvent{Type: "thinking_done", ThinkingSignature: bs.signature.String()}
			}
			delete(blocks, event.Index)

		case "message_stop":
			if usageStarted {
				u := accumulated
				ch <- StreamEvent{Type: "usage", Usage: &u}
			}
			ch <- StreamEvent{Type: "done"}
			return

		case "error":
			errMsg := "unknown error"
			if event.Error != nil {
				errMsg = event.Error.Message
			}
			logger.Ctx(ctx).Warn("anthropic stream error event",
				zap.String("model", p.model),
				zap.String("message", errMsg),
				zap.String("raw", data))
			ch <- StreamEvent{Type: "error", Error: errMsg}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Ctx(ctx).Warn("anthropic SSE stream scanner error",
			zap.String("model", p.model),
			zap.Error(err))
		if usageStarted {
			u := accumulated
			ch <- StreamEvent{Type: "usage", Usage: &u}
		}
		ch <- StreamEvent{Type: "error", Error: fmt.Sprintf("stream interrupted: %v", err)}
		return
	}

	// 流自然结束但没收到 message_stop —— 视为异常，不能伪装成 done，
	// 否则前端会把空消息当成功完成。runner 看到 error 后会按重试策略处理。
	logger.Ctx(ctx).Warn("anthropic stream ended without message_stop",
		zap.String("model", p.model),
		zap.Bool("had_usage", usageStarted),
		zap.Int("pending_blocks", len(blocks)))
	if usageStarted {
		u := accumulated
		ch <- StreamEvent{Type: "usage", Usage: &u}
	}
	ch <- StreamEvent{Type: "error", Error: "stream ended unexpectedly without message_stop"}
}
