package conversation_svc

import (
	"context"
	"os"
	"time"

	"ops-cat/internal/model/entity/conversation_entity"
	"ops-cat/internal/repository/conversation_repo"
)

// ConversationSvc 会话业务接口
type ConversationSvc interface {
	Create(ctx context.Context, conv *conversation_entity.Conversation) error
	List(ctx context.Context) ([]*conversation_entity.Conversation, error)
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	Update(ctx context.Context, conv *conversation_entity.Conversation) error
	Delete(ctx context.Context, id int64) error

	// 消息持久化
	SaveMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
}

type conversationSvc struct{}

var defaultConversation = &conversationSvc{}

// Conversation 获取 ConversationSvc 实例
func Conversation() ConversationSvc {
	return defaultConversation
}

func (s *conversationSvc) Create(ctx context.Context, conv *conversation_entity.Conversation) error {
	now := time.Now().Unix()
	conv.Createtime = now
	conv.Updatetime = now
	conv.Status = conversation_entity.StatusActive

	// 创建工作目录（本地 CLI 模式）
	if conv.WorkDir != "" {
		if err := os.MkdirAll(conv.WorkDir, 0755); err != nil {
			return err
		}
	}

	return conversation_repo.Conversation().Create(ctx, conv)
}

func (s *conversationSvc) List(ctx context.Context) ([]*conversation_entity.Conversation, error) {
	return conversation_repo.Conversation().List(ctx)
}

func (s *conversationSvc) Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error) {
	return conversation_repo.Conversation().Find(ctx, id)
}

func (s *conversationSvc) Update(ctx context.Context, conv *conversation_entity.Conversation) error {
	conv.Updatetime = time.Now().Unix()
	return conversation_repo.Conversation().Update(ctx, conv)
}

func (s *conversationSvc) Delete(ctx context.Context, id int64) error {
	// 获取会话信息以清理工作目录
	conv, err := conversation_repo.Conversation().Find(ctx, id)
	if err != nil {
		return err
	}

	// 软删除
	if err := conversation_repo.Conversation().Delete(ctx, id); err != nil {
		return err
	}

	// 删除消息
	_ = conversation_repo.Conversation().DeleteMessages(ctx, id)

	// 清理工作目录
	if conv.WorkDir != "" {
		_ = os.RemoveAll(conv.WorkDir)
	}

	return nil
}

func (s *conversationSvc) SaveMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
	// 先删除旧消息
	if err := conversation_repo.Conversation().DeleteMessages(ctx, conversationID); err != nil {
		return err
	}
	// 设置排序和时间
	now := time.Now().Unix()
	for i, msg := range msgs {
		msg.ConversationID = conversationID
		msg.SortOrder = i
		msg.Createtime = now
	}
	return conversation_repo.Conversation().CreateMessages(ctx, msgs)
}

func (s *conversationSvc) LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error) {
	return conversation_repo.Conversation().ListMessages(ctx, conversationID)
}
