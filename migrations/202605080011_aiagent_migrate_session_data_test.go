package migrations

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/glebarez/sqlite"
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// runMigrationsUpTo applies all registered migrations up to (and including) targetID.
// 复用 production 用的 gormigrate 包，所以测的就是发出去的版本。
func runMigrationsUpTo(t *testing.T, targetID string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	all := allMigrationsForTest()
	upto := []*gormigrate.Migration{}
	for _, m := range all {
		upto = append(upto, m)
		if m.ID == targetID {
			break
		}
	}
	m := gormigrate.New(db, gormigrate.DefaultOptions, upto)
	require.NoError(t, m.Migrate())
	return db
}

// allMigrationsForTest 返回与 migrations.go 同序的全量迁移列表。test-only。
// 维护性：如果新增迁移记得追加。
func allMigrationsForTest() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		migration202603220001(),
		migration202603260001(),
		migration202603270001(),
		migration202603290001(),
		migration202603300001(),
		migration202603300002(),
		migration202603310001(),
		migration202604050001(),
		migration202604140001(),
		migration202604160001(),
		migration202604170001(),
		migration202604220001(),
		migration202604230001(),
		migration202604270001(),
		migration202605010001(),
		migration202605060001(),
		migration202605070001(),
		migration202605080001(),
		migration202605080010(),
		migration202605080011(),
		migration202605080012(),
	}
}

func TestMigrate202605080011_OverridesMessagesFromSessionData(t *testing.T) {
	// 跑到 202605080010 — schema 加完列，但还没跑数据迁移
	db := runMigrationsUpTo(t, "202605080010")

	sd := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "real prompt", Persist: true, Time: time.Unix(1000, 0)},
			{ID: "m2", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "real reply", Persist: true, Time: time.Unix(1001, 0)},
		},
		State: agent.State{ThreadID: "tid-1", Values: map[string]string{"k": "v"}},
	}
	blob, _ := json.Marshal(sd)

	conv := &conversation_entity.Conversation{
		Title:        "test",
		ProviderType: "anthropic",
		Status:       conversation_entity.StatusActive,
		SessionData:  string(blob),
		Createtime:   time.Now().Unix(),
		Updatetime:   time.Now().Unix(),
	}
	require.NoError(t, db.Create(conv).Error)

	// 模拟前端写入的"伪"消息行 — 应被 202605080011 删除
	stale := []*conversation_entity.Message{
		{ConversationID: conv.ID, Role: "user", Content: "stale prompt", SortOrder: 0, Createtime: time.Now().Unix()},
		{ConversationID: conv.ID, Role: "assistant", Content: "stale reply", SortOrder: 1, Createtime: time.Now().Unix()},
	}
	require.NoError(t, db.Create(&stale).Error)

	// 跑下一个迁移
	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080011()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got).Error)
	require.Len(t, got, 2)
	assert.Equal(t, "m1", got[0].CagoID, "first row should come from session_data Messages, not stale frontend rows")
	assert.Equal(t, "real prompt", got[0].Content)
	assert.Equal(t, "m2", got[1].CagoID)
	assert.Equal(t, "real reply", got[1].Content)
	assert.Equal(t, "user", got[0].Role)
	assert.Equal(t, "model", got[1].Origin)
	assert.Equal(t, int64(1000), got[0].MsgTime)

	var convAfter conversation_entity.Conversation
	require.NoError(t, db.First(&convAfter, conv.ID).Error)
	assert.Equal(t, "tid-1", convAfter.ThreadID)
	assert.Equal(t, "", convAfter.SessionData, "session_data should be cleared after migration")
	values, err := convAfter.GetStateValues()
	require.NoError(t, err)
	assert.Equal(t, "v", values["k"])
}

func TestMigrate202605080011_SkipsBadBlob(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title:        "bad",
		ProviderType: "anthropic",
		Status:       conversation_entity.StatusActive,
		SessionData:  "{not valid json",
		Createtime:   time.Now().Unix(),
		Updatetime:   time.Now().Unix(),
	}
	require.NoError(t, db.Create(conv).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080011()})
	require.NoError(t, next.Migrate(), "migration should not fail on bad blob")

	var convAfter conversation_entity.Conversation
	require.NoError(t, db.First(&convAfter, conv.ID).Error)
	assert.Equal(t, "{not valid json", convAfter.SessionData, "bad blob conversation should be untouched")
}

func TestMigrate202605080011_SkipsEmptyBlob(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title:        "empty",
		ProviderType: "anthropic",
		Status:       conversation_entity.StatusActive,
		SessionData:  "",
		Createtime:   time.Now().Unix(),
		Updatetime:   time.Now().Unix(),
	}
	require.NoError(t, db.Create(conv).Error)

	// pre-existing message row should NOT be touched (no session_data, nothing to override)
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "user", Content: "kept", SortOrder: 0, Createtime: time.Now().Unix(),
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080011()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Find(&got).Error)
	require.Len(t, got, 1, "row should be preserved when session_data is empty")
	assert.Equal(t, "kept", got[0].Content)
}
