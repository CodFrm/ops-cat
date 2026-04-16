package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/sshpool"
	"go.uber.org/zap"

	"golang.org/x/crypto/ssh"
)

// batchInput is the JSON input format for the batch command.
type batchInput struct {
	Commands []batchCommand `json:"commands"`
}

type batchCommand struct {
	Asset   string `json:"asset"`
	Type    string `json:"type,omitempty"` // "exec"|"sql"|"redis", default "exec"
	Command string `json:"command"`
}

// batchResult is the per-command result in the JSON output.
type batchResult struct {
	AssetID   int64  `json:"asset_id"`
	AssetName string `json:"asset_name"`
	Type      string `json:"type"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr,omitempty"`
	Error     string `json:"error,omitempty"`
}

type batchOutput struct {
	Results []batchResult `json:"results"`
}

// resolvedBatchCmd is a batch command with resolved asset info.
type resolvedBatchCmd struct {
	asset    *asset_entity.Asset
	cmdType  string // "exec"|"sql"|"redis"
	command  string
	decision *ai.CheckResult // 策略预检结果，用于审计
}

var validBatchTypes = map[string]bool{"exec": true, "sql": true, "redis": true, "mongo": true}

func cmdBatch(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		printBatchUsage()
		return 0
	}

	// Step 1: Parse input
	commands, err := parseBatchInput(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if len(commands) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no commands provided")
		printBatchUsage()
		return 1
	}

	// Step 2: Resolve all assets
	resolved := make([]resolvedBatchCmd, len(commands))
	for i, cmd := range commands {
		asset, resolveErr := resolveAsset(ctx, cmd.Asset)
		if resolveErr != nil {
			fmt.Fprintf(os.Stderr, "Error resolving asset %q: %v\n", cmd.Asset, resolveErr)
			return 1
		}
		cmdType := cmd.Type
		if cmdType == "" {
			cmdType = "exec"
		}
		resolved[i] = resolvedBatchCmd{
			asset:   asset,
			cmdType: cmdType,
			command: cmd.Command,
		}
	}

	// Step 3: Policy pre-check — split into auto-allow / auto-deny / need-confirm
	type permBucket struct {
		idx    int
		result ai.CheckResult
	}
	var autoAllow, autoDeny, needConfirm []permBucket

	for i, cmd := range resolved {
		permCtx := ai.WithSessionID(ctx, session)
		pr := ai.CheckPermission(permCtx, cmd.cmdType, cmd.asset.ID, cmd.command)
		prCopy := pr
		resolved[i].decision = &prCopy
		bucket := permBucket{idx: i, result: pr}
		switch pr.Decision {
		case ai.Allow:
			autoAllow = append(autoAllow, bucket)
		case ai.Deny:
			autoDeny = append(autoDeny, bucket)
		default:
			needConfirm = append(needConfirm, bucket)
		}
	}

	// Build results array
	results := make([]batchResult, len(resolved))
	for i, cmd := range resolved {
		results[i] = batchResult{
			AssetID:   cmd.asset.ID,
			AssetName: cmd.asset.Name,
			Type:      cmd.cmdType,
			Command:   cmd.command,
			ExitCode:  -1, // default: not executed
		}
	}

	auditCtx := ai.WithSessionID(ctx, session)
	auditCtx = ai.WithAuditSource(auditCtx, "opsctl")

	// Fill in denied results + write audit for each denied command
	for _, b := range autoDeny {
		cmd := resolved[b.idx]
		results[b.idx].Error = fmt.Sprintf("denied by policy: %s", b.result.Message)
		argsJSON := fmt.Sprintf(`{"asset_id":%d,"command":%q}`, cmd.asset.ID, truncateStr(cmd.command, 200))
		writeOpsctlAudit(auditCtx, batchAuditTool(cmd.cmdType), argsJSON, "", fmt.Errorf("denied by policy: %s", b.result.Message), cmd.decision)
	}

	// Determine which commands to execute
	execSet := make(map[int]bool)
	for _, b := range autoAllow {
		execSet[b.idx] = true
	}

	// Step 4: Batch approval for need-confirm commands
	if len(needConfirm) > 0 {
		batchItems := make([]approval.BatchItem, 0, len(needConfirm))
		for _, b := range needConfirm {
			cmd := resolved[b.idx]
			batchItems = append(batchItems, approval.BatchItem{
				Type:      cmd.cmdType,
				AssetID:   cmd.asset.ID,
				AssetName: cmd.asset.Name,
				Command:   cmd.command,
			})
		}

		approvalResult, approvalErr := requireBatchApproval(ctx, batchItems, session)
		if approvalErr != nil {
			// All need-confirm commands are denied — write audit for each
			for _, b := range needConfirm {
				cmd := resolved[b.idx]
				results[b.idx].Error = fmt.Sprintf("approval failed: %v", approvalErr)
				argsJSON := fmt.Sprintf(`{"asset_id":%d,"command":%q}`, cmd.asset.ID, truncateStr(cmd.command, 200))
				decision := &ai.CheckResult{Decision: ai.Deny, DecisionSource: approvalResult.DecisionSource}
				writeOpsctlAudit(auditCtx, batchAuditTool(cmd.cmdType), argsJSON, "", approvalErr, decision)
			}
		} else {
			session = approvalResult.SessionID
			auditCtx = ai.WithSessionID(auditCtx, session)
			// Update decision to user_allow for approved commands
			for _, b := range needConfirm {
				resolved[b.idx].decision = &ai.CheckResult{
					Decision:       ai.Allow,
					DecisionSource: ai.SourceUserAllow,
				}
				execSet[b.idx] = true
			}
		}
	}

	// Step 5: Parallel execution
	if len(execSet) > 0 {
		const maxConcurrency = 10
		sem := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup

		for idx := range execSet {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				cmd := resolved[i]
				results[i] = executeBatchItem(auditCtx, handlers, cmd)
			}(idx)
		}
		wg.Wait()
	}

	// Step 6: Output JSON
	output := batchOutput{Results: results}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
		return 1
	}

	// Exit 0 if batch mechanism succeeded (even if individual commands failed)
	// Exit 1 only if ALL commands failed
	allFailed := true
	for _, r := range results {
		if r.Error == "" && r.ExitCode == 0 {
			allFailed = false
			break
		}
	}
	if allFailed && len(results) > 0 {
		return 1
	}
	return 0
}

// executeBatchItem runs a single command and returns the result.
func executeBatchItem(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, cmd resolvedBatchCmd) batchResult {
	result := batchResult{
		AssetID:   cmd.asset.ID,
		AssetName: cmd.asset.Name,
		Type:      cmd.cmdType,
		Command:   cmd.command,
	}

	switch cmd.cmdType {
	case "exec":
		result = executeBatchExec(ctx, cmd)
	case "sql":
		result = executeBatchHandler(ctx, handlers, "exec_sql", cmd, map[string]any{
			"asset_id": float64(cmd.asset.ID),
			"sql":      cmd.command,
		})
	case "redis":
		result = executeBatchHandler(ctx, handlers, "exec_redis", cmd, map[string]any{
			"asset_id": float64(cmd.asset.ID),
			"command":  cmd.command,
		})
	case "mongo":
		// Parse command as JSON: {"operation":"find","database":"db","collection":"col","query":"{}"}
		var mongoArgs struct {
			Operation  string `json:"operation"`
			Database   string `json:"database"`
			Collection string `json:"collection"`
			Query      string `json:"query"`
		}
		if err := json.Unmarshal([]byte(cmd.command), &mongoArgs); err != nil {
			result.Error = fmt.Sprintf("invalid mongo args JSON: %v", err)
			result.ExitCode = -1
			return result
		}
		result = executeBatchHandler(ctx, handlers, "exec_mongo", cmd, map[string]any{
			"asset_id":   float64(cmd.asset.ID),
			"operation":  mongoArgs.Operation,
			"database":   mongoArgs.Database,
			"collection": mongoArgs.Collection,
			"query":      mongoArgs.Query,
		})
	default:
		result.Error = fmt.Sprintf("unsupported type: %s", cmd.cmdType)
		result.ExitCode = -1
	}

	// Write audit log with decision from policy pre-check
	argsJSON := fmt.Sprintf(`{"asset_id":%d,"command":%q}`, cmd.asset.ID, truncateStr(cmd.command, 200))
	var execErr error
	if result.Error != "" {
		execErr = fmt.Errorf("%s", result.Error)
	}
	writeOpsctlAudit(ctx, batchAuditTool(cmd.cmdType), argsJSON, result.Stdout, execErr, cmd.decision)

	return result
}

// executeBatchExec runs an SSH exec command and captures output.
func executeBatchExec(ctx context.Context, cmd resolvedBatchCmd) batchResult {
	result := batchResult{
		AssetID:   cmd.asset.ID,
		AssetName: cmd.asset.Name,
		Type:      cmd.cmdType,
		Command:   cmd.command,
	}

	outBuf := ai.NewLimitedBuffer(auditOutputLimit)
	errBuf := ai.NewLimitedBuffer(auditOutputLimit)

	if proxy := getSSHProxyClient(); proxy != nil {
		exitCode, execErr := proxy.Exec(sshpool.ProxyRequest{
			AssetID: cmd.asset.ID,
			Command: cmd.command,
		}, nil, outBuf, errBuf)
		result.ExitCode = exitCode
		result.Stdout = outBuf.String()
		result.Stderr = errBuf.String()
		if execErr != nil {
			result.Error = execErr.Error()
		}
		return result
	}

	// Fallback: direct SSH
	execErr := ai.ExecWithStdio(ctx, cmd.asset.ID, cmd.command, nil, outBuf, errBuf)
	result.Stdout = outBuf.String()
	result.Stderr = errBuf.String()
	if execErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(execErr, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.ExitCode = -1
			result.Error = execErr.Error()
		}
	}
	return result
}

// executeBatchHandler runs a sql/redis command via the tool handler.
func executeBatchHandler(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, toolName string, cmd resolvedBatchCmd, params map[string]any) batchResult {
	result := batchResult{
		AssetID:   cmd.asset.ID,
		AssetName: cmd.asset.Name,
		Type:      cmd.cmdType,
		Command:   cmd.command,
	}

	ctx = ai.WithAuditSource(ctx, "opsctl")
	handler, ok := handlers[toolName]
	if !ok {
		result.Error = fmt.Sprintf("unknown tool: %s", toolName)
		result.ExitCode = -1
		return result
	}

	output, err := handler(ctx, params)
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result
	}

	result.Stdout = output
	result.ExitCode = 0
	return result
}

// requireBatchApproval sends a single batch approval request to the desktop app.
func requireBatchApproval(ctx context.Context, items []approval.BatchItem, session string) (ApprovalResult, error) {
	if session == "" {
		id := newSessionID()
		if err := writeActiveSession(id); err != nil {
			logger.Default().Warn("write active session", zap.Error(err))
		}
		session = id
	}

	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)

	authToken, err := bootstrap.ReadAuthToken(dataDir)
	if err != nil {
		logger.Default().Warn("read auth token", zap.Error(err))
	}

	// Build detail string for the request
	details := make([]string, 0, len(items))
	for _, item := range items {
		details = append(details, fmt.Sprintf("[%s] %s: %s", item.Type, item.AssetName, truncateStr(item.Command, 80)))
	}

	resp, err := approval.RequestApprovalWithToken(sockPath, authToken, approval.ApprovalRequest{
		Type:       "batch",
		Detail:     strings.Join(details, "\n"),
		SessionID:  session,
		BatchItems: items,
	})
	if err != nil {
		return ApprovalResult{
			Decision:       ai.Deny,
			DecisionSource: ai.SourcePolicyDeny,
			SessionID:      session,
		}, fmt.Errorf("desktop app is not running: %v", err)
	}
	if !resp.Approved {
		reason := resp.Reason
		if reason == "" {
			reason = "denied"
		}
		return ApprovalResult{
			Decision:       ai.Deny,
			DecisionSource: ai.SourceUserDeny,
			SessionID:      session,
		}, fmt.Errorf("batch denied: %s", reason)
	}

	return ApprovalResult{
		Decision:       ai.Allow,
		DecisionSource: ai.SourceUserAllow,
		SessionID:      session,
	}, nil
}

// parseBatchInput parses input from either stdin JSON or positional args.
func parseBatchInput(args []string) ([]batchCommand, error) {
	// Check if stdin has data (pipe mode)
	if stat, err := os.Stdin.Stat(); err == nil {
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				return nil, fmt.Errorf("read stdin: %w", readErr)
			}
			if len(data) > 0 {
				var input batchInput
				if err := json.Unmarshal(data, &input); err != nil {
					return nil, fmt.Errorf("parse JSON input: %w", err)
				}
				// Validate types
				for i := range input.Commands {
					if input.Commands[i].Type == "" {
						input.Commands[i].Type = "exec"
					}
					if !validBatchTypes[input.Commands[i].Type] {
						return nil, fmt.Errorf("invalid type %q for command %d (must be exec/sql/redis/mongo)", input.Commands[i].Type, i)
					}
				}
				return input.Commands, nil
			}
		}
	}

	// Args mode: parse 'type:asset:command' or 'asset:command'
	if len(args) == 0 {
		return nil, nil
	}

	var commands []batchCommand
	for _, arg := range args {
		cmd, err := parseBatchArg(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid argument %q: %w", arg, err)
		}
		commands = append(commands, cmd)
	}
	return commands, nil
}

// parseBatchArg parses a single batch arg: 'type:asset:command' or 'asset:command'
func parseBatchArg(arg string) (batchCommand, error) {
	// Split on first ':'
	idx := strings.IndexByte(arg, ':')
	if idx < 0 {
		return batchCommand{}, fmt.Errorf("expected format 'asset:command' or 'type:asset:command'")
	}

	first := arg[:idx]
	rest := arg[idx+1:]

	// Check if first part is a known type
	if validBatchTypes[first] {
		// 'type:asset:command' — split rest on first ':'
		idx2 := strings.IndexByte(rest, ':')
		if idx2 < 0 {
			return batchCommand{}, fmt.Errorf("expected format 'type:asset:command'")
		}
		return batchCommand{
			Type:    first,
			Asset:   rest[:idx2],
			Command: rest[idx2+1:],
		}, nil
	}

	// 'asset:command' — default type exec
	return batchCommand{
		Type:    "exec",
		Asset:   first,
		Command: rest,
	}, nil
}

func newSessionID() string {
	return fmt.Sprintf("batch_%d", time.Now().UnixNano())
}

// batchAuditTool maps batch command type to audit tool name.
func batchAuditTool(cmdType string) string {
	switch cmdType {
	case "sql":
		return "exec_sql"
	case "redis":
		return "exec_redis"
	case "mongo":
		return "exec_mongo"
	default:
		return "exec"
	}
}

func printBatchUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] batch [args...]

Executes multiple commands in parallel with a single approval request.
Supports exec (SSH), sql, redis, and mongo command types.

Input Modes:
  Stdin JSON (AI-friendly):
    echo '{"commands":[
      {"asset":"web-01","type":"exec","command":"uptime"},
      {"asset":"db-01","type":"sql","command":"SELECT 1"},
      {"asset":"cache","type":"redis","command":"PING"}
    ]}' | opsctl batch

  Positional Args:
    opsctl batch 'web-01:uptime' 'db-01:hostname'
    opsctl batch 'sql:db-01:SELECT 1' 'redis:cache:PING' 'web-01:uptime'

    Format: 'asset:command' (default exec) or 'type:asset:command'

Output:
  JSON with per-command results:
    {"results":[{"asset_id":1,"asset_name":"web-01","type":"exec",
      "command":"uptime","exit_code":0,"stdout":"...","stderr":""}]}

  Exit code: 0 if any command succeeded, 1 if all failed.

Examples:
  opsctl batch '1:uptime' '2:hostname'
  opsctl batch 'sql:prod-db:SELECT COUNT(*) FROM users' 'redis:cache:INFO'
  echo '{"commands":[{"asset":"1","command":"uptime"}]}' | opsctl batch
`)
}
