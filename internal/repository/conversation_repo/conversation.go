//go:generate mockgen -source=conversation.go -destination=mock_conversation_repo/conversation.go -package=mock_conversation_repo
package conversation_repo

import (
	"context"

	"github.com/cago-frame/cago/database/db"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
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

	// AppendAt 在指定 (conversationID, sortOrder) 位置插入新消息行。
	// 调用方须保证该位置尚不存在记录（否则触发唯一索引冲突）。
	AppendAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error

	// UpdateAt 按 (conversationID, sortOrder) 自然键更新已有消息行。
	// 只写 role/blocks/mentions/token_usage/partial_reason/partial_detail/createtime；
	// id 与 conversation_id 不变。
	UpdateAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error

	// TruncateFrom 删除同一会话中 sort_order >= fromSortOrder 的所有行。
	// 用于 history rewrite / compact 场景的收敛。
	TruncateFrom(ctx context.Context, conversationID int64, fromSortOrder int) error

	// LoadOrdered 按 sort_order ASC 返回指定会话的所有消息行。
	LoadOrdered(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)

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

// NewConversation 创建默认实现。所有方法走 db.Ctx(ctx) 以复用 cago 的事务传播。
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

func (r *conversationRepo) AppendAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error {
	msg.ConversationID = conversationID
	msg.SortOrder = sortOrder
	return db.Ctx(ctx).Create(msg).Error
}

func (r *conversationRepo) UpdateAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error {
	return db.Ctx(ctx).Model(&conversation_entity.Message{}).
		Where("conversation_id = ? AND sort_order = ?", conversationID, sortOrder).
		Updates(map[string]any{
			"role":           msg.Role,
			"blocks":         msg.Blocks,
			"mentions":       msg.Mentions,
			"token_usage":    msg.TokenUsage,
			"partial_reason": msg.PartialReason,
			"partial_detail": msg.PartialDetail,
			"createtime":     msg.Createtime,
		}).Error
}

func (r *conversationRepo) TruncateFrom(ctx context.Context, conversationID int64, fromSortOrder int) error {
	return db.Ctx(ctx).
		Where("conversation_id = ? AND sort_order >= ?", conversationID, fromSortOrder).
		Delete(&conversation_entity.Message{}).Error
}

func (r *conversationRepo) LoadOrdered(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error) {
	var msgs []*conversation_entity.Message
	if err := db.Ctx(ctx).
		Where("conversation_id = ?", conversationID).
		Order("sort_order ASC").
		Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

