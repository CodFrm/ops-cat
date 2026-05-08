package migrations

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// migration202605080012 把前 cago 单源时代由前端 SaveConversationMessages 写入的
// 单行多 Block 消息（kind=”/cago_id=”）展开为 cago 形态的多行（text /
// tool_call / tool_result）。202605080011 已经把 conversations.session_data 非空
// 的会话覆盖过；本迁移只动剩下的 legacy 行——纯前 cago 时代会话的"一回合一行
// 多 Block"老格式。
//
// 渲染端（loadConversationDisplayMessages）改回回合聚合后，迁出的多行 cago 行
// 会被合并回一个气泡，与用户记忆中的渲染形态一致。tool 调用 ID 是迁移生成的
// 合成 ID（来源 ContentBlock.ToolCallID 或 fallback 到 row_id+block_idx），
// 渲染端按 tool_call_id 配对 tool_result 不依赖行序。
//
// 不变量：
//   - 同一会话内同时存在 legacy 与 cago 行的混合不合法（cago era 的 gormStore.Save
//     首次调用会把 cago_id=” 的行清掉），本迁移按"会话内有任一 legacy 行就重建"
//     的规则处理；如果存在已是 cago 形态的行（防御场景），保留它们并重排 sort_order
//   - 会话内行被全量重建，sort_order 重排为 0..N-1
//   - mentions 跟到展开后的 user text 行；token_usage 跟到 assistant 回合最后一行
//     cago 消息（与 cago event_bridge.lastAssistantMsgID 语义对齐）
//   - 中途中断的 tool（Status=running 且无 Content）只发 tool_call，不发 tool_result
func migration202605080012() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080012",
		Migrate: func(tx *gorm.DB) error {
			// 找所有还有 legacy 行的 conversation_id
			var convIDs []int64
			if err := tx.Model(&conversation_entity.Message{}).
				Where("kind = '' AND cago_id = ''").
				Distinct("conversation_id").
				Pluck("conversation_id", &convIDs).Error; err != nil {
				return err
			}
			for _, convID := range convIDs {
				if err := expandLegacyConversation(tx, convID); err != nil {
					return fmt.Errorf("expand legacy conv %d: %w", convID, err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 不可逆 — 旧渲染路径已下线
			return nil
		},
	}
}

// expandLegacyConversation 重建一个会话的 conversation_messages：legacy 行按 Block
// 展开成 cago 行，cago 形态的行（防御场景）原样保留，最后整体重排 sort_order。
func expandLegacyConversation(tx *gorm.DB, convID int64) error {
	var rows []conversation_entity.Message
	if err := tx.Where("conversation_id = ?", convID).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().Unix()
	out := make([]*conversation_entity.Message, 0, len(rows))
	for _, r := range rows {
		isLegacy := r.Kind == "" && r.CagoID == ""
		if !isLegacy {
			cp := r
			cp.ID = 0 // 让 gorm 重新分配，避免 INSERT 主键冲突
			out = append(out, &cp)
			continue
		}
		expanded := expandLegacyRow(convID, r, now)
		out = append(out, expanded...)
	}

	// 重排 sort_order 0..N-1
	for i, m := range out {
		m.SortOrder = i
	}

	// 全量替换该会话的所有行
	if err := tx.Where("conversation_id = ?", convID).
		Delete(&conversation_entity.Message{}).Error; err != nil {
		return err
	}
	if len(out) > 0 {
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
	}
	return nil
}

// expandLegacyRow 把单条 legacy 消息行按 Blocks 边界展开为多条 cago 行。
//   - 用户行：始终产出 1 条 text-kind 行；mentions 保留在该行
//   - 助手行：每个 text block → 1 条 text 行；每个 tool block → 1 条 tool_call
//     行（+ 1 条 tool_result 行如果 block 有结果或错误）
//   - assistant 回合的 token_usage 跟到最后产出的那一行（lastAssistantMsgID 语义）
func expandLegacyRow(convID int64, r conversation_entity.Message, now int64) []*conversation_entity.Message {
	msgTime := r.MsgTime
	if msgTime == 0 {
		msgTime = r.Createtime
	}
	role := r.Role
	if role == "" {
		role = "assistant"
	}

	// 解析 Blocks；空/损坏走 Content 兜底
	var blocks []conversation_entity.ContentBlock
	if r.Blocks != "" {
		_ = json.Unmarshal([]byte(r.Blocks), &blocks)
	}
	if len(blocks) == 0 && r.Content != "" {
		blocks = []conversation_entity.ContentBlock{{Type: "text", Content: r.Content}}
	}
	// 空内容空 blocks → 整行丢弃（不产出 cago 行）
	if len(blocks) == 0 {
		return nil
	}

	out := make([]*conversation_entity.Message, 0, len(blocks)+1)
	for i, b := range blocks {
		switch b.Type {
		case "text", "":
			out = append(out, &conversation_entity.Message{
				ConversationID: convID,
				CagoID:         legacyCagoID(convID, r.ID, i, "text"),
				Kind:           "text",
				Origin:         legacyOrigin(role),
				Role:           role,
				Content:        b.Content,
				Persist:        true,
				Createtime:     now,
				MsgTime:        msgTime,
			})
		case "tool":
			toolCallID := b.ToolCallID
			if toolCallID == "" {
				// 兜底：合成一个 ID，确保渲染端按 ID 配对仍然可行
				toolCallID = fmt.Sprintf("legacy-tool-%d-%d", r.ID, i)
			}
			callJSON, _ := json.Marshal(struct {
				ID   string          `json:"id"`
				Name string          `json:"name"`
				Args json.RawMessage `json:"args"`
			}{
				ID:   toolCallID,
				Name: b.ToolName,
				Args: legacyToolArgs(b.ToolInput),
			})
			out = append(out, &conversation_entity.Message{
				ConversationID: convID,
				CagoID:         legacyCagoID(convID, r.ID, i, "call"),
				Kind:           "tool_call",
				Origin:         "model",
				Role:           "assistant",
				ToolCallJSON:   string(callJSON),
				Persist:        true,
				Createtime:     now,
				MsgTime:        msgTime,
			})
			// 只有"已完成（含 error）"且有内容/错误的 tool 才发 tool_result；
			// 中途被中断的 tool（status=running 且无 Content）渲染端展示为"running"占位。
			// cago 的 tool_result message 同时带 ToolCall（含 call ID）与 ToolResult，
			// 渲染端按 ToolCallJSON.id 把 result 配回 call；这里也保持一致。
			var resultJSON string
			if b.Status == "error" && b.Content != "" {
				j, _ := json.Marshal(struct {
					Err string `json:"err,omitempty"`
				}{Err: b.Content})
				resultJSON = string(j)
			} else if b.Status != "running" && b.Content != "" {
				j, _ := json.Marshal(struct {
					Result any `json:"result,omitempty"`
				}{Result: b.Content})
				resultJSON = string(j)
			}
			if resultJSON != "" {
				out = append(out, &conversation_entity.Message{
					ConversationID: convID,
					CagoID:         legacyCagoID(convID, r.ID, i, "result"),
					Kind:           "tool_result",
					Origin:         "tool",
					Role:           "tool",
					ToolCallJSON:   string(callJSON), // cago 协议 + 渲染端 ID 配对依赖
					ToolResultJSON: resultJSON,
					Persist:        true,
					Createtime:     now,
					MsgTime:        msgTime,
				})
			}
		default:
			// 未知 block 类型：当 text 处理保留 Content
			out = append(out, &conversation_entity.Message{
				ConversationID: convID,
				CagoID:         legacyCagoID(convID, r.ID, i, "text"),
				Kind:           "text",
				Origin:         legacyOrigin(role),
				Role:           role,
				Content:        b.Content,
				Persist:        true,
				Createtime:     now,
				MsgTime:        msgTime,
			})
		}
	}

	// mentions 留在用户行的 text 段（首段）
	if len(out) > 0 && r.Mentions != "" && role == "user" {
		out[0].Mentions = r.Mentions
	}
	// token_usage 留在 assistant 回合最后产出那行（lastAssistantMsgID 语义）
	if len(out) > 0 && r.TokenUsage != "" && role == "assistant" {
		out[len(out)-1].TokenUsage = r.TokenUsage
	}
	return out
}

// legacyCagoID 给 legacy 迁出行铸合成 cago_id。格式："legacy:<conv>:<row>:<block>:<tag>"。
// 不冲突即可，前端只用作 React key、repo 用作 upsert 自然键。
func legacyCagoID(convID, rowID int64, blockIdx int, tag string) string {
	return fmt.Sprintf("legacy:%d:%d:%d:%s", convID, rowID, blockIdx, tag)
}

// legacyOrigin 把 legacy role 字符串映射回 cago Origin。
func legacyOrigin(role string) string {
	switch role {
	case "user":
		return "user"
	case "tool":
		return "tool"
	default:
		return "model"
	}
}

// legacyToolArgs 把 legacy ToolInput 字符串解析成 args JSON；空或不合法 JSON 时
// 兜底为 "{}"，避免 cago 路径下 json.Unmarshal 解析 args 时炸。
func legacyToolArgs(toolInput string) json.RawMessage {
	if toolInput == "" {
		return json.RawMessage("{}")
	}
	var probe any
	if err := json.Unmarshal([]byte(toolInput), &probe); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(toolInput)
}
