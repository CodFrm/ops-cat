package conversation_svc

import (
	"context"
	"sync"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

// ConversationSvc 会话业务接口
type ConversationSvc interface {
	Create(ctx context.Context, conv *conversation_entity.Conversation) error
	List(ctx context.Context) ([]*conversation_entity.Conversation, error)
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	Update(ctx context.Context, conv *conversation_entity.Conversation) error
	UpdateTitle(ctx context.Context, id int64, title string) error
	UpdateWorkDir(ctx context.Context, id int64, workDir string) error
	Delete(ctx context.Context, id int64) error

	// 消息持久化
	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
}

type conversationSvc struct {
	// saveLocks 为每个 conversationID 维护一把互斥锁，供上层（gormStore）按需使用。
	saveLocks sync.Map // map[int64]*sync.Mutex
}

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

func (s *conversationSvc) UpdateTitle(ctx context.Context, id int64, title string) error {
	return conversation_repo.Conversation().UpdateTitle(ctx, id, title, time.Now().Unix())
}

func (s *conversationSvc) UpdateWorkDir(ctx context.Context, id int64, workDir string) error {
	return conversation_repo.Conversation().UpdateWorkDir(ctx, id, workDir, time.Now().Unix())
}

func (s *conversationSvc) Delete(ctx context.Context, id int64) error {
	// 软删除
	if err := conversation_repo.Conversation().Delete(ctx, id); err != nil {
		return err
	}

	// 删除消息
	if err := conversation_repo.Conversation().DeleteMessages(ctx, id); err != nil {
		logger.Default().Warn("delete conversation messages", zap.Int64("id", id), zap.Error(err))
	}

	// 清理 saveLocks，避免删除后的 conversationID 继续占用 mutex。
	s.saveLocks.Delete(id)

	return nil
}

func (s *conversationSvc) LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error) {
	return conversation_repo.Conversation().ListMessages(ctx, conversationID)
}

