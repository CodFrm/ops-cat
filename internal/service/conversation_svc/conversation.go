package conversation_svc

import (
	"context"
	"encoding/json"
	"fmt"
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

	// UpsertCagoMessages 是 cago gormStore 的写入入口（替代旧的 SaveMessages）。
	// 同 conversation 的并发调用通过 saveLocks 串行化；行级 upsert 委托给
	// conversation_repo.UpsertMessagesByCagoID。msgs 的 ConversationID 与
	// SortOrder/Createtime 由本方法填充——调用方无需预设。
	UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error

	// UpdateConversationState 把 cago Session 的 ThreadID + State.Values 写入
	// conversations 表的 thread_id / state_values 列。values 序列化为 JSON；
	// 空 map / nil 映射到空字符串（GetStateValues 返回 nil）。
	UpdateConversationState(ctx context.Context, conversationID int64, threadID string, values map[string]string) error

	// UpdateMessageTokenUsage 把已序列化的 token_usage JSON 写到指定 (conversationID, cagoID)
	// 消息行。由 gormStore.Save drain System.pendingUsage 时调用，纯透传。
	UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error
}

type conversationSvc struct {
	// saveLocks 为每个 conversationID 维护一把互斥锁。
	// 同会话的 cago Save 并发到来时串行化，避免 upsert 间的快照交错（cago
	// 每次 Save 发的是全量快照，后到的应当 last-write-wins）。
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

func (s *conversationSvc) UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
	// 复用 saveLocks：同会话的 cago Save 并发到来时串行化，避免 upsert 间的快照交错
	// （cago 每次 Save 发的是全量快照，后到的应当 last-write-wins）。
	lockI, _ := s.saveLocks.LoadOrStore(conversationID, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().Unix()
	for i, m := range msgs {
		m.ConversationID = conversationID
		if m.SortOrder == 0 {
			m.SortOrder = i
		}
		if m.Createtime == 0 {
			m.Createtime = now
		}
	}
	return conversation_repo.Conversation().UpsertMessagesByCagoID(ctx, conversationID, msgs)
}

func (s *conversationSvc) UpdateConversationState(ctx context.Context, conversationID int64, threadID string, values map[string]string) error {
	var jsonStr string
	if len(values) > 0 {
		b, err := json.Marshal(values)
		if err != nil {
			return fmt.Errorf("marshal state values: %w", err)
		}
		jsonStr = string(b)
	}
	return conversation_repo.Conversation().UpdateState(ctx, conversationID, threadID, jsonStr)
}

func (s *conversationSvc) UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error {
	return conversation_repo.Conversation().UpdateMessageTokenUsage(ctx, conversationID, cagoID, tokenUsageJSON)
}
