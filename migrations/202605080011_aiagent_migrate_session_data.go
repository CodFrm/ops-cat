package migrations

import (
	"encoding/json"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// migration202605080011 把 conversations.session_data 的 cago SessionData blob
// 覆盖性地展开到 conversation_messages + conversations.{thread_id, state_values}，
// 然后清空 session_data 列。语义：LLM 视角即真相 — 此前由前端写入的
// conversation_messages 行被丢弃。
//
// 不变量：
//   - session_data 为空的会话不动
//   - session_data 反序列化失败的会话跳过（迁移日志面奇怪；脏数据不阻塞）
//   - wireResult 形状必须和 internal/aiagent/store.go:messageToRow 完全一致，
//     新行才能被新 gormStore.Load 路径回放
func migration202605080011() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080011",
		Migrate: func(tx *gorm.DB) error {
			var convs []conversation_entity.Conversation
			if err := tx.Where("session_data <> ''").Find(&convs).Error; err != nil {
				return err
			}
			for _, c := range convs {
				var data agent.SessionData
				if err := json.Unmarshal([]byte(c.SessionData), &data); err != nil {
					// 脏数据：跳过，不阻塞迁移。
					continue
				}
				// 删除既有 conversation_messages（前端写入的版本被丢弃，
				// session_data 优先）
				if err := tx.Where("conversation_id = ?", c.ID).
					Delete(&conversation_entity.Message{}).Error; err != nil {
					return err
				}
				rows := make([]*conversation_entity.Message, 0, len(data.Messages))
				now := time.Now().Unix()
				for i, m := range data.Messages {
					row := &conversation_entity.Message{
						ConversationID: c.ID,
						CagoID:         m.ID,
						ParentID:       m.ParentID,
						Kind:           string(m.Kind),
						Origin:         string(m.Origin),
						Role:           string(m.Role),
						Content:        m.Text,
						Persist:        m.Persist,
						SortOrder:      i,
						Createtime:     now,
					}
					if !m.Time.IsZero() {
						row.MsgTime = m.Time.Unix()
					}
					if len(m.Thinking) > 0 {
						b, _ := json.Marshal(m.Thinking)
						row.Thinking = string(b)
					}
					if m.ToolCall != nil {
						b, _ := json.Marshal(m.ToolCall)
						row.ToolCallJSON = string(b)
					}
					if m.ToolResult != nil {
						// ToolResult.Err 是 error 接口；按字符串编码（与
						// gormStore.messageToRow 完全一致，保证新行能被 Load 回放）
						type wireResult struct {
							Result   any           `json:"result,omitempty"`
							Err      string        `json:"err,omitempty"`
							Duration time.Duration `json:"duration,omitempty"`
						}
						w := wireResult{Result: m.ToolResult.Result, Duration: m.ToolResult.Duration}
						if m.ToolResult.Err != nil {
							w.Err = m.ToolResult.Err.Error()
						}
						b, _ := json.Marshal(w)
						row.ToolResultJSON = string(b)
					}
					if len(m.Raw) > 0 {
						row.Raw = string(m.Raw)
					}
					rows = append(rows, row)
				}
				if len(rows) > 0 {
					if err := tx.Create(&rows).Error; err != nil {
						return err
					}
				}
				stateValuesJSON := ""
				if len(data.State.Values) > 0 {
					b, _ := json.Marshal(data.State.Values)
					stateValuesJSON = string(b)
				}
				if err := tx.Model(&conversation_entity.Conversation{}).
					Where("id = ?", c.ID).
					Updates(map[string]any{
						"thread_id":    data.State.ThreadID,
						"state_values": stateValuesJSON,
						"session_data": "",
					}).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 不可逆 — 旧版本不再读 session_data。
			return nil
		},
	}
}
