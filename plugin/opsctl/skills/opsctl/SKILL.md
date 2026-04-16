---
name: opsctl
description: "opskat CLI for asset management and remote operations (SSH, SQL, Redis, file transfer). Use when: managing server assets, executing remote commands, writing opsctl scripts/automation, or working with approval/grant/session workflows. Also triggers for: deploying to servers, server diagnostics/troubleshooting, batch operations across fleet, database queries, file transfers between servers, server inventory/discovery."
---

# opsctl CLI Tool

Standalone CLI for asset management and remote operations without the GUI. All managed assets (servers, databases, Redis) are stored in the desktop app — use `list`/`get` to discover available targets before operating.

## Global Flags

- `--data-dir <path>` — Override app data directory
- `--master-key <key>` — Master encryption key (env: `OPSKAT_MASTER_KEY`)
- `--session <id>` — Session ID for batch approval (env: `OPSKAT_SESSION_ID`)

## Asset Resolution

Assets can be referenced by:
- **Numeric ID**: `opsctl get asset 1`
- **Name**: `opsctl get asset web-server`
- **Group/Name path**: `opsctl get asset production/web-01`

## Context Efficiency

Minimize output to save context window:
- **Filter lists**: `opsctl list assets --type ssh --group-id 2` instead of unfiltered `list assets` when the target type/group is known.
- **Targeted get**: Use `get asset <name>` for a single asset instead of listing all then filtering.
- **Batch over sequential**: One `opsctl batch` call returns structured JSON — more compact than N separate `exec` outputs with shell overhead.
- **Pipe to grep/head**: When only partial output is needed, pipe remote commands: `opsctl exec web -- "tail -50 /var/log/app.log"` instead of dumping entire logs.

## Approval Mechanism

Most write operations require desktop app approval.

**Flow**: policy check → grant pattern match → session auto-approve → desktop app approval dialog.

- **Queue mode**: Multiple concurrent approval requests are queued into a single dialog. User can approve/deny individually or batch "Approve All" / "Deny All".
- **Offline**: Policy/grant matches still auto-approve; otherwise rejects. CP/Create/Update always need desktop app.
- **Pre-approve patterns**: Use `grant submit` or `request_permission` tool to submit command patterns (supports `*` wildcard). Approved patterns auto-pass subsequent matching commands.

## Sessions

Sessions auto-create on first write — do NOT manually `session start`. "Allow Session" in the first approval dialog auto-approves subsequent operations. Sessions expire after 24 hours.

For explicit session management, grant workflow, and details, see [references/commands.md](references/commands.md).

## Parallel Execution

**Preferred: `opsctl batch`** — Execute multiple commands (exec/sql/redis) in a single invocation with one approval dialog and parallel execution. This avoids approval race conditions and process-level failures.

```bash
# Args mode (default exec, use type: prefix for sql/redis)
opsctl batch '1:uptime' 'sql:2:SELECT 1' 'redis:3:PING'

# JSON stdin mode (AI-friendly)
echo '{"commands":[
  {"asset":"web-01","type":"exec","command":"uptime"},
  {"asset":"db-01","type":"sql","command":"SELECT COUNT(*) FROM users"},
  {"asset":"cache","type":"redis","command":"PING"}
]}' | opsctl batch
```

Output is structured JSON with per-command results (`exit_code`, `stdout`, `stderr`, `error`).

**Alternative: Parallel sub-agents** — For operations that `batch` doesn't support (e.g., `cp`, `create`), dispatch parallel sub-agents. The desktop app queues concurrent approval requests into a single dialog with "Approve All" / "Deny All" buttons.

**Setup for sub-agents**: Ensure approval is handled before parallelizing:
- **Option A**: Run one command first → user selects "Allow Session" → subsequent commands auto-approve
- **Option B**: `grant submit` patterns for all targets upfront → all matching commands auto-approve

**Parallelizable scenarios**: batch `init`, same command on N servers, multi-target file transfers, independent database queries.

## Commands

Core commands: `list`, `get`, `create`, `update`, `ssh`, `exec`, `batch`, `sql`, `redis`, `mongo`, `cp`, `grant`, `session`, `init`.

For full command reference with flags and examples, see [references/commands.md](references/commands.md).

## Init — Asset Environment Discovery

`/opsctl:init` — Auto-discover server environment via SSH and update asset descriptions. Supports single asset or batch group processing.

## Error Handling

- **User rejection** (output contains "user denied execution" or "user denied grant approval"): Stop the entire task immediately. Report the denied command and wait for user instructions. Do NOT retry, work around, or continue with remaining steps.
- **SSH connection failure**: Report the error, check asset config with `get asset`. Do not retry blindly — ask user if host/credentials changed.
- **Partial batch failure**: `batch` returns per-command results. Report failed commands with their errors, summarize successes. Ask user how to proceed with failures.
- **Command not found on remote**: Suggest installing the missing tool or an alternative command. Do not assume package managers.

## Common Workflows

### Fleet Diagnostics

```bash
# Check disk/memory across all production servers
opsctl batch 'web-01:df -h && free -h' 'web-02:df -h && free -h' 'db-01:df -h && free -h'
```

### Deploy Config → Restart Service

```bash
# 1. Pre-approve the operations
opsctl grant submit web-01 web-02 "tee /etc/app/config.yml" "systemctl restart app"

# 2. Deploy (all auto-approved by grant)
cat config.yml | opsctl exec web-01 -- tee /etc/app/config.yml
cat config.yml | opsctl exec web-02 -- tee /etc/app/config.yml
opsctl batch 'web-01:systemctl restart app' 'web-02:systemctl restart app'
```

### Cross-Environment Data Migration

```bash
# Export from staging, import to prod (direct streaming, no local disk)
opsctl exec staging-db -- "mysqldump -u app dbname | gzip" > /tmp/dump.sql.gz
opsctl exec prod-db -- "gunzip | mysql -u app dbname" < /tmp/dump.sql.gz

# Or query + transfer
opsctl sql staging-db "SELECT * FROM config WHERE env='staging'"
opsctl cp staging:/var/backups/db.sql prod:/var/tmp/db.sql
```

### Batch Server Setup

```bash
# Create assets → init discovery (use parallel sub-agents for create)
opsctl create asset --name web-03 --host 10.0.1.3 --username root
opsctl create asset --name web-04 --host 10.0.1.4 --username root
# Then batch init with /opsctl:init --group <group-id>
```
