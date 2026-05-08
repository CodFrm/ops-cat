package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// makeSubagentObserver returns an agent.Observer that forwards child-stream
// events to the parent stream's emitter, tagging each event with the
// sub-agent's role/task. role/task are captured at construction.
func makeSubagentObserver(em EventEmitter, convID int64, role, task string) agent.Observer {
	// 子 agent 不会被前端 Steer 进来 follow-up（cago dispatch_subagent 给子 agent 派
	// 单一 prompt + tool 循环，没有用户中途插话路径），bridge 的 popDisplay 走默认
	// 空实现即可。万一未来有 follow-up，会 emit 不带 content 的 batch，前端会忽略。
	br := newBridge(taggingEmitter{inner: em, role: role, task: task}, nil)
	return func(_ context.Context, ev agent.Event) {
		br.translate(convID, ev)
	}
}

// makeSubagentObserverEmitStart synchronously emits an agent_start event.
// Returns the observer to install on the child agent (which captures further
// events). Caller is responsible for emitting agent_end via the dispatcher
// PostToolUse on the parent.
func makeSubagentObserverEmitStart(em EventEmitter, convID int64, role, task string) agent.Observer {
	em.Emit(convID, ai.StreamEvent{
		Type:      "agent_start",
		AgentRole: role,
		AgentTask: task,
	})
	return makeSubagentObserver(em, convID, role, task)
}

// taggingEmitter wraps an EventEmitter and stamps AgentRole/AgentTask onto
// each forwarded event before delegating.
type taggingEmitter struct {
	inner EventEmitter
	role  string
	task  string
}

func (e taggingEmitter) Emit(convID int64, ev ai.StreamEvent) {
	if ev.AgentRole == "" {
		ev.AgentRole = e.role
	}
	if ev.AgentTask == "" {
		ev.AgentTask = e.task
	}
	e.inner.Emit(convID, ev)
}

// MakeAgentEndHook returns a PostToolUse hook that converts the
// dispatch_subagent tool's response into an agent_end Wails event with the
// summary truncated to 2048 chars (matching the legacy spawn_agent behavior).
// Register only with matcher "dispatch_subagent".
func MakeAgentEndHook(em EventEmitter) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		if in.ToolName != "dispatch_subagent" {
			return nil, nil
		}
		var summary string
		_ = json.Unmarshal(in.ToolResponse, &summary)
		if summary == "" {
			summary = string(in.ToolResponse)
		}
		if len(summary) > 2048 {
			summary = summary[:2048] + "..."
		}
		em.Emit(getConvID(ctx), ai.StreamEvent{
			Type:    "agent_end",
			Content: summary,
		})
		return nil, nil
	}
}
