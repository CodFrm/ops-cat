# aiagent cago v2 升级 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 OpsKat `internal/aiagent/` 从 cago agents v1（Session/Stream/8-stage Hook）整体迁到 cago agents v2（Conversation+Runner / iter.Seq[Event] / 4-stage Hook / Conv.Watch 增量持久化），过程中删 god-object、`pendingMentions/Usage/Displays` 三处 stash、自家 retry/sidecar；维持前端 Wails StreamEvent 契约不变。

**Architecture:** D 形态——`*Manager`（单例 cago `*agent.Agent` + per-conv `*ConvHandle` map）+ `*ConvHandle`（持长生命 `*agent.Conversation` + `*agent.Runner`，统一入口 `Send` 优先 Steer + ErrSteerNoActiveTurn 兜底）。持久化走 `agent/store.Recorder.Bind(conv)` + 自定义 `agent.Store` 实现写 `conversation_messages`。Hook 以独立文件存在，PreToolUse / PostToolUse / UserPromptSubmit / TurnEnd 分别承载策略 / 审计 / mention / 子 agent。配套 cago 上游 PR 加 `WithSendDisplay` / `WithSteerDisplay` / `MetadataBlock`，让"展示文本 ≠ LLM body" 由 cago 直接承担。

**Tech Stack:** Go 1.25 / cago `agent` v2 / GORM + SQLite / gormigrate / GoConvey + testify + go.uber.org/mock + go-sqlmock / providertest（cago）/ Wails v2

**Spec:** `docs/superpowers/specs/2026-05-10-aiagent-cago-v2-upgrade-design.md`

---

## 上下文 / 入门须知

**两个仓库**，分两个工作树：

| 仓库 | 路径 | 作用 |
|---|---|---|
| cago agents | `/Users/codfrm/Code/cago/agents` | Phase 1 改这里（`agent` 包加 MetadataBlock + WithSendDisplay + WithSteerDisplay）|
| OpsKat | `/Users/codfrm/Code/opskat/opskat`（本工作树）| Phase 2-5 全在这里 |

OpsKat `go.mod` 已 `replace github.com/cago-frame/agents => /Users/codfrm/Code/cago/agents`，cago 改完 OpsKat 立即可用，无需 push。

**测试框架**：

- Go 单测：`goconvey` + `testify/assert` + `go.uber.org/mock` + `go-sqlmock` + `miniredis`
- cago 端：纯 `testing` + `provider/providertest`（mock provider，喂 chunk 序列）
- 跑单测：`go test -v -run TestXxx ./internal/aiagent/...`
- 全量：`make test`
- Lint：`make lint`

**Mock 生成**：每个 `mock_*/` 子目录顶部有 `//go:generate mockgen -source=...`；规模生成走 `go generate ./...`。

**当前分支**：`worktree-replace-builtin-agent-with-cago`（OpsKat 端）。直接在该分支累积子 commit。cago 端独立分支。

**提交风格**：gitmoji——✨ 新功能 / 🐛 bugfix / ♻️ 重构 / 🔥 删除 / ✅ 测试 / 🔧 配置。Chinese commit messages（见现有 git log）。

---

## Phase 1 — cago 上游 PR（在 `/Users/codfrm/Code/cago/agents`）

### Task 1: cago — 新增 MetadataBlock 类型

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/agent/content_blocks.go`
- Test: `/Users/codfrm/Code/cago/agents/agent/content_blocks_test.go`

- [ ] **Step 1: Write failing test**

新建 `agent/content_blocks_test.go`（如已有就追加）：

```go
package agent_test

import (
	"testing"

	"github.com/cago-frame/agents/agent"
)

func TestMetadataBlock_ContentBlockType(t *testing.T) {
	b := agent.MetadataBlock{Key: "display", Value: "raw text"}
	if got := b.ContentBlockType(); got != "metadata" {
		t.Fatalf("ContentBlockType() = %q, want %q", got, "metadata")
	}
}

func TestMetadataBlock_ImplementsContentBlock(t *testing.T) {
	var _ agent.ContentBlock = agent.MetadataBlock{} // compile-time check
}
```

- [ ] **Step 2: Run to verify failure**

```
cd /Users/codfrm/Code/cago/agents
go test ./agent -run TestMetadataBlock -v
```

Expected: `undefined: agent.MetadataBlock` — compile error.

- [ ] **Step 3: Add MetadataBlock to content_blocks.go**

在 `agent/content_blocks.go` 末尾追加：

```go
// MetadataBlock carries side-channel data attached to a Message that should
// be persisted (and visible to ConversationReader-consuming Hooks) but is
// NOT included in the ChatRequest sent to the LLM. Use cases include the
// pre-mention-expansion display text (so the UI can render the user's raw
// input while the LLM sees the expanded body), tool refs, asset refs, etc.
//
// Value is serialized verbatim by Recorder/Store impls; no schema is enforced
// — caller-defined.
type MetadataBlock struct {
	Key   string
	Value any
}

func (MetadataBlock) ContentBlockType() string { return "metadata" }
```

- [ ] **Step 4: Run to verify pass**

```
go test ./agent -run TestMetadataBlock -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
cd /Users/codfrm/Code/cago/agents
git checkout -b feat/metadata-block-and-display-options
git add agent/content_blocks.go agent/content_blocks_test.go
git commit -m "✨ agent: add MetadataBlock for side-channel data on Message"
```

---

### Task 2: cago — InputBuilder 跳过 MetadataBlock

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/agent/inputbuilder.go`
- Test: `/Users/codfrm/Code/cago/agents/agent/inputbuilder_test.go`

- [ ] **Step 1: Write failing test**

在 `agent/inputbuilder_test.go` 末尾追加：

```go
func TestBuildRequest_StripsMetadataBlock(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: []agent.ContentBlock{
			agent.TextBlock{Text: "hello @srv1"},
			agent.MetadataBlock{Key: "display", Value: "hi @srv1"},
		}},
	}
	req := agent.BuildRequest(agent.RequestSpec{Messages: msgs})
	if len(req.Messages) != 1 {
		t.Fatalf("got %d msgs, want 1", len(req.Messages))
	}
	got := req.Messages[0].Content
	if got != "hello @srv1" {
		t.Fatalf("Content = %q, want %q (MetadataBlock should be stripped)", got, "hello @srv1")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./agent -run TestBuildRequest_StripsMetadataBlock -v
```

Expected: 失败——FAIL，可能 panic 或 Content 含 metadata 字符串（取决于现 `convertMessage` 怎么处理未识别 block）。

- [ ] **Step 3: Modify convertMessage to skip MetadataBlock**

在 `agent/inputbuilder.go` 的 `convertMessage` 函数 `for _, b := range m.Content` switch 里加一个 case（放在 TextBlock 之后、ToolUseBlock 之前）：

```go
case MetadataBlock:
    // not sent to LLM — UI/persistence-only side channel
    continue
```

- [ ] **Step 4: Run to verify pass**

```
go test ./agent -run TestBuildRequest -v
```

Expected: 全 PASS（含已有 TestBuildRequest_*）。

- [ ] **Step 5: Commit**

```bash
git add agent/inputbuilder.go agent/inputbuilder_test.go
git commit -m "✨ agent: InputBuilder skips MetadataBlock when sending to LLM"
```

---

### Task 3: cago — `WithSendDisplay` SendOption

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/agent/send_options.go`
- Test: `/Users/codfrm/Code/cago/agents/agent/send_options_test.go`

- [ ] **Step 1: Write failing test**

在 `agent/send_options_test.go` 末尾追加：

```go
func TestWithSendDisplay_AppendsMetadataBlock(t *testing.T) {
	blocks, err := agent.BuildUserContentForTest("hello", []agent.SendOption{
		agent.WithSendDisplay("hello @srv1"),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	tb, ok := blocks[0].(agent.TextBlock)
	if !ok || tb.Text != "hello" {
		t.Fatalf("blocks[0] = %#v, want TextBlock{hello}", blocks[0])
	}
	mb, ok := blocks[1].(agent.MetadataBlock)
	if !ok || mb.Key != "display" || mb.Value != "hello @srv1" {
		t.Fatalf("blocks[1] = %#v, want MetadataBlock{display, hello @srv1}", blocks[1])
	}
}
```

注：`BuildUserContentForTest` 是这个测试需要的导出别名。如果 `buildUserContent` 当前是私有，需要在 `send_options.go` 的同包里加一个 test-only 导出（`agent_export_test.go`），或者直接在测试里调用未导出函数（同包测试文件 `package agent` 即可）。**改测试文件的 package 为 `package agent`**，直接调用 `buildUserContent` —— 不引入 export-only 别名。重写：

```go
package agent

import "testing"

func TestWithSendDisplay_AppendsMetadataBlock(t *testing.T) {
	blocks, err := buildUserContent("hello", []SendOption{
		WithSendDisplay("hello @srv1"),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].(TextBlock).Text != "hello" {
		t.Fatalf("blocks[0] mismatch: %#v", blocks[0])
	}
	mb, ok := blocks[1].(MetadataBlock)
	if !ok || mb.Key != "display" || mb.Value != "hello @srv1" {
		t.Fatalf("blocks[1] = %#v, want MetadataBlock{display, hello @srv1}", blocks[1])
	}
}
```

放到 `agent/send_options_internal_test.go`（`package agent` 内部测试文件）。

- [ ] **Step 2: Run to verify failure**

```
go test ./agent -run TestWithSendDisplay -v
```

Expected: `undefined: WithSendDisplay`。

- [ ] **Step 3: Implement WithSendDisplay**

在 `agent/send_options.go` 末尾追加：

```go
// WithSendDisplay attaches a display-text MetadataBlock to the user message
// being sent. The block is persisted with the message and visible to Hooks
// reading via ConversationReader, but is stripped by InputBuilder before the
// message reaches the LLM. Use this when the chat UI wants to show the user's
// raw input (e.g. "@srv1 status") while the LLM receives a different body
// (the mention-expanded form).
//
// Multiple WithSendDisplay calls each append a separate block; consumers
// typically read only the first one.
func WithSendDisplay(displayText string) SendOption {
	return func(c *sendConfig) {
		c.extraBlocks = append(c.extraBlocks, MetadataBlock{Key: "display", Value: displayText})
	}
}
```

- [ ] **Step 4: Run to verify pass**

```
go test ./agent -run TestWithSendDisplay -v
go test ./agent -run TestBuildUser -v   # 顺便验证 buildUserContent 现有 case 不挂
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add agent/send_options.go agent/send_options_internal_test.go
git commit -m "✨ agent: add WithSendDisplay SendOption (display text != LLM body)"
```

---

### Task 4: cago — `Runner.Steer` 加 opts...，`WithSteerDisplay`

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/agent/runner.go`
- Test: `/Users/codfrm/Code/cago/agents/agent/runner_steer_test.go`

- [ ] **Step 1: Write failing test**

在 `agent/runner_steer_test.go` 末尾追加：

```go
func TestSteer_PreservesDisplayMetadata(t *testing.T) {
	// turn 1: 慢流，等 user Steer 注入后让 turn 自然结束
	prov := providertest.New().
		QueueStreamFunc(func(ctx context.Context) <-chan provider.StreamChunk {
			ch := make(chan provider.StreamChunk, 8)
			go func() {
				defer close(ch)
				for _, r := range "hello world" {
					select {
					case <-ctx.Done():
						return
					case ch <- provider.StreamChunk{ContentDelta: string(r)}:
					}
					time.Sleep(5 * time.Millisecond)
				}
				ch <- provider.StreamChunk{FinishReason: provider.FinishStop}
			}()
			return ch
		}).
		QueueStream(
			provider.StreamChunk{ContentDelta: "ack"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

	a := agent.New(prov)
	conv := agent.NewConversation()
	r := a.Runner(conv)
	defer func() { _ = r.Close() }()

	events, err := r.Send(context.Background(), "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	go func() {
		// 流出第一个字符后 steer
		time.Sleep(8 * time.Millisecond)
		_ = r.Steer(context.Background(), "expanded body", agent.WithSteerDisplay("@raw display"))
	}()
	for range events {
	}

	// 找到被 steer 注入的 user msg：role=user，应当含 MetadataBlock{Key:"display"}
	var found bool
	for i := 0; i < conv.Len(); i++ {
		m, _ := conv.MessageAt(i)
		if m.Role != agent.RoleUser {
			continue
		}
		for _, b := range m.Content {
			if mb, ok := b.(agent.MetadataBlock); ok && mb.Key == "display" && mb.Value == "@raw display" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("steer'd user msg missing display MetadataBlock; conv=%+v", conv.Messages())
	}
}

func TestSteer_NoOptsBackwardCompat(t *testing.T) {
	// 确保不传 opts 时旧行为不变
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "x"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	a := agent.New(prov)
	conv := agent.NewConversation()
	r := a.Runner(conv)
	defer func() { _ = r.Close() }()
	events, _ := r.Send(context.Background(), "hi")
	for range events {
	}
	// turn 已结束，Steer 应返 ErrSteerNoActiveTurn（不传 opts 也要兼容）
	if err := r.Steer(context.Background(), "late"); !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		t.Fatalf("want ErrSteerNoActiveTurn, got %v", err)
	}
}
```

测试文件需要 import：`context`, `errors`, `testing`, `time`, `agent`, `provider`, `providertest`。

- [ ] **Step 2: Run to verify failure**

```
go test ./agent -run "TestSteer_PreservesDisplayMetadata|TestSteer_NoOptsBackwardCompat" -v
```

Expected: 失败——`Steer` 当前签名只有 `(ctx, text)`，传 opts 编译失败。

- [ ] **Step 3: 改 Steer 签名 + 加 SteerOption**

在 `agent/runner.go` 找到 `func (r *Runner) Steer(...)`，改为：

```go
// SteerOption tunes Steer behavior; sealed.
type SteerOption func(*steerConfig)

type steerConfig struct {
	displayText string
}

// WithSteerDisplay attaches a display-text MetadataBlock to the steer'd
// user message. Mirror of WithSendDisplay for steer-time injection.
func WithSteerDisplay(displayText string) SteerOption {
	return func(c *steerConfig) { c.displayText = displayText }
}

// Steer enqueues a user message to be injected at the next safe point in the
// current turn (after tool dispatch, or after a no-tool LLM completion via
// auto-continuation, before the next LLM call). Returns ErrSteerNoActiveTurn
// if the turn has already decided to exit — at that point use Send to start
// a new turn instead.
//
// SteerOption can attach side-channel metadata via WithSteerDisplay; the
// metadata persists with the message but is stripped by InputBuilder before
// the LLM sees it.
func (r *Runner) Steer(ctx context.Context, text string, opts ...SteerOption) error {
	cfg := &steerConfig{}
	for _, o := range opts {
		o(cfg)
	}
	r.mu.Lock()
	if !r.steerOpen {
		r.mu.Unlock()
		return ErrSteerNoActiveTurn
	}
	r.mu.Unlock()
	r.steerMu.Lock()
	r.steerQueue = append(r.steerQueue, steerEntry{text: text, displayText: cfg.displayText})
	r.steerMu.Unlock()
	return nil
}
```

注意：`steerQueue` 元素类型从 `string` 改为 `steerEntry`。新增结构：

```go
type steerEntry struct {
	text        string
	displayText string
}
```

`drainSteer()` 也跟着改：

```go
func (r *Runner) drainSteer() []steerEntry {
	r.steerMu.Lock()
	defer r.steerMu.Unlock()
	if len(r.steerQueue) == 0 {
		return nil
	}
	out := r.steerQueue
	r.steerQueue = nil
	return out
}
```

`steerQueue` 字段类型改 `[]steerEntry`：在 Runner struct 找到 `steerQueue []string` 改为 `steerQueue []steerEntry`。

- [ ] **Step 4: 找到 drainSteer 的 caller，注入 displayText**

`grep -n drainSteer agent/loop.go agent/runner.go`，找到把队列内容变成 user message 的地方。把 `text` 直接做 user msg 改为：

```go
// 假设原代码大概是：
// for _, t := range r.drainSteer() {
//     conv.Append(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: t}}})
// }
// 改为：
for _, e := range r.drainSteer() {
    content := []ContentBlock{TextBlock{Text: e.text}}
    if e.displayText != "" {
        content = append(content, MetadataBlock{Key: "display", Value: e.displayText})
    }
    conv.Append(Message{Role: RoleUser, Content: content})
}
```

具体行用 `grep -n "drainSteer\|steerQueue" agent/loop.go` 定位。

- [ ] **Step 5: Run to verify pass**

```
go test ./agent -run TestSteer -v
go test ./agent -v   # 全包跑一遍，确保没破坏其它 steer 测试
```

Expected: 全 PASS。

- [ ] **Step 6: Commit**

```bash
git add agent/runner.go agent/loop.go agent/runner_steer_test.go
git commit -m "✨ agent: Steer accepts SteerOption; add WithSteerDisplay"
```

---

### Task 5: cago — 全包 lint + 提 PR

- [ ] **Step 1: 全包测试**

```
cd /Users/codfrm/Code/cago/agents
make test
make lint
```

Expected: 全绿。

- [ ] **Step 2: 检查 OpsKat 仓库该 cago 版本编译还能过**

```
cd /Users/codfrm/Code/opskat/opskat
go build ./...
```

Expected: 全编译过（OpsKat 还在用 v1 调用，不影响）。如果失败，看是否 cago Steer 的旧调用方在 OpsKat 内部——通常只在 `internal/aiagent/system.go` 的 `Session.FollowUp`，OpsKat 这次升级前还在用 v1 Session，与新加的 Steer opts 没冲突。

- [ ] **Step 3: 推送 cago PR**

```bash
cd /Users/codfrm/Code/cago/agents
git push -u origin feat/metadata-block-and-display-options
gh pr create --title "✨ agent: MetadataBlock + WithSendDisplay/WithSteerDisplay" \
  --body "$(cat <<'EOF'
## Summary

- 新增 `MetadataBlock{Key, Value}` ContentBlock：携带 side-channel 数据（如 mention 展开前的展示文本），InputBuilder 跳过、Recorder/Store 透传
- 新增 `WithSendDisplay(displayText)` SendOption：在 user msg 上挂 display MetadataBlock
- `Runner.Steer` 接受 `SteerOption` 变参；新增 `WithSteerDisplay(displayText)`

## Motivation

Chat 类 agent 普遍存在"展示文本 ≠ LLM body"（mention/macro 展开）。当前下游靠旁路 FIFO 维护对齐，bug-prone。把这层语义放到 cago 解决一次。

## Test plan

- [x] `agent/content_blocks_test.go::TestMetadataBlock_*`
- [x] `agent/inputbuilder_test.go::TestBuildRequest_StripsMetadataBlock`
- [x] `agent/send_options_internal_test.go::TestWithSendDisplay_AppendsMetadataBlock`
- [x] `agent/runner_steer_test.go::TestSteer_PreservesDisplayMetadata`
- [x] `agent/runner_steer_test.go::TestSteer_NoOptsBackwardCompat`
- [x] `make test` && `make lint`
EOF
)"
```

- [ ] **Step 4: PR merge 后 OpsKat 这边 bump go.mod**

PR 合并后回到 OpsKat：

```
cd /Users/codfrm/Code/opskat/opskat
go get github.com/cago-frame/agents@<merged-commit-or-tag>
go mod tidy
```

如果暂时不想等 PR 合并，本地 `replace` 已经指向本地 cago 工作树，直接进 Phase 2 即可。

- [ ] **Step 5: Commit OpsKat 端 bump**

```bash
git add go.mod go.sum
git commit -m "🔧 deps: bump cago/agents to MetadataBlock + display opts"
```

---

## Phase 2 — OpsKat schema 演进

### Task 6: 新增 schema 迁移文件

**Files:**
- Create: `migrations/202605100001_aiagent_v2_schema.go`
- Test: `migrations/202605100001_aiagent_v2_schema_test.go`
- Modify: `migrations/migrations.go`（注册新迁移）

- [ ] **Step 1: Write failing test**

新建 `migrations/202605100001_aiagent_v2_schema_test.go`：

```go
package migrations

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func Test202605100001_DropsLegacyColumnsAndAddsPartialReason(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// 模拟一份 v1 schema：conversation_messages 含老列
	assert.NoError(t, db.Exec(`
		CREATE TABLE conversation_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_calls TEXT,
			tool_call_id TEXT,
			blocks TEXT,
			mentions TEXT,
			token_usage TEXT,
			sort_order INTEGER DEFAULT 0,
			createtime INTEGER,
			cago_id TEXT,
			parent_id TEXT,
			kind TEXT,
			origin TEXT,
			thinking TEXT,
			tool_call_json TEXT,
			tool_result_json TEXT,
			persist INTEGER DEFAULT 1,
			raw TEXT,
			msg_time INTEGER
		)
	`).Error)

	// 插入一行带 v1 内容
	assert.NoError(t, db.Exec(`
		INSERT INTO conversation_messages
		(conversation_id, role, content, blocks, sort_order, cago_id, kind, origin)
		VALUES (1, 'user', 'hello', '[{"type":"text","content":"hello"}]', 0, 'old-cago-id-1', 'message', 'user')
	`).Error)

	// 跑迁移
	mig := migration202605100001()
	assert.NoError(t, mig.Migrate(db))

	// 老列应当被删
	var count int
	assert.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, "cago_id").Scan(&count).Error)
	assert.Equal(t, 0, count, "cago_id should be dropped")
	assert.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, "parent_id").Scan(&count).Error)
	assert.Equal(t, 0, count, "parent_id should be dropped")
	assert.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, "content").Scan(&count).Error)
	assert.Equal(t, 0, count, "content should be dropped")

	// 新列 partial_reason 加上
	assert.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, "partial_reason").Scan(&count).Error)
	assert.Equal(t, 1, count, "partial_reason should exist")

	// 唯一索引 (conversation_id, sort_order) 存在
	var idxName string
	assert.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='conversation_messages' AND name=?`, "idx_conv_msg_unique").Scan(&idxName).Error)
	assert.Equal(t, "idx_conv_msg_unique", idxName)

	// 行数据保留：blocks/mentions/role/sort_order
	var blocks, role string
	var sortOrder int
	assert.NoError(t, db.Raw(`SELECT blocks, role, sort_order FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocks, &role, &sortOrder))
	assert.Contains(t, blocks, "hello")
	assert.Equal(t, "user", role)
	assert.Equal(t, 0, sortOrder)
}

func Test202605100001_BackfillsContentToBlocks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	assert.NoError(t, db.Exec(`
		CREATE TABLE conversation_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_calls TEXT,
			tool_call_id TEXT,
			blocks TEXT,
			mentions TEXT,
			token_usage TEXT,
			sort_order INTEGER DEFAULT 0,
			createtime INTEGER,
			cago_id TEXT,
			parent_id TEXT,
			kind TEXT,
			origin TEXT,
			thinking TEXT,
			tool_call_json TEXT,
			tool_result_json TEXT,
			persist INTEGER DEFAULT 1,
			raw TEXT,
			msg_time INTEGER
		)
	`).Error)

	// 一行只有 content，没有 blocks
	assert.NoError(t, db.Exec(`
		INSERT INTO conversation_messages (conversation_id, role, content, blocks, sort_order)
		VALUES (1, 'assistant', 'fallback text', '', 0)
	`).Error)

	mig := migration202605100001()
	assert.NoError(t, mig.Migrate(db))

	var blocks string
	assert.NoError(t, db.Raw(`SELECT blocks FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocks))
	assert.Contains(t, blocks, "fallback text", "blocks should contain backfilled content")
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./migrations -run Test202605100001 -v
```

Expected: `undefined: migration202605100001`。

- [ ] **Step 3: 写迁移**

新建 `migrations/202605100001_aiagent_v2_schema.go`：

```go
package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605100001 把 conversation_messages 从 cago v1 平铺形态升到
// v2：删 v1 字段（cago_id / parent_id / kind / origin / persist /
// tool_call_id / tool_calls / thinking / tool_call_json / tool_result_json /
// raw / content / msg_time），加 partial_reason，建 (conversation_id,
// sort_order) 唯一索引；空 blocks 行的 content 字段 backfill 进 blocks。
//
// SQLite 的 ALTER TABLE 限制：不能直接 DROP COLUMN（取决于版本）；这里走
// "建新表 → copy 数据 → drop 旧表 → rename" 的 CTAS 套路。
func migration202605100001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605100001_aiagent_v2_schema",
		Migrate: func(tx *gorm.DB) error {
			// 1. backfill：blocks 为空时把 content 折叠进去
			rows, err := tx.Raw(`
				SELECT id, content
				FROM conversation_messages
				WHERE (blocks IS NULL OR blocks = '') AND content != ''
			`).Rows()
			if err != nil {
				return fmt.Errorf("backfill scan: %w", err)
			}
			type pair struct {
				id      int64
				content string
			}
			var todo []pair
			for rows.Next() {
				var p pair
				if err := rows.Scan(&p.id, &p.content); err != nil {
					rows.Close()
					return fmt.Errorf("backfill row: %w", err)
				}
				todo = append(todo, p)
			}
			rows.Close()
			for _, p := range todo {
				blob, err := json.Marshal([]map[string]string{
					{"type": "text", "text": p.content},
				})
				if err != nil {
					return fmt.Errorf("backfill marshal id=%d: %w", p.id, err)
				}
				if err := tx.Exec(`UPDATE conversation_messages SET blocks=? WHERE id=?`, string(blob), p.id).Error; err != nil {
					return fmt.Errorf("backfill update id=%d: %w", p.id, err)
				}
			}

			// 2. CTAS：新表 conversation_messages_v2
			if err := tx.Exec(`
				CREATE TABLE conversation_messages_v2 (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					conversation_id INTEGER NOT NULL,
					role TEXT NOT NULL,
					blocks TEXT,
					mentions TEXT,
					token_usage TEXT,
					partial_reason TEXT NOT NULL DEFAULT '',
					sort_order INTEGER NOT NULL DEFAULT 0,
					createtime INTEGER
				)
			`).Error; err != nil {
				return fmt.Errorf("create v2 table: %w", err)
			}

			// 3. copy 数据
			if err := tx.Exec(`
				INSERT INTO conversation_messages_v2
				(id, conversation_id, role, blocks, mentions, token_usage, partial_reason, sort_order, createtime)
				SELECT id, conversation_id, role,
				       COALESCE(blocks, ''),
				       COALESCE(mentions, ''),
				       COALESCE(token_usage, ''),
				       '' as partial_reason,
				       COALESCE(sort_order, 0),
				       COALESCE(createtime, 0)
				FROM conversation_messages
			`).Error; err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			// 4. drop 旧表 + rename
			if err := tx.Exec(`DROP TABLE conversation_messages`).Error; err != nil {
				return fmt.Errorf("drop old: %w", err)
			}
			if err := tx.Exec(`ALTER TABLE conversation_messages_v2 RENAME TO conversation_messages`).Error; err != nil {
				return fmt.Errorf("rename: %w", err)
			}

			// 5. 唯一索引
			if err := tx.Exec(`
				CREATE UNIQUE INDEX idx_conv_msg_unique
				ON conversation_messages(conversation_id, sort_order)
			`).Error; err != nil {
				return fmt.Errorf("create index: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 简单回滚：删唯一索引；列不还原（迁移本身有损）
			return tx.Exec(`DROP INDEX IF EXISTS idx_conv_msg_unique`).Error
		},
	}
}
```

- [ ] **Step 4: 注册迁移**

编辑 `migrations/migrations.go`，把 `migration202605100001()` 加到迁移列表（按 ID 时间顺序）：

```go
// 在 List 切片中追加（确保位于 202605080012 之后）
migration202605100001(),
```

- [ ] **Step 5: Run to verify pass**

```
go test ./migrations -run Test202605100001 -v
```

Expected: PASS。

- [ ] **Step 6: 全量迁移测试，确认链条不挂**

```
go test ./migrations/... -v
```

Expected: 全 PASS（含已有迁移测）。

- [ ] **Step 7: Commit**

```bash
git add migrations/202605100001_aiagent_v2_schema.go migrations/202605100001_aiagent_v2_schema_test.go migrations/migrations.go
git commit -m "✨ migrations: aiagent v2 schema (drop v1 cols + partial_reason + unique idx)"
```

---

### Task 7: 更新 `conversation_entity.Message` 结构

**Files:**
- Modify: `internal/model/entity/conversation_entity/conversation.go`
- Test: `internal/model/entity/conversation_entity/conversation_test.go`

- [ ] **Step 1: 写期望的测试**

替换/追加 `internal/model/entity/conversation_entity/conversation_test.go` 中相关测试：

```go
func TestMessage_V2Fields(t *testing.T) {
	convey.Convey("Message v2 字段", t, func() {
		m := Message{
			ID:             0,
			ConversationID: 42,
			Role:           "assistant",
			Blocks:         `[{"type":"text","text":"hi"}]`,
			Mentions:       `[]`,
			TokenUsage:     `{"prompt":10,"completion":5}`,
			PartialReason:  "errored",
			SortOrder:      3,
		}
		convey.So(m.PartialReason, convey.ShouldEqual, "errored")
		convey.So(m.SortOrder, convey.ShouldEqual, 3)
	})
}
```

老的引用 `m.CagoID`、`m.Kind`、`m.Origin` 等的测试一并删掉。

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/model/entity/conversation_entity/... -v
```

Expected: `undefined: PartialReason` 或类似。

- [ ] **Step 3: 改 Message 结构**

替换 `internal/model/entity/conversation_entity/conversation.go` 的 `Message struct`：

```go
// Message 会话消息实体（cago agents v2 schema）
type Message struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64  `gorm:"column:conversation_id;index;not null"`
	Role           string `gorm:"column:role;type:varchar(20);not null"`
	Blocks         string `gorm:"column:blocks;type:text"`              // []ContentBlock JSON（含 MetadataBlock）
	Mentions       string `gorm:"column:mentions;type:text"`            // []ai.MentionedAsset JSON
	TokenUsage     string `gorm:"column:token_usage;type:text"`         // *agent.Usage JSON，仅 assistant 可能有
	PartialReason  string `gorm:"column:partial_reason;type:varchar(16);not null;default:''"`
	SortOrder      int    `gorm:"column:sort_order;not null;default:0;uniqueIndex:idx_conv_msg_unique,priority:2"`
	Createtime     int64  `gorm:"column:createtime"`
}
```

注意 `uniqueIndex` 标签——和 `ConversationID` 标签的 unique index priority 一起组成复合唯一键。给 `ConversationID` 加：

```go
ConversationID int64 `gorm:"column:conversation_id;not null;uniqueIndex:idx_conv_msg_unique,priority:1"`
```

把已删字段（CagoID/ParentID/Kind/Origin/Persist/Thinking/ToolCallJSON/ToolResultJSON/Raw/MsgTime/ToolCalls/ToolCallID/Content）全部从 struct 移除。

- [ ] **Step 4: 修补编译错误**

```
go build ./...
```

会报多处引用已删字段。一一修复——这些都是必经的删法点。先记一份清单：

```
go build ./... 2>&1 | grep -E "undefined|cannot use" | head -40
```

逐个修。多数将在后续 Task 里替换；此处只让代码能编译。简单做法：把所有引用了已删字段的位置注释掉或删掉对应分支（`Sed`/逐个 Edit）。

不能编译过的影响范围（基于 grep）：
- `internal/aiagent/store.go` —— 全 v1 store；Phase 3 会重写，先临时把所有引用旧字段的代码块全注释掉
- `internal/repository/conversation_repo/conversation.go` —— 涉及行级 upsert；同 Phase 3 重写
- `internal/aiagent/store_test.go` —— v1 测试；本步直接 `t.Skip("v1 store removed")` 顶住

具体：

```bash
# 临时手段：重命名 store.go → store.v1.go.bak，让 Phase 3 重写时直接删
mv internal/aiagent/store.go internal/aiagent/store.v1.go.bak
mv internal/aiagent/store_test.go internal/aiagent/store_test.v1.go.bak
```

`conversation_repo/conversation.go` 同样重命名 `_test`，主文件内的方法暂时改为返 `errors.New("v1 store removed; await Phase 3")` 即可——下一步会全替换。

- [ ] **Step 5: Run to verify build**

```
go build ./...
```

Expected: 编译过。其它包对该 entity 的引用如果还报错，把错误位置加 `// TODO: rewire in Phase 3` 占位（只要本 Task 范围内）；store/repo 这两个会在 Task 8/9 重写。

- [ ] **Step 6: Run to verify entity test**

```
go test ./internal/model/entity/conversation_entity/... -v
```

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/model/entity/conversation_entity/conversation.go internal/model/entity/conversation_entity/conversation_test.go
git add internal/aiagent/store.v1.go.bak internal/aiagent/store_test.v1.go.bak    # 占位文件
git rm internal/aiagent/store.go internal/aiagent/store_test.go                    # 删原 v1
# conversation_repo 还在编译过，先不动
git commit -m "♻️ conversation_entity: 升级到 cago v2 平铺字段（删 cago_id 等 v1 列）"
```

---

## Phase 3 — 新 aiagent 层（TDD）

### Task 8: `conversation_repo` 重写为按 `(conv_id, sort_order)` upsert

**Files:**
- Modify: `internal/repository/conversation_repo/conversation.go`
- Test: `internal/repository/conversation_repo/conversation_test.go`

- [ ] **Step 1: Write failing test**

替换 `internal/repository/conversation_repo/conversation_test.go`（保留类似 case 但改 schema）：

```go
package conversation_repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

func setupTest(t *testing.T) (context.Context, *gorm.DB, ConversationRepo) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&conversation_entity.Message{}, &conversation_entity.Conversation{}))
	return context.Background(), db, NewConversation(db)
}

func TestAppendAt_InsertsRow(t *testing.T) {
	ctx, _, repo := setupTest(t)
	err := repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1,
		Role:           "user",
		Blocks:         `[{"type":"text","text":"hi"}]`,
		PartialReason:  "",
		SortOrder:      0,
	})
	assert.NoError(t, err)

	got, err := repo.LoadOrdered(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "user", got[0].Role)
	assert.Equal(t, 0, got[0].SortOrder)
}

func TestUpdateAt_UpdatesRow(t *testing.T) {
	ctx, _, repo := setupTest(t)
	_ = repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "assistant", Blocks: `[]`, SortOrder: 0})
	err := repo.UpdateAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "assistant", Blocks: `[{"type":"text","text":"done"}]`,
		PartialReason: "errored", TokenUsage: `{"total":42}`, SortOrder: 0,
	})
	assert.NoError(t, err)
	got, _ := repo.LoadOrdered(ctx, 1)
	assert.Equal(t, "errored", got[0].PartialReason)
	assert.Contains(t, got[0].TokenUsage, "42")
}

func TestTruncateFrom_DeletesTail(t *testing.T) {
	ctx, _, repo := setupTest(t)
	for i := 0; i < 4; i++ {
		_ = repo.AppendAt(ctx, 1, i, &conversation_entity.Message{ConversationID: 1, Role: "user", SortOrder: i})
	}
	err := repo.TruncateFrom(ctx, 1, 2)
	assert.NoError(t, err)
	got, _ := repo.LoadOrdered(ctx, 1)
	assert.Len(t, got, 2)
	assert.Equal(t, 0, got[0].SortOrder)
	assert.Equal(t, 1, got[1].SortOrder)
}

func TestLoadOrdered_OrderedBySortOrder(t *testing.T) {
	ctx, _, repo := setupTest(t)
	_ = repo.AppendAt(ctx, 1, 2, &conversation_entity.Message{ConversationID: 1, Role: "a", SortOrder: 2})
	_ = repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "b", SortOrder: 0})
	_ = repo.AppendAt(ctx, 1, 1, &conversation_entity.Message{ConversationID: 1, Role: "c", SortOrder: 1})
	got, _ := repo.LoadOrdered(ctx, 1)
	assert.Equal(t, []string{"b", "c", "a"}, []string{got[0].Role, got[1].Role, got[2].Role})
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/repository/conversation_repo/... -v
```

Expected: 接口方法名不存在，编译失败。

- [ ] **Step 3: 重写 ConversationRepo 接口与实现**

替换 `internal/repository/conversation_repo/conversation.go` 的接口与方法（保留其它已存在的 conversation 相关方法不动；只改 message 部分）：

```go
package conversation_repo

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// ConversationRepo 维护 conversation_messages 行级访问。cago v2 寻址用
// (conversation_id, sort_order) — 与 cago Conversation 的 (convID, index)
// 一一对应。
type ConversationRepo interface {
	// 既有方法保留
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	Update(ctx context.Context, c *conversation_entity.Conversation) error

	// v2 message 行级
	AppendAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error
	UpdateAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error
	TruncateFrom(ctx context.Context, conversationID int64, sortOrder int) error
	LoadOrdered(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
}

type conversationRepo struct {
	db *gorm.DB
}

func NewConversation(db *gorm.DB) ConversationRepo {
	return &conversationRepo{db: db}
}

var defaultRepo ConversationRepo

func Conversation() ConversationRepo { return defaultRepo }

func RegisterConversation(r ConversationRepo) { defaultRepo = r }

func (r *conversationRepo) Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error) {
	var c conversation_entity.Conversation
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *conversationRepo) Update(ctx context.Context, c *conversation_entity.Conversation) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *conversationRepo) AppendAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error {
	msg.ConversationID = conversationID
	msg.SortOrder = sortOrder
	// (conversation_id, sort_order) 唯一索引 → 冲突即 v2 升级路径里的 ChangeAppended 又一次发生（重启重放等）；
	// 上层应用通过先 Truncate 再 Append 保证；这里冲突视为错误。
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *conversationRepo) UpdateAt(ctx context.Context, conversationID int64, sortOrder int, msg *conversation_entity.Message) error {
	return r.db.WithContext(ctx).Model(&conversation_entity.Message{}).
		Where("conversation_id = ? AND sort_order = ?", conversationID, sortOrder).
		Updates(map[string]any{
			"role":           msg.Role,
			"blocks":         msg.Blocks,
			"mentions":       msg.Mentions,
			"token_usage":    msg.TokenUsage,
			"partial_reason": msg.PartialReason,
			"createtime":     msg.Createtime,
		}).Error
}

func (r *conversationRepo) TruncateFrom(ctx context.Context, conversationID int64, sortOrder int) error {
	return r.db.WithContext(ctx).
		Where("conversation_id = ? AND sort_order >= ?", conversationID, sortOrder).
		Delete(&conversation_entity.Message{}).Error
}

func (r *conversationRepo) LoadOrdered(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error) {
	var msgs []*conversation_entity.Message
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("sort_order ASC").
		Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}
```

老接口（`UpsertMessagesByCagoID` / `DeleteByCagoIDs` / `UpdateState` / `UpdateMessageTokenUsage` 等）整个删——它们都依赖被删的 v1 字段。

把 `mock_conversation_repo/conversation.go` 删除（mockgen 输出会重新生成）。

- [ ] **Step 4: 重生成 mock**

`internal/repository/conversation_repo/conversation.go` 顶部加（如未存在）：

```go
//go:generate mockgen -source=conversation.go -destination=mock_conversation_repo/conversation.go -package=mock_conversation_repo
```

跑：

```
go generate ./internal/repository/conversation_repo/...
```

- [ ] **Step 5: Run to verify pass**

```
go test ./internal/repository/conversation_repo/... -v
```

Expected: PASS。

- [ ] **Step 6: 编译全仓库（多处会因接口变化报错；逐个修）**

```
go build ./...
```

主要影响：
- `internal/service/conversation_svc/*` —— 改用新方法名；移除 `UpsertCagoMessages` / `UpdateConversationState` 包装（Task 后续删除）
- `internal/aiagent/store.v1.go.bak` —— 已是 .bak 不参与编译
- `internal/app/app_ai.go` —— 依赖 svc；先简化方法签名让其编译过，逻辑后续 Task 重接

```bash
# 简单做法：把 conversation_svc 中 message 相关方法直接删掉（service 层后续也要重做）；
# 让 app_ai.go 暂时不调它们。
```

更具体：见 Task 9 的工作。本 Task 暂时把仓库改成"内部 conversation_repo 已 v2、外部 service 接口未通"的中间态，commit 即可——下一 Task 立刻接上。

实操路径：

```bash
# 1) 把 conversation_svc.go 里所有引用旧 repo 方法的地方删/打 stub
# 2) 把 app_ai.go 里 Save/Load 路径暂时打 stub 返回 nil
# 3) make sure go build ./... 过
```

如果太杂乱，本 Task 6 可以直接合到 Task 9 一起 commit；分两个 commit 是为了 review 容易。视实情而定。

- [ ] **Step 7: Commit**

```bash
git add internal/repository/conversation_repo/conversation.go
git add internal/repository/conversation_repo/conversation_test.go
git add internal/repository/conversation_repo/mock_conversation_repo/
git rm internal/repository/conversation_repo/conversation_upsert_test.go
# service / app_ai stub 改动一并加
git add internal/service/conversation_svc/ internal/app/app_ai.go
git commit -m "♻️ conversation_repo: 行级 (conversation_id, sort_order) upsert（cago v2 寻址）"
```

---

### Task 9: `aiagent.gormStore` v2 实现（agent.Store 接口）

**Files:**
- Create: `internal/aiagent/store_gorm.go`
- Test: `internal/aiagent/store_gorm_test.go`

- [ ] **Step 1: Write failing test**

新建 `internal/aiagent/store_gorm_test.go`：

```go
package aiagent

import (
	"context"
	"strconv"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

func setupGormStore(t *testing.T) (context.Context, agent.Store) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(
		&conversation_entity.Message{},
		&conversation_entity.Conversation{},
	))
	repo := conversation_repo.NewConversation(db)
	conversation_repo.RegisterConversation(repo)
	// 提前插一行 conversation row 让 LoadConversation 不为空
	assert.NoError(t, db.Exec(`INSERT INTO conversations (id, title) VALUES (1, 'test')`).Error)
	return context.Background(), NewGormStore()
}

func TestGormStore_AppendAndLoad(t *testing.T) {
	ctx, s := setupGormStore(t)
	err := s.AppendMessage(ctx, "1", 0, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "hello"}},
	})
	assert.NoError(t, err)

	msgs, _, err := s.LoadConversation(ctx, "1")
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, agent.RoleUser, msgs[0].Role)
	tb, ok := msgs[0].Content[0].(agent.TextBlock)
	assert.True(t, ok)
	assert.Equal(t, "hello", tb.Text)
}

func TestGormStore_UpdateMessage_FillsPartialAndUsage(t *testing.T) {
	ctx, s := setupGormStore(t)
	_ = s.AppendMessage(ctx, "1", 0, agent.Message{Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming})
	usage := agent.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15}
	err := s.UpdateMessage(ctx, "1", 0, agent.Message{
		Role:          agent.RoleAssistant,
		Content:       []agent.ContentBlock{agent.TextBlock{Text: "done"}},
		PartialReason: agent.PartialErrored,
		Usage:         &usage,
	})
	assert.NoError(t, err)

	msgs, _, _ := s.LoadConversation(ctx, "1")
	assert.Equal(t, agent.PartialErrored, msgs[0].PartialReason)
	assert.NotNil(t, msgs[0].Usage)
	assert.Equal(t, 15, msgs[0].Usage.TotalTokens)
}

func TestGormStore_TruncateAfter(t *testing.T) {
	ctx, s := setupGormStore(t)
	for i := 0; i < 4; i++ {
		_ = s.AppendMessage(ctx, "1", i, agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: strconv.Itoa(i)}}})
	}
	assert.NoError(t, s.TruncateAfter(ctx, "1", 2))
	msgs, _, _ := s.LoadConversation(ctx, "1")
	assert.Len(t, msgs, 2)
}

func TestGormStore_LoadFixesStreamingTail(t *testing.T) {
	ctx, s := setupGormStore(t)
	_ = s.AppendMessage(ctx, "1", 0, agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}}})
	_ = s.AppendMessage(ctx, "1", 1, agent.Message{Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming, Content: []agent.ContentBlock{agent.TextBlock{Text: "half "}}})

	msgs, _, _ := s.LoadConversation(ctx, "1")
	assert.Len(t, msgs, 2)
	assert.Equal(t, agent.PartialErrored, msgs[1].PartialReason, "streaming tail should be rewritten to errored on load")
	tb := msgs[1].Content[0].(agent.TextBlock)
	assert.Equal(t, "half ", tb.Text, "partial content preserved")
}

func TestGormStore_PreservesMetadataBlock(t *testing.T) {
	ctx, s := setupGormStore(t)
	_ = s.AppendMessage(ctx, "1", 0, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.TextBlock{Text: "expanded body"},
			agent.MetadataBlock{Key: "display", Value: "@srv1 status"},
		},
	})
	msgs, _, _ := s.LoadConversation(ctx, "1")
	assert.Len(t, msgs[0].Content, 2)
	mb, ok := msgs[0].Content[1].(agent.MetadataBlock)
	assert.True(t, ok)
	assert.Equal(t, "display", mb.Key)
	assert.Equal(t, "@srv1 status", mb.Value)
}

func parseSessionIDInt(t *testing.T, sid string) int64 {
	id, err := strconv.ParseInt(sid, 10, 64)
	assert.NoError(t, err)
	return id
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/aiagent -run TestGormStore -v
```

Expected: `undefined: NewGormStore`。

- [ ] **Step 3: 实现 store_gorm.go**

新建 `internal/aiagent/store_gorm.go`：

```go
package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

// gormStore 把 cago agent.Conversation 的增量变更落到 conversation_messages
// 的 (conversation_id, sort_order) 行上。配合 agent/store.Recorder 监听
// Conv.Watch 使用。
type gormStore struct {
	repo conversation_repo.ConversationRepo
}

func NewGormStore() agent.Store {
	return &gormStore{repo: conversation_repo.Conversation()}
}

func newGormStoreWithRepo(r conversation_repo.ConversationRepo) agent.Store {
	return &gormStore{repo: r}
}

func parseSessionID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("aiagent: bad session id %q: %w", s, err)
	}
	return id, nil
}

func (g *gormStore) AppendMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	row, err := messageToRow(convID, index, msg)
	if err != nil {
		return err
	}
	return g.repo.AppendAt(ctx, convID, index, row)
}

func (g *gormStore) UpdateMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	row, err := messageToRow(convID, index, msg)
	if err != nil {
		return err
	}
	return g.repo.UpdateAt(ctx, convID, index, row)
}

func (g *gormStore) TruncateAfter(ctx context.Context, sessionID string, index int) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	return g.repo.TruncateFrom(ctx, convID, index)
}

func (g *gormStore) LoadConversation(ctx context.Context, sessionID string) ([]agent.Message, agent.BranchInfo, error) {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return nil, agent.BranchInfo{}, err
	}
	rows, err := g.repo.LoadOrdered(ctx, convID)
	if err != nil {
		return nil, agent.BranchInfo{}, err
	}
	out := make([]agent.Message, 0, len(rows))
	for _, r := range rows {
		m, err := rowToMessage(r)
		if err != nil {
			return nil, agent.BranchInfo{}, err
		}
		out = append(out, m)
	}
	// crash-recovery invariant：尾部 PartialStreaming 残留改写为 errored
	if n := len(out); n > 0 && out[n-1].PartialReason == agent.PartialStreaming {
		out[n-1].PartialReason = agent.PartialErrored
	}
	return out, agent.BranchInfo{}, nil
}

// messageToRow 把 cago Message 序列化为 DB 行。
//   - blocks: 全部 ContentBlock JSON（含 MetadataBlock）
//   - mentions: 从 MetadataBlock{Key:"mentions"} 提取
//   - token_usage: msg.Usage JSON（仅 assistant 非 nil 时填）
func messageToRow(convID int64, index int, m agent.Message) (*conversation_entity.Message, error) {
	blocksJSON, err := json.Marshal(serializeBlocks(m.Content))
	if err != nil {
		return nil, fmt.Errorf("marshal blocks: %w", err)
	}
	mentions := extractMentions(m.Content)
	mentionsJSON, err := json.Marshal(mentions)
	if err != nil {
		return nil, fmt.Errorf("marshal mentions: %w", err)
	}
	var usageJSON string
	if m.Usage != nil {
		b, err := json.Marshal(m.Usage)
		if err != nil {
			return nil, fmt.Errorf("marshal usage: %w", err)
		}
		usageJSON = string(b)
	}
	created := m.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return &conversation_entity.Message{
		ConversationID: convID,
		Role:           string(m.Role),
		Blocks:         string(blocksJSON),
		Mentions:       string(mentionsJSON),
		TokenUsage:     usageJSON,
		PartialReason:  m.PartialReason,
		SortOrder:      index,
		Createtime:     created.Unix(),
	}, nil
}

func rowToMessage(r *conversation_entity.Message) (agent.Message, error) {
	var rawBlocks []map[string]any
	if r.Blocks != "" {
		if err := json.Unmarshal([]byte(r.Blocks), &rawBlocks); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal blocks: %w", err)
		}
	}
	content, err := deserializeBlocks(rawBlocks)
	if err != nil {
		return agent.Message{}, err
	}
	var usage *agent.Usage
	if r.TokenUsage != "" {
		var u agent.Usage
		if err := json.Unmarshal([]byte(r.TokenUsage), &u); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal usage: %w", err)
		}
		usage = &u
	}
	created := time.Unix(r.Createtime, 0)
	return agent.Message{
		Role:          agent.Role(r.Role),
		Content:       content,
		CreatedAt:     created,
		PartialReason: r.PartialReason,
		Usage:         usage,
	}, nil
}

// serializeBlocks 把 ContentBlock slice 序列化成 [{"type":..., ...}, ...]
// 形态。tag 用 ContentBlockType()，便于 deserializeBlocks 反向构造。
func serializeBlocks(blocks []agent.ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		entry := map[string]any{"type": b.ContentBlockType()}
		switch v := b.(type) {
		case agent.TextBlock:
			entry["text"] = v.Text
		case agent.ToolUseBlock:
			entry["id"] = v.ID
			entry["name"] = v.Name
			entry["input"] = v.Input
			entry["raw_args"] = v.RawArgs
		case agent.ToolResultBlock:
			entry["tool_use_id"] = v.ToolUseID
			entry["is_error"] = v.IsError
			entry["content"] = serializeBlocks(v.Content)
		case agent.ThinkingBlock:
			entry["text"] = v.Text
			entry["signature"] = v.Signature
		case agent.MetadataBlock:
			entry["key"] = v.Key
			entry["value"] = v.Value
		case agent.ImageBlock:
			entry["media_type"] = v.MediaType
			entry["url"] = v.Source.URL
			entry["inline"] = v.Source.Inline
		}
		out = append(out, entry)
	}
	return out
}

func deserializeBlocks(raw []map[string]any) ([]agent.ContentBlock, error) {
	out := make([]agent.ContentBlock, 0, len(raw))
	for _, r := range raw {
		switch r["type"] {
		case "text":
			out = append(out, agent.TextBlock{Text: asString(r["text"])})
		case "tool_use":
			input, _ := r["input"].(map[string]any)
			out = append(out, agent.ToolUseBlock{
				ID:      asString(r["id"]),
				Name:    asString(r["name"]),
				Input:   input,
				RawArgs: asString(r["raw_args"]),
			})
		case "tool_result":
			subRaw, _ := r["content"].([]any)
			subTyped := make([]map[string]any, 0, len(subRaw))
			for _, x := range subRaw {
				if m, ok := x.(map[string]any); ok {
					subTyped = append(subTyped, m)
				}
			}
			sub, err := deserializeBlocks(subTyped)
			if err != nil {
				return nil, err
			}
			isErr, _ := r["is_error"].(bool)
			out = append(out, agent.ToolResultBlock{
				ToolUseID: asString(r["tool_use_id"]),
				IsError:   isErr,
				Content:   sub,
			})
		case "thinking":
			out = append(out, agent.ThinkingBlock{Text: asString(r["text"]), Signature: asString(r["signature"])})
		case "metadata":
			out = append(out, agent.MetadataBlock{Key: asString(r["key"]), Value: r["value"]})
		case "image":
			out = append(out, agent.ImageBlock{
				MediaType: asString(r["media_type"]),
				Source: agent.BlobSource{
					URL: asString(r["url"]),
				},
			})
		}
	}
	return out, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// extractMentions 从 Content 里查找 MetadataBlock{Key:"mentions"}, 取它的 Value
// 序列化成 JSON。提供给 messageToRow 把 mentions 列单独写库（保 v1 行为）。
func extractMentions(blocks []agent.ContentBlock) any {
	for _, b := range blocks {
		if mb, ok := b.(agent.MetadataBlock); ok && mb.Key == "mentions" {
			return mb.Value
		}
	}
	return []any{}
}
```

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestGormStore -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/store_gorm.go internal/aiagent/store_gorm_test.go
git commit -m "✨ aiagent: gormStore v2 (agent.Store impl，行级 sort_order 持久化)"
```

---

### Task 10: `bridge.go` cago Event → ai.StreamEvent

**Files:**
- Create: `internal/aiagent/bridge.go`
- Test: `internal/aiagent/bridge_test.go`

- [ ] **Step 1: Write failing test**

新建 `internal/aiagent/bridge_test.go`：

```go
package aiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
)

type captureEmitter struct {
	convID int64
	events []ai.StreamEvent
}

func (c *captureEmitter) Emit(convID int64, ev ai.StreamEvent) {
	c.convID = convID
	c.events = append(c.events, ev)
}

func TestBridge_TextDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em, nil)
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hi"})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "content", em.events[0].Type)
	assert.Equal(t, "hi", em.events[0].Content)
	assert.Equal(t, int64(42), em.convID)
}

func TestBridge_ThinkingDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em, nil)
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "let me think"})
	assert.Equal(t, "thinking", em.events[0].Type)
	assert.Equal(t, "let me think", em.events[0].Content)
}

func TestBridge_ErrorDoesNotEmitDone(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em, nil)
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventError, Error: errors.New("boom")})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "error", em.events[0].Type)
	assert.Contains(t, em.events[0].Error, "boom")
}

func TestBridge_DoneEmittedOnce(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em, nil)
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	doneCount := 0
	for _, ev := range em.events {
		if ev.Type == "done" {
			doneCount++
		}
	}
	assert.Equal(t, 2, doneCount, "every EventDone gets relayed (cago only emits per-Send)")
}

func TestBridge_RetryEvent(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em, nil)
	b.onEvent(context.Background(), agent.Event{
		Kind: agent.EventRetry,
		Retry: &agent.RetryEvent{
			Attempt: 2,
			Delay:   0,
			Cause:   errors.New("503"),
		},
	})
	assert.Equal(t, "retry", em.events[0].Type)
	assert.Contains(t, em.events[0].Content, "2/")
	assert.Contains(t, em.events[0].Error, "503")
}

func TestBridge_QueueConsumedBatch_AggregatesUserPromptSubmits(t *testing.T) {
	em := &captureEmitter{}
	conv := agent.NewConversation()
	conv.Append(agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded A"},
		agent.MetadataBlock{Key: "display", Value: "raw A"},
	}})
	conv.Append(agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded B"},
		agent.MetadataBlock{Key: "display", Value: "raw B"},
	}})
	b := newBridge(42, em, conv)

	// 两个 UserPromptSubmit 紧跟 → flush as batch on next non-prompt event
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventUserPromptSubmit})
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventUserPromptSubmit})
	b.onEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "ack"})

	// 期望：第一条是 queue_consumed_batch，包 ["raw A", "raw B"]；之后是 content
	assert.Equal(t, "queue_consumed_batch", em.events[0].Type)
	assert.Equal(t, []string{"raw A", "raw B"}, em.events[0].QueueContents)
	assert.Equal(t, "content", em.events[1].Type)
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/aiagent -run TestBridge -v
```

Expected: `undefined: newBridge`。

- [ ] **Step 3: 实现 bridge.go**

新建 `internal/aiagent/bridge.go`：

```go
package aiagent

import (
	"context"
	"errors"
	"fmt"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// eventBridge 订阅 cago Runner.OnEvent，把事件翻译为 OpsKat ai.StreamEvent
// 推到 EventEmitter。同一 convID 一个 bridge 实例。
//
// 不变量：
//   - 不直接访问 conv 写路径；只读 Conv.MessageAt(...) 拿最近 user 消息的
//     display MetadataBlock（用于 queue_consumed_batch 聚合）
//   - EventDone 是唯一的"done" 信号；EventCancelled / EventError 不重复 emit done
//   - EventUserPromptSubmit 进缓冲，遇到非 UserPromptSubmit 事件时一次性 flush
//     成一条 queue_consumed_batch
type eventBridge struct {
	convID  int64
	emit    EventEmitter
	conv    agent.ConversationReader // 可空（测试 / 不需要 display 时）
	pending []string
}

func newBridge(convID int64, em EventEmitter, conv agent.ConversationReader) *eventBridge {
	return &eventBridge{convID: convID, emit: em, conv: conv}
}

func (b *eventBridge) onEvent(ctx context.Context, ev agent.Event) {
	if ev.Kind != agent.EventUserPromptSubmit && len(b.pending) > 0 {
		b.flushBatch()
	}
	switch ev.Kind {
	case agent.EventTextDelta:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "content", Content: ev.Delta})

	case agent.EventThinkingDelta:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "thinking", Content: ev.Delta})

	case agent.EventMessageEnd:
		// 仅在 thinking 段结束时给前端一个收尾标志；空内容
		if ev.PartialReason == agent.PartialNone {
			b.emit.Emit(b.convID, ai.StreamEvent{Type: "thinking_done"})
		}

	case agent.EventPreToolUse:
		if ev.Tool != nil {
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type:      "tool_call",
				ToolName:  ev.Tool.Name,
				ToolInput: stringifyMap(ev.Tool.Input),
				ToolCallID: ev.Tool.ToolUseID,
			})
		}

	case agent.EventPostToolUse:
		if ev.Tool != nil && ev.Tool.Output != nil {
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type:       "tool_result",
				ToolName:   ev.Tool.Name,
				ToolResult: serializeOutputBlocks(ev.Tool.Output.Content),
				IsError:    ev.Tool.Output.IsError,
				ToolCallID: ev.Tool.ToolUseID,
			})
		}

	case agent.EventError:
		errMsg := ""
		if ev.Error != nil {
			var he *agent.HookError
			if errors.As(ev.Error, &he) {
				errMsg = fmt.Sprintf("[%s%s] %v", he.Stage, hookErrorTool(he), he.Cause)
			} else {
				errMsg = ev.Error.Error()
			}
		}
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "error", Error: errMsg})

	case agent.EventCancelled:
		// 取消状态已在 conv.PartialReason；done 由 EventDone 单点收尾，这里不发
		return

	case agent.EventTurnEnd:
		// 观察用，不直接 emit；用于强制 flush batch
		return

	case agent.EventDone:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "done"})

	case agent.EventUserPromptSubmit:
		display := b.readLastUserDisplay()
		b.pending = append(b.pending, display)

	case agent.EventRetry:
		if ev.Retry != nil {
			cause := ""
			if ev.Retry.Cause != nil {
				cause = ev.Retry.Cause.Error()
			}
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type:    "retry",
				Content: fmt.Sprintf("%d/?", ev.Retry.Attempt),
				Error:   cause,
			})
		}
	}
}

func (b *eventBridge) flushBatch() {
	if len(b.pending) == 0 {
		return
	}
	b.emit.Emit(b.convID, ai.StreamEvent{
		Type:          "queue_consumed_batch",
		QueueContents: append([]string(nil), b.pending...),
	})
	b.pending = nil
}

// readLastUserDisplay 在 Conv 末尾找最近一条 user message，从其 Content 里取
// MetadataBlock{Key:"display"} 的 Value 字符串。找不到返空字符串（前端会兜底）。
func (b *eventBridge) readLastUserDisplay() string {
	if b.conv == nil {
		return ""
	}
	for i := b.conv.Len() - 1; i >= 0; i-- {
		m, err := b.conv.MessageAt(i)
		if err != nil {
			return ""
		}
		if m.Role != agent.RoleUser {
			continue
		}
		for _, blk := range m.Content {
			if mb, ok := blk.(agent.MetadataBlock); ok && mb.Key == "display" {
				if s, ok := mb.Value.(string); ok {
					return s
				}
			}
		}
		return ""
	}
	return ""
}

func hookErrorTool(he *agent.HookError) string {
	if he.Tool != "" {
		return ":" + he.Tool
	}
	return ""
}

func stringifyMap(m map[string]any) string {
	// 保持现有 ai.StreamEvent.ToolInput 形态（JSON 字符串）
	if m == nil {
		return ""
	}
	b, err := jsonMarshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

func serializeOutputBlocks(blocks []agent.ContentBlock) string {
	out := serializeBlocks(blocks)
	b, err := jsonMarshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

func jsonMarshal(v any) ([]byte, error) {
	// 包一层方便测试 mock；目前直接转
	return jsonMarshalImpl(v)
}
```

`jsonMarshalImpl` 在另一个 utility 文件里加：

```go
// internal/aiagent/json_helpers.go
package aiagent

import "encoding/json"

func jsonMarshalImpl(v any) ([]byte, error) {
	return json.Marshal(v)
}
```

注意 `ai.StreamEvent.QueueContents` / `ToolInput` / `ToolResult` / `ToolCallID` / `IsError` 等字段假定已存在于 `ai.StreamEvent`——通过 `grep -n "QueueContents\|ToolInput\|ToolResult" internal/ai/*.go` 验证。如果某个字段不存在，按 ai.StreamEvent 实际形态调整 emit 调用（保 v1 行为契约 #1 不变）。

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestBridge -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/bridge.go internal/aiagent/bridge_test.go internal/aiagent/json_helpers.go
git commit -m "✨ aiagent: bridge cago Event → ai.StreamEvent (queue_consumed_batch 聚合)"
```

---

### Task 11: `recover.go` Conversation 装载时 streaming → errored 修正（已并入 store_gorm.go）

`store_gorm.go::LoadConversation` 已经做了这件事；本 Task 仅追加一个 invariant 测验证全路径：

**Files:**
- Test: `internal/aiagent/recover_test.go`

- [ ] **Step 1: Write test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

func TestRecover_StreamingTailRewrittenAcrossLoad(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&conversation_entity.Message{}, &conversation_entity.Conversation{}))
	repo := conversation_repo.NewConversation(db)
	conversation_repo.RegisterConversation(repo)
	assert.NoError(t, db.Exec(`INSERT INTO conversations (id, title) VALUES (1, 't')`).Error)

	s := newGormStoreWithRepo(repo)
	ctx := context.Background()

	// 模拟"上次跑挂了"：尾部一条 streaming 留在库里
	_ = s.AppendMessage(ctx, "1", 0, agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}}})
	_ = s.AppendMessage(ctx, "1", 1, agent.Message{
		Role:          agent.RoleAssistant,
		Content:       []agent.ContentBlock{agent.TextBlock{Text: "halfway "}},
		PartialReason: agent.PartialStreaming,
	})

	msgs, _, err := s.LoadConversation(ctx, "1")
	assert.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, agent.PartialErrored, msgs[1].PartialReason)
	assert.Equal(t, "halfway ", msgs[1].Content[0].(agent.TextBlock).Text)
}
```

- [ ] **Step 2: Run to verify pass**

已在 store_gorm.go 实现；此测应直接通过。

```
go test ./internal/aiagent -run TestRecover -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/aiagent/recover_test.go
git commit -m "✅ aiagent: recover invariant 测试 (streaming → errored on Load)"
```

---

### Task 12: `hook_mentions.go`（UserPromptSubmit）

**Files:**
- Create: `internal/aiagent/hook_mentions.go`
- Test: `internal/aiagent/hook_mentions_test.go`

- [ ] **Step 1: Write failing test**

新建 `internal/aiagent/hook_mentions_test.go`：

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

type fakeMentionResolver struct{}

func (f *fakeMentionResolver) Expand(ctx context.Context, raw string) (expanded string, mentions []map[string]any, openTabs []string, err error) {
	if raw == "@srv1 status" {
		return "[server srv1 (id=1)] status", []map[string]any{{"asset_id": 1, "asset_name": "srv1"}}, []string{"asset:1"}, nil
	}
	return raw, nil, nil, nil
}

type captureTabOpener struct{ opened []string }

func (c *captureTabOpener) Open(ctx context.Context, target string) error {
	c.opened = append(c.opened, target)
	return nil
}

func TestMentionsHook_RewritesAndOpensTab(t *testing.T) {
	opener := &captureTabOpener{}
	h := newMentionsHook(&fakeMentionResolver{}, opener)
	out, err := h(context.Background(), &agent.UserPromptInput{Text: "@srv1 status"})
	assert.NoError(t, err)
	assert.Equal(t, "[server srv1 (id=1)] status", out.ModifiedText)
	assert.Equal(t, []string{"asset:1"}, opener.opened)
}

func TestMentionsHook_NoMention_PassThrough(t *testing.T) {
	opener := &captureTabOpener{}
	h := newMentionsHook(&fakeMentionResolver{}, opener)
	out, err := h(context.Background(), &agent.UserPromptInput{Text: "no mention here"})
	assert.NoError(t, err)
	assert.Empty(t, out.ModifiedText)
	assert.Empty(t, opener.opened)
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/aiagent -run TestMentionsHook -v
```

Expected: `undefined: newMentionsHook`。

- [ ] **Step 3: 实现 hook_mentions.go**

新建 `internal/aiagent/hook_mentions.go`，把现 `hook_prompt.go` 的逻辑搬过来并适配 v2 4-stage 签名：

```go
package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

// MentionResolver 解析 raw 文本中的 @mention，返回展开后的 LLM body、
// mention 列表（结构与 ai.MentionedAsset 一致）、要打开的 tab 引用。
type MentionResolver interface {
	Expand(ctx context.Context, raw string) (expanded string, mentions []map[string]any, openTabs []string, err error)
}

// TabOpener 把 mention 解析出来的 tab 引用打开（asset / extension / etc）。
type TabOpener interface {
	Open(ctx context.Context, target string) error
}

// newMentionsHook 返回一个 UserPromptSubmit hook：
//   - 解析 @mention，必要时改写 ModifiedText 为 expanded LLM body
//   - 把 mention 列表收集起来（cago 注入 user msg 后由 store 提取写库）
//   - 打开 tab 副作用
func newMentionsHook(r MentionResolver, t TabOpener) agent.UserPromptHook {
	return func(ctx context.Context, in *agent.UserPromptInput) (*agent.UserPromptOutput, error) {
		expanded, _, tabs, err := r.Expand(ctx, in.Text)
		if err != nil {
			return nil, err
		}
		for _, target := range tabs {
			if err := t.Open(ctx, target); err != nil {
				return nil, err
			}
		}
		out := &agent.UserPromptOutput{}
		if expanded != in.Text {
			out.ModifiedText = expanded
		}
		return out, nil
	}
}
```

把 `hook_prompt.go` 中真实的 mention 解析 / open-tab 逻辑拆出来实现 `MentionResolver` / `TabOpener`——具体业务搬移由该包既有代码决定（不在本计划范围）。

注意：mention 列表本身写库是通过 user msg 的 `MetadataBlock{Key:"mentions", Value:[...]}`——目前 cago 不支持 hook 在改写文本的同时给 user msg 注入额外 content blocks（`UserPromptOutput` 只能改 text）。**这是一个 deferred gap**：本步先实现到"改写文本 + 开 tab"，mentions 列写库放到 Task 21（service 层处理）：在 `App.SendAIMessage` 入口调用 mention 解析、把 mentions 通过 SendOption 之外的旁路（Manager 维护一个 per-conv pendingMentions map）传给 store。这是与当前 OpsKat v1 同样的妥协，不在本次理想化范围。

如果要真正干净：可在 Phase 1 cago 上游 PR 里再加一项 `UserPromptOutput.AdditionalBlocks []ContentBlock`——但范围已经超过了本次预算。

**已记录的妥协**：mentions 落库继续走 `Manager.pendingMentions` 旁路（Task 21 实现），未来 cago PR 加 `UserPromptOutput.AdditionalBlocks []ContentBlock` 后再清理。本 Task 只实现"改写 + 开 tab"。

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestMentionsHook -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/hook_mentions.go internal/aiagent/hook_mentions_test.go
git commit -m "✨ aiagent: hook_mentions UserPromptSubmit (改写 + 开 tab；mentions 落库走 Manager 旁路)"
```

---

### Task 13: `hook_policy.go`（PreToolUse 命令策略）

**Files:**
- Create: `internal/aiagent/hook_policy.go`
- Test: `internal/aiagent/hook_policy_test.go`

把现 `hook_policy.go` 的策略 gate 业务搬到 v2 PreToolUseHook 签名。

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

func TestPolicyHook_AllowsSafeCommand(t *testing.T) {
	h := newPolicyHook(&fakeChecker{allow: true})
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "ls"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
}

func TestPolicyHook_DeniesDangerous(t *testing.T) {
	h := newPolicyHook(&fakeChecker{allow: false, reason: "rm -rf 禁用"})
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "rm -rf /"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.Equal(t, "rm -rf 禁用", out.DenyReason)
}

type fakeChecker struct {
	allow  bool
	reason string
}

func (f *fakeChecker) Check(ctx context.Context, toolName string, input map[string]any) (allowed bool, reason string, err error) {
	return f.allow, f.reason, nil
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/aiagent -run TestPolicyHook -v
```

Expected: `undefined: newPolicyHook`。

- [ ] **Step 3: 实现 hook_policy.go**

```go
package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PolicyChecker 沿用 internal/ai 的策略层接口；这里包薄一层好测。
type PolicyChecker interface {
	Check(ctx context.Context, toolName string, input map[string]any) (allowed bool, reason string, err error)
}

func newPolicyHook(c PolicyChecker) agent.PreToolUseHook {
	return func(ctx context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		allowed, reason, err := c.Check(ctx, in.ToolName, in.Input)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return &agent.PreToolUseOutput{Decision: agent.DecisionDeny, DenyReason: reason}, nil
		}
		return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
	}
}

// 生产构造：包 ai.CheckPermission。
func PolicyHookProd() agent.PreToolUseHook {
	return newPolicyHook(&aiCheckerAdapter{})
}

type aiCheckerAdapter struct{}

func (a *aiCheckerAdapter) Check(ctx context.Context, toolName string, input map[string]any) (bool, string, error) {
	res := ai.CheckPermission(ctx, toolName, input)
	if res.Allowed() {
		return true, "", nil
	}
	return false, res.Reason(), nil
}
```

`ai.CheckPermission` 与 `res.Allowed()` / `.Reason()` 取决于现 `internal/ai/permission.go`——根据该文件的真实 API 调整 adapter 即可。

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestPolicyHook -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/hook_policy.go internal/aiagent/hook_policy_test.go
git commit -m "✨ aiagent: hook_policy PreToolUse 命令策略 gate"
```

---

### Task 14: `hook_rounds.go`（PreToolUse + OnRunnerStart 复位）

**Files:**
- Create: `internal/aiagent/hook_rounds.go`
- Test: `internal/aiagent/hook_rounds_test.go`

- [ ] **Step 1-5: 实现回合上限**

```go
package aiagent

import (
	"context"
	"sync/atomic"

	"github.com/cago-frame/agents/agent"
)

type roundsCounter struct {
	used int32
	max  int32
}

func newRoundsCounter(max int) *roundsCounter {
	return &roundsCounter{max: int32(max)}
}

func (rc *roundsCounter) Reset() {
	atomic.StoreInt32(&rc.used, 0)
}

func (rc *roundsCounter) Hook() agent.PreToolUseHook {
	return func(ctx context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		used := atomic.AddInt32(&rc.used, 1)
		if used > rc.max {
			return &agent.PreToolUseOutput{
				Decision:   agent.DecisionDeny,
				DenyReason: "已达回合上限",
			}, nil
		}
		return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
	}
}
```

测试：

```go
func TestRoundsCounter_Cap(t *testing.T) {
	rc := newRoundsCounter(2)
	h := rc.Hook()
	for i := 0; i < 2; i++ {
		out, _ := h(context.Background(), &agent.PreToolUseInput{})
		assert.Equal(t, agent.DecisionPass, out.Decision)
	}
	out, _ := h(context.Background(), &agent.PreToolUseInput{})
	assert.Equal(t, agent.DecisionDeny, out.Decision)
}

func TestRoundsCounter_ResetClearsBudget(t *testing.T) {
	rc := newRoundsCounter(1)
	h := rc.Hook()
	_, _ = h(context.Background(), &agent.PreToolUseInput{})
	out1, _ := h(context.Background(), &agent.PreToolUseInput{})
	assert.Equal(t, agent.DecisionDeny, out1.Decision)
	rc.Reset()
	out2, _ := h(context.Background(), &agent.PreToolUseInput{})
	assert.Equal(t, agent.DecisionPass, out2.Decision)
}
```

注：Manager 在构造 Agent 时既挂 `agent.PreToolUse("", rc.Hook())` 又挂 `agent.OnRunnerStart(func(...) error { rc.Reset(); return nil })`。

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestRoundsCounter -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/hook_rounds.go internal/aiagent/hook_rounds_test.go
git commit -m "✨ aiagent: hook_rounds PreToolUse 上限 + OnRunnerStart 复位"
```

---

### Task 15: `hook_audit.go`（PostToolUse 审计落库）

**Files:**
- Create: `internal/aiagent/hook_audit.go`
- Test: `internal/aiagent/hook_audit_test.go`

把 `sidecar.go` 的 audit drainer 业务直接挪到 PostToolUse Hook，不再走中间漏斗。

- [ ] **Step 1: Write test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

type captureAudit struct{ rows []string }

func (c *captureAudit) Write(ctx context.Context, toolName, input, output string, isError bool) error {
	c.rows = append(c.rows, toolName+":"+output)
	return nil
}

func TestAuditHook_WritesPerCall(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "ls"},
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
		},
	})
	assert.NoError(t, err)
	assert.Nil(t, out.ModifiedOutput)
	assert.Len(t, a.rows, 1)
	assert.Contains(t, a.rows[0], "ssh.exec")
}
```

- [ ] **Step 2: 实现 hook_audit.go**

```go
package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"
)

type AuditWriter interface {
	Write(ctx context.Context, toolName, inputJSON, outputJSON string, isError bool) error
}

func newAuditHook(w AuditWriter) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		inJSON, _ := json.Marshal(in.Input)
		outBlocks := serializeBlocks(in.Output.Content)
		outJSON, _ := json.Marshal(outBlocks)
		_ = w.Write(ctx, in.ToolName, string(inJSON), string(outJSON), in.Output.IsError)
		// 不修改 output；纯观察
		return &agent.PostToolUseOutput{}, nil
	}
}

// AuditWriterProd 默认实现接 audit_repo。具体 import 与字段映射照现
// internal/ai/audit.go 的形态搬过来。
```

- [ ] **Step 3: Run to verify pass**

```
go test ./internal/aiagent -run TestAuditHook -v
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/aiagent/hook_audit.go internal/aiagent/hook_audit_test.go
git commit -m "✨ aiagent: hook_audit PostToolUse 直连 audit_repo（删 sidecar 漏斗）"
```

---

### Task 16: `hook_approval.go` + `approval_gateway.go` 重构

**Files:**
- Create: `internal/aiagent/hook_approval.go`
- Modify: `internal/aiagent/approval_gateway.go`
- Test: `internal/aiagent/hook_approval_test.go`、`internal/aiagent/approval_gateway_test.go`（保留并改写现有测试）

cago `agent/approve.Approver` 提供 Pending iter + Approve/Deny 控制；OpsKat `ApprovalGateway` 壳保 Wails 事件契约不变。

- [ ] **Step 1: 确认 cago `agent/approve` 包形态**

```bash
ls /Users/codfrm/Code/cago/agents/agent/approve/   # 必须存在 approver.go + hook.go
cat /Users/codfrm/Code/cago/agents/agent/approve/approver.go
cat /Users/codfrm/Code/cago/agents/agent/approve/hook.go
```

预验证：本仓库 cago 已有该子包（approver.go + hook.go + doc.go + 测试）。检查 `approve.New() *Approver`、`approve.Hook(*Approver, opts...) PreToolUseHook`、`Approver.Pending() iter.Seq[Pending]`、`Approve(id)` / `Deny(id, reason)` 是否齐全。如缺接口，先在 Phase 1 cago PR 范围内补上再回到本 Task。

- [ ] **Step 2: 重写 ApprovalGateway**

按 spec §10.2 写：

```go
package aiagent

import (
	"context"
	"sync"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/agent/approve"

	"github.com/opskat/opskat/internal/ai"
)

type ApprovalGateway struct {
	approver *approve.Approver
	emit     EventEmitter
	grants   LocalGrantStore
	resolver PendingResolver

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // pending.ID → cancel hook for Cancel-during-pending
}

func NewApprovalGateway(em EventEmitter, grants LocalGrantStore, resolver PendingResolver) *ApprovalGateway {
	return &ApprovalGateway{
		approver: approve.New(),
		emit:     em,
		grants:   grants,
		resolver: resolver,
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (g *ApprovalGateway) Approver() *approve.Approver { return g.approver }

func (g *ApprovalGateway) Hook() agent.PreToolUseHook { return approve.Hook(g.approver) }

// Run 应该作为 manager 启动时的常驻 goroutine 跑（每个 Manager 一个）。
func (g *ApprovalGateway) Run(ctx context.Context) {
	for p := range g.approver.Pending() {
		g.handlePending(ctx, p)
	}
}

func (g *ApprovalGateway) handlePending(ctx context.Context, p approve.Pending) {
	convID := pendingConvID(p) // 从 p.Input / external lookup
	if g.grants.Allowed(ctx, p.ToolName, p.ToolUseID, p.Input) {
		g.approver.Approve(p.ID)
		return
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type:       "approval_request",
		ToolName:   p.ToolName,
		ToolCallID: p.ToolUseID,
		// ToolInput: jsonify(p.Input),
	})
	decision, err := g.resolver.Wait(ctx, p.ID)
	if err != nil {
		g.approver.Deny(p.ID, "canceled")
		g.emit.Emit(convID, ai.StreamEvent{Type: "approval_resolved", ToolCallID: p.ToolUseID, Result: "denied"})
		return
	}
	switch decision.Action {
	case "approve":
		g.approver.Approve(p.ID)
	case "always":
		_ = g.grants.Allow(ctx, p.ToolName, p.Input)
		g.approver.Approve(p.ID)
	case "deny":
		g.approver.Deny(p.ID, decision.Reason)
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type:       "approval_resolved",
		ToolCallID: p.ToolUseID,
		Result:     decision.Action,
	})
}
```

`PendingResolver.Wait(ctx, id)` 必须 select 在 ctx.Done()——这是 Cancel 透传的关键。如果当前 PendingResolver 实现没做，本步要补一行 `select { case <-ctx.Done(): return ..., ctx.Err() }`。

- [ ] **Step 3: 测试**

```go
func TestApprovalGateway_AlwaysGrantsShortCircuit(t *testing.T) {
	em := &captureEmitter{}
	grants := &fakeGrants{allowed: true}
	resolver := &fakeResolver{}
	g := NewApprovalGateway(em, grants, resolver)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	// 通过 cago approve.Hook 路径触发 pending：构造 Hook 调用一遍即可走完
	// "Pending 入队 → handlePending(命中 grants 短路 Approve)" 的全链路。
	hook := g.Hook()
	out, err := hook(ctx, &agent.PreToolUseInput{
		ToolName:  "Write",
		ToolUseID: "tu_1",
		Input:     map[string]any{"path": "/tmp/x"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionApprove, out.Decision)
	// resolver 应该 0 次调用（grants 短路）
	assert.Equal(t, 0, resolver.waitCalls)
}

func TestApprovalGateway_CancelUnblocks(t *testing.T) {
	// 验证 ctx.Done 后 resolver.Wait 立即返
	ctx, cancel := context.WithCancel(context.Background())
	resolver := &fakeResolver{block: true}
	em := &captureEmitter{}
	g := NewApprovalGateway(em, &fakeGrants{}, resolver)
	go g.Run(ctx)
	cancel()
	// resolver.Wait 应已退出；用 race detector 跑
}
```

具体实现细节、faker 类型按 cago `approve.Approver` 真实接口写。`fakeGrants` 实现 `LocalGrantStore`（已存在于 OpsKat），`fakeResolver` 实现 `PendingResolver`：

```go
type fakeGrants struct{ allowed bool }
func (f *fakeGrants) Allowed(ctx context.Context, tool, id string, input map[string]any) bool { return f.allowed }
func (f *fakeGrants) Allow(ctx context.Context, tool string, input map[string]any) error { return nil }

type fakeResolver struct {
    block     bool
    waitCalls int
}
func (f *fakeResolver) Wait(ctx context.Context, pendingID string) (Decision, error) {
    f.waitCalls++
    if f.block {
        <-ctx.Done()
        return Decision{}, ctx.Err()
    }
    return Decision{Action: "approve"}, nil
}
```

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run "TestApprovalGateway" -v -race
```

Expected: PASS（race detector 不报告 data race）。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/hook_approval.go internal/aiagent/approval_gateway.go internal/aiagent/approval_gateway_test.go
git commit -m "♻️ aiagent: ApprovalGateway 重构为 wraps cago approve.Approver"
```

---

### Task 17: `subagent_dispatch.go`（PostToolUse 截断 summary）

**Files:**
- Create: `internal/aiagent/subagent_dispatch.go`
- Test: `internal/aiagent/subagent_dispatch_test.go`
- Delete: `internal/aiagent/subagent_observer.go`、`subagent_ops.go`（部分迁移）

- [ ] **Step 1: 移植截断 summary 逻辑**

把现 `subagent_observer.go` 中"截断 child summary、回写父 ToolResultBlock"的业务装到 PostToolUseHook：

```go
package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

func newSubagentDispatchHook(maxLen int) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		if in.ToolName != "dispatch_subagent" || in.Output == nil {
			return &agent.PostToolUseOutput{}, nil
		}
		truncated := truncateBlocks(in.Output.Content, maxLen)
		if blocksEqual(truncated, in.Output.Content) {
			return &agent.PostToolUseOutput{}, nil
		}
		return &agent.PostToolUseOutput{
			ModifiedOutput: &agent.ToolResultBlock{
				ToolUseID: in.Output.ToolUseID,
				IsError:   in.Output.IsError,
				Content:   truncated,
			},
		}, nil
	}
}

func truncateBlocks(blocks []agent.ContentBlock, maxLen int) []agent.ContentBlock {
	out := make([]agent.ContentBlock, 0, len(blocks))
	remaining := maxLen
	for _, b := range blocks {
		if tb, ok := b.(agent.TextBlock); ok {
			if len(tb.Text) > remaining {
				out = append(out, agent.TextBlock{Text: tb.Text[:remaining] + "…[截断]"})
				return out
			}
			remaining -= len(tb.Text)
		}
		out = append(out, b)
	}
	return out
}

func blocksEqual(a, b []agent.ContentBlock) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] { // 简单 compareable check；若某 block 不可比，单独处理
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: 测试**

```go
func TestSubagentDispatchHook_TruncatesLongOutput(t *testing.T) {
	h := newSubagentDispatchHook(10)
	out, _ := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "dispatch_subagent",
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "0123456789ABCDEF"}},
		},
	})
	assert.NotNil(t, out.ModifiedOutput)
	tb := out.ModifiedOutput.Content[0].(agent.TextBlock)
	assert.Contains(t, tb.Text, "…[截断]")
}

func TestSubagentDispatchHook_NonSubagentNoOp(t *testing.T) {
	h := newSubagentDispatchHook(10)
	out, _ := h(context.Background(), &agent.PostToolUseInput{ToolName: "ssh.exec", Output: &agent.ToolResultBlock{}})
	assert.Nil(t, out.ModifiedOutput)
}
```

```bash
git add internal/aiagent/subagent_dispatch.go internal/aiagent/subagent_dispatch_test.go
git rm internal/aiagent/subagent_observer.go internal/aiagent/subagent_observer_test.go
git commit -m "♻️ aiagent: subagent dispatch 截断 summary 改为 PostToolUse hook"
```

---

### Task 18: `handle.go` ConvHandle

**Files:**
- Create: `internal/aiagent/handle.go`
- Test: `internal/aiagent/handle_test.go`

- [ ] **Step 1: Write failing test（providertest 喂脚本）**

```go
package aiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	"github.com/stretchr/testify/assert"
)

func setupHandle(t *testing.T, prov provider.Provider) (*ConvHandle, *captureEmitter) {
	a := agent.New(prov)
	conv := agent.NewConversation()
	em := &captureEmitter{}
	r := a.Runner(conv)
	bridge := newBridge(1, em, conv)
	r.OnEvent(agent.AnyEvent, bridge.onEvent)
	return &ConvHandle{convID: 1, conv: conv, runner: r}, em
}

func TestConvHandle_SendNewTurnWhenNoActive(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "hello"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	h, em := setupHandle(t, prov)
	defer h.Close()

	err := h.Send(context.Background(), "raw text", "expanded body")
	assert.NoError(t, err)

	// content 与 done 应当出现
	types := streamTypes(em.events)
	assert.Contains(t, types, "content")
	assert.Contains(t, types, "done")
}

func TestConvHandle_SendDuringActiveUsesSteer(t *testing.T) {
	prov := providertest.New().
		QueueStream(
			provider.StreamChunk{ContentDelta: "first"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		).
		QueueStream(
			provider.StreamChunk{ContentDelta: "second"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
	h, _ := setupHandle(t, prov)
	defer h.Close()

	// 异步在 turn 1 中插一条 steer
	go func() {
		_ = h.Send(context.Background(), "raw 2", "expanded 2")
	}()
	err := h.Send(context.Background(), "raw 1", "expanded 1")
	assert.NoError(t, err)

	// conv 末尾应有两个 user message + 两段 assistant text
	last, _ := h.conv.MessageAt(h.conv.Len() - 1)
	assert.Equal(t, agent.RoleAssistant, last.Role)
}

func TestConvHandle_Cancel(t *testing.T) {
	prov := providertest.New().QueueStreamFunc(neverEndingStream())
	h, em := setupHandle(t, prov)
	defer h.Close()

	go func() {
		_ = h.Send(context.Background(), "hi", "hi")
	}()
	// 等首字 emit
	waitFor(t, func() bool { return len(em.events) > 0 })
	assert.NoError(t, h.Cancel("user"))

	// 末条应当 PartialReason=cancelled
	waitFor(t, func() bool {
		last, _ := h.conv.MessageAt(h.conv.Len() - 1)
		return last.PartialReason == agent.PartialCancelled
	})
}

func TestConvHandle_Edit(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "old"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "new"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	h, _ := setupHandle(t, prov)
	defer h.Close()
	_ = h.Send(context.Background(), "u1", "u1")
	assert.NoError(t, h.Edit(context.Background(), 0, "u1-edit", "u1-edit"))
	last, _ := h.conv.MessageAt(h.conv.Len() - 1)
	assert.Equal(t, "new", last.Content[0].(agent.TextBlock).Text)
}

func TestConvHandle_Regenerate(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "v1"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "v2"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	h, _ := setupHandle(t, prov)
	defer h.Close()
	_ = h.Send(context.Background(), "u", "u")
	// 现 conv = [u, assistant("v1")]
	assert.NoError(t, h.Regenerate(context.Background(), 1))
	last, _ := h.conv.MessageAt(h.conv.Len() - 1)
	assert.Equal(t, "v2", last.Content[0].(agent.TextBlock).Text)
}

// helpers
func streamTypes(evs []ai.StreamEvent) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor timed out")
}

func neverEndingStream() func(ctx context.Context) <-chan provider.StreamChunk {
	return func(ctx context.Context) <-chan provider.StreamChunk {
		ch := make(chan provider.StreamChunk, 8)
		go func() {
			defer close(ch)
			ch <- provider.StreamChunk{ContentDelta: "drip"}
			<-ctx.Done()
		}()
		return ch
	}
}

var _ = errors.Is // satisfy unused import linter when conditions allow
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/aiagent -run TestConvHandle -v
```

Expected: undefined `*ConvHandle`、`Send`、`Cancel`、`Edit`、`Regenerate`、`Close`。

- [ ] **Step 3: 实现 handle.go**

```go
package aiagent

import (
	"context"
	"errors"

	"github.com/cago-frame/agents/agent"
)

// ConvHandle 包一个会话的 cago Conversation + Runner，向 OpsKat 上层暴露
// 五个动作 (Send/Cancel/Edit/Regenerate/Close)。Send 是统一入口：先尝试 Steer
// 注入到当前活跃 turn；turn 已结束 (ErrSteerNoActiveTurn) 则起新 turn。
type ConvHandle struct {
	convID  int64
	conv    *agent.Conversation
	runner  *agent.Runner
	bridge  *eventBridge
	unbind  func() // store Recorder.Bind 的 unsubscribe
	closed  bool
}

// Send: raw 是 UI 展示文本；llmBody 是 mention-expanded 后的 LLM 输入。
func (h *ConvHandle) Send(ctx context.Context, raw, llmBody string) error {
	err := h.runner.Steer(ctx, llmBody, agent.WithSteerDisplay(raw))
	if err == nil {
		return nil // mid-turn injected; auto-continue
	}
	if !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		return err
	}
	events, err := h.runner.Send(ctx, llmBody, agent.WithSendDisplay(raw))
	if err != nil {
		return err
	}
	for range events { /* drain；OnEvent 已经在另一路推送 */ }
	return nil
}

func (h *ConvHandle) Cancel(reason string) error {
	if reason == "" {
		reason = "user"
	}
	return h.runner.Cancel(reason)
}

func (h *ConvHandle) Edit(ctx context.Context, idx int, raw, llmBody string) error {
	if err := h.conv.Truncate(idx); err != nil {
		return err
	}
	h.conv.Append(buildUserMessage(raw, llmBody))
	return h.resend(ctx)
}

func (h *ConvHandle) Regenerate(ctx context.Context, assistIdx int) error {
	if err := h.conv.Truncate(assistIdx); err != nil {
		return err
	}
	return h.resend(ctx)
}

func (h *ConvHandle) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	if h.unbind != nil {
		h.unbind()
	}
	return h.runner.Close()
}

func (h *ConvHandle) resend(ctx context.Context) error {
	events, err := h.runner.Resend(ctx)
	if err != nil {
		return err
	}
	for range events {
	}
	return nil
}

func buildUserMessage(raw, llmBody string) agent.Message {
	content := []agent.ContentBlock{agent.TextBlock{Text: llmBody}}
	if raw != "" && raw != llmBody {
		content = append(content, agent.MetadataBlock{Key: "display", Value: raw})
	}
	return agent.Message{Role: agent.RoleUser, Content: content}
}
```

- [ ] **Step 4: Run to verify pass**

```
go test ./internal/aiagent -run TestConvHandle -v -race
```

Expected: PASS。如果 race 报警，看是否是测试侧 helper 缺锁。

- [ ] **Step 5: Commit**

```bash
git add internal/aiagent/handle.go internal/aiagent/handle_test.go
git commit -m "✨ aiagent: ConvHandle (Send 优先 Steer + Cancel/Edit/Regenerate)"
```

---

### Task 19: `manager.go` Manager

**Files:**
- Create: `internal/aiagent/manager.go`
- Test: `internal/aiagent/manager_test.go`

- [ ] **Step 1: Write test**

```go
func TestManager_OpenConversation_LazyLoad(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "hi"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	m := setupManager(t, prov)
	defer m.Close()

	h1, err := m.Handle(context.Background(), 1)
	assert.NoError(t, err)
	h2, err := m.Handle(context.Background(), 1)
	assert.NoError(t, err)
	assert.Same(t, h1, h2, "second call returns cached handle")
}

func TestManager_DifferentConvsIndependent(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "a"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "b"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	m := setupManager(t, prov)
	defer m.Close()

	h1, _ := m.Handle(context.Background(), 1)
	h2, _ := m.Handle(context.Background(), 2)
	assert.NotSame(t, h1, h2)
	assert.NoError(t, h1.Send(context.Background(), "x", "x"))
	assert.NoError(t, h2.Send(context.Background(), "y", "y"))
}
```

- [ ] **Step 2: 实现 manager.go**

```go
package aiagent

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"

	"github.com/opskat/opskat/internal/ai"
)

type ManagerOptions struct {
	Provider     provider.Provider // cago provider
	System       string
	Tools        []agent.Tool
	MaxRounds    int
	RetryPolicy  agent.RetryPolicy
	Emitter      EventEmitter
	Resolver     PendingResolver
	LocalGrants  LocalGrantStore
	AuditWriter  AuditWriter
	PolicyChecker PolicyChecker
	Mention      MentionResolver
	TabOpener    TabOpener
}

type Manager struct {
	agent     *agent.Agent
	store     agent.Store
	approver  *ApprovalGateway
	emit      EventEmitter
	rounds    *roundsCounter

	mu      sync.Mutex
	handles map[int64]*ConvHandle
}

func NewManager(ctx context.Context, opts ManagerOptions) *Manager {
	if opts.MaxRounds == 0 {
		opts.MaxRounds = 50
	}
	rc := newRoundsCounter(opts.MaxRounds)
	gw := NewApprovalGateway(opts.Emitter, opts.LocalGrants, opts.Resolver)
	go gw.Run(ctx)

	mentionsHook := newMentionsHook(opts.Mention, opts.TabOpener)
	policyHook := newPolicyHook(opts.PolicyChecker)
	auditHook := newAuditHook(opts.AuditWriter)
	approvalHook := gw.Hook()
	subagentHook := newSubagentDispatchHook(2000)

	a := agent.New(opts.Provider,
		agent.System(opts.System),
		agent.Tools(opts.Tools...),
		agent.UserPromptSubmit(mentionsHook),
		agent.PreToolUse("", policyHook),
		agent.PreToolUse("", rc.Hook()),
		agent.PreToolUse(approvalMatcher(), approvalHook),
		agent.PostToolUse("", auditHook),
		agent.PostToolUse("dispatch_subagent", subagentHook),
		agent.OnRunnerStart(func(_ context.Context, _ *agent.Runner) error {
			rc.Reset()
			return nil
		}),
		agent.Retry(opts.RetryPolicy),
	)
	return &Manager{
		agent:    a,
		store:    NewGormStore(),
		approver: gw,
		emit:     opts.Emitter,
		rounds:   rc,
		handles:  make(map[int64]*ConvHandle),
	}
}

func (m *Manager) Handle(ctx context.Context, convID int64) (*ConvHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.handles[convID]; ok {
		return h, nil
	}
	sid := strconv.FormatInt(convID, 10)
	msgs, branch, err := m.store.LoadConversation(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("load conv %d: %w", convID, err)
	}
	conv := agent.LoadConversation(sid, msgs, agent.WithBranchedFrom(branch))
	r := m.agent.Runner(conv)
	bridge := newBridge(convID, m.emit, conv)
	r.OnEvent(agent.AnyEvent, bridge.onEvent)
	unbind := agentstore.Recorder(m.store).Bind(conv)
	h := &ConvHandle{
		convID: convID,
		conv:   conv,
		runner: r,
		bridge: bridge,
		unbind: unbind,
	}
	m.handles[convID] = h
	return h, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.handles {
		_ = h.Close()
	}
	m.handles = nil
	return nil
}

func approvalMatcher() string {
	// 与 v1 相同的工具集合：write / edit / Bash / 各种 SSH/DB tool
	return "Bash|Write|Edit|ssh.exec|sql.execute|redis.exec|kafka.produce"
}
```

`provider` import：`"github.com/cago-frame/agents/provider"`。

- [ ] **Step 3-5: Run + commit**

```
go test ./internal/aiagent -run TestManager -v
```

```bash
git add internal/aiagent/manager.go internal/aiagent/manager_test.go
git commit -m "✨ aiagent: Manager (Agent factory + per-conv ConvHandle 缓存)"
```

---

## Phase 4 — 集成

### Task 20: app_ai.go 切到 Manager

**Files:**
- Modify: `internal/app/app_ai.go`

替换现 `App.SendAIMessage` / `Stop` / `Edit` / `Regenerate` 内部实现，从直接持 `*aiagent.System` 改为：

```go
// 假设 App 持有 *aiagent.Manager
type App struct {
	// ...
	mgr *aiagent.Manager
}

func (a *App) SendAIMessage(convID int64, raw string) error {
	llm, err := a.expandMentions(raw) // 沿用既有逻辑
	if err != nil {
		return err
	}
	h, err := a.mgr.Handle(a.ctx, convID)
	if err != nil {
		return err
	}
	return h.Send(a.ctx, raw, llm)
}

func (a *App) StopAI(convID int64) error {
	h, err := a.mgr.Handle(a.ctx, convID)
	if err != nil {
		return err
	}
	return h.Cancel("user")
}

func (a *App) EditAIMessage(convID int64, idx int, raw string) error {
	llm, _ := a.expandMentions(raw)
	h, _ := a.mgr.Handle(a.ctx, convID)
	return h.Edit(a.ctx, idx, raw, llm)
}

func (a *App) RegenerateAIMessage(convID int64, assistIdx int) error {
	h, _ := a.mgr.Handle(a.ctx, convID)
	return h.Regenerate(a.ctx, assistIdx)
}
```

具体方法名 / 参数 / Wails binding 与现 `internal/app/app_ai.go` 现有 binding 对齐。

- [ ] **Step 1: 编译过**

```
go build ./...
```

- [ ] **Step 2: 全量测试**

```
make test
```

主要确保 `internal/app/...` 测试不挂。

- [ ] **Step 3: Wails binding regen**

```
make dev   # 起 dev server，让 wails generate ./frontend/wailsjs
# 验证：grep "SendAIMessage" frontend/wailsjs/go/app/App.* — 签名对得上
```

- [ ] **Step 4: Commit**

```bash
git add internal/app/app_ai.go frontend/wailsjs/
git commit -m "♻️ app_ai: 切到 *aiagent.Manager（v2 入口）"
```

---

### Task 21: 删除 v1 残骸 + Manager 旁路 mentions stash

**Files:**
- Delete: `internal/aiagent/system.go`、`sidecar.go`、`retry.go`、`event_bridge.go`、`emitter.go` 内 v1 相关、`hook_prompt.go`、`hook_audit.go` 旧版（已被 hook_audit.go 替换）

注：本 Task 是删干净 + 加 mentions 旁路。

- [ ] **Step 1: 删 v1 文件**

```bash
git rm internal/aiagent/system.go internal/aiagent/system_test.go
git rm internal/aiagent/sidecar.go internal/aiagent/sidecar_test.go
git rm internal/aiagent/retry.go internal/aiagent/retry_test.go
git rm internal/aiagent/event_bridge.go internal/aiagent/event_bridge_test.go
git rm internal/aiagent/store.v1.go.bak internal/aiagent/store_test.v1.go.bak
git rm internal/aiagent/hook_prompt.go internal/aiagent/hook_prompt_test.go    # 已被 hook_mentions.go 替代
# subagent_ops.go：内里有些 SubagentOps 业务可能 Manager 注册 tools 时用到，按需保留 / 拆
```

- [ ] **Step 2: 加 Manager.pendingMentions 旁路（mentions 落库）**

`Manager` 加一个 per-conv pending mentions map：

```go
type Manager struct {
    // ... 已有字段
    pendingMu       sync.Mutex
    pendingMentions map[int64][]map[string]any // convID → mentions of next user msg
}

func (m *Manager) StashMentions(convID int64, mentions []map[string]any) {
    m.pendingMu.Lock()
    defer m.pendingMu.Unlock()
    m.pendingMentions[convID] = mentions
}

// gormStore 在 AppendMessage 时拉这个 stash 写入 mentions 列。
// Manager 在构造 NewGormStore 时把自己作为 pendingMentionsProvider 传进去。
```

`store_gorm.go::messageToRow` 改：当 Role=user 时，如果 `*gormStore` 持有 `pendingMentionsProvider` 就 pop 一次并把 mentions 序列化进 `Mentions` 列。

具体接线：

```go
// Manager → store_gorm.go 注入
type pendingMentionsProvider interface {
    PopPendingMentions(convID int64) []map[string]any
}

type gormStore struct {
    repo     conversation_repo.ConversationRepo
    mentions pendingMentionsProvider // 可空（纯单测时 nil）
}

func NewGormStore(p pendingMentionsProvider) agent.Store {
    return &gormStore{repo: conversation_repo.Conversation(), mentions: p}
}

// messageToRow 在 Role=RoleUser 时
//   if g.mentions != nil { mentions := g.mentions.PopPendingMentions(convID); ... }
```

`Manager.NewManager` 把 `m` 自身作为 provider 传给 `NewGormStore(m)`：

```go
func NewManager(...) *Manager {
    m := &Manager{ ... pendingMentions: make(map[int64][]map[string]any) }
    m.store = NewGormStore(m)
    ...
}

func (m *Manager) PopPendingMentions(convID int64) []map[string]any {
    m.pendingMu.Lock()
    defer m.pendingMu.Unlock()
    out := m.pendingMentions[convID]
    delete(m.pendingMentions, convID)
    return out
}
```

`App.SendAIMessage` 解析完 mention 后 `mgr.StashMentions(convID, mentions)` 再 `handle.Send(ctx, raw, llm)`——cago 把 user msg Append 到 conv 触发 `gormStore.AppendMessage` → `messageToRow` 调 `PopPendingMentions` 写 `mentions` 列。

- [ ] **Step 3: 编译 + 测试**

```
go build ./...
make test
```

预期：删除编译过；测试与原 v1 路径相关的全部应已转向 v2。

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "🔥 aiagent: 删 v1 残骸 (system/sidecar/retry/event_bridge/hook_prompt)"
```

---

## Phase 5 — 验收

### Task 22: 端到端手测 5 个核心场景

**Files:** 不改代码；记录测试矩阵。

- [ ] **Step 1: `make dev`**
- [ ] **Step 2: 跑 5 个场景，每个截图 / 记日志**

| # | 场景 | 操作 | 期望 |
|---|---|---|---|
| 1 | 输出时停止 | 发"讲个长故事" → 流到一半点 Stop | partial 文本保留；下次"继续"包含 partial |
| 2 | LLM 失败 | 断网或换错 API key 后发消息 | 显示 partial（如有）+ 错误气泡；恢复网络后 Send 包含 partial |
| 3 | Steer 中插 | 发"帮我看 srv1" → 流中再发"也看 srv2" | 一个 turn 内连续两段 assistant；前端 queue_consumed_batch 显示 |
| 4 | 编辑重发 | 选中某条 user 消息编辑 → 提交 | 后续消息丢弃；新 assistant 出现 |
| 5 | 重新生成 | 选中某条 assistant 消息 → 重生 | 该条之后丢弃；新 assistant 出现 |
| 6 | 多会话并跑 | 两个 tab 同时发 | 互不串扰，各自显示 |
| 7 | 崩溃恢复 | 流式中 kill 进程 → 重启 | 末条 partial 显示 errored，不再转圈 |
| 8 | 审批中按 Stop | 触发审批 → 等弹框出现 → 按 Stop | 审批立即解锁，不卡死 |

- [ ] **Step 3: 任何 fail 项回到对应 Task 修；通过后录绿**

---

### Task 23: 全量 lint + test + 提交收尾

- [ ] **Step 1: lint**

```
make lint
cd frontend && pnpm lint
```

- [ ] **Step 2: test**

```
make test
cd frontend && pnpm test
```

- [ ] **Step 3: 最终 commit**

```bash
git add -A
git commit -m "✅ aiagent: cago v2 升级完成 (lint/test 全绿)" --allow-empty
git log --oneline -25
```

- [ ] **Step 4: 准备 PR**

PR 标题：`✨ aiagent: 升级到 cago agents v2`
PR body：摘要本计划 § Goal + § Architecture，列出每个 commit 的作用。

---

## 自检矩阵（spec → 任务覆盖）

| Spec 条 | 由哪个 Task 接住 |
|---|---|
| §3 #1 Wails StreamEvent 不变 | Task 10（bridge.go 测试覆盖每个 Type） |
| §3 #2 mention 解析 / 落库 | Task 12（hook 改写）+ Task 21（Manager 旁路落库） |
| §3 #3 命令策略 gate | Task 13 |
| §3 #4 审批 UX | Task 16 |
| §3 #5 审计落库 | Task 15 |
| §3 #6 回合上限 | Task 14 |
| §3 #7 dispatch_subagent 截断 | Task 17 |
| §3 #8 skills/extensions/slash command | 沿用现 ai.* 模块；Manager.Tools 注入处 |
| §3 #9 token usage 落库 | Task 9（store_gorm.UpdateMessage）|
| §3 #10 steer display ≠ LLM body | Task 1-4（cago PR）+ Task 18（ConvHandle 用 WithSendDisplay/WithSteerDisplay）|
| §3 #11 queue_consumed_batch 聚合 | Task 10（bridge.flushBatch） |
| §3 #12 跨重启恢复 | Task 9 + Task 11 |
| §4 5 个场景 | Task 22 |
| §5 cago 上游 PR | Task 1-5 |
| §6 包布局 | Task 9 / 10 / 11 / 18 / 19 + Task 12-17 |
| §7 4-stage Hook 拓扑 | Task 12-17、Task 19 注册 |
| §8 schema 演进 | Task 6 |
| §9 event bridge | Task 10 |
| §10 审批 | Task 16 |
| §11 retry | Task 19（agent.Retry option）；retry.go 删除 in Task 21 |
| §12 crash recovery invariant | Task 9 + Task 11 |
| §13 Cancel 透传 | Task 18（ConvHandle.Cancel）+ Task 16（ApprovalGateway.Run select on ctx）|
| §14 测试策略 | 每个 Task 自带 _test.go；Task 22 端到端 |
| §15 迁移序列 | 本计划全程 |
