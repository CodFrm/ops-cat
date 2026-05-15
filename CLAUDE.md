# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpsKat — AI-first desktop app for managing remote infra (SSH, MySQL/PostgreSQL, Redis, MongoDB, Kafka, K8s). **Wails v2** (Go 1.25 + React 19), IPC only — no HTTP API. Module: `github.com/opskat/opskat`.

`AGENTS.md` is the Codex twin of this file — keep both in sync.

## Common Commands

```bash
# Dev / build
make dev                                 # Wails hot-reload
make build / make build-embed            # Production (embed = bundled opsctl)
make build-cli                           # Standalone opsctl

# Test
make test                                # All Go tests
go test ./internal/ai/ -run TestName     # Single Go test
cd frontend && pnpm test                 # Frontend (vitest)
make test-fixtures && make test-e2e      # E2E (needs ../extensions sibling)
make test-cover                          # Coverage HTML

# Lint
make lint / make lint-fix                # golangci-lint
cd frontend && pnpm lint / pnpm lint:fix

# Extensions
make devserver EXT=<name>                # Single-extension dev (refuses if OPSKAT_ENV=production)
```

## Architecture

**Backend layers** — bindings stay thin: parse → service → return. Business rules in `service/`, persistence in `repository/`. Logic inside `App` is unreachable from tests and `opsctl`.

```
main.go → internal/app/ (Wails bindings, IPC boundary)
            → internal/service/    (*_svc, business logic)
            → internal/repository/ (interface + impl)
            → internal/model/      (entities)
```

**Key subsystems:**
- `internal/ai/` — provider abstraction (Anthropic/OpenAI), tool registry, conversation runner, audit. Per-protocol policy checkers: `command_policy.go`, `k8s_policy.go`, `kafka_policy.go`, mongo/redis equivalents.
- `internal/assettype/` — per-asset adapters (ssh/db/redis/mongo/kafka/k8s/serial) wired through `registry.go`. New asset types plug in here, not by hardcoding type strings.
- `internal/sshpool/`, `internal/connpool/` — SSH pool (Unix socket proxy for opsctl); DB/Redis tunnels.
- `internal/approval/` — Unix-socket approval flow between desktop app and opsctl.
- `internal/bootstrap/` — DB, credentials, migrations, auth tokens.
- `pkg/extension/` — WASM runtime (wazero); `HostProvider` in `host.go` defines capabilities.
- `cmd/opsctl/`, `cmd/devserver/` — standalone CLI / single-extension dev server.
- `plugin/` — Claude Code plugin marketplace; installed via `make install-skill`.

**Data:** GORM + SQLite, gormigrate migrations in `/migrations/`. Soft deletes via `Status` (`StatusActive=1`, `StatusDeleted=2`), **not** GORM soft delete. Credentials: Argon2id + AES-256-GCM, master key in OS keychain.

**Extensions:** WASM modules with `manifest.json`-declared tools. AI invokes them via a **single `exec_tool`** (not one tool per extension). Dispatcher in `internal/ai/tool_handler_ext.go` enforces extension policy type against asset policy groups before `Plugin.CallTool`.

**Frontend** (`frontend/`, pnpm workspace): root app uses `@opskat/ui` (`packages/ui`); `packages/devserver-ui` is embedded by `cmd/devserver`. Vite 6, Tailwind 4, shadcn/ui (Radix), Zustand 5.
- **No React Router** — custom tabs in `tabStore` (`terminal | ai | query | page | info`). One Zustand store per domain in `src/stores/`.
- Backend via Wails bindings (`frontend/wailsjs/go/app/App`); events via `EventsOn()`.
- i18n: i18next, locales in `src/i18n/locales/{zh-CN,en}/common.json`, all keys under `"common"` → `t("key.subkey")`.
- Tests: Vitest + happy-dom + RTL; Wails runtime mocked in `src/__tests__/setup.ts`.

## Conventions

- **Commits:** gitmoji (✨ feature, 🐛 fix, ♻️ refactor, 🎨 UI, ⚡️ perf, 🔒 security, 🔧 config, ✅ tests, 📄 docs, 🚀 release).
- **Go:** mocks in `mock_*/` (`go.uber.org/mock`, regen `go generate ./...`); tests use goconvey + testify.
- **Frontend:** Prettier 120 col, 2-space.

## Fix policy — TDD, root cause, in scope

- **Reproduce as a failing test first** (`go test` / `vitest`) before touching impl, failing for the right reason (same error/assertion the user reported). If a test isn't reasonable, say so explicitly. No exceptions for "obvious" one-liners.
- **Stay in scope.** A fix touches the producer, its test, and at most an in-scope drift under the cursor (stale docstring, lying CLAUDE.md line, obvious one-liner) — fix those *now*, don't TODO. No drive-by refactors / rename sweeps / formatter passes / dead-code cleanup in the same change. Multi-day refactors or hot-subsystem rework → flag and ask.
- **Fix root causes.** Don't guard at the call site to mask a bad producer; fix the producer. Don't re-normalize a field at multiple consumers; normalize once at the boundary. A "why this workaround" comment usually means the underlying code should change instead.

## No meaningless fallbacks

Defensive code for cases that can't happen, swallowed errors, or shims for retired data become load-bearing noise — future readers can't tell what's real, and the bug stays hidden behind the guard.

- **Validate at boundaries only.** IPC into `internal/app/*.go` and WASM host calls in `pkg/extension/host.go` are boundaries — check them. Go-to-Go between `service`/`repository`/`internal/ai/` is trusted — no `if x == nil` between them.
- **Don't double-default user-configurable fields.** `Icon` / `Type` / `Color` / `PolicyGroup` already have canonical helpers (`getIconComponent` + `getIconColor`, `getAssetType`, `resolvePolicyGroup`). `value || "default"` at the call site overrides the user's intentional empty value and hides bugs in the helper.
- **Don't swallow errors.** `if err != nil { return nil }` / `catch { return defaultState }` masks failure and propagates corrupt state. Surface it; only catch when there's a concrete recovery for a specific error type.
- **No runtime shims for retired data.** Migrations in `/migrations/` run once. Don't sprinkle `if legacyField != "" { ... }`, `_renamed` placeholders, or `// removed in v1.x` comments — delete the field from the model.
- **"Fallback" comments are a smell.** `// just in case` / `// 防止 nil` / `// 兼容老数据` — if X can happen, fix the producer; if not, delete the line. `recover()` is only for goroutine boundaries that must not crash the app (extension WASM dispatch, AI tool execution) and must record the panic.

## Reuse first — grep before writing

Parallel copies drift within weeks. Before any new component/hook/util/Go helper, grep for the existing one.

- **Shared UI primitives** exist: `AssetSelect` / `AssetMultiSelect` / `GroupSelect`, `TreeSelect` / `TreeCheckList`, `ConfirmDialog`, `PasswordSourceField`, `IconPicker`, terminal panes, query result grid, tab system, shortcut store. Don't re-derive expand/collapse, tri-state checkboxes, search/pinyin, shortcuts, approval flows, or icon resolution.
- **Shared filters/loading** belong in `useAssetStore` / `useAssetTree` / `useGroupTree` / `useShortcutStore`. New filter → hook option, not inline.
- **Cross-cutting concerns** (logging, audit, AI tool registration, approval, credential encryption, connection pools, i18n) have canonical entry points — don't spin up a second one.

Heuristics: importing a primitive (`lucide-react`, tree, Radix, `ConfirmDialog`, xterm) **and** an entity store from a new file usually means you're re-implementing a picker/pane/dialog. Copying >10 lines → extract. Same fix in two near-identical blocks → the second is the bug; delete it, call the first.

## ⚠️ Generated / auto-managed files

| Path | Producer | Regenerate |
|------|----------|------------|
| `frontend/wailsjs/go/app/App.{d.ts,js}`, `models.ts` | Wails (from `internal/app/*.go` + Go structs) | `make dev` / `wails build` |
| `frontend/wailsjs/runtime/*` | Wails runtime shim | ships with Wails CLI |
| `internal/**/mock_*/` | `mockgen` | `go generate ./...` |
| `internal/embedded/opsctl_bin` | `make build-cli-embed` | `make build-embed` |
| `frontend/packages/devserver-ui/dist/` | Vite (embedded by `cmd/devserver`) | `make build-devserver-ui` |

Lockfiles (`go.sum`, `frontend/pnpm-lock.yaml`) — never hand-edit; use `go mod tidy` / `pnpm add|remove|install`.
