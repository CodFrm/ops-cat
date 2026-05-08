package conversation_repo

import (
	"context"

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
	CreateMessages(ctx context.Context, msgs []*conversation_entity.Message) error
	DeleteMessages(ctx context.Context, conversationID int64) error
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

func (r *conversationRepo) CreateMessages(ctx context.Context, msgs []*conversation_entity.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	return db.Ctx(ctx).Create(&msgs).Error
}

func (r *conversationRepo) DeleteMessages(ctx context.Context, conversationID int64) error {
	return db.Ctx(ctx).Where("conversation_id = ?", conversationID).
		Delete(&conversation_entity.Message{}).Error
}
