# Redis Browser Upgrade Design

## Background

OpsKat already has a Redis asset type, a key browser, a key detail pane, command execution, SSH tunnel support, credential resolution, AI/opsctl Redis policy checks, and audit infrastructure. The current GUI Redis path is still thin: most operations are assembled in the frontend and executed through generic `ExecuteRedis` / `ExecuteRedisArgs` bindings.

This change upgrades Redis management as an OpsKat-native feature while using mature Redis client workflows as feature coverage and UX references. OpsKat keeps its existing asset model, credential handling, Wails IPC, Zustand stores, shared UI primitives, and service-layer conventions.

## Scope

Included:

- Typed Redis service APIs for key scanning, key summary/detail, key creation, renaming, deletion, TTL updates, value mutations, slowlog, client list, command history, monitor, and pub/sub.
- Redis browser improvements: dynamic database list from keyspace info, configurable scan page size, type filter, exact key lookup, key separator, list/tree view, and stable cursor paging.
- Key operations: create, rename, single delete, batch delete, pattern delete with confirmation, TTL/PERSIST, refresh, copy, and per-type CRUD for string/hash/list/set/zset/stream.
- Value viewing: raw text, formatted JSON, hex, and base64 views. Save semantics must be explicit: formatted display does not silently change stored bytes.
- Operational panels: Slowlog, Client List, command history, Pub/Sub, and Monitor.
- Connection enhancements: default DB, scan page size, key separator, command timeout, TLS certificate options, SNI, and insecure TLS toggle.

Excluded from this implementation:

- Redis Cluster.
- Redis Sentinel.
- HTTP/SOCKS proxy.
- Unix socket.
- Full import/export workflows.
- Custom decoder commands and non-core codecs such as gzip/zstd/lz4/msgpack/php/pickle.

These exclusions avoid replacing `*redis.Client` with `redis.UniversalClient` in this version and keep database semantics, multi-key commands, and SSH tunnel behavior stable.

## Architecture

### Backend

Add `internal/service/redis_svc` as the Redis business layer. Wails methods in `internal/app` remain thin: validate/parse input, call `redis_svc`, marshal responses, and return errors. Shared connection setup remains in `internal/connpool`.

The service owns Redis-specific behavior:

- Resolve assets and credentials.
- Dial Redis with the existing SSH pool.
- Apply operation timeouts.
- Use argument arrays rather than shell-like splitting for any operation with user values.
- Return typed DTOs instead of generic command JSON where possible.
- Validate dangerous pattern operations before execution.

Existing AI/opsctl command execution can keep using `internal/ai.ExecuteRedisRaw`. The GUI service does not change AI tool behavior.

### Frontend

Keep the existing query tab architecture. Redis state remains in `frontend/src/stores/queryStore.ts`, but Redis behavior is split into small helpers and typed Wails calls instead of constructing most commands in components.

UI changes stay inside the current Redis query surface:

- Left pane: database selector, type filter, exact lookup, search pattern, list/tree toggle, refresh, load more.
- Right pane: details header, value viewer/editor, type-specific collection table, stream viewer, and bottom command input.
- Additional Redis operational tabs or sections live inside the Redis panel, not as new app-level routes.

Shared components from `@opskat/ui` are preferred. Destructive actions use `ConfirmDialog`.

### Data Flow

1. User opens a Redis asset query tab.
2. `queryStore` initializes Redis tab state from asset config defaults.
3. Browser calls typed backend methods such as `RedisScanKeys` and `RedisListDatabases`.
4. Selecting a key calls `RedisGetKeyDetail`, which returns summary and first-page values.
5. Mutations call specific methods such as `RedisSetStringValue`, `RedisHashSet`, `RedisRenameKey`, or `RedisSetKeyTTL`.
6. The store refreshes affected key/detail state after successful mutation.
7. Monitor and Pub/Sub use Wails events with explicit start/stop bindings and cancelable backend contexts.

## Safety

- All value-writing operations use argument arrays to preserve spaces and binary-like text.
- Pattern delete requires an explicit non-empty pattern that is not `*`, plus a frontend confirmation.
- `FLUSHDB`, `FLUSHALL`, `CONFIG SET`, `DEBUG`, `SHUTDOWN`, and similar broad commands are not added as GUI affordances.
- TLS insecure mode is explicit and stored as a connection option.
- Monitor and Pub/Sub sessions are cancelable and cleaned up when stopped or when the app context ends.
- Backend APIs return bounded pages for keys and values to avoid loading large databases into memory by default.

## Testing

Backend tests cover service helpers, DTO parsing, paging argument generation, value encoding helpers, dangerous pattern validation, and command-history behavior with a fake Redis executor where practical.

Frontend tests cover store behavior and components for key filtering, type filters, exact lookup state, destructive confirmations, value format switching, and operation panel state transitions. Existing Redis tests remain valid.

Manual verification covers opening a Redis tab, scanning keys, selecting values for each type, editing values, changing TTL, deleting keys, viewing slowlog/client list, starting/stopping monitor, using pub/sub, and running the existing command input.
