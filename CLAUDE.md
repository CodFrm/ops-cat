# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpsKat is an AI-first desktop application for managing remote infrastructure (SSH, databases, Redis). Built with **Wails v2** (Go 1.25 backend + React 19 frontend). The desktop app communicates via Wails IPC — there is no HTTP API.

Module: `github.com/opskat/opskat`

## Common Commands

```bash
# Development
make dev              # Wails dev mode with hot reload
make install          # Install frontend deps (pnpm)
make run              # Run embedded production build (no hot reload)
make clean            # Remove build/bin, frontend/dist, embedded opsctl, coverage, root-level binaries

# Build
make build            # Production build
make build-embed      # Production with embedded opsctl CLI
make build-cli        # Standalone opsctl CLI binary
make install-cli      # Install opsctl to GOPATH/bin

# Testing
make test                              # Go tests (internal, cmd/opsctl, cmd/devserver, pkg)
make test-cover                        # Coverage HTML → opens in browser
make test-fixtures && make test-e2e    # E2E (needs ../extensions sibling repo)
go test ./internal/ai/ -run TestName   # Single test
cd frontend && pnpm test               # Frontend tests (vitest)

# Lint
make lint / make lint-fix              # golangci-lint (10m timeout, .golangci.yml)
cd frontend && pnpm lint / pnpm lint:fix

# Extensions DevServer (refuses to run when OPSKAT_ENV=production)
make devserver EXT=<name>     # Builds extension in ../extensions, loads its WASM + manifest
make build-devserver-ui       # Rebuild embedded devserver UI

# Plugin
make install-skill            # Register Claude Code opsctl plugin (symlinks plugin/ into ~/.claude)
```

## Architecture

### Backend (Go) — Layered

```
main.go (Wails entry)
  └─ internal/app/        Wails binding layer (App struct — public methods exposed to frontend via IPC)
       ├─ internal/service/    Business logic (15 *_svc packages)
       ├─ internal/repository/ Data access (13 *_repo, interface + impl pattern)
       └─ internal/model/      Domain entities
```

**Key subsystems:**
- `internal/ai/` — AI agent: provider abstraction (Anthropic/OpenAI), tool registry, command policy, conversation runner, context compression, audit logging
- `internal/sshpool/` — SSH connection pool with Unix socket proxy for opsctl
- `internal/connpool/` — Database/Redis tunnel management
- `internal/approval/` — Inter-process approval workflow (Unix socket between desktop app and opsctl)
- `internal/bootstrap/` — App init: database, credentials, migrations, auth tokens
- `internal/embedded/` — Embedded opsctl binary (build tag: `embed_opsctl`)
- `pkg/extension/` — WASM extension runtime (wazero); `HostProvider` in `pkg/extension/host.go` defines capabilities exposed to WASM (I/O, KV, asset config, file dialogs, logging, events)
- `cmd/opsctl/` — Standalone CLI for remote ops, designed for AI assistant integration
- `cmd/devserver/` — Standalone HTTP dev server for a single extension

**Repos:** Each has interface + default singleton + `Register()`/getter. **Database:** GORM + SQLite, gormigrate migrations in `/migrations/`. **Credentials:** Argon2id KDF + AES-256-GCM, master key in OS keychain.

**Extensions:** WASM modules loaded at runtime; tools declared in `manifest.json`. AI invokes them via a **single `exec_tool`** (not individual tools per extension). Handler `internal/ai/tool_handler_ext.go` dispatches by `extension` + `tool` args, enforces extension policy type against asset policy groups, then calls `Plugin.CallTool`.

### Frontend (React + TypeScript)

`frontend/` is a pnpm workspace. Root app consumes `@opskat/ui` (`packages/ui`); `packages/devserver-ui` is embedded by `cmd/devserver`. Vite 6, Tailwind 4, shadcn/ui (Radix), Zustand 5.

- **No React Router** — custom tab-based navigation in `tabStore`. Tab types: `terminal | ai | query | page | info`.
- **State:** one Zustand store per domain in `src/stores/`.
- **Backend calls:** Wails-generated bindings from `frontend/wailsjs/go/app/App`; real-time via `EventsOn()`.
- **i18n:** i18next, locales in `src/i18n/locales/{zh-CN,en}/common.json`; keys namespaced under `"common"`, use `t("key.subkey")`.
- **Terminal:** xterm.js 6 with split-pane.
- **Tests:** Vitest + happy-dom + RTL; Wails runtime mocked in `src/__tests__/setup.ts`.

### CI (`.github/workflows/ci.yml`)

Triggers on PR + push to `main` / `develop/*`. Jobs: Go lint (golangci-lint) + Go test, frontend lint + test + build.

## Conventions

- **Commits:** gitmoji prefixes — ✨ feature, 🐛 fix, ♻️ refactor, 🎨 UI, ⚡️ perf, 🔒 security, 🔧 config, ✅ tests, 📄 docs, 🚀 release.
- **Go mocks:** `go.uber.org/mock` in `mock_*/` subdirs; regenerate via `go generate ./...`.
- **Go test assertions:** goconvey + testify.
- **Frontend formatting:** Prettier (120 char width, 2-space indent).
- **Soft deletes:** `Status` field (`StatusActive=1`, `StatusDeleted=2`), not GORM soft delete.
- **Version info:** embedded via ldflags at build time.

## Fix-now policy — don't park tech debt

When you find a real, in-scope problem while working on something else (Makefile target drifted from its docstring, CLAUDE.md line lying about the code, dead reference, obvious one-line bug under your cursor) — fix it in the same change. Don't open a TODO, don't ship docs that lie about the code.

Bar: "broken thing under my hands, fix is small and obvious". Multi-day refactors / hot-subsystem changes / things needing design discussion → call out and ask, don't silently expand scope.

## Code smells — reuse first

**Before writing any new component, hook, util, or Go helper: grep first.** Parallel copies drift within weeks (missing features, inconsistent UX, stale fixes).

Recurring smells in this repo:

- **Hand-rolled UI instead of the shared primitive.** Use `AssetSelect` / `AssetMultiSelect` / `GroupSelect`, `TreeSelect` / `TreeCheckList`, `ConfirmDialog`, `PasswordSourceField`, `IconPicker`, the terminal panes, query result grid, tab system, shortcut store. Don't re-derive expand/collapse, tri-state checkboxes, search/pinyin, keyboard shortcuts, approval flows, or icon resolution in a leaf component.
- **Hardcoded defaults instead of the entity's own field.** When an entity carries a configured property (asset/group `Icon`, asset `Type`, `Color`, policy group), resolve via the canonical helper (`getIconComponent` + `getIconColor`, `getAssetType`); fall back only when empty.
- **Duplicating filters / data loading at call sites.** Common filters (`Status === 1`, type filter, excludeIds, sort) and data access belong in the shared hook/store (`useAssetStore`, `useAssetTree`, `useGroupTree`, `useShortcutStore`). New filter? Add a hook option, don't inline.
- **Fat Wails binding methods in `internal/app/*.go`.** Bindings should be thin: parse args → call service → return. Business rules in `internal/service/`; persistence in `internal/repository/`. Logic inside `App` becomes unreachable from tests and from `opsctl`.
- **Re-implementing cross-cutting concerns.** Logging, audit, AI tool registration, approval, credential encryption, connection pools, i18n keys all have canonical entry points (`internal/ai/`, `internal/approval/`, `internal/sshpool/`, `internal/connpool/`, `src/i18n`). Don't spin up a second one.

Heuristics:

- A new file importing a primitive (`lucide-react`, tree component, Radix, `ConfirmDialog`, xterm) **and** an entity store usually means you're re-implementing a picker/pane/dialog — grep first.
- About to copy–paste >10 lines? Extract instead.
- A fix has to be applied to two near-identical blocks → the second block is the bug; delete it, call the first.

## ⚠️ Generated / auto-managed files — do not edit by hand

| Path | Producer | Regenerate |
|------|----------|------------|
| `frontend/wailsjs/go/app/App.{d.ts,js}`, `models.ts` | Wails (from `internal/app/*.go` `App` methods + Go structs) | `make dev` / `wails build` |
| `frontend/wailsjs/runtime/runtime.{js,d.ts}` | Wails runtime shim | shipped with Wails CLI |
| `internal/**/mock_*/` (e.g. `mock_asset_repo/asset.go`) | `mockgen` (`go.uber.org/mock`) | `go generate ./...` against the matching `//go:generate` directive |
| `internal/embedded/opsctl_bin` | `make build-cli-embed` | `make build-embed` |
| `frontend/packages/devserver-ui/dist/` | Vite build, embedded into `cmd/devserver` via `embed.go` | `make build-devserver-ui` |

Build artifacts (gitignored, removed by `make clean`): `build/bin/`, `frontend/dist/`, `coverage.out`, `coverage.html`, `coverage_new.out`, `internal/embedded/opsctl_bin`, `frontend/package.json.md5`, top-level `opskat`/`opsctl`/`devserver` binaries.

Lockfiles — never hand-edit; use the package manager: `go.sum` (`go mod tidy`), `frontend/pnpm-lock.yaml` (`pnpm add/remove/install`).
