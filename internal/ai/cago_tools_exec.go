package ai

import "github.com/cago-frame/agents/tool"

// execCagoTools SSH / 文件传输 / grant 申请 共 4 个工具。
// 命令类工具（run_command / upload_file / download_file）标 Serial：跟现有"整轮串行"语义对齐，
// 防止同会话内并发产生不可预期的资源争用（同一 SSH 连接复用、SFTP 句柄、审计排序等）。
// request_permission 不直接执行命令，但语义上属于"重操作触发面板"，沿用 Serial 以保证审批弹窗串行可控。
func execCagoTools() []tool.Tool {
	return []tool.Tool{
		rawTool(
			"run_command",
			"Execute a shell command on a remote server via SSH and return the output. Credentials are resolved automatically from the app's encrypted store — do not ask the user for passwords. IMPORTANT: The command runs on the REMOTE server, not locally.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "Target server asset ID. Use list_assets to find available IDs."},
				paramSpec{name: "command", typ: "string", required: true, desc: "Shell command to execute on the remote server."},
			),
			true,
			handleRunCommand,
		),
		rawTool(
			"upload_file",
			"Upload a local file to a remote server via SFTP. Credentials are resolved automatically.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "Target server asset ID."},
				paramSpec{name: "local_path", typ: "string", required: true, desc: "Absolute path of the local file to upload."},
				paramSpec{name: "remote_path", typ: "string", required: true, desc: "Destination path on the remote server (including filename)."},
			),
			true,
			handleUploadFile,
		),
		rawTool(
			"download_file",
			"Download a file from a remote server to the local machine via SFTP. Credentials are resolved automatically.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "Source server asset ID."},
				paramSpec{name: "remote_path", typ: "string", required: true, desc: "Path of the file on the remote server."},
				paramSpec{name: "local_path", typ: "string", required: true, desc: "Absolute local path to save the file (including filename)."},
			),
			true,
			handleDownloadFile,
		),
		rawTool(
			"request_permission",
			"Request approval for grant of command patterns BEFORE executing them. Submit command patterns (one per line, supports '*' wildcard) for one or more target assets. The user will review and may edit the patterns before approving. Once approved, subsequent run_command/exec_sql/exec_redis/exec_mongo/exec_k8s/kafka_* calls matching any approved pattern will be auto-approved.",
			makeSchema(
				paramSpec{name: "items", typ: "string", required: true, desc: `JSON array of items. Each item: {"asset_id": <number>, "command_patterns": "<patterns separated by newline>"}. Example: [{"asset_id":1,"command_patterns":"cat /var/log/*\nsystemctl * nginx"},{"asset_id":2,"command_patterns":"SELECT * FROM users"}]`},
				paramSpec{name: "reason", typ: "string", required: true, desc: "Brief explanation of why these permissions are needed."},
			),
			true,
			handleRequestGrant,
		),
	}
}
