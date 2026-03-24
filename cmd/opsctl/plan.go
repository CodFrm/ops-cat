package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/google/uuid"
)

// planInput 从 stdin 读取的计划 JSON 格式
type planInput struct {
	Description string          `json:"description"`
	Items       []planInputItem `json:"items"`
}

type planInputItem struct {
	Type    string `json:"type"`    // "exec", "cp", "create", "update"
	Asset   string `json:"asset"`   // 资产名称或 ID
	Group   string `json:"group"`   // 资产组名称或 ID
	Command string `json:"command"` // 命令模式
	Detail  string `json:"detail"`
}

func cmdPlan(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printPlanUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	switch args[0] {
	case "submit":
		return cmdPlanSubmit(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown plan subcommand %q\n\nRun 'opsctl plan --help' for usage.\n", args[0]) //nolint:gosec // args[0] is from CLI args
		return 1
	}
}

func cmdPlanSubmit(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("plan submit", flag.ContinueOnError)
	groupFlag := fs.String("group", "", "Default group for items without asset/group (name or ID)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	remaining := fs.Args()

	// 解析默认资产（位置参数）
	var defaultAssetID int64
	var defaultAssetName string
	if len(remaining) > 0 {
		asset, err := resolveAsset(ctx, remaining[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		defaultAssetID = asset.ID
		defaultAssetName = asset.Name
	}

	// 解析默认组（--group 参数）
	var defaultGroupID int64
	var defaultGroupName string
	if *groupFlag != "" {
		gid, gname, err := resolveGroup(ctx, *groupFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		defaultGroupID = gid
		defaultGroupName = gname
	}

	// 从 stdin 读取 JSON
	var input planInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", err)
		return 1
	}

	if len(input.Items) == 0 {
		fmt.Fprintln(os.Stderr, "Error: plan must contain at least one item")
		return 1
	}

	// 解析资产/组名称，构建 PlanItems
	var planItems []approval.PlanItem
	for i, item := range input.Items {
		var assetID int64
		var assetName string
		var groupID int64
		var groupName string

		if item.Asset != "" {
			// item 指定了资产，优先使用
			asset, err := resolveAsset(ctx, item.Asset)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: item %d: %v\n", i+1, err)
				return 1
			}
			assetID = asset.ID
			assetName = asset.Name
		} else if item.Group != "" {
			// item 指定了组
			gid, gname, err := resolveGroup(ctx, item.Group)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: item %d: %v\n", i+1, err)
				return 1
			}
			groupID = gid
			groupName = gname
		} else if defaultAssetID > 0 {
			// 使用命令行指定的默认资产
			assetID = defaultAssetID
			assetName = defaultAssetName
		} else if defaultGroupID > 0 {
			// 使用命令行指定的默认组
			groupID = defaultGroupID
			groupName = defaultGroupName
		}

		planItems = append(planItems, approval.PlanItem{
			Type:      item.Type,
			AssetID:   assetID,
			AssetName: assetName,
			GroupID:   groupID,
			GroupName: groupName,
			Command:   item.Command,
			Detail:    item.Detail,
		})
	}

	// 生成 UUIDv4
	sessionID := uuid.New().String()

	// 通过 socket 发送 plan 请求
	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)

	resp, err := approval.RequestApproval(sockPath, approval.ApprovalRequest{
		Type:        "plan",
		SessionID:   sessionID,
		PlanItems:   planItems,
		Description: input.Description,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: desktop app is not running -- plan approval requires the running desktop app\n(%v)\n", err)
		return 1
	}

	if !resp.Approved {
		reason := resp.Reason
		if reason == "" {
			reason = "denied"
		}
		fmt.Fprintf(os.Stderr, "Plan denied: %s\n", reason)
		return 1
	}

	// 输出 session ID 到 stdout
	fmt.Println(resp.SessionID)
	return 0
}

func printPlanUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl plan submit [options] [asset]

Subcommands:
  submit    Submit a batch plan for approval

Submit reads a JSON plan from stdin and sends it to the desktop app for approval.
If approved, the session ID is printed to stdout. Use it with --session to
execute pre-approved operations without individual approval dialogs.

Options:
  --group <name|id>   Default group for items without explicit asset/group.
                      Approved commands apply to all assets in the group.

A positional asset argument sets the default asset for items without explicit
asset/group. The --group flag and positional asset are mutually exclusive in
effect: items use asset first, then group, then the CLI defaults.

Input JSON format:
  {
    "description": "Plan description",
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

Scope priority (per item): asset > group > CLI default asset > CLI --group

Examples:
  # Submit plan for a specific asset
  echo '{"description":"Deploy","items":[{"type":"exec","command":"uptime"}]}' | opsctl plan submit web-01

  # Submit plan for a group (all assets in "production")
  echo '{"description":"Health check","items":[{"type":"exec","command":"uptime"}]}' | opsctl plan submit --group production

  # Mixed: per-item asset/group overrides
  cat <<EOF | opsctl plan submit
  {"description":"Mixed","items":[
    {"type":"exec","asset":"web-01","command":"systemctl restart nginx"},
    {"type":"exec","group":"database","command":"pg_isready"}
  ]}
  EOF
`)
}
