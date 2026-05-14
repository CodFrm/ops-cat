package ai

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// execTools SSH / 串口 / 文件传输 / grant 申请。
// 命令类工具（run_command / run_serial_command / upload_file / download_file）标 Serial：跟原"整轮串行"语义对齐，
// 防止同会话内并发产生不可预期的资源争用（同一 SSH 连接复用、SFTP 句柄、审计排序等）。
// request_permission 不直接执行命令，但语义上属于"重操作触发面板"，沿用 Serial 以保证审批弹窗串行可控。
func execTools() []tool.Tool {
	return []tool.Tool{
		&tool.RawTool{
			NameStr: "run_command",
			DescStr: "Execute a shell command on a remote server via SSH and return the output. Credentials are resolved automatically from the app's encrypted store — do not ask the user for passwords. IMPORTANT: The command runs on the REMOTE server, not locally.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id": {Type: "number", Description: "Target server asset ID. Use list_assets to find available IDs."},
					"command":  {Type: "string", Description: "Shell command to execute on the remote server."},
				},
				Required: []string{"asset_id", "command"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleRunCommand(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "run_serial_command",
			DescStr: "Send a command to a serial port device (e.g. network switch, firewall console) and return the output. The serial session must already be connected by the user in the terminal tab. The command is sent over the existing serial connection and output is collected until silence (2s) or max timeout (15s). Use this for H3C, Huawei, Cisco and other console-connected devices.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id": {Type: "number", Description: "Target serial asset ID. Use list_assets with asset_type='serial' to find it."},
					"command":  {Type: "string", Description: "Command to send to the serial device (e.g. 'display version', 'show ip interface brief')."},
				},
				Required: []string{"asset_id", "command"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleRunSerialCommand(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "upload_file",
			DescStr: "Upload a local file to a remote server via SFTP. Credentials are resolved automatically.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":    {Type: "number", Description: "Target server asset ID."},
					"local_path":  {Type: "string", Description: "Absolute path of the local file to upload."},
					"remote_path": {Type: "string", Description: "Destination path on the remote server (including filename)."},
				},
				Required: []string{"asset_id", "local_path", "remote_path"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleUploadFile(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "download_file",
			DescStr: "Download a file from a remote server to the local machine via SFTP. Credentials are resolved automatically.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":    {Type: "number", Description: "Source server asset ID."},
					"remote_path": {Type: "string", Description: "Path of the file on the remote server."},
					"local_path":  {Type: "string", Description: "Absolute local path to save the file (including filename)."},
				},
				Required: []string{"asset_id", "remote_path", "local_path"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleDownloadFile(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "batch_command",
			DescStr: "Execute commands on multiple assets in parallel. Supports exec (SSH), sql (database), and redis command types. Each command is policy-checked; items needing user confirmation are batched into a single approval prompt. Results are returned per-asset (success or error). Prefer this over looping run_command/exec_sql/exec_redis when targeting >1 asset.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"commands": {Type: "string", Description: `JSON array of commands. Each item: {"asset": "name-or-id", "type": "exec|sql|redis", "command": "..."}. Type defaults to "exec". Example: [{"asset":"web-1","type":"exec","command":"uptime"},{"asset":"42","type":"sql","command":"SELECT VERSION()"}]`},
				},
				Required: []string{"commands"},
			},
			// batch 内部自己做并发控制（max 10），父级 dispatcher 不需要再串行。
			IsSerial: false,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleBatchCommand(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "request_permission",
			DescStr: "Request approval for grant of command patterns BEFORE executing them. Submit command patterns (one per line, supports '*' wildcard) for one or more target assets. The user will review and may edit the patterns before approving. Once approved, subsequent run_command/exec_sql/exec_redis/exec_mongo/exec_k8s/kafka_* calls matching any approved pattern will be auto-approved.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"items":  {Type: "string", Description: `JSON array of items. Each item: {"asset_id": <number>, "command_patterns": "<patterns separated by newline>"}. Example: [{"asset_id":1,"command_patterns":"cat /var/log/*\nsystemctl * nginx"},{"asset_id":2,"command_patterns":"SELECT * FROM users"}]`},
					"reason": {Type: "string", Description: "Brief explanation of why these permissions are needed."},
				},
				Required: []string{"items", "reason"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleRequestGrant(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
	}
}
