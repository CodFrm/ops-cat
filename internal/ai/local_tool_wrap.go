package ai

import "github.com/cago-frame/agents/tool"

// localRenames 把 cago 默认本地工具改名 + 描述前置警示。
// 覆盖 7 件套：bash/write/edit/read/grep/find/ls —— 但凡是操作或查询用户本机文件系统、
// shell 的工具一律加 local_ 前缀，让 LLM 在工具列表里就一眼区分本地 vs 远程，
// 杜绝"用户说服务器 /etc/nginx 而 LLM 调 read /etc/nginx"这类误用。
//
// 不在列表里的工具：
//   - bash_output / kill_shell：cago tool/bash/background.go 的 runtime 返回文案
//     写死了这两个名字（"Use bash_output with shell_id=..."），改名会让 LLM 困惑。
//   - task_*：任务管理，与文件/shell 操作正交，不会被 LLM 用错。
var localRenames = map[string]string{
	"bash":  "local_bash",
	"write": "local_write",
	"edit":  "local_edit",
	"read":  "local_read",
	"grep":  "local_grep",
	"find":  "local_find",
	"ls":    "local_ls",
}

// localWarning 前置到被重命名工具的 Description 前面，提醒 LLM 这些是本机工具，
// 远程操作必须改用 run_command / exec_sql / exec_redis 等。
const localWarning = "LOCAL MACHINE ONLY — do not use for actions on " +
	"remote SSH/DB/Redis/Mongo/K8s assets. For remote work use " +
	"run_command / exec_sql / exec_redis / exec_mongo / exec_k8s. "

// WrapLocalTool 实现 cago coding.WithToolDecorator 的 decorator 签名。
// 命中 localRenames 的工具返回带新名字 + 警示描述的浅拷贝；其它工具原样返回。
// 仅对 *tool.RawTool 生效——cago 所有内置工具都是这个类型。
func WrapLocalTool(t tool.Tool) tool.Tool {
	raw, ok := t.(*tool.RawTool)
	if !ok {
		return t
	}
	newName, hit := localRenames[raw.NameStr]
	if !hit {
		return t
	}
	clone := *raw
	clone.NameStr = newName
	clone.DescStr = localWarning + raw.DescStr
	return &clone
}
