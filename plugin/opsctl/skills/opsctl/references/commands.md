# opsctl Command Reference

## list

### `list assets [flags]`

List managed server assets. Does not include description — use `get asset` to view descriptions.

**Flags**:
- `--type <string>` — Filter by asset type (e.g., "ssh")
- `--group-id <int>` — Filter by group ID (0 = all)

```bash
opsctl list assets
opsctl list assets --type ssh --group-id 2
```

### `list groups`

List all asset groups. Does not include description — use `get group` to view descriptions.

```bash
opsctl list groups
```

## get

### `get asset <asset>`

Get asset details including description and SSH config (host, port, username, auth method).

```bash
opsctl get asset web-server
opsctl get asset 1
opsctl get asset production/web-01
```

### `get group <group>`

Get group details including description.

```bash
opsctl get group 1
opsctl get group production
```

## ssh

### `ssh <asset>`

Open interactive SSH terminal. No approval needed (human use).

- Full terminal emulation (xterm-256color)
- Terminal resize via SIGWINCH
- Exit code propagation

```bash
opsctl ssh web-server
```

## exec

### `exec <asset> [--] <command>`

Execute remote command via SSH with stdio piping.

**Behavior**:
- If stdin is piped (not a terminal), data forwards to remote stdin
- stdout/stderr pass through directly
- Remote exit code propagated as opsctl exit code

**Approval flow**:
1. Command policy check (allow-list/deny-list per asset)
2. Session check (grant item consumption or session auto-approve)
3. Desktop app approval (blocks until response)

```bash
opsctl exec web-server -- uptime
opsctl exec 1 -- ls -la /var/log
echo "data" | opsctl exec web-server -- cat
opsctl exec web-01 -- systemctl restart nginx
```

## sql

### `sql <asset> [flags] "<SQL>"`

Execute SQL on a database asset (MySQL, PostgreSQL). Requires approval.

**Flags**:
- `-f <file>` — Read SQL from file instead of argument
- `-d <database>` — Override the default database for this execution

```bash
opsctl sql prod-db "SELECT * FROM users LIMIT 10"
opsctl sql prod-db -f migration.sql
opsctl sql prod-db -d other_db "SHOW TABLES"
```

## redis

### `redis <asset> [flags] "<command>"`

Execute a Redis command on a Redis asset. Requires approval.

**Flags**:
- `-n <db>` — Override the default database number (0-15)

```bash
opsctl redis cache "GET session:abc123"
opsctl redis cache "HGETALL user:1"
opsctl redis cache -n 2 "KEYS user:*"
opsctl redis cache "SET key value EX 3600"
```

## mongo

### `mongo <asset> [flags] [query_json]`

Execute MongoDB operations on a MongoDB asset. Requires approval.

**Flags**:
- `-o <operation>` — Operation type (default: find): find, findOne, insertOne, insertMany, updateOne, updateMany, deleteOne, deleteMany, aggregate, countDocuments
- `-d <database>` — Database name (required)
- `-c <collection>` — Collection name (required)

**Examples**:

```bash
opsctl mongo prod-db -d mydb -c users -o find '{"filter":{"status":"active"}}'
opsctl mongo prod-db -d mydb -c logs -o aggregate '{"pipeline":[{"$match":{"level":"error"}}]}'
opsctl mongo prod-db -d mydb -c users -o countDocuments '{"filter":{}}'
```

## batch

### `batch [args...]`

Execute multiple commands in parallel with a single approval request. Supports exec (SSH), sql, redis, and mongo types in a single batch.

**Input modes**:

1. **Stdin JSON** (AI-friendly — primary mode):
```bash
echo '{"commands":[
  {"asset":"web-01","type":"exec","command":"uptime"},
  {"asset":"db-01","type":"sql","command":"SELECT 1"},
  {"asset":"cache","type":"redis","command":"PING"},
  {"asset":"mongo-db","type":"mongo","command":"{\"operation\":\"find\",\"database\":\"mydb\",\"collection\":\"users\",\"query\":\"{}\"}"}
]}' | opsctl batch
```

2. **Positional args**:
```bash
# Default type=exec
opsctl batch 'web-01:uptime' 'db-01:hostname'
# With type prefix (type:asset:command)
opsctl batch 'sql:db-01:SELECT 1' 'redis:cache:PING' 'web-01:uptime'
```

**Args format**: `asset:command` (default exec) or `type:asset:command`. First `:` before a known type (`exec`/`sql`/`redis`/`mongo`) is the type separator.

**Output**: JSON with per-command results:
```json
{
  "results": [
    {"asset_id":1,"asset_name":"web-01","type":"exec","command":"uptime","exit_code":0,"stdout":"...","stderr":""},
    {"asset_id":2,"asset_name":"db-01","type":"sql","command":"SELECT 1","exit_code":0,"stdout":"...","error":""}
  ]
}
```

**Exit code**: 0 if any command succeeded, 1 if all failed.

**Approval flow**: Policy pre-check per command → single batch approval dialog for all need-confirm commands → parallel execution.

## create

### `create asset [flags]`

Create a new asset (ssh, database, redis, or mongodb). Requires approval.

**Required flags**:
- `--name <string>` — Display name
- `--host <string>` — Hostname or IP
- `--username <string>` — Login username

**Optional flags**:
- `--type <string>` — Asset type: "ssh" (default), "database", "redis", or "mongodb"
- `--port <int>` — Port number (default: auto by type — 22/3306/5432/6379/27017)
- `--auth-type <string>` — SSH auth method: "password" or "key" (SSH type only)
- `--driver <string>` — Database driver: "mysql" or "postgresql" (required for database type)
- `--database <string>` — Default database name (database type)
- `--read-only` — Enable read-only mode (database type)
- `--ssh-asset <asset>` — SSH asset name/ID for tunnel connection (database/redis types)
- `--group-id <int>` — Group ID (0 = ungrouped)
- `--description <string>` — Description
- `--icon <string>` — Icon name (default: auto by type). Available icons:
  - Infrastructure: server, database, cloud, monitor, laptop, router, hard-drive, globe, shield, container, cpu, network
  - Cloud: aws, azure, gcp, alicloud, tencentcloud, huaweicloud, cloudflare
  - DB/Middleware: mysql, postgresql, redis, mongodb, elasticsearch, kafka, mariadb, sqlite, rabbitmq, etcd, clickhouse
  - System/OS: docker, kubernetes, linux, windows, ubuntu, centos, debian, redhat, macos
  - DevOps: nginx, grafana, prometheus

```bash
opsctl create asset --name "Web Server" --host 10.0.0.1 --username root
opsctl create asset --type database --driver mysql --name "Prod DB" --host db.internal --username app
opsctl create asset --type database --driver postgresql --name "Analytics" --host pg.internal --port 5432 --username readonly --read-only
opsctl create asset --type mongodb --name "MongoDB" --host mongo.internal --port 27017 --username admin
opsctl create asset --type redis --name "Cache" --host redis.internal --username default
opsctl create asset --type database --driver mysql --name "DB via SSH" --host 127.0.0.1 --username app --ssh-asset web-server
```

## update

### `update asset <asset> [flags]`

Update an existing asset. Only provided fields change. Requires approval.

**Optional flags**:
- `--name <string>` — New display name
- `--host <string>` — New hostname/IP
- `--port <int>` — New SSH port (0 = unchanged)
- `--username <string>` — New SSH username
- `--description <string>` — New description
- `--group-id <int>` — New group ID (-1 = unchanged, 0 = ungrouped)
- `--icon <string>` — New icon name (see `opsctl create asset --help` for full list)

```bash
opsctl update asset web-server --name "New Name"
opsctl update asset 1 --host 192.168.1.100 --port 2222
opsctl update asset 1 --icon kubernetes
```

## cp

### `cp <source> <destination>`

SCP-style file transfer via SFTP. Requires approval.

**Path format**:
- Local: `/path/to/file` or `./relative`
- Remote: `<asset>:<remote-path>`

**Transfer modes**:
- Local → Remote: `opsctl cp ./config.yml web-server:/etc/app/config.yml`
- Remote → Local: `opsctl cp 1:/var/log/app.log ./app.log`
- Remote → Remote: `opsctl cp 1:/etc/hosts 2:/tmp/hosts` (direct streaming, no local disk)

## grant

### `grant submit <asset> <pattern>...` (simple mode)

Submit exec command patterns for a single asset. No stdin needed.

```bash
opsctl grant submit web-01 "systemctl *" "df -h" "uptime"
opsctl grant submit --group production "uptime" "df -h"
```

### `grant submit [options] [asset...] < input` (JSON mode)

Complex grants from stdin with per-item asset/group overrides.

**Options**:
- `--group <name|id>` — Default group for items without asset/group (repeatable: `--group g1 --group g2`)

**Input JSON**:
```json
{
  "description": "Grant description",
  "items": [
    {"type": "exec", "asset": "web-01", "command": "uptime"},
    {"type": "exec", "group": "production", "command": "systemctl status *"},
    {"type": "cp", "asset": "web-server", "detail": "upload config.yml"},
    {"type": "exec", "command": "df -h"}
  ]
}
```

**Item fields**:
- `type` — "exec", "cp", "create", "update"
- `asset` — Asset name or ID (targets a single asset)
- `group` — Group name or ID (targets all assets in the group)
- `command` — Shell command pattern (supports `*` wildcard)
- `detail` — Human-readable description

Items without asset/group inherit from positional args and `--group` flags (expanded to one item per target).

**Output**: Session ID (UUID) on approval, error on denial.

```bash
# Single asset
opsctl grant submit web-01 < grant.json
# Multiple assets (each item expanded to all targets)
echo '{"items":[{"type":"exec","command":"uptime"}]}' | opsctl grant submit web-01 web-02 web-03
# Per-item overrides (no expansion)
opsctl grant submit < complex-grant.json
# Commands matching grant patterns auto-pass
opsctl exec web-01 -- uptime
```

## session

Sessions are auto-created on the first write operation if none exists. Explicit `session start` is only needed if you want to manage the lifecycle manually.

**Storage**: `.opskat/sessions/<scope>` in CWD (walks up directory tree). Scope is derived from terminal env vars (`TERM_SESSION_ID`, `ITERM_SESSION_ID`, `WT_SESSION`, `WINDOWID`) so different terminal windows get separate sessions. **Sessions expire after 24 hours.**

**Session ID resolution priority**:
1. `--session <id>` global flag (explicit)
2. `OPSKAT_SESSION_ID` environment variable (desktop app injects this)
3. `.opskat/sessions/<scope>` file (auto-created, walks up directory tree)

### `session start`

Create a session and print its ID. Writes to `.opskat/sessions/<scope>` in CWD.

### `session end`

End the current active session (removes the session file).

### `session status`

Show the current active session ID.

```bash
# Auto session (default, no manual steps needed)
opsctl exec web-01 -- uptime       # auto-creates session on first call
opsctl exec web-02 -- df -h        # reuses same session

# Explicit management (cross-terminal/scripting only)
SESSION=$(opsctl session start)
opsctl --session $SESSION exec web-01 -- uptime
opsctl session end
```

## version

Print CLI version.

```bash
opsctl version
```
