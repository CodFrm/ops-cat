# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

OpsKat is an AI-first desktop application for managing remote infrastructure (SSH, databases, Redis). Built with **Wails v2** (Go 1.25 backend + React 19 frontend). The desktop app communicates via Wails IPC — there is no HTTP API.

Module: `github.com/opskat/opskat`

## Common Commands

### Development
```bash
make dev              # Wails dev mode with hot reload
make install          # Install frontend deps (pnpm)
make run              # Run the embedded production build (no hot reload)
make clean            # Remove build/bin, frontend/dist, embedded opsctl, coverage files
```

### Build
```bash
make build            # Production build
make build-embed      # Production with embedded opsctl CLI
make build-cli        # Standalone opsctl CLI binary
make install-cli      # Install opsctl to GOPATH/bin
```

### Testing
```bash
make test                              # Go tests (internal, cmd/opsctl, cmd/devserver, pkg)
make test-cover                        # Coverage report → coverage.html, opens in browser
go test ./internal/ai/...             # Single package
go test ./internal/ai/ -run TestName  # Single test
cd frontend && pnpm test              # Frontend tests (vitest)
cd frontend && pnpm test:watch        # Frontend tests in watch mode
```

### Linting & Formatting
```bash
make lint             # golangci-lint (10m timeout, config in .golangci.yml)
make lint-fix         # golangci-lint with auto-fix
cd frontend && pnpm lint       # ESLint + Prettier
cd frontend && pnpm lint:fix   # ESLint auto-fix
```

### Extensions (DevServer)
```bash
make devserver EXT=<name>       # Run isolated dev server for one extension
                                # Builds extension in ../extensions, loads its WASM + manifest
make build-devserver-ui         # Rebuild embedded devserver UI (frontend/packages/devserver-ui)
```
DevServer refuses to start when `OPSKAT_ENV=production`. Extension source lives in a sibling repo at `../extensions/` (see reference memory).

### Plugin
```bash
make install-skill    # Register Codex opsctl plugin (symlinks plugin/ into ~/.Codex)
```

## Architecture

### Backend (Go) — Layered Architecture

```
main.go (Wails entry)
  └─ internal/app/        App struct — Wails binding layer, all public methods exposed to frontend via IPC
       ├─ internal/service/    Business logic (15 service packages: ssh_svc, sftp_svc, ai_provider_svc, extension_svc, etc.)
       ├─ internal/repository/ Data access (12 repos with interface + impl pattern)
       └─ internal/model/      Domain entities
```

**Key subsystems:**
- `internal/ai/` — AI agent: provider abstraction (Anthropic/OpenAI), tool registry, command policy checker, conversation runner, context compression, audit logging
- `internal/sshpool/` — SSH connection pool with Unix socket proxy for opsctl CLI
- `internal/connpool/` — Database/Redis tunnel management
- `internal/approval/` — Inter-process approval workflow (Unix socket between desktop app and opsctl)
- `internal/bootstrap/` — App initialization: database, credentials, migrations, auth tokens
- `internal/embedded/` — Embedded opsctl binary (build tag: `embed_opsctl`)
- `pkg/extension/` — WASM extension runtime (wazero): manifest parsing, plugin lifecycle, host bridge (I/O, KV, file dialogs, action events), policy evaluation
- `internal/service/extension_svc/` + `internal/app/app_extension.go` + `app_ext_host.go` — extension install/load/wiring into the desktop app
- `cmd/opsctl/` — Standalone CLI tool for remote operations, designed for AI assistant integration
- `cmd/devserver/` — Standalone HTTP dev server for a single extension (loads one WASM + manifest, proxies frontend HMR)

**Repository pattern:** Each repo has an interface, a default singleton, and `Register()`/getter functions.

**Database:** GORM + SQLite, migrations in `/migrations/` using gormigrate.

**Credential encryption:** Argon2id KDF + AES-256-GCM, master key in OS keychain.

**Extension system:** Extensions are WASM modules loaded at runtime. Each extension declares tools in `manifest.json`. AI invokes extension tools via a **single `exec_tool` tool** (not individual tools per extension) — the handler at `internal/ai/tool_handler_ext.go` dispatches by `extension` + `tool` args, enforces the extension's policy type against asset policy groups, and calls `Plugin.CallTool`. Host capabilities exposed to WASM are defined by `HostProvider` in `pkg/extension/host.go` (I/O open/read/write, KV, asset config, file dialogs, logging, events).

### Frontend (React + TypeScript)

Located in `frontend/` — a pnpm workspace monorepo. Root app consumes `@opskat/ui` (from `packages/ui`); `packages/devserver-ui` is embedded by `cmd/devserver`. Uses Vite 6 bundler, Tailwind CSS 4, shadcn/ui (Radix), Zustand 5 for state.

**No React Router** — uses a custom tab-based navigation system (`tabStore`). Tab types: terminal, ai, query, page, info.

**State stores** (`src/stores/`): One Zustand store per domain — assetStore, tabStore, terminalStore, aiStore, queryStore, sftpStore, shortcutStore, terminalThemeStore.

**Backend calls:** Generated Wails bindings in `frontend/wailsjs/`. Import from `wailsjs/go/app/App`. Real-time updates via `EventsOn()`.

**i18n:** i18next with `zh-CN` and `en` locales in `src/i18n/locales/`.

**Terminal:** xterm.js 6 with split-pane support.

**Tests:** Vitest + happy-dom + React Testing Library. Setup file mocks Wails runtime at `src/__tests__/setup.ts`.

### CI (`.github/workflows/ci.yml`)

Runs on PR and pushes to main/develop:
- Go: golangci-lint + `go test`
- Frontend: `pnpm lint` + `pnpm test` + `pnpm build`

### Git Commit Convention

Use **gitmoji** for commit messages. Common prefixes:
- ✨ New feature
- 🐛 Bug fix
- ♻️ Refactor
- 🎨 UI improvement
- ⚡️ Performance
- 🔒 Security
- 🔧 Configuration / tooling
- ✅ Tests
- 📄 Documentation
- 🚀 Deploy / release related

### Conventions

- Go mocks: generated with `go.uber.org/mock` in `mock_*/` subdirectories
- Go test assertions: goconvey + testify
- Frontend formatting: Prettier (120 char width, 2-space indent)
- Soft deletes via Status field (StatusActive=1, StatusDeleted=2), not GORM soft delete
- i18n keys namespaced under `"common"` — use `t("key.subkey")`
- Version info embedded via ldflags at build time

### Code smells to avoid — reuse first, SOLID always

**Before writing any new component, hook, util, or Go helper: grep the codebase first.** If similar behavior already exists, extend or wrap it — do NOT fork a parallel copy. Every parallel copy we've shipped has drifted from the canonical one (missing features, inconsistent UX, stale bugfixes) and turned into tech debt within weeks.

Concrete smells we've hit and keep hitting:

- **Parallel hand-rolled UI instead of the shared primitive.**
  If a shared component exists — `AssetSelect` / `AssetMultiSelect` / `GroupSelect`, `TreeSelect` / `TreeCheckList`, `ConfirmDialog`, `PasswordSourceField`, `IconPicker`, terminal panes, query result grid, drawer/dialog wrappers, tab system, shortcut store — use it. Don't re-derive expand/collapse state, tri-state checkboxes, search/pinyin, keyboard shortcuts, approval flows, or icon resolution in a leaf component.
- **Hardcoded defaults instead of reading the entity's own field.**
  When an entity carries a user-configured property (asset/group `Icon`, asset `Type`, `Color`, policy group, etc.), render/resolve it via the canonical helper (`getIconComponent` + `getIconColor`, `getAssetType`, etc.), falling back to a generic default only when the field is empty. Never slap a fixed constant over every row.
- **Duplicating filters, data loading, or derivations at call sites.**
  Common filters (`Status === 1`, type filter, excludeIds, sort order) and data access belong in the shared hook/store (`useAssetStore`, `useAssetTree`, `useGroupTree`, `useShortcutStore`, …) — leaf components consume, not re-fetch. If a caller needs a new filter, add it to the hook, don't inline a new one.
- **Fat Wails binding methods in `internal/app/*.go`.**
  Binding methods should be thin: parse args, call a service, return. Business rules live in `internal/service/`; persistence in `internal/repository/`. If you're writing a query, a policy check, or a retry loop inside `App`, push it down a layer — otherwise the logic becomes unreachable from tests and from `opsctl`.
- **Re-implementing cross-cutting concerns.**
  Logging, audit, AI tool registration, approval flow, credential encryption, connection pools, i18n keys — all have canonical entry points (`internal/ai/`, `internal/approval/`, `internal/sshpool/`, `internal/connpool/`, `src/i18n`). Don't spin up a second logger, a second approval socket, or a parallel i18n scheme.

SOLID reminders — the recurring regressions map back to these:

- **SRP** — one responsibility per unit. A picker renders a picker; it doesn't own filtering, data fetching, icon theming, and Wails calls all at once.
- **OCP** — extend the shared layer (add a prop / hook option / strategy) to absorb a new case; don't fork a parallel copy to add the feature.
- **LSP** — when swapping a hand-rolled widget for a shared one, preserve the caller contract (value type, empty state, default filters like `activeOnly`).
- **ISP** — keep prop and interface surfaces minimal. Prefer option-object props (`UseAssetTreeOptions`-style) over ten booleans; don't leak every internal knob.
- **DIP** — UI depends on hooks/stores; services depend on repo interfaces; binding methods depend on services. Never skip layers to "save a line."

Rules of thumb:

- If a new file imports a primitive (`lucide-react`, tree component, Radix primitive, `ConfirmDialog`, xterm, etc.) **and** an entity list/store, you are probably re-implementing a picker/pane/dialog that already exists — stop and grep first.
- If you're about to copy–paste more than ~10 lines from another file, that's the signal to extract, not copy.
- If a fix has to be applied to two near-identical blocks, the second block is the bug — delete it, call the first one.

### ⚠️ Generated / auto-managed files — DO NOT edit by hand

These files are produced by tools and will be overwritten. Change the source instead, then regenerate.

| Path | Producer | How to regenerate |
|------|----------|-------------------|
| `frontend/wailsjs/go/app/App.d.ts` | Wails (from `internal/app/*.go` exported methods on `App`) | `make dev` / `wails build` |
| `frontend/wailsjs/go/app/App.js`   | Wails (same source) | `make dev` / `wails build` |
| `frontend/wailsjs/go/models.ts`    | Wails (from Go structs returned/accepted by App methods) | `make dev` / `wails build` |
| `frontend/wailsjs/runtime/runtime.js`, `runtime.d.ts` | Wails runtime shim | shipped with Wails CLI |
| `internal/**/mock_*/` (e.g. `mock_asset_repo/asset.go`) | `mockgen` (`go.uber.org/mock`) — header `// Code generated by MockGen. DO NOT EDIT.` | `go generate ./...` against the matching `//go:generate` directive on the source repo interface |
| `internal/embedded/opsctl_bin` | `make build-cli-embed` | `make build-embed` rebuilds it |
| `frontend/packages/devserver-ui/dist/` | Vite build, embedded into `cmd/devserver` via `embed.go` | `make build-devserver-ui` |

Build artifacts and caches (gitignored, safe to delete via `make clean`): `build/bin/`, `frontend/dist/`, `coverage.out`, `coverage.html`, `coverage_new.out`, `tsconfig.tsbuildinfo`, `package.json.md5`, the top-level `opskat`, `opsctl`, `devserver` binaries.

Lockfiles — never hand-edit; modify via the package manager: `go.sum` (use `go mod tidy`), `frontend/pnpm-lock.yaml` (use `pnpm add/remove/install`).
