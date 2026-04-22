package conversation_svc

import (
	"context"
	"os"
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
	Delete(ctx context.Context, id int64) error

	// 消息持久化
	SaveMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
}

type conversationSvc struct {
	// saveLocks 为每个 conversationID 维护一把互斥锁。
	// SaveMessages 实现为 delete-all + insert-all，两个 IPC 并发调用时，
	// 若不加锁，后到的 delete 可能覆盖先到的 insert，导致数据丢失。
	// 此外，该锁也让 Save 调用按 IPC 到达顺序落盘，前端依赖这个顺序来保证
	// 「晚调度的快照覆盖早调度的快照」。
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
	if err := conversation_repo.Conversation().DeleteMessages(ctx, id); err != nil {
		logger.Default().Warn("delete conversation messages", zap.Int64("id", id), zap.Error(err))
	}

	// 清理 saveLocks，避免删除后的 conversationID 继续占用 mutex。
	s.saveLocks.Delete(id)

	// 清理工作目录
	if conv.WorkDir != "" {
		if err := os.RemoveAll(conv.WorkDir); err != nil {
			logger.Default().Warn("remove conversation work dir", zap.String("dir", conv.WorkDir), zap.Error(err))
		}
	}

	return nil
}

func (s *conversationSvc) SaveMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
	// 按 conversationID 加锁，串行化同一会话的 delete+insert，防止并发覆盖丢数据。
	lockI, _ := s.saveLocks.LoadOrStore(conversationID, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

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
