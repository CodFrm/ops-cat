package ai

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/cago-frame/agents/agent"
)

// LocalToolApprovalRequest 是 LocalToolGate 发往前端的本地工具审批载荷。
//
// 与 ApprovalItem 不同：本地工具没有 asset/group 概念，只有命令/路径本体；
// SubCommands 用于 bash 复合命令的展示与默认 pattern 生成。
type LocalToolApprovalRequest struct {
	ToolName        string   // "bash" | "write" | "edit"
	Command         string   // bash: 原始 command；write/edit: path
	Detail          string   // write 内容预览 / edit 改动预览，bash 留空
	SubCommands     []string // bash 按 mvdan.cc/sh 拆分的子命令；write/edit 为单条 = path
	DefaultPatterns []string // 默认 pattern（"git pull" → "git *"，path 默认原值），前端预填可编辑
}

// LocalToolConfirmFunc 由上层（App）注入，发起 Wails 事件并阻塞等待用户响应。
type LocalToolConfirmFunc func(ctx context.Context, req LocalToolApprovalRequest) ApprovalResponse

// LocalToolGate 拦截 coding system 的 bash/write/edit 工具调用。
//
// 行为对齐 run_command：bash 按 && / || / ; / | 拆出子命令，全部命中已存 pattern
// （* 通配，path.Match 语义）才放行；否则发起审批。"本次会话允许" 把用户编辑后的
// pattern 写入会话内存白名单，键为 conversationID。
type LocalToolGate struct {
	confirm LocalToolConfirmFunc
	allowed sync.Map // map[int64][]allowEntry
}

type allowEntry struct {
	Tool    string
	Pattern string
}

// NewLocalToolGate 构造 gate。
// confirm 为 nil 时测试场景可用，但任何未命中调用将直接 Deny。
func NewLocalToolGate(confirm LocalToolConfirmFunc) *LocalToolGate {
	return &LocalToolGate{confirm: confirm}
}

// Hook 返回挂到 coding system 的 PreToolUseHook。
func (g *LocalToolGate) Hook() agent.PreToolUseHook {
	return func(ctx context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		convID := GetConversationID(ctx)
		toolName := in.ToolName

		subjects := extractSubjects(toolName, in.Input)
		if len(subjects) == 0 {
			// 输入异常（缺字段 / 空字符串），不阻塞，让工具自身处理并返回错误。
			return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
		}

		if g.allMatch(convID, toolName, subjects) {
			return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
		}

		if g.confirm == nil {
			return &agent.PreToolUseOutput{
				Decision:   agent.DecisionDeny,
				DenyReason: fmt.Sprintf("no approval mechanism configured for local tool %s", toolName),
			}, nil
		}

		req := LocalToolApprovalRequest{
			ToolName:        toolName,
			Command:         primaryCommand(toolName, in.Input),
			Detail:          detailOf(toolName, in.Input),
			SubCommands:     subjects,
			DefaultPatterns: defaultPatterns(toolName, subjects),
		}
		resp := g.confirm(ctx, req)

		switch resp.Decision {
		case "deny":
			return &agent.PreToolUseOutput{
				Decision:   agent.DecisionDeny,
				DenyReason: fmt.Sprintf("USER DENIED: user rejected local tool %s. Stop the current task.", toolName),
			}, nil
		case "allowAll":
			for _, p := range patternsFromResponse(toolName, subjects, resp.EditedItems) {
				g.remember(convID, toolName, p)
			}
			return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
		default:
			// "allow" 或其他都按单次允许处理；不写白名单。
			return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
		}
	}
}

// Reset 清空指定 convID 的白名单。在删除会话或重置 provider 时调用。
func (g *LocalToolGate) Reset(convID int64) {
	g.allowed.Delete(convID)
}

func (g *LocalToolGate) remember(convID int64, tool, pattern string) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return
	}
	cur, _ := g.allowed.LoadOrStore(convID, []allowEntry{})
	entries := cur.([]allowEntry)
	for _, e := range entries {
		if e.Tool == tool && e.Pattern == pattern {
			return
		}
	}
	entries = append(entries, allowEntry{Tool: tool, Pattern: pattern})
	g.allowed.Store(convID, entries)
}

func (g *LocalToolGate) allMatch(convID int64, tool string, subjects []string) bool {
	cur, ok := g.allowed.Load(convID)
	if !ok {
		return false
	}
	entries := cur.([]allowEntry)
	if len(entries) == 0 {
		return false
	}
	for _, sub := range subjects {
		matched := false
		for _, e := range entries {
			if e.Tool != tool {
				continue
			}
			if matchLocalPattern(tool, e.Pattern, sub) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// matchLocalPattern：bash 用 MatchCommandRule（与 run_command 一致，支持 *），
// write/edit 用 path.Match（POSIX glob，* 不跨 /）。
func matchLocalPattern(tool, pattern, subject string) bool {
	if pattern == "*" || pattern == subject {
		return true
	}
	switch tool {
	case "bash":
		return MatchCommandRule(pattern, subject)
	default:
		ok, _ := path.Match(pattern, subject)
		return ok
	}
}

// extractSubjects 解析工具输入得到需要审批的主体列表。
func extractSubjects(tool string, in map[string]any) []string {
	switch tool {
	case "bash":
		cmd, _ := in["command"].(string)
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return nil
		}
		subs, err := ExtractSubCommands(cmd)
		if err != nil || len(subs) == 0 {
			return []string{cmd}
		}
		return subs
	case "write", "edit":
		p, _ := in["path"].(string)
		p = strings.TrimSpace(p)
		if p == "" {
			return nil
		}
		return []string{p}
	}
	return nil
}

func primaryCommand(tool string, in map[string]any) string {
	switch tool {
	case "bash":
		cmd, _ := in["command"].(string)
		return cmd
	case "write", "edit":
		p, _ := in["path"].(string)
		return p
	}
	return ""
}

// detailOf 给前端展示用的补充内容：write 显示前若干内容，edit 显示 diff 摘要。
func detailOf(tool string, in map[string]any) string {
	switch tool {
	case "write":
		c, _ := in["content"].(string)
		return truncatePreview(c, 800)
	case "edit":
		edits, ok := in["edits"].([]any)
		if !ok {
			return ""
		}
		var b strings.Builder
		for i, e := range edits {
			m, _ := e.(map[string]any)
			oldT, _ := m["oldText"].(string)
			newT, _ := m["newText"].(string)
			fmt.Fprintf(&b, "--- edit %d ---\n- %s\n+ %s\n", i+1, truncatePreview(oldT, 200), truncatePreview(newT, 200))
			if b.Len() > 800 {
				b.WriteString("...(truncated)")
				break
			}
		}
		return b.String()
	}
	return ""
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// defaultPatterns 为每条 subject 生成默认 pattern。
// bash: 取第一个 token + " *"；write/edit: 原 path（用户再编辑加 * 或 **）。
func defaultPatterns(tool string, subjects []string) []string {
	out := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if tool == "bash" {
			out = append(out, defaultBashPattern(s))
		} else {
			out = append(out, s)
		}
	}
	return out
}

func defaultBashPattern(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return cmd
	}
	return fields[0] + " *"
}

// patternsFromResponse 取用户最终用作白名单的 pattern 列表。
//   - 优先 EditedItems（用户在 dialog 里手工编辑）；多行按 \n 拆分。
//   - 否则回退到 defaultPatterns。
func patternsFromResponse(tool string, subjects []string, edited []ApprovalItem) []string {
	var out []string
	for _, item := range edited {
		for line := range strings.SplitSeq(item.Command, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, line)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	return defaultPatterns(tool, subjects)
}
