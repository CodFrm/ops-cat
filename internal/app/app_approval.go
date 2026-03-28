package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/grant_entity"
	"github.com/opskat/opskat/internal/repository/grant_repo"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// startApprovalServer 启动 opsctl 审批 Unix socket 服务
func (a *App) startApprovalServer(authToken string) {
	handler := func(req approval.ApprovalRequest) approval.ApprovalResponse {
		// 数据变更通知：opsctl 通知前端刷新
		if req.Type == "notify" {
			wailsRuntime.EventsEmit(a.ctx, "data:changed", map[string]any{
				"resource": req.Detail,
			})
			return approval.ApprovalResponse{Approved: true}
		}

		// 授权审批
		if req.Type == "grant" {
			return a.handleGrantApproval(req)
		}

		// 批量执行审批
		if req.Type == "batch" {
			return a.handleBatchApproval(req)
		}

		// 扩展工具执行
		if req.Type == "ext_tool" {
			return a.handleExtToolApproval(req)
		}

		// 单条审批
		confirmID := fmt.Sprintf("opsctl_%d", time.Now().UnixNano())

		// 激活应用窗口
		a.activateWindow()

		wailsRuntime.EventsEmit(a.ctx, "opsctl:approval", map[string]any{
			"confirm_id": confirmID,
			"type":       req.Type,
			"asset_id":   req.AssetID,
			"asset_name": req.AssetName,
			"command":    req.Command,
			"detail":     req.Detail,
			"session_id": req.SessionID,
		})

		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingOpsctlApprovals.Store(confirmID, ch)
		defer a.pendingOpsctlApprovals.Delete(confirmID)

		select {
		case resp := <-ch:
			if resp.Decision == "deny" {
				return approval.ApprovalResponse{Approved: false, Reason: "user denied"}
			}
			// "allowAll" → 保存用户编辑后的 grant 模式（支持 * 通配符）
			if resp.Decision == "allowAll" && req.SessionID != "" {
				pattern := req.Command
				if len(resp.EditedItems) > 0 {
					pattern = resp.EditedItems[0].Command
				}
				ai.SaveGrantPattern(a.langCtx(), req.SessionID, req.AssetID, req.AssetName, pattern)
			}
			return approval.ApprovalResponse{Approved: true}
		case <-a.ctx.Done():
			return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
		case <-a.shutdownCh:
			return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
		}
	}

	srv := approval.NewServer(handler, authToken)
	sockPath := approval.SocketPath(bootstrap.AppDataDir())
	if err := srv.Start(sockPath); err != nil {
		log.Printf("Approval server failed to start: %v", err)
		return
	}
	a.approvalServer = srv
}

// startSSHPoolServer 启动 SSH 连接池 proxy 服务
func (a *App) startSSHPoolServer(authToken string) {
	dialer := &appPoolDialer{sshManager: a.sshManager}
	a.sshPool = sshpool.NewPool(dialer, 5*time.Minute)
	a.sshProxyServer = sshpool.NewServer(a.sshPool, authToken)
	sockPath := sshpool.SocketPath(bootstrap.AppDataDir())
	if err := a.sshProxyServer.Start(sockPath); err != nil {
		log.Printf("SSH pool server failed to start: %v", err)
		return
	}
}

// handleBatchApproval 处理批量执行审批（exec/sql/redis 混合）
func (a *App) handleBatchApproval(req approval.ApprovalRequest) approval.ApprovalResponse {
	confirmID := fmt.Sprintf("batch_%d", time.Now().UnixNano())

	// 构建前端事件数据
	items := make([]map[string]any, 0, len(req.BatchItems))
	for _, item := range req.BatchItems {
		items = append(items, map[string]any{
			"type":       item.Type,
			"asset_id":   item.AssetID,
			"asset_name": item.AssetName,
			"command":    item.Command,
		})
	}

	// 激活应用窗口
	a.activateWindow()

	wailsRuntime.EventsEmit(a.ctx, "opsctl:batch-approval", map[string]any{
		"confirm_id": confirmID,
		"session_id": req.SessionID,
		"items":      items,
	})

	ch := make(chan ai.ApprovalResponse, 1)
	a.pendingOpsctlApprovals.Store(confirmID, ch)
	defer a.pendingOpsctlApprovals.Delete(confirmID)

	select {
	case resp := <-ch:
		if resp.Decision == "deny" {
			return approval.ApprovalResponse{Approved: false, Reason: "user denied"}
		}
		return approval.ApprovalResponse{Approved: true}
	case <-a.ctx.Done():
		return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
	case <-a.shutdownCh:
		return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
	}
}

// handleGrantApproval 处理批量计划审批
func (a *App) handleGrantApproval(req approval.ApprovalRequest) approval.ApprovalResponse {
	ctx := a.langCtx()
	sessionID := req.SessionID

	// 写入 DB
	session := &grant_entity.GrantSession{
		ID:          sessionID,
		Description: req.Description,
		Status:      grant_entity.GrantStatusPending,
		Createtime:  time.Now().Unix(),
	}
	if err := grant_repo.Grant().CreateSession(ctx, session); err != nil {
		// Session may already exist (e.g., multiple request_permission calls in same conversation)
		if _, getErr := grant_repo.Grant().GetSession(ctx, sessionID); getErr != nil {
			return approval.ApprovalResponse{Approved: false, Reason: "failed to create grant session"}
		}
	}

	var items []*grant_entity.GrantItem
	for i, pi := range req.GrantItems {
		items = append(items, &grant_entity.GrantItem{
			GrantSessionID: sessionID,
			ItemIndex:      i,
			ToolName:       pi.Type,
			AssetID:        pi.AssetID,
			AssetName:      pi.AssetName,
			GroupID:        pi.GroupID,
			GroupName:      pi.GroupName,
			Command:        pi.Command,
			Detail:         pi.Detail,
		})
	}
	if err := grant_repo.Grant().CreateItems(ctx, items); err != nil {
		return approval.ApprovalResponse{Approved: false, Reason: "failed to create grant items"}
	}

	// 构建前端事件数据
	eventItems := make([]map[string]any, 0, len(req.GrantItems))
	for _, pi := range req.GrantItems {
		eventItems = append(eventItems, map[string]any{
			"type":       pi.Type,
			"asset_id":   pi.AssetID,
			"asset_name": pi.AssetName,
			"group_id":   pi.GroupID,
			"group_name": pi.GroupName,
			"command":    pi.Command,
			"detail":     pi.Detail,
		})
	}

	// 激活应用窗口
	a.activateWindow()

	wailsRuntime.EventsEmit(a.ctx, "opsctl:grant-approval", map[string]any{
		"session_id":  sessionID,
		"description": req.Description,
		"items":       eventItems,
	})

	// 等待前端响应
	ch := make(chan ai.ApprovalResponse, 1)
	a.pendingOpsctlApprovals.Store(sessionID, ch)
	defer a.pendingOpsctlApprovals.Delete(sessionID)

	select {
	case resp := <-ch:
		if resp.Decision == "deny" {
			if err := grant_repo.Grant().UpdateSessionStatus(ctx, sessionID, grant_entity.GrantStatusRejected); err != nil {
				logger.Default().Error("update grant session status to rejected", zap.Error(err))
			}
			return approval.ApprovalResponse{Approved: false, Reason: "user denied", SessionID: sessionID}
		}
		if err := grant_repo.Grant().UpdateSessionStatus(ctx, sessionID, grant_entity.GrantStatusApproved); err != nil {
			logger.Default().Error("update grant session status to approved", zap.Error(err))
		}
		// 处理用户编辑的 items
		if len(resp.EditedItems) > 0 {
			var items []*grant_entity.GrantItem
			for i, edit := range resp.EditedItems {
				lines := strings.Split(edit.Command, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					items = append(items, &grant_entity.GrantItem{
						GrantSessionID: sessionID,
						ItemIndex:      i,
						ToolName:       "exec",
						AssetID:        edit.AssetID,
						AssetName:      edit.AssetName,
						GroupID:        edit.GroupID,
						GroupName:      edit.GroupName,
						Command:        line,
					})
				}
			}
			if len(items) > 0 {
				if err := grant_repo.Grant().UpdateItems(ctx, sessionID, items); err != nil {
					logger.Default().Error("update grant items", zap.Error(err))
				}
			}
		}
		finalResp := approval.ApprovalResponse{Approved: true, SessionID: sessionID}
		if finalItems, err := grant_repo.Grant().ListItems(ctx, sessionID); err == nil {
			for _, item := range finalItems {
				finalResp.EditedItems = append(finalResp.EditedItems, approval.GrantItem{
					Type:      item.ToolName,
					AssetID:   item.AssetID,
					AssetName: item.AssetName,
					GroupID:   item.GroupID,
					GroupName: item.GroupName,
					Command:   item.Command,
					Detail:    item.Detail,
				})
			}
		}
		return finalResp
	case <-a.ctx.Done():
		if err := grant_repo.Grant().UpdateSessionStatus(ctx, sessionID, grant_entity.GrantStatusRejected); err != nil {
			logger.Default().Error("update grant session status to rejected on shutdown", zap.Error(err))
		}
		return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
	case <-a.shutdownCh:
		if err := grant_repo.Grant().UpdateSessionStatus(ctx, sessionID, grant_entity.GrantStatusRejected); err != nil {
			logger.Default().Error("update grant session status to rejected on shutdown", zap.Error(err))
		}
		return approval.ApprovalResponse{Approved: false, Reason: "app shutting down"}
	}
}

// handleExtToolApproval 处理扩展工具执行请求
func (a *App) handleExtToolApproval(req approval.ApprovalRequest) approval.ApprovalResponse {
	if a.extHost == nil {
		return approval.ApprovalResponse{Approved: false, Reason: "extension system not initialized"}
	}
	plugin := a.extHost.GetPlugin(req.Extension)
	if plugin == nil {
		return approval.ApprovalResponse{Approved: false, Reason: fmt.Sprintf("extension %q not loaded", req.Extension)}
	}
	result, err := plugin.CallTool(context.Background(), req.Tool, json.RawMessage(req.ArgsJSON))
	if err != nil {
		return approval.ApprovalResponse{Approved: false, Reason: err.Error()}
	}
	return approval.ApprovalResponse{Approved: true, Result: string(result)}
}

// RespondOpsctlApproval 前端响应 opsctl 审批请求（统一入口）
func (a *App) RespondOpsctlApproval(confirmID string, resp ai.ApprovalResponse) {
	if v, ok := a.pendingOpsctlApprovals.Load(confirmID); ok {
		ch := v.(chan ai.ApprovalResponse)
		select {
		case ch <- resp:
		default:
		}
	}
}

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
		case <-a.ctx.Done():
			return false, nil
		case <-a.shutdownCh:
			return false, nil
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
