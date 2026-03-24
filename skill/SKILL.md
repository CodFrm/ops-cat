---
name: opsctl
description: "ops-cat CLI tool (opsctl) for server asset management and remote operations. Use when: (1) user asks about opsctl commands or usage, (2) user wants to manage assets, execute remote commands, transfer files, or SSH into servers via CLI, (3) user asks to write scripts or automation using opsctl, (4) user invokes /opsctl. Covers: list/get/create/update assets, exec, ssh, cp, sql, redis, session and plan approval workflow."
---

# opsctl CLI Tool

ops-cat's standalone CLI for asset management and remote operations without the GUI.

## Global Flags

- `--data-dir <path>` — Override app data directory (default: platform-specific)
- `--master-key <key>` — Master encryption key (env: `OPS_CAT_MASTER_KEY`)
- `--session <id>` — Session ID for batch approval (env: `OPS_CAT_SESSION_ID`)

## Asset Resolution

Assets can be referenced by:
- **Numeric ID**: `opsctl get asset 1`
- **Name**: `opsctl get asset web-server`
- **Group/Name path**: `opsctl get asset production/web-01` (disambiguates duplicates)

## Command Quick Reference

| Command | Description |
|---------|-------------|
| `list assets [--type ssh\|database\|redis] [--group-id N]` | List assets with optional filters |
| `list groups` | List all asset groups |
| `get asset <asset>` | Get asset details (SSH config, etc.) |
| `create asset --type ssh --name X --host X --username X [--port N] [--auth-type key\|password] [--group-id N]` | Create SSH asset (needs approval) |
| `create asset --type database --driver mysql\|postgresql --name X --host X --username X [--port N] [--read-only] [--ssh-asset ID]` | Create database asset |
| `create asset --type redis --name X --host X [--port N] [--ssh-asset ID]` | Create Redis asset |
| `update asset <asset> [--name X] [--host X] [--port N] [--username X] [--group-id N]` | Update asset (needs approval) |
| `ssh <asset>` | Interactive SSH terminal (no approval needed) |
| `exec <asset> -- <command>` | Execute remote command (approval/policy checked) |
| `sql <asset> "<SQL>"` | Execute SQL on database asset (approval/policy checked) |
| `sql <asset> -f <file.sql>` | Execute SQL from file |
| `redis <asset> "<command>"` | Execute Redis command (approval/policy checked) |
| `cp <src> <dst>` | File transfer: local↔remote or remote↔remote (needs approval) |
| `plan submit` | Submit batch plan from stdin JSON, returns session ID |
| `session start` | Create a new approval session |
| `session end` | End the current active session |
| `session status` | Show the current active session ID |
| `init <asset\|--group N>` | Discover server environment and update asset description ([details](references/ops-init.md)) |

For detailed command documentation, see [references/commands.md](references/commands.md).

## Approval Mechanism

Most write operations require desktop app approval via Unix socket (`<data-dir>/approval.sock`).

**Exec approval flow**:
1. Check asset's command policy (allow-list → execute, deny-list → reject)
2. Check plan session if `--session` points to a plan
3. Check if session is already approved → auto-allow
4. Fall back to desktop app approval (blocks until response)

## Session Workflow

For consecutive opsctl operations, create a session to avoid per-operation approval:

```bash
# Create session
SESSION=$(opsctl session start)

# Use --session flag (or OPS_CAT_SESSION_ID env var)
opsctl --session $SESSION exec web-01 -- uptime
opsctl --session $SESSION exec web-02 -- df -h
opsctl --session $SESSION cp ./config.yml web-01:/etc/app/

# End session when done
opsctl session end
```

On the first operation, the user will be prompted to approve. If they choose **"Allow Session"**, all subsequent operations within the same session are auto-approved.

**Session ID resolution priority**:
1. `--session <id>` global flag
2. `OPS_CAT_SESSION_ID` environment variable
3. Active session file (created by `opsctl session start`)

**Batch plan workflow** — pre-approve specific operations:
```bash
SESSION=$(opsctl plan submit < plan.json)
opsctl --session $SESSION exec web-01 -- systemctl restart app
```

## Init — Asset Environment Discovery

Auto-discover server environment via SSH and persist a structured description to the asset's `description` field. See [references/ops-init.md](references/ops-init.md) for full instructions.

```bash
/opsctl init web-server       # Single asset
/opsctl init --group 2        # All assets in group
/opsctl init                  # Interactive selection
```

## Common Patterns

**Query a database**:
```bash
opsctl sql prod-db "SELECT * FROM users LIMIT 10"
opsctl sql prod-db -f migration.sql
opsctl sql prod-db -d other_db "SHOW TABLES"
```

**Query Redis**:
```bash
opsctl redis cache "GET session:abc123"
opsctl redis cache "HGETALL user:1"
opsctl redis cache "SET key value EX 3600"
```

**Pipe data through remote command**:
```bash
cat config.yml | opsctl exec web -- tee /etc/app/config.yml
```

**Direct server-to-server file transfer** (no local disk):
```bash
opsctl cp staging:/var/backups/db.sql prod:/var/tmp/db.sql
```

**Deploy workflow with session**:
```bash
SESSION=$(opsctl session start)
opsctl --session $SESSION exec web-01 -- systemctl stop app
opsctl --session $SESSION cp ./app web-01:/usr/local/bin/app
opsctl --session $SESSION exec web-01 -- systemctl start app
opsctl session end
```
