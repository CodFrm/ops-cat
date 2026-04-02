package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

func cmdExt(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printExtUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		return cmdExtList()
	case "exec":
		return cmdExtExec(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown ext subcommand %q\n\nRun 'opsctl ext --help' for usage.\n", sub)
		return 1
	}
}

// cmdExtList lists installed extensions by scanning manifest files.
func cmdExtList() int {
	extDir := filepath.Join(bootstrap.AppDataDir(), "extensions")

	entries, err := os.ReadDir(extDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("[]")
			return 0
		}
		fmt.Fprintf(os.Stderr, "Error: read extensions directory: %v\n", err)
		return 1
	}

	type extInfo struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		DisplayName string   `json:"displayName,omitempty"`
		Description string   `json:"description,omitempty"`
		Tools       []string `json:"tools,omitempty"`
	}

	var results []extInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(extDir, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath) //nolint:gosec // path constructed from ReadDir within extensions directory
		if err != nil {
			continue // skip dirs without manifest
		}
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			I18n    struct {
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
			} `json:"i18n"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		info := extInfo{
			Name:        manifest.Name,
			Version:     manifest.Version,
			DisplayName: manifest.I18n.DisplayName,
			Description: manifest.I18n.Description,
		}
		for _, t := range manifest.Tools {
			info.Tools = append(info.Tools, t.Name)
		}
		results = append(results, info)
	}

	if results == nil {
		results = []extInfo{}
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
	return 0
}

// cmdExtExec executes an extension tool: delegate to desktop app first, fallback to local.
func cmdExtExec(args []string) int {
	if len(args) < 2 || args[0] == "-h" || args[0] == "--help" {
		printExtExecUsage()
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
			return 0
		}
		return 1
	}

	extName := args[0]
	toolName := args[1]

	// Parse --args flag from remaining args
	var toolArgs = json.RawMessage("{}")
	for i := 2; i < len(args); i++ {
		if args[i] == "--args" && i+1 < len(args) {
			toolArgs = json.RawMessage(args[i+1])
			break
		}
		// Support --args='{...}' form
		if strings.HasPrefix(args[i], "--args=") {
			toolArgs = json.RawMessage(strings.TrimPrefix(args[i], "--args="))
			break
		}
	}

	// Validate JSON
	if !json.Valid(toolArgs) {
		fmt.Fprintf(os.Stderr, "Error: --args must be valid JSON\n")
		return 1
	}

	// Try delegate mode first (desktop app running)
	result, err := delegateExtExec(extName, toolName, toolArgs)
	if err == nil {
		printToolResult(result)
		return 0
	}

	// If desktop app is not running, fallback to local mode
	if strings.Contains(err.Error(), "cannot connect") {
		return localExtExec(extName, toolName, toolArgs)
	}

	// Delegation succeeded but tool execution failed
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	return 1
}

// delegateExtExec sends an ext_tool request to the desktop app via approval socket.
func delegateExtExec(extName, toolName string, toolArgs json.RawMessage) (string, error) {
	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)

	token, err := bootstrap.ReadAuthToken(dataDir)
	if err != nil {
		logger.Default().Warn("read auth token", zap.Error(err))
	}

	resp, err := approval.RequestApprovalWithToken(sockPath, token, approval.ApprovalRequest{
		Type:      "ext_tool",
		Extension: extName,
		Tool:      toolName,
		ToolArgs:  toolArgs,
	})
	if err != nil {
		return "", err
	}

	if resp.ToolError != "" {
		return "", fmt.Errorf("%s", resp.ToolError)
	}

	return resp.ToolResult, nil
}

// printToolResult pretty-prints a JSON tool result to stdout.
func printToolResult(result string) {
	var obj any
	if json.Unmarshal([]byte(result), &obj) == nil {
		pretty, err := json.MarshalIndent(obj, "", "  ")
		if err == nil {
			fmt.Println(string(pretty))
			return
		}
	}
	fmt.Println(result)
}

func printExtUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl ext <subcommand> [arguments]

Subcommands:
  list                              List installed extensions
  exec <extension> <tool> [--args]  Execute an extension tool

When the desktop app is running, ext exec delegates execution to it.
Otherwise, extensions are loaded and executed locally via WASM.

Examples:
  opsctl ext list
  opsctl ext exec oss list_buckets --args '{"asset_id": 1}'
  opsctl ext exec oss upload_file --args='{"asset_id": 1, "bucket": "my-bucket", "key": "file.txt"}'
`)
}

func printExtExecUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl ext exec <extension> <tool> [--args '<json>']

Arguments:
  extension   Extension name (e.g., "oss")
  tool        Tool name within the extension (e.g., "list_buckets")
  --args      Tool arguments as a JSON object (default: "{}")

Execution Mode:
  If the desktop app is running, the tool is executed via delegation (using the
  app's loaded extensions and credentials). Otherwise, the extension is loaded
  locally from the extensions directory and executed via WASM.

Examples:
  opsctl ext exec oss list_buckets --args '{"asset_id": 1}'
  opsctl ext exec kubernetes get_pods --args '{"asset_id": 2, "namespace": "default"}'
`)
}
