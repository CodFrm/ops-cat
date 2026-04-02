package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/buildinfo"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/configs"
)

// Execute runs the opsctl CLI and returns the exit code.
func Execute() int {
	if len(os.Args) < 2 {
		printUsage()
		return 1
	}

	// Parse global flags before the verb
	globalFlags := flag.NewFlagSet("opsctl", flag.ContinueOnError)
	dataDir := globalFlags.String("data-dir", "", "Override the application data directory")
	masterKey := globalFlags.String("master-key", "", "Override the master encryption key (env: OPSKAT_MASTER_KEY)")
	sessionFlag := globalFlags.String("session", "", "Session ID for batch approval (env: OPSKAT_SESSION_ID)")

	// Find the first non-flag argument (verb) position
	verbIdx := 1
	for verbIdx < len(os.Args) && strings.HasPrefix(os.Args[verbIdx], "-") {
		verbIdx++
		if verbIdx < len(os.Args) && !strings.HasPrefix(os.Args[verbIdx], "-") &&
			verbIdx-1 > 0 && !strings.Contains(os.Args[verbIdx-1], "=") {
			verbIdx++
		}
	}

	if err := globalFlags.Parse(os.Args[1:verbIdx]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Environment variable fallback
	if *masterKey == "" {
		if envKey := os.Getenv("OPSKAT_MASTER_KEY"); envKey != "" {
			*masterKey = envKey
		}
	}

	remaining := os.Args[verbIdx:]
	if len(remaining) == 0 {
		printUsage()
		return 1
	}

	verb := remaining[0]
	args := remaining[1:]

	if verb == "version" {
		v := configs.Version
		if c := buildinfo.ShortCommitID(); c != "" {
			v += " (" + c + ")"
		}
		fmt.Println(v)
		return 0
	}
	if verb == "help" || verb == "-h" || verb == "--help" {
		printUsage()
		return 0
	}
	// Initialize database, credentials, repositories
	ctx := context.Background()
	if err := bootstrap.Init(ctx, bootstrap.Options{
		DataDir:   *dataDir,
		MasterKey: *masterKey,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// CLI 默认使用英文策略消息
	ctx = ai.WithPolicyLang(ctx, "en")

	// Load app config (MCP port, etc.)
	resolvedDataDir := *dataDir
	if resolvedDataDir == "" {
		resolvedDataDir = bootstrap.AppDataDir()
	}
	if _, err := bootstrap.LoadConfig(resolvedDataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
	}

	handlers := buildHandlerMap()

	// 创建 SSH 连接池，供 redis/sql 命令的 SSH 隧道使用
	sshPool := sshpool.NewPool(&ai.AIPoolDialer{}, 5*time.Minute)
	defer sshPool.Close()
	ctx = ai.WithSSHPool(ctx, sshPool)

	// Resolve session ID: flag > env > active-session file
	resolvedSession := resolveSessionID(*sessionFlag)

	switch verb {
	case "list":
		return cmdList(ctx, handlers, args)
	case "get":
		return cmdGet(ctx, handlers, args)
	case "exec":
		return cmdExec(ctx, args, resolvedSession)
	case "create":
		return cmdCreate(ctx, handlers, args, resolvedSession)
	case "update":
		return cmdUpdate(ctx, handlers, args, resolvedSession)
	case "cp":
		return cmdCp(ctx, handlers, args, resolvedSession)
	case "sql":
		return cmdSQL(ctx, handlers, args, resolvedSession)
	case "redis":
		return cmdRedisCmd(ctx, handlers, args, resolvedSession)
	case "ssh":
		return cmdSSH(ctx, args)
	case "batch":
		return cmdBatch(ctx, handlers, args, resolvedSession)
	case "grant":
		return cmdGrant(ctx, args, resolvedSession)
	case "session":
		return cmdSession(args)
	case "ext":
		return cmdExt(args)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\nRun 'opsctl help' for usage.\n", verb)
		return 1
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `opsctl - CLI for managing ops-cat remote server assets

Usage:
  opsctl [global-flags] <command> [arguments]

Commands:
  list      List resources (assets or groups)
  get       Get detailed information about a resource
  ssh       Open an interactive SSH terminal session
  exec      Execute a shell command on a remote server via SSH
  sql       Execute SQL on a database asset (MySQL, PostgreSQL)
  redis     Execute a Redis command on a Redis asset
  create    Create a new resource (ssh, database, or redis)
  update    Update an existing resource
  cp        Copy files between local and remote servers (scp-style)
  batch     Execute multiple commands in parallel (exec/sql/redis)
  grant     Submit a batch grant for approval
  session   Manage approval sessions (start, end, status)
  ext       Manage and execute extension tools (list, exec)
  version   Print version information
  help      Show this help message

Note:
  Assets can be referenced by numeric ID or by name.
  Use "group/name" to disambiguate when multiple assets share a name.
  Write operations (exec, cp, create, update) require desktop app approval.

Approval & Sessions:
  Write operations require approval from the running desktop app. On first
  write, a session is auto-created in .opscat/sessions/. When the user
  approves with "Allow Session", all subsequent operations in the same
  session are auto-approved. Sessions expire after 24 hours.

Global Flags:
  --data-dir <path>     Override the application data directory
                        (default: platform-specific, e.g. ~/Library/Application Support/ops-cat)
  --master-key <key>    Override the master encryption key for credential decryption
                        (env: OPSKAT_MASTER_KEY)
  --session <id>        Session ID for approval (env: OPSKAT_SESSION_ID)
                        Auto-created if not specified. Use 'opsctl session start'
                        to explicitly create one.

Run 'opsctl <command> --help' for more information on a specific command.

Examples:
  opsctl list assets                              List all server assets
  opsctl list assets --type ssh --group-id 3      List SSH assets in group 3
  opsctl get asset web-server                     Show details by name
  opsctl get asset 1                              Show details by ID
  opsctl ssh web-server                           Open interactive SSH session
  opsctl ssh production/web-01                    Disambiguate by group/name
  opsctl exec web-server -- uptime                Run command (auto-creates session)
  opsctl sql prod-db "SELECT * FROM users"        Query a database
  opsctl redis cache "GET session:abc"            Execute Redis command
  opsctl create asset --type database --driver mysql --name "DB" --host db.local --username app
  opsctl cp ./config.yml web-server:/etc/app/     Upload a file
  opsctl cp 1:/var/log/app.log ./app.log          Download a file
  opsctl --session $ID exec web-01 -- uptime      Use explicit session
  opsctl ext list                                   List installed extensions
  opsctl ext exec oss list_buckets --args '{}'       Execute extension tool
`)
}
