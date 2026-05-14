package ai

import (
	"github.com/cago-frame/agents/tool"
)

// auditWriter 由 auditPostHook 在每次工具调用后异步写审计日志。
// 包级变量便于测试时替换（参考 audit_test.go）。
var auditWriter AuditWriter = NewDefaultAuditWriter()

// Tools 返回所有 cago 原生工具实例。各 *_tools_*.go 文件直接以 *tool.RawTool 字面量定义工具，
// 不再依赖 rawTool / makeSchema 等包装：handler 内部按 cago 原生 (ctx, in) → (*ToolResultBlock, error)
// 签名构造返回值，权限决策通过 setCheckResult 透出，审计写入由全局 auditPostHook 异步完成。
func Tools() []tool.Tool {
	tools := make([]tool.Tool, 0, 24)
	tools = append(tools, assetTools()...)
	tools = append(tools, execTools()...)
	tools = append(tools, dataTools()...)
	tools = append(tools, kafkaTools()...)
	tools = append(tools, extTools()...)
	return tools
}
