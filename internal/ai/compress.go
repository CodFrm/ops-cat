package ai

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

const (
	// CompressThreshold triggers compression when context usage exceeds this ratio (80%)
	CompressThreshold = 0.8
	// KeepRecentMessages number of recent messages to preserve during compression (excluding system)
	KeepRecentMessages = 6
)

// estimateTokens estimates token count for messages using a heuristic
func estimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateStringTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += estimateStringTokens(tc.Function.Name)
			total += estimateStringTokens(tc.Function.Arguments)
		}
	}
	return total
}

func estimateStringTokens(s string) int {
	charCount := utf8.RuneCountInString(s)
	byteCount := len(s)
	// Heuristic: CJK-heavy text ~1.5 chars/token, English ~4 chars/token
	// Compromise: (byteCount + charCount) / 5
	if charCount == 0 {
		return 0
	}
	return (byteCount + charCount) / 5
}

// needsCompression checks if context compression is needed
func needsCompression(messages []Message, contextWindow int) bool {
	if contextWindow <= 0 {
		return false
	}
	tokens := estimateTokens(messages)
	threshold := int(float64(contextWindow) * CompressThreshold)
	return tokens > threshold
}

// compressMessages compresses older messages using LLM summarization,
// preserving the system prompt and recent messages.
func compressMessages(ctx context.Context, provider Provider, messages []Message) []Message {
	if len(messages) <= KeepRecentMessages+1 {
		return messages
	}

	var systemMsg *Message
	var otherMsgs []Message
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			m := msg
			systemMsg = &m
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	if len(otherMsgs) <= KeepRecentMessages {
		return messages
	}

	oldMsgs := otherMsgs[:len(otherMsgs)-KeepRecentMessages]
	recentMsgs := otherMsgs[len(otherMsgs)-KeepRecentMessages:]

	conversationText := buildConversationSummary(oldMsgs)
	compressed := llmCompress(ctx, provider, conversationText)

	result := make([]Message, 0, 2+len(recentMsgs))
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}
	result = append(result, Message{
		Role:    RoleUser,
		Content: compressed,
	})
	result = append(result, Message{
		Role:    RoleAssistant,
		Content: "Understood, I have the context from our previous conversation. Let's continue.",
	})
	for _, msg := range recentMsgs {
		stripped := msg
		// 压缩时丢弃思考内容，节省 token；签名跟着一起清，不能脱离 thinking 单独存在。
		stripped.Thinking = ""
		stripped.ThinkingSignature = ""
		result = append(result, stripped)
	}
	return result
}

// buildConversationSummary converts messages into a structured text representation
func buildConversationSummary(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case RoleUser:
			sb.WriteString("User: ")
			sb.WriteString(truncateRunes(msg.Content, 500))
			sb.WriteString("\n")
		case RoleAssistant:
			sb.WriteString("Assistant: ")
			sb.WriteString(truncateRunes(msg.Content, 500))
			if len(msg.ToolCalls) > 0 {
				sb.WriteString(" [called tools: ")
				for i, tc := range msg.ToolCalls {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(tc.Function.Name)
				}
				sb.WriteString("]")
			}
			sb.WriteString("\n")
		case RoleTool:
			sb.WriteString("Tool result: ")
			sb.WriteString(truncateRunes(msg.Content, 200))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// llmCompress calls the LLM to produce a structured summary of the conversation
func llmCompress(ctx context.Context, provider Provider, conversationText string) string {
	prompt := fmt.Sprintf(`Compress the following conversation into a concise structured summary. Preserve:

1. **User's goals**: What the user is trying to accomplish
2. **Completed actions**: What operations were performed (tool calls, commands run, assets modified)
3. **Key findings**: Important results, errors encountered, or information discovered
4. **Current state**: Where the conversation left off, any pending tasks

Be specific — include asset names/IDs, command outputs that matter, and error details. Omit pleasantries and redundant tool call details.

Output the summary directly without headers or formatting instructions.

Conversation:
%s`, conversationText)

	messages := []Message{
		{Role: RoleUser, Content: prompt},
	}

	ch, err := provider.Chat(ctx, messages, nil)
	if err != nil {
		logger.Ctx(ctx).Warn("compress conversation failed", zap.Error(err))
		return "[Previous conversation context was compressed]\n\n" + truncateRunes(conversationText, 2000)
	}

	var result strings.Builder
	for event := range ch {
		if event.Type == "content" {
			result.WriteString(event.Content)
		}
		if event.Type == "error" {
			logger.Ctx(ctx).Warn("compress conversation error", zap.String("error", event.Error))
			return "[Previous conversation context was compressed]\n\n" + truncateRunes(conversationText, 2000)
		}
	}

	if result.Len() == 0 {
		return "[Previous conversation context was compressed]\n\n" + truncateRunes(conversationText, 2000)
	}

	return "[Summary of previous conversation]\n\n" + result.String()
}
