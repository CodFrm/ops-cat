package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// spawnAgentContextKey 用于在 context 中传递 Sub Agent 依赖
type spawnAgentContextKey struct{}

// SpawnAgentDeps spawn_agent 工具需要的依赖
type SpawnAgentDeps struct {
	Provider    Provider
	Checker     *CommandPolicyChecker
	OnEvent     func(StreamEvent)
	NewExecutor func() ToolExecutor // 工厂函数，每个 Sub Agent 创建独立 executor
}

// WithSpawnAgentDeps 将 Sub Agent 依赖注入 context
func WithSpawnAgentDeps(ctx context.Context, deps *SpawnAgentDeps) context.Context {
	return context.WithValue(ctx, spawnAgentContextKey{}, deps)
}

func getSpawnAgentDeps(ctx context.Context) *SpawnAgentDeps {
	if v := ctx.Value(spawnAgentContextKey{}); v != nil {
		return v.(*SpawnAgentDeps)
	}
	return nil
}

func handleSpawnAgent(ctx context.Context, args map[string]any) (string, error) {
	// 检查是否已是 Sub Agent（防止嵌套）
	if isSubAgent(ctx) {
		return "Error: Sub-agents cannot spawn other sub-agents. Please complete the task directly.", nil
	}

	role, _ := args["role"].(string)
	task, _ := args["task"].(string)
	if role == "" || task == "" {
		return "", fmt.Errorf("role and task are required")
	}

	deps := getSpawnAgentDeps(ctx)
	if deps == nil {
		return "", fmt.Errorf("spawn_agent not available in this context")
	}

	// 解析可选参数
	var toolNames []string
	if toolsRaw, ok := args["tools"]; ok {
		if toolsJSON, err := json.Marshal(toolsRaw); err == nil {
			if err := json.Unmarshal(toolsJSON, &toolNames); err != nil {
				logger.Ctx(ctx).Warn("parse spawn_agent tools param", zap.Error(err))
			}
		}
	}

	// 构建 Sub Agent 工具列表
	var subTools []ToolDef
	if len(toolNames) > 0 {
		allDefs := AllToolDefs()
		nameSet := make(map[string]bool)
		for _, n := range toolNames {
			nameSet[n] = true
		}
		for _, def := range allDefs {
			if nameSet[def.Name] {
				subTools = append(subTools, def)
			}
		}
	}

	// 构建 Sub Agent system prompt
	promptParts := make([]string, 0, 4)
	promptParts = append(promptParts, fmt.Sprintf("You are a sub-agent with the role: %s", role))
	promptParts = append(promptParts, fmt.Sprintf("Your task: %s", task))
	promptParts = append(promptParts, "Complete the task thoroughly and report your findings. Be concise in your final summary.")
	promptParts = append(promptParts, "If you need command execution permissions, use request_permission to request them before running commands.")

	config := NewSubAgentConfig()
	config.SystemPrompt = strings.Join(promptParts, "\n\n")
	if subTools != nil {
		config.Tools = subTools
	}

	// 通知前端 Sub Agent 开始
	deps.OnEvent(StreamEvent{
		Type:      "agent_start",
		AgentRole: role,
		AgentTask: task,
	})

	// 创建 Sub Agent（使用独立 executor 工厂）
	subAgent := NewAgent(deps.Provider, func() ToolExecutor {
		return deps.NewExecutor()
	}, deps.Checker, config)

	// 构建初始消息
	subMessages := []Message{
		{Role: RoleSystem, Content: config.SystemPrompt},
		{Role: RoleUser, Content: task},
	}

	// 运行 Sub Agent，转发事件到前端
	var resultContent string
	subCtx := WithAgentRole(withSubAgentFlag(ctx), role)
	err := subAgent.Chat(subCtx, subMessages, func(event StreamEvent) {
		switch event.Type {
		case "content":
			resultContent += event.Content
			deps.OnEvent(event)
		case "done":
			// Sub Agent 结束，不转发 done（由主 Agent 控制）
		default:
			deps.OnEvent(event)
		}
	}, nil)

	// 通知前端 Sub Agent 结束
	summary := resultContent
	if len(summary) > 2048 {
		summary = summary[:2048] + "..."
	}
	deps.OnEvent(StreamEvent{
		Type:    "agent_end",
		Content: summary,
	})

	if err != nil {
		logger.Ctx(ctx).Warn("sub-agent execution failed", zap.Error(err))
		return fmt.Sprintf("Sub-agent failed: %s\n\nPartial result:\n%s", err.Error(), summary), nil
	}

	return summary, nil
}

// Sub Agent 标记（用于防止嵌套）
type subAgentFlagKey struct{}

func withSubAgentFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, subAgentFlagKey{}, true)
}

func isSubAgent(ctx context.Context) bool {
	if v := ctx.Value(subAgentFlagKey{}); v != nil {
		return v.(bool)
	}
	return false
}
