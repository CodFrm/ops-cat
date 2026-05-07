package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// grantInput 从 stdin 读取的授权 JSON 格式
type grantInput struct {
	Description string           `json:"description"`
	Items       []grantInputItem `json:"items"`
}

type grantInputItem struct {
	Type    string `json:"type"`    // "exec", "cp", "create", "update"
	Asset   string `json:"asset"`   // 资产名称或 ID
	Group   string `json:"group"`   // 资产组名称或 ID
	Command string `json:"command"` // 命令模式
	Detail  string `json:"detail"`
}

func cmdGrant(ctx context.Context, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printGrantUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	switch args[0] {
	case "submit":
		return cmdGrantSubmit(ctx, args[1:], session)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown grant subcommand %q\n\nRun 'opsctl grant --help' for usage.\n", args[0])
		return 1
	}
}

// stringSliceFlag 支持重复指定的字符串 flag（如 --group a --group b）
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return fmt.Sprintf("%v", *s) }
func (s *stringSliceFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

// resolvedTarget 解析后的默认目标
type resolvedTarget struct {
	AssetID   int64
	AssetName string
	GroupID   int64
	GroupName string
}

func cmdGrantSubmit(ctx context.Context, args []string, session string) int {
	fs := flag.NewFlagSet("grant submit", flag.ContinueOnError)
	var groupFlags stringSliceFlag
	fs.Var(&groupFlags, "group", "Default group for items without asset/group (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	remaining := fs.Args()

	// 检测 stdin 是否为终端（TTY）
	stdinIsTerminal := false
	if fi, err := os.Stdin.Stat(); err == nil {
		stdinIsTerminal = fi.Mode()&os.ModeCharDevice != 0
	}

	// 简单模式：opsctl grant submit <asset> "pattern1" "pattern2"
	// 当 stdin 是终端且有 2+ 位置参数时，第一个为资产，其余为 exec 命令模式
	var defaultTargets []resolvedTarget
	var input grantInput

	if stdinIsTerminal && (len(remaining) >= 2 || (len(groupFlags) > 0 && len(remaining) >= 1)) {
		// 简单模式：解析资产和命令模式
		var patterns []string
		if len(groupFlags) > 0 {
			// --group 模式：所有位置参数都是命令模式
			patterns = remaining
		} else {
			// 第一个参数是资产，其余是命令模式
			asset, err := resolveAsset(ctx, remaining[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			defaultTargets = append(defaultTargets, resolvedTarget{
				AssetID: asset.ID, AssetName: asset.Name,
			})
			patterns = remaining[1:]
		}
		for _, g := range groupFlags {
			gid, gname, err := resolveGroup(ctx, g)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			defaultTargets = append(defaultTargets, resolvedTarget{
				GroupID: gid, GroupName: gname,
			})
		}
		for _, p := range patterns {
			input.Items = append(input.Items, grantInputItem{Type: "exec", Command: p})
		}
		input.Description = fmt.Sprintf("grant: %s", strings.Join(patterns, ", "))
	} else {
		// JSON 模式：解析默认目标，从 stdin 读取
		for _, arg := range remaining {
			asset, err := resolveAsset(ctx, arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			defaultTargets = append(defaultTargets, resolvedTarget{
				AssetID: asset.ID, AssetName: asset.Name,
			})
		}
		for _, g := range groupFlags {
			gid, gname, err := resolveGroup(ctx, g)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			defaultTargets = append(defaultTargets, resolvedTarget{
				GroupID: gid, GroupName: gname,
			})
		}
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", err)
			return 1
		}
	}

	if len(input.Items) == 0 {
		fmt.Fprintln(os.Stderr, "Error: grant must contain at least one item")
		return 1
	}

	// 解析资产/组名称，构建 GrantItems
	var grantItems []approval.GrantItem
	for i, item := range input.Items {
		if item.Asset != "" || item.Group != "" {
			// item 指定了明确目标
			var assetID int64
			var assetName string
			var groupID int64
			var groupName string
			if item.Asset != "" {
				asset, err := resolveAsset(ctx, item.Asset)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: item %d: %v\n", i+1, err)
					return 1
				}
				assetID = asset.ID
				assetName = asset.Name
			} else {
				gid, gname, err := resolveGroup(ctx, item.Group)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: item %d: %v\n", i+1, err)
					return 1
				}
				groupID = gid
				groupName = gname
			}
			grantItems = append(grantItems, approval.GrantItem{
				Type: item.Type, AssetID: assetID, AssetName: assetName,
				GroupID: groupID, GroupName: groupName,
				Command: item.Command, Detail: item.Detail,
			})
		} else if len(defaultTargets) > 0 {
			// 展开为每个默认目标一条 item
			for _, t := range defaultTargets {
				grantItems = append(grantItems, approval.GrantItem{
					Type: item.Type, AssetID: t.AssetID, AssetName: t.AssetName,
					GroupID: t.GroupID, GroupName: t.GroupName,
					Command: item.Command, Detail: item.Detail,
				})
			}
		} else {
			// 无目标
			grantItems = append(grantItems, approval.GrantItem{
				Type: item.Type, Command: item.Command, Detail: item.Detail,
			})
		}
	}

	// 优先使用已有 session，没有时生成新的
	sessionID := session
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 通过 socket 发送 grant 请求
	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)
	authToken, err := bootstrap.ReadAuthToken(dataDir)
	if err != nil {
		logger.Ctx(ctx).Warn("read auth token", zap.Error(err))
	}

	// 构建审计用的请求 JSON（使用解析后的 grantItems，包含资产信息）
	argsJSON, _ := json.Marshal(grantAuditArgs(input.Description, grantItems))
	auditCtx := ai.WithAuditSource(ctx, "opsctl")
	auditCtx = ai.WithSessionID(auditCtx, sessionID)

	resp, err := approval.RequestApprovalWithToken(sockPath, authToken, approval.ApprovalRequest{
		Type:        "grant",
		SessionID:   sessionID,
		GrantItems:  grantItems,
		Description: input.Description,
	})
	if err != nil {
		writeOpsctlAudit(auditCtx, "grant_submit", string(argsJSON), "", err, &ai.CheckResult{
			Decision: ai.Deny, DecisionSource: ai.SourceGrantDeny,
		})
		fmt.Fprintf(os.Stderr, "Error: desktop app is not running -- grant approval requires the running desktop app\n(%v)\n", err)
		return 1
	}

	if !resp.Approved {
		reason := resp.Reason
		if reason == "" {
			reason = "denied"
		}
		writeOpsctlAudit(auditCtx, "grant_submit", string(argsJSON), "", fmt.Errorf("grant denied: %s", reason), &ai.CheckResult{
			Decision: ai.Deny, DecisionSource: ai.SourceGrantDeny,
		})
		fmt.Fprintf(os.Stderr, "Grant denied: %s\n", reason)
		return 1
	}

	// 审计：grant 批准（如果用户编辑了 items，使用编辑后的内容）
	auditArgs := argsJSON
	if len(resp.EditedItems) > 0 {
		auditArgs, _ = json.Marshal(grantAuditArgs(input.Description, resp.EditedItems))
	}
	writeOpsctlAudit(auditCtx, "grant_submit", string(auditArgs), resp.SessionID, nil, &ai.CheckResult{
		Decision: ai.Allow, DecisionSource: ai.SourceGrantAllow,
	})

	// 输出 session ID 到 stdout
	fmt.Println(resp.SessionID)
	return 0
}

// grantAuditArgs 构建 grant_submit 审计参数，提取首个资产 ID 到顶层供 WriteToolCall 识别
func grantAuditArgs(description string, items []approval.GrantItem) map[string]any {
	args := map[string]any{
		"description": description,
		"items":       items,
	}
	for _, item := range items {
		if item.AssetID > 0 {
			args["asset_id"] = item.AssetID
			break
		}
	}
	return args
}

func printGrantUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl grant submit <asset> <pattern>...          (simple mode)
  opsctl grant submit [options] [asset...] < input   (JSON mode)

Subcommands:
  submit    Submit a batch grant for approval

Simple mode: pass an asset and command patterns as positional arguments.
All patterns are treated as "exec" type. No stdin required.

JSON mode: pipe a JSON grant via stdin for complex grants (multiple types,
per-item asset/group overrides). If approved, the session ID is printed to
stdout. Use it with --session to execute pre-approved operations.

Options:
  --group <name|id>   Default group (repeatable: --group g1 --group g2).
                      Approved commands apply to all assets in the group.

Simple mode examples:
  # Single asset with patterns
  opsctl grant submit web-01 "systemctl *" "df -h" "uptime"

  # Group with patterns
  opsctl grant submit --group production "uptime" "df -h"

JSON mode examples:
  # Single asset
  echo '{"description":"Deploy","items":[{"type":"exec","command":"uptime"}]}' | opsctl grant submit web-01

  # Multiple assets
  echo '{"description":"Health check","items":[{"type":"exec","command":"uptime"}]}' | opsctl grant submit web-01 web-02 web-03

  # Per-item asset/group overrides (no expansion)
  cat <<EOF | opsctl grant submit
  {"description":"Mixed","items":[
    {"type":"exec","asset":"web-01","command":"systemctl restart nginx"},
    {"type":"exec","group":"database","command":"pg_isready"}
  ]}
  EOF

JSON input format:
  {
    "description": "Grant description",
    "items": [
      {"type": "exec", "asset": "web-01", "command": "uptime"},
      {"type": "exec", "group": "production", "command": "systemctl status *"},
      {"type": "exec", "command": "df -h"}
    ]
  }

Item fields:
  type      "exec", "cp", "create", or "update"
  asset     Asset name or ID (targets a single asset)
  group     Group name or ID (targets all assets in the group)
  command   Shell command pattern (supports * wildcard)
  detail    Human-readable description
`)
}
