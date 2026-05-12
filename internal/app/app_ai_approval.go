package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/ai"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// makeCommandConfirmFunc 创建统一审批回调，向 AI 聊天流发送 approval_request 事件并阻塞等待
func (a *App) makeCommandConfirmFunc() ai.CommandConfirmFunc {
	return func(ctx context.Context, kind string, items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse {
		convID := ai.GetConversationID(ctx)
		if convID == 0 {
			convID = a.currentConversationID // fallback
		}
		confirmID := fmt.Sprintf("ai_%d_%d", convID, time.Now().UnixNano())
		eventName := fmt.Sprintf("ai:event:%d", convID)

		// 向 AI 聊天流发送 approval_request 事件
		wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
			Type:      "approval_request",
			Kind:      kind,
			Items:     items,
			ConfirmID: confirmID,
			AgentRole: agentRole,
		})

		// 阻塞等待前端响应
		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingAIApprovals.Store(confirmID, ch)
		defer a.pendingAIApprovals.Delete(confirmID)

		select {
		case resp := <-ch:
			// 发送确认结果事件更新 UI 状态
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   resp.Decision,
			})
			return resp
		case <-ctx.Done():
			// 审批等待跟随当前对话上下文取消，避免 run_command 已停止但审批协程仍悬挂。
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   "deny",
			})
			return ai.ApprovalResponse{Decision: "deny"}
		case <-a.ctx.Done():
			return ai.ApprovalResponse{Decision: "deny"}
		case <-a.shutdownCh:
			return ai.ApprovalResponse{Decision: "deny"}
		}
	}
}

// makeGrantRequestFunc 创建 Grant 审批回调，使用 inline approval
func (a *App) makeGrantRequestFunc() ai.GrantRequestFunc {
	return func(ctx context.Context, items []ai.ApprovalItem, reason string) (bool, []string) {
		convID := ai.GetConversationID(ctx)
		if convID == 0 {
			convID = a.currentConversationID // fallback
		}
		confirmID := fmt.Sprintf("grant_%d_%d", convID, time.Now().UnixNano())
		eventName := fmt.Sprintf("ai:event:%d", convID)

		wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
			Type:        "approval_request",
			Kind:        "grant",
			Items:       items,
			ConfirmID:   confirmID,
			Description: reason,
			SessionID:   fmt.Sprintf("conv_%d", convID),
		})

		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingAIApprovals.Store(confirmID, ch)
		defer a.pendingAIApprovals.Delete(confirmID)

		select {
		case resp := <-ch:
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   resp.Decision,
			})
			if resp.Decision == "deny" {
				return false, nil
			}
			var finalPatterns []string
			if len(resp.EditedItems) > 0 {
				for _, item := range resp.EditedItems {
					cmd := strings.TrimSpace(item.Command)
					if cmd != "" {
						finalPatterns = append(finalPatterns, cmd)
						ai.SaveGrantPattern(a.langCtx(), fmt.Sprintf("conv_%d", convID), item.AssetID, item.AssetName, cmd)
					}
				}
			} else {
				for _, item := range items {
					finalPatterns = append(finalPatterns, item.Command)
					ai.SaveGrantPattern(a.langCtx(), fmt.Sprintf("conv_%d", convID), item.AssetID, item.AssetName, item.Command)
				}
			}
			return true, finalPatterns
		case <-ctx.Done():
			// grant 申请与单次审批一致，优先受会话级 ctx 控制，避免页面已取消但授权流程未退出。
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   "deny",
			})
			return false, nil
		case <-a.ctx.Done():
			return false, nil
		case <-a.shutdownCh:
			return false, nil
		}
	}
}

// makeLocalToolConfirmFunc 创建 coding agent 本地工具（bash/write/edit）审批回调。
//
// 复用 pendingAIApprovals channel 阻塞机制；事件 Kind="local_tool"，
// 与现有 single/batch/grant 不共用 dialog 组件——前端按 Kind 路由。
func (a *App) makeLocalToolConfirmFunc() ai.LocalToolConfirmFunc {
	return func(ctx context.Context, req ai.LocalToolApprovalRequest) ai.ApprovalResponse {
		convID := ai.GetConversationID(ctx)
		if convID == 0 {
			convID = a.currentConversationID
		}
		confirmID := fmt.Sprintf("local_tool_%d_%d", convID, time.Now().UnixNano())
		eventName := fmt.Sprintf("ai:event:%d", convID)

		a.activateWindow()
		wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
			Type:      "approval_request",
			Kind:      "local_tool",
			ConfirmID: confirmID,
			ToolName:  req.ToolName,
			Items: []ai.ApprovalItem{{
				Type:    req.ToolName,
				Command: req.Command,
				Detail:  req.Detail,
			}},
			Patterns: req.DefaultPatterns,
		})

		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingAIApprovals.Store(confirmID, ch)
		defer a.pendingAIApprovals.Delete(confirmID)

		select {
		case resp := <-ch:
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   resp.Decision,
			})
			return resp
		case <-ctx.Done():
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:      "approval_result",
				ConfirmID: confirmID,
				Content:   "deny",
			})
			return ai.ApprovalResponse{Decision: "deny"}
		case <-a.ctx.Done():
			return ai.ApprovalResponse{Decision: "deny"}
		case <-a.shutdownCh:
			return ai.ApprovalResponse{Decision: "deny"}
		}
	}
}

// RespondAIApproval 前端响应 AI 审批请求（统一入口）
func (a *App) RespondAIApproval(confirmID string, resp ai.ApprovalResponse) {
	if v, ok := a.pendingAIApprovals.Load(confirmID); ok {
		ch := v.(chan ai.ApprovalResponse)
		select {
		case ch <- resp:
		default:
		}
	}
}
