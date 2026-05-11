package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// TestBuildDisplayMessages_MultiRoundMergesIntoSingleAssistant 回归 bug2a：
// 刷新后用户看到两个 assistant 气泡（一个带 tool_use 无结果、一个是最终 text 回复）
// 而不是一个聚合的 assistant 气泡——根因是 cago 每次 LLM 调用记一条 assistant
// 消息（text+tool_use 是第 N 条，最终格式化文本是第 N+2 条，中间夹一个 user-role
// tool_result）。buildDisplayMessages 没合并这两条 assistant，前端就渲染成 2 个气泡。
//
// 现场流式时 aiStore 把所有 tool/text 都 append 到「最后一条 assistant」的
// blocks，体验上是 1 个气泡。历史回放必须对齐这一点：连续 assistant 间只夹
// tool_result-only user 行的，要折叠成 1 条。
func TestBuildDisplayMessages_MultiRoundMergesIntoSingleAssistant(t *testing.T) {
	rows := []*conversation_entity.Message{
		// 0: user 提问
		{
			ID:             1,
			ConversationID: 1,
			Role:           "user",
			Blocks:         mustJSON(t, []map[string]any{{"type": "text", "text": "@local-docker 检查容器"}}),
			SortOrder:      0,
			Createtime:     1,
		},
		// 1: assistant 回 text + tool_use（usage = round1）
		{
			ID:             2,
			ConversationID: 1,
			Role:           "assistant",
			Blocks: mustJSON(t, []map[string]any{
				{"type": "text", "text": "好的，我来检查"},
				{"type": "tool_use", "id": "call_1", "name": "run_command", "input": map[string]any{"asset_id": float64(37), "command": "docker ps"}},
			}),
			TokenUsage: mustJSON(t, map[string]any{"inputTokens": 100, "outputTokens": 20}),
			SortOrder:  1,
			Createtime: 2,
		},
		// 2: tool-role 只含 tool_result（cago v2 把 tool turn 的 role 定义成 "tool"，
		// 不是 "user"——provider/types.go:12 RoleTool = "tool"）。
		{
			ID:             3,
			ConversationID: 1,
			Role:           "tool",
			Blocks: mustJSON(t, []map[string]any{
				{"type": "tool_result", "tool_use_id": "call_1", "is_error": false,
					"content": []map[string]any{{"type": "text", "text": "CONTAINER ID  NAME\nabc123  astrbot"}}},
			}),
			SortOrder:  2,
			Createtime: 3,
		},
		// 3: assistant 给最终格式化回复（usage = round2）
		{
			ID:             4,
			ConversationID: 1,
			Role:           "assistant",
			Blocks:         mustJSON(t, []map[string]any{{"type": "text", "text": "以下是容器列表："}}),
			TokenUsage:     mustJSON(t, map[string]any{"inputTokens": 150, "outputTokens": 80}),
			SortOrder:      3,
			Createtime:     4,
		},
	}

	out := buildDisplayMessages(rows)

	// 期望：tool_result-only user 被吃掉 + 两条 assistant 合并 → out 只有 2 条。
	require.Len(t, out, 2, "连续 assistant 中间只夹 tool_result-only user，必须合并成 1 条")
	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "assistant", out[1].Role)

	// 合并后的 assistant 应该顺序持有：text → tool(含 result) → text2。
	asst := out[1]
	require.GreaterOrEqual(t, len(asst.Blocks), 3, "合并后的 blocks: [text, tool, text2]")

	// 第一块 text。
	assert.Equal(t, "text", asst.Blocks[0].Type)
	assert.Contains(t, asst.Blocks[0].Content, "好的")

	// 中间 tool 块要挂上 result 内容 + Status=completed。
	var toolBlock *conversation_entity.ContentBlock
	for i := range asst.Blocks {
		if asst.Blocks[i].Type == "tool" {
			toolBlock = &asst.Blocks[i]
			break
		}
	}
	require.NotNil(t, toolBlock, "合并后必须有 tool block")
	assert.Equal(t, "run_command", toolBlock.ToolName)
	assert.Equal(t, "call_1", toolBlock.ToolCallID)
	assert.Equal(t, "completed", toolBlock.Status)
	assert.Contains(t, toolBlock.Content, "astrbot", "tool result 内容应回贴到 tool block")

	// 最后一块 text 是最终格式化回复。
	lastText := asst.Blocks[len(asst.Blocks)-1]
	assert.Equal(t, "text", lastText.Type)
	assert.Contains(t, lastText.Content, "以下是容器列表")

	// Content 也要拼接两轮文本，便于复制按钮取整条。
	assert.Contains(t, asst.Content, "好的")
	assert.Contains(t, asst.Content, "以下是容器列表")

	// tokenUsage 必须累加，对齐 live 流式的 usage handler（sum across rounds）。
	require.NotNil(t, asst.TokenUsage, "合并后必须有 tokenUsage")
	assert.Equal(t, 250, asst.TokenUsage.InputTokens, "100+150")
	assert.Equal(t, 100, asst.TokenUsage.OutputTokens, "20+80")
}

// TestBuildDisplayMessages_AssistantSeparatedByRealUserMsgNotMerged 边界：
// 两条 assistant 中间夹的是真正的 user 提问（不是 tool_result），不能合并。
func TestBuildDisplayMessages_AssistantSeparatedByRealUserMsgNotMerged(t *testing.T) {
	rows := []*conversation_entity.Message{
		{ID: 1, Role: "user", Blocks: mustJSON(t, []map[string]any{{"type": "text", "text": "q1"}}), SortOrder: 0},
		{ID: 2, Role: "assistant", Blocks: mustJSON(t, []map[string]any{{"type": "text", "text": "a1"}}), SortOrder: 1},
		{ID: 3, Role: "user", Blocks: mustJSON(t, []map[string]any{{"type": "text", "text": "q2"}}), SortOrder: 2},
		{ID: 4, Role: "assistant", Blocks: mustJSON(t, []map[string]any{{"type": "text", "text": "a2"}}), SortOrder: 3},
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 4, "user/assistant/user/assistant 必须保留 4 条")
}

// TestBuildDisplayMessages_PartialReasonAndDetailPropagate 验证流中途出错后，
// 重新进入会话时，已输出的内容 + PartialReason/PartialDetail 都能透传给前端，
// 不会再像旧实现那样把"出错"状态在加载路径上丢掉。
func TestBuildDisplayMessages_PartialReasonAndDetailPropagate(t *testing.T) {
	rows := []*conversation_entity.Message{
		{
			ID:         1,
			Role:       "user",
			Blocks:     mustJSON(t, []map[string]any{{"type": "text", "text": "explain X"}}),
			SortOrder:  0,
			Createtime: 1,
		},
		{
			ID:   2,
			Role: "assistant",
			Blocks: mustJSON(t, []map[string]any{
				{"type": "text", "text": "Let me start by"},
			}),
			PartialReason: "errored",
			PartialDetail: "context deadline exceeded",
			SortOrder:     1,
			Createtime:    2,
		},
	}

	out := buildDisplayMessages(rows)
	require.Len(t, out, 2)
	asst := out[1]
	assert.Equal(t, "assistant", asst.Role)
	assert.Equal(t, "errored", asst.PartialReason, "PartialReason must reach the frontend so the bubble can render the error state")
	assert.Equal(t, "context deadline exceeded", asst.PartialDetail, "PartialDetail must round-trip so the error message survives reload")
	require.Len(t, asst.Blocks, 1, "已输出的内容必须保留")
	assert.Contains(t, asst.Blocks[0].Content, "Let me start by")
}

// TestBuildDisplayMessages_MultiRoundMergedTakesLastPartialState 验证多轮 assistant
// 折叠时，partial 状态取**最后一条**——前面几条必然是已 finalized（cago 不会在
// errored/canceled 之后继续下一轮），合并后整组的状态应反映最末那条的命运。
func TestBuildDisplayMessages_MultiRoundMergedTakesLastPartialState(t *testing.T) {
	rows := []*conversation_entity.Message{
		{ID: 1, Role: "user", Blocks: mustJSON(t, []map[string]any{{"type": "text", "text": "q"}}), SortOrder: 0},
		// 第一轮 assistant：正常 finalized。
		{
			ID:   2,
			Role: "assistant",
			Blocks: mustJSON(t, []map[string]any{
				{"type": "tool_use", "id": "c1", "name": "echo", "input": map[string]any{}},
			}),
			SortOrder: 1,
		},
		// tool_result-only。
		{
			ID:   3,
			Role: "tool",
			Blocks: mustJSON(t, []map[string]any{
				{"type": "tool_result", "tool_use_id": "c1", "content": []map[string]any{{"type": "text", "text": "ok"}}},
			}),
			SortOrder: 2,
		},
		// 第二轮 assistant：流中途出错。
		{
			ID:            4,
			Role:          "assistant",
			Blocks:        mustJSON(t, []map[string]any{{"type": "text", "text": "partial reply"}}),
			PartialReason: "errored",
			PartialDetail: "upstream 503",
			SortOrder:     3,
		},
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 2, "两轮 assistant 中间只夹 tool_result，应折叠为 1 条")
	asst := out[1]
	assert.Equal(t, "errored", asst.PartialReason, "合并后必须反映最后一条的 partial 状态")
	assert.Equal(t, "upstream 503", asst.PartialDetail)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
