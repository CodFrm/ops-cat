package ai

import (
	"encoding/json"
	"fmt"
)

// Claude CLI stream-json 事件解析
// 事件格式为 NDJSON，每行一个 JSON 对象

// claudeRawEvent Claude CLI stream-json 原始事件
type claudeRawEvent struct {
	Type      string          `json:"type"`       // system, assistant, stream_event, result
	Subtype   string          `json:"subtype"`    // init 等
	SessionID string          `json:"session_id"` // system init 时返回
	Event     json.RawMessage `json:"event"`      // stream_event 时的子事件
	Result    string          `json:"result"`     // result 时的最终文本
	// assistant 消息
	Message *claudeAssistantMessage `json:"message"`
}

type claudeAssistantMessage struct {
	Role    string               `json:"role"`
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type string `json:"type"` // text, tool_use
	Text string `json:"text"` // text 类型
	Name string `json:"name"` // tool_use 类型
	ID   string `json:"id"`   // tool_use 类型
}

// claudeStreamSubEvent stream_event 内部事件
type claudeStreamSubEvent struct {
	Type         string               `json:"type"` // content_block_start, content_block_delta, content_block_stop, message_start, message_delta, message_stop
	Index        int                  `json:"index"`
	Delta        *claudeDelta         `json:"delta"`
	ContentBlock *claudeContentBlock  `json:"content_block"`
}

type claudeDelta struct {
	Type string `json:"type"` // text_delta, input_json_delta
	Text string `json:"text"` // text_delta 时
}

// ClaudeEventParser 解析 Claude CLI stream-json 事件
type ClaudeEventParser struct {
	SessionID    string
	currentTools map[int]string // index → tool name
	toolInputs   map[int]string // index → accumulated JSON input
}

// NewClaudeEventParser 创建解析器
func NewClaudeEventParser() *ClaudeEventParser {
	return &ClaudeEventParser{
		currentTools: make(map[int]string),
		toolInputs:   make(map[int]string),
	}
}

// ParseLine 解析一行 JSON，返回 StreamEvent 和是否完成
// 返回的 events 可能为空（忽略的事件），done 表示对话结束
func (p *ClaudeEventParser) ParseLine(line string) (events []StreamEvent, done bool) {
	if line == "" {
		return nil, false
	}

	var raw claudeRawEvent
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return []StreamEvent{{Type: "error", Error: fmt.Sprintf("解析事件失败: %s", err)}}, false
	}

	switch raw.Type {
	case "system":
		return p.handleSystem(&raw)
	case "stream_event":
		return p.handleStreamEvent(raw.Event)
	case "assistant":
		return p.handleAssistant(&raw)
	case "result":
		return nil, true
	}

	return nil, false
}

func (p *ClaudeEventParser) handleSystem(raw *claudeRawEvent) ([]StreamEvent, bool) {
	if raw.Subtype == "init" && raw.SessionID != "" {
		p.SessionID = raw.SessionID
	}
	return nil, false
}

func (p *ClaudeEventParser) handleStreamEvent(eventData json.RawMessage) ([]StreamEvent, bool) {
	if eventData == nil {
		return nil, false
	}

	var sub claudeStreamSubEvent
	if err := json.Unmarshal(eventData, &sub); err != nil {
		return nil, false
	}

	switch sub.Type {
	case "content_block_start":
		if sub.ContentBlock != nil && sub.ContentBlock.Type == "tool_use" {
			p.currentTools[sub.Index] = sub.ContentBlock.Name
			p.toolInputs[sub.Index] = ""
			return []StreamEvent{{
				Type:     "tool_start",
				ToolName: sub.ContentBlock.Name,
			}}, false
		}

	case "content_block_delta":
		if sub.Delta != nil {
			switch sub.Delta.Type {
			case "text_delta":
				if sub.Delta.Text != "" {
					return []StreamEvent{{
						Type:    "content",
						Content: sub.Delta.Text,
					}}, false
				}
			case "input_json_delta":
				// 累积工具输入 JSON
				if sub.Delta.Text != "" {
					p.toolInputs[sub.Index] += sub.Delta.Text
				}
			}
		}

	case "content_block_stop":
		if toolName, ok := p.currentTools[sub.Index]; ok {
			// 工具输入累积完成，提取摘要
			input := extractToolInputSummary(toolName, p.toolInputs[sub.Index])
			delete(p.currentTools, sub.Index)
			delete(p.toolInputs, sub.Index)
			if input != "" {
				return []StreamEvent{{
					Type:      "tool_start",
					ToolName:  toolName,
					ToolInput: input,
				}}, false
			}
		}

	case "message_stop":
		// 消息结束，但不一定是对话结束（可能还有 tool 执行后续轮次）
	}

	return nil, false
}

func (p *ClaudeEventParser) handleAssistant(_ *claudeRawEvent) ([]StreamEvent, bool) {
	// assistant 消息包含完整文本，但 stream_event 已经通过 delta 发送过了
	// 不再重复发送，避免内容重复
	return nil, false
}

// extractToolInputSummary 从工具 JSON 输入中提取摘要
func extractToolInputSummary(toolName, inputJSON string) string {
	if inputJSON == "" {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &args); err != nil {
		return inputJSON
	}
	// 根据工具类型提取关键字段
	switch toolName {
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			return cmd
		}
	case "Read":
		if p, ok := args["file_path"].(string); ok {
			return p
		}
	case "Write":
		if p, ok := args["file_path"].(string); ok {
			return p
		}
	case "Edit":
		if p, ok := args["file_path"].(string); ok {
			return p
		}
	case "Glob":
		if p, ok := args["pattern"].(string); ok {
			return p
		}
	case "Grep":
		if p, ok := args["pattern"].(string); ok {
			return p
		}
	}
	// 回退：返回整个 JSON
	return inputJSON
}
