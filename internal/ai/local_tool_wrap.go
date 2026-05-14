package ai

import "github.com/cago-frame/agents/tool"

// localRenames 把 cago 默认本地工具改名 + 描述前置警示。
// 仅覆盖三件"动作类"工具：bash_output / kill_shell 不改名（cago tool/bash/background.go
// 的 runtime 返回文案写死了这两个名字），read/grep/find/ls/task_* 是惰性查询，
// LLM 不会拿它们误操作远程资产，无需改名。
var localRenames = map[string]string{
	"bash":  "local_bash",
	"write": "local_write",
	"edit":  "local_edit",
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
