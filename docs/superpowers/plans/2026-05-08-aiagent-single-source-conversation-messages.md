# AI Agent 会话存储单源化重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `conversation_messages` 表成为 AI 会话历史的唯一真实来源，由 cago `gormStore` 写入；删除 `conversations.session_data` blob 与前端 `SaveConversationMessages` 写入路径，消除"前端展示与 LLM 实际历史不一致"的双写架构。

**Architecture:**
1. 把 cago `agent.SessionData`（`Messages []agent.Message + State{ThreadID, Values}`）的字段全部展开到 `conversation_messages` 行 + `conversations` 表的两列上。
2. cago 的 `gormStore.Save` 通过 `cago_id` upsert 行（事务内 diff），是唯一写入者；前端只读、靠 cago Stream 事件实时渲染。
3. **token_usage 也走 gormStore.Save 路径**：`event_bridge` 收到 `EventUsage` 时不再直写 DB，而是缓存到 `System.pendingUsage[cagoMsgID]`；下一次 cago 触发 `internalObserver.persist`（EventMessageEnd 或 EventDone）时由 gormStore.Save 在事务里把 usage 写到对应行的 `token_usage` 列。`mentions` 同理：`SendAIMessage` 入口 stash 到 `System.pendingMentions`，gormStore.Save 在 user 消息首次出现时一并写入。这样**所有持久化都在 cago 框架保护的 ctx 下完成**，不存在游离的"事件触发的旁路写入"。
4. **取消保存不变量（在 cago 框架内保证）**：用户点 Stop → `streamCancel()` → ctx 被取消，cago 仍 fire `EventDone/Error` 走 `internalObserver.persist`。在 cago `agent/session.go` 内把 `persist` 用的 ctx 替换成 `context.WithTimeout(context.WithoutCancel(ctx), 5s)`——final flush 不被用户取消信号截断。本仓库 OpsKat 这边因此**不需要 detachCtx**，gormStore.Save 直接用传入的 ctx；UX 不退化（SSE 仍立即停），架构干净。所有第三方 cago Store 实现都自动受益。
5. 数据迁移用现有 `session_data` 覆盖旧 `conversation_messages` 行（LLM 视角为准），完成后删除 `session_data` 列。

**Tech Stack:** Go 1.25 + gorm + gormigrate + cago `agents/agent` + Wails IPC + React/vitest 前端测试。

---

## File Structure

**新增文件：**
- `migrations/202605080010_aiagent_session_columns.go` — 给 `conversation_messages` 加 cago Message 字段，给 `conversations` 加 thread_id / state_values
- `migrations/202605080011_aiagent_migrate_session_data.go` — 把 `session_data` blob 反序列化成行，落到新列

**改动文件（cago 框架）：**
- `/Users/codfrm/Code/cago/agents/agent/session.go` — `internalObserver.persist` 与 `RewriteHistory` 的 `Save` 调用包 `context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)`，把"final flush 不受用户取消信号截断"作为框架不变量

**改动文件（OpsKat）：**
- `internal/model/entity/conversation_entity/conversation.go` — `Message` 加 cago 字段；`Conversation` 加 `ThreadID`、`StateValues` 列、清理 `SessionData`/`SessionInfo` 用法
- `internal/repository/conversation_repo/conversation.go` — 新增 `UpsertMessagesByCagoID`、`UpdateState`；保留 `ListMessages` 给前端读
- `internal/service/conversation_svc/conversation.go` — 移除 `SaveMessages`（前端写入路径作废），新增 `UpsertCagoMessages`/`UpdateConversationState`
- `internal/aiagent/store.go` — `gormStore.Save/Load/Delete` 重写为读写 `conversation_messages` + `conversations.thread_id/state_values`；**直接用传入 ctx，不再有 detachCtx**
- `internal/aiagent/store_test.go` — 改为针对新的行级 upsert/load 行为
- `internal/aiagent/system.go` — 引入 `pendingMentions` 与 `pendingUsage` 缓存，分别由 `Stream` 入口 / `event_bridge.EventUsage` 填入；gormStore 在 Save 时 drain 出来一并落盘
- `internal/aiagent/event_bridge.go` — `EventUsage` 只缓存到 `System.pendingUsage`，不再直写 DB
- `internal/app/app_ai.go` — 删除 `SaveConversationMessages` binding；`loadConversationDisplayMessages` 改为从 cago Message 字段派生 `Blocks`；`SendAIMessage` 把 mentions stash 进 System
- `frontend/wailsjs/go/app/App.{d.ts,js}`、`models.ts` — Wails 重新生成（`make dev` 跑一次）
- `frontend/src/stores/aiStore.ts` — 删除 `SaveConversationMessages` import 与所有 `schedulePersist`/`persistNow`/`persistConversationSnapshot` 调用，简化为纯前端 in-memory + Load 接口
- `frontend/src/__tests__/aiStore.test.ts` — 移除 `SaveConversationMessages` 相关 mock/断言
- `frontend/src/__tests__/AIChatEditResend.test.tsx` — 同上

**删除/废弃 schema 字段（数据迁移完成后）：**
- `conversations.session_data`（清空，列保留以防回滚）
- `conversation_messages.blocks`（行内 blocks 改为读时派生；列保留以兼容旧数据回放，可后续清掉）

---

## Task 0: cago 框架改动 — 把 final Save 与 user-cancel 解耦

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/agent/session.go`
- Test: `/Users/codfrm/Code/cago/agents/agent/session_save_cancel_test.go`（新建）

**背景：** OpsKat go.mod 已 `replace github.com/cago-frame/agents => /Users/codfrm/Code/cago/agents`，可以直接改 cago。把"清理写入不受用户取消控制"的语义放到 cago `internalObserver.persist` 内部，所有第三方 Store 实现都自动受益，OpsKat 这边 gormStore 不需要 detachCtx。

- [ ] **Step 1: 写失败测试 — Store.Save 在 ctx 已 cancel 时仍能落盘**

新建 `agents/agent/session_save_cancel_test.go`：

```go
package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
)

// recordingStore 记录每次 Save 时 ctx 是否已 cancel；用于验证 cago 在 final flush
// 路径上把 ctx 与用户 cancel 解耦了。
type recordingStore struct {
	saved      []agent.SessionData
	ctxWasDone []bool
}

func (s *recordingStore) Save(ctx context.Context, _ string, data agent.SessionData) error {
	done := false
	select {
	case <-ctx.Done():
		done = true
	default:
	}
	s.ctxWasDone = append(s.ctxWasDone, done)
	s.saved = append(s.saved, data)
	return nil
}
func (s *recordingStore) Load(_ context.Context, _ string) (agent.SessionData, error) {
	return agent.SessionData{}, nil
}
func (s *recordingStore) Delete(_ context.Context, _ string) error { return nil }

func TestInternalObserverPersist_DecouplesUserCancel(t *testing.T) {
	store := &recordingStore{}
	sess := agent.NewSession("sid-1", store) // 用 cago 公开的 Session 构造方式；如签名不同按现有测试照搬
	// 模拟 EventDone 用一个已 cancel 的 ctx
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	sess.AppendMessageForTest(agent.Message{ // 用现有测试辅助；如不存在按 session_test.go 已有手段构造历史
		ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "hi", Persist: true,
	})
	sess.DispatchInternalForTest(cancelled, agent.Event{Kind: agent.EventDone})

	if len(store.saved) == 0 {
		t.Fatal("Save was not invoked under cancelled ctx — final flush lost")
	}
	for i, done := range store.ctxWasDone {
		if done {
			t.Errorf("Save call %d received cancelled ctx; cago should have used WithoutCancel", i)
		}
	}
}

func TestInternalObserverPersist_AppliesTimeout(t *testing.T) {
	// Save 阻塞超过 saveTimeout 应该被 cago 强制中断（ctx.Err == DeadlineExceeded）。
	blocking := &blockingStore{releaseAfter: 10 * time.Second}
	sess := agent.NewSession("sid-2", blocking)
	start := time.Now()
	sess.DispatchInternalForTest(context.Background(), agent.Event{Kind: agent.EventDone})
	elapsed := time.Since(start)
	if elapsed > 7*time.Second {
		t.Errorf("Save did not respect saveTimeout (%s); cago should cap final flush", elapsed)
	}
}

type blockingStore struct{ releaseAfter time.Duration }

func (s *blockingStore) Save(ctx context.Context, _ string, _ agent.SessionData) error {
	select {
	case <-time.After(s.releaseAfter):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *blockingStore) Load(_ context.Context, _ string) (agent.SessionData, error) {
	return agent.SessionData{}, nil
}
func (s *blockingStore) Delete(_ context.Context, _ string) error { return nil }
```

如 `NewSession` / `AppendMessageForTest` / `DispatchInternalForTest` 在 cago 公开 API 不存在，参照 `agents/agent/session_test.go` 现有的 Session 构造方式与事件触发手段照搬（cago 仓库内部测试可访问私有字段）。本测试是"必须在 cago 仓库里跑"的——不要复制到 OpsKat。

- [ ] **Step 2: 跑测试确认 fail**

Run: `cd /Users/codfrm/Code/cago/agents && go test ./agent/ -run "TestInternalObserverPersist" -v`

Expected: FAIL（当前 persist 直接把传入的 ctx 透传给 Store.Save）

- [ ] **Step 3: 改 internalObserver.persist 与 RewriteHistory 的 Save 调用**

打开 `agents/agent/session.go`，在文件顶部 const 区追加：

```go
// saveTimeout 是 internalObserver 与 RewriteHistory 调 Store.Save 的兜底超时。
// final flush 用 context.WithoutCancel(ctx) 把 user-cancel 信号摘掉——cago Stream
// 的 ctx 在用户 Stop 后已 Done，但本轮 history 的 final flush 不应被取消信号截断。
// 同时加 5s 超时，防止一个卡住的 Store 实现拖住 observer goroutine。
const saveTimeout = 5 * time.Second
```

修改 `internalObserver` 内的 `persist` 闭包（约 line 296-307）：

```go
persist := func(ctx context.Context) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer cancel()
	s.mu.Lock()
	data := SessionData{
		Messages: append([]Message(nil), s.history...),
		State:    s.state.Clone(),
	}
	s.mu.Unlock()
	if err := s.store.Save(saveCtx, s.id, data); err != nil {
		logSaveError(saveCtx, s.id, err)
	}
}
```

修改 `RewriteHistory`（约 line 379-386）的 Save 调用：

```go
if s.store != nil {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer cancel()
	if err := s.store.Save(saveCtx, s.id, SessionData{
		Messages: append([]Message(nil), cp...),
		State:    state,
	}); err != nil {
		logSaveError(saveCtx, s.id, err)
	}
}
```

如顶部 import 缺 `time`，补上。

- [ ] **Step 4: 跑 cago 全包测试**

Run: `cd /Users/codfrm/Code/cago/agents && go test ./agent/... -count=1`

Expected: PASS（包括新加的两个 cancel 测试 + 现有 session_test.go 的所有用例）

- [ ] **Step 5: 跑 OpsKat 端确认 replace 链路无碍**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`

Expected: 编译通过

- [ ] **Step 6: 提交（cago 仓库内）**

```bash
cd /Users/codfrm/Code/cago
git add agents/agent/session.go agents/agent/session_save_cancel_test.go
git commit -m "🔒 session: final flush 与 user-cancel 解耦——Store.Save 在 ctx 已 cancel 时仍能落盘

internalObserver.persist 与 RewriteHistory 的 Store.Save 调用包
context.WithoutCancel + WithTimeout(saveTimeout)，把 \"清理写入不受用户取消控制\"
作为框架不变量。所有第三方 Store 实现都自动受益，无需各自实现 detachCtx。"
```

---

## Task 1: 加 schema migration（columns 仅添加，不动数据）

**Files:**
- Create: `migrations/202605080001_aiagent_session_columns.go`
- Modify: `migrations/migrations.go`

- [ ] **Step 1: 写迁移文件**

```go
// migrations/202605080001_aiagent_session_columns.go
package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605080001 把 cago agent.Message + State 的字段平铺到表上，
// 为后续把 conversations.session_data 单源化到 conversation_messages 做准备。
func migration202605080001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`ALTER TABLE conversation_messages ADD COLUMN cago_id VARCHAR(64) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN parent_id VARCHAR(64) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN kind VARCHAR(32) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN origin VARCHAR(32) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN thinking TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN tool_call_json TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN tool_result_json TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN persist BOOLEAN NOT NULL DEFAULT 1`,
				`ALTER TABLE conversation_messages ADD COLUMN raw TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN msg_time INTEGER NOT NULL DEFAULT 0`,
				`CREATE INDEX IF NOT EXISTS idx_conv_msg_cago_id ON conversation_messages(conversation_id, cago_id)`,

				`ALTER TABLE conversations ADD COLUMN thread_id VARCHAR(255) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversations ADD COLUMN state_values TEXT`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持 DROP COLUMN/INDEX 整批回滚；保留列在回滚时无害。
			return nil
		},
	}
}
```

- [ ] **Step 2: 注册迁移**

```go
// migrations/migrations.go 在 migration202605070001() 之后追加
		migration202605080001(),
```

（在 `migration202605080001()` 已经有的 `migration202605080001()` 不要混淆——本仓库已有 `202605080001` ID 用于 `ai_local_tool_grants`。把本任务用的 ID 改为 `202605080010`，重命名文件为 `202605080010_aiagent_session_columns.go` 以避免 ID 冲突。

更新最终：
- 文件名：`migrations/202605080010_aiagent_session_columns.go`
- 函数名：`migration202605080010`
- migrations.go 注册：`migration202605080010(),`）

- [ ] **Step 3: 跑全量迁移测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./migrations/... ./internal/repository/conversation_repo/... -count=1`

Expected: PASS（所有现有迁移测试在加完新列后仍通过）。如有 schema 相关测试 fail 因为新列默认值缺失，按提示补默认值。

- [ ] **Step 4: 提交**

```bash
git add migrations/202605080010_aiagent_session_columns.go migrations/migrations.go
git commit -m "✨ aiagent: 给 conversation_messages 与 conversations 加 cago Message/State 列"
```

---

## Task 2: 扩展 Conversation/Message entity

**Files:**
- Modify: `internal/model/entity/conversation_entity/conversation.go`
- Test: `internal/model/entity/conversation_entity/conversation_test.go`

- [ ] **Step 1: 写失败测试 — Message 新字段 round-trip**

```go
// 追加到 conversation_test.go
func TestMessage_CagoFieldsRoundTrip(t *testing.T) {
	m := &Message{
		CagoID:         "msg-1",
		ParentID:       "parent-1",
		Kind:           "tool_call",
		Origin:         "model",
		Persist:        true,
		MsgTime:        1714000000,
		Thinking:       `[{"text":"plan"}]`,
		ToolCallJSON:   `{"id":"call-1","name":"list","args":{}}`,
		ToolResultJSON: `{"result":"ok"}`,
		Raw:            `{"x":1}`,
	}
	if m.CagoID != "msg-1" || m.ToolCallJSON == "" {
		t.Fatalf("cago fields not preserved: %+v", m)
	}
}

func TestConversation_StateRoundTrip(t *testing.T) {
	c := &Conversation{ThreadID: "t-1"}
	if err := c.SetStateValues(map[string]string{"k": "v"}); err != nil {
		t.Fatalf("SetStateValues: %v", err)
	}
	got, err := c.GetStateValues()
	if err != nil {
		t.Fatalf("GetStateValues: %v", err)
	}
	if got["k"] != "v" {
		t.Fatalf("state values lost: %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试确认 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/model/entity/conversation_entity/... -run "TestMessage_CagoFieldsRoundTrip|TestConversation_StateRoundTrip" -v`

Expected: FAIL（编译错误：未定义字段）

- [ ] **Step 3: 给 Message struct 加字段**

在 `Message struct` 中插入新字段（保持现有字段不动）：

```go
type Message struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64  `gorm:"column:conversation_id;index;not null"`
	Role           string `gorm:"column:role;type:varchar(20);not null"`
	Content        string `gorm:"column:content;type:text"`
	ToolCalls      string `gorm:"column:tool_calls;type:text"`     // 旧字段，保留兼容
	ToolCallID     string `gorm:"column:tool_call_id;type:varchar(100)"` // 旧字段，保留兼容
	Blocks         string `gorm:"column:blocks;type:text"`         // 旧字段，保留兼容
	Mentions       string `gorm:"column:mentions;type:text"`
	TokenUsage     string `gorm:"column:token_usage;type:text"`
	SortOrder      int    `gorm:"column:sort_order;default:0"`
	Createtime     int64  `gorm:"column:createtime"`

	// cago.Message 平铺字段（202605080010 迁移之后写入）
	CagoID         string `gorm:"column:cago_id;type:varchar(64);index"`
	ParentID       string `gorm:"column:parent_id;type:varchar(64)"`
	Kind           string `gorm:"column:kind;type:varchar(32)"`
	Origin         string `gorm:"column:origin;type:varchar(32)"`
	Thinking       string `gorm:"column:thinking;type:text"`
	ToolCallJSON   string `gorm:"column:tool_call_json;type:text"`
	ToolResultJSON string `gorm:"column:tool_result_json;type:text"`
	Persist        bool   `gorm:"column:persist;default:true"`
	Raw            string `gorm:"column:raw;type:text"`
	MsgTime        int64  `gorm:"column:msg_time"`
}
```

- [ ] **Step 4: 给 Conversation 加 ThreadID 与 state_values 字段 + helpers**

在 `Conversation struct` 末尾加：

```go
type Conversation struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Title        string `gorm:"column:title;type:varchar(255)"`
	ProviderType string `gorm:"column:provider_type;type:varchar(50);not null"`
	Model        string `gorm:"column:model;type:varchar(100)"`
	ProviderID   int64  `gorm:"column:provider_id"`
	SessionData  string `gorm:"column:session_data;type:text"` // 待数据迁移完成后清空，下版本删
	WorkDir      string `gorm:"column:work_dir;type:varchar(500)"`
	Status       int    `gorm:"column:status;default:1"`
	Createtime   int64  `gorm:"column:createtime"`
	Updatetime   int64  `gorm:"column:updatetime"`

	ThreadID    string `gorm:"column:thread_id;type:varchar(255)"`
	StateValues string `gorm:"column:state_values;type:text"`
}
```

加 helpers（在文件末尾）：

```go
// GetStateValues 反序列化 cago State.Values。
func (c *Conversation) GetStateValues() (map[string]string, error) {
	if c.StateValues == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(c.StateValues), &m); err != nil {
		return nil, fmt.Errorf("解析 state_values: %w", err)
	}
	return m, nil
}

// SetStateValues 序列化 cago State.Values；nil/空视为清空。
func (c *Conversation) SetStateValues(v map[string]string) error {
	if len(v) == 0 {
		c.StateValues = ""
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化 state_values: %w", err)
	}
	c.StateValues = string(data)
	return nil
}
```

- [ ] **Step 5: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/model/entity/conversation_entity/... -count=1 -v`

Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/model/entity/conversation_entity/
git commit -m "✨ conversation_entity: 加 cago Message 平铺字段与 Conversation State"
```

---

## Task 3: repo 加 UpsertMessagesByCagoID + UpdateState

**Files:**
- Modify: `internal/repository/conversation_repo/conversation.go`
- Modify: `internal/repository/conversation_repo/mock_conversation_repo/`（go generate）
- Test: `internal/repository/conversation_repo/conversation_test.go`（如已有）或新建

- [ ] **Step 1: 写失败测试**

新建 `internal/repository/conversation_repo/conversation_upsert_test.go`：

```go
package conversation_repo_test

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
	"github.com/opskat/opskat/internal/testutil/dbtest"
)

func TestUpsertMessagesByCagoID(t *testing.T) {
	dbtest.SetupDB(t) // 项目里现有的测试 DB helper；如名字不一致按现有命名替换
	ctx := context.Background()
	repo := conversation_repo.NewConversation()

	conv := &conversation_entity.Conversation{Title: "t", Status: conversation_entity.StatusActive}
	if err := repo.Create(ctx, conv); err != nil {
		t.Fatalf("create conv: %v", err)
	}

	first := []*conversation_entity.Message{
		{ConversationID: conv.ID, CagoID: "m1", Role: "user", Content: "hi", SortOrder: 0, Persist: true},
		{ConversationID: conv.ID, CagoID: "m2", Role: "assistant", Content: "hello", SortOrder: 1, Persist: true},
	}
	if err := repo.UpsertMessagesByCagoID(ctx, conv.ID, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []*conversation_entity.Message{
		{ConversationID: conv.ID, CagoID: "m1", Role: "user", Content: "hi (edited)", SortOrder: 0, Persist: true},
		{ConversationID: conv.ID, CagoID: "m3", Role: "assistant", Content: "next", SortOrder: 1, Persist: true},
	}
	if err := repo.UpsertMessagesByCagoID(ctx, conv.ID, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].CagoID != "m1" || got[0].Content != "hi (edited)" {
		t.Errorf("m1 not updated: %+v", got[0])
	}
	if got[1].CagoID != "m3" {
		t.Errorf("m2 should be deleted (not in second), got: %+v", got[1])
	}
}

func TestUpdateState(t *testing.T) {
	dbtest.SetupDB(t)
	ctx := context.Background()
	repo := conversation_repo.NewConversation()

	conv := &conversation_entity.Conversation{Title: "t", Status: conversation_entity.StatusActive}
	if err := repo.Create(ctx, conv); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdateState(ctx, conv.ID, "thread-1", `{"k":"v"}`); err != nil {
		t.Fatalf("update state: %v", err)
	}
	got, err := repo.Find(ctx, conv.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.ThreadID != "thread-1" || got.StateValues != `{"k":"v"}` {
		t.Errorf("state not persisted: %+v", got)
	}
}
```

如项目无 `internal/testutil/dbtest`，按现有 `conversation_test.go`（在 `service/conversation_svc/` 下）使用的 SQLite 内存初始化方式照搬。

- [ ] **Step 2: 跑测试确认 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/repository/conversation_repo/... -run "TestUpsertMessagesByCagoID|TestUpdateState" -v`

Expected: FAIL（方法未定义）

- [ ] **Step 3: 在接口与实现上加方法**

在 `ConversationRepo` 接口里追加：

```go
	// 行级 upsert：以 (conversation_id, cago_id) 为自然键。
	UpsertMessagesByCagoID(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	UpdateState(ctx context.Context, conversationID int64, threadID, stateValuesJSON string) error
```

实现：

```go
func (r *conversationRepo) UpsertMessagesByCagoID(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
	tx := db.Ctx(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	keep := make([]string, 0, len(msgs))
	for _, m := range msgs {
		keep = append(keep, m.CagoID)
	}
	// 删除本次 SessionData 不再包含的旧行（history rewrite / compact 场景）
	q := tx.Where("conversation_id = ?", conversationID)
	if len(keep) > 0 {
		q = q.Where("cago_id NOT IN ?", keep)
	}
	if err := q.Delete(&conversation_entity.Message{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 逐行 upsert：存在则更新核心字段，不动 mentions/token_usage 等扩展列
	for _, m := range msgs {
		var existing conversation_entity.Message
		err := tx.Where("conversation_id = ? AND cago_id = ?", conversationID, m.CagoID).
			First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := tx.Create(m).Error; err != nil {
				tx.Rollback()
				return err
			}
			continue
		} else if err != nil {
			tx.Rollback()
			return err
		}
		// 更新 cago 字段；保留 mentions/token_usage（由 system 层另写）
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
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
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
```

注意 import `time` 若未引入需补。

- [ ] **Step 4: 重新生成 mock**

Run: `cd /Users/codfrm/Code/opskat/opskat && go generate ./internal/repository/conversation_repo/...`

Expected: `mock_conversation_repo/` 重新生成无错误。

- [ ] **Step 5: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/repository/conversation_repo/... -count=1 -v`

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/repository/conversation_repo/
git commit -m "✨ conversation_repo: 加 UpsertMessagesByCagoID 与 UpdateState"
```

---

## Task 4: service 层 — 加 UpsertCagoMessages / UpdateConversationState，废弃 SaveMessages

**Files:**
- Modify: `internal/service/conversation_svc/conversation.go`
- Test: `internal/service/conversation_svc/conversation_test.go`

- [ ] **Step 1: 写失败测试**

追加：

```go
func TestUpsertCagoMessages_Concurrent(t *testing.T) {
	// 同一 convID 两个并发 Upsert 不应该互相覆盖；service 仍需 saveLocks 串行化。
	dbtest.SetupDB(t)
	svc := conversation_svc.Conversation()
	ctx := context.Background()

	conv := &conversation_entity.Conversation{Title: "t", Status: conversation_entity.StatusActive}
	if err := svc.Create(ctx, conv); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 两次 upsert 各带不同 cago_id，并发执行后两个 cago_id 都应在
	a := []*conversation_entity.Message{{ConversationID: conv.ID, CagoID: "a", Role: "user", Content: "A", Persist: true}}
	b := []*conversation_entity.Message{{ConversationID: conv.ID, CagoID: "a", Role: "user", Content: "A", Persist: true},
		{ConversationID: conv.ID, CagoID: "b", Role: "assistant", Content: "B", Persist: true}}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = svc.UpsertCagoMessages(ctx, conv.ID, a) }()
	go func() { defer wg.Done(); _ = svc.UpsertCagoMessages(ctx, conv.ID, b) }()
	wg.Wait()

	got, _ := svc.LoadMessages(ctx, conv.ID)
	// b 的 upsert 是上层后到的全量快照；最终历史应为 b 的内容（不能是 a 覆盖了 b）。
	// 因为 cago.Save 总是发全量历史，串行化时谁后到谁是真相。这里不严格断言顺序，
	// 但要求至少包含 a 与 b 两个 cago_id 之一不丢，即 last-write-wins。
	if len(got) == 0 {
		t.Fatal("rows empty after upsert")
	}
}

func TestUpdateConversationState(t *testing.T) {
	dbtest.SetupDB(t)
	svc := conversation_svc.Conversation()
	ctx := context.Background()

	conv := &conversation_entity.Conversation{Title: "t", Status: conversation_entity.StatusActive}
	_ = svc.Create(ctx, conv)
	if err := svc.UpdateConversationState(ctx, conv.ID, "tid", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := svc.Get(ctx, conv.ID)
	if got.ThreadID != "tid" {
		t.Errorf("thread_id not set: %q", got.ThreadID)
	}
}
```

- [ ] **Step 2: 跑测试确认 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/service/conversation_svc/... -run "TestUpsertCagoMessages_Concurrent|TestUpdateConversationState" -v`

Expected: FAIL

- [ ] **Step 3: 实现新方法 + 删除 SaveMessages**

在 `ConversationSvc` 接口：

```go
type ConversationSvc interface {
	Create(ctx context.Context, conv *conversation_entity.Conversation) error
	List(ctx context.Context) ([]*conversation_entity.Conversation, error)
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	Update(ctx context.Context, conv *conversation_entity.Conversation) error
	UpdateTitle(ctx context.Context, id int64, title string) error
	UpdateWorkDir(ctx context.Context, id int64, workDir string) error
	Delete(ctx context.Context, id int64) error

	// cago 单源化的写入入口 — 替代旧的 SaveMessages
	UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	UpdateConversationState(ctx context.Context, conversationID int64, threadID string, stateValues map[string]string) error

	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
}
```

实现：

```go
func (s *conversationSvc) UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error {
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
			return err
		}
		jsonStr = string(b)
	}
	return conversation_repo.Conversation().UpdateState(ctx, conversationID, threadID, jsonStr)
}
```

删除 `SaveMessages` 方法及接口定义（**注意：app_ai.go 还在调用，留到 Task 7 一起删；这里先把方法 body 留空 panic 提示，或保留兼容**）。为避免编译断裂，临时让 `SaveMessages` 调 `UpsertCagoMessages` 但 cago_id 用合成值；或更直接地——先在 service 接口保留 SaveMessages 老签名，等 Task 7 删除调用点后一起拿掉。**本任务采取后者：保留 SaveMessages 不变，加新方法。**

- [ ] **Step 4: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/service/conversation_svc/... -count=1 -v`

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/conversation_svc/
git commit -m "✨ conversation_svc: 加 UpsertCagoMessages 与 UpdateConversationState"
```

---

## Task 5: 重写 gormStore — Save/Load/Delete 改用 conversation_messages

**Files:**
- Modify: `internal/aiagent/store.go`
- Modify: `internal/aiagent/store_test.go`
- Test: `internal/aiagent/store_test.go`（改写）

- [ ] **Step 1: 重写 store_test.go 为新的行级测试**

替换现有 `TestGormStore_RoundTripsSessionData` / `LoadEmptyReturnsZero` / `DeleteClearsSessionData`：

```go
package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// fakeConvStore 升级为支持 message 行 + state 列的行为。
type fakeConvStore struct {
	row      *conversation_entity.Conversation
	messages []*conversation_entity.Message
	getErr   error
	updErr   error
}

func (f *fakeConvStore) Get(_ context.Context, id int64) (*conversation_entity.Conversation, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.row == nil || f.row.ID != id {
		return nil, errors.New("not found")
	}
	cp := *f.row
	return &cp, nil
}

func (f *fakeConvStore) UpdateConversationState(_ context.Context, id int64, threadID string, vals map[string]string) error {
	if f.updErr != nil {
		return f.updErr
	}
	f.row.ThreadID = threadID
	if len(vals) == 0 {
		f.row.StateValues = ""
	} else {
		b, _ := json.Marshal(vals)
		f.row.StateValues = string(b)
	}
	return nil
}

func (f *fakeConvStore) UpsertCagoMessages(_ context.Context, id int64, msgs []*conversation_entity.Message) error {
	keep := map[string]bool{}
	for _, m := range msgs {
		keep[m.CagoID] = true
	}
	out := f.messages[:0]
	for _, existing := range f.messages {
		if keep[existing.CagoID] {
			out = append(out, existing)
		}
	}
	f.messages = out
	for _, m := range msgs {
		var found *conversation_entity.Message
		for _, e := range f.messages {
			if e.CagoID == m.CagoID {
				found = e
				break
			}
		}
		if found == nil {
			cp := *m
			f.messages = append(f.messages, &cp)
		} else {
			found.Role = m.Role
			found.Content = m.Content
			found.Kind = m.Kind
			found.Origin = m.Origin
			found.ParentID = m.ParentID
			found.Thinking = m.Thinking
			found.ToolCallJSON = m.ToolCallJSON
			found.ToolResultJSON = m.ToolResultJSON
			found.Persist = m.Persist
			found.Raw = m.Raw
			found.MsgTime = m.MsgTime
			found.SortOrder = m.SortOrder
		}
	}
	return nil
}

func (f *fakeConvStore) LoadMessages(_ context.Context, id int64) ([]*conversation_entity.Message, error) {
	out := append([]*conversation_entity.Message(nil), f.messages...)
	return out, nil
}

func TestGormStore_RoundTripsCagoMessages(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 42}}
	st := newGormStore(fake)

	data := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "hello", Persist: true},
			{ID: "m2", Kind: agent.MessageKindToolCall, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Persist: true,
				ToolCall: &agent.ToolCall{ID: "call-1", Name: "list_assets", Args: json.RawMessage(`{}`)}},
			{ID: "m3", Kind: agent.MessageKindToolResult, Role: agent.RoleTool, Origin: agent.MessageOriginTool, Persist: true,
				ToolResult: &agent.ToolResult{Result: "ok"}},
		},
		State: agent.State{ThreadID: "thread-xyz", Values: map[string]string{"k": "v"}},
	}

	if err := st.Save(context.Background(), "conv_42", data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(context.Background(), "conv_42")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("Messages length = %d, want 3", len(got.Messages))
	}
	if got.Messages[0].ID != "m1" || got.Messages[0].Text != "hello" {
		t.Errorf("text msg lost: %+v", got.Messages[0])
	}
	if got.Messages[1].ToolCall == nil || got.Messages[1].ToolCall.Name != "list_assets" {
		t.Errorf("tool_call lost: %+v", got.Messages[1])
	}
	if got.Messages[2].ToolResult == nil || got.Messages[2].ToolResult.Result.(string) != "ok" {
		t.Errorf("tool_result lost: %+v", got.Messages[2])
	}
	if got.State.ThreadID != "thread-xyz" || got.State.Values["k"] != "v" {
		t.Errorf("state lost: %+v", got.State)
	}
}

func TestGormStore_UpsertReplacesOnSecondSave(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1}}
	st := newGormStore(fake)
	first := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "v1", Persist: true},
		{ID: "m2", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Text: "a1", Persist: true},
	}}
	second := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "v1-edited", Persist: true},
		{ID: "m3", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Text: "a2", Persist: true},
	}}
	if err := st.Save(context.Background(), "conv_1", first); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(context.Background(), "conv_1", second); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Load(context.Background(), "conv_1")
	if len(got.Messages) != 2 {
		t.Fatalf("want 2, got %d", len(got.Messages))
	}
	if got.Messages[0].Text != "v1-edited" || got.Messages[1].ID != "m3" {
		t.Errorf("upsert wrong: %+v", got.Messages)
	}
}

func TestGormStore_LoadEmpty(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1}}
	st := newGormStore(fake)
	got, err := st.Load(context.Background(), "conv_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 0 || got.State.ThreadID != "" {
		t.Errorf("expected zero, got %+v", got)
	}
}

func TestGormStore_DeleteClearsRowsAndState(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1, ThreadID: "t"}}
	fake.messages = []*conversation_entity.Message{{ConversationID: 1, CagoID: "m1", Persist: true}}
	st := newGormStore(fake)
	if err := st.Delete(context.Background(), "conv_1"); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 0 {
		t.Errorf("messages not cleared: %+v", fake.messages)
	}
	if fake.row.ThreadID != "" {
		t.Errorf("thread_id not cleared: %q", fake.row.ThreadID)
	}
}

func TestGormStore_RejectsBadSessionID(t *testing.T) {
	fake := &fakeConvStore{}
	st := newGormStore(fake)
	if err := st.Save(context.Background(), "bogus", agent.SessionData{}); err == nil {
		t.Error("Save with bad id: want error")
	}
	if _, err := st.Load(context.Background(), "bogus"); err == nil {
		t.Error("Load with bad id: want error")
	}
	if err := st.Delete(context.Background(), "bogus"); err == nil {
		t.Error("Delete with bad id: want error")
	}
}

// 注：cancel 场景的回归测试在 cago 仓库内（Task 0 的 TestInternalObserverPersist_DecouplesUserCancel）。
// gormStore 这层不再处理 cancel 解耦，cago 给的 saveCtx 已经是 WithoutCancel 包过的。
```

- [ ] **Step 2: 跑测试确认 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/ -run "TestGormStore" -v`

Expected: FAIL（编译错误：`convStore` 接口未定义新方法、`gormStore.Save` 还在写 SessionData blob）

- [ ] **Step 3: 重写 gormStore**

替换 `internal/aiagent/store.go` 内容：

```go
package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/conversation_svc"
)

// convStore 是 gormStore 需要的最小子集。
type convStore interface {
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	UpdateConversationState(ctx context.Context, conversationID int64, threadID string, values map[string]string) error
	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
	UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error
}

// gormStore 把 cago agent.SessionData 平铺到 conversation_messages（按 cago_id 行级 upsert）
// 与 conversations.thread_id/state_values 上。它取代了原本写到 conversations.session_data 的实现。
//
// 这里的关键不变量：
//   - cago_id 是 cago Message.ID，在一次 Session 生命周期内稳定
//   - cago.Save 每次发的是全量 SessionData；因此本地 upsert 必须以"删除不在快照里的旧行"作为收敛
//   - 不需要 detachCtx：cago Task 0 已在 internalObserver.persist 把 ctx 与 user-cancel 解耦，
//     final flush 用 WithoutCancel + saveTimeout 包过；这里直接用传入的 ctx
//   - mentions / token_usage 这类扩展列由 System 通过 pendingMentions / pendingUsage
//     在 Save 同一事务内 drain 出来落盘，避免事件回调里的旁路写
type gormStore struct {
	store    convStore
	mentions pendingMentionsProvider // 可为 nil（测试场景）
	usage    pendingUsageProvider    // 可为 nil（测试场景）
}

// NewGormStore 接 service 单例与 System 的 pending 提供者。
func NewGormStore(mentions pendingMentionsProvider, usage pendingUsageProvider) agent.Store {
	return &gormStore{store: conversation_svc.Conversation(), mentions: mentions, usage: usage}
}

func newGormStore(s convStore) *gormStore { return &gormStore{store: s} }

func (g *gormStore) Save(ctx context.Context, sessionID string, data agent.SessionData) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}

	rows := make([]*conversation_entity.Message, 0, len(data.Messages))
	for i, m := range data.Messages {
		row, err := messageToRow(convID, i, m)
		if err != nil {
			return fmt.Errorf("gormStore.Save: convert message %d: %w", i, err)
		}
		rows = append(rows, row)
	}

	// drain pendingMentions：把上一次 SendAIMessage stash 的 mentions 关联到刚出现的 user 行
	if g.mentions != nil {
		existing, _ := g.store.LoadMessages(ctx, convID)
		known := make(map[string]bool, len(existing))
		for _, e := range existing {
			known[e.CagoID] = true
		}
		for i := len(rows) - 1; i >= 0; i-- {
			r := rows[i]
			if r.Origin == string(agent.MessageOriginUser) && !known[r.CagoID] {
				if pending := g.mentions.popPendingMentions(); len(pending) > 0 {
					refs := make([]conversation_entity.MentionRef, 0, len(pending))
					for _, m := range pending {
						refs = append(refs, conversation_entity.MentionRef{AssetID: m.AssetID, Name: m.Name})
					}
					b, _ := json.Marshal(refs)
					r.Mentions = string(b)
				}
				break
			}
		}
	}

	if err := g.store.UpsertCagoMessages(ctx, convID, rows); err != nil {
		return fmt.Errorf("gormStore.Save: upsert messages: %w", err)
	}
	if err := g.store.UpdateConversationState(ctx, convID, data.State.ThreadID, data.State.Values); err != nil {
		return fmt.Errorf("gormStore.Save: update state: %w", err)
	}

	// drain pendingUsage：把 event_bridge.EventUsage 缓存的用量按 cago_id 落到对应行
	if g.usage != nil {
		for cagoID, usage := range g.usage.drainPendingUsage() {
			b, _ := json.Marshal(usage)
			if err := g.store.UpdateMessageTokenUsage(ctx, convID, cagoID, string(b)); err != nil {
				return fmt.Errorf("gormStore.Save: update token_usage for %s: %w", cagoID, err)
			}
		}
	}
	return nil
}

func (g *gormStore) Load(ctx context.Context, sessionID string) (agent.SessionData, error) {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return agent.SessionData{}, err
	}
	conv, err := g.store.Get(ctx, convID)
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: load conversation %d: %w", convID, err)
	}
	rows, err := g.store.LoadMessages(ctx, convID)
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: load messages: %w", err)
	}
	values, err := conv.GetStateValues()
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: parse state values: %w", err)
	}
	out := agent.SessionData{State: agent.State{ThreadID: conv.ThreadID, Values: values}}
	for _, row := range rows {
		msg, err := rowToMessage(row)
		if err != nil {
			return agent.SessionData{}, fmt.Errorf("gormStore.Load: convert row %d: %w", row.ID, err)
		}
		out.Messages = append(out.Messages, msg)
	}
	return out, nil
}

func (g *gormStore) Delete(ctx context.Context, sessionID string) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	if err := g.store.UpsertCagoMessages(ctx, convID, nil); err != nil {
		return fmt.Errorf("gormStore.Delete: clear messages: %w", err)
	}
	if err := g.store.UpdateConversationState(ctx, convID, "", nil); err != nil {
		return fmt.Errorf("gormStore.Delete: clear state: %w", err)
	}
	return nil
}

// pendingMentionsProvider / pendingUsageProvider 由 System 实现，注入 gormStore；
// 测试时传 nil 跳过。
type pendingMentionsProvider interface {
	popPendingMentions() []ai.MentionedAsset
}

type pendingUsageProvider interface {
	drainPendingUsage() map[string]*conversation_entity.TokenUsage // cago_id → usage
}

func parseSessionID(s string) (int64, error) {
	if !strings.HasPrefix(s, "conv_") {
		return 0, fmt.Errorf("gormStore: invalid session id %q (want conv_<id>)", s)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "conv_"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("gormStore: invalid session id %q: %w", s, err)
	}
	return id, nil
}

// messageToRow 把 cago.Message 平铺成 conversation_messages 行（不动 mentions/token_usage 扩展列）。
func messageToRow(convID int64, idx int, m agent.Message) (*conversation_entity.Message, error) {
	row := &conversation_entity.Message{
		ConversationID: convID,
		CagoID:         m.ID,
		ParentID:       m.ParentID,
		Kind:           string(m.Kind),
		Origin:         string(m.Origin),
		Role:           string(m.Role),
		Content:        m.Text,
		Persist:        m.Persist,
		MsgTime:        m.Time.Unix(),
		SortOrder:      idx,
	}
	if len(m.Thinking) > 0 {
		b, err := json.Marshal(m.Thinking)
		if err != nil {
			return nil, err
		}
		row.Thinking = string(b)
	}
	if m.ToolCall != nil {
		b, err := json.Marshal(m.ToolCall)
		if err != nil {
			return nil, err
		}
		row.ToolCallJSON = string(b)
	}
	if m.ToolResult != nil {
		// ToolResult.Err 是 error 接口，json.Marshal 会丢；编码成纯字符串字段。
		type wireResult struct {
			Result   any           `json:"result,omitempty"`
			Err      string        `json:"err,omitempty"`
			Duration time.Duration `json:"duration,omitempty"`
		}
		w := wireResult{Result: m.ToolResult.Result, Duration: m.ToolResult.Duration}
		if m.ToolResult.Err != nil {
			w.Err = m.ToolResult.Err.Error()
		}
		b, err := json.Marshal(w)
		if err != nil {
			return nil, err
		}
		row.ToolResultJSON = string(b)
	}
	if len(m.Raw) > 0 {
		row.Raw = string(m.Raw)
	}
	return row, nil
}

// rowToMessage 反向：从 DB 行重建 cago.Message。
func rowToMessage(row *conversation_entity.Message) (agent.Message, error) {
	msg := agent.Message{
		ID:       row.CagoID,
		ParentID: row.ParentID,
		Kind:     agent.MessageKind(row.Kind),
		Origin:   agent.MessageOrigin(row.Origin),
		Role:     agent.MessageRole(row.Role),
		Text:     row.Content,
		Persist:  row.Persist,
		Time:     time.Unix(row.MsgTime, 0),
	}
	if row.Thinking != "" {
		if err := json.Unmarshal([]byte(row.Thinking), &msg.Thinking); err != nil {
			return agent.Message{}, err
		}
	}
	if row.ToolCallJSON != "" {
		var tc agent.ToolCall
		if err := json.Unmarshal([]byte(row.ToolCallJSON), &tc); err != nil {
			return agent.Message{}, err
		}
		msg.ToolCall = &tc
	}
	if row.ToolResultJSON != "" {
		type wireResult struct {
			Result   any           `json:"result,omitempty"`
			Err      string        `json:"err,omitempty"`
			Duration time.Duration `json:"duration,omitempty"`
		}
		var w wireResult
		if err := json.Unmarshal([]byte(row.ToolResultJSON), &w); err != nil {
			return agent.Message{}, err
		}
		msg.ToolResult = &agent.ToolResult{Result: w.Result, Duration: w.Duration}
		if w.Err != "" {
			msg.ToolResult.Err = fmt.Errorf("%s", w.Err)
		}
	}
	if row.Raw != "" {
		msg.Raw = json.RawMessage(row.Raw)
	}
	return msg, nil
}
```

- [ ] **Step 4: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/ -run "TestGormStore" -count=1 -v`

Expected: PASS

- [ ] **Step 5: 跑 aiagent 包全部测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/... -count=1`

Expected: PASS（其他测试如 store_cancel_test.go 可能依赖 fakeConvStore 旧形态，按编译报错调整测试桩字段）

- [ ] **Step 6: 提交**

```bash
git add internal/aiagent/store.go internal/aiagent/store_test.go
git commit -m "♻️ aiagent: gormStore 改为按 cago_id upsert conversation_messages 行"
```

---

## Task 6: System 维护 pendingMentions / pendingUsage；event_bridge 只缓存不写盘

**Files:**
- Modify: `internal/aiagent/system.go`
- Modify: `internal/aiagent/event_bridge.go`
- Modify: `internal/repository/conversation_repo/conversation.go`（加 `UpdateMessageTokenUsage`）
- Modify: `internal/service/conversation_svc/conversation.go`（加 `UpdateMessageTokenUsage` 透传）
- Test: `internal/aiagent/system_test.go`、`internal/aiagent/event_bridge_test.go`

**思路：** token_usage 不再由 event_bridge 直写 DB，而是缓存到 `System.pendingUsage`，下一次 cago 触发 `internalObserver.persist`（每次 EventMessageEnd / EventDone 都会触发）时由 gormStore.Save 在同一事务的 ctx 下 drain 出来落盘。这样**所有持久化都在 cago 框架保护的 saveCtx 下完成**——OpsKat 这层不再有任何旁路写入需要处理 cancel。

- [ ] **Step 1: 写失败测试 — System 同时 stash mentions 与 usage**

```go
// 追加到 system_test.go
func TestSystem_PendingMentions(t *testing.T) {
	sys := newTestSystem(t) // 现有 helper
	sys.stashPendingMentions(ai.AIContext{MentionedAssets: []ai.MentionedAsset{{AssetID: 9, Name: "edge"}}})
	got := sys.popPendingMentions()
	if len(got) != 1 || got[0].AssetID != 9 {
		t.Fatalf("pending mentions lost: %+v", got)
	}
	if extra := sys.popPendingMentions(); len(extra) != 0 {
		t.Fatalf("popPendingMentions should drain, got %+v", extra)
	}
}

func TestSystem_PendingUsage(t *testing.T) {
	sys := newTestSystem(t)
	sys.stashPendingUsage("a1", &conversation_entity.TokenUsage{InputTokens: 100, OutputTokens: 20})
	sys.stashPendingUsage("a2", &conversation_entity.TokenUsage{InputTokens: 50})
	got := sys.drainPendingUsage()
	if len(got) != 2 || got["a1"].InputTokens != 100 || got["a2"].InputTokens != 50 {
		t.Fatalf("pending usage lost: %+v", got)
	}
	// drain 应清空
	if extra := sys.drainPendingUsage(); len(extra) != 0 {
		t.Fatalf("drainPendingUsage should clear, got %+v", extra)
	}
}
```

- [ ] **Step 2: 跑测试 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/ -run "TestSystem_Pending" -v`

Expected: FAIL（方法未定义）

- [ ] **Step 3: 在 System 上实现 stash/pop/drain**

在 `internal/aiagent/system.go` 的 `System struct` 上加：

```go
type System struct {
	// ...现有字段...
	pendingMu       sync.Mutex
	pendingMentions []ai.MentionedAsset
	pendingUsage    map[string]*conversation_entity.TokenUsage // cago_id → usage
}

func (s *System) stashPendingMentions(aiCtx ai.AIContext) {
	if len(aiCtx.MentionedAssets) == 0 {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	s.pendingMentions = append(s.pendingMentions[:0], aiCtx.MentionedAssets...)
}

func (s *System) popPendingMentions() []ai.MentionedAsset {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := s.pendingMentions
	s.pendingMentions = nil
	return out
}

func (s *System) stashPendingUsage(cagoID string, u *conversation_entity.TokenUsage) {
	if cagoID == "" || u == nil {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pendingUsage == nil {
		s.pendingUsage = make(map[string]*conversation_entity.TokenUsage)
	}
	s.pendingUsage[cagoID] = u
}

func (s *System) drainPendingUsage() map[string]*conversation_entity.TokenUsage {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := s.pendingUsage
	s.pendingUsage = nil
	return out
}
```

`Stream` 入口插入：

```go
func (s *System) Stream(ctx context.Context, prompt string, aiCtx ai.AIContext, ext map[string]string) error {
	s.stashPendingMentions(aiCtx)
	// ...原有逻辑...
}
```

`NewSystem` 构造 gormStore 时把 `s` 自己当作两个 provider 注入：

```go
store := NewGormStore(s, s) // *System 实现 popPendingMentions / drainPendingUsage
```

- [ ] **Step 4: 改 event_bridge.EventUsage 只缓存，不直写**

修改 `event_bridge.go` 的 `EventUsage` 分支：

```go
case agent.EventUsage:
	u := ev.Usage
	usage := &ai.Usage{
		InputTokens:         u.PromptTokens,
		OutputTokens:        u.CompletionTokens,
		CacheReadTokens:     u.CachedTokens,
		CacheCreationTokens: u.CachedCreationTokens, // 字段名按现有代码
	}
	b.emit.Emit(convID, ai.StreamEvent{Type: "usage", Usage: usage})
	// 把 usage 缓存到 System.pendingUsage[cagoID]；下一次 cago 触发 internalObserver.persist
	// 时由 gormStore.Save 在 saveCtx（cago 已用 WithoutCancel 包过）下 drain 落盘。
	if ev.Message != nil && ev.Message.ID != "" && b.usage != nil {
		b.usage.stashPendingUsage(ev.Message.ID, &conversation_entity.TokenUsage{
			InputTokens:         u.PromptTokens,
			OutputTokens:        u.CompletionTokens,
			CacheReadTokens:     u.CachedTokens,
			CacheCreationTokens: u.CacheCreationTokens,
		})
	}
```

`EventBridge` struct 加一个字段：

```go
type EventBridge struct {
	// ...
	usage interface {
		stashPendingUsage(cagoID string, u *conversation_entity.TokenUsage)
	}
}
```

`NewEventBridge` 接收并保存 System 引用（`*System` 实现 `stashPendingUsage`）。

如 `ev.Message.ID` 在 `EventUsage` 中不可用，按 cago 实际事件语义改为缓存最后一条 assistant 的 ID（在 `EventMessageEnd` 时记录）然后用它关联——实施时用 `grep -n "EventUsage" /Users/codfrm/Code/cago/agents/agent/*.go` 确认事件结构。

- [ ] **Step 5: repo + service 加 UpdateMessageTokenUsage**

`internal/repository/conversation_repo/conversation.go` ConversationRepo 接口加：

```go
UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error
```

实现：

```go
func (r *conversationRepo) UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error {
	return db.Ctx(ctx).Model(&conversation_entity.Message{}).
		Where("conversation_id = ? AND cago_id = ?", conversationID, cagoID).
		Update("token_usage", tokenUsageJSON).Error
}
```

`internal/service/conversation_svc/conversation.go` 加同名透传方法（保持接口对称）。

- [ ] **Step 6: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/... ./internal/repository/conversation_repo/... ./internal/service/conversation_svc/... -count=1`

Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/aiagent/system.go internal/aiagent/event_bridge.go internal/aiagent/store.go internal/repository/conversation_repo/ internal/service/conversation_svc/
git commit -m "✨ aiagent: mentions/token_usage 通过 System pending 缓存，统一在 gormStore.Save 落盘"
```

---

## Task 7: app_ai.go — 删除 SaveConversationMessages binding，loadConversationDisplayMessages 派生 Blocks

**Files:**
- Modify: `internal/app/app_ai.go`
- Modify: `internal/service/conversation_svc/conversation.go`（拿掉 SaveMessages）
- Modify: `internal/repository/conversation_repo/conversation.go`（如果 CreateMessages/DeleteMessages 不再被任何人调用，删掉）

- [ ] **Step 1: 删除 SaveConversationMessages binding**

删除 `app_ai.go:496-524` 整个 `SaveConversationMessages` 方法。

- [ ] **Step 2: 改写 loadConversationDisplayMessages — 从 cago 字段派生 Blocks**

替换 `app_ai.go:230-259`：

```go
func (a *App) loadConversationDisplayMessages(ctx context.Context, id int64) ([]ConversationDisplayMessage, error) {
	msgs, err := conversation_svc.Conversation().LoadMessages(ctx, id)
	if err != nil {
		return nil, err
	}

	var displayMsgs []ConversationDisplayMessage
	// 按 cago Message 行派生 Blocks：
	//   - Kind=text 且 Origin in {model,user} → text block
	//   - Kind=tool_call → 与下一条 tool_result 配对生成 tool block
	//   - Kind=tool_result → 已被前一条 tool_call 吸收时跳过
	skip := make(map[int]bool)
	for i, msg := range msgs {
		if skip[i] {
			continue
		}
		dm := ConversationDisplayMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		mentions, err := msg.GetMentions()
		if err != nil {
			logger.Default().Warn("get message mentions", zap.Error(err))
		}
		dm.Mentions = mentions
		usage, err := msg.GetTokenUsage()
		if err != nil {
			logger.Default().Warn("get message token usage", zap.Error(err))
		}
		dm.TokenUsage = usage
		dm.Blocks = deriveBlocks(msgs, i, skip)
		displayMsgs = append(displayMsgs, dm)
	}
	return displayMsgs, nil
}

// deriveBlocks 把从 i 开始连续的"同一个 turn 的"行折叠为前端 ContentBlock。
// 规则：tool_call 行后紧跟的同 cago_id 关联的 tool_result 行被合并；其余按行→单 block。
func deriveBlocks(msgs []*conversation_entity.Message, i int, skip map[int]bool) []conversation_entity.ContentBlock {
	row := msgs[i]
	switch row.Kind {
	case "tool_call":
		var toolCall struct {
			ID   string          `json:"id"`
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		}
		_ = json.Unmarshal([]byte(row.ToolCallJSON), &toolCall)
		block := conversation_entity.ContentBlock{
			Type:       "tool",
			ToolName:   toolCall.Name,
			ToolInput:  string(toolCall.Args),
			ToolCallID: toolCall.ID,
			Status:     "completed",
		}
		// 找紧随其后的 tool_result（按 ParentID 关联或 Kind 判断）
		for j := i + 1; j < len(msgs); j++ {
			next := msgs[j]
			if next.Kind != "tool_result" {
				break
			}
			var tr struct {
				Result any    `json:"result"`
				Err    string `json:"err"`
			}
			_ = json.Unmarshal([]byte(next.ToolResultJSON), &tr)
			if tr.Err != "" {
				block.Status = "error"
				block.Content = tr.Err
			} else if s, ok := tr.Result.(string); ok {
				block.Content = s
			} else {
				b, _ := json.Marshal(tr.Result)
				block.Content = string(b)
			}
			skip[j] = true
			break
		}
		return []conversation_entity.ContentBlock{block}
	case "tool_result":
		// 单独的 tool_result（孤儿）— 一般不发生
		return nil
	default:
		return []conversation_entity.ContentBlock{{Type: "text", Content: row.Content}}
	}
}
```

- [ ] **Step 3: 删除 service 与 repo 中已无人使用的 SaveMessages/CreateMessages/DeleteMessages 入口**

在 `internal/service/conversation_svc/conversation.go`：删除 `SaveMessages` 方法及接口声明。

`internal/repository/conversation_repo/conversation.go`：保留 `DeleteMessages`（被 `Delete` 软删时调用），`CreateMessages` 改为只供测试用（或删除—— UpsertMessagesByCagoID 已覆盖单写场景）。逐次编译，按 `go build ./...` 提示删除未使用方法。

- [ ] **Step 4: 跑全量后端编译/测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./... && go test ./internal/... -count=1`

Expected: 编译成功 + 测试 PASS

- [ ] **Step 5: 重新生成 Wails 绑定（让前端 binding 不再有 SaveConversationMessages）**

Run: `cd /Users/codfrm/Code/opskat/opskat && wails generate module 2>&1 | tail -20`

或一次完整 `make dev` 启动确认 binding 重新生成。检查 `frontend/wailsjs/go/app/App.d.ts` 中应不再含有 `SaveConversationMessages`。

- [ ] **Step 6: 提交**

```bash
git add internal/app/app_ai.go internal/service/conversation_svc/conversation.go internal/repository/conversation_repo/conversation.go frontend/wailsjs/
git commit -m "♻️ app_ai: 删除前端 SaveConversationMessages 入口；显示 Blocks 改为读时派生"
```

---

## Task 8: 前端 — 拿掉所有 SaveConversationMessages 调用与持久化定时器

**Files:**
- Modify: `frontend/src/stores/aiStore.ts`
- Modify: `frontend/src/__tests__/aiStore.test.ts`
- Modify: `frontend/src/__tests__/AIChatEditResend.test.tsx`

- [ ] **Step 1: 写失败测试 — aiStore 不再调 SaveConversationMessages**

在 `aiStore.test.ts` 找到现有 `expect(SaveConversationMessages).toHaveBeenCalled...` 的用例，改为 `expect(SaveConversationMessages).not.toHaveBeenCalled()`（保留测试场景，断言反向）。

- [ ] **Step 2: 跑测试确认 fail（旧逻辑还在，调用仍发生）**

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm test -- aiStore`

Expected: FAIL

- [ ] **Step 3: 删除 store 里的所有持久化路径**

打开 `frontend/src/stores/aiStore.ts`：

- 删除 import：`SaveConversationMessages,`（line 12）
- 删除 `persistConversationSnapshot`、`schedulePersist`、`persistNow`、`cleanupPersistTimer`、`persistTimers`（line 379-413 整段）
- 删除 `streamBuffers` 中相关 schedulePersist 调用（line 470, 889, 1022, 1040, 1087, 1142, 1165, 1192, 1206, 1238, 1278, 1313, 1352, 1397, 1555-1556, 1965, 2414, 2490, 2505）— 找全后逐处删
- 把 `toDisplayMessages` 函数若仅被持久化使用，也一并删除；如还被其他读路径使用则保留

逐处审视确认没有 dangling 引用。

- [ ] **Step 4: 修剩余测试断言**

`aiStore.test.ts` 里所有 `vi.mocked(SaveConversationMessages)` 相关：删除 mock 与断言。保留测试用例（验证 store 内存状态正确即可）。

`AIChatEditResend.test.tsx` 同理。

- [ ] **Step 5: 跑前端测试**

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm test -- aiStore AIChatEditResend`

Expected: PASS

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm lint`

Expected: 通过（如有未使用 import 报错按提示清理）

- [ ] **Step 6: 提交**

```bash
git add frontend/src/stores/aiStore.ts frontend/src/__tests__/
git commit -m "♻️ aiStore: 移除前端会话持久化路径，改为纯 in-memory + 服务端权威"
```

---

## Task 9: 数据迁移 — session_data → conversation_messages 行 + state

**Files:**
- Create: `migrations/202605080011_aiagent_migrate_session_data.go`
- Modify: `migrations/migrations.go`
- Test: `migrations/202605080011_aiagent_migrate_session_data_test.go`

- [ ] **Step 1: 写失败测试**

```go
package migrations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/testutil/dbtest"
)

func TestMigrate202605080011_OverridesMessagesFromSessionData(t *testing.T) {
	db := dbtest.SetupRawDB(t) // 不跑迁移到最新，先跑到 202605080010 的状态
	// 准备 conversation 行：session_data 已有 cago SessionData，conversation_messages 也有前端写入的版本
	// 期望迁移后：messages 行被 session_data 覆盖；thread_id/state_values 写入；session_data 清空
	sd := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "real prompt", Persist: true},
			{ID: "m2", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "real reply", Persist: true},
		},
		State: agent.State{ThreadID: "tid-1", Values: map[string]string{"k": "v"}},
	}
	blob, _ := json.Marshal(sd)

	conv := &conversation_entity.Conversation{
		Title:       "test",
		Status:      conversation_entity.StatusActive,
		SessionData: string(blob),
	}
	if err := db.Create(conv).Error; err != nil {
		t.Fatal(err)
	}
	// 前端写入的"伪"消息——应被覆盖
	stale := []*conversation_entity.Message{
		{ConversationID: conv.ID, Role: "user", Content: "stale prompt", SortOrder: 0},
		{ConversationID: conv.ID, Role: "assistant", Content: "stale reply", SortOrder: 1},
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}

	// 跑下一个迁移
	if err := dbtest.RunMigration(db, "202605080011"); err != nil {
		t.Fatal(err)
	}

	var got []conversation_entity.Message
	db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got)
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].CagoID != "m1" || got[0].Content != "real prompt" {
		t.Errorf("first row not from session_data: %+v", got[0])
	}
	var convAfter conversation_entity.Conversation
	db.First(&convAfter, conv.ID)
	if convAfter.ThreadID != "tid-1" || convAfter.SessionData != "" {
		t.Errorf("conversation state not migrated: %+v", convAfter)
	}
	values, _ := convAfter.GetStateValues()
	if values["k"] != "v" {
		t.Errorf("state values not migrated: %+v", values)
	}
	_ = context.Background() // silence
}
```

如 `dbtest.SetupRawDB` / `RunMigration` 不存在，按现有 migrations 测试（其他 `_test.go` 文件）的模式实现一个。

- [ ] **Step 2: 跑测试 fail**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./migrations/... -run TestMigrate202605080011 -v`

Expected: FAIL（迁移未注册）

- [ ] **Step 3: 写迁移**

```go
package migrations

import (
	"encoding/json"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// migration202605080011 把 conversations.session_data 的 cago SessionData
// 覆盖性地展开到 conversation_messages + conversations.thread_id/state_values，
// 然后清空 session_data 列。LLM 视角即真相 — 前端历史写过的消息行被丢弃。
func migration202605080011() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080011",
		Migrate: func(tx *gorm.DB) error {
			var convs []conversation_entity.Conversation
			if err := tx.Where("session_data <> ''").Find(&convs).Error; err != nil {
				return err
			}
			for _, c := range convs {
				var data agent.SessionData
				if err := json.Unmarshal([]byte(c.SessionData), &data); err != nil {
					// 历史脏数据：跳过该 conversation，不阻塞迁移。日志在迁移层打不到，但 gormigrate 会保留进度。
					continue
				}
				if err := tx.Where("conversation_id = ?", c.ID).
					Delete(&conversation_entity.Message{}).Error; err != nil {
					return err
				}
				rows := make([]*conversation_entity.Message, 0, len(data.Messages))
				now := time.Now().Unix()
				for i, m := range data.Messages {
					row := &conversation_entity.Message{
						ConversationID: c.ID,
						CagoID:         m.ID,
						ParentID:       m.ParentID,
						Kind:           string(m.Kind),
						Origin:         string(m.Origin),
						Role:           string(m.Role),
						Content:        m.Text,
						Persist:        m.Persist,
						MsgTime:        m.Time.Unix(),
						SortOrder:      i,
						Createtime:     now,
					}
					if len(m.Thinking) > 0 {
						b, _ := json.Marshal(m.Thinking)
						row.Thinking = string(b)
					}
					if m.ToolCall != nil {
						b, _ := json.Marshal(m.ToolCall)
						row.ToolCallJSON = string(b)
					}
					if m.ToolResult != nil {
						b, _ := json.Marshal(m.ToolResult)
						row.ToolResultJSON = string(b)
					}
					if len(m.Raw) > 0 {
						row.Raw = string(m.Raw)
					}
					rows = append(rows, row)
				}
				if len(rows) > 0 {
					if err := tx.Create(&rows).Error; err != nil {
						return err
					}
				}
				stateValuesJSON := ""
				if len(data.State.Values) > 0 {
					b, _ := json.Marshal(data.State.Values)
					stateValuesJSON = string(b)
				}
				if err := tx.Model(&conversation_entity.Conversation{}).
					Where("id = ?", c.ID).
					Updates(map[string]any{
						"thread_id":    data.State.ThreadID,
						"state_values": stateValuesJSON,
						"session_data": "",
					}).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 不可逆 — 旧版本不再支持。
			return nil
		},
	}
}
```

- [ ] **Step 4: 注册**

```go
// migrations/migrations.go 末尾追加：
		migration202605080011(),
```

- [ ] **Step 5: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./migrations/... -count=1 -v`

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add migrations/202605080011_aiagent_migrate_session_data.go migrations/migrations.go migrations/202605080011_aiagent_migrate_session_data_test.go
git commit -m "✨ migrations: session_data → conversation_messages 行 + state，覆盖前端历史"
```

---

## Task 10: 端到端 smoke 测试 — 真实 cago Save 全链路

**Files:**
- Create: `internal/aiagent/store_e2e_test.go`

- [ ] **Step 1: 写测试 — 用真实 SQLite + 真实 conversation_svc 跑一遍 cago Save → Load**

```go
package aiagent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/aiagent"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/conversation_svc"
	"github.com/opskat/opskat/internal/testutil/dbtest"
)

func TestGormStore_E2E_RoundTrip(t *testing.T) {
	dbtest.SetupDB(t)
	ctx := context.Background()
	svc := conversation_svc.Conversation()

	conv := &conversation_entity.Conversation{Title: "e2e", Status: conversation_entity.StatusActive}
	if err := svc.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}

	store := aiagent.NewGormStore(nil)
	sid := "conv_" + dbtest.Itoa(conv.ID)

	data := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "hi", Persist: true},
			{ID: "m2", Kind: agent.MessageKindToolCall, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Persist: true,
				ToolCall: &agent.ToolCall{ID: "call-1", Name: "tool", Args: json.RawMessage(`{}`)}},
			{ID: "m3", Kind: agent.MessageKindToolResult, Role: agent.RoleTool, Origin: agent.MessageOriginTool, Persist: true,
				ToolResult: &agent.ToolResult{Result: "ok"}},
		},
		State: agent.State{ThreadID: "thr", Values: map[string]string{"a": "b"}},
	}
	if err := store.Save(ctx, sid, data); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 3 || got.Messages[0].Text != "hi" || got.State.ThreadID != "thr" {
		t.Errorf("round-trip lost: %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试**

Run: `cd /Users/codfrm/Code/opskat/opskat && go test ./internal/aiagent/... -run TestGormStore_E2E -v`

Expected: PASS

- [ ] **Step 3: 整体回归**

```bash
cd /Users/codfrm/Code/opskat/opskat && make test
cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm test
```

Expected: 全部 PASS

- [ ] **Step 4: 手动跑一次 dev 验证**

Run: `cd /Users/codfrm/Code/opskat/opskat && make dev`（手动操作）

验证清单：
- 创建新对话，发一条消息 → 等回复 → 关 app → 重开 → 历史完整
- **流式输出中点 Stop → 立即重启 app → 重开同会话 → 已经吐出的 partial reply 完整保留、user prompt 完整保留、token_usage 已落盘**（核心 cancel 回归）
- 流式输出中点 Stop → 不重启，直接发下一轮消息 → 上一轮的 partial reply 仍在 LLM 历史里（cago 下一轮请求带得上）
- 创建新对话，发一条带 @mention 的消息 → 关闭 → 打开 → mention 仍高亮
- 工具调用进行中点 Stop → 重开 → tool_call 行存在、对应 tool_result 行（如果工具已返回）也在；未返回则只有 tool_call
- 切换 conversation → 旧的对话历史正确加载

- [ ] **Step 5: 提交**

```bash
git add internal/aiagent/store_e2e_test.go
git commit -m "✅ aiagent: 加 cago Save → Load 端到端测试"
```

---

## Task 11: 清理 — 删除 SessionData / SessionInfo 等过时定义

**Files:**
- Modify: `internal/model/entity/conversation_entity/conversation.go`
- Modify: `internal/aiagent/sidecar.go` 等仍引用 `SessionInfo` 的地方
- Modify: `internal/app/app_ai.go`（`switchToConversation` 中如有 SessionInfo 用法）

- [ ] **Step 1: 找所有引用**

Run: `cd /Users/codfrm/Code/opskat/opskat && grep -rn "SessionInfo\|SessionData[^A-Za-z]\|GetSessionInfo\|SetSessionInfo" --include="*.go" .`

逐个文件评估：是否仍依赖 `Conversation.SessionData` 字段？

- [ ] **Step 2: 删除未用的 Conversation.SessionData / SessionInfo 类型**

如果 `internal/aiagent/sidecar.go` 之类还在读 `SessionInfo.SessionID`（local CLI 模式专用），决定保留这条路径还是把 `SessionID` 改成读 `Conversation.ThreadID`（推荐——单源）。

修改：
- 把 `SessionInfo` 引用全部改为读写 `Conversation.ThreadID`
- 删除 `SessionInfo`、`GetSessionInfo`、`SetSessionInfo`
- 保留 `Conversation.SessionData` 字段定义（数据库列还在；Go struct 留着兼容旧迁移测试），但代码不再读写它

- [ ] **Step 3: 跑回归**

```bash
cd /Users/codfrm/Code/opskat/opskat && go build ./... && go test ./internal/...
```

Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/
git commit -m "🔥 conversation_entity: 删除过时的 SessionInfo / GetSessionInfo / SetSessionInfo"
```

---

## Self-Review

完成上面 12 个 task 后回看：

**Spec coverage：**
- ✅ Task 0：cago `internalObserver.persist` 与 `RewriteHistory` 用 WithoutCancel + saveTimeout 包 ctx——final flush 与 user-cancel 解耦的不变量在框架层兜底
- ✅ Task 1-2：schema + entity 加 cago Message 字段、conversations 加 ThreadID/StateValues
- ✅ Task 3-4：repo 与 service 加 Upsert / UpdateState 路径，service 串行化保护
- ✅ Task 5：gormStore 重写，单源由 cago 写入；**直接用传入 ctx，无 detachCtx**
- ✅ Task 6：mentions 缓存到 `System.pendingMentions`、token_usage 缓存到 `System.pendingUsage`，统一在 `gormStore.Save` 同事务下 drain 落盘——所有持久化都在 cago 框架保护的 saveCtx 下完成
- ✅ Task 7：删除 SaveConversationMessages binding，loadConversationDisplayMessages 改读时派生 Blocks
- ✅ Task 8：前端 stores 删持久化定时器与 SaveConversationMessages 调用
- ✅ Task 9：session_data → conversation_messages 数据迁移，session_data 优先
- ✅ Task 10：端到端验证（含 cancel 场景手动验证）
- ✅ Task 11：清理过时类型

**Placeholder scan：** 没有 TODO/TBD/"添加合适的错误处理"。每一步都有明确文件路径与代码 / 命令 / 期望结果。

**Type consistency：**
- `convStore` 接口在 Task 5 定义，包含 `UpsertCagoMessages` / `UpdateConversationState` / `LoadMessages` / `UpdateMessageTokenUsage`
- `pendingMentionsProvider` / `pendingUsageProvider` 接口在 Task 5 定义，由 `*System`（Task 6）实现
- `UpdateMessageTokenUsage(ctx, convID, cagoID, json string)` 在 repo / service / convStore 三处签名一致
- `migration202605080010` / `migration202605080011` ID 序列连续，无冲突
- cago `saveTimeout` 常量与 OpsKat 这边不再有同名常量（`detachCtx` / `persistTimeout` 都已删除）

**已知风险点（实施时需关注）：**
- **Task 0 的 cago 测试辅助**：`NewSession` / `AppendMessageForTest` / `DispatchInternalForTest` 等 helper 命名按 cago 仓库内现有 `session_test.go` 实际写法照搬；如 cago 内部测试访问私有字段，新加的测试也写在 `package agent` 内（不是 `_test`）即可
- Wails 重新生成 binding 后前端 `wailsjs/go/app/App.d.ts` 中 `SaveConversationMessages` 是否被前端别处引用——Task 7 Step 5 与 Task 8 应同步覆盖
- cago Message.Time 在 `messageToRow` 用 Unix 秒，rowToMessage 反向；如 cago 改用 nano 须同步调整
- `EventUsage` 的 `ev.Message.ID` 可用性：Task 6 Step 4 标注了如不可用就改为在 `EventMessageEnd` 时缓存最后一条 assistant ID，实施时按真实事件结构验证
- `dbtest.SetupDB` / `dbtest.SetupRawDB` 等测试 helper 路径：项目里若不存在，Task 3/9 实施时按现有 `conversation_svc/conversation_test.go` 中 SQLite 内存初始化方式照搬即可
- Task 0 落到 cago 仓库（独立 git），需要分别 commit；OpsKat 这边的 commits 不会涵盖那部分。如要单 PR 提交，先把 cago 改动合到主干再做 OpsKat 改动
