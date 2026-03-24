package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/configs"
	"golang.org/x/crypto/ssh"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
		fatal(err)
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
		os.Exit(1)
	}

	verb := remaining[0]
	args := remaining[1:]

	if verb == "version" {
		fmt.Println(configs.Version)
		return
	}
	if verb == "help" || verb == "-h" || verb == "--help" {
		printUsage()
		return
	}

	// Initialize database, credentials, repositories
	ctx := context.Background()
	if err := bootstrap.Init(ctx, bootstrap.Options{
		DataDir:   *dataDir,
		MasterKey: *masterKey,
	}); err != nil {
		fatal(err)
	}

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

	var exitCode int
	switch verb {
	case "list":
		exitCode = cmdList(ctx, handlers, args)
	case "get":
		exitCode = cmdGet(ctx, handlers, args)
	case "exec":
		exitCode = cmdExec(ctx, args, resolvedSession)
	case "create":
		exitCode = cmdCreate(ctx, handlers, args, resolvedSession)
	case "update":
		exitCode = cmdUpdate(ctx, handlers, args, resolvedSession)
	case "cp":
		exitCode = cmdCp(ctx, handlers, args, resolvedSession)
	case "sql":
		exitCode = cmdSQL(ctx, handlers, args, resolvedSession)
	case "redis":
		exitCode = cmdRedisCmd(ctx, handlers, args, resolvedSession)
	case "ssh":
		exitCode = cmdSSH(ctx, args)
	case "plan":
		exitCode = cmdPlan(ctx, args)
	case "session":
		exitCode = cmdSession(args)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\nRun 'opsctl help' for usage.\n", verb) //nolint:gosec // verb is from CLI args, not user-controlled web input
		exitCode = 1
	}
	os.Exit(exitCode)
}

func buildHandlerMap() map[string]ai.ToolHandlerFunc {
	m := make(map[string]ai.ToolHandlerFunc)
	for _, def := range ai.AllToolDefs() {
		m[def.Name] = def.Handler
	}
	return m
}

// --- Commands ---

func cmdList(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printListUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	resource := args[0]
	switch resource {
	case "assets":
		fs := flag.NewFlagSet("list assets", flag.ExitOnError)
		assetType := fs.String("type", "", "Filter by asset type (e.g. \"ssh\")")
		groupID := fs.Int64("group-id", 0, "Filter by group ID (0 = all groups)")
		fs.Usage = func() { printListAssetsUsage() }
		_ = fs.Parse(args[1:])

		params := map[string]any{}
		if *assetType != "" {
			params["asset_type"] = *assetType
		}
		if *groupID != 0 {
			params["group_id"] = float64(*groupID)
		}
		return callHandler(ctx, handlers, "list_assets", params)

	case "groups":
		return callHandler(ctx, handlers, "list_groups", nil)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: assets, groups\n", resource) //nolint:gosec // resource is from CLI args
		return 1
	}
}

func cmdGet(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printGetUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}
	if len(args) < 2 {
		printGetUsage()
		return 1
	}

	resource := args[0]
	switch resource {
	case "asset":
		id, err := resolveAssetID(ctx, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return callHandler(ctx, handlers, "get_asset", map[string]any{
			"id": float64(id),
		})
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: asset\n", resource) //nolint:gosec // resource is from CLI args
		return 1
	}
}

func cmdExec(ctx context.Context, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printExecUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	assetID := asset.ID

	command := extractCommand(args[1:])
	if command == "" {
		printExecUsage()
		return 1
	}

	// Require approval
	argsJSON := fmt.Sprintf(`{"asset_id":%d,"command":%q}`, assetID, command)
	approvalResult, err := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "exec",
		AssetID:   assetID,
		AssetName: asset.Name,
		Command:   command,
		Detail:    fmt.Sprintf("opsctl exec %s -- %s", args[0], command),
		SessionID: session,
	})
	// 注入 SessionID 到 context，供审计写入器使用
	auditCtx := ai.WithSessionID(ctx, approvalResult.SessionID)

	if err != nil {
		writeOpsctlAudit(auditCtx, "exec", argsJSON, "", err, approvalResult.ToCheckResult())
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Detect if stdin is a pipe (not a terminal)
	var stdin io.Reader
	if stat, err := os.Stdin.Stat(); err == nil {
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			stdin = os.Stdin
		}
	}

	// 尝试通过 proxy 执行（复用 ops-cat 连接池）
	if proxy := getSSHProxyClient(); proxy != nil {
		exitCode, execErr := proxy.Exec(sshpool.ProxyRequest{
			AssetID: assetID,
			Command: command,
		}, stdin, os.Stdout, os.Stderr)
		auditResult := fmt.Sprintf(`{"exit_code":%d}`, exitCode)
		writeOpsctlAudit(auditCtx, "exec", argsJSON, auditResult, execErr, approvalResult.ToCheckResult())
		if execErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", execErr)
			return 1
		}
		return exitCode
	}

	// Fallback: 直连
	execErr := ai.ExecWithStdio(ctx, assetID, command, stdin, os.Stdout, os.Stderr)

	// 审计日志
	auditResult := `{"status":"completed"}`
	if execErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(execErr, &exitErr) {
			auditResult = fmt.Sprintf(`{"exit_code":%d}`, exitErr.ExitStatus())
		}
	}
	writeOpsctlAudit(auditCtx, "exec", argsJSON, auditResult, execErr, approvalResult.ToCheckResult())

	if execErr != nil {
		// Propagate remote command exit code
		var exitErr *ssh.ExitError
		if errors.As(execErr, &exitErr) {
			return exitErr.ExitStatus()
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", execErr)
		return 1
	}
	return 0
}

func cmdCreate(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printCreateUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	resource := args[0]
	switch resource {
	case "asset":
		fs := flag.NewFlagSet("create asset", flag.ExitOnError)
		assetType := fs.String("type", "ssh", `Asset type: "ssh", "database", or "redis"`)
		name := fs.String("name", "", "Display name for the asset (required)")
		host := fs.String("host", "", "Hostname or IP address (required)")
		port := fs.Int("port", 0, "Port number (default: auto by type)")
		username := fs.String("username", "", "Login username (required)")
		authType := fs.String("auth-type", "password", "SSH auth method: password or key")
		driver := fs.String("driver", "", `Database driver: "mysql" or "postgresql" (required for database type)`)
		database := fs.String("database", "", "Default database name (for database type)")
		readOnly := fs.Bool("read-only", false, "Enable read-only mode (for database type)")
		sshAsset := fs.String("ssh-asset", "", "SSH asset name/ID for tunnel connection (for database/redis)")
		groupID := fs.Int64("group-id", 0, "Group ID to assign the asset to (0 = ungrouped)")
		description := fs.String("description", "", "Optional description or notes")
		fs.Usage = func() { printCreateAssetUsage() }
		_ = fs.Parse(args[1:])

		if *name == "" || *host == "" || *username == "" {
			fmt.Fprintln(os.Stderr, "Error: --name, --host, and --username are required")
			fmt.Fprintln(os.Stderr)
			printCreateAssetUsage()
			return 1
		}

		// 自动设置默认端口
		if *port == 0 {
			switch *assetType {
			case "ssh":
				*port = 22
			case "database":
				switch *driver {
				case "mysql":
					*port = 3306
				case "postgresql":
					*port = 5432
				default:
					*port = 3306
				}
			case "redis":
				*port = 6379
			}
		}

		params := map[string]any{
			"name":     *name,
			"type":     *assetType,
			"host":     *host,
			"port":     float64(*port),
			"username": *username,
		}
		if *assetType == "ssh" && *authType != "" {
			params["auth_type"] = *authType
		}
		if *assetType == "database" {
			if *driver != "" {
				params["driver"] = *driver
			}
			if *database != "" {
				params["database"] = *database
			}
			if *readOnly {
				params["read_only"] = "true"
			}
		}
		if *sshAsset != "" {
			sshID, resolveErr := resolveAssetID(ctx, *sshAsset)
			if resolveErr != nil {
				fmt.Fprintf(os.Stderr, "Error resolving SSH asset: %v\n", resolveErr)
				return 1
			}
			params["ssh_asset_id"] = float64(sshID)
		}
		if *groupID != 0 {
			params["group_id"] = float64(*groupID)
		}
		if *description != "" {
			params["description"] = *description
		}
		// Require approval
		if _, err := requireApproval(ctx, approval.ApprovalRequest{
			Type:      "create",
			Detail:    fmt.Sprintf("opsctl create asset --type %s --name %s --host %s", *assetType, *name, *host),
			SessionID: session,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}

		return callHandler(ctx, handlers, "add_asset", params)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: asset\n", resource) //nolint:gosec // resource is from CLI args
		return 1
	}
}

func cmdUpdate(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printUpdateUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}
	if len(args) < 2 {
		printUpdateUsage()
		return 1
	}

	resource := args[0]
	switch resource {
	case "asset":
		id, err := resolveAssetID(ctx, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}

		fs := flag.NewFlagSet("update asset", flag.ExitOnError)
		name := fs.String("name", "", "New display name")
		host := fs.String("host", "", "New hostname or IP address")
		port := fs.Int("port", 0, "New SSH port number (0 = unchanged)")
		username := fs.String("username", "", "New SSH login username")
		description := fs.String("description", "", "New description")
		groupID := fs.Int64("group-id", -1, "New group ID (-1 = unchanged, 0 = ungrouped)")
		fs.Usage = func() { printUpdateAssetUsage() }
		_ = fs.Parse(args[2:])

		params := map[string]any{
			"id": float64(id),
		}
		if *name != "" {
			params["name"] = *name
		}
		if *host != "" {
			params["host"] = *host
		}
		if *port != 0 {
			params["port"] = float64(*port)
		}
		if *username != "" {
			params["username"] = *username
		}
		if *description != "" {
			params["description"] = *description
		}
		if *groupID >= 0 {
			params["group_id"] = float64(*groupID)
		}
		// Require approval
		if _, err := requireApproval(ctx, approval.ApprovalRequest{
			Type:      "update",
			AssetID:   id,
			Detail:    fmt.Sprintf("opsctl update asset %s", args[1]),
			SessionID: session,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}

		return callHandler(ctx, handlers, "update_asset", params)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: asset\n", resource) //nolint:gosec // resource is from CLI args
		return 1
	}
}

func cmdCp(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printCpUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}
	if len(args) < 2 {
		printCpUsage()
		return 1
	}

	src, dst := args[0], args[1]
	srcAssetID, srcPath, err := parseRemotePathCtx(ctx, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	dstAssetID, dstPath, err := parseRemotePathCtx(ctx, dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	srcIsRemote := srcAssetID > 0
	dstIsRemote := dstAssetID > 0

	// Require approval for any remote operation
	if srcIsRemote || dstIsRemote {
		approvalAssetID := srcAssetID
		if dstIsRemote {
			approvalAssetID = dstAssetID
		}
		if _, err := requireApproval(ctx, approval.ApprovalRequest{
			Type:      "cp",
			AssetID:   approvalAssetID,
			Detail:    fmt.Sprintf("opsctl cp %s %s", src, dst),
			SessionID: session,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}

	// 尝试通过 proxy 执行文件传输
	if proxy := getSSHProxyClient(); proxy != nil {
		return cmdCpViaProxy(proxy, srcAssetID, srcPath, dstAssetID, dstPath, srcIsRemote, dstIsRemote, src, dst)
	}

	// Fallback: 直连
	switch {
	case !srcIsRemote && !dstIsRemote:
		fmt.Fprintln(os.Stderr, "Error: at least one path must be remote (<asset>:<path>)")
		return 1

	case !srcIsRemote && dstIsRemote:
		// Upload: local -> remote
		return callHandler(ctx, handlers, "upload_file", map[string]any{
			"asset_id":    float64(dstAssetID),
			"local_path":  src,
			"remote_path": dstPath,
		})

	case srcIsRemote && !dstIsRemote:
		// Download: remote -> local
		return callHandler(ctx, handlers, "download_file", map[string]any{
			"asset_id":    float64(srcAssetID),
			"remote_path": srcPath,
			"local_path":  dst,
		})

	default:
		// Asset-to-asset transfer: remote -> remote
		if err := ai.CopyBetweenAssets(ctx, srcAssetID, srcPath, dstAssetID, dstPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}
}

// cmdCpViaProxy 通过 proxy 执行文件传输
func cmdCpViaProxy(proxy *sshpool.Client, srcAssetID int64, srcPath string, dstAssetID int64, dstPath string, srcIsRemote, dstIsRemote bool, src, dst string) int {
	switch {
	case !srcIsRemote && !dstIsRemote:
		fmt.Fprintln(os.Stderr, "Error: at least one path must be remote (<asset>:<path>)")
		return 1

	case !srcIsRemote && dstIsRemote:
		// Upload: local -> remote
		f, err := os.Open(src) //nolint:gosec // src is a user-provided local file path
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		if err := proxy.Upload(sshpool.ProxyRequest{
			AssetID: dstAssetID,
			DstPath: dstPath,
		}, f); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case srcIsRemote && !dstIsRemote:
		// Download: remote -> local
		f, err := os.Create(dst) //nolint:gosec // dst is a user-provided local file path
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		if err := proxy.Download(sshpool.ProxyRequest{
			AssetID: srcAssetID,
			SrcPath: srcPath,
		}, f); err != nil {
			_ = os.Remove(dst) //nolint:gosec // dst is a user-provided local file path
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	default:
		// Asset-to-asset transfer: remote -> remote
		if err := proxy.Copy(sshpool.ProxyRequest{
			AssetID:    dstAssetID,
			SrcAssetID: srcAssetID,
			SrcPath:    srcPath,
			DstPath:    dstPath,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}
}

func cmdSQL(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printSQLUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("sql", flag.ContinueOnError)
	file := fs.String("f", "", "Read SQL from file")
	database := fs.String("d", "", "Override default database")
	fs.Usage = func() { printSQLUsage() }
	_ = fs.Parse(args[1:])

	var sqlText string
	if *file != "" {
		data, readErr := os.ReadFile(*file)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", readErr)
			return 1
		}
		sqlText = string(data)
	} else {
		sqlText = strings.Join(fs.Args(), " ")
	}
	if sqlText == "" {
		fmt.Fprintln(os.Stderr, "Error: SQL statement is required")
		printSQLUsage()
		return 1
	}

	// Require approval
	if _, approvalErr := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "sql",
		AssetID:   asset.ID,
		AssetName: asset.Name,
		Command:   sqlText,
		Detail:    fmt.Sprintf("opsctl sql %s %q", args[0], truncateStr(sqlText, 100)),
		SessionID: session,
	}); approvalErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", approvalErr)
		return 1
	}

	params := map[string]any{
		"asset_id": float64(asset.ID),
		"sql":      sqlText,
	}
	if *database != "" {
		params["database"] = *database
	}
	return callHandler(ctx, handlers, "exec_sql", params)
}

func cmdRedisCmd(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printRedisUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	command := strings.Join(args[1:], " ")
	if command == "" {
		fmt.Fprintln(os.Stderr, "Error: Redis command is required")
		printRedisUsage()
		return 1
	}

	// Require approval
	if _, approvalErr := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "redis",
		AssetID:   asset.ID,
		AssetName: asset.Name,
		Command:   command,
		Detail:    fmt.Sprintf("opsctl redis %s %q", args[0], truncateStr(command, 100)),
		SessionID: session,
	}); approvalErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", approvalErr)
		return 1
	}

	return callHandler(ctx, handlers, "exec_redis", map[string]any{
		"asset_id": float64(asset.ID),
		"command":  command,
	})
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Helpers ---

func callHandler(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, toolName string, params map[string]any) int {
	handler, ok := handlers[toolName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Internal error: unknown tool %s\n", toolName)
		return 1
	}

	if params == nil {
		params = map[string]any{}
	}

	ctx = ai.WithAuditSource(ctx, "opsctl")
	result, err := handler(ctx, params)

	// 写审计日志
	argsJSON, _ := json.Marshal(params)
	writeOpsctlAudit(ctx, toolName, string(argsJSON), result, err, nil)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// 写操作成功后通知桌面端刷新 UI
	if toolName == "add_asset" || toolName == "update_asset" {
		go approval.SendNotification(
			approval.SocketPath(bootstrap.AppDataDir()),
			"asset",
		)
	}

	// Pretty-print JSON output
	var obj any
	if json.Unmarshal([]byte(result), &obj) == nil {
		pretty, err := json.MarshalIndent(obj, "", "  ")
		if err == nil {
			fmt.Println(string(pretty))
			return 0
		}
	}
	fmt.Println(result)
	return 0
}

// parseRemotePath parses <asset>:<path> format where <asset> is an ID or name.
// Returns (assetID, path, error). If not a remote path, assetID is 0.
func parseRemotePathCtx(ctx context.Context, s string) (int64, string, error) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return 0, s, nil
	}
	prefix := s[:idx]
	remotePath := s[idx+1:]

	// Must start with / to be a remote path (avoid matching C:\windows paths or names without colon)
	if !strings.HasPrefix(remotePath, "/") {
		return 0, s, nil
	}

	id, err := resolveAssetID(ctx, prefix)
	if err != nil {
		return 0, "", fmt.Errorf("resolving asset %q: %w", prefix, err)
	}
	return id, remotePath, nil
}

// parseRemotePath is the legacy non-context version used by tests.
func parseRemotePath(s string) (int64, string) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return 0, s
	}
	id, err := strconv.ParseInt(s[:idx], 10, 64)
	if err != nil {
		return 0, s
	}
	return id, s[idx+1:]
}

// extractCommand extracts the command string after "--" separator.
// If no "--" is found, all args are joined as the command.
func extractCommand(args []string) string {
	for i, arg := range args {
		if arg == "--" {
			parts := args[i+1:]
			if len(parts) == 0 {
				return ""
			}
			return strings.Join(parts, " ")
		}
	}
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	return ""
}

// opsctlAuditWriter 全局审计写入器
var opsctlAuditWriter ai.AuditWriter = ai.NewDefaultAuditWriter()

// writeOpsctlAudit 统一的审计日志写入函数
func writeOpsctlAudit(ctx context.Context, toolName, argsJSON, result string, execErr error, decision *ai.CheckResult) {
	opsctlAuditWriter.WriteToolCall(ctx, ai.ToolCallInfo{
		ToolName: toolName,
		ArgsJSON: argsJSON,
		Result:   result,
		Error:    execErr,
		Decision: decision,
	})
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// --- Usage text ---

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
  plan      Submit a batch plan for approval
  session   Manage approval sessions (start, end, status)
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
                        (env: OPS_CAT_MASTER_KEY)
  --session <id>        Session ID for approval (env: OPS_CAT_SESSION_ID)
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
`)
}

func printListUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl list <resource> [flags]

Resources:
  assets    List server assets
  groups    List asset groups

Run 'opsctl list assets --help' for asset-specific flags.

Examples:
  opsctl list assets
  opsctl list assets --type ssh --group-id 3
  opsctl list groups
`)
}

func printListAssetsUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl list assets [flags]

Flags:
  --type <string>       Filter by asset type (e.g. "ssh"). Omit to list all types.
  --group-id <int>      Filter by group ID. 0 or omit to list across all groups.

Examples:
  opsctl list assets
  opsctl list assets --type ssh
  opsctl list assets --group-id 3
`)
}

func printGetUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl get <resource> <asset>

Resources:
  asset     Get detailed asset information including SSH connection config

Arguments:
  asset     Asset name or numeric ID (use 'opsctl list assets' to find them)

Examples:
  opsctl get asset web-server
  opsctl get asset 1
  opsctl get asset production/web-01
`)
}

func printExecUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] exec <asset> [--] <command>

Arguments:
  asset       Asset name or numeric ID
  command     Shell command to execute on the remote server.
              Use '--' to separate the command from opsctl flags.
              Everything after '--' is joined into a single command string.

Pipe Support:
  If stdin is not a terminal (i.e., data is piped in), it is forwarded to the
  remote command's stdin. The remote command's stdout and stderr are written
  directly to local stdout and stderr, enabling Unix pipe chains.

  The exit code of the remote command is propagated as opsctl's exit code.

Approval:
  This command requires approval from the running desktop app.
  - Commands matching the asset's allow list execute without approval.
  - Commands matching the deny list are rejected immediately.
  - A session is auto-created if not specified. Once the user approves with
    "Allow Session", subsequent commands in the same session skip approval.

Examples:
  opsctl exec web-server -- uptime
  opsctl exec 1 -- ls -la /var/log
  opsctl exec production/web-01 -- cat /etc/hosts
  echo "hello" | opsctl exec web-server -- cat
  opsctl --session $ID exec web-01 -- systemctl restart nginx
`)
}

func printCreateUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl create <resource> [flags]

Resources:
  asset     Create a new asset (ssh, database, or redis)

Run 'opsctl create asset --help' for details.
`)
}

func printCreateAssetUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] create asset [flags]

Required Flags:
  --name <string>         Display name for the asset
  --host <string>         Hostname or IP address
  --username <string>     Login username

Optional Flags:
  --type <string>         Asset type: "ssh" (default), "database", or "redis"
  --port <int>            Port number (default: auto by type — 22/3306/5432/6379)
  --auth-type <string>    SSH auth method: "password" or "key" (SSH type only)
  --driver <string>       Database driver: "mysql" or "postgresql" (database type, required)
  --database <string>     Default database name (database type)
  --read-only             Enable read-only mode (database type)
  --ssh-asset <asset>     SSH asset name/ID for tunnel connection (database/redis types)
  --group-id <int>        Group ID to assign the asset to (0 = ungrouped)
  --description <string>  Optional description or notes

Approval:
  Requires desktop app approval. Session auto-created if not specified.

Examples:
  opsctl create asset --name "Web Server" --host 10.0.0.1 --username root
  opsctl create asset --type database --driver mysql --name "Prod DB" --host db.internal --username app
  opsctl create asset --type database --driver postgresql --name "Analytics" --host pg.internal --port 5432 --username readonly --read-only
  opsctl create asset --type redis --name "Cache" --host redis.internal --username default
  opsctl create asset --type database --driver mysql --name "DB via SSH" --host 127.0.0.1 --username app --ssh-asset web-server
`)
}

func printUpdateUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl update <resource> <asset> [flags]

Resources:
  asset     Update an existing SSH server asset

Run 'opsctl update asset <asset> --help' for details.
`)
}

func printUpdateAssetUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] update asset <asset> [flags]

Arguments:
  asset     Asset name or numeric ID

Flags (only provided fields are updated, others remain unchanged):
  --name <string>         New display name
  --host <string>         New hostname or IP address
  --port <int>            New SSH port number (0 = unchanged)
  --username <string>     New SSH login username
  --description <string>  New description
  --group-id <int>        New group ID (-1 = unchanged, 0 = ungrouped)

Approval:
  Requires desktop app approval. Session auto-created if not specified.

Examples:
  opsctl update asset web-server --name "New Name"
  opsctl update asset 1 --host 192.168.1.100 --port 2222
  opsctl update asset web-server --group-id 3
`)
}

func printSQLUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] sql <asset> [flags] "<SQL>"

Arguments:
  asset     Database asset name or numeric ID

Flags:
  -f <file>     Read SQL from file instead of argument
  -d <database> Override the default database for this execution

Approval:
  This command requires approval from the running desktop app.
  SQL statements are checked against the asset's query policy:
  - Allowed types (e.g. SELECT) execute without approval
  - Denied types (e.g. DROP TABLE) are rejected
  - Other statements require user confirmation

Examples:
  opsctl sql prod-db "SELECT * FROM users LIMIT 10"
  opsctl sql prod-db "INSERT INTO logs (msg) VALUES ('test')"
  opsctl sql prod-db -f migration.sql
  opsctl sql prod-db -d other_db "SHOW TABLES"
`)
}

func printRedisUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] redis <asset> "<command>"

Arguments:
  asset     Redis asset name or numeric ID
  command   Redis command (e.g. "GET mykey", "HGETALL user:1")

Approval:
  This command requires approval from the running desktop app.
  Commands are checked against the asset's Redis policy:
  - Dangerous commands (FLUSHDB, CONFIG SET, etc.) are rejected by default
  - Other commands require user confirmation on first use

Examples:
  opsctl redis cache "GET session:abc123"
  opsctl redis cache "HGETALL user:1"
  opsctl redis cache "SET key value EX 3600"
  opsctl redis cache "KEYS user:*"
`)
}

func printCpUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] cp <source> <destination>

Path Format:
  Local path:   /path/to/file  or  ./relative/path
  Remote path:  <asset>:<remote-path>  (asset name or ID)

At least one of source or destination must be a remote path.

Transfer Modes:
  Local -> Remote     Upload a file to a remote server via SFTP
  Remote -> Local     Download a file from a remote server via SFTP
  Remote -> Remote    Stream a file directly between two assets (no local disk)

Approval:
  Requires desktop app approval. Session auto-created if not specified.

Examples:
  opsctl cp ./config.yml web-server:/etc/app/config.yml   Upload by name
  opsctl cp 1:/var/log/app.log ./app.log                  Download by ID
  opsctl cp 1:/etc/hosts 2:/tmp/hosts                     Transfer between assets
`)
}
