# Replace Built-in Agent with Cago — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace OpsKat's in-house AI agent loop in `internal/ai/` with cago's `app/coding` framework, while preserving every current OpsKat AI behavior (events, approvals, audit, sub-agents, retries, mention prompts).

**Architecture:** New `internal/aiagent/` package wraps `coding.System` per Conversation; OpsKat ops tools (SSH/DB/Redis/MongoDB/Kafka/k8s/extensions) are wrapped as cago `tool.Tool`; policy / audit / max-rounds are PreToolUse + PostToolUse hooks; sub-agents use `dispatch_subagent` with multiple custom Entries; per-turn prompt injection via `UserPromptSubmit` hook; events translated through a Wails bridge so frontend stays untouched.

**Tech Stack:** Go 1.25, [cago-frame/agents](https://github.com/cago-frame/agents) (pinned via replace directive during dev), Wails v2, gorm + gormigrate, React 19 (frontend cwd picker).

**Spec:** [`docs/superpowers/specs/2026-05-07-replace-builtin-agent-with-cago-design.md`](../specs/2026-05-07-replace-builtin-agent-with-cago-design.md).

**Working repos:**
- OpsKat: `/Users/codfrm/Code/opskat/opskat` (this repo)
- Cago: `/Users/codfrm/Code/cago/agents` (separate repo for Phase 0)

---

## Phase 0 — Cago upstream additions

These tasks happen in `/Users/codfrm/Code/cago/agents`. Each task ends with a commit on a feature branch (one branch per change → one PR per change). While PRs are in flight, OpsKat consumes the cago fork via a `replace` directive in `go.mod`.

### Task 0.1: Add `provider.ProviderError` type

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/provider/types.go`

**Why:** OpsKat's `internal/ai/provider.go` defines `*ProviderError{Err, RetryAfter, StatusCode}` so the retry layer can honor `Retry-After`. cago has no equivalent; we add one.

- [ ] **Step 1: Write the failing test**

Append to `/Users/codfrm/Code/cago/agents/provider/types_test.go` (create if absent):

```go
package provider

import (
	"errors"
	"testing"
)

func TestProviderError_UnwrapAndRetryAfter(t *testing.T) {
	inner := errors.New("rate limited")
	pe := &ProviderError{Err: inner, RetryAfter: "5", StatusCode: 429}
	if !errors.Is(pe, inner) {
		t.Fatal("ProviderError must wrap inner via errors.Is")
	}
	var got *ProviderError
	if !errors.As(pe, &got) {
		t.Fatal("ProviderError must satisfy errors.As")
	}
	if got.RetryAfter != "5" || got.StatusCode != 429 {
		t.Fatalf("fields lost: %+v", got)
	}
	if pe.Error() != "rate limited" {
		t.Fatalf("Error() = %q, want %q", pe.Error(), "rate limited")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
cd /Users/codfrm/Code/cago/agents
go test ./provider/ -run TestProviderError_UnwrapAndRetryAfter
```

Expected: FAIL — `undefined: ProviderError`.

- [ ] **Step 3: Implement the type**

Append to `/Users/codfrm/Code/cago/agents/provider/types.go`:

```go
// ProviderError wraps API errors with retry metadata. Provider implementations
// should return a *ProviderError on StreamChunk.Err (or as the error from
// ChatCompletion) when the upstream returned a transient HTTP status carrying
// Retry-After, so the caller can honor backoff hints.
type ProviderError struct {
	Err        error
	RetryAfter string // raw Retry-After header value (seconds or HTTP-date)
	StatusCode int
}

func (e *ProviderError) Error() string { return e.Err.Error() }
func (e *ProviderError) Unwrap() error { return e.Err }
```

- [ ] **Step 4: Verify pass**

```
go test ./provider/ -run TestProviderError_UnwrapAndRetryAfter -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
cd /Users/codfrm/Code/cago/agents
git checkout -b feat/provider-error
git add provider/types.go provider/types_test.go
git commit -m "feat(provider): add ProviderError with Retry-After + status code"
```

---

### Task 0.2: Add `provider.Usage.CacheCreationTokens` field

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/provider/types.go` (the `Usage` struct)
- Modify: `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics.go` (stop folding CacheCreationInputTokens into PromptTokens; expose it separately)

**Why:** Anthropic's prompt-cache **write** count (CacheCreationInputTokens) is currently folded into `PromptTokens` (anthropics.go:422), making it impossible to attribute cache-write cost. OpsKat reports this as a separate metric.

- [ ] **Step 1: Write the failing test**

Add to `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics_test.go` a test that constructs a fake `anthropic.MessageDeltaUsage` with `CacheCreationInputTokens=100`, `CacheReadInputTokens=200`, `InputTokens=50` and asserts the resulting `provider.Usage` has `CacheCreationTokens=100`, `CachedTokens=200`, `PromptTokens=50` (NOT 350).

```go
func TestExtractUsage_SplitsCacheCreationFromPrompt(t *testing.T) {
	u := anthropic.MessageDeltaUsage{
		InputTokens:              50,
		CacheCreationInputTokens: 100,
		CacheReadInputTokens:     200,
		OutputTokens:             10,
	}
	got := extractUsageFromDelta(u) // helper to be added in Step 3
	if got.PromptTokens != 50 {
		t.Errorf("PromptTokens = %d, want 50 (raw input only)", got.PromptTokens)
	}
	if got.CacheCreationTokens != 100 {
		t.Errorf("CacheCreationTokens = %d, want 100", got.CacheCreationTokens)
	}
	if got.CachedTokens != 200 {
		t.Errorf("CachedTokens = %d, want 200", got.CachedTokens)
	}
	if got.CompletionTokens != 10 {
		t.Errorf("CompletionTokens = %d, want 10", got.CompletionTokens)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

```
go test ./provider/anthropics/ -run TestExtractUsage_SplitsCacheCreationFromPrompt
```

Expected: FAIL — `extractUsageFromDelta` undefined and `provider.Usage` lacks `CacheCreationTokens`.

- [ ] **Step 3: Add field + helper + adjust mapping**

In `/Users/codfrm/Code/cago/agents/provider/types.go` add the field:

```go
// Usage token 消耗统计
type Usage struct {
	PromptTokens        int
	CompletionTokens    int
	ReasoningTokens     int
	CachedTokens        int // prompt cache read
	CacheCreationTokens int // prompt cache write (Anthropic only today)
	TotalTokens         int
}
```

In `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics.go`, find lines ~421-423 and replace with:

```go
// extractUsageFromDelta maps Anthropic MessageDeltaUsage onto provider.Usage.
// PromptTokens carries raw new input only; cache write/read are exposed separately.
func extractUsageFromDelta(u anthropic.MessageDeltaUsage) provider.Usage {
	return provider.Usage{
		PromptTokens:        int(u.InputTokens),
		CompletionTokens:    int(u.OutputTokens),
		CachedTokens:        int(u.CacheReadInputTokens),
		CacheCreationTokens: int(u.CacheCreationInputTokens),
		TotalTokens:         int(u.InputTokens) + int(u.CacheCreationInputTokens) + int(u.CacheReadInputTokens) + int(u.OutputTokens),
	}
}
```

Update the call site near line 419-426 (the streaming `MessageDelta` case) to call `extractUsageFromDelta` instead of inline-summing into `PromptTokens`.

Apply the same split in the non-streaming `extractUsage` helper around line 408 (whichever helper handles `anthropic.Usage` for `ChatCompletion`).

- [ ] **Step 4: Run all anthropic tests**

```
go test ./provider/anthropics/ -v
```

Expected: PASS (including the new test and any existing usage-shape assertions, which may also need to change — update them too if so).

- [ ] **Step 5: Commit**

```
git checkout -b feat/usage-cache-creation
git add provider/types.go provider/anthropics/
git commit -m "feat(provider): expose CacheCreationTokens separately from PromptTokens"
```

---

### Task 0.3: Auto-inject `cache_control: ephemeral` in anthropics provider

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics.go` (`buildRequest` / message construction)
- Reference: `/Users/codfrm/Code/opskat/opskat/internal/ai/anthropic.go` for the existing OpsKat strategy.

**Why:** OpsKat today injects `cache_control: {"type":"ephemeral"}` on the **last system block** and the **last 1-2 user blocks** to enable Anthropic prompt cache. This must move into cago so behavior is preserved when OpsKat switches providers.

- [ ] **Step 1: Read OpsKat's cache strategy**

Read `internal/ai/anthropic.go` in the OpsKat repo, locate the `cache_control` injection logic. Note how many user blocks are tagged (last 1 vs last 2) and any conditions.

- [ ] **Step 2: Write failing test**

Add `TestBuildRequest_AnnotatesLastSystemAndUserWithCacheControl` to `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics_test.go`. Mock or capture the `MessageNewParams` produced by `buildRequest` for an input with 1 system + 3 user + 2 assistant messages; assert:

- The system block has `CacheControl.Ephemeral` set.
- The last user message in `Messages` has its last text content block tagged `cache_control: ephemeral`.

(Use `param.IsSet` or whatever the Anthropic SDK exposes; copy the assertion idiom from existing anthropics tests.)

- [ ] **Step 3: Run test, expect FAIL**

```
go test ./provider/anthropics/ -run TestBuildRequest_AnnotatesLastSystemAndUserWithCacheControl
```

- [ ] **Step 4: Implement auto-injection in buildRequest**

In `buildRequest`, after the system block and `Messages` slice are assembled, walk back to:
1. The system block — set its `CacheControl` field to ephemeral.
2. The last `user` `Messages` entry's last text block — set `CacheControl` ephemeral.

Match the exact SDK shape used elsewhere in the file. Add a one-line comment: `// Cache breakpoint: last system + last user — preserves behavior ported from OpsKat.`

- [ ] **Step 5: Run test, expect PASS; run full provider test suite**

```
go test ./provider/anthropics/ -v
```

- [ ] **Step 6: Commit**

```
git checkout -b feat/anthropic-cache-control
git add provider/anthropics/
git commit -m "feat(anthropics): auto-tag last system + last user block with cache_control: ephemeral"
```

---

### Task 0.4: Surface Retry-After as `*provider.ProviderError`

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/provider/anthropics/anthropics.go` (error paths in `ChatStream` and `ChatCompletion`)
- Modify: `/Users/codfrm/Code/cago/agents/provider/openai/openai.go` (same)

**Why:** Both providers swallow Retry-After today; OpsKat's retry needs it.

- [ ] **Step 1: Write failing test for anthropic**

Add to `anthropics_test.go`:

```go
func TestChatStream_429ReturnsProviderError(t *testing.T) {
	// Stand up an httptest.Server that responds 429 with Retry-After: 7.
	// Construct a Provider pointed at it; call ChatStream; drain the channel;
	// the last chunk's Err must satisfy errors.As(err, &*provider.ProviderError{})
	// with RetryAfter=="7", StatusCode==429.
}
```

(Spell out the httptest setup fully — copy from any existing test in the package that does HTTP-level mocking. If none exists, create the pattern here.)

- [ ] **Step 2: Run test, expect FAIL**

- [ ] **Step 3: Implement Retry-After plumbing**

The Anthropic SDK exposes HTTP-level errors as `*anthropic.Error`; check its `Response` field for the `Retry-After` header. Wrap the error:

```go
if apiErr := new(anthropic.Error); errors.As(err, &apiErr) {
    var retryAfter string
    if apiErr.Response != nil {
        retryAfter = apiErr.Response.Header.Get("Retry-After")
    }
    return &provider.ProviderError{Err: err, RetryAfter: retryAfter, StatusCode: apiErr.StatusCode}
}
return err
```

Apply at every error-return site in `ChatStream` / `ChatCompletion`.

- [ ] **Step 4: Repeat for openai/openai.go**

Same pattern, using whatever HTTP-error type `sashabaranov/go-openai` returns (`*openai.APIError` or similar). Add a parallel test.

- [ ] **Step 5: Run both provider test suites**

```
go test ./provider/anthropics/ ./provider/openai/ -v
```

- [ ] **Step 6: Commit**

```
git checkout -b feat/provider-retry-after
git add provider/
git commit -m "feat(provider): surface Retry-After as *ProviderError on ChatStream/ChatCompletion errors"
```

---

### Task 0.5 (optional): DeepSeek `reasoning_content` round-trip in openai provider

**Files:**
- Modify: `/Users/codfrm/Code/cago/agents/provider/openai/openai.go`

**Why:** DeepSeek-v4 thinking-mode rejects multi-turn requests where an `assistant` message with `tool_calls` does not include `reasoning_content`. cago openai provider currently strips/ignores it. (See spec §4.2.)

If the upstream PR can't land in time, skip this task and write the same logic as a wrapper provider in OpsKat (`internal/aiagent/provider_deepseek_wrap.go`) — flagged as the fallback in the spec.

- [ ] **Step 1: Write failing test**

Mock OpenAI server that asserts: when sending a message with `Thinking[].Text != ""` and `ToolCalls`, the request body must contain `"reasoning_content": "<text>"` on that assistant message.

- [ ] **Step 2: Run test, expect FAIL**

- [ ] **Step 3: Implement on serialization side**

In the request marshaller, when emitting a message with role=assistant + non-empty Thinking, set `reasoning_content` = `Thinking[0].Text` (concatenate if multiple blocks).

- [ ] **Step 4: Implement on deserialization side**

In `ChatStream`, when a chunk's delta has `reasoning_content`, append it as a `ThinkingDelta`.

- [ ] **Step 5: Run tests, commit**

```
git checkout -b feat/openai-reasoning-content
git add provider/openai/
git commit -m "feat(openai): round-trip reasoning_content for DeepSeek thinking mode"
```

---

### Task 0.6: Open PRs

- [ ] **Step 1: Push each branch and open a PR**

```
cd /Users/codfrm/Code/cago/agents
git push -u origin feat/provider-error
git push -u origin feat/usage-cache-creation
git push -u origin feat/anthropic-cache-control
git push -u origin feat/provider-retry-after
git push -u origin feat/openai-reasoning-content   # only if Task 0.5 done
```

Open one PR per branch on GitHub via `gh pr create` (cago uses standard GitHub flow). Title format: `feat(<scope>): <summary>`.

- [ ] **Step 2: Pin OpsKat's go.mod to the local cago checkout**

In `/Users/codfrm/Code/opskat/opskat/go.mod`, append a `replace` directive (do NOT commit yet — this is dev-only):

```
replace github.com/cago-frame/agents => /Users/codfrm/Code/cago/agents
```

Run `go mod tidy` in OpsKat. Confirm the dev build picks up the local cago.

(This `replace` will be removed and replaced with a normal version pin in Phase 2 once the upstream PRs merge.)

---

## Phase 1 — OpsKat `internal/aiagent/` package

All work below is in `/Users/codfrm/Code/opskat/opskat`. Each task ends with a Go test passing and a commit.

### Task 1.1: Scaffold `internal/aiagent/` package + feature flag

**Files:**
- Create: `internal/aiagent/doc.go`
- Create: `internal/aiagent/feature_flag.go`
- Create: `internal/aiagent/feature_flag_test.go`

- [ ] **Step 1: Write failing test**

`internal/aiagent/feature_flag_test.go`:

```go
package aiagent

import (
	"os"
	"testing"
)

func TestEnabled_DefaultFalse(t *testing.T) {
	t.Setenv("OPSKAT_AI_BACKEND", "")
	if Enabled() {
		t.Fatal("default should be false during Phase 1")
	}
}

func TestEnabled_TrueWhenEnvCago(t *testing.T) {
	t.Setenv("OPSKAT_AI_BACKEND", "cago")
	if !Enabled() {
		t.Fatal("OPSKAT_AI_BACKEND=cago should enable")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```
go test ./internal/aiagent/ -run TestEnabled
```

- [ ] **Step 3: Implement**

`internal/aiagent/doc.go`:

```go
// Package aiagent integrates OpsKat with cago's app/coding framework.
// It replaces the legacy in-house agent loop in internal/ai while preserving
// every existing AI behavior (events, approvals, audit, sub-agents, retries).
// See docs/superpowers/specs/2026-05-07-replace-builtin-agent-with-cago-design.md.
package aiagent
```

`internal/aiagent/feature_flag.go`:

```go
package aiagent

import "os"

// Enabled reports whether the cago-backed agent should be used.
// Phase 1 default: opt-in via OPSKAT_AI_BACKEND=cago.
// Phase 2 default: enabled unconditionally; flag becomes a no-op.
func Enabled() bool {
	return os.Getenv("OPSKAT_AI_BACKEND") == "cago"
}
```

- [ ] **Step 4: Verify pass**

- [ ] **Step 5: Commit**

```
git checkout -b feat/aiagent-scaffold
git add internal/aiagent/
git commit -m "✨ scaffold internal/aiagent package + feature flag"
```

---

### Task 1.2: Define `aiagent.Deps` (per-System dependency container)

**Files:**
- Create: `internal/aiagent/deps.go`

- [ ] **Step 1: Implement directly (no test — pure struct)**

`internal/aiagent/deps.go`:

```go
package aiagent

import (
	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/service/kafka_svc"
	"github.com/opskat/opskat/internal/sshpool"
)

// Deps is the per-Conversation (per-coding.System) dependency bag.
// Tools, hooks, and observers borrow from it.
//
// Lifetime invariants:
//   - sshPool is App-shared (one process-wide instance) — provided by the App.
//   - sshCache, mongoCache, kafkaSvc are per-Conversation; constructed per Deps,
//     closed via Deps.Close() when the System closes.
type Deps struct {
	// App-shared (do NOT close from Deps)
	SSHPool *sshpool.Pool

	// Per-Conversation (closed by Deps.Close)
	SSHCache     *ai.SSHClientCache
	MongoCache   *ai.MongoDBClientCache
	KafkaService *kafka_svc.Service

	// Reused references
	PolicyChecker *ai.CommandPolicyChecker
}

// NewDeps constructs a fresh per-Conversation Deps bag.
// pool MUST be the App-shared SSH pool; checker MUST be the App-shared checker.
func NewDeps(pool *sshpool.Pool, checker *ai.CommandPolicyChecker) *Deps {
	return &Deps{
		SSHPool:       pool,
		SSHCache:      ai.NewSSHClientCache(),
		MongoCache:    ai.NewMongoDBClientCache(),
		KafkaService:  kafka_svc.New(pool),
		PolicyChecker: checker,
	}
}

// Close releases per-Conversation resources. Idempotent.
func (d *Deps) Close() error {
	if d.KafkaService != nil {
		d.KafkaService.Close()
		d.KafkaService = nil
	}
	if d.MongoCache != nil {
		_ = d.MongoCache.Close()
		d.MongoCache = nil
	}
	if d.SSHCache != nil {
		_ = d.SSHCache.Close()
		d.SSHCache = nil
	}
	return nil
}
```

- [ ] **Step 2: Verify build**

```
go build ./internal/aiagent/
```

- [ ] **Step 3: Commit**

```
git add internal/aiagent/deps.go
git commit -m "✨ add aiagent.Deps per-Conversation dependency container"
```

---

### Task 1.3: `wrapToolDef` — turn `ai.ToolDef` into `cago tool.Tool`

**Files:**
- Create: `internal/aiagent/tools_ops.go`
- Create: `internal/aiagent/tools_ops_test.go`

- [ ] **Step 1: Write failing test**

`internal/aiagent/tools_ops_test.go`:

```go
package aiagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/opskat/opskat/internal/ai"
)

func TestWrapToolDef_PassesArgsAndReturnsString(t *testing.T) {
	def := ai.ToolDef{
		Name:        "echo_test",
		Description: "echo for tests",
		Params: []ai.ParamDef{
			{Name: "msg", Type: ai.ParamString, Description: "msg", Required: true},
		},
		Handler: func(_ context.Context, args map[string]any) (string, error) {
			return "got:" + args["msg"].(string), nil
		},
	}
	tool := wrapToolDef(def, &Deps{})
	if tool.Name() != "echo_test" {
		t.Fatalf("Name=%q", tool.Name())
	}
	got, err := tool.Call(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if s, ok := got.(string); !ok || s != "got:hi" {
		t.Fatalf("Call returned %v (%T)", got, got)
	}
}

func TestWrapToolDef_TruncatesLongResult(t *testing.T) {
	big := strings.Repeat("X", 64*1024) // 64KB
	def := ai.ToolDef{
		Name: "big_test", Description: "big",
		Handler: func(_ context.Context, _ map[string]any) (string, error) { return big, nil },
	}
	tool := wrapToolDef(def, &Deps{})
	got, _ := tool.Call(context.Background(), json.RawMessage(`{}`))
	s := got.(string)
	if !strings.Contains(s, "Output truncated") {
		t.Fatal("missing truncation marker")
	}
	if !strings.Contains(s, "exceeds 32768 byte limit") {
		t.Fatalf("missing limit hint, got tail %q", s[len(s)-200:])
	}
}

func TestWrapToolDef_BuildsJSONSchemaFromParamDefs(t *testing.T) {
	def := ai.ToolDef{
		Name: "x", Description: "x",
		Params: []ai.ParamDef{
			{Name: "a", Type: ai.ParamString, Description: "a", Required: true},
			{Name: "b", Type: ai.ParamNumber, Description: "b"},
		},
		Handler: func(context.Context, map[string]any) (string, error) { return "", nil },
	}
	tool := wrapToolDef(def, &Deps{})
	var schema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" {
		t.Fatalf("type=%s", schema.Type)
	}
	if schema.Properties["a"].Type != "string" {
		t.Fatalf("a.type=%s", schema.Properties["a"].Type)
	}
	if schema.Properties["b"].Type != "number" {
		t.Fatalf("b.type=%s", schema.Properties["b"].Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "a" {
		t.Fatalf("required=%v", schema.Required)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```
go test ./internal/aiagent/ -run TestWrapToolDef
```

- [ ] **Step 3: Implement**

`internal/aiagent/tools_ops.go`:

```go
package aiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cago-frame/agents/tool"

	"github.com/opskat/opskat/internal/ai"
)

const maxResultLen = 32 * 1024

// wrapToolDef adapts an OpsKat ai.ToolDef to cago's tool.Tool interface,
// preserving the existing handler signature, JSON schema, ctx-injected
// dependencies, and the 32KB result truncation with the helpful tail message.
func wrapToolDef(def ai.ToolDef, deps *Deps) tool.Tool {
	schema := buildSchema(def.Params)
	return &tool.RawTool{
		NameStr:   def.Name,
		DescStr:   def.Description,
		SchemaRaw: schema,
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var args map[string]any
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, fmt.Errorf("invalid args: %w", err)
				}
			} else {
				args = map[string]any{}
			}
			ctx = injectDeps(ctx, deps)
			result, err := def.Handler(ctx, args)
			if err != nil {
				return fmt.Sprintf("Tool execution error: %s", err.Error()), nil
			}
			return truncate(result), nil
		},
	}
}

func buildSchema(params []ai.ParamDef) json.RawMessage {
	properties := map[string]any{}
	var required []string
	for _, p := range params {
		properties[p.Name] = map[string]any{
			"type":        string(p.Type),
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	out := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	b, _ := json.Marshal(out)
	return b
}

func truncate(s string) string {
	if len(s) <= maxResultLen {
		return s
	}
	return s[:2048] + fmt.Sprintf(
		"\n\n--- Output truncated ---\nOutput too large (%d bytes, exceeds %d byte limit). Use more precise filters, pipe through | head or | grep, or split the query.",
		len(s), maxResultLen)
}

// injectDeps re-creates the context-keyed dependencies the legacy
// DefaultToolExecutor used to set: SSH cache, SSH pool, MongoDB cache, Kafka.
// Each Deps is per-Conversation, so calls inside one Conversation share caches.
func injectDeps(ctx context.Context, deps *Deps) context.Context {
	if deps == nil {
		return ctx
	}
	if deps.SSHCache != nil {
		ctx = ai.WithSSHCache(ctx, deps.SSHCache)
	}
	if deps.SSHPool != nil {
		ctx = ai.WithSSHPool(ctx, deps.SSHPool)
	}
	if deps.MongoCache != nil {
		ctx = ai.WithMongoDBCache(ctx, deps.MongoCache)
	}
	if deps.KafkaService != nil {
		ctx = ai.WithKafkaService(ctx, deps.KafkaService)
	}
	if deps.PolicyChecker != nil {
		ctx = ai.WithPolicyChecker(ctx, deps.PolicyChecker)
	}
	return ctx
}
```

- [ ] **Step 4: Verify pass**

```
go test ./internal/aiagent/ -run TestWrapToolDef -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/tools_ops.go internal/aiagent/tools_ops_test.go
git commit -m "✨ wrapToolDef adapter (ai.ToolDef → cago tool.Tool) with 32KB truncation"
```

---

### Task 1.4: `OpsTools()` — wrap all OpsKat tool defs

**Files:**
- Modify: `internal/aiagent/tools_ops.go`
- Modify: `internal/aiagent/tools_ops_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/aiagent/tools_ops_test.go`:

```go
func TestOpsTools_HasAllExpectedNames(t *testing.T) {
	tools := OpsTools(&Deps{})
	want := map[string]bool{
		"list_assets": false, "get_asset": false, "run_command": false,
		"exec_k8s": false, "add_asset": false, "update_asset": false,
		"list_groups": false, "get_group": false, "add_group": false, "update_group": false,
		"upload_file": false, "download_file": false,
		"exec_sql": false, "exec_redis": false, "exec_mongo": false,
		"kafka_cluster": false, "kafka_topic": false, "kafka_consumer_group": false,
		"kafka_acl": false, "kafka_schema": false, "kafka_connect": false, "kafka_message": false,
		"request_permission": false, "batch_command": false, "exec_tool": false,
	}
	for _, tt := range tools {
		if _, ok := want[tt.Name()]; ok {
			want[tt.Name()] = true
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("missing tool: %s", n)
		}
	}
}

func TestOpsTools_ExcludesSpawnAgent(t *testing.T) {
	tools := OpsTools(&Deps{})
	for _, tt := range tools {
		if tt.Name() == "spawn_agent" {
			t.Fatal("spawn_agent must be excluded — replaced by dispatch_subagent")
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

Append to `internal/aiagent/tools_ops.go`:

```go
// OpsTools returns the full set of OpsKat ops tools as cago tool.Tool values,
// bound to the given Deps. The legacy spawn_agent is excluded — sub-agent
// dispatch is now handled by cago's dispatch_subagent + custom Entries.
func OpsTools(deps *Deps) []tool.Tool {
	defs := ai.AllToolDefs()
	out := make([]tool.Tool, 0, len(defs))
	for _, d := range defs {
		if d.Name == "spawn_agent" {
			continue
		}
		out = append(out, wrapToolDef(d, deps))
	}
	return out
}
```

- [ ] **Step 4: Verify pass**

```
go test ./internal/aiagent/ -run TestOpsTools -v
```

- [ ] **Step 5: Delete `spawn_agent` from `ai.AllToolDefs`**

In `/Users/codfrm/Code/opskat/opskat/internal/ai/tool_registry.go`, remove the `spawn_agent` entry from `AllToolDefs()`. Tests may break — fix them in this commit.

Then `git mv internal/ai/tool_handler_agent.go internal/ai/tool_handler_agent.go.bak` (we'll fully delete in Phase 2 once nothing references it). Or simpler: delete it now and remove imports — `ai.AllToolDefs()` no longer references its handler.

Actually: just delete `tool_handler_agent.go` and remove the entry. The legacy `Agent.Chat` path doesn't need `spawn_agent` either since it's the new architecture's responsibility.

- [ ] **Step 6: Commit**

```
git add internal/aiagent/ internal/ai/
git commit -m "✨ OpsTools(): wrap all ops ToolDefs; remove spawn_agent (replaced by dispatch_subagent)"
```

---

### Task 1.5: Audit context sidecar (Pre→Post `CheckResult` handoff)

**Files:**
- Create: `internal/aiagent/sidecar.go`
- Create: `internal/aiagent/sidecar_test.go`

**Why:** cago `HookInput` has no per-call sidecar field between Pre and Post. We thread `*ai.CheckResult` via a process-wide `sync.Map` keyed by `ToolEvent.ID` — populated in PreToolUse, drained + deleted in PostToolUse.

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"testing"

	"github.com/opskat/opskat/internal/ai"
)

func TestSidecar_PutDrain(t *testing.T) {
	s := newSidecar()
	r := &ai.CheckResult{Decision: ai.Allow, MatchedPattern: "ls *"}
	s.put("tool_id_1", r)
	got := s.drain("tool_id_1")
	if got == nil || got.MatchedPattern != "ls *" {
		t.Fatalf("drained = %+v", got)
	}
	// second drain returns nil
	if s.drain("tool_id_1") != nil {
		t.Fatal("drain must remove the entry")
	}
}

func TestSidecar_DrainMissingIsNil(t *testing.T) {
	s := newSidecar()
	if s.drain("missing") != nil {
		t.Fatal("missing key should drain to nil")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"sync"

	"github.com/opskat/opskat/internal/ai"
)

// sidecar threads *ai.CheckResult from PreToolUse to PostToolUse keyed by
// cago's ToolEvent.ID. cago HookInput has no per-call sidecar field; this
// fills the gap. One sidecar instance per *coding.System.
type sidecar struct {
	m sync.Map // map[string]*ai.CheckResult
}

func newSidecar() *sidecar { return &sidecar{} }

func (s *sidecar) put(toolID string, r *ai.CheckResult) {
	if toolID == "" || r == nil {
		return
	}
	s.m.Store(toolID, r)
}

func (s *sidecar) drain(toolID string) *ai.CheckResult {
	if toolID == "" {
		return nil
	}
	v, ok := s.m.LoadAndDelete(toolID)
	if !ok {
		return nil
	}
	return v.(*ai.CheckResult)
}
```

- [ ] **Step 4: Verify pass**

- [ ] **Step 5: Commit**

```
git add internal/aiagent/sidecar.go internal/aiagent/sidecar_test.go
git commit -m "✨ aiagent.sidecar: thread CheckResult Pre→PostToolUse keyed by ToolEvent.ID"
```

---

### Task 1.6: PolicyApprovalEmitter interface (avoid Wails dep in package)

**Files:**
- Create: `internal/aiagent/emitter.go`

**Why:** The hooks emit `approval_request` / `approval_result` / `agent_start` / `retry` / `queue_consumed` Wails events. We don't want `internal/aiagent` to import `wails/v2/pkg/runtime` directly — keep that in `internal/app`. Pass an interface.

- [ ] **Step 1: Implement directly (interface only)**

```go
package aiagent

import "github.com/opskat/opskat/internal/ai"

// EventEmitter is the abstraction over Wails event delivery used by the hooks
// and the bridge. internal/app implements it by calling wailsRuntime.EventsEmit.
// Tests substitute a recording fake.
type EventEmitter interface {
	Emit(convID int64, event ai.StreamEvent)
}

// EmitterFunc adapts a function to the EventEmitter interface.
type EmitterFunc func(convID int64, event ai.StreamEvent)

func (f EmitterFunc) Emit(convID int64, event ai.StreamEvent) { f(convID, event) }
```

- [ ] **Step 2: Build & commit**

```
go build ./internal/aiagent/
git add internal/aiagent/emitter.go
git commit -m "✨ aiagent.EventEmitter interface (Wails-free abstraction)"
```

---

### Task 1.7: PolicyApproval gateway (extract from `app_approval.go`)

**Files:**
- Create: `internal/aiagent/approval_gateway.go`
- Create: `internal/aiagent/approval_gateway_test.go`

**Why:** `makeCommandConfirmFunc` and `makeGrantRequestFunc` from `internal/app/app_approval.go` blend Wails-event emission with channel-based wait. We refactor them so the channel-wait logic lives in `aiagent` and the Wails emission goes through the `EventEmitter` interface. The existing `App.pendingAIApprovals` map stays in `internal/app`, accessed via callbacks.

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/ai"
)

func TestApprovalGateway_RequestEmitsAndBlocks(t *testing.T) {
	emitted := make(chan ai.StreamEvent, 1)
	em := EmitterFunc(func(_ int64, ev ai.StreamEvent) { emitted <- ev })
	resolve := make(chan ai.ApprovalResponse, 1)
	gw := NewApprovalGateway(em, func(confirmID string) (chan ai.ApprovalResponse, func()) {
		return resolve, func() {}
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		resolve <- ai.ApprovalResponse{Decision: "allow"}
	}()

	resp := gw.RequestSingle(context.Background(), 7, "exec",
		[]ai.ApprovalItem{{Type: "exec", AssetID: 1, Command: "ls"}}, "")

	if resp.Decision != "allow" {
		t.Fatalf("decision = %s", resp.Decision)
	}
	ev := <-emitted
	if ev.Type != "approval_request" {
		t.Fatalf("first event = %s", ev.Type)
	}
	ev2 := <-emitted
	if ev2.Type != "approval_result" || ev2.Content != "allow" {
		t.Fatalf("second event = %+v", ev2)
	}
}

func TestApprovalGateway_ContextCancelDenies(t *testing.T) {
	em := EmitterFunc(func(int64, ai.StreamEvent) {})
	gw := NewApprovalGateway(em, func(string) (chan ai.ApprovalResponse, func()) {
		return make(chan ai.ApprovalResponse), func() {}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp := gw.RequestSingle(ctx, 1, "exec", nil, "")
	if resp.Decision != "deny" {
		t.Fatalf("expected deny on canceled ctx, got %s", resp.Decision)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/ai"
)

// PendingResolver hands out a one-shot response channel for a confirmID and a
// teardown func that the caller defers. Implemented by internal/app's
// pendingAIApprovals sync.Map.
type PendingResolver func(confirmID string) (chan ai.ApprovalResponse, func())

// ApprovalGateway bridges the cago hook layer (which calls RequestSingle/Grant)
// to the Wails event layer (EventEmitter) and the response channel layer
// (PendingResolver). It encapsulates window activation, event emission, and
// cancellation handling.
type ApprovalGateway struct {
	emit       EventEmitter
	resolver   PendingResolver
	activate   func() // window activation hook; nil = no-op
}

// NewApprovalGateway constructs a gateway. activate may be nil when no window
// activation is desired (tests / headless).
func NewApprovalGateway(em EventEmitter, resolver PendingResolver) *ApprovalGateway {
	return &ApprovalGateway{emit: em, resolver: resolver}
}

// SetActivateFunc registers an optional window-activation callback called
// immediately before emitting an approval_request event.
func (g *ApprovalGateway) SetActivateFunc(fn func()) { g.activate = fn }

// RequestSingle emits an approval_request and blocks until the user responds
// or ctx cancels. kind ∈ {"single","batch"}. agentRole is "" for top-level.
func (g *ApprovalGateway) RequestSingle(ctx context.Context, convID int64, kind string,
	items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse {

	confirmID := fmt.Sprintf("ai_%d_%d", convID, time.Now().UnixNano())
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type: "approval_request", Kind: kind, Items: items,
		ConfirmID: confirmID, AgentRole: agentRole,
	})

	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: resp.Decision,
		})
		return resp
	case <-ctx.Done():
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: "deny",
		})
		return ai.ApprovalResponse{Decision: "deny"}
	}
}

// RequestGrant emits an approval_request kind=grant and blocks for response.
// Returns (approved, finalPatterns) — finalPatterns is the user's edited list
// (may differ from input items.Command).
func (g *ApprovalGateway) RequestGrant(ctx context.Context, convID int64,
	items []ai.ApprovalItem, reason string) (bool, []ai.ApprovalItem) {

	sessionID := fmt.Sprintf("conv_%d", convID)
	confirmID := fmt.Sprintf("grant_%d_%d", convID, time.Now().UnixNano())
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(convID, ai.StreamEvent{
		Type: "approval_request", Kind: "grant", Items: items,
		ConfirmID: confirmID, Description: reason, SessionID: sessionID,
	})

	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: resp.Decision,
		})
		if resp.Decision == "deny" {
			return false, nil
		}
		final := resp.EditedItems
		if len(final) == 0 {
			final = items
		}
		return true, final
	case <-ctx.Done():
		g.emit.Emit(convID, ai.StreamEvent{
			Type: "approval_result", ConfirmID: confirmID, Content: "deny",
		})
		return false, nil
	}
}
```

- [ ] **Step 4: Verify pass**

```
go test ./internal/aiagent/ -run TestApprovalGateway -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/approval_gateway.go internal/aiagent/approval_gateway_test.go
git commit -m "✨ aiagent.ApprovalGateway: emit approval_request, block, emit approval_result"
```

---

### Task 1.8: PreToolUse hook — policy check

**Files:**
- Create: `internal/aiagent/hook_policy.go`
- Create: `internal/aiagent/hook_policy_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

func TestPolicyHook_AllowResultPlantedInSidecar(t *testing.T) {
	sc := newSidecar()
	deps := &Deps{}
	gw := newFakeGateway()
	hook := makePolicyHook(deps, sc, gw, fakeChecker(ai.CheckResult{Decision: ai.Allow, DecisionSource: ai.SourcePolicyAllow}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:     agent.StagePreToolUse,
		ToolName:  "run_command",
		ToolInput: json.RawMessage(`{"asset_id":1,"command":"ls /tmp"}`),
		Raw:       json.RawMessage(`{"id":"call_1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatal("Allow path must not Deny")
	}
	if r := sc.drain("call_1"); r == nil || r.Decision != ai.Allow {
		t.Fatalf("sidecar lost CheckResult: %+v", r)
	}
}

func TestPolicyHook_DenyShortCircuits(t *testing.T) {
	sc := newSidecar()
	gw := newFakeGateway()
	hook := makePolicyHook(&Deps{}, sc, gw, fakeChecker(ai.CheckResult{Decision: ai.Deny, Message: "nope"}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage: agent.StagePreToolUse, ToolName: "run_command",
		ToolInput: json.RawMessage(`{"asset_id":1,"command":"rm -rf /"}`),
		Raw:       json.RawMessage(`{"id":"call_x"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Decision != agent.DecisionDeny {
		t.Fatalf("expected Deny, got %+v", out)
	}
	if out.Reason != "nope" {
		t.Fatalf("reason = %q", out.Reason)
	}
}

// helpers
type fakeGateway struct{ called bool; resp ai.ApprovalResponse }
func newFakeGateway() *fakeGateway { return &fakeGateway{resp: ai.ApprovalResponse{Decision: "allow"}} }
// implement enough of ApprovalGateway shape for the hook — see makePolicyHook signature
```

(Adjust the helper to match the gateway interface introduced in Step 3.)

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PolicyChecker is the minimal interface the hook needs from
// *ai.CommandPolicyChecker. Allows tests to inject fakes.
type PolicyChecker interface {
	Check(ctx context.Context, assetID int64, command string) ai.CheckResult
}

// approvalRequester is the slice of ApprovalGateway the policy hook calls.
type approvalRequester interface {
	RequestSingle(ctx context.Context, convID int64, kind string,
		items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse
}

// makePolicyHook constructs the PreToolUse hook. It:
//   1. extracts asset_id + command from ToolInput by tool name,
//   2. calls checker.Check,
//   3. on Allow: plants CheckResult in sidecar, returns nil (no-op),
//   4. on Deny: plants result, returns agent.Deny(reason),
//   5. on NeedConfirm: emits approval_request via gw, blocks on response,
//      converts response to Allow/Deny.
func makePolicyHook(deps *Deps, sc *sidecar, gw approvalRequester, checker PolicyChecker) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		toolID := extractToolID(in.Raw) // populated by event_bridge
		convID := getConvID(ctx)
		agentRole := getAgentRole(ctx) // empty for top-level

		assetID, command, kind, ok := extractAssetAndCommand(in.ToolName, in.ToolInput)
		if !ok {
			// tool not policy-gated → allow without check
			return nil, nil
		}

		result := checker.Check(ctx, assetID, command)
		sc.put(toolID, &result)

		switch result.Decision {
		case ai.Allow:
			return nil, nil
		case ai.Deny:
			return agent.Deny(result.Message), nil
		}

		// NeedConfirm
		items := []ai.ApprovalItem{{
			Type: kind, AssetID: assetID, Command: command,
		}}
		resp := gw.RequestSingle(ctx, convID, kind, items, agentRole)
		switch resp.Decision {
		case "allow", "allowAll":
			result.Decision = ai.Allow
			result.DecisionSource = ai.SourceUserAllow
			sc.put(toolID, &result)
			if resp.Decision == "allowAll" {
				saveGrantPatternFromResponse(ctx, convID, assetID, items[0], resp)
			}
			return nil, nil
		default:
			result.Decision = ai.Deny
			result.DecisionSource = ai.SourceUserDeny
			sc.put(toolID, &result)
			return agent.Deny("user denied"), nil
		}
	}
}

// extractAssetAndCommand returns (assetID, commandSummary, kind, isPolicyGated).
// kind is the ApprovalItem.Type ("exec","sql","redis","mongo","kafka","k8s","cp").
func extractAssetAndCommand(toolName string, raw json.RawMessage) (int64, string, string, bool) {
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	get := func(k string) string { v, _ := args[k].(string); return v }
	getNum := func(k string) int64 {
		switch v := args[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
		return 0
	}

	switch toolName {
	case "run_command":
		return getNum("asset_id"), get("command"), "exec", true
	case "exec_sql":
		return getNum("asset_id"), get("sql"), "sql", true
	case "exec_redis":
		return getNum("asset_id"), get("command"), "redis", true
	case "exec_mongo":
		return getNum("asset_id"), get("operation"), "mongo", true
	case "exec_k8s":
		return getNum("asset_id"), get("command"), "k8s", true
	case "upload_file":
		return getNum("asset_id"), "upload " + get("local_path") + " → " + get("remote_path"), "cp", true
	case "download_file":
		return getNum("asset_id"), "download " + get("remote_path") + " → " + get("local_path"), "cp", true
	case "kafka_cluster", "kafka_topic", "kafka_consumer_group", "kafka_acl",
		"kafka_schema", "kafka_connect", "kafka_message":
		return getNum("asset_id"), get("operation") + ":" + get("topic"), "kafka", true
	}
	return 0, "", "", false
}

// extractToolID parses the ToolEvent.ID we propagate via HookInput.Raw.
func extractToolID(raw json.RawMessage) string {
	var probe struct{ ID string `json:"id"` }
	_ = json.Unmarshal(raw, &probe)
	return probe.ID
}

// Local placeholders — Task 1.13 deletes these and the package-level
// versions in context.go take over. Keeping them here lets this task's
// tests pass standalone.
func getConvID(ctx context.Context) int64     { return 0 }
func getAgentRole(ctx context.Context) string { return "" }

func saveGrantPatternFromResponse(ctx context.Context, convID, assetID int64,
	item ai.ApprovalItem, resp ai.ApprovalResponse) {
	pattern := item.Command
	if len(resp.EditedItems) > 0 {
		pattern = resp.EditedItems[0].Command
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return
	}
	// Reuse existing grant-pattern persistence from internal/ai.
	// session ID format must match opsctl's so legacy grant matching keeps working.
	_ = ai.SaveGrantPattern // existing function; called from outer App layer until conv-context is wired
}
```

(NOTE: `getConvID` / `getAgentRole` / `saveGrantPatternFromResponse` are stubbed here; Task 1.10 wires them.)

- [ ] **Step 4: Run, expect PASS for the two cases tested**

```
go test ./internal/aiagent/ -run TestPolicyHook -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/hook_policy.go internal/aiagent/hook_policy_test.go
git commit -m "✨ PreToolUse policy hook: gate ops tools via CommandPolicyChecker + ApprovalGateway"
```

---

### Task 1.9: PostToolUse hook — audit writer

**Files:**
- Create: `internal/aiagent/hook_audit.go`
- Create: `internal/aiagent/hook_audit_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

type fakeAuditWriter struct{ got ai.ToolCallInfo }

func (f *fakeAuditWriter) WriteToolCall(_ context.Context, info ai.ToolCallInfo) { f.got = info }

func TestAuditHook_DrainsSidecarAndWritesAudit(t *testing.T) {
	sc := newSidecar()
	sc.put("call_42", &ai.CheckResult{Decision: ai.Allow, MatchedPattern: "ls *", DecisionSource: ai.SourcePolicyAllow})
	w := &fakeAuditWriter{}
	hook := makeAuditHook(sc, w)

	_, err := hook(context.Background(), agent.HookInput{
		Stage:        agent.StagePostToolUse,
		ToolName:     "run_command",
		ToolInput:    json.RawMessage(`{"asset_id":1,"command":"ls /"}`),
		ToolResponse: json.RawMessage(`"output here"`),
		Raw:          json.RawMessage(`{"id":"call_42"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.got.ToolName != "run_command" {
		t.Errorf("ToolName = %q", w.got.ToolName)
	}
	if w.got.Decision == nil || w.got.Decision.Decision != ai.Allow {
		t.Errorf("decision lost: %+v", w.got.Decision)
	}
	if w.got.Decision.MatchedPattern != "ls *" {
		t.Errorf("pattern lost: %+v", w.got.Decision)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// makeAuditHook returns a PostToolUse hook that reads the CheckResult planted
// by the policy hook and writes a fire-and-forget audit row.
func makeAuditHook(sc *sidecar, writer ai.AuditWriter) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		toolID := extractToolID(in.Raw)
		decision := sc.drain(toolID)

		var result string
		_ = json.Unmarshal(in.ToolResponse, &result)

		go writer.WriteToolCall(ctx, ai.ToolCallInfo{
			ToolName: in.ToolName,
			ArgsJSON: string(in.ToolInput),
			Result:   result,
			Decision: decision,
		})
		return nil, nil
	}
}
```

- [ ] **Step 4: Verify pass**

```
go test ./internal/aiagent/ -run TestAuditHook -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/hook_audit.go internal/aiagent/hook_audit_test.go
git commit -m "✨ PostToolUse audit hook: drain sidecar + DefaultAuditWriter"
```

---

### Task 1.10: Round-cap PreToolUse hook

**Files:**
- Create: `internal/aiagent/hook_rounds.go`
- Create: `internal/aiagent/hook_rounds_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
)

func TestRoundsHook_StopsAtCap(t *testing.T) {
	hook := makeRoundsHook(2) // cap=2

	for i := 0; i < 2; i++ {
		out, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
		if err != nil {
			t.Fatal(err)
		}
		if out != nil && out.Continue != nil && !*out.Continue {
			t.Fatalf("turn %d should not stop", i)
		}
	}

	out, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Continue == nil || *out.Continue {
		t.Fatalf("expected StopRun on 3rd call, got %+v", out)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/cago-frame/agents/agent"
)

// makeRoundsHook returns a PreToolUse hook that emits StopRun after `cap`
// invocations on the same hook instance. Each Stream gets its own hook
// instance via System.Stream so counts don't leak across runs.
func makeRoundsHook(cap int) agent.HookFunc {
	if cap <= 0 {
		cap = 50
	}
	var n int64
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		current := atomic.AddInt64(&n, 1)
		if current > int64(cap) {
			return agent.StopRun(fmt.Sprintf("max rounds (%d) reached", cap)), nil
		}
		return nil, nil
	}
}
```

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestRoundsHook -v
git add internal/aiagent/hook_rounds.go internal/aiagent/hook_rounds_test.go
git commit -m "✨ rounds hook: enforce 50/30/100 round cap via PreToolUse StopRun"
```

---

### Task 1.11: UserPromptSubmit hook — per-turn AdditionalContext

**Files:**
- Create: `internal/aiagent/hook_prompt.go`
- Create: `internal/aiagent/hook_prompt_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"strings"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

func TestPromptHook_InjectsTabsAndMentions(t *testing.T) {
	state := &PerTurnState{}
	state.Set(ai.AIContext{
		OpenTabs:        []ai.TabInfo{{Type: "ssh", AssetID: 9, AssetName: "edge-1"}},
		MentionedAssets: []ai.MentionedAsset{{AssetID: 9, Name: "edge-1", Type: "ssh", Host: "10.0.0.1"}},
	}, nil)

	hook := makePromptHook(state)
	out, err := hook(context.Background(), agent.HookInput{Stage: agent.StageUserPromptSubmit})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.AdditionalContext == "" {
		t.Fatal("expected AdditionalContext")
	}
	if !strings.Contains(out.AdditionalContext, "edge-1") {
		t.Errorf("missing tab name in: %s", out.AdditionalContext)
	}
	if !strings.Contains(out.AdditionalContext, "@edge-1") {
		t.Errorf("missing mention in: %s", out.AdditionalContext)
	}
}

func TestPromptHook_NoStateNoOp(t *testing.T) {
	state := &PerTurnState{}
	hook := makePromptHook(state)
	out, _ := hook(context.Background(), agent.HookInput{Stage: agent.StageUserPromptSubmit})
	if out != nil && out.AdditionalContext != "" {
		t.Fatalf("expected empty additionalContext, got %q", out.AdditionalContext)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"strings"
	"sync"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PerTurnState carries the latest aiCtx + extension SKILL.md table for the
// next stream call. SendAIMessage updates it before each Stream(...) call.
// One PerTurnState per *coding.System.
type PerTurnState struct {
	mu        sync.Mutex
	aiCtx     ai.AIContext
	extSkills map[string]string // ext name → SKILL.md
}

func (s *PerTurnState) Set(c ai.AIContext, ext map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiCtx = c
	s.extSkills = ext
}

func (s *PerTurnState) snapshot() (ai.AIContext, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aiCtx, s.extSkills
}

// makePromptHook returns a UserPromptSubmit hook that reads the latest
// PerTurnState and assembles an AdditionalContext bundle covering open tabs,
// @-mentions, and per-extension SKILL.md fragments.
func makePromptHook(state *PerTurnState) agent.HookFunc {
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		c, ext := state.snapshot()

		var parts []string
		if t := buildTabContext(c.OpenTabs); t != "" {
			parts = append(parts, t)
		}
		if m := ai.RenderMentionContext(c.MentionedAssets); m != "" {
			parts = append(parts, m)
		}
		if len(ext) > 0 {
			for name, md := range ext {
				parts = append(parts, "## From extension: "+name+"\n"+md)
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		return agent.AddContext(strings.Join(parts, "\n\n")), nil
	}
}

func buildTabContext(tabs []ai.TabInfo) string {
	if len(tabs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The user currently has these tabs open:\n")
	for _, t := range tabs {
		typeName := t.Type
		switch t.Type {
		case "ssh":
			typeName = "SSH Terminal"
		case "database":
			typeName = "Database Query"
		case "redis":
			typeName = "Redis"
		case "sftp":
			typeName = "SFTP"
		}
		b.WriteString("- ")
		b.WriteString(typeName)
		b.WriteString(": \"")
		b.WriteString(t.AssetName)
		b.WriteString("\"\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestPromptHook -v
git add internal/aiagent/hook_prompt.go internal/aiagent/hook_prompt_test.go
git commit -m "✨ UserPromptSubmit hook: inject open-tabs + mentions + ext SKILL.md as AdditionalContext"
```

---

### Task 1.12: Static system prompt builder

**Files:**
- Create: `internal/aiagent/system_prompt.go`
- Create: `internal/aiagent/system_prompt_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"strings"
	"testing"
)

func TestStaticSystemPrompt_Has5Sections(t *testing.T) {
	got := StaticSystemPrompt("zh-cn")
	for _, want := range []string{
		"OpsKat AI assistant",
		"Chinese (Simplified)",
		"update_asset",
		"tool execution fails",
		"user denies",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing section keyword %q", want)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

Copy the static portions of `internal/ai/prompt_builder.go` (`buildRoleDescription`, `buildLanguageHint`, `buildKnowledgeGuidance`, `buildErrorRecoveryGuidance`, `buildUserDenialGuidance`) verbatim into `system_prompt.go` as `StaticSystemPrompt(lang string) string`. Mention/tab parts stay out — those go through Task 1.11.

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestStaticSystemPrompt -v
git add internal/aiagent/system_prompt.go internal/aiagent/system_prompt_test.go
git commit -m "✨ static system prompt (role/language/knowledge/error/denial); per-turn parts via hook"
```

---

### Task 1.13: Convert ID + role context plumbing

**Files:**
- Modify: `internal/aiagent/hook_policy.go` (replace stubs in Task 1.8)
- Create: `internal/aiagent/context.go`
- Create: `internal/aiagent/context_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"
)

func TestConvIDAndAgentRole_RoundTrip(t *testing.T) {
	ctx := WithConvID(context.Background(), 42)
	ctx = WithAgentRole(ctx, "ops-explorer")
	if v := getConvID(ctx); v != 42 {
		t.Fatalf("convID = %d", v)
	}
	if v := getAgentRole(ctx); v != "ops-explorer" {
		t.Fatalf("agentRole = %q", v)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

`internal/aiagent/context.go`:

```go
package aiagent

import "context"

type ctxKey int

const (
	keyConvID ctxKey = iota
	keyAgentRole
)

// WithConvID stamps the active conversation ID for hooks and the bridge.
func WithConvID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, keyConvID, id)
}

// WithAgentRole tags the ctx with the active sub-agent role (empty = top-level).
func WithAgentRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, keyAgentRole, role)
}

func getConvID(ctx context.Context) int64 {
	if v, ok := ctx.Value(keyConvID).(int64); ok {
		return v
	}
	return 0
}

func getAgentRole(ctx context.Context) string {
	if v, ok := ctx.Value(keyAgentRole).(string); ok {
		return v
	}
	return ""
}
```

In `hook_policy.go`, delete the local `getConvID`/`getAgentRole` stubs from Task 1.8 (the file now uses the package-level versions).

- [ ] **Step 4: Verify build + tests**

```
go test ./internal/aiagent/ -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/
git commit -m "✨ aiagent.WithConvID / WithAgentRole context plumbing; wire policy hook"
```

---

### Task 1.14: Event bridge — cago Stream → Wails StreamEvent

**Files:**
- Create: `internal/aiagent/event_bridge.go`
- Create: `internal/aiagent/event_bridge_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"sync"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"

	"github.com/opskat/opskat/internal/ai"
)

func TestBridge_TextDeltaToContent(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(99, agent.Event{Kind: agent.EventTextDelta, Text: "hello"})
	if len(rec.events) != 1 || rec.events[0].Type != "content" || rec.events[0].Content != "hello" {
		t.Fatalf("got %+v", rec.events)
	}
}

func TestBridge_ThinkingThenTextSynthesizesThinkingDone(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventThinkingDelta, Text: "reflecting"})
	br.translate(1, agent.Event{Kind: agent.EventTextDelta, Text: "answer"})

	if len(rec.events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(rec.events), rec.events)
	}
	if rec.events[1].Type != "thinking_done" {
		t.Fatalf("expected synthesized thinking_done as second event, got %s", rec.events[1].Type)
	}
}

func TestBridge_UsageMappingExposesCacheCreation(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventUsage, Usage: provider.Usage{
		PromptTokens: 100, CompletionTokens: 20, CachedTokens: 50, CacheCreationTokens: 30,
	}})
	if rec.events[0].Usage == nil {
		t.Fatal("missing usage")
	}
	u := rec.events[0].Usage
	if u.InputTokens != 100 || u.OutputTokens != 20 || u.CacheReadTokens != 50 || u.CacheCreationTokens != 30 {
		t.Fatalf("usage mapping wrong: %+v", u)
	}
}

func TestBridge_ToolEventsCarryToolCallID(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventPreToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Input: []byte(`{"x":1}`)}})
	br.translate(1, agent.Event{Kind: agent.EventPostToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Response: []byte(`"ok"`)}})
	if rec.events[0].ToolCallID != "abc" || rec.events[1].ToolCallID != "abc" {
		t.Fatalf("ToolCallID lost: %+v", rec.events)
	}
}

type recordEmitter struct {
	mu     sync.Mutex
	events []ai.StreamEvent
}

func (r *recordEmitter) Emit(_ int64, ev ai.StreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

// silence unused
var _ context.Context
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// bridge translates cago agent.Event values into OpsKat ai.StreamEvent values
// and emits them through the EventEmitter. One bridge instance per Stream.
//
// State: tracks "thinking active" so we can synthesize a thinking_done event
// when a non-thinking event interrupts a thinking-delta sequence.
type bridge struct {
	emit            EventEmitter
	thinkingActive  bool
}

func newBridge(em EventEmitter) *bridge { return &bridge{emit: em} }

func (b *bridge) translate(convID int64, ev agent.Event) {
	// Synthesize thinking_done.
	if b.thinkingActive && ev.Kind != agent.EventThinkingDelta {
		b.emit.Emit(convID, ai.StreamEvent{Type: "thinking_done"})
		b.thinkingActive = false
	}

	switch ev.Kind {
	case agent.EventTextDelta:
		b.emit.Emit(convID, ai.StreamEvent{Type: "content", Content: ev.Text})
	case agent.EventThinkingDelta:
		b.thinkingActive = true
		b.emit.Emit(convID, ai.StreamEvent{Type: "thinking", Content: ev.Text})
	case agent.EventPreToolUse:
		if ev.Tool != nil {
			b.emit.Emit(convID, ai.StreamEvent{
				Type:       "tool_start",
				ToolName:   ev.Tool.Name,
				ToolInput:  string(ev.Tool.Input),
				ToolCallID: ev.Tool.ID,
			})
		}
	case agent.EventPostToolUse:
		if ev.Tool != nil {
			var content string
			_ = json.Unmarshal(ev.Tool.Response, &content)
			if content == "" {
				content = string(ev.Tool.Response)
			}
			b.emit.Emit(convID, ai.StreamEvent{
				Type:       "tool_result",
				ToolName:   ev.Tool.Name,
				Content:    content,
				ToolCallID: ev.Tool.ID,
			})
		}
	case agent.EventUsage:
		u := ev.Usage
		b.emit.Emit(convID, ai.StreamEvent{
			Type: "usage",
			Usage: &ai.Usage{
				InputTokens:         u.PromptTokens,
				OutputTokens:        u.CompletionTokens,
				CacheReadTokens:     u.CachedTokens,
				CacheCreationTokens: u.CacheCreationTokens,
			},
		})
	case agent.EventDone:
		b.emit.Emit(convID, ai.StreamEvent{Type: "done"})
	case agent.EventError:
		var msg string
		if ev.Err != nil {
			msg = ev.Err.Error()
		}
		b.emit.Emit(convID, ai.StreamEvent{Type: "error", Error: msg})
	}
}
```

- [ ] **Step 4: Verify pass**

```
go test ./internal/aiagent/ -run TestBridge -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/event_bridge.go internal/aiagent/event_bridge_test.go
git commit -m "✨ bridge: cago Event → OpsKat StreamEvent (incl. thinking_done synthesis + Usage mapping)"
```

---

### Task 1.15: Sub-agent observer (forwards child events tagged with role)

**Files:**
- Create: `internal/aiagent/subagent_observer.go`
- Create: `internal/aiagent/subagent_observer_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

func TestSubagentObserver_TagsToolStartWithAgentRole(t *testing.T) {
	rec := &recordEmitter{}
	obs := makeSubagentObserver(rec, 11, "ops-explorer", "explore /etc")

	obs(context.Background(), agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{ID: "x1", Name: "list_assets"},
	})

	var ts ai.StreamEvent
	for _, e := range rec.events {
		if e.Type == "tool_start" {
			ts = e
		}
	}
	if ts.AgentRole != "ops-explorer" {
		t.Fatalf("AgentRole = %q", ts.AgentRole)
	}
}

func TestSubagentObserver_EmitsAgentStart(t *testing.T) {
	rec := &recordEmitter{}
	_ = makeSubagentObserverEmitStart(rec, 1, "ops-explorer", "task X")
	if len(rec.events) != 1 || rec.events[0].Type != "agent_start" {
		t.Fatalf("expected one agent_start, got %+v", rec.events)
	}
	if rec.events[0].AgentRole != "ops-explorer" || rec.events[0].AgentTask != "task X" {
		t.Fatalf("fields lost: %+v", rec.events[0])
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// makeSubagentObserver returns an agent.Observer that forwards child-stream
// events to the parent stream's emitter, tagging each event with the
// sub-agent's role/task. role/task are captured at construction.
func makeSubagentObserver(em EventEmitter, convID int64, role, task string) agent.Observer {
	br := newBridge(taggingEmitter{inner: em, role: role, task: task})
	return func(_ context.Context, ev agent.Event) {
		br.translate(convID, ev)
	}
}

// makeSubagentObserverEmitStart synchronously emits an agent_start event.
// Returns the observer to install on the child agent (which captures further
// events). Caller is responsible for emitting agent_end via the dispatcher
// PostToolUse on the parent.
func makeSubagentObserverEmitStart(em EventEmitter, convID int64, role, task string) agent.Observer {
	em.Emit(convID, ai.StreamEvent{
		Type:      "agent_start",
		AgentRole: role,
		AgentTask: task,
	})
	return makeSubagentObserver(em, convID, role, task)
}

// taggingEmitter wraps an EventEmitter and stamps AgentRole/AgentTask onto
// each forwarded event before delegating.
type taggingEmitter struct {
	inner EventEmitter
	role  string
	task  string
}

func (e taggingEmitter) Emit(convID int64, ev ai.StreamEvent) {
	if ev.AgentRole == "" {
		ev.AgentRole = e.role
	}
	if ev.AgentTask == "" {
		ev.AgentTask = e.task
	}
	e.inner.Emit(convID, ev)
}
```

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestSubagentObserver -v
git add internal/aiagent/subagent_observer.go internal/aiagent/subagent_observer_test.go
git commit -m "✨ sub-agent observer: tag child events with agent_role/task; emit agent_start"
```

---

### Task 1.15b: dispatch_subagent PostToolUse hook for `agent_end`

**Files:**
- Modify: `internal/aiagent/subagent_observer.go` (add `MakeAgentEndHook`)
- Create test in `internal/aiagent/subagent_observer_test.go`

**Why:** cago's `subagent.NewTool` returns the child run's final text as the tool's response. We need to surface that as an `agent_end` Wails event so the frontend's `agent` block transitions from "running" → "completed" with the truncated summary.

- [ ] **Step 1: Write failing test**

```go
func TestMakeAgentEndHook_EmitsAgentEndOnDispatchPost(t *testing.T) {
	rec := &recordEmitter{}
	hook := MakeAgentEndHook(rec)
	long := strings.Repeat("Y", 4096)
	_, _ = hook(context.Background(), agent.HookInput{
		Stage:        agent.StagePostToolUse,
		ToolName:     "dispatch_subagent",
		ToolResponse: []byte(strconv.Quote(long)),
	})
	if len(rec.events) != 1 || rec.events[0].Type != "agent_end" {
		t.Fatalf("expected agent_end, got %+v", rec.events)
	}
	if len(rec.events[0].Content) > 2048+3 {
		t.Fatalf("summary not truncated to 2048 (+ellipsis): %d", len(rec.events[0].Content))
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

Append to `internal/aiagent/subagent_observer.go`:

```go
// MakeAgentEndHook returns a PostToolUse hook that converts the
// dispatch_subagent tool's response into an agent_end Wails event with the
// summary truncated to 2048 chars (matching the legacy spawn_agent behavior).
// Register only with matcher "dispatch_subagent".
func MakeAgentEndHook(em EventEmitter) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		if in.ToolName != "dispatch_subagent" {
			return nil, nil
		}
		var summary string
		_ = json.Unmarshal(in.ToolResponse, &summary)
		if summary == "" {
			summary = string(in.ToolResponse)
		}
		if len(summary) > 2048 {
			summary = summary[:2048] + "..."
		}
		em.Emit(getConvID(ctx), ai.StreamEvent{
			Type:    "agent_end",
			Content: summary,
		})
		return nil, nil
	}
}
```

- [ ] **Step 4: Wire into System (Task 1.19's coding.WithAgentOpts)**

Add to the agent.Option list in `NewSystem`:

```go
agent.PostToolUse("dispatch_subagent", MakeAgentEndHook(opts.Emitter)),
```

- [ ] **Step 5: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestMakeAgentEndHook -v
git add internal/aiagent/
git commit -m "✨ agent_end hook: emit truncated summary on dispatch_subagent PostToolUse"
```

---

### Task 1.16: Ops Entries (`subagent_ops.go`)

**Files:**
- Create: `internal/aiagent/subagent_ops.go`
- Create: `internal/aiagent/subagent_ops_test.go`

- [ ] **Step 1: Write failing test**

```go
package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/provider/providertest"
)

func TestOpsExplorerEntry_HasReadOnlyTools(t *testing.T) {
	mock := providertest.New()
	deps := &Deps{}
	em := EmitterFunc(func(int64, _ struct{}) {}) // unused in this test
	_ = em

	entry := OpsExplorerEntry(mock, deps, "/")
	if entry.Type != "ops-explorer" {
		t.Fatalf("Type = %q", entry.Type)
	}
	// Inspect Entry's Agent's tools — implementation depends on cago API; assert
	// that critical write-tools are absent.
	for _, name := range []string{"add_asset", "update_asset", "exec_redis"} {
		if entryHasTool(entry, name) {
			t.Errorf("write-tool %q must be excluded from ops-explorer", name)
		}
	}
	for _, name := range []string{"list_assets", "get_asset", "run_command"} {
		if !entryHasTool(entry, name) {
			t.Errorf("read-tool %q missing from ops-explorer", name)
		}
	}
	_ = context.Background
}

// entryHasTool helper — requires a way to introspect cago Entry.Agent's tools.
// If cago doesn't expose tools after construction, instead inspect the slice
// passed to subagent.Entry construction by saving it on a sidecar. Update the
// implementation accordingly in Step 3.
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/tool"
	"github.com/cago-frame/agents/tool/subagent"

	"github.com/opskat/opskat/internal/ai"
)

const (
	roleExplorer = "ops-explorer"
	roleBatch    = "ops-batch"
	roleReadOnly = "ops-readonly"
)

// OpsExplorerEntry builds the read-leaning sub-agent for "investigate this".
// Tools: list/get assets+groups, read-only run_command/exec_sql/exec_redis/exec_mongo.
func OpsExplorerEntry(prov provider.Provider, deps *Deps, cwd string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets": true, "get_asset": true,
		"list_groups": true, "get_group": true,
		"run_command": true, "exec_sql": true, "exec_redis": true,
		"exec_mongo": true, "exec_k8s": true,
		"kafka_cluster": true, "kafka_topic": true, "kafka_consumer_group": true,
		"kafka_acl": true, "kafka_schema": true, "kafka_connect": true, "kafka_message": true,
		"request_permission": true, // child can still request grants
	})
	return coding.Explore(prov, cwd,
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-explorer sub-agent. Investigate efficiently and report findings concisely."),
	)
}

// OpsBatchEntry — for "execute the same thing across N assets" tasks.
func OpsBatchEntry(prov provider.Provider, deps *Deps, cwd string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets": true, "get_asset": true, "list_groups": true,
		"run_command": true, "exec_sql": true, "exec_redis": true, "exec_mongo": true,
		"batch_command": true, "request_permission": true,
	})
	e := coding.GeneralPurpose(prov, cwd,
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-batch sub-agent. Coordinate parallel ops across multiple assets and consolidate results."),
	)
	e.Type = roleBatch
	return e
}

// OpsReadOnlyEntry — strictly read commands, no execution.
func OpsReadOnlyEntry(prov provider.Provider, deps *Deps, cwd string) subagent.Entry {
	tools := filterTools(OpsTools(deps), map[string]bool{
		"list_assets": true, "get_asset": true, "list_groups": true, "get_group": true,
	})
	e := coding.Explore(prov, cwd,
		coding.SubagentWithTools(tools...),
		coding.SubagentWithSystem("You are an OpsKat ops-readonly sub-agent. Inspect inventory; do not execute commands."),
	)
	e.Type = roleReadOnly
	return e
}

func filterTools(in []tool.Tool, allow map[string]bool) []tool.Tool {
	out := make([]tool.Tool, 0, len(in))
	for _, t := range in {
		if allow[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

// entryHasTool helper for tests (also useful at runtime).
// If cago's subagent.Entry exposes Tools(), use that; otherwise, OpsKat
// constructs entries via filterTools and we cache the names locally.
func entryHasTool(e subagent.Entry, name string) bool {
	// Cago exposes the underlying agent's tools through the Agent field; we
	// can't introspect easily without going through the runtime. As a
	// pragmatic substitute, this helper is only used by tests and reads from
	// a per-entry tool-name index stamped at construction below.
	_ = agent.System // pull cago agent into this file
	idx, _ := entryToolNames.Load(e.Type)
	if idx == nil {
		return false
	}
	for _, n := range idx.([]string) {
		if n == name {
			return true
		}
	}
	return false
}
```

(Add a private `var entryToolNames sync.Map` that the three constructors populate before returning. Tests rely on it.)

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestOpsExplorer -v
git add internal/aiagent/subagent_ops.go internal/aiagent/subagent_ops_test.go
git commit -m "✨ ops Entries: ops-explorer / ops-batch / ops-readonly for dispatch_subagent"
```

---

### Task 1.17: Session.Store adapter (gormStore)

**Files:**
- Create: `internal/aiagent/store.go`
- Create: `internal/aiagent/store_test.go`

- [ ] **Step 1: Write failing test**

Test that `gormStore.Save(ctx, sessionID, msgs)` writes rows to `conversation_messages` keyed by parsing `sessionID = fmt.Sprintf("conv_%d", convID)`; `Load` returns them in `SortOrder` ascending. Use the existing `bootstrap_test.go` pattern (in-memory SQLite fixture) — copy it from `internal/repository/conversation_repo`.

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/conversation_svc"
)

// gormStore satisfies cago's agent.Store by reading/writing conversation_messages.
// SessionID format: "conv_<conversationID>" (matches OpsKat's existing convention).
type gormStore struct{}

// NewGormStore constructs a gormStore. No state — relies on the global
// conversation_svc.Conversation() singleton.
func NewGormStore() agent.Store { return &gormStore{} }

func (s *gormStore) Save(ctx context.Context, sessionID string, msgs []agent.Message) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	rows := make([]*conversation_entity.Message, 0, len(msgs))
	for i, m := range msgs {
		rows = append(rows, &conversation_entity.Message{
			ConversationID: convID,
			Role:           string(m.Role),
			Content:        m.Text,
			SortOrder:      i,
			Createtime:     time.Now().Unix(),
		})
	}
	return conversation_svc.Conversation().SaveMessages(ctx, convID, rows)
}

func (s *gormStore) Load(ctx context.Context, sessionID string) ([]agent.Message, error) {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := conversation_svc.Conversation().LoadMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	out := make([]agent.Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, agent.Message{
			Role: agent.Role(r.Role),
			Text: r.Content,
		})
	}
	return out, nil
}

func parseSessionID(s string) (int64, error) {
	if !strings.HasPrefix(s, "conv_") {
		return 0, fmt.Errorf("invalid session id: %q", s)
	}
	return strconv.ParseInt(strings.TrimPrefix(s, "conv_"), 10, 64)
}
```

(NOTE: cago's `agent.Message` shape — Role / Text / Kind etc — must be checked when implementing. Adjust the field names if cago's Message exposes more (e.g., `agent.Message.Kind`). Map only the fields cago needs to round-trip history.)

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestGormStore -v
git add internal/aiagent/store.go internal/aiagent/store_test.go
git commit -m "✨ gormStore adapter: cago Session.Store ↔ conversation_messages"
```

---

### Task 1.18: Retry wrapper

**Files:**
- Create: `internal/aiagent/retry.go`
- Create: `internal/aiagent/retry_test.go`

- [ ] **Step 1: Write failing test**

Test calcRetryDelay's schedule (10 attempts, [2,4,8,15,15,...]s, ±20% jitter) and Retry-After honoring. Compose test cases for each attempt index.

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

Copy the entire `calcRetryDelay` / `addJitter` / `retryDelays[]` / `MaxRetries` from `internal/ai/conversation_runner.go:212-230` verbatim into `internal/aiagent/retry.go`. Add:

```go
// RunWithRetry wraps sess.Stream with the legacy retry policy: 10 attempts,
// exponential backoff with ±20% jitter, Retry-After honored. Between attempts
// it emits a "retry" Wails event via em. The drain callback runs once per
// successful Stream-open and is responsible for consuming events (typically
// translating them through the event bridge).
func RunWithRetry(ctx context.Context, sess *agent.Session, prompt string,
	em EventEmitter, convID int64,
	drain func(stream *agent.Stream),
) (*agent.Result, error) {

	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		stream, err := sess.Stream(ctx, prompt)
		if err != nil {
			lastErr = err
			if !shouldRetry(err) || attempt == MaxRetries {
				return nil, err
			}
		} else {
			drain(stream)
			res, runErr := stream.Result()
			if runErr == nil {
				return res, nil
			}
			lastErr = runErr
			if !shouldRetry(runErr) || attempt == MaxRetries {
				return res, runErr
			}
		}
		delay := calcRetryDelay(attempt, lastErr)
		em.Emit(convID, ai.StreamEvent{
			Type:    "retry",
			Content: fmt.Sprintf("%d/%d", attempt, MaxRetries),
			Error:   lastErr.Error(),
		})
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func shouldRetry(err error) bool {
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return pe.StatusCode >= 500 || pe.StatusCode == 429
	}
	return true // be permissive — let the retry budget run out instead
}
```

- [ ] **Step 4: Verify pass; commit**

```
go test ./internal/aiagent/ -run TestRetry -v
git add internal/aiagent/retry.go internal/aiagent/retry_test.go
git commit -m "✨ retry wrapper: 10-attempt schedule, jitter, Retry-After honoring, retry event"
```

---

### Task 1.19: System constructor + lifecycle

**Files:**
- Create: `internal/aiagent/system.go`
- Create: `internal/aiagent/system_test.go`

- [ ] **Step 1: Write the smoke test**

```go
package aiagent

import (
	"context"
	"os"
	"testing"

	"github.com/cago-frame/agents/provider/providertest"

	"github.com/opskat/opskat/internal/ai"
)

func TestSystem_NewClose(t *testing.T) {
	mock := providertest.New()
	mock.QueueStream() // empty — close immediately

	sys, err := NewSystem(context.Background(), SystemOptions{
		Provider:      mock,
		Cwd:           os.TempDir(),
		ConvID:        1,
		Lang:          "en",
		Deps:          &Deps{},
		Emitter:       EmitterFunc(func(int64, ai.StreamEvent) {}),
		PolicyChecker: nil, // OK for smoke
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sys.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```go
package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/agents/provider"
)

// SystemOptions wires together the dependencies for one *coding.System.
// One System per active Conversation.
type SystemOptions struct {
	Provider      provider.Provider
	Cwd           string
	ConvID        int64
	Lang          string
	Deps          *Deps
	Emitter       EventEmitter
	PolicyChecker PolicyChecker
	AuditWriter   ai.AuditWriter // nil = ai.NewDefaultAuditWriter()
	Resolver      PendingResolver
	Activate      func() // window activation; may be nil
}

// System is the OpsKat-facing handle around cago's coding.System.
type System struct {
	cs       *coding.System
	sess     *agent.Session
	convID   int64
	emitter  EventEmitter
	turnState *PerTurnState
	sidecar  *sidecar
	bridge   *bridge
}

// NewSystem assembles the per-Conversation cago System and returns a *Session
// the caller drives with Stream/Steer/FollowUp.
func NewSystem(ctx context.Context, opts SystemOptions) (*System, error) {
	if opts.Deps == nil {
		opts.Deps = NewDeps(nil, nil)
	}
	if opts.AuditWriter == nil {
		opts.AuditWriter = ai.NewDefaultAuditWriter()
	}

	gw := NewApprovalGateway(opts.Emitter, opts.Resolver)
	gw.SetActivateFunc(opts.Activate)

	sc := newSidecar()
	turnState := &PerTurnState{}

	policyHook := makePolicyHook(opts.Deps, sc, gw, opts.PolicyChecker)
	auditHook := makeAuditHook(sc, opts.AuditWriter)
	roundsHook := makeRoundsHook(50)
	promptHook := makePromptHook(turnState)

	parentTools := append([]tool.Tool{}, OpsTools(opts.Deps)...)
	parentTools = append(parentTools, wrapToolDef(extToolDef(), opts.Deps))

	subEntries := []subagent.Entry{
		OpsExplorerEntry(opts.Provider, opts.Deps, opts.Cwd),
		OpsBatchEntry(opts.Provider, opts.Deps, opts.Cwd),
		OpsReadOnlyEntry(opts.Provider, opts.Deps, opts.Cwd),
	}

	cs, err := coding.New(ctx, opts.Provider, opts.Cwd,
		coding.AppendSystem(StaticSystemPrompt(opts.Lang)),
		coding.WithExtraTools(parentTools...),
		coding.WithExtraSubagents(subEntries...),
		coding.WithSkillDirs(opts.Cwd+"/.claude/skills"),    // confine to cwd; do NOT scan ~/.claude
		coding.WithCompactionThreshold(80000),                // matches today's heuristic
		coding.WithAgentOpts(
			agent.PreToolUse("", policyHook),
			agent.PreToolUse("", roundsHook),
			agent.PostToolUse("", auditHook),
			agent.UserPromptSubmit(promptHook),
		),
	)
	if err != nil {
		return nil, err
	}

	a := cs.Agent()
	sess := a.Session(agent.WithStore(NewGormStore()), agent.WithID(fmt.Sprintf("conv_%d", opts.ConvID)))

	return &System{
		cs:        cs,
		sess:      sess,
		convID:    opts.ConvID,
		emitter:   opts.Emitter,
		turnState: turnState,
		sidecar:   sc,
		bridge:    newBridge(opts.Emitter),
	}, nil
}

// Stream sends prompt+context for the next turn, drains the cago Stream
// through the retry wrapper, translates events to Wails, and returns when done.
func (s *System) Stream(ctx context.Context, prompt string, aiCtx ai.AIContext, ext map[string]string) error {
	s.turnState.Set(aiCtx, ext)
	ctx = WithConvID(ctx, s.convID)

	streamCtx, cancel := context.WithCancel(ctx)
	s.streamCancel = cancel
	defer cancel()

	// RunWithRetry handles 10-attempt backoff + Retry-After + emits "retry" events.
	// We pass an Observer-style draining function via a wrapper: each Stream
	// produced inside RunWithRetry has its events translated by s.bridge before
	// the retry layer decides whether to retry or return.
	_, err := RunWithRetry(streamCtx, s.sess, prompt, s.emitter, s.convID,
		func(stream *agent.Stream) {
			for stream.Next() {
				s.bridge.translate(s.convID, stream.Event())
			}
		},
	)
	return err
}

// Steer injects a mid-cycle user message. Mirrors today's QueueAIMessage.
func (s *System) Steer(ctx context.Context, body, displayContent string) error {
	s.emitter.Emit(s.convID, ai.StreamEvent{Type: "queue_consumed", Content: displayContent})
	return s.sess.Steer(ctx, body, agent.AsUser(), agent.Persist(true))
}

// Close releases sub-agents, the parent agent, and per-Conversation Deps.
func (s *System) Close(ctx context.Context) error {
	_ = s.sess.Close(ctx)
	if s.cs != nil {
		_ = s.cs.Close(ctx)
	}
	// Deps is owned by the App; not closed here.
	return nil
}
```

(`extToolDef()` is a small helper that returns the `exec_tool` ToolDef from `ai.AllToolDefs()`. Could also just include `exec_tool` in `OpsTools` — adjust to your preference.)

- [ ] **Step 4: Verify smoke test passes**

```
go test ./internal/aiagent/ -run TestSystem -v
```

- [ ] **Step 5: Commit**

```
git add internal/aiagent/system.go internal/aiagent/system_test.go
git commit -m "✨ aiagent.System: coding.New wrapper + per-Conversation lifecycle"
```

---

## Phase 2 — App integration behind feature flag

### Task 2.1: Wire `App.aiAgentSystems` map (parallel to `App.aiAgent`)

**Files:**
- Modify: `internal/app/app.go` (add field)
- Modify: `internal/app/app_ai.go`

- [ ] **Step 1: Add a parallel runner store**

In `internal/app/app.go`'s `App` struct, add:

```go
aiAgentSystems sync.Map // convID → *aiagent.System (Phase 1 cago-backend runners)
```

- [ ] **Step 2: New `sendAIMessageCago` function**

Add in `internal/app/app_ai.go` a parallel `sendAIMessageCago(convID, messages, aiCtx)` that:
1. Looks up / creates a `*aiagent.System` per convID.
2. Builds a single user prompt string from `messages` (last user turn).
3. Calls `sys.Stream(ctx, prompt, aiCtx, extSkillMDs)`.

The legacy `SendAIMessage` continues unchanged. The Wails-bound entrypoint dispatches:

```go
func (a *App) SendAIMessage(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
	if aiagent.Enabled() {
		return a.sendAIMessageCago(convID, messages, aiCtx)
	}
	return a.sendAIMessageLegacy(convID, messages, aiCtx)
}
```

(Rename current body to `sendAIMessageLegacy`.)

- [ ] **Step 3: Build & smoke test the legacy path still works**

```
make build && OPSKAT_AI_BACKEND= make dev
```

Open one conversation, send a message — verify legacy path still works.

- [ ] **Step 4: Commit**

```
git add internal/app/
git commit -m "✨ feature-flag dispatch: SendAIMessage routes to cago when OPSKAT_AI_BACKEND=cago"
```

---

### Task 2.2: Wire approval gateway into App

**Files:**
- Modify: `internal/app/app_approval.go`
- Modify: `internal/app/app_ai.go`

- [ ] **Step 1: Construct gateway in `activateProvider`**

When the feature flag is on, replace `makeCommandConfirmFunc` with the call into `ApprovalGateway`. The `pendingAIApprovals` map and `RespondAIApproval` Wails binding stay unchanged — both legacy and new paths read/write the same map.

The gateway's `PendingResolver` adapts the existing map:

```go
resolver := func(confirmID string) (chan ai.ApprovalResponse, func()) {
    ch := make(chan ai.ApprovalResponse, 1)
    a.pendingAIApprovals.Store(confirmID, ch)
    return ch, func() { a.pendingAIApprovals.Delete(confirmID) }
}
```

- [ ] **Step 2: Wire window activation**

```go
gw.SetActivateFunc(func() { a.activateWindow() })
```

- [ ] **Step 3: Smoke test**

Run dev with `OPSKAT_AI_BACKEND=cago`, trigger a `run_command` requiring confirmation, verify:
- approval_request event reaches frontend
- frontend renders the approval block
- responding "allow" resumes the run
- `RespondAIApproval` writes to `pendingAIApprovals[confirmID]`

- [ ] **Step 4: Commit**

```
git add internal/app/
git commit -m "🔧 wire ApprovalGateway into App; share pendingAIApprovals across both backends"
```

---

### Task 2.3: Wire QueueAIMessage to System.Steer

**Files:**
- Modify: `internal/app/app_ai.go`

- [ ] **Step 1: Branch QueueAIMessage**

```go
func (a *App) QueueAIMessage(convID int64, content string, mentions []ai.MentionedAsset) error {
	if aiagent.Enabled() {
		v, ok := a.aiAgentSystems.Load(convID)
		if !ok {
			return fmt.Errorf("conversation %d not active", convID)
		}
		sys := v.(*aiagent.System)
		body := content
		if mctx := ai.RenderMentionContext(mentions); mctx != "" {
			body = mctx + "\n\n" + content
		}
		return sys.Steer(a.ctx, body, content)
	}
	// legacy path unchanged below
	...
}
```

- [ ] **Step 2: Smoke test**

Mid-stream, queue a `@assetname` message; verify the user message appears with chips and the AI continues with mention context.

- [ ] **Step 3: Commit**

```
git add internal/app/app_ai.go
git commit -m "✨ QueueAIMessage routes to System.Steer when feature flag is on"
```

---

### Task 2.4: Wire StopAIGeneration

**Files:**
- Modify: `internal/app/app_ai.go`

- [ ] **Step 1: Branch StopAIGeneration**

When flag is on, cancel the `*aiagent.System`'s active stream context. Add a `Cancel()` method on `aiagent.System` that exposes the cancel func captured at `Stream` start (store it on the System struct).

Update `aiagent/system.go`:

```go
type System struct {
	...
	streamCancel context.CancelFunc
}

func (s *System) Stream(ctx context.Context, ...) error {
	streamCtx, cancel := context.WithCancel(ctx)
	s.streamCancel = cancel
	defer cancel()
	...
}

func (s *System) Cancel() {
	if s.streamCancel != nil {
		s.streamCancel()
	}
}
```

In `app_ai.go`'s `StopAIGeneration`:

```go
if aiagent.Enabled() {
    if v, ok := a.aiAgentSystems.Load(convID); ok {
        v.(*aiagent.System).Cancel()
    }
    return nil
}
```

After cancel, emit a `stopped` event so frontend cleans up cancelled tool blocks.

- [ ] **Step 2: Smoke test stop flow**

- [ ] **Step 3: Commit**

```
git add internal/aiagent/system.go internal/app/app_ai.go
git commit -m "✨ StopAIGeneration: cancel cago Stream + emit stopped event"
```

---

### Task 2.5: Conversation switch / delete close cago Systems

**Files:**
- Modify: `internal/app/app_ai.go`

- [ ] **Step 1: On DeleteConversation**

```go
if v, ok := a.aiAgentSystems.LoadAndDelete(id); ok {
    _ = v.(*aiagent.System).Close(a.ctx)
}
```

- [ ] **Step 2: On `resetRunners` (provider switch)**

Iterate `a.aiAgentSystems` and `Close()` each, then `Range` delete.

- [ ] **Step 3: Smoke test conversation switch + provider switch**

- [ ] **Step 4: Commit**

```
git add internal/app/app_ai.go
git commit -m "🔧 close *aiagent.System on conversation delete + provider switch"
```

---

### Task 2.6: Run regression matrix under feature flag

- [ ] **Step 1: Manual regression checklist**

Open a fresh OpsKat dev build with `OPSKAT_AI_BACKEND=cago`. Walk through:

- [ ] Send a simple text question; AI responds.
- [ ] Send a `run_command` request; approval_request appears; approve; tool runs; tool_result rendered.
- [ ] Same as above but deny; AI acknowledges and stops.
- [ ] `request_permission` flow (grant kind); user edits patterns; approve; subsequent commands matching the pattern run without prompting.
- [ ] `batch_command` across 3 assets; approval and per-asset results.
- [ ] `dispatch_subagent type=ops-explorer task=...`; agent_start/agent_end blocks render; nested tool_start under agent block.
- [ ] Stop generation mid-tool-call; `stopped` event; tool block goes "cancelled".
- [ ] Mid-stream `@asset` queued message; `queue_consumed` followed by new user/assistant pair; mention chips intact.
- [ ] DeepSeek-v4 thinking model: turn 2 with prior tool_calls succeeds (no 400).
- [ ] Anthropic Claude with extended thinking: thinking block round-trip OK.
- [ ] Audit log row written for each tool call with correct Decision / MatchedPattern.
- [ ] opsctl IPC approval still works (single + batch + grant + ext_tool).
- [ ] Switch provider mid-conversation; subsequent send rebuilds correctly.

- [ ] **Step 2: Commit a regression report**

```
git add docs/superpowers/plans/
git commit -m "📄 regression report: cago backend behind feature flag"
```

(Append the checklist results as a comment block in the plan file or a sibling `.md`. Capture failures so subsequent tasks fix them.)

---

### Task 2.7: Flip default + delete legacy code

**Files:**
- Modify: `internal/aiagent/feature_flag.go`
- Delete: `internal/ai/agent.go`, `internal/ai/conversation_runner.go`, `internal/ai/compress.go`, `internal/ai/provider.go`, `internal/ai/anthropic.go`, `internal/ai/openai.go`, `internal/ai/tool_handler_agent.go`
- Modify: `internal/app/app_ai.go` (drop legacy branches)
- Modify: `internal/app/app_ai_provider.go` (the legacy `activateProvider` body)

Only do this **after** Task 2.6 is green AND the cago upstream PRs from Phase 0 are merged.

- [ ] **Step 1: Flip the flag default**

```go
func Enabled() bool { return os.Getenv("OPSKAT_AI_BACKEND") != "legacy" }
```

(Keep an off-switch for one release as a safety valve.)

- [ ] **Step 2: Delete legacy files**

```
git rm internal/ai/agent.go internal/ai/conversation_runner.go internal/ai/compress.go \
       internal/ai/provider.go internal/ai/anthropic.go internal/ai/openai.go \
       internal/ai/tool_handler_agent.go
```

- [ ] **Step 3: Remove legacy branches in app_ai.go / app_ai_provider.go**

`activateProvider`: rebuild as cago Provider construction (use `cago/provider/openai` and `cago/provider/anthropics`). Remove the `*ai.Agent` field.

When mapping OpsKat's reasoning effort to cago's `provider.ThinkingConfig`, collapse `xhigh` and `max` down to `provider.ThinkingHigh` (cago doesn't model finer levels — see spec §4.3 ThinkingEffort note):

```go
func toThinkingEffort(e string) provider.ThinkingEffort {
    switch strings.ToLower(strings.TrimSpace(e)) {
    case "low":
        return provider.ThinkingLow
    case "medium":
        return provider.ThinkingMedium
    case "high", "xhigh", "max":
        return provider.ThinkingHigh
    }
    return ""
}
```

`SendAIMessage`, `QueueAIMessage`, `StopAIGeneration`: remove the `if aiagent.Enabled() {} else {}` branching; keep only the cago path.

- [ ] **Step 4: Update go.mod (remove `replace` if upstream merged)**

```
go mod edit -dropreplace github.com/cago-frame/agents
go mod tidy
```

Pin the new published version.

- [ ] **Step 5: Build + full test suite**

```
make lint
make test
cd frontend && pnpm test
```

All green.

- [ ] **Step 6: Commit**

```
git add -A
git commit -m "♻️ replace built-in AI agent with cago app/coding; delete legacy internal/ai loop"
```

---

## Phase 3 — Frontend cwd picker UI

### Task 3.1: Conversation creation accepts cwd

**Files:**
- Modify: `internal/app/app_ai.go` (`CreateConversation` accepts cwd)
- Modify: `internal/service/conversation_svc/conversation.go` (remove the dangerous `os.RemoveAll(WorkDir)` cleanup since user-chosen dirs must NOT be deleted)

- [ ] **Step 1: Backend change — don't auto-delete WorkDir**

Open `internal/service/conversation_svc/conversation.go:93-97`. Delete the `os.RemoveAll(conv.WorkDir)` block — the column now stores a user-chosen path; deleting it on conversation delete would erase user data.

- [ ] **Step 2: Default cwd = user home for new conversations**

In `App.CreateConversation`, set `conv.WorkDir = userHomeDir` if empty.

```go
home, _ := os.UserHomeDir()
conv := &conversation_entity.Conversation{
    Title:      "新对话",
    ProviderID: providerID,
    WorkDir:    home,
}
```

- [ ] **Step 3: Add `UpdateConversationCwd(id, cwd)` Wails binding**

```go
func (a *App) UpdateConversationCwd(id int64, cwd string) error {
    return conversation_svc.Conversation().UpdateWorkDir(a.langCtx(), id, cwd)
}
```

Add the corresponding repo + service method.

- [ ] **Step 4: Tests + commit**

```
go test ./internal/service/conversation_svc/...
git add -A
git commit -m "✨ Conversation.WorkDir: user-chosen cwd; default user home; remove dangerous auto-delete"
```

---

### Task 3.2: cwd picker UI

**Files:**
- Create: `frontend/src/components/ai/CwdPicker.tsx`
- Modify: `frontend/src/stores/aiStore.ts` (call new Wails binding)
- Modify: `frontend/src/components/ai/SideAssistantHeader.tsx` (renders the conversation title row; insert `<CwdPicker />` next to it)

- [ ] **Step 1: Identify AI panel header**

```
rg "conversation" frontend/src/components/ai/ -l
```

Find the file that renders the active conversation's title (e.g., `ConversationHeader.tsx` / `AIPanel.tsx`).

- [ ] **Step 2: Build CwdPicker.tsx**

Renders the current `conv.WorkDir`, a button to "change directory", and on click opens a native folder picker. Use Wails `OpenDirectoryDialog` runtime API (already used elsewhere — grep `OpenDirectoryDialog`).

```tsx
import { OpenDirectoryDialog } from "@/wailsjs/runtime/runtime";
import { UpdateConversationCwd } from "@/wailsjs/go/app/App";

interface Props { convId: number; cwd: string; onChange: (cwd: string) => void; }

export function CwdPicker({ convId, cwd, onChange }: Props) {
  const pick = async () => {
    const dir = await OpenDirectoryDialog({ DefaultDirectory: cwd });
    if (!dir) return;
    await UpdateConversationCwd(convId, dir);
    onChange(dir);
  };
  return (
    <button onClick={pick} className="text-xs text-muted-foreground hover:underline">
      📁 {cwd || "(not set)"}
    </button>
  );
}
```

- [ ] **Step 3: Wire into AI panel header**

Render `<CwdPicker />` in the conversation header. On change, update local state and refresh the conversation row from `aiStore`.

- [ ] **Step 4: Add to aiStore**

```ts
async updateConversationCwd(convId: number, cwd: string) {
  await UpdateConversationCwd(convId, cwd);
  // refresh
  const convs = await ListConversations();
  set({ conversations: convs });
}
```

- [ ] **Step 5: Type-check + lint**

```
cd frontend && pnpm lint && pnpm tsc --noEmit
```

- [ ] **Step 6: Commit**

```
git add frontend/src
git commit -m "✨ AI panel cwd picker: choose working directory per conversation"
```

---

### Task 3.3: cwd indicator in conversation list

**Files:**
- Modify: `frontend/src/components/ai/ConversationList.tsx` (or similar)

- [ ] **Step 1: Show cwd basename next to each conversation**

Display `path.basename(conv.WorkDir)` as a subtitle / chip. Helpful for users with many conversations across projects.

- [ ] **Step 2: Smoke test, commit**

```
git add frontend/src
git commit -m "🎨 conversation list: show cwd basename"
```

---

### Task 3.4: Optional — surface `/compact` and `/help` in chat input

**Files:**
- Modify: `frontend/src/components/ai/ChatInput.tsx`

- [ ] **Step 1: Detect `/` prefix; resolve via SlashRegistry**

Add a Wails binding `RunSlash(convId, command, args)` that calls `system.SlashRegistry().Resolve(...)` then `Run(ctx, sess)`. Render the returned `BuiltinOutput.Notice` as a system message in the conversation.

- [ ] **Step 2: Smoke test `/compact` mid-conversation; verify EventCompacted handled**

- [ ] **Step 3: Commit**

```
git add -A
git commit -m "✨ chat input: support /compact and /help slash commands"
```

---

## Self-Review Checklist

Before declaring the plan complete, verify:

- [ ] Every spec §5 entry is covered by a Phase 1/2 task.
- [ ] §5.4 "behaviors explicitly preserved" — every bullet maps to a task.
- [ ] §4.1 + §4.2 + §4.3 cago changes are in Phase 0.
- [ ] §6 Pre→Post sidecar mechanism implemented (Task 1.5).
- [ ] §3.3 event mapping (incl. thinking_done synthesis + Usage incl. CacheCreationTokens) implemented (Task 1.14).
- [ ] §8 Testing strategy reflected in TDD steps throughout.
- [ ] §9 acceptance criteria verifiable from Task 2.6 regression matrix.
- [ ] §10 phased rollout matches Phase 0/1/2/3 in this plan.

If any item lacks a task, stop here and add the task before handing off to executing.
