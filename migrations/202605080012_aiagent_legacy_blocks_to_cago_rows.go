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
// 单行多 Block 消息（kind="/cago_id="）展开为 cago 形态的多行（text /
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
//     首次调用会把 cago_id=" 的行清掉），本迁移按"会话内有任一 legacy 行就重建"
//     的规则处理；如果存在已是 cago 形态的行（防御场景），保留它们并重排 sort_order
//   - 会话内行被全量重建，sort_order 重排为 0..N-1
//   - mentions 跟到展开后的 user text 行；token_usage 跟到 assistant 回合最后一行
//     cago 消息（与 cago event_bridge.lastAssistantMsgID 语义对齐）
//   - 中途中断的 tool（Status=running 且无 Content）只发 tool_call，不发 tool_result
//
// NOTE: 此迁移在 202605100001 之前运行（100001 才删除 v1 列）。行的写入全部走
// raw SQL，避免依赖已删除的 conversation_entity.Message v1 Go 字段。
func migration202605080012() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080012",
		Migrate: func(tx *gorm.DB) error {
			// 找所有还有 legacy 行的 conversation_id（kind="" AND cago_id="" 是 legacy 标志）
			rows, err := tx.Raw(`
				SELECT DISTINCT conversation_id
				FROM conversation_messages
				WHERE kind = '' AND cago_id = ''
			`).Rows()
			if err != nil {
				return err
			}
			var convIDs []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					_ = rows.Close()
					return err
				}
				convIDs = append(convIDs, id)
			}
			_ = rows.Close()
			if err := rows.Err(); err != nil {
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

// legacyDBRow is an in-memory representation of a conversation_messages row including
// v1 columns (which still exist in the DB when this migration runs, before
// 202605100001 drops them). Raw SQL is used to avoid depending on the Go struct v1
// fields that have been removed.
type legacyDBRow struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
	Blocks         string
	Mentions       string
	TokenUsage     string
	SortOrder      int
	Createtime     int64
	CagoID         string
	Kind           string
	Origin         string
	MsgTime        int64
}

// legacyOutRow is the in-memory shape of a row to be inserted by expandLegacyConversation.
type legacyOutRow struct {
	convID         int64
	cagoID         string
	kind           string
	origin         string
	role           string
	content        string
	blocks         string
	mentions       string
	tokenUsage     string
	toolCallJSON   string
	toolResultJSON string
	persist        bool
	sortOrder      int
	createtime     int64
	msgTime        int64
}

// expandLegacyConversation 重建一个会话的 conversation_messages：legacy 行按 Block
// 展开成 cago 行，cago 形态的行（防御场景）原样保留，最后整体重排 sort_order。
func expandLegacyConversation(tx *gorm.DB, convID int64) error {
	rows, err := tx.Raw(`
		SELECT id, conversation_id, role, COALESCE(content,''), COALESCE(blocks,''),
		       COALESCE(mentions,''), COALESCE(token_usage,''),
		       COALESCE(sort_order,0), COALESCE(createtime,0),
		       COALESCE(cago_id,''), COALESCE(kind,''), COALESCE(origin,''),
		       COALESCE(msg_time,0)
		FROM conversation_messages
		WHERE conversation_id = ?
		ORDER BY sort_order ASC, id ASC
	`, convID).Rows()
	if err != nil {
		return err
	}
	var dbRows []legacyDBRow
	for rows.Next() {
		var r legacyDBRow
		if err := rows.Scan(
			&r.ID, &r.ConversationID, &r.Role, &r.Content, &r.Blocks,
			&r.Mentions, &r.TokenUsage, &r.SortOrder, &r.Createtime,
			&r.CagoID, &r.Kind, &r.Origin, &r.MsgTime,
		); err != nil {
			_ = rows.Close()
			return err
		}
		dbRows = append(dbRows, r)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(dbRows) == 0 {
		return nil
	}

	now := time.Now().Unix()
	var out []legacyOutRow
	for _, r := range dbRows {
		isLegacy := r.Kind == "" && r.CagoID == ""
		if !isLegacy {
			// 已是 cago 形态 — 原样保留。
			// 注意：legacyOutRow 故意省略 thinking / raw / parent_id 三列。
			// 这三列只在 cago v1 SessionData 时代由 080011 写入，而 080011 在 cago
			// v2 升级时已被删除，所以全新装机本身就不会有这三列数据；已经应用过
			// 旧 080012 的生产 DB 由 gormigrate 按 ID 跳过本迁移，不受影响；
			// 紧随其后运行的 202605100001 又会把这三列从 schema 上彻底拿掉。
			out = append(out, legacyOutRow{
				convID:     r.ConversationID,
				cagoID:     r.CagoID,
				kind:       r.Kind,
				origin:     r.Origin,
				role:       r.Role,
				content:    r.Content,
				blocks:     r.Blocks,
				mentions:   r.Mentions,
				tokenUsage: r.TokenUsage,
				persist:    true,
				createtime: r.Createtime,
				msgTime:    r.MsgTime,
			})
			continue
		}
		expanded := expandLegacyRowRaw(convID, r, now)
		out = append(out, expanded...)
	}

	// 重排 sort_order 0..N-1
	for i := range out {
		out[i].sortOrder = i
	}

	// 全量替换该会话的所有行
	if err := tx.Exec(`DELETE FROM conversation_messages WHERE conversation_id = ?`, convID).Error; err != nil {
		return err
	}
	for _, m := range out {
		persist := 0
		if m.persist {
			persist = 1
		}
		if err := tx.Exec(`
			INSERT INTO conversation_messages
			(conversation_id, cago_id, kind, origin, role, content, blocks, mentions,
			 token_usage, tool_call_json, tool_result_json, persist, sort_order, createtime, msg_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, m.convID, m.cagoID, m.kind, m.origin, m.role, m.content, m.blocks, m.mentions,
			m.tokenUsage, m.toolCallJSON, m.toolResultJSON, persist, m.sortOrder, m.createtime, m.msgTime,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// expandLegacyRowRaw 把单条 legacy 消息行按 Blocks 边界展开为多条 cago 行（不依赖 entity v1 字段）。
//   - 用户行：始终产出 1 条 text-kind 行；mentions 保留在该行
//   - 助手行：每个 text block → 1 条 text 行；每个 tool block → 1 条 tool_call
//     行（+ 1 条 tool_result 行如果 block 有结果或错误）
//   - assistant 回合的 token_usage 跟到最后产出的那一行（lastAssistantMsgID 语义）
func expandLegacyRowRaw(convID int64, r legacyDBRow, now int64) []legacyOutRow {
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

	var out []legacyOutRow
	for i, b := range blocks {
		switch b.Type {
		case "text", "":
			out = append(out, legacyOutRow{
				convID:     convID,
				cagoID:     legacyCagoID(convID, r.ID, i, "text"),
				kind:       "text",
				origin:     legacyOrigin(role),
				role:       role,
				content:    b.Content,
				persist:    true,
				createtime: now,
				msgTime:    msgTime,
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
			out = append(out, legacyOutRow{
				convID:       convID,
				cagoID:       legacyCagoID(convID, r.ID, i, "call"),
				kind:         "tool_call",
				origin:       "model",
				role:         "assistant",
				toolCallJSON: string(callJSON),
				persist:      true,
				createtime:   now,
				msgTime:      msgTime,
			})
			// 只有"已完成（含 error）"且有内容/错误的 tool 才发 tool_result；
			// 中途被中断的 tool（status=running 且无 Content）渲染端展示为"running"占位。
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
				out = append(out, legacyOutRow{
					convID:         convID,
					cagoID:         legacyCagoID(convID, r.ID, i, "result"),
					kind:           "tool_result",
					origin:         "tool",
					role:           "tool",
					toolCallJSON:   string(callJSON), // cago 协议 + 渲染端 ID 配对依赖
					toolResultJSON: resultJSON,
					persist:        true,
					createtime:     now,
					msgTime:        msgTime,
				})
			}
		default:
			// 未知 block 类型：当 text 处理保留 Content
			out = append(out, legacyOutRow{
				convID:     convID,
				cagoID:     legacyCagoID(convID, r.ID, i, "text"),
				kind:       "text",
				origin:     legacyOrigin(role),
				role:       role,
				content:    b.Content,
				persist:    true,
				createtime: now,
				msgTime:    msgTime,
			})
		}
	}

	// mentions 留在用户行的 text 段（首段）
	if len(out) > 0 && r.Mentions != "" && role == "user" {
		out[0].mentions = r.Mentions
	}
	// token_usage 留在 assistant 回合最后产出那行（lastAssistantMsgID 语义）
	if len(out) > 0 && r.TokenUsage != "" && role == "assistant" {
		out[len(out)-1].tokenUsage = r.TokenUsage
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
