# 2026-05-10 — aiagent 升级到 cago agents v2 设计

**Status**: Draft
**Authors**: brainstorming session
**Predecessor**: `2026-05-07-replace-builtin-agent-with-cago-design.md`（已落地的 cago v1 接入）
**Cago upstream spec**: `cago/agents/docs/superpowers/specs/2026-05-09-agent-rebuild-design.md` + Phase 6 (`2026-05-10`)

---

## 1. 背景

OpsKat `internal/aiagent/` 当前基于 cago agents v1（`agent.Session` + `agent.Stream` + `Backend` + 8-stage Hook + 全量 `SessionData` Store）。cago 上游已重构为 v2：拆 `Session` 为 `*Conversation`（数据）+ `*Runner`（短生命运行时）+ `*Agent`（工厂），用 `iter.Seq[Event]` 替代 `Stream`，4-stage Hook，`agent/store` 子包通过 `Conv.Watch` 增量持久化，`Conv.Truncate` / `Resend` / `runner.Cancel` / `runner.Steer` 都成为一等公民。Legacy `agent/` 包将随 Phase 6 删除——OpsKat 必须升级。

本设计解决三件事：
- 把 OpsKat 端整体迁到 v2-native 形状（顺势重构，**不做 shim**）
- 用 v2 原生能力直接覆盖产品 5 个核心场景（停止 / 错误 / steer / 编辑重发 / 重新生成）
- 在 cago 上游补两处对 chat-app 通用、v2 当前漏掉的能力（A1/A2 PR）

## 2. 范围

### 范围内

- 重写 `internal/aiagent/`：以 `*Manager` + `*ConvHandle` 替代 `*System` god-object；hooks 4-stage；store 走 `agent/store.Recorder` + `Conv.Watch`
- 删除 `internal/aiagent/retry.go`：以 cago 原生 `agent.Retry(RetryPolicy{...})` 替代
- 删除 `internal/aiagent/sidecar.go`：审计走纯 PostToolUse hook
- 删除 OpsKat-side `pendingMentions` / `pendingUsage` / `pendingDisplays` 三处 stash
- `conversation_messages` schema 演进：删 v1 平铺列、新增 `partial_reason`、`(conversation_id, sort_order)` 唯一键、一份 backfill 迁移
- cago 上游 PR：`WithSendDisplay` / `WithSteerDisplay` / `MetadataBlock`（A1+A2）
- 测试与序列化部署

### 不在范围

- 前端代码改动（Wails `StreamEvent` 形态契约不变 → 前端 0 改）
- 删除 `conversations.session_data` / `state_values` 列（本次仅停写；后续独立 PR）
- `cago_id` 列以前承担过的"前端引用消息"职责改造——本次直接删，前端用 `(conversation_id, sort_order)` 引用
- Branching（`Conv.BranchFrom`）的 UX——本次仅"覆盖"语义（Truncate）
- Token-limit "Continue" 按钮 UX 增量——本次 bridge 默认按 done 处理

## 3. 保留契约（业务/UX 完全不变）

| # | 项 | 含义 |
|---|---|---|
| 1 | Wails `ai.StreamEvent` 所有 `Type` 值与字段 | `content` / `thinking` / `thinking_done` / `tool_call` / `tool_result` / `approval_request` / `approval_resolved` / `queue_consumed_batch` / `error` / `retry` / `done` 全部保持现状；前端 0 改 |
| 2 | `@mention` 解析、UI mention chip 渲染、自动开 tab 副作用 | 全部保留；mention 列继续按当前格式落 `conversation_messages.mentions` |
| 3 | 命令策略 gate 规则 | k8s / kafka / mongodb / redis / ssh / query / opsctl 各 policy 不动 |
| 4 | 审批 UX | 预览框、approve once / always、LocalGrants（write/edit "始终允许"）、deny 文案 i18n |
| 5 | 审计落库内容与格式 | `audit_repo` 写入字段不变 |
| 6 | 回合上限默认 50，每次 Send 复位 | `MaxRounds` 默认值与计数语义不变 |
| 7 | `dispatch_subagent` 截断 summary 行为 | 父 stream 仍仅看到 PreToolUse / PostToolUse + 截断 summary |
| 8 | Skills / extensions / slash command 注入 | 进 prompt 的内容与时机不变 |
| 9 | Token usage 的 UI 展示 | per-message token usage 落库内容不变（仅 row key 从 `cago_id` 换成 `(conversation_id, sort_order)`），按现有 StreamEvent 推送 |
| 10 | Steer 展示文本 ≠ LLM body | 用户输入 `@srv1 status`，UI 显示原文，LLM 拿到 mention-expanded body |
| 11 | `queue_consumed_batch` 聚合 | 多条 steer 在前端表现为一个批次 |
| 12 | 跨 app 重启恢复会话 | `conversation_messages` 读出来还是同一棵会话 |

## 4. 用 v2 原生能力覆盖的场景

cago v2 默认 `DefaultStripPartialReasons = [PartialStreaming]`：只 strip 流式中残留的 streaming partial（启动时清异常），其余 PartialReason（cancelled / errored / tokens_limit / timeout）默认**保留**并在下一轮 LLM 输入里回放。这一行 default 一次性覆盖了产品的多个核心场景：

| 场景 | v2 落点 |
|---|---|
| 停止 → 已输出保存 → 下次 Send 带回 | `runner.Cancel` 同步 flush，落 `PartialReason="cancelled"`；下一轮 `BuildRequest` 默认保留它喂回 LLM |
| 错误 → 已输出保存 → 错误气泡 → 下次 Send 带回（不要错误本身） | 流中失败后 partial 落 `PartialReason="errored"`，错误本身只通过 `EventError` 上报、**不写进 conv 的 Content**；下一轮默认保留 errored partial 喂回 LLM；UI 渲染 partial 内容 + 单独错误气泡 |
| Steer 中插 | `runner.Steer(ctx, llmText, WithSteerDisplay(raw))`；safe point 注入；turn 即将以 no-tool 收尾时若有 pending steer 自动 auto-continue 一轮 |
| 编辑某条重发 | `conv.Truncate(idx)` + `conv.Append(userMsg)` + `runner.Resend(ctx)` |
| 某条重新生成 | `conv.Truncate(assistIdx)` + `runner.Resend(ctx)` |
| 自动重试 | `agent.Retry(RetryPolicy{MaxAttempts:10, BackoffFn: opskatSchedule})` Option；`EventRetry` 携带 `Attempt`/`Delay`/`Cause`；retry 间 errored partial 自动喂回 |
| 多会话并跑 | 每个 ConvHandle 独占一个 `*Conversation` + `*Runner`；Cancel / Steer / 事件互不串扰 |
| 崩溃后重启 | `Manager.OpenConversation(convID)` 加载时把末尾 `streaming` partial 强制改写为 `errored`（store 层 invariant，单点） |

**结论**：5 个产品场景 + 3 个隐含必备场景全部由 v2 native 覆盖；OpsKat 端只剩"配胶水"的工作。

## 5. cago 上游 PR（A1+A2）

v2 当前漏掉一处对所有 chat-app agent 通用的能力：**用户输入有"展示文本"和"LLM body"两条值**（mention / macro 展开后给 LLM 的串和给 UI 看的原文不一样）。OpsKat v1 时代用旁路 FIFO 兜底，bug-prone。本次同步上游修复。

### 5.1 新增公共 API

```go
// agents/agent/send_options.go
func WithSendDisplay(displayText string) SendOption  // 在 user msg 上挂 MetadataBlock{Key:"display", Value:displayText}

// agents/agent/runner.go - Steer 加 opts...
func (r *Runner) Steer(ctx context.Context, llmText string, opts ...SteerOption) error

type SteerOption interface { /* sealed */ }
func WithSteerDisplay(displayText string) SteerOption

// agents/agent/message.go - 新 ContentBlock 实现
type MetadataBlock struct {
    Key   string
    Value any
}
func (MetadataBlock) contentBlockType() string { return "metadata" }
```

### 5.2 行为约束

- **InputBuilder 跳过 MetadataBlock**：构造 ChatRequest 时滤掉 `MetadataBlock`，避免污染 LLM 输入
- **Recorder/Store 透传 MetadataBlock**：作为普通 ContentBlock JSON 序列化落库；存储侧无侵入
- **Hook 见到的 ConversationReader 包含 MetadataBlock**：UserPromptSubmit hook 可读取 display 值

### 5.3 测试

`agents/agent/inputbuilder_test.go`：增 `TestBuildRequest_StripsMetadataBlock`
`agents/agent/runner_steer_test.go`：增 `TestSteer_PreservesDisplayMetadata`
`agents/agent/runner_userprompt_test.go`：增 `TestSend_PreservesDisplayMetadata`

### 5.4 PR 顺序

cago PR 先合并并发布；OpsKat 这边 `go.mod` bump cago 到含 A1+A2 的版本后才动 OpsKat 代码。

## 6. OpsKat 端架构

### 6.1 包布局

```
internal/aiagent/
├── manager.go            # *Manager: 单例 *agent.Agent + per-conv *ConvHandle 缓存
├── handle.go             # *ConvHandle: 一个会话的入口 (Send/Steer/Cancel/Edit/Regenerate/Close)
├── bridge.go             # eventBridge: Runner.OnEvent + Conv.Watch → ai.StreamEvent
├── store_gorm.go         # agent/store.Store impl 落 conversation_messages
├── recover.go            # Conversation 加载时 streaming → errored 修正
├── hook_mentions.go      # UserPromptSubmit: mention 解析 + 开 tab + LLM body 改写
├── hook_policy.go        # PreToolUse: 命令策略
├── hook_audit.go         # PostToolUse: 审计落库（直连 audit_repo，无 sidecar）
├── hook_rounds.go        # PreToolUse: 回合上限闭包计数；OnRunnerStart 复位
├── hook_approval.go      # PreToolUse: 阻塞等审批；裹 cago agent/approve.Approver
├── approval_gateway.go   # ApprovalGateway 壳保留：Wails 事件契约不动；内部 wraps Approver
├── subagent_dispatch.go  # dispatch_subagent tool 适配（PostToolUse 截断 summary）
├── deps.go               # 依赖注入入口
└── mock_*/               # mockgen 生成
```

**消失的旧文件**：`system.go` / `sidecar.go` / `retry.go` / `event_bridge.go`（合并到 `bridge.go`）/ `emitter.go` 之外的 `EventEmitter` 内部封装。

### 6.2 *Manager

```go
type Manager struct {
    agent     *agent.Agent              // 单例（基于当前 provider 配置 + 全部 hooks）
    store     agent.Store                // gormStore（包了 agent/store.Recorder）
    approver  *approve.Approver          // cago 审批底座
    handles   map[int64]*ConvHandle      // per-conv handle 缓存
    mu        sync.Mutex
    emitter   EventEmitter
    deps      *Deps
}

func NewManager(opts ManagerOptions) *Manager

func (m *Manager) Handle(ctx context.Context, convID int64) (*ConvHandle, error) {
    // 缓存命中直接返；否则 LoadConversation + 启动 bridge + 创建 Runner + 缓存
}

func (m *Manager) CloseConversation(convID int64) error
```

`Manager` 在 App 启动时构造；切换 provider / model 时重构（与现状一致）。

### 6.3 *ConvHandle

```go
type ConvHandle struct {
    convID  int64
    conv    *agent.Conversation
    runner  *agent.Runner             // 长生命，独占 conv；多次 Send 复用
    bridge  *eventBridge              // 订阅 conv.Watch + runner.OnEvent → emitter
    unbindStore func()                // agent/store.Recorder.Bind 的 Unsubscribe
}

func (h *ConvHandle) Send(ctx context.Context, raw, llmBody string) error {
    // 唯一入口同时处理"全新发送"和"steer 中插"
    err := h.runner.Steer(ctx, llmBody, agent.WithSteerDisplay(raw))
    if err == nil {
        return nil  // mid-turn injected; auto-continue 接管
    }
    if !errors.Is(err, agent.ErrSteerNoActiveTurn) {
        return err
    }
    // turn 已结束 → 起新轮
    events, err := h.runner.Send(ctx, llmBody, agent.WithSendDisplay(raw))
    if err != nil {
        return err
    }
    for range events { /* drain；bridge goroutine 已经在 OnEvent 处理推送 */ }
    return nil
}

func (h *ConvHandle) Cancel(reason string) error {
    return h.runner.Cancel(reason)
}

func (h *ConvHandle) Edit(ctx context.Context, idx int, raw, llmBody string) error {
    if err := h.conv.Truncate(idx); err != nil { return err }
    h.conv.Append(buildUserMessage(llmBody, raw))
    return h.resend(ctx)
}

func (h *ConvHandle) Regenerate(ctx context.Context, assistIdx int) error {
    if err := h.conv.Truncate(assistIdx); err != nil { return err }
    return h.resend(ctx)
}

func (h *ConvHandle) Close() error
```

`runner` 是长生命的 per-conv 独占资源——同一会话不并发 Send；一次 Send 用完后 Runner 不 close，等下一次 Send/Steer 复用，直到 ConvHandle 关闭。

## 7. Hook 拓扑（4-stage）

| Stage | Matcher | Hook | 业务 |
|---|---|---|---|
| `UserPromptSubmit` | — | `hook_mentions.go` | 解析 `@mention`，开 tab 副作用，把 LLM body 改写为 mention-expanded 文本（`ModifiedText`），原文通过 `WithSendDisplay`/`WithSteerDisplay` 在 Message 上挂 MetadataBlock |
| `PreToolUse` | `""` | `hook_policy.go` | 命令策略 gate（沿用现有 `internal/ai/*_policy.go`） |
| `PreToolUse` | `""` | `hook_rounds.go` | 回合上限闭包计数，`MaxRounds=50` |
| `PreToolUse` | `"Bash\|Write\|Edit\|...审批工具集"` | `hook_approval.go` | 阻塞等 `*Approver` 决议；deny 时返 DenyReason（i18n） |
| `PostToolUse` | `""` | `hook_audit.go` | `audit_repo.Insert(...)`（直连，无 sidecar） |
| `PostToolUse` | `"dispatch_subagent"` | `subagent_dispatch.go` | 截断 child summary，回写父 ToolResultBlock |
| Lifecycle | — | `OnRunnerStart` | rounds 计数器复位 |

注意 v1 的 `SessionStart` 没有直接对应——其语义现在分成：构造 Agent 时的 Option（一次性配置）+ `OnRunnerStart` 的每轮 lifecycle 回调。

## 8. 持久化设计

### 8.1 Schema 变更

`migrations/202605100001_aiagent_v2_schema.go`：

```go
// Up:
//   1. 删除列：
//        cago_id, parent_id, kind, origin, persist,
//        tool_call_id, tool_calls, thinking,
//        tool_call_json, tool_result_json, raw, content
//   2. 新增列：
//        partial_reason VARCHAR(16) NOT NULL DEFAULT ''
//   3. 新建唯一索引：
//        idx_conv_msg_unique ON conversation_messages(conversation_id, sort_order)
//   4. backfill：
//        把已有行的 content/tool_*_json/raw 折叠进 blocks（多数行已有 blocks）
//        sort_order 已存在；保证每会话内单调
```

`(conversation_id, sort_order)` 成为行级 upsert / DELETE 的自然键，对应 cago v2 的 `(convID, index)` 寻址。

`conversations.session_data` / `state_values` 本次保留列，**仅停止写入**——后续独立 PR 删。

### 8.2 store_gorm.go：实现 `agent.Store`

```go
type gormStore struct {
    repo conversation_repo.ConversationRepo
}

func (g *gormStore) AppendMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
    // 序列化 msg → conversation_entity.Message：role / blocks(JSON) / partial_reason / token_usage
    // INSERT 行 (conversation_id, sort_order=index)；冲突视为 UpdateMessage
}

func (g *gormStore) UpdateMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
    // UPDATE WHERE conversation_id=? AND sort_order=?
    // 用于 ChangeFinalized：补 partial_reason / token_usage / blocks 终态
}

func (g *gormStore) TruncateAfter(ctx context.Context, sessionID string, index int) error {
    // DELETE WHERE conversation_id=? AND sort_order >= ?
}

func (g *gormStore) LoadConversation(ctx context.Context, sessionID string) ([]agent.Message, agent.BranchInfo, error) {
    // SELECT ... ORDER BY sort_order ASC
    // 末尾 streaming → errored 修正在这里执行（recover.go 内）
}
```

`Manager.OpenConversation(convID)` 时：
```go
msgs, branch, _ := store.LoadConversation(ctx, fmt.Sprint(convID))
conv := agent.LoadConversation(fmt.Sprint(convID), msgs, agent.WithBranchedFrom(branch))
unbind := agentstore.Recorder(store).Bind(conv)
```

### 8.3 Mention / Usage 数据落库

- **Mentions**：`hook_mentions.go` 在 `UserPromptSubmit` 中解析后，作为 `MetadataBlock{Key:"mentions", Value:[]ai.MentionedAsset}` 挂在 user msg；`gormStore.AppendMessage` 检测到 Role=user 时，从 Content 里的 MetadataBlock 提取 mentions JSON 写入 `conversation_messages.mentions` 列。`pendingMentions` map / drain 全部删除。
- **Token usage**：v2 `Message.Usage` 是原生字段，`ChangeFinalized` 时由 cago 自动填充；`gormStore.UpdateMessage` 直接读 `msg.Usage` 序列化到 `conversation_messages.token_usage`。`pendingUsage` map / drain 全部删除。
- **Steer display**：`MetadataBlock{Key:"display", Value:rawText}` 由 cago 在 `Steer` / `Send` 注入；`gormStore` 透传。前端通过 conversation_messages 读出 blocks JSON，按 MetadataBlock.Key 渲染。`pendingDisplays` FIFO 删除。

## 9. Event bridge

bridge 一个 goroutine，订阅 `Runner.OnEvent(AnyEvent, ...)` + `Conv.Watch()`，把 cago 事件翻译成现有 `ai.StreamEvent` 推到 `EventEmitter`。

### 9.1 事件映射

| 来源 | 现 `StreamEvent.Type` | 备注 |
|---|---|---|
| `Event{Kind: EventTextDelta}` | `content` | `Content: ev.Delta` |
| `Event{Kind: EventThinkingDelta}` | `thinking` | |
| `Event{Kind: EventMessageEnd}` 且 `PartialReason==""` | `thinking_done`（如有 thinking） | 仅 finalize 信号 |
| `Event{Kind: EventPreToolUse}` | `tool_call` | tool name + input |
| `Event{Kind: EventPostToolUse}` | `tool_result` | tool name + output blocks + IsError |
| `Event{Kind: EventError}` | `error` | 解包 `*HookError` 时 Error 文案带 stage；不发 `done`，由 EventDone 单点收尾 |
| `Event{Kind: EventCancelled}` | （不直接 emit） | 取消状态已在 `conv.PartialReason="cancelled"`；由 EventDone 单点收尾 |
| `Event{Kind: EventTurnEnd}` | （观察用） | bridge 内部用作 batch flush |
| `Event{Kind: EventDone}` | `done` | **唯一**的"完成"信号；同一次 Send 只 emit 一次 done |
| `Event{Kind: EventUserPromptSubmit}` | `queue_consumed_batch`（聚合） | 见 §9.2 |
| `Event{Kind: EventRetry}` | `retry` | `Content: fmt.Sprintf("%d/%d", ev.Retry.Attempt, MaxAttempts)`, `Error: ev.Retry.Cause.Error()` |
| `Event{Kind: EventCompacted}` | （本次不接） | compaction 不在范围 |
| `Approver.Pending()` | `approval_request` | `ApprovalGateway` 直接 emit，不走 bridge |
| `Approver.Approve/Deny` 副作用 | `approval_resolved` | 同上 |

### 9.2 `queue_consumed_batch` 聚合

cago `drainSteer` 在一个 safe point 可能一次注入多条 user msg（前一个 turn 结束前用户 steer 多条）。bridge 维护小累加器：

- 收到 `EventUserPromptSubmit` → 从 `Conv` 读最新 user msg 的 MetadataBlock display 值 push 进 buffer
- 收到任何非 `EventUserPromptSubmit` 事件 → flush buffer 为一条 `queue_consumed_batch`，携带 `QueueContents: [...]`
- 收到 `EventTurnEnd` / `EventDone` → flush

行为与 v1 时代 `pendingFollowUps` 一致；前端 0 改。

### 9.3 实现

```go
type eventBridge struct {
    convID  int64
    emit    EventEmitter
    conv    agent.ConversationReader
    pending []string  // queue_consumed_batch 累加器
}

func newBridge(convID int64, em EventEmitter, conv *agent.Conversation, runner *agent.Runner) (*eventBridge, func()) {
    b := &eventBridge{convID: convID, emit: em, conv: conv}
    unsub := runner.OnEvent(agent.AnyEvent, b.onEvent)
    // Conv.Watch 用于 store；bridge 仅吃 OnEvent
    return b, unsub
}

func (b *eventBridge) onEvent(ctx context.Context, ev agent.Event) {
    if ev.Kind != agent.EventUserPromptSubmit && len(b.pending) > 0 {
        b.flushBatch()
    }
    switch ev.Kind {
    case agent.EventUserPromptSubmit:
        b.pending = append(b.pending, b.readDisplayFromLastUser())
    case agent.EventTextDelta: b.emit.Emit(b.convID, ai.StreamEvent{Type: "content", Content: ev.Delta})
    // ... 其它 case
    }
}
```

## 10. 审批

### 10.1 cago `agent/approve.Approver` 复用

```go
m.approver = approve.New()

a := agent.New(prov,
    agent.PreToolUse("Bash|Write|Edit|<其它需审批工具>", approve.Hook(m.approver)),
    // ...
)
```

`*Approver.Pending() iter.Seq[Pending]` 由 OpsKat 的 `ApprovalGateway` 消费。

### 10.2 ApprovalGateway 壳：保 Wails 契约

```go
type ApprovalGateway struct {
    emit     EventEmitter
    approver *approve.Approver
    grants   LocalGrantStore  // write/edit "始终允许" 持久化
    resolver PendingResolver  // 现有 channel-based 等待 UI 结果
}

func (g *ApprovalGateway) Run(ctx context.Context) {
    for p := range g.approver.Pending() {
        if g.grants.Allowed(ctx, p.ToolName, p.ToolUseID, p.Input) {
            g.approver.Approve(p.ID)
            continue
        }
        g.emit.Emit(p.ConvID, ai.StreamEvent{Type: "approval_request", ...})
        // 等 PendingResolver
        decision := g.resolver.Wait(ctx, p.ID)
        switch decision.Action {
        case "approve":   g.approver.Approve(p.ID)
        case "always":    g.grants.Allow(ctx, ...); g.approver.Approve(p.ID)
        case "deny":      g.approver.Deny(p.ID, i18n.T(decision.Reason))
        }
        g.emit.Emit(p.ConvID, ai.StreamEvent{Type: "approval_resolved", ...})
    }
}
```

Wails 事件 `approval_request` / `approval_resolved` 形态与现状一致——前端 0 改。

## 11. 自动重试

```go
agent.Retry(agent.RetryPolicy{
    MaxAttempts: 10,
    BackoffFn:   opskatRetrySchedule,  // [2,4,8,15×7]s + ±20% jitter
    // ShouldRetry: nil → cago 默认（408/425/429/5xx + net.Error{Timeout} + EOF/connection reset）
})
```

retry.go 整段删除。`opskatRetrySchedule` 是从老 retry.go 抽出来的纯函数（4 行）。

`EventRetry` 由 cago 在重试前 emit；bridge 翻译为 `StreamEvent{Type:"retry", Content:"N/10", Error:cause}`。`Retry-After` 由 cago 自动 honor（无需 OpsKat 介入）。

## 12. 崩溃恢复 invariant

`store_gorm.go::LoadConversation`：

```go
func (g *gormStore) LoadConversation(ctx context.Context, sessionID string) ([]agent.Message, agent.BranchInfo, error) {
    msgs := /* SELECT ... ORDER BY sort_order */
    if n := len(msgs); n > 0 && msgs[n-1].PartialReason == agent.PartialStreaming {
        msgs[n-1].PartialReason = agent.PartialErrored
    }
    return msgs, branchInfo, nil
}
```

单点 invariant：所有 conv 入口都过这里，永不可能见到"永远转圈"的 streaming 残留。

## 13. Cancel 透传

OpsKat 端的所有长跑动作（SSH exec / DB query / Redis op / Kafka op / k8s op）已经全部 select 在 ctx.Done()——这是 v1 时代就维持好的不变量。v2 不变化此处：

- `handle.Cancel()` → `runner.Cancel(reason)`
- cago 取消 turn ctx → 所有正在 await 的 hook（含 `approve.Hook`）立刻 return ctx.Err()
- cago dispatcher flush 后落 `PartialReason="cancelled"` → bridge emit cancelled+done

ApprovalGateway 的 PendingResolver 也必须 select on ctx.Done()——审批中按 Stop 必须立即解锁；这是回归测试的硬验收点。

cago `agent/approve.Hook` 内部对 ctx 的观测需要在实现期 verify：若 cago Hook 不在等 Approve/Deny 时同步 select on ctx.Done，则需要要么在 OpsKat ApprovalGateway 加一层 ctx-aware wrapper，要么向上游补丁。

## 14. 测试策略

### 14.1 单元

| 文件 | 焦点 |
|---|---|
| `manager_test.go` | OpenConversation 缓存命中 / 加载 / streaming 修正 / Close 释放 |
| `handle_test.go` | Send / Steer fallback / Cancel / Edit / Regenerate；用 cago `providertest.New()` 喂脚本；assert conv 形态 + emitter 抓到的 StreamEvent |
| `bridge_test.go` | 每个 cago Event 类型 → 对应 StreamEvent（snapshot test）；queue_consumed_batch 聚合；EventRetry 映射 |
| `store_gorm_test.go` | Append / Update / Truncate / Load + sqlmock；streaming → errored；Mentions / Usage 列写入；MetadataBlock 透传 |
| `hook_*_test.go` | 每个 hook 独立单测，沿用现有 mock_* / testutils |
| `approval_gateway_test.go` | grants 短路 / approve / always / deny / Cancel 中断 |
| `recover_test.go` | crash-restart 三种残留态修正 |

### 14.2 端到端

`aiagent_e2e_test.go`：

- T1：Send 跑通一轮带 tool；asssert conv 形态 + 全部 StreamEvent
- T2：流中 Cancel；assert PartialReason="cancelled" + 下一轮 LLM 输入包含 partial
- T3：流中 provider 503 不重试；assert PartialReason="errored" + 错误 StreamEvent
- T4：流中 provider 503 + Retry(3) → 第二次 200；assert EventRetry → "retry" StreamEvent；最终内容拼接正确
- T5：Steer 中插；assert 同一 turn 内连续两段 assistant text + queue_consumed_batch event
- T6：Steer 在 turn 结束后；assert ErrSteerNoActiveTurn 兜底走 Send
- T7：Edit U2 → 后续 conv 行为
- T8：Regenerate A2
- T9：审批 deny / approve / always / Cancel-during-pending
- T10：multi-conv 并跑：两个 ConvHandle 同时 Send + 各自 Cancel 不串扰

### 14.3 cago 上游 PR 测试

见 §5.3。

## 15. 迁移序列

按子 commit 分片（一个 PR），每片独立可编译可测：

| # | 内容 | 验证 |
|---|---|---|
| 1 | cago 上游 PR：A1+A2 | cago `make test` |
| 2 | OpsKat `go.mod` bump cago 到含 A1+A2 的版本 | `go build ./...` |
| 3 | schema 迁移文件 + backfill | `make migrate` 在 fresh + 已有 DB 上跑通 |
| 4 | `internal/aiagent/store_gorm.go` v2 实现（新文件，老 gormStore 暂留） | `go test ./internal/aiagent/...` |
| 5 | `manager.go` / `handle.go` / `bridge.go` / `recover.go` 新建 | 单元绿 |
| 6 | hooks 4-stage 迁移（mentions / policy / audit / rounds / approval / subagent_dispatch） | 每个 hook 单测绿 |
| 7 | `approval_gateway.go` 改造（裹 cago Approver） | 单测绿 |
| 8 | `app_ai.go` 切到新 `*Manager` 入口 | E2E 绿 |
| 9 | 删除 v1 残骸：`system.go` / `sidecar.go` / `retry.go` / `event_bridge.go` / `emitter.go` 旧实现 / pendingMentions/Usage/Displays | `go build` 全绿 |
| 10 | 验收：手测 5 个核心场景 + multi-conv + crash recovery + approval cancel | 通过 |

回滚：单 PR、commit 粒度可二分；任何阶段失败可 `git revert`。

## 16. 决定快照

| 维度 | 决定 |
|---|---|
| 整体形态 | D（无 god-object）；`*Manager` + `*ConvHandle` |
| Runner 寿命 | 长生命，per-conv 独占 |
| 编辑/重发语义 | Truncate（覆盖），不走 BranchFrom |
| Steer / 多轮输入 | 统一走 `handle.Send` → 优先 Steer + ErrSteerNoActiveTurn 兜底 Send；不再单独维护 post-turn queue |
| Retry | cago `agent.Retry(...)`；OpsKat retry.go 删除 |
| Persistence | `agent/store.Recorder` + `agent.Store` 实现；增量 Append/Update/Truncate；`(conversation_id, sort_order)` 自然键 |
| Mention / Usage 落库 | Hook + MetadataBlock + store impl 提取写 `mentions` / `token_usage` 列；删 pendingMentions/pendingUsage stash |
| Display ≠ LLM body | cago 上游加 `WithSendDisplay` / `WithSteerDisplay` / `MetadataBlock`（A1+A2 PR） |
| 审批 | cago `agent/approve.Approver` 底座 + OpsKat `ApprovalGateway` 壳保 Wails 契约 |
| Crash recovery | `LoadConversation` 单点 invariant：streaming → errored |
| Cancel 透传 | runner.Cancel → ctx 取消 → hooks / tools / approver 全部 select on ctx.Done |
| StreamEvent 契约 | 完全保留；前端 0 改 |
| `conversation_messages` schema | 删 v1 平铺列 + 加 partial_reason + (conv_id, sort_order) 唯一键 |
| `conversations.session_data` / `state_values` 列 | 本次仅停写；后续独立 PR 删 |
| `cago_id` 列 | 删；前端用 (conversation_id, sort_order) 引用 message |
| Branching / Compaction / Token-limit "Continue" | 不在范围 |

## 17. Open questions（实现期再定）

- **opskatRetrySchedule 的具体节奏**：`[2,4,8,15×7]s + ±20%` 直接搬过来 vs 借机改成纯 cago 默认指数退避（`InitialDelay=2s, MaxDelay=15s`）。倾向直接搬过来——保 UX 不变。
- **MetadataBlock.Value 的 JSON 序列化形态**：`map[string]any` 还是 `json.RawMessage`。倾向 `map[string]any`，简单。
- **Approval `*Approver` 是否每个 ConvHandle 一个还是 Manager 共享**：cago `Hook(*Approver)` 注册时 Approver 已经 captured；Manager 共享更简单（多 conv pending 用 ConvID 区分）。倾向共享。
- **Multi-conv 同时 Cancel 的 ApprovalGateway 行为**：`PendingResolver` 在 conv-1 Cancel 时不影响 conv-2 的 pending——需要 resolver per-conv channel 或 ID 路由。实现期定。
- **`OPSKAT_AI_BACKEND` 之类 feature flag**：本次不引入；纯切换。
- **cago `approve.Hook` 是否原生 select on ctx.Done()**：若否，OpsKat 加 wrapper 或上游补丁；不阻塞 spec 推进。

---

## 附录 A — 5 个产品场景到 v2 的精确映射

### A.1 输出时用户停止，已输出内容保存 + 下次发送带上

```
turn 1:
  handle.Send("讲故事") → runner.Send + WithSendDisplay
  EventTextDelta×N → bridge emit "content"
  user 点 Stop:
    handle.Cancel() → runner.Cancel("user")
  cago dispatcher flush → conv 末条 PartialReason="cancelled" + Content 含已流出文本
  bridge emit cancelled "done"

下一次 user 发"继续":
  handle.Send("继续") → runner.Send (Steer 失败 ErrSteerNoActiveTurn → fallback)
  cago BuildRequest 默认保留 cancelled partial，注入到 ChatRequest.Messages
  LLM 看到："上一轮 assistant 回了 [partial 文本]"，自然续写
```

### A.2 LLM API 失败，保留用户内容 + 错误气泡 + 下次带上

```
turn 1:
  handle.Send("查询库存") → runner.Send
  EventTextDelta×K → bridge "content"
  provider 502:
    cago 落 conv 末条 PartialReason="errored"，Content = 已流出文本
    EventError → bridge "error"（错误文案；不进 conv）
    EventDone → bridge "done"

UI:
  渲染 partial 文本气泡 + 下方独立"出错了"气泡（基于"error" event）

user 再 Send "继续":
  cago 默认保留 errored partial 注入到下一轮
  bridge 不再有 error event
```

### A.3 Steer：输出过程中继续输入并发送

```
turn 1:
  handle.Send("帮我看 A 服务器") → runner.Send + WithSendDisplay
  EventPreToolUse(ssh.exec) → bridge "tool_call"
  ...
  user 又在 UI 输入"也看下磁盘" → handle.Send("也看下磁盘"):
    runner.Steer(ctx, llmBody, WithSteerDisplay) → nil（active turn）
  cago drainSteer 在 safe point 把 user msg "也看下磁盘" append 到 conv
  EventUserPromptSubmit → bridge buffer
  下一轮 LLM call 拿到完整 history，回 "好的我一起看"
  EventTextDelta → bridge "content"（注意：触发 buffer flush 为 "queue_consumed_batch"）
```

### A.4 编辑某条重发

```
conv: U1 A1 U2 A2 U3 A3
user 编辑 U2 的内容:
  handle.Edit(idx=2, raw, llmBody)
    conv.Truncate(2)             → DELETE WHERE sort_order >= 2
    conv.Append(userMsg(llmBody, raw))  → INSERT sort_order=2
    runner.Resend(ctx)           → 拿新历史发 LLM
  conv 终态：U1 A1 U2' A2'
```

### A.5 重新生成

```
conv: U1 A1 U2 A2
user 点 A2 的 Regenerate:
  handle.Regenerate(assistIdx=3)
    conv.Truncate(3)             → DELETE WHERE sort_order >= 3
    runner.Resend(ctx)           → 同 U2 发出
  conv 终态：U1 A1 U2 A2'（新内容）
```
