package conversation_repo

import (
	"context"
	"errors"
	"time"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"

	"github.com/cago-frame/cago/database/db"
	"gorm.io/gorm"
)

// ConversationRepo 会话数据访问接口
type ConversationRepo interface {
	Find(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	List(ctx context.Context) ([]*conversation_entity.Conversation, error)
	Create(ctx context.Context, conv *conversation_entity.Conversation) error
	Update(ctx context.Context, conv *conversation_entity.Conversation) error
	UpdateTitle(ctx context.Context, id int64, title string, updatetime int64) error
	UpdateWorkDir(ctx context.Context, id int64, workDir string, updatetime int64) error
	Delete(ctx context.Context, id int64) error

	// 消息操作
	ListMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
	DeleteMessages(ctx context.Context, conversationID int64) error

	// UpsertMessagesByCagoID 按 (conversation_id, cago_id) 自然键行级 upsert：
	// - 删除当前快照里没有的旧行（history rewrite/compact 场景的收敛）
	// - 已存在的 cago_id 行更新 cago 字段 + role/content/sort_order；不动
	//   mentions/token_usage 这两个由 System pending 缓存写的扩展列
	// - 新行直接 Create
	// 整体在事务内完成。
	UpsertMessagesByCagoID(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error

	// UpdateState 写入 cago Session 的 thread_id / state_values，并刷新 updatetime。
	// stateValuesJSON 已是序列化好的 JSON 字符串（service 层负责 marshal）；
	// 空字符串视为清空（GetStateValues 返回 nil）。
	UpdateState(ctx context.Context, conversationID int64, threadID, stateValuesJSON string) error

	// UpdateMessageTokenUsage 把序列化好的 token_usage JSON 写到指定 (conversationID, cagoID) 行。
	// 由 gormStore.Save drain System.pendingUsage 时调用；
	// 写不存在的 cago_id 是 no-op（行未被 upsert 创建则跳过）。
	UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error
}

var defaultConversation ConversationRepo

// Conversation 获取 ConversationRepo 实例
func Conversation() ConversationRepo {
	return defaultConversation
}

// RegisterConversation 注册 ConversationRepo 实现
func RegisterConversation(i ConversationRepo) {
	defaultConversation = i
}

// conversationRepo 默认实现
type conversationRepo struct{}

// NewConversation 创建默认实现
func NewConversation() ConversationRepo {
	return &conversationRepo{}
}

func (r *conversationRepo) Find(ctx context.Context, id int64) (*conversation_entity.Conversation, error) {
	var conv conversation_entity.Conversation
	if err := db.Ctx(ctx).Where("id = ? AND status = ?", id, conversation_entity.StatusActive).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

func (r *conversationRepo) List(ctx context.Context) ([]*conversation_entity.Conversation, error) {
	var convs []*conversation_entity.Conversation
	if err := db.Ctx(ctx).Where("status = ?", conversation_entity.StatusActive).
		Order("updatetime DESC").Find(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

func (r *conversationRepo) Create(ctx context.Context, conv *conversation_entity.Conversation) error {
	return db.Ctx(ctx).Create(conv).Error
}

func (r *conversationRepo) Update(ctx context.Context, conv *conversation_entity.Conversation) error {
	return db.Ctx(ctx).Save(conv).Error
}

func (r *conversationRepo) UpdateTitle(ctx context.Context, id int64, title string, updatetime int64) error {
	result := db.Ctx(ctx).
		Model(&conversation_entity.Conversation{}).
		Where("id = ? AND status = ?", id, conversation_entity.StatusActive).
		Updates(map[string]any{
			"title":      title,
			"updatetime": updatetime,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepo) UpdateWorkDir(ctx context.Context, id int64, workDir string, updatetime int64) error {
	result := db.Ctx(ctx).
		Model(&conversation_entity.Conversation{}).
		Where("id = ? AND status = ?", id, conversation_entity.StatusActive).
		Updates(map[string]any{
			"work_dir":   workDir,
			"updatetime": updatetime,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Model(&conversation_entity.Conversation{}).Where("id = ?", id).
		Update("status", conversation_entity.StatusDeleted).Error
}

func (r *conversationRepo) ListMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error) {
	var msgs []*conversation_entity.Message
	if err := db.Ctx(ctx).Where("conversation_id = ?", conversationID).
		Order("sort_order ASC").Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

func (r *conversationRepo) DeleteMessages(ctx context.Context, conversationID int64) error {
	return db.Ctx(ctx).Where("conversation_id = ?", conversationID).
		Delete(&conversation_entity.Message{}).Error
}

func (r *conversationRepo) UpsertMessagesByCagoID(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
	return db.Ctx(ctx).Transaction(func(tx *gorm.DB) error {
		keep := make([]string, 0, len(msgs))
		for _, m := range msgs {
			keep = append(keep, m.CagoID)
		}
		// 删除本次快照不再包含的旧行。当 keep 为空时（Delete 路径），删除所有该会话的 message 行。
		del := tx.Where("conversation_id = ?", conversationID)
		if len(keep) > 0 {
			del = del.Where("cago_id NOT IN ?", keep)
		}
		if err := del.Delete(&conversation_entity.Message{}).Error; err != nil {
			return err
		}
		// 逐行 upsert
		for _, m := range msgs {
			var existing conversation_entity.Message
			err := tx.Where("conversation_id = ? AND cago_id = ?", conversationID, m.CagoID).
				First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(m).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			updates := map[string]any{
				"role":             m.Role,
				"content":          m.Content,
				"parent_id":        m.ParentID,
				"kind":             m.Kind,
				"origin":           m.Origin,
				"thinking":         m.Thinking,
				"tool_call_json":   m.ToolCallJSON,
				"tool_result_json": m.ToolResultJSON,
				"persist":          m.Persist,
				"raw":              m.Raw,
				"msg_time":         m.MsgTime,
				"sort_order":       m.SortOrder,
			}
			if err := tx.Model(&conversation_entity.Message{}).
				Where("conversation_id = ? AND cago_id = ?", conversationID, m.CagoID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *conversationRepo) UpdateState(ctx context.Context, conversationID int64, threadID, stateValuesJSON string) error {
	return db.Ctx(ctx).Model(&conversation_entity.Conversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"thread_id":    threadID,
			"state_values": stateValuesJSON,
			"updatetime":   time.Now().Unix(),
		}).Error
}

func (r *conversationRepo) UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error {
	return db.Ctx(ctx).Model(&conversation_entity.Message{}).
		Where("conversation_id = ? AND cago_id = ?", conversationID, cagoID).
		Update("token_usage", tokenUsageJSON).Error
}
