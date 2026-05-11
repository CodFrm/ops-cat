package aiagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/ai"
)

// newConfirmID 生成审批 confirmID（"ai-<hex>"）。RequestSingle 走 emitter →
// resolver 通道唯一标识当次请求。
func newConfirmID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "ai-" + hex.EncodeToString(buf[:])
}

// LocalGrantStore 是 cago built-in 工具（bash / write / edit）的会话级
// "始终允许" 开关。生产实现在 local_grants.go 走 local_tool_grant_repo 落库。
type LocalGrantStore interface {
	Has(ctx context.Context, sessionID, toolName string) bool
	Save(ctx context.Context, sessionID, toolName string)
}

// PendingResolver 给一个 confirmID 拿一次性响应 channel + 释放函数。app 侧用
// sync.Map 实现：把 confirmID 注册到 pendingAIApprovals，等前端 RespondAIApproval
// 把响应推进来。
type PendingResolver func(confirmID string) (chan ai.ApprovalResponse, func())

// ApprovalGateway 是每个 ConvHandle 持有的审批通道。结构非常薄：发 wails 事件、
// 等响应、(可选) 维护 cago built-in 工具的 grant 缓存。
//
// 设计：policy hook 在 NeedConfirm 分支直接调 RequestSingle 完成 round trip，
// hook 自己返回 DecisionPass / DecisionDeny。不再通过 cago approve.Approver
// 二级 hook 链路（v1 残留链路已经在 task #13 里删除）。
type ApprovalGateway struct {
	convID   int64
	emit     EventEmitter
	grants   LocalGrantStore
	resolver PendingResolver
	activate func() // 可选：弹卡时把窗口拉前台
}

func NewApprovalGateway(convID int64, em EventEmitter, grants LocalGrantStore, resolver PendingResolver) *ApprovalGateway {
	return &ApprovalGateway{
		convID:   convID,
		emit:     em,
		grants:   grants,
		resolver: resolver,
	}
}

func (g *ApprovalGateway) SetActivateFunc(fn func()) { g.activate = fn }

// RequestSingle 发起一次单项审批，阻塞等用户决策或 ctx 取消。
//
// 对本地 cago 工具（kind 以 "local_" 开头）做两个特殊处理：
//   - 进入前先查 LocalGrantStore：若 (除 local_bash 外) 该 toolName 已有会话级
//     放行记录，直接返回合成 allow，跳过 UI 弹卡。
//   - 用户选 "allowAll" 时，把该 toolName 落 grants（同样豁免 bash —— bash 不
//     允许永久放行，每次都必须确认，避免一次 "remember" 把 shell 注入面打开）。
//
// 返回 ai.ApprovalResponse.Decision ∈ {"allow", "allowAll", "deny"}；ctx 取消
// 视作 "deny"。
func (g *ApprovalGateway) RequestSingle(ctx context.Context, kind string, items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse {
	sessionID := fmt.Sprintf("conv_%d", g.convID)
	localTool := localToolName(kind) // 非 local_* 时为空串

	// 本地工具命中已记忆的会话级放行 → 直接合成 allow。
	if localTool != "" && kind != "local_bash" && g.grants != nil {
		if g.grants.Has(ctx, sessionID, localTool) {
			return ai.ApprovalResponse{Decision: "allow"}
		}
	}

	confirmID := newConfirmID()
	if g.activate != nil {
		g.activate()
	}
	g.emit.Emit(g.convID, ai.StreamEvent{
		Type:      "approval_request",
		Kind:      kind,
		ConfirmID: confirmID,
		Items:     items,
		AgentRole: agentRole,
	})
	ch, release := g.resolver(confirmID)
	defer release()

	select {
	case resp := <-ch:
		g.emitResult(confirmID, resp.Decision)
		// allowAll 在 write/edit 上落 grants；bash 永远不落 —— allowAll 退化为单次 allow。
		if resp.Decision == "allowAll" && localTool != "" && kind != "local_bash" && g.grants != nil {
			g.grants.Save(ctx, sessionID, localTool)
		}
		return resp
	case <-ctx.Done():
		g.emitResult(confirmID, "deny")
		return ai.ApprovalResponse{Decision: "deny"}
	}
}

func (g *ApprovalGateway) emitResult(confirmID, decision string) {
	g.emit.Emit(g.convID, ai.StreamEvent{
		Type:      "approval_result",
		ConfirmID: confirmID,
		Content:   decision,
	})
}

// localToolName 把 "local_xxx" 形态的 kind 转成 LocalGrantStore 的 toolName
// （strip "local_" 前缀）。非 local_* 返回 ""，调用方据此判断是否走本地工具分支。
func localToolName(kind string) string {
	if !strings.HasPrefix(kind, "local_") {
		return ""
	}
	return kind[len("local_"):]
}
