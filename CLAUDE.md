# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpsKat is an AI-first desktop app for managing remote infrastructure (SSH, databases, Redis). **Wails v2** (Go 1.25 + React 19), IPC only — no HTTP API. Module: `github.com/opskat/opskat`.

## Common Commands

```bash
# Dev / build
make dev              # Wails hot-reload dev
make build            # Production build
make build-embed      # Production with embedded opsctl
make build-cli        # Standalone opsctl CLI
make clean

# Test
make test                              # All Go tests
go test ./internal/ai/ -run TestName   # Single Go test
cd frontend && pnpm test               # Frontend (vitest)
make test-fixtures && make test-e2e    # E2E (needs ../extensions sibling)

# Lint
make lint / make lint-fix              # golangci-lint
cd frontend && pnpm lint / pnpm lint:fix

# Extensions
make devserver EXT=<name>              # Standalone dev server for one extension (refuses if OPSKAT_ENV=production)
```

## Architecture

### Backend (Go) — layered

```
main.go → internal/app/ (Wails bindings)
            → internal/service/    (business logic, *_svc)
            → internal/repository/ (data access, interface + impl)
            → internal/model/      (entities)
```

Bindings stay thin: parse → service → return. Business rules in `service/`, persistence in `repository/`. Logic inside `App` is unreachable from tests and `opsctl`.

**Key subsystems:**
- `internal/ai/` — provider abstraction (Anthropic/OpenAI), tool registry, command policy, conversation runner, audit
- `internal/sshpool/`, `internal/connpool/` — SSH pool (Unix socket proxy for opsctl); DB/Redis tunnels
- `internal/approval/` — Unix-socket approval flow between desktop app and opsctl
- `internal/bootstrap/` — DB, credentials, migrations, auth tokens
- `pkg/extension/` — WASM runtime (wazero); `HostProvider` in `host.go` defines WASM capabilities
- `cmd/opsctl/`, `cmd/devserver/` — standalone CLI / single-extension dev server

**Data:** GORM + SQLite, gormigrate migrations in `/migrations/`. Soft deletes via `Status` field (`StatusActive=1`, `StatusDeleted=2`), **not** GORM soft delete. Credentials: Argon2id KDF + AES-256-GCM, master key in OS keychain.

**Extensions:** WASM modules with `manifest.json`-declared tools. AI invokes them via a **single `exec_tool`** (not one tool per extension). Dispatcher in `internal/ai/tool_handler_ext.go` enforces extension policy type against asset policy groups before `Plugin.CallTool`.

### Frontend (React + TypeScript)

`frontend/` is a pnpm workspace. Root app uses `@opskat/ui` (`packages/ui`); `packages/devserver-ui` is embedded by `cmd/devserver`. Vite 6, Tailwind 4, shadcn/ui (Radix), Zustand 5.

- **No React Router** — custom tabs in `tabStore`. Tab types: `terminal | ai | query | page | info`.
- One Zustand store per domain in `src/stores/`.
- Backend calls via Wails-generated bindings (`frontend/wailsjs/go/app/App`); events via `EventsOn()`.
- i18n: i18next, locales in `src/i18n/locales/{zh-CN,en}/common.json`, all keys under `"common"` namespace → `t("key.subkey")`.
- Terminal: xterm.js 6, split-pane.
- Tests: Vitest + happy-dom + RTL; Wails runtime mocked in `src/__tests__/setup.ts`.

## Conventions

- **Commits:** gitmoji — ✨ feature, 🐛 fix, ♻️ refactor, 🎨 UI, ⚡️ perf, 🔒 security, 🔧 config, ✅ tests, 📄 docs, 🚀 release.
- **Go mocks:** `go.uber.org/mock` in `mock_*/` subdirs; regen via `go generate ./...`.
- **Go tests:** goconvey + testify.
- **Frontend:** Prettier (120 col, 2-space).

## Fix policy — TDD, root cause, in scope, no parking

- **TDD bug-fix workflow.** Reproduce the bug as a failing test first (`go test` for backend, `vitest` for frontend) before touching the implementation. The test must fail for the right reason — same error/assertion the user reported. Only then apply the fix and watch the same test go green. Skipping the failing-test step is not allowed even for "obvious" one-liners; if the bug can't be reasonably reproduced in a test, say so explicitly and explain why before patching.
- **Don't touch unrelated files.** A bug fix touches the producer of the bug, its test, and at most an in-scope drift directly under your cursor. Resist drive-by refactors, rename sweeps, formatter passes, dead-code cleanup, or unrelated test churn in the same change — they bury the actual fix in the diff and break bisect.
- **Fix root causes, not symptoms.** Don't guard at the call site to mask a producer that emits bad values — fix the producer. Don't re-normalize a field at multiple consumers — normalize once at the boundary. If the design is wrong, refactor the affected piece; don't route around it. A comment explaining why a workaround is needed usually means the underlying code should change.
- **Fix in-scope drift in the same change.** Stale docstring, lying CLAUDE.md line, dead reference, obvious one-line bug under your cursor → fix it now, don't TODO it.
- **Stay in scope.** Multi-day refactors / hot subsystems / design-discussion territory → flag and ask. Genuine out-of-scope workaround → isolate it in one place with a clear comment and surface it; don't normalize patching as the default.

## No meaningless fallbacks — don't pad code with "just in case"

Defensive code that handles cases that can't happen, swallows errors, or papers over upstream bugs becomes load-bearing noise: future readers can't tell what's real, and the actual bug stays hidden behind the guard.

- **Validate at boundaries, not between trusted layers.** `internal/app/*.go` parses IPC input from the frontend — that's a boundary, check it. A `service` calling its own `repository`, or a `repository` returning to its service, is not — don't `if x == nil` between them. Same for WASM host calls into `pkg/extension/host.go` (boundary) vs. Go-to-Go calls inside `internal/ai/` (trusted).
- **Don't double-default user-configurable fields.** `Icon` / `Type` / `Color` / `PolicyGroup` resolution already has a canonical helper (`getIconComponent` + `getIconColor`, `getAssetType`, `resolvePolicyGroup`) that handles the empty case. Adding `value || "default"` at the call site overrides the user's intentional empty value and hides bugs in the helper.
- **Don't swallow errors to keep the flow going.** `if err != nil { return nil }` / `catch { return defaultState }` masks the real failure and lets corrupted state propagate. Surface it (return the error, throw, log+abort the operation) — only catch when you have a concrete recovery for a specific error type.
- **No runtime compatibility shims for retired data.** Schema/data migrations live in `/migrations/` and run once; don't sprinkle `if legacyField != "" { ... }` at read sites "to be safe." If the field is gone, delete it from the model — no `_renamed` placeholders, no `// removed in v1.x` comments.
- **Treat "fallback" comments as a smell.** `// fallback for X`, `// just in case`, `// 防止 nil`, `// 兼容老数据` — ask whether X can actually happen. If yes, fix the producer. If no, delete the line. The comment is usually telling you the underlying code is wrong.
- **Don't catch panics to soldier on.** `recover()` belongs at goroutine boundaries that must not crash the app (e.g. extension WASM dispatch, AI tool execution) and must record the panic. Wrapping a normal function in `defer recover()` to "be robust" hides logic bugs.

## Reuse first — grep before writing new code

Parallel copies drift within weeks. Before writing any component / hook / util / Go helper, grep for the existing one.

Recurring smells:
- **Hand-rolled UI instead of the shared primitive.** `AssetSelect` / `AssetMultiSelect` / `GroupSelect`, `TreeSelect` / `TreeCheckList`, `ConfirmDialog`, `PasswordSourceField`, `IconPicker`, terminal panes, query result grid, tab system, shortcut store all exist — don't re-derive expand/collapse, tri-state checkboxes, search/pinyin, shortcuts, approval flows, or icon resolution.
- **Hardcoded defaults instead of the entity's own field.** Resolve `Icon` / `Type` / `Color` / policy group via the canonical helper (`getIconComponent` + `getIconColor`, `getAssetType`); fall back only when empty.
- **Inline filters / data loading.** Common filters (`Status === 1`, type filter, excludeIds, sort) belong in the shared hook (`useAssetStore`, `useAssetTree`, `useGroupTree`, `useShortcutStore`). New filter → hook option, not inline.
- **Re-implementing cross-cutting concerns.** Logging, audit, AI tool registration, approval, credential encryption, connection pools, i18n all have canonical entry points — don't spin up a second one.

Heuristics: importing a primitive (`lucide-react`, tree, Radix, `ConfirmDialog`, xterm) **and** an entity store from a new file usually means you're re-implementing a picker/pane/dialog. About to copy >10 lines? Extract. Same fix in two near-identical blocks? The second block is the bug — delete it, call the first.

## ⚠️ Generated / auto-managed files

| Path | Producer | Regenerate |
|------|----------|------------|
| `frontend/wailsjs/go/app/App.{d.ts,js}`, `models.ts` | Wails (from `internal/app/*.go` + Go structs) | `make dev` / `wails build` |
| `frontend/wailsjs/runtime/runtime.{js,d.ts}` | Wails runtime shim | shipped with Wails CLI |
| `internal/**/mock_*/` | `mockgen` | `go generate ./...` |
| `internal/embedded/opsctl_bin` | `make build-cli-embed` | `make build-embed` |
| `frontend/packages/devserver-ui/dist/` | Vite (embedded by `cmd/devserver`) | `make build-devserver-ui` |

Lockfiles (`go.sum`, `frontend/pnpm-lock.yaml`) — never hand-edit; use `go mod tidy` / `pnpm add|remove|install`.
