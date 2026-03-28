package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cago-frame/cago/configs"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/extension"
)

func cmdExt(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printExtUsage()
		return 1
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		return cmdExtList(ctx)
	case "exec":
		return cmdExtExec(ctx, subArgs)
	case "help", "-h", "--help":
		printExtUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown ext command %q\n\nRun 'opsctl ext help' for usage.\n", sub)
		return 1
	}
}

func cmdExtList(ctx context.Context) int {
	mgr := newExtManager()
	if err := mgr.Scan(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	exts := mgr.Extensions()
	if len(exts) == 0 {
		fmt.Println("No extensions installed.")
		return 0
	}
	type listItem struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	items := make([]listItem, 0, len(exts))
	for _, ext := range exts {
		items = append(items, listItem{
			Name:        ext.Manifest.Name,
			DisplayName: ext.Manifest.DisplayName,
			Version:     ext.Manifest.Version,
			Description: ext.Manifest.Description,
		})
	}
	pretty, _ := json.MarshalIndent(items, "", "  ")
	fmt.Println(string(pretty))
	return 0
}

func cmdExtExec(ctx context.Context, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: opsctl ext exec <extension> <tool> [--args '{...}']")
		return 1
	}
	extName := args[0]
	toolName := args[1]

	fs := flag.NewFlagSet("ext exec", flag.ContinueOnError)
	argsJSON := fs.String("args", "{}", "Tool arguments as JSON")
	if err := fs.Parse(args[2:]); err != nil {
		return 1
	}

	// 尝试委托模式：桌面 App 运行时通过 approval socket 执行
	dataDir := bootstrap.AppDataDir()
	socketPath := filepath.Join(dataDir, "approval.sock")
	if _, err := os.Stat(socketPath); err == nil {
		return cmdExtExecDelegate(ctx, socketPath, extName, toolName, *argsJSON)
	}

	// 本地模式：直接加载 WASM 执行
	return cmdExtExecLocal(ctx, extName, toolName, *argsJSON)
}

func cmdExtExecDelegate(ctx context.Context, socketPath, extName, toolName, argsJSON string) int {
	dataDir := bootstrap.AppDataDir()
	authToken, err := bootstrap.ReadAuthToken(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Delegate failed (falling back to local): %v\n", err)
		return cmdExtExecLocal(ctx, extName, toolName, argsJSON)
	}

	resp, err := approval.RequestApprovalWithToken(socketPath, authToken, approval.ApprovalRequest{
		Type:      "ext_tool",
		Extension: extName,
		Tool:      toolName,
		ArgsJSON:  argsJSON,
		Detail:    fmt.Sprintf("opsctl ext exec %s %s", extName, toolName),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Delegate failed (falling back to local): %v\n", err)
		return cmdExtExecLocal(ctx, extName, toolName, argsJSON)
	}
	if !resp.Approved {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Reason)
		return 1
	}

	// Pretty-print result
	var obj any
	if json.Unmarshal([]byte(resp.Result), &obj) == nil {
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Println(resp.Result)
	}
	return 0
}

func cmdExtExecLocal(ctx context.Context, extName, toolName, argsJSON string) int {
	mgr := newExtManager()
	if err := mgr.Scan(); err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning extensions: %v\n", err)
		return 1
	}

	ext := mgr.GetExtension(extName)
	if ext == nil {
		fmt.Fprintf(os.Stderr, "Error: extension %q not found\n", extName)
		return 1
	}

	host := extension.NewExtensionHost(nil)
	defer host.Close(ctx)
	plugin, err := host.LoadPlugin(ctx, ext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading extension: %v\n", err)
		return 1
	}

	result, err := plugin.CallTool(ctx, toolName, json.RawMessage(argsJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Pretty-print JSON output
	var obj any
	if json.Unmarshal(result, &obj) == nil {
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Println(string(result))
	}
	return 0
}

func newExtManager() *extension.Manager {
	dataDir := bootstrap.AppDataDir()
	extensionsDir := filepath.Join(dataDir, "extensions")
	return extension.NewManager(extensionsDir, configs.Version)
}

func printExtUsage() {
	fmt.Fprint(os.Stderr, `opsctl ext - WASM extensions

Usage:
  opsctl ext <command> [arguments]

Commands:
  list                              List installed extensions
  exec <extension> <tool> [--args]  Execute an extension tool

Install/remove extensions via the desktop app Settings → Extensions.

Examples:
  opsctl ext list
  opsctl ext exec oss list_buckets --args '{"asset_id": 1}'
`)
}
