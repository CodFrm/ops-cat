package ai

import (
	"encoding/json"

	"github.com/cago-frame/agents/agent"
)

// splitForReplay 把 OpsKat 的消息列表切分为「需要灌进 cago Conversation 的历史」+「本轮要发给 LLM 的最后一条用户文本」。
//
// 调用约定：messages 来自 SendAIMessage 入参，包含全部历史 + 末尾的新 user message。
// 如果末尾不是 user message（边界场景，比如纯历史回放），则全部归入 history、lastUserText = ""。
func splitForReplay(messages []Message) (history []Message, lastUserText string) {
	if len(messages) == 0 {
		return nil, ""
	}
	last := messages[len(messages)-1]
	if last.Role == RoleUser {
		return messages[:len(messages)-1], last.Content
	}
	return messages, ""
}

// convertToCagoMessages 把 OpsKat 的 []Message 转换为 cago agent.Message 切片。
//
// 转换规则：
//   - user        → agent.Message{Role: RoleUser,    Content: [TextBlock]}
//   - assistant   → Content 依次包含 ThinkingBlock?（若 Thinking 非空）+ TextBlock?（若 Content 非空）
//   - 每个 ToolCall 转 ToolUseBlock（State=ToolUseReady，Input 来自 Arguments JSON）
//   - tool        → Content = [ToolResultBlock{ToolUseID, Content:[TextBlock]}]
//
// ReasoningContent（OpenAI/DeepSeek 风格）目前与 Thinking 合并处理——cago 的 ThinkingBlock 兼容两者。
// ThinkingBlock.Signature 仅在 Anthropic 流式 thinking 模式必填，DB schema 当前未持久化此字段；
// 重放时缺 Signature 可能导致 Anthropic 拒绝继续多轮 thinking——这是已知 follow-up（见 plan 风险章节）。
func convertToCagoMessages(messages []Message) []agent.Message {
	out := make([]agent.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case RoleUser:
			out = append(out, agent.Message{
				Role:    agent.RoleUser,
				Content: []agent.ContentBlock{&agent.TextBlock{Text: m.Content}},
			})

		case RoleAssistant:
			var blocks []agent.ContentBlock
			thinking := m.Thinking
			if thinking == "" {
				thinking = m.ReasoningContent
			}
			if thinking != "" {
				blocks = append(blocks, &agent.ThinkingBlock{Text: thinking})
			}
			if m.Content != "" {
				blocks = append(blocks, &agent.TextBlock{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
						// 解析失败时仍记录 tool_use（State=ToolUseMalformed）让 cago 可识别
						blocks = append(blocks, &agent.ToolUseBlock{
							ID:      tc.ID,
							Name:    tc.Function.Name,
							RawArgs: tc.Function.Arguments,
							State:   agent.ToolUseMalformed,
						})
						continue
					}
				}
				blocks = append(blocks, &agent.ToolUseBlock{
					ID:      tc.ID,
					Name:    tc.Function.Name,
					Input:   input,
					RawArgs: tc.Function.Arguments,
					State:   agent.ToolUseReady,
				})
			}
			out = append(out, agent.Message{
				Role:    agent.RoleAssistant,
				Content: blocks,
			})

		case RoleTool:
			out = append(out, agent.Message{
				Role: agent.RoleTool,
				Content: []agent.ContentBlock{&agent.ToolResultBlock{
					ToolUseID: m.ToolCallID,
					Content:   []agent.ContentBlock{&agent.TextBlock{Text: m.Content}},
				}},
			})

		case RoleSystem:
			// cago 不在 Conversation 里存 system，由 agent.System(...) 配置——重放时跳过。
		}
	}
	return out
}
