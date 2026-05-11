package aiagent

import (
	"sync"

	"github.com/opskat/opskat/internal/ai"
)

// toolDecisionStore 是单个 ConvHandle 内 policy hook → audit hook 的旁路通道：
// PreToolUse 把 policy 决策按 ToolUseID 存进来，PostToolUse 在 audit 阶段
// LoadAndDelete 拿走。cago 的 PreHook/PostHook 之间没有可携带数据的官方 ctx
// 通道（PreHook 的 ctx 修改在外层 Run 里不持久），所以走这条进程内 map。
//
// 生命周期：每个 ConvHandle 在 buildAgentOptions 里 new 一个；handle 关闭即
// 释放（sync.Map 会被 GC 回收）。ToolUseID 在 cago 内是 turn-uniq，cross-handle
// 共享会冲突，所以**绝不要**在 Manager 层共享同一个 store。
type toolDecisionStore struct {
	m sync.Map // toolUseID → *ai.CheckResult
}

func newToolDecisionStore() *toolDecisionStore { return &toolDecisionStore{} }

// Stash 写入一次 policy 决策。同一 toolUseID 多次写后写覆盖前（hook 链路里
// policy 只调用一次，但 Confirm 分支收到用户响应后会更新源/来源字段）。
func (s *toolDecisionStore) Stash(toolUseID string, r *ai.CheckResult) {
	if toolUseID == "" || r == nil {
		return
	}
	s.m.Store(toolUseID, r)
}

// Pop 取走并删除一条决策；audit hook 应当在每次 PostToolUse 调用一次。
// 不存在时返回 nil（早期 turn 里 tool 在 policy 门控之外，audit 自然没决策）。
func (s *toolDecisionStore) Pop(toolUseID string) *ai.CheckResult {
	if toolUseID == "" {
		return nil
	}
	v, ok := s.m.LoadAndDelete(toolUseID)
	if !ok {
		return nil
	}
	return v.(*ai.CheckResult)
}
