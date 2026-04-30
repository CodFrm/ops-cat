# Redis Browser Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade OpsKat Redis management into a typed, safer, fuller browser/editor experience without adding Cluster or Sentinel support in this version.

**Architecture:** Keep Wails bindings thin and move Redis business behavior into `internal/service/redis_svc`. Preserve the current React query tab model and incrementally replace frontend command assembly with typed backend calls.

**Tech Stack:** Go 1.25, `github.com/redis/go-redis/v9`, Wails v2 IPC, React 19, Zustand, Vitest, goconvey/testify.

---

## File Structure

- Create `internal/service/redis_svc/service.go`: asset resolution, connection lifecycle, typed Redis operations.
- Create `internal/service/redis_svc/types.go`: request/response DTOs for Wails and tests.
- Create `internal/service/redis_svc/value_format.go`: raw/json/hex/base64 display and save helpers.
- Create `internal/service/redis_svc/session.go`: monitor/pubsub session registry and command history.
- Create `internal/service/redis_svc/*_test.go`: backend unit tests.
- Modify `internal/model/entity/asset_entity/asset.go`: extend `RedisConfig` with non-breaking optional connection/browser settings.
- Modify `internal/connpool/redis.go`: apply TLS cert/SNI/insecure and timeout settings from `RedisConfig`.
- Modify `internal/app/app_query.go`: add thin Redis typed Wails bindings and delegate existing Redis execution to the service where useful.
- Regenerate `frontend/wailsjs/go/app/App.{d.ts,js}` and `frontend/wailsjs/go/models.ts` using Wails generation.
- Modify `frontend/src/stores/queryStore.ts`: use typed Redis APIs and add browser/operation state.
- Modify `frontend/src/components/asset/RedisConfigSection.tsx`: expose new Redis config fields.
- Modify `frontend/src/components/query/RedisPanel.tsx`: add Redis browser/ops layout.
- Modify `frontend/src/components/query/RedisKeyBrowser.tsx`: add type filter, exact lookup, dynamic DB list, and batch actions.
- Modify `frontend/src/components/query/RedisKeyDetail.tsx`: add create/rename/TTL/delete controls and operation tabs.
- Modify `frontend/src/components/query/RedisStringEditor.tsx`: add raw/json/hex/base64 view modes.
- Modify `frontend/src/components/query/RedisCollectionTable.tsx`: use typed mutation APIs and complete stream support through companion component.
- Modify `frontend/src/components/query/RedisStreamViewer.tsx`: support stream add/delete and paging.
- Add `frontend/src/components/query/RedisOpsPanel.tsx`: slowlog, clients, history, monitor, pub/sub.
- Modify `frontend/src/i18n/locales/{zh-CN,en}/common.json`: add labels and confirmations.
- Add or update frontend tests under `frontend/src/__tests__/`.

## Tasks

### Task 1: Backend DTOs and value format helpers

- [ ] Write failing tests in `internal/service/redis_svc/value_format_test.go` for JSON formatting, invalid JSON fallback, hex encoding, base64 encoding, and save pass-through semantics.
- [ ] Add `internal/service/redis_svc/types.go` and `value_format.go`.
- [ ] Run `go test ./internal/service/redis_svc -run TestRedisValueFormat -count=1`.

### Task 2: Backend Redis service core

- [ ] Write failing tests for database keyspace parsing, key scan option validation, dangerous pattern delete validation, and result conversion.
- [ ] Add `service.go` with typed methods for list databases, scan keys, key summary/detail, TTL, rename, delete, and per-type mutations.
- [ ] Keep Wails-unfriendly logic out of `internal/app`.
- [ ] Run `go test ./internal/service/redis_svc -count=1`.

### Task 3: Connection config enhancements

- [ ] Write failing tests for `RedisConfig` JSON compatibility and TLS option mapping.
- [ ] Extend `RedisConfig` with `command_timeout_seconds`, `scan_page_size`, `key_separator`, `tls_insecure`, `tls_server_name`, `tls_ca`, `tls_cert`, and `tls_key`.
- [ ] Update `connpool.DialRedis` to apply timeout and TLS settings without changing existing direct/SSH behavior.
- [ ] Run `go test ./internal/model/entity/asset_entity ./internal/connpool -count=1`.

### Task 4: Wails bindings and event sessions

- [ ] Add tests for monitor/pubsub session registry lifecycle where practical.
- [ ] Add thin `App.Redis*` binding methods for typed operations and start/stop event sessions.
- [ ] Regenerate Wails frontend bindings using the project-supported Wails generation path.
- [ ] Run `go test ./internal/app ./internal/service/redis_svc -count=1`.

### Task 5: Store migration

- [ ] Write or update Vitest coverage for Redis store initialization, scan keys, select key, mutations, and ops state.
- [ ] Update `queryStore.ts` to use typed Redis APIs for browser/detail/mutations while keeping command input compatible.
- [ ] Run `cd frontend && pnpm test -- queryStore`.

### Task 6: Browser and detail UI

- [ ] Update Redis browser UI for dynamic DB list, type filter, exact lookup, key separator tree mode, refresh, load more, context actions, and batch delete.
- [ ] Update key detail UI for summary fields, rename, create key entry point, TTL/PERSIST, delete, typed refresh, and value mode controls.
- [ ] Add i18n strings in English and Chinese.
- [ ] Run targeted component tests.

### Task 7: Value editors and collection CRUD

- [ ] Update string editor for raw/json/hex/base64 views and safe save semantics.
- [ ] Update collection table to use typed hash/list/set/zset mutation APIs.
- [ ] Update stream viewer to add/delete entries and page safely.
- [ ] Run targeted component tests.

### Task 8: Redis operations panel

- [ ] Add `RedisOpsPanel.tsx` for Slowlog, Client List, command history, Monitor, and Pub/Sub.
- [ ] Wire Wails events with start/stop cleanup on unmount.
- [ ] Add tests for panel state and stop cleanup.
- [ ] Run targeted component tests.

### Task 9: Full verification and push

- [ ] Run `go test ./internal/... ./cmd/opsctl/... ./pkg/...`.
- [ ] Run `cd frontend && pnpm test`.
- [ ] Run `cd frontend && pnpm build`.
- [ ] Run `git status --short`.
- [ ] Push `feature/redis-browser-upgrade` to `origin`.

## Self-Review

The plan covers the approved A scope and explicitly excludes Cluster/Sentinel/proxy/import-export/full decoder support. It keeps business logic out of Wails bindings and preserves OpsKat conventions around service layering, shared UI, i18n, and test-first implementation.
