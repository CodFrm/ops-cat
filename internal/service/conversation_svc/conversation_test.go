package conversation_svc

import (
	"context"
	"errors"
	"testing"

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

func TestConversationSvc_UpdateWorkDir(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("更新会话工作目录", t, func() {
		convey.Convey("写入 workDir 与新 updatetime", func() {
			mockRepo.EXPECT().UpdateWorkDir(gomock.Any(), int64(1), "/tmp/cwd", gomock.Any()).DoAndReturn(
				func(_ context.Context, _ int64, _ string, updatetime int64) error {
					assert.Greater(t, updatetime, int64(0))
					return nil
				},
			)

			err := Conversation().UpdateWorkDir(ctx, 1, "/tmp/cwd")
			assert.NoError(t, err)
		})

		convey.Convey("repo 返回错误时透传", func() {
			mockRepo.EXPECT().UpdateWorkDir(gomock.Any(), int64(2), "/x", gomock.Any()).
				Return(errors.New("not found"))

			err := Conversation().UpdateWorkDir(ctx, 2, "/x")
			assert.Error(t, err)
		})
	})
}

func TestConversationSvc_Delete(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("删除会话", t, func() {
		convey.Convey("删除成功（软删除+删除消息）", func() {
			mockRepo.EXPECT().Delete(gomock.Any(), int64(1)).Return(nil)
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(1)).Return(nil)

			err := Conversation().Delete(ctx, 1)
			assert.NoError(t, err)
		})

		convey.Convey("软删除失败时返回错误", func() {
			mockRepo.EXPECT().Delete(gomock.Any(), int64(999)).Return(errors.New("db error"))

			err := Conversation().Delete(ctx, 999)
			assert.Error(t, err)
		})

		convey.Convey("删除消息失败不影响会话删除结果", func() {
			mockRepo.EXPECT().Delete(gomock.Any(), int64(2)).Return(nil)
			mockRepo.EXPECT().DeleteMessages(gomock.Any(), int64(2)).Return(errors.New("msg delete error"))

			err := Conversation().Delete(ctx, 2)
			assert.NoError(t, err) // 消息删除失败只打日志，不返回错误
		})
	})
}

func TestConversationSvc_LoadMessages(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("加载消息", t, func() {
		convey.Convey("返回排序后的消息列表", func() {
			expected := []*conversation_entity.Message{
				{ID: 1, ConversationID: 1, Role: "user", SortOrder: 0},
				{ID: 2, ConversationID: 1, Role: "assistant", SortOrder: 1},
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
