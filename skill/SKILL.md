---
name: opsctl
description: "opskat CLI tool (opsctl) for server asset management and remote operations. Use when: (1) user asks about opsctl commands or usage, (2) user wants to manage assets, execute remote commands, transfer files, or SSH into servers via CLI, (3) user asks to write scripts or automation using opsctl, (4) user invokes /opsctl. Covers: list/get/create/update assets, exec, ssh, cp, sql, redis, session and plan approval workflow."
---

# opsctl CLI Tool

opskat's standalone CLI for asset management and remote operations without the GUI.

## Data Directory

/Users/codfrm/Library/Application Support/opskat

## Data Directory

/Users/codfrm/Library/Application Support/opskat

## Global Flags

- `--data-dir <path>` — Override app data directory (default: platform-specific)
- `--master-key <key>` — Master encryption key (env: `OPSKAT_MASTER_KEY`)
- `--session <id>` — Session ID for batch approval (env: `OPSKAT_SESSION_ID`)

## Asset Resolution

Assets can be referenced by:
- **Numeric ID**: `opsctl get asset 1`
- **Name**: `opsctl get asset web-server`
- **Group/Name path**: `opsctl get asset production/web-01` (disambiguates duplicates)

## Command Quick Reference

| Command | Description |
|---------|-------------|
| `list assets [--type ssh\|database\|redis] [--group-id N]` | List assets with optional filters (no description) |
| `list groups` | List all asset groups (no description) |
| `get asset <asset>` | Get asset details including description and SSH config |
| `get group <group>` | Get group details including description |
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
| `plan submit [asset] [--group <name\|id>]` | Submit batch plan from stdin JSON, returns session ID. Optional asset/group sets default scope. |
| `session start` | Create a new approval session |
| `session end` | End the current active session |
| `session status` | Show the current active session ID |
| `init <asset\|--group N>` | Discover server environment and update asset description ([details](references/ops-init.md)) |

For detailed command documentation, see [references/commands.md](references/commands.md).

## Approval Mechanism

Most write operations require desktop app approval via Unix socket (`<data-dir>/approval.sock`).

**Exec approval flow**:
1. Check asset's command policy (allow-list → execute, deny-list → reject)
2. Check plan items with pattern matching (approved plans → auto-allow matching commands)
3. Check session remembered patterns → auto-allow
4. Fall back to desktop app approval (blocks until response)

**Permission pre-request flow** (`request_permission` tool):
1. AI submits command patterns (one per line, supports `*` wildcard) for a target asset
2. Desktop app shows permission approval dialog, user can edit patterns before approving
3. Approved patterns are stored as plan items in database
4. Subsequent commands matching any approved pattern auto-pass without further prompts

## User Rejected Approval — MUST STOP

**When the user explicitly rejects an approval or permission request, you MUST immediately stop the current task. Do NOT attempt to retry, work around, or continue with subsequent steps.**

Scenarios that require an immediate stop:

1. **User rejected execution approval** — The user denied the approval dialog. Output contains "用户拒绝执行".
2. **User rejected permission request** — A `request_permission` plan was rejected by the user. Output contains "用户拒绝 Plan 审批".

**Correct behavior**:
- Stop the entire task immediately — do not execute any remaining steps.
- Report to the user which command was denied.
- Wait for user instructions before taking any further action.

**Do NOT**:
- Retry the same command or a similar variant hoping it will pass.
- Skip the denied step and continue with subsequent steps (the rest of the plan likely depends on it).
- Treat the rejection as a non-fatal warning.

## Session Workflow

For consecutive opsctl operations, create a session to avoid per-operation approval:

```bash
# Create session
SESSION=$(opsctl session start)

# Use --session flag (or OPSKAT_SESSION_ID env var)
opsctl --session $SESSION exec web-01 -- uptime
opsctl --session $SESSION exec web-02 -- df -h
opsctl --session $SESSION cp ./config.yml web-01:/etc/app/

# End session when done
opsctl session end
```

On the first operation, the user will be prompted to approve. If they choose **"Remember"**, the command pattern is stored for auto-approval of matching commands.

**Session ID resolution priority**:
1. `--session <id>` global flag
2. `OPSKAT_SESSION_ID` environment variable
3. Active session file (created by `opsctl session start`)

**Plan workflow** — pre-approve command patterns:
```bash
# Submit plan for a specific asset
SESSION=$(opsctl plan submit web-01 < plan.json)
# Submit plan for a group (applies to all assets in group)
SESSION=$(opsctl plan submit --group production < plan.json)
# Commands matching plan patterns auto-pass
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
