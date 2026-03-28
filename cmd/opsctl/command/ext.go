package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cago-frame/cago/configs"
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
	case "install":
		return cmdExtInstall(ctx, subArgs)
	case "remove":
		return cmdExtRemove(ctx, subArgs)
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

func cmdExtInstall(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("ext install", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: opsctl ext install <path>")
		return 1
	}
	sourcePath := fs.Arg(0)

	mgr := newExtManager()
	info, err := mgr.Install(sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Installed extension %q v%s\n", info.Manifest.Name, info.Manifest.Version)
	return 0
}

func cmdExtRemove(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: opsctl ext remove <name>")
		return 1
	}
	name := args[0]

	mgr := newExtManager()
	if err := mgr.Remove(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Removed extension %q\n", name)
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

	result, err := plugin.CallTool(ctx, toolName, json.RawMessage(*argsJSON))
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
	fmt.Fprint(os.Stderr, `opsctl ext - Manage WASM extensions

Usage:
  opsctl ext <command> [arguments]

Commands:
  list                              List installed extensions
  install <path>                    Install extension from local path
  remove <name>                     Remove an installed extension
  exec <extension> <tool> [--args]  Execute an extension tool

Examples:
  opsctl ext list
  opsctl ext install ./my-extension/
  opsctl ext remove oss
  opsctl ext exec oss list_buckets --args '{"prefix": "data/"}'
`)
}
