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

## Fix policy — root cause, in scope, no parking

- **Fix root causes, not symptoms.** Don't guard at the call site to mask a producer that emits bad values — fix the producer. Don't re-normalize a field at multiple consumers — normalize once at the boundary. If the design is wrong, refactor the affected piece; don't route around it. A comment explaining why a workaround is needed usually means the underlying code should change.
- **Fix in-scope drift in the same change.** Stale docstring, lying CLAUDE.md line, dead reference, obvious one-line bug under your cursor → fix it now, don't TODO it.
- **Stay in scope.** Multi-day refactors / hot subsystems / design-discussion territory → flag and ask. Genuine out-of-scope workaround → isolate it in one place with a clear comment and surface it; don't normalize patching as the default.

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
