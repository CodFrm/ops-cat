package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// buildDisplayMessages 把按 sort_order 排好的 cago-shape conversation_messages 行
// 聚合成前端展示用的 ConversationDisplayMessage 列表：
//   - 一条 user text 行 → 一条 user 显示消息
//   - 连续的 model/tool 行（assistant 一回合内的 text + tool_call + tool_result）
//     → 一条 assistant 显示消息，多个 Blocks
//   - tool_result 按 ToolCallID 对应到本回合内的 tool_call，不依赖行序
//
// 这是 loadConversationDisplayMessages 里被抽出来的纯函数，方便单测。

func mkText(cagoID, role, origin, content string) *conversation_entity.Message {
	return &conversation_entity.Message{
		CagoID: cagoID, Kind: "text", Role: role, Origin: origin, Content: content, Persist: true,
	}
}

func mkToolCall(cagoID, toolID, name, args string) *conversation_entity.Message {
	tcJSON, _ := json.Marshal(struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}{ID: toolID, Name: name, Args: json.RawMessage(args)})
	return &conversation_entity.Message{
		CagoID: cagoID, Kind: "tool_call", Role: "assistant", Origin: "model",
		ToolCallJSON: string(tcJSON), Persist: true,
	}
}

// mkToolResult 仿照 cago 协议：tool_result message 同时带 ToolCall（含原 call ID
// 用于配对）与 ToolResult；store.messageToRow 会把两段都写到同一行的两个 JSON 列。
func mkToolResult(cagoID, toolID, result string) *conversation_entity.Message {
	tcJSON, _ := json.Marshal(struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}{ID: toolID, Args: json.RawMessage("{}")})
	resJSON, _ := json.Marshal(struct {
		Result any `json:"result,omitempty"`
	}{Result: result})
	return &conversation_entity.Message{
		CagoID: cagoID, Kind: "tool_result", Role: "tool", Origin: "tool",
		ToolCallJSON: string(tcJSON), ToolResultJSON: string(resJSON), Persist: true,
	}
}

// 一回合 = 一气泡：assistant 的 text + tool_call + tool_result + text 应聚合为
// 一条 ConversationDisplayMessage，而不是多个分裂气泡。
func TestBuildDisplayMessages_AssistantTurnAggregatesBlocks(t *testing.T) {
	rows := []*conversation_entity.Message{
		mkText("u1", "user", "user", "hi"),
		mkText("a1", "assistant", "model", "let me check"),
		mkToolCall("a2", "call-1", "list_assets", `{"x":1}`),
		mkToolResult("a3", "call-1", "ok-result"),
		mkText("a4", "assistant", "model", "done"),
	}
	out := buildDisplayMessages(rows)

	require.Len(t, out, 2, "user + 一个 assistant 回合 = 2 条显示消息")

	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "hi", out[0].Content)

	assert.Equal(t, "assistant", out[1].Role)
	require.Len(t, out[1].Blocks, 3, "回合内 text + tool + text 应聚合为 3 个 Block")
	assert.Equal(t, "text", out[1].Blocks[0].Type)
	assert.Equal(t, "let me check", out[1].Blocks[0].Content)
	assert.Equal(t, "tool", out[1].Blocks[1].Type)
	assert.Equal(t, "list_assets", out[1].Blocks[1].ToolName)
	assert.Equal(t, "call-1", out[1].Blocks[1].ToolCallID)
	assert.Equal(t, "ok-result", out[1].Blocks[1].Content)
	assert.Equal(t, "completed", out[1].Blocks[1].Status)
	assert.Equal(t, "text", out[1].Blocks[2].Type)
	assert.Equal(t, "done", out[1].Blocks[2].Content)
}

// 多 tool 回合：cago 顺序是 tool_call_A, tool_call_B, tool_result_A, tool_result_B
// （或乱序）；按 i+1 配对会把 B 的 result 错配给 A、把 A 的真 result 丢成"孤儿
// tool_result"。新逻辑必须按 ToolCallID 配对，结果严格归位。
func TestBuildDisplayMessages_MultiToolTurnPairsByID(t *testing.T) {
	rows := []*conversation_entity.Message{
		mkText("u1", "user", "user", "go"),
		mkText("a1", "assistant", "model", "running both"),
		mkToolCall("a2", "call-A", "tool_a", `{}`),
		mkToolCall("a3", "call-B", "tool_b", `{}`),
		mkToolResult("a4", "call-B", "B-result"), // 注意先 B 后 A，模拟错位
		mkToolResult("a5", "call-A", "A-result"),
		mkText("a6", "assistant", "model", "all done"),
	}

	out := buildDisplayMessages(rows)
	require.Len(t, out, 2)

	asst := out[1]
	require.Len(t, asst.Blocks, 4, "text + tool A + tool B + text")
	assert.Equal(t, "text", asst.Blocks[0].Type)

	// Block[1] 是 call-A 的 tool block，必须配到 A-result
	assert.Equal(t, "tool", asst.Blocks[1].Type)
	assert.Equal(t, "call-A", asst.Blocks[1].ToolCallID)
	assert.Equal(t, "A-result", asst.Blocks[1].Content, "call-A 的内容必须是 A-result，不能错配 B-result")

	// Block[2] 是 call-B 的 tool block，必须配到 B-result
	assert.Equal(t, "tool", asst.Blocks[2].Type)
	assert.Equal(t, "call-B", asst.Blocks[2].ToolCallID)
	assert.Equal(t, "B-result", asst.Blocks[2].Content)

	assert.Equal(t, "text", asst.Blocks[3].Type)
	assert.Equal(t, "all done", asst.Blocks[3].Content)
}

// 多回合：user → assistant turn1 → user → assistant turn2 → 4 条显示消息。
// 不能把回合 1 和回合 2 错并到一起。
func TestBuildDisplayMessages_TurnsBoundedByUser(t *testing.T) {
	rows := []*conversation_entity.Message{
		mkText("u1", "user", "user", "q1"),
		mkText("a1", "assistant", "model", "r1"),
		mkText("u2", "user", "user", "q2"),
		mkText("a2", "assistant", "model", "r2"),
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 4)
	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "q1", out[0].Content)
	assert.Equal(t, "assistant", out[1].Role)
	assert.Equal(t, "r1", out[1].Blocks[0].Content)
	assert.Equal(t, "user", out[2].Role)
	assert.Equal(t, "q2", out[2].Content)
	assert.Equal(t, "assistant", out[3].Role)
	assert.Equal(t, "r2", out[3].Blocks[0].Content)
}

// 没匹配上 tool_result 的 tool_call（生成中被 Stop 截断）→ 显示成 running 占位，
// 不能丢掉这个 tool block。
func TestBuildDisplayMessages_OrphanToolCallShowsRunning(t *testing.T) {
	rows := []*conversation_entity.Message{
		mkText("u1", "user", "user", "go"),
		mkText("a1", "assistant", "model", "running"),
		mkToolCall("a2", "call-1", "exec", `{}`),
		// 没有 tool_result
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 2)
	require.Len(t, out[1].Blocks, 2)
	assert.Equal(t, "tool", out[1].Blocks[1].Type)
	assert.Equal(t, "running", out[1].Blocks[1].Status, "未配对的 tool_call 应显示为 running 而不是 completed")
	assert.Empty(t, out[1].Blocks[1].Content)
}

// system / hook_context / steering / compaction_summary 等非展示 kind 不能产出气泡。
func TestBuildDisplayMessages_DropsNonDisplayKinds(t *testing.T) {
	rows := []*conversation_entity.Message{
		{CagoID: "s1", Kind: "system", Role: "system", Origin: "framework", Content: "system prompt", Persist: true},
		mkText("u1", "user", "user", "hi"),
		{CagoID: "h1", Kind: "hook_context", Role: "system", Origin: "hook", Content: "hook ctx", Persist: true},
		mkText("a1", "assistant", "model", "hello"),
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 2)
	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "assistant", out[1].Role)
}

// mentions 在 user 行 → user 显示消息上；token_usage 在 assistant 回合任一行 →
// 该 assistant 显示消息上（cago bridge 把它写到回合最后一行，但为容错把"回合内
// 任意一行带 token_usage 都聚合上去"）。
func TestBuildDisplayMessages_MentionsAndTokenUsage(t *testing.T) {
	mentions, _ := json.Marshal([]conversation_entity.MentionRef{{AssetID: 7, Name: "srv"}})
	usage, _ := json.Marshal(conversation_entity.TokenUsage{InputTokens: 10, OutputTokens: 20})

	userRow := mkText("u1", "user", "user", "hi @srv")
	userRow.Mentions = string(mentions)
	finalText := mkText("a3", "assistant", "model", "done")
	finalText.TokenUsage = string(usage)

	rows := []*conversation_entity.Message{
		userRow,
		mkText("a1", "assistant", "model", "let me"),
		mkToolCall("a2", "call-1", "exec", `{}`),
		finalText,
	}
	out := buildDisplayMessages(rows)
	require.Len(t, out, 2)

	require.Len(t, out[0].Mentions, 1)
	assert.Equal(t, "srv", out[0].Mentions[0].Name)

	require.NotNil(t, out[1].TokenUsage)
	assert.Equal(t, 10, out[1].TokenUsage.InputTokens)
	assert.Equal(t, 20, out[1].TokenUsage.OutputTokens)
}
