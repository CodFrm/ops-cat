# Replace Built-in Agent with Cago Coding Agent — Design

- Status: Draft
- Author: 王一之
- Date: 2026-05-07
- Related code: `internal/ai/`, `internal/app/app_ai*.go`, `internal/app/app_approval.go`
- External: [cago-frame/agents](https://github.com/cago-frame/agents) (`/Users/codfrm/Code/cago/agents`)

## 1. Goal & Product Direction

Replace the in-house AI agent loop in `internal/ai/` with the [cago coding agent framework](https://github.com/cago-frame/agents), specifically the `app/coding` package, so OpsKat AI gains:

- Mature multi-turn loop / hook / observer / session / subagent dispatcher / compactor — currently re-invented in OpsKat.
- Built-in **local coding tool suite** (bash / read / write / edit / grep / find / ls / todo) on top of OpsKat's existing **remote ops tools** (SSH / DB / Redis / MongoDB / Kafka / k8s / extensions).

**Product change**: OpsKat AI evolves from a remote-ops assistant into a "workstation assistant" that also operates the user's local filesystem and shell — gated by the same approval/audit pipeline.

## 2. Decisions Locked In

The product/architecture decisions below are confirmed inputs to this design.

| Decision | Choice |
|---|---|
| Replacement scope | Adopt the full `app/coding` system (not just `agent` core, not just provider) |
| Local coding tools (bash/read/write/edit/grep/find/ls/todo) | **Keep all** — AI gets local + remote capability |
| Working directory (`cwd`) policy | **Per-conversation cwd**, remembered across runs; UI lets user pick/switch |
| Subagent system | Use cago `dispatch_subagent` + custom OpsKat ops Entry; **delete** OpsKat's `spawn_agent` |
| Provider layer | Use cago's built-in providers; **upstream-contribute** a real Anthropic implementation to cago |

## 3. Architecture

### 3.1 New layering

```
internal/app/app_ai.go (Wails bindings, slimmed)
       │
       ▼
internal/aiagent/                       ← new package, replaces the Agent loop in internal/ai
  ├─ system.go        coding.New(...) wrapper + lifecycle (per-conversation)
  ├─ tools_ops.go     OpsKat ops tools (list_assets / run_command / exec_sql / ...) wrapped as cago tool.Tool
  ├─ tools_ext.go     exec_tool (extension WASM dispatch) wrapped as cago tool.Tool
  ├─ subagent_ops.go  ops-explorer Entry (subagent.Entry) injected into dispatch_subagent
  ├─ hook_policy.go   CommandPolicyChecker → PreToolUse hook (approval interception, window activation)
  ├─ hook_audit.go    AuditingExecutor → PostToolUse hook (audit log writes; reads CheckResult from per-call sidecar)
  ├─ hook_rounds.go   PreToolUse hook enforcing max-rounds cap (50 main / 30 sub / 100 absolute)
  ├─ hook_prompt.go   UserPromptSubmit hook injecting per-turn AdditionalContext (open tabs / mentions / ext SKILL.md)
  ├─ event_bridge.go  cago Stream → Wails ai:event:%d StreamEvent translator (synthesizes thinking_done + tags sub-agent events)
  ├─ store.go         cago Session.Store backed by conversation_entity.Message
  ├─ system_prompt.go static parts of PromptBuilder; piped through coding.AppendSystem at System construction
  └─ retry.go         retry wrapper (cago does not bundle retry; replays last user prompt via Session.FollowUp)

internal/ai/  (kept; consumed by aiagent)
  ├─ command_policy.go / *_policy.go   policy logic, CheckPermission(...) unchanged
  ├─ command_rule.go / command_shell.go
  ├─ audit.go (DefaultAuditWriter / ToolCallInfo)  business code kept; AuditingExecutor wrapper retired
  ├─ approval_types.go
  ├─ tool_handlers_*.go    handler bodies kept; wrapped by aiagent/tools_ops.go
  ├─ fetch_models.go       unchanged
  ├─ prompt_builder.go     unchanged
  └─ provider.go / anthropic.go / openai.go → DELETED, replaced by cago provider

cago upstream change (PR)
  └─ provider/anthropics: full implementation (migrated from OpsKat anthropic.go),
       supports extended thinking + cache breakpoint + ThinkingConfig
```

### 3.2 Conversation lifecycle

- On first send to a conversation, call `coding.New(ctx, prov, conv.Cwd, opts...)` exactly once and store the `*coding.System` + bound `*agent.Session` in `App.runners`.
- On conversation switch / delete: `sys.Close(ctx)`.
- On provider switch: `resetRunners()` closes everything; next send rebuilds.
- **App-shared (one instance per process)**: `App.sshPool` (the long-lived SSH dial pool). Today it is injected via context; the new design hands it to each `aiagent.Deps` at System construction so all per-Conversation tools dial through it.
- **Per-Conversation (one instance per `*coding.System`)**: `SSHClientCache`, `MongoDBClientCache`, `kafka_svc.Service` (Kafka client manager). These are short-lived per-session caches today and stay so; they live as fields on the cago `tool.Tool` struct instances bound to that System.
- This split preserves today's behavior: the dial pool is shared (no extra TCP), but per-call caches don't bleed across conversations.

### 3.3 Wails event bridge

A single goroutine per active stream drains `stream.Next()` and translates each `agent.Event` into the existing `ai:event:%d` Wails event shape. **Frontend stays untouched.**

| cago `agent.Event` | OpsKat `StreamEvent` |
|---|---|
| `EventTextDelta` | `content` |
| `EventThinkingDelta` | `thinking` |
| `EventPreToolUse` | `tool_start` |
| `EventPostToolUse` | `tool_result` |
| `EventUsage` | `usage` |
| `EventDone` | `done` |
| `EventError` | `error` |
| `EventCompacted` | (no equivalent — log only) |
| `EventStop` | (internal — used to detect end-of-turn) |
| `EventMessageEnd` | drives synthesized `thinking_done` (see below); otherwise dropped |

**Frontend event audit** (verified against `frontend/src/stores/aiStore.ts:handleStreamEvent`): the consumed event types are exactly `content`, `thinking`, `thinking_done`, `usage`, `agent_start`, `agent_end`, `tool_start`, `tool_result`, `approval_request`, `approval_result`, `queue_consumed`, `stopped`, `retry`, `done`, `error`. **The legacy `tool_call` aggregate event is emitted by the current backend but has no `case` branch in the frontend switch — it is dead.** The bridge does not synthesize it.

**ID mapping**: `ToolEvent.ID` (cago) → `StreamEvent.ToolCallID` (OpsKat). Frontend uses this to restore tool history across turn reloads, so the bridge must not lose / regenerate IDs.

**Token usage mapping** (`provider.Usage` → `StreamEvent.Usage`):

- `PromptTokens` → `InputTokens`
- `CompletionTokens` → `OutputTokens`
- `CachedTokens` → `CacheReadTokens`
- **(new)** `CacheCreationTokens` → `CacheCreationTokens` — requires adding a `CacheCreationTokens int` field to cago's `provider.Usage` (see §4.1). Without it Anthropic cache-write cost reporting regresses.

Events with **no cago equivalent**, emitted manually by OpsKat code:

- `thinking_done` — synthesized by the bridge as a "thinking phase ended" marker. State machine: enter "thinking-active" on first `EventThinkingDelta`; emit `thinking_done` when the next event of any other kind arrives (typically `EventTextDelta` or `EventPreToolUse`).
- `approval_request` / `approval_result` — emitted from `hook_policy.go` before/after blocking on the approval channel.
- `queue_consumed` — emitted from the wrapper that calls `Stream.Steer` for queued user messages.
- `agent_start` / `agent_end` — emitted from a per-subagent observer registered on the ops Entry (so child events surface in the parent stream). The observer also tags each forwarded event with the sub-agent's role/task so `tool_start` / `approval_request` events fired inside the sub-agent carry the right `agent_role` / `agent_task` fields (see §6 propagation risk).
- `retry` — emitted from `retry.go` between attempts.
- `stopped` — emitted when ctx cancellation interrupts the run.

## 4. Cago Upstream Changes

**One real new file (`provider/anthropics`), plus minimal additions to existing types.** Everything else is absorbed inside OpsKat.

### 4.1 Required: `provider/anthropics` full implementation

The current `provider/anthropics` is a placeholder (per cago README). The OpsKat anthropic provider is production-grade and must be ported up.

Scope of the PR:

- Implement `Provider.Name() / ChatCompletion() / ChatStream()`.
- Anthropic Messages API SSE parsing → cago `StreamChunk` (delta semantics):
  - `content_block_delta(text_delta)` → `StreamChunk.ContentDelta`
  - `content_block_delta(thinking_delta)` → `StreamChunk.ThinkingDelta`
  - `content_block_start/stop(tool_use)` + `input_json_delta` → `StreamChunk.ToolCallDelta`
  - `message_delta(stop_reason, usage)` → final chunk's `FinishReason` + `Usage`
- `provider.ThinkingConfig.Effort` → Anthropic `thinking: {type:"enabled", budget_tokens}` map (`low=1024 / medium=4096 / high=16000`).
- Automatic prompt-cache `cache_control: ephemeral` injection on the last system block + last 1-2 user blocks (the existing OpsKat `anthropic.go` strategy — verbatim port).
- Retry-After header propagation: surface as `*provider.ProviderError` on `StreamChunk.Err`. cago does not currently have this type — **add it to `cago/provider`** as part of the same PR (`type ProviderError struct { Err error; RetryAfter string; StatusCode int }`).
- Add `CacheCreationTokens int` field to `provider.Usage` to capture Anthropic prompt-cache **write** counts (today cago only models cache **read** via `CachedTokens`). Both OpsKat's existing telemetry and the new `provider/anthropics` need this.
- **Round-trip Anthropic `ThinkingBlock.Signature`**: `provider.Message.Thinking[].Signature` already exists in cago — the new `provider/anthropics` must emit signatures into `Thinking[]` on response and re-send them verbatim on the next request (Anthropic extended-thinking spec requires this for interleaved tool calls). Verify the openai provider doesn't strip Signature from Message values that pass through it (it shouldn't, since OpenAI ignores the field).

### 4.2 Required: DeepSeek-v4 `reasoning_content` round-trip in `provider/openai`

Today `internal/ai/agent.go:needsReasoningContent` forces the assistant message to carry `reasoning_content` back when calling DeepSeek-v4 thinking models — without it, a multi-turn turn-2 request fails 400 ("messages with tool_calls must have reasoning_content").

cago's `provider/openai` does not currently model this. Two options:

1. **Upstream patch** to `cago/provider/openai`: when serializing an assistant `Message`, populate `reasoning_content` from `Message.Thinking[*].Text`; when deserializing the streamed response, route `delta.reasoning_content` back into `Thinking[]`. **Recommended** — small (~30 LOC), benefits all DeepSeek users.
2. **OpsKat-side wrapper provider** that decorates the upstream openai provider with the reasoning_content roundtrip. Local, no upstream review delay.

Default to option 1; fall back to option 2 if the PR stalls.

### 4.3 Not changing cago — verified

- **Subagent events not bubbling** — by design (`subagent` spec §11.1). Worked around with an `agent.Observe` on each ops Entry's child agent that converts subagent events into parent-stream `agent_start`/`agent_end`/sub-tool events.
- **Blocking approval via PreToolUse hook** — `Hook.Fn` is synchronous; returning `Deny()` blocks the tool. Direct equivalent of `CommandConfirmFunc`.
- **Mid-cycle user message injection** — `Stream.Steer` covers `QueueMessage` semantically.
- **Compaction** — `coding.WithCompactionThreshold(N)` auto-triggers; `coding.System.Compact(ctx, sess)` manual. Replaces `internal/ai/compress.go`.
- **`ThinkingEffort` enum** — OpsKat's `xhigh` / `max` map down to `high` in the OpsKat-side adapter; not enough signal to extend cago's enum yet.

## 5. OpsKat Internal Changes (by risk)

### 5.1 Delete / replace (high-risk — needs full regression)

| Delete | Replacement |
|---|---|
| `internal/ai/agent.go` (Agent + DefaultToolExecutor) | `internal/aiagent/system.go` (cago `coding.System` wrapper) |
| `internal/ai/conversation_runner.go` | cago `Session.Stream` + `internal/aiagent/retry.go` |
| `internal/ai/provider.go` Provider interface | cago `provider.Provider` |
| `internal/ai/anthropic.go` / `internal/ai/openai.go` | cago providers (anthropic via §4.1 PR) |
| `internal/ai/compress.go` | cago compactor |
| `internal/ai/tool_handler_agent.go` (`spawn_agent`) | cago `dispatch_subagent` + ops Entry |
| `internal/ai/tool_handler_batch.go` (`batch_command`) | Kept as a parent-level standalone tool (mirrors today). The handler body is unchanged; it's wrapped by `tools_ops.go` like any other ops tool. |
| `internal/ai/audit.go` `AuditingExecutor` wrapper | PostToolUse hook |

### 5.2 Keep & adapt (low-risk)

| Keep | Adaptation |
|---|---|
| `*_policy.go` / `command_rule.go` / `command_shell.go` | unchanged; called from inside hooks |
| `audit.go` `DefaultAuditWriter` / `ToolCallInfo` / `WriteToolCall` | unchanged; called from PostToolUse hook |
| `tool_handlers_asset.go` / `tool_handlers_exec.go` / `kafka_*.go` / `redis_helper.go` / etc. handler funcs | unchanged; wrapped by a `ToolDef → tool.Tool` adapter helper |
| `prompt_builder.go` (PromptBuilder + AIContext + MentionedAsset) | unchanged; output piped via `coding.AppendSystem(...)` |
| `fetch_models.go` | unchanged (orthogonal feature) |
| `policy_ext.go` (extension policy) | unchanged |
| `notifier.go` (audit / approval notification) | unchanged |
| `conn_cache.go` (generic typed cache for SSH / MongoDB) | unchanged; consumed by the new per-System tool struct fields |
| `policy_tester.go` + `*_test.go` | unchanged (test infra) |

### 5.3 New (medium-risk)

- `internal/aiagent/system.go` — per-Conversation `coding.System`. Conversation entity gains a `Cwd string` field (gormigrate migration: default = user home).
- `internal/aiagent/event_bridge.go` — Event translator; the only place that emits Wails events for AI streaming.
- `internal/aiagent/hook_policy.go` — PreToolUse hook. Inspects `ToolEvent.Name` + parses `ToolEvent.Input` to recover `asset_id` + `command`, then calls `CheckPermission`. On `NeedConfirm` emits `approval_request`, blocks on the approval channel, emits `approval_result`. On `Deny` returns `agent.Deny(reason)`.
- `internal/aiagent/hook_audit.go` — PostToolUse hook. Calls `DefaultAuditWriter.WriteToolCall` with the same `ToolCallInfo` shape used today (decision is read from a per-call `*CheckResult` planted by the policy hook).
- `internal/aiagent/subagent_ops.go` — `ops-explorer` (and possibly `ops-batch`) `subagent.Entry` factories. Tool subset: `list_assets`, `get_asset`, `list_groups`, `get_group`, read-only `run_command` / `exec_sql` / `exec_redis` / `exec_mongo` (still policy-gated). Each Entry's child agent is registered with an Observer that re-emits its events into the parent stream as `agent_start` / `agent_end` / sub-tool events.
- `internal/aiagent/retry.go` — wraps `Session.Stream`. Catches `EventError` carrying a `*ProviderError`, honors `Retry-After`, retries up to 10× with the existing exponential+jitter schedule. On retry emits a `retry` Wails event.
- `internal/aiagent/tools_ops.go` — `wrapToolDef(def ToolDef, deps *Deps) tool.Tool`. Generates the JSON Schema from `[]ParamDef` (mirrors `ToOpenAITools`), routes `Call(ctx, args)` through the existing handler signature `(ctx, map[string]any) (string, error)`, and returns the string result (cago `Tool.Call` returns `any`, so a string is a valid result).

### 5.4 Behaviors explicitly preserved (no regressions)

This is the canonical list of current OpsKat AI behaviors that the new design **must** preserve. Each item names the existing call site and the new home.

**Conversation loop**

- Max-rounds cap (`DefaultMainAgentRounds=50`, `MaxAbsoluteRounds=100`, sub-agent default 30): cago has no built-in cap. Implemented as a counting `PreToolUse` hook in `aiagent/hook_rounds.go` that emits `agent.StopRun(reason)` when the count exceeds the limit.
- Tool result truncation (`MaxResultLen=32KB`) with the helpful tail message (`"Output too large (X bytes ...). Use more precise filters, pipe through | head or | grep, or split the query."`): implemented in the `wrapToolDef` shim before returning the value to cago. Same constants reused.
- Retry with backoff: 10 attempts, schedule `[2,4,8,15,15,...]s`, ±20% jitter, honors `Retry-After` header from `*provider.ProviderError`. Lives in `aiagent/retry.go`. Constants `MaxRetries=10`, `MaxRetryDelay=15s` and the schedule slice are copied from `internal/ai/conversation_runner.go`.
- Mid-cycle queued user messages: `App.QueueAIMessage(convID, content, mentions)` Wails binding kept; the body now calls `Stream.Steer(ctx, body, agent.AsUser(), agent.Persist(true))` where `body = RenderMentionContext(mentions) + "\n\n" + content` — same prefixing as today's `app_ai.go:382-388`. The `queue_consumed` event is emitted **before** Steer, carrying the original `content` (not `body`) so the frontend re-renders mention chips correctly.
- Drain pending on Stop: when the user stops the run, residual queued messages are dropped (today's behavior in `conversation_runner.go:run` defer). Same.

**Sub-agent**

- Nesting prevention: preserved by construction — `dispatch_subagent` is registered only on the parent agent's tools, never on any child Entry's tools, so a sub-agent has no way to call `dispatch_subagent` itself. (Today's `isSubAgent(ctx)` check is no longer needed.)
- Independent tool dependency scope: each ops Entry's child agent gets its own `aiagent.Deps` with its own per-System caches (today's "fresh executor per Sub Agent" via `NewExecutor` factory). Achieved by constructing fresh tool struct instances per Entry.
- Result summary truncation at 2048 chars: applied inside the `agent_end` observer that wraps each ops Entry, before emitting the event.
- `agent_role` / `agent_task` propagation: the per-subagent observer captures the Entry's role/task at construction and tags every forwarded sub-event (so a `tool_start` or `approval_request` fired inside the sub-agent's loop carries the right `agent_role` field for the frontend's nested rendering).
- **Behavior change accepted**: `spawn_agent`'s runtime `tools=[...]` subsetting param is gone. Mitigation: define ≥3 ops Entries (`ops-explorer` read-only, `ops-batch` parallel exec, `ops-readonly` strictly read commands) so the LLM picks the right scope by `type`. Frontend stays consistent because `dispatch_subagent` invocations are still surfaced as `agent_start` blocks.

**Approval flow**

- All three `kind` values (`single` / `batch` / `grant`) preserved; `ApprovalItem` and `ApprovalResponse` shapes unchanged.
- Window activation on `approval_request` (today's `a.activateWindow()` in `app_approval.go:53`): replicated in `hook_policy.go` before blocking.
- Browser `Notification` when document is hidden (today's `aiStore.ts:1153`): unchanged — frontend behavior, no backend churn.
- `Decision="allowAll"` branch (single-approval path that calls `SaveGrantPattern` to remember the user's edit as a session-scoped grant): preserved in `hook_policy.go` after the user response.
- Grant editing: `EditedItems` round-trip from frontend → `SubmitGrantMulti` → DB-persisted `GrantSession` / `GrantItem` rows → matched on subsequent calls. All this code (`grant_repo`, `grant_entity`, `policy_group_resolve.go`) is unchanged.
- `request_permission` tool: kept as a regular `tool.Tool`. Its `Call()` runs `CommandPolicyChecker.SubmitGrantMulti` which calls `grantRequestFunc` which emits the `approval_request kind=grant` event and blocks. Same machinery; only the wrapper changes.

**System prompt (per-turn)**

Today `App.SendAIMessage` calls `NewPromptBuilder(lang, aiCtx).Build()` per turn — open tabs / mentions / extension SKILL.md change between turns. cago `coding.New` fixes the system prompt at construction. Solution: split the prompt:

- **Static parts** (role description / language hint / knowledge guidance / error recovery / user denial guidance) live in `coding.AppendSystem(...)` set once at System construction.
- **Per-turn parts** (open tabs / mention context / extension SKILL.md keyed by current open-tab types) injected via a `UserPromptSubmit` hook returning `agent.AddContext(text)`. The hook reads the latest `aiCtx` from a struct on `aiagent.System`; the Wails binding `SendAIMessage` updates the struct before each `Stream(...)` call.
- Language hint is per-conversation but stable within a conversation, so it can sit in static parts as long as conversations don't change language mid-run.

**Audit**

- All audit fields (`Source`, `ConversationID`, `SessionID`, `GrantSessionID`, `Decision`, `MatchedPattern`, `ToolName`, `ArgsJSON`, `Result`, `Error`, `AssetID`, `AssetName`) preserved on `audit_entity.AuditLog` rows.
- Pre→Post `CheckResult` handoff: see §6.
- `writeGrantSubmitAudit` (recorded when grant patterns are saved): keep as-is; called by `SaveGrantPattern`.

**Wails bindings preserved with unchanged signatures**

`CreateConversation`, `ListConversations`, `SwitchConversation`, `LoadConversationMessages`, `UpdateConversationTitle`, `DeleteConversation`, `SaveConversationMessages`, `SendAIMessage`, `QueueAIMessage`, `StopAIGeneration`, `RespondPermission`, `RespondAIApproval`, `RespondOpsctlApproval`, `WaitAIFlushAck`, `DrainAIFlushAck`, `GetCurrentConversationID`. Only the bodies change. Frontend bindings (`frontend/wailsjs/go/app/App.{d.ts,js}`) auto-regenerate; should produce no diff for the binding signatures.

**Session history persistence**

cago's `Session.Stream(ctx, prompt)` accepts a single prompt string per turn and relies on `Session.Store` for history. OpsKat's existing flow has the **frontend** authoritatively persist messages via `SaveConversationMessages` and load them via `LoadConversationMessages`, with backend storage in `conversation_entity.Message` (with `blocks` JSON). Decision:

- Implement a thin `aiagent.gormStore` that satisfies `agent.Store` and reads/writes the existing `conversation_entity.Message` table — no schema change beyond adding `Cwd` to the parent Conversation row.
- Frontend persistence path stays as a **secondary consumer** for now (its `displayMsgs` carry the rich `blocks` array which is needed for UI rendering and is a superset of what cago Session.Store carries).
- A follow-up cleanup (post-migration) can collapse to a single source of truth.

**OpenAI / Anthropic provider features preserved**

- Anthropic prompt-cache `cache_control: ephemeral` on last system block + last 1-2 user blocks (verbatim port to cago `provider/anthropics`).
- Anthropic extended-thinking `Signature` round-trip (§4.1).
- DeepSeek-v4 `reasoning_content` round-trip (§4.2).
- OpenAI `reasoning_effort` (cago `provider.ThinkingConfig` already covers `low/medium/high`; OpsKat's `xhigh`/`max` map down to `high` in the adapter, with a TODO to extend cago if the user reports it).
- `Retry-After` header → `*provider.ProviderError.RetryAfter` (added in §4.1).

### 5.5 Frontend (minimal)

- `ai:event:%d` shape unchanged — event bridge guarantees compatibility.
- New UI: per-conversation `cwd` picker + indicator (recently-used list).
- (Optional, follow-up) Surface `/compact` and `/help` slash commands.

## 6. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `app/coding` defaults to scanning `~/.claude/skills` — would inject the user's Claude Code skills into OpsKat's system prompt | Use `coding.WithSkillDirs(<cwd>/.claude/skills)` to restrict to the cwd chain only |
| Subagent events not bubbling — `agent_start/agent_end` lost | Inject `agent.Observe` on each ops Entry's child agent, forward to Wails |
| Per-conversation `*coding.System` memory cost | LRU bound on `runners`; eager `Close()` on switch + ttl on idle |
| No retry in cago | OpsKat retry wrapper around `Session.Stream`; replays the last user prompt via `Session.FollowUp` after backoff |
| Tool concurrency: today OpsKat runs tool calls in parallel via errgroup; cago has `Tool.Serial()` opt-in | Don't set `Serial()` on OpsKat tools; parallel execution preserved. PreToolUse hook is per-call so concurrency is safe |
| DB schema change (Conversation.Cwd) | Add gormigrate migration; default = `os.UserHomeDir()` for existing rows |
| Local bash gives AI shell access to the user's machine | Same approval pipeline as remote ops: `bash` tool runs through PreToolUse hook; the policy uses the same allow/deny rule machinery (extended to a "local" pseudo-asset) |
| PreToolUse → PostToolUse state propagation. The audit hook needs `CheckResult.DecisionSource / MatchedPattern` set by the policy hook. cago `HookInput` has no per-call sidecar between Pre and Post stages. | Implementation must use one of: (a) `context.Context` value plumbing if cago passes the same ctx to both stages — to be **verified** during Phase 1; (b) an `aiagent.callRegistry` `sync.Map[ToolEvent.ID]*CheckResult` populated in PreToolUse, drained + deleted in PostToolUse. (b) is the safe default. |

## 7. Out of Scope (for this design)

- TUI mode (`agent.TUI()` / `claudecode` backend / `codex` backend) — headless only for now.
- MCP bridge (`cago/mcp`) — not consumed by OpsKat in v1.
- Migrating `internal/ai/fetch_models.go` to live with cago — orthogonal; stays in OpsKat.
- Replacing extension WASM runtime — only the AI-facing `exec_tool` shim moves; the wazero runtime stays.
- **`opsctl` standalone CLI and its approval flow**: unaffected. opsctl uses a **separate** Unix-socket protocol (`internal/approval/` package) and emits its own Wails events (`opsctl:approval`, `opsctl:batch-approval`, `opsctl:grant-approval`, `opsctl:ext-tool-exec`, `data:changed`). These pipelines live in `app_approval.go:startApprovalServer` and are entirely independent of the AI agent loop being replaced here. opsctl consumes `internal/ai/permission.go` + `*_policy.go` + `command_rule.go` + `grant_repo` for its policy / grant matching; all of those are preserved (§5.2). The shared `App.pendingOpsctlApprovals` map and `RespondOpsctlApproval` Wails binding stay. **No opsctl code change for this migration.**

## 8. Testing Strategy

- **Provider unit tests** (in cago PR): use cago's existing `providertest.Mock` to validate the new Anthropic provider parses SSE chunks correctly — text delta / thinking delta / tool-call delta / cache headers / Retry-After / stop reasons.
- **Hook unit tests**: `hook_policy.go` and `hook_audit.go` are pure functions of `HookInput` + a fake `CommandPolicyChecker`. Cover: Allow / Deny / NeedConfirm with approve / NeedConfirm with deny / grant flow / Pre→Post `CheckResult` handoff (test the chosen mechanism from §6).
- **Tool wrapper tests**: `wrapToolDef` should round-trip the existing `ToolDef` JSON Schema; reuse the existing `tool_registry.go` test fixtures.
- **Subagent observer tests**: assert that `agent.Observe` on an ops Entry's child agent re-emits the expected `agent_start` / `agent_end` Wails events with correct sub-tool nesting.
- **Event bridge tests**: feed a scripted cago `Stream` (via `agent.NewStreamForTesting`) and assert byte-equivalent `ai:event:%d` output for every event type in §3.3, including the synthesized aggregate `tool_call` event and ToolCallID propagation.
- **Regression coverage** (manual + automated where possible): existing E2E flows must still pass — send / stop / queue mid-cycle / single-asset approval / batch approval / grant flow / audit log writes / `exec_tool` (extension WASM) / sub-agent dispatch.
- **Feature-flag rollout**: while `OPSKAT_AI_BACKEND=cago` is opt-in (Phase 1), CI runs the Go test suite under both backends; flip in Phase 2 once parity is green for one full release cycle.

## 9. Acceptance Criteria

Every item in §5.4 must round-trip green; specifically:

- Streaming events (`content` / `thinking` / `thinking_done` / `usage` / `agent_start` / `agent_end` / `tool_start` / `tool_result` / `approval_request` / `approval_result` / `queue_consumed` / `stopped` / `retry` / `done` / `error`) emit at byte-equivalent shape to today's backend.
- Approval flows (single / batch / grant) — including the `Decision="allowAll"` branch saving a session grant, edited grant items round-trip, and window activation when document is hidden.
- Mid-cycle `QueueAIMessage` injects mention-prefixed text via `Stream.Steer` and emits `queue_consumed` with the original (unprefixed) content.
- Sub-agent dispatch via `dispatch_subagent` — `agent_start` / `agent_end` blocks render with role/task; nested `tool_start` events appear under the agent block; child events bubble through the per-Entry observer.
- Max-rounds cap stops the loop with the right reason at 50 (main) / 30 (sub) / 100 (absolute hard cap).
- Tool result truncation kicks in at 32KB and emits the existing helpful tail message.
- Retry with backoff: 10 attempts, exponential, ±20% jitter, honors `Retry-After`, `retry` Wails event emitted between attempts.
- Audit rows in `audit_entity.AuditLog` carry the same fields as today (Source / ConversationID / SessionID / GrantSessionID / Decision / MatchedPattern / ToolName / ArgsJSON / Result / Error / AssetID / AssetName).
- Per-conversation `cwd` picker UI; AI reads / writes / runs bash in the chosen directory, gated by the same PreToolUse approval pipeline as remote ops.
- DeepSeek-v4 thinking-mode multi-turn: turn 2 with prior assistant `tool_calls` succeeds (`reasoning_content` round-trip works).
- Anthropic extended-thinking multi-turn: thinking blocks are re-sent with their `Signature` intact and Anthropic does not 400.
- opsctl approval flows (single / batch / grant / ext_tool / notify) work unchanged — same Wails events, same socket protocol.
- All existing OpsKat AI Wails bindings keep their signatures (§5.4 list); frontend bindings regenerate without diff for those signatures.
- `internal/ai/agent.go` + `conversation_runner.go` + `compress.go` + `provider.go` + `anthropic.go` + `openai.go` + `tool_handler_agent.go` are deleted.
- cago PRs landed (or pinned via fork): `provider/anthropics` (§4.1) + `provider.Usage.CacheCreationTokens` + `provider.ProviderError`. Optional: openai DeepSeek `reasoning_content` round-trip (§4.2).

## 10. Phased Rollout

1. **Phase 0 (cago side)** — open the `provider/anthropics` PR. Land it (or pin a fork) before Phase 2.
2. **Phase 1 (OpsKat side, parallelizable)** — build `internal/aiagent/` end-to-end against cago `provider/openai` only. Wire one conversation type behind a feature flag (`OPSKAT_AI_BACKEND=cago`).
3. **Phase 2** — flip default to `cago` when Anthropic provider is ready; delete `internal/ai/agent.go` + friends.
4. **Phase 3** — add cwd picker UI, surface `/compact` `/help`.

## 11. References

- cago repo: `/Users/codfrm/Code/cago/agents`
- cago README: `/Users/codfrm/Code/cago/agents/README.md`
- cago skill: `/Users/codfrm/Code/cago/agents/SKILL.md`
- OpsKat AI: `internal/ai/`, `internal/app/app_ai.go`, `internal/app/app_approval.go`
