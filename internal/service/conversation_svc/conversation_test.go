package conversation_svc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
	"github.com/opskat/opskat/internal/repository/conversation_repo/mock_conversation_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupTest(t *testing.T) (context.Context, *mock_conversation_repo.MockConversationRepo) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(func() { mockCtrl.Finish() })
	ctx := context.Background()
	mockRepo := mock_conversation_repo.NewMockConversationRepo(mockCtrl)
	conversation_repo.RegisterConversation(mockRepo)
	return ctx, mockRepo
}

func TestConversationSvc_Create(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("创建会话", t, func() {
		convey.Convey("创建成功，设置时间戳和状态", func() {
			conv := &conversation_entity.Conversation{
				Title:        "测试会话",
				ProviderType: "openai",
				Model:        "gpt-4",
			}
			mockRepo.EXPECT().Create(gomock.Any(), conv).Return(nil)

			err := Conversation().Create(ctx, conv)
			assert.NoError(t, err)
			assert.Greater(t, conv.Createtime, int64(0))
			assert.Greater(t, conv.Updatetime, int64(0))
			assert.Equal(t, conversation_entity.StatusActive, conv.Status)
		})

		convey.Convey("repo返回错误时创建失败", func() {
			conv := &conversation_entity.Conversation{
				Title:        "测试",
				ProviderType: "openai",
			}
			mockRepo.EXPECT().Create(gomock.Any(), conv).Return(errors.New("db error"))

			err := Conversation().Create(ctx, conv)
			assert.Error(t, err)
		})
	})
}

func TestConversationSvc_List(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("列出会话", t, func() {
		convey.Convey("返回会话列表", func() {
			expected := []*conversation_entity.Conversation{
				{ID: 1, Title: "会话1", ProviderType: "openai", Updatetime: 200},
				{ID: 2, Title: "会话2", ProviderType: "local_cli", Updatetime: 100},
			}
			mockRepo.EXPECT().List(gomock.Any()).Return(expected, nil)

			got, err := Conversation().List(ctx)
			assert.NoError(t, err)
			assert.Len(t, got, 2)
			assert.Equal(t, "会话1", got[0].Title)
		})

		convey.Convey("空列表", func() {
			mockRepo.EXPECT().List(gomock.Any()).Return(nil, nil)

			got, err := Conversation().List(ctx)
			assert.NoError(t, err)
			assert.Empty(t, got)
		})
	})
}

func TestConversationSvc_Get(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("获取会话", t, func() {
		convey.Convey("存在的会话返回成功", func() {
			expected := &conversation_entity.Conversation{
				ID: 1, Title: "测试会话", ProviderType: "openai",
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(expected, nil)

			got, err := Conversation().Get(ctx, 1)
			assert.NoError(t, err)
			assert.Equal(t, "测试会话", got.Title)
		})

		convey.Convey("不存在的会话返回错误", func() {
			mockRepo.EXPECT().Find(gomock.Any(), int64(999)).Return(nil, errors.New("record not found"))

			_, err := Conversation().Get(ctx, 999)
			assert.Error(t, err)
		})
	})
}

func TestConversationSvc_Update(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("更新会话", t, func() {
		convey.Convey("更新成功，设置updatetime", func() {
			conv := &conversation_entity.Conversation{
				ID:    1,
				Title: "更新标题",
			}
			mockRepo.EXPECT().Update(gomock.Any(), conv).Return(nil)

			err := Conversation().Update(ctx, conv)
			assert.NoError(t, err)
			assert.Greater(t, conv.Updatetime, int64(0))
		})
	})
}

func TestConversationSvc_UpdateTitle(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("更新会话标题", t, func() {
		convey.Convey("仅更新标题和 updatetime", func() {
			mockRepo.EXPECT().UpdateTitle(gomock.Any(), int64(1), "更新标题", gomock.Any()).DoAndReturn(
				func(_ context.Context, _ int64, _ string, updatetime int64) error {
					assert.Greater(t, updatetime, int64(0))
					return nil
				},
			)

			err := Conversation().UpdateTitle(ctx, 1, "更新标题")
			assert.NoError(t, err)
		})
	})
}

func TestConversationSvc_Delete(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("删除会话", t, func() {
		convey.Convey("删除成功（软删除+删除消息）", func() {
			conv := &conversation_entity.Conversation{ID: 1, Title: "测试"}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(conv, nil)
			mockRepo.EXPECT().Delete(gomock.Any(), int64(1)).Return(nil)
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(1)).Return(nil)

			err := Conversation().Delete(ctx, 1)
			assert.NoError(t, err)
		})

		convey.Convey("会话不存在时删除失败", func() {
			mockRepo.EXPECT().Find(gomock.Any(), int64(999)).Return(nil, errors.New("not found"))

			err := Conversation().Delete(ctx, 999)
			assert.Error(t, err)
		})

		convey.Convey("删除消息失败不影响会话删除结果", func() {
			conv := &conversation_entity.Conversation{ID: 2, Title: "测试2"}
			mockRepo.EXPECT().Find(gomock.Any(), int64(2)).Return(conv, nil)
			mockRepo.EXPECT().Delete(gomock.Any(), int64(2)).Return(nil)
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(2)).Return(errors.New("msg delete error"))

			err := Conversation().Delete(ctx, 2)
			assert.NoError(t, err) // 消息删除失败只打日志，不返回错误
		})
	})
}

func TestConversationSvc_SaveMessages(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("保存消息", t, func() {
		convey.Convey("先删旧消息再创建新消息", func() {
			msgs := []*conversation_entity.Message{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
			}
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(1)).Return(nil)
			mockRepo.EXPECT().CreateMessages(gomock.Any(), msgs).DoAndReturn(
				func(_ context.Context, msgs []*conversation_entity.Message) error {
					// 验证排序和时间已设置
					for i, msg := range msgs {
						assert.Equal(t, int64(1), msg.ConversationID)
						assert.Equal(t, i, msg.SortOrder)
						assert.Greater(t, msg.Createtime, int64(0))
					}
					return nil
				},
			)

			err := Conversation().SaveMessages(ctx, 1, msgs)
			assert.NoError(t, err)
		})

		convey.Convey("删除旧消息失败则返回错误", func() {
			msgs := []*conversation_entity.Message{
				{Role: "user", Content: "test"},
			}
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(1)).Return(errors.New("delete error"))

			err := Conversation().SaveMessages(ctx, 1, msgs)
			assert.Error(t, err)
		})
	})
}

// TestConversationSvc_SaveMessages_ConcurrentSameID 验证同一 conversationID 的并发 SaveMessages
// 被序列化，防止后到的 delete 覆盖先到的 insert。
// 这是守护修复「关闭软件丢失 AI 对话」的关键不变量：前端立即落盘 + 300ms 防抖落盘
// 会从不同 IPC goroutine 并发进入 SaveMessages，没有序列化就会丢数据。
func TestConversationSvc_SaveMessages_ConcurrentSameID(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(42)).Times(2).DoAndReturn(
		func(_ context.Context, _ int64) error {
			n := inFlight.Add(1)
			// 记录峰值并发数；若大于 1 说明没锁住。
			for {
				m := maxInFlight.Load()
				if n <= m || maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
			// 给另一个 goroutine 抢跑的机会，放大竞争窗口。
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
			return nil
		},
	)
	mockRepo.EXPECT().CreateMessages(gomock.Any(), gomock.Any()).Times(2).Return(nil)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Conversation().SaveMessages(ctx, 42, []*conversation_entity.Message{{Role: "user", Content: "x"}})
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), maxInFlight.Load(), "同一 conversationID 的 SaveMessages 应串行")
}

// TestConversationSvc_SaveMessages_ConcurrentDifferentID 验证不同 conversationID 的
// SaveMessages 仍可并发执行，锁粒度正确。
func TestConversationSvc_SaveMessages_ConcurrentDifferentID(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	for _, id := range []int64{101, 102} {
		mockRepo.EXPECT().DeleteMessages(gomock.Any(), id).DoAndReturn(
			func(_ context.Context, _ int64) error {
				n := inFlight.Add(1)
				for {
					m := maxInFlight.Load()
					if n <= m || maxInFlight.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				inFlight.Add(-1)
				return nil
			},
		)
		mockRepo.EXPECT().CreateMessages(gomock.Any(), gomock.Any()).Return(nil)
	}

	var wg sync.WaitGroup
	for _, id := range []int64{101, 102} {
		wg.Add(1)
		go func(convID int64) {
			defer wg.Done()
			_ = Conversation().SaveMessages(ctx, convID, []*conversation_entity.Message{{Role: "user", Content: "x"}})
		}(id)
	}
	wg.Wait()

	assert.Equal(t, int32(2), maxInFlight.Load(), "不同 conversationID 的 SaveMessages 应可并发")
}

func TestConversationSvc_LoadMessages(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("加载消息", t, func() {
		convey.Convey("返回排序后的消息列表", func() {
			expected := []*conversation_entity.Message{
				{ID: 1, ConversationID: 1, Role: "user", Content: "问题", SortOrder: 0},
				{ID: 2, ConversationID: 1, Role: "assistant", Content: "回答", SortOrder: 1},
			}
			mockRepo.EXPECT().ListMessages(gomock.Any(), int64(1)).Return(expected, nil)

			got, err := Conversation().LoadMessages(ctx, 1)
			assert.NoError(t, err)
			assert.Len(t, got, 2)
			assert.Equal(t, "user", got[0].Role)
			assert.Equal(t, "assistant", got[1].Role)
		})
	})
}
