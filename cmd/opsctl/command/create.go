package command

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/assettype"
)

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
		assetType := fs.String("type", "ssh", `Asset type: "ssh", "database", "redis", "mongodb", or "k8s"`)
		name := fs.String("name", "", "Display name for the asset (required)")
		host := fs.String("host", "", "Hostname or IP address (required)")
		port := fs.Int("port", 0, "Port number (default: auto by type)")
		username := fs.String("username", "", "Login username (required)")
		authType := fs.String("auth-type", "password", "SSH auth method: password or key")
		driver := fs.String("driver", "", `Database driver: "mysql" or "postgresql" (required for database type)`)
		database := fs.String("database", "", "Default database name (for database type)")
		readOnly := fs.Bool("read-only", false, "Enable read-only mode (for database type)")
		sshAsset := fs.String("ssh-asset", "", "SSH asset name/ID for tunnel connection (for database/redis/k8s)")
		kubeconfig := fs.String("kubeconfig", "", "Kubeconfig YAML content (k8s type)")
		kubeconfigFile := fs.String("kubeconfig-file", "", "Path to kubeconfig YAML file (k8s type)")
		k8sNamespace := fs.String("namespace", "", "Default Kubernetes namespace (k8s type)")
		k8sContext := fs.String("context", "", "Kubeconfig context name (k8s type)")
		groupID := fs.Int64("group-id", 0, "Group ID to assign the asset to (0 = ungrouped)")
		description := fs.String("description", "", "Optional description or notes")
		icon := fs.String("icon", "", "Icon name (e.g. server, kubernetes, docker)")
		fs.Usage = func() { printCreateAssetUsage() }
		_ = fs.Parse(args[1:])

		if *kubeconfig == "" && *kubeconfigFile != "" {
			data, readErr := os.ReadFile(*kubeconfigFile)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "Error reading kubeconfig file: %v\n", readErr)
				return 1
			}
			*kubeconfig = string(data)
		}

		if *name == "" {
			fmt.Fprintln(os.Stderr, "Error: --name is required")
			fmt.Fprintln(os.Stderr)
			printCreateAssetUsage()
			return 1
		}
		if *assetType == "k8s" {
			if *kubeconfig == "" {
				fmt.Fprintln(os.Stderr, "Error: --kubeconfig or --kubeconfig-file is required for k8s assets")
				fmt.Fprintln(os.Stderr)
				printCreateAssetUsage()
				return 1
			}
		} else if *host == "" || *username == "" {
			fmt.Fprintln(os.Stderr, "Error: --host and --username are required")
			fmt.Fprintln(os.Stderr)
			printCreateAssetUsage()
			return 1
		}

		// 自动设置默认端口
		if *port == 0 {
			if h, ok := assettype.Get(*assetType); ok {
				*port = h.DefaultPort()
			}
			// Database driver-specific override
			if *assetType == "database" && *driver == "postgresql" {
				*port = 5432
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
		if *assetType == "k8s" {
			params["kubeconfig"] = *kubeconfig
			if *k8sNamespace != "" {
				params["namespace"] = *k8sNamespace
			}
			if *k8sContext != "" {
				params["context"] = *k8sContext
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
		if *icon != "" {
			params["icon"] = *icon
		}
		// Require approval
		detail := fmt.Sprintf("opsctl create asset --type %s --name %s", *assetType, *name)
		if *assetType != "k8s" {
			detail = fmt.Sprintf("%s --host %s", detail, *host)
		}
		if _, err := requireApproval(ctx, approval.ApprovalRequest{
			Type:      "create",
			Detail:    detail,
			SessionID: session,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}

		return callHandler(ctx, handlers, "add_asset", params)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: asset\n", resource)
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
		icon := fs.String("icon", "", "New icon name (e.g. server, kubernetes, docker)")
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
		if *icon != "" {
			params["icon"] = *icon
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
		fmt.Fprintf(os.Stderr, "Error: unknown resource %q. Supported: asset\n", resource)
		return 1
	}
}

func printCreateUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl create <resource> [flags]

Resources:
  asset     Create a new asset (ssh, database, redis, mongodb, or k8s)

Run 'opsctl create asset --help' for details.
`)
}

func printCreateAssetUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] create asset [flags]

Required Flags:
  --name <string>         Display name for the asset
  --host <string>         Hostname or IP address (required except k8s)
  --username <string>     Login username (required except k8s)

Optional Flags:
  --type <string>         Asset type: "ssh" (default), "database", "redis", "mongodb", or "k8s"
  --port <int>            Port number (default: auto by type — 22/3306/5432/6379/27017)
  --auth-type <string>    SSH auth method: "password" or "key" (SSH type only)
  --driver <string>       Database driver: "mysql" or "postgresql" (database type, required)
  --database <string>     Default database name (database type)
  --read-only             Enable read-only mode (database type)
  --kubeconfig <string>   Kubeconfig YAML content (k8s type)
  --kubeconfig-file <path>
                          Path to kubeconfig YAML file (k8s type)
  --namespace <string>    Default Kubernetes namespace (k8s type)
  --context <string>      Kubeconfig context name (k8s type)
  --ssh-asset <asset>     SSH asset name/ID for tunnel connection (database/redis/k8s types)
  --group-id <int>        Group ID to assign the asset to (0 = ungrouped)
  --description <string>  Optional description or notes
  --icon <string>         Icon name (default: auto by type)

Available Icons:
  Infrastructure: server, database, cloud, monitor, laptop, router, hard-drive,
                  globe, shield, container, cpu, network
  Cloud:          aws, azure, gcp, alicloud, tencentcloud, huaweicloud, cloudflare
  DB/Middleware:   mysql, postgresql, redis, mongodb, elasticsearch, kafka, mariadb,
                  sqlite, rabbitmq, etcd, clickhouse
  System/OS:      docker, kubernetes, linux, windows, ubuntu, centos, debian,
                  redhat, macos
  DevOps:         nginx, grafana, prometheus

Approval:
  Requires desktop app approval. Session auto-created if not specified.

Examples:
  opsctl create asset --name "Web Server" --host 10.0.0.1 --username root
  opsctl create asset --type database --driver mysql --name "Prod DB" --host db.internal --username app
  opsctl create asset --type database --driver postgresql --name "Analytics" --host pg.internal --port 5432 --username readonly --read-only
  opsctl create asset --type redis --name "Cache" --host redis.internal --username default
  opsctl create asset --type database --driver mysql --name "DB via SSH" --host 127.0.0.1 --username app --ssh-asset web-server
  opsctl create asset --type k8s --name "Prod Cluster" --kubeconfig-file ~/.kube/config --context prod
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
  --icon <string>         New icon name (see 'opsctl create asset --help' for list)

Approval:
  Requires desktop app approval. Session auto-created if not specified.

Examples:
  opsctl update asset web-server --name "New Name"
  opsctl update asset 1 --host 192.168.1.100 --port 2222
  opsctl update asset web-server --group-id 3
`)
}
