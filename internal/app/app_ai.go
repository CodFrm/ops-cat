package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/anthropics"
	openaiCago "github.com/cago-frame/agents/provider/openai"
	openaiSDK "github.com/sashabaranov/go-openai"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/aiagent"
	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/ai_provider_svc"
	"github.com/opskat/opskat/internal/service/conversation_svc"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// normalizeConversationTitle 统一会话标题规则，避免首次命名和后续编辑首条消息时产生不同标题。
func normalizeConversationTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "新对话"
	}
	titleRunes := []rune(title)
	if len(titleRunes) > 50 {
		title = string(titleRunes[:50])
	}
	return title
}

// activateProvider 根据激活的 AI Provider 配置构建 cago provider 并重建 Manager。
// 切换 provider 时若不重置 Manager，旧 *aiagent.ConvHandle 会继续持有旧 provider
// 引用，必须重启软件才能生效。
func (a *App) activateProvider(p *ai_provider_entity.AIProvider) error {
	prov, err := a.buildCagoProvider(p)
	if err != nil {
		return err
	}
	a.aiProvider = prov
	a.aiModel = p.Model
	a.resetManager(prov, p.Model)
	return nil
}

// resetManager closes any existing Manager and constructs a new one bound
// to the given cago provider. Called when the active AI Provider changes.
//
// 工具/策略/审计/提及/打 tab 都用真实适配器：
//   - Tools: aiagent.OpsTools(deps) 把 ai.AllToolDefs() 全包成 agent.Tool
//   - System: aiagent.StaticSystemPrompt(a.lang)
//   - PolicyChecker: opsPolicyChecker —— 走 ai.CheckPermission，NeedConfirm 时
//     通过 ApprovalGateway 弹卡（policy hook 内部完成审批 round trip）
//   - AuditWriter: opsAuditWriter —— 写 audit_repo
//   - Mention: opsMentionResolver —— 扫描 @name → asset_repo 查询 → 渲染 context
//   - TabOpener: opsTabOpener —— 通过 wailsRuntime.EventsEmit 让前端开 tab
func (a *App) resetManager(prov provider.Provider, model string) {
	if a.mgr != nil {
		_ = a.mgr.Close()
	}
	emitter := aiagent.EmitterFunc(func(convID int64, event ai.StreamEvent) {
		eventName := fmt.Sprintf("ai:event:%d", convID)
		wailsRuntime.EventsEmit(a.ctx, eventName, event)
	})
	resolver := aiagent.PendingResolver(func(confirmID string) (chan ai.ApprovalResponse, func()) {
		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingAIApprovals.Store(confirmID, ch)
		return ch, func() { a.pendingAIApprovals.Delete(confirmID) }
	})
	// PolicyChecker 走 nil confirmFunc —— v2 审批分两条腿：
	//   - 单命令策略 NeedConfirm：cago policy hook (aiagent.newPolicyHook) →
	//     ApprovalGateway.RequestSingle 弹卡。
	//   - request_permission (grant 申请)：tool handler → ai.GetGrantApprover →
	//     ApprovalGateway.RequestGrant 弹卡 + 落 grant_repo。
	// CommandPolicyChecker 仍由 ai 层一些只读路径 (policy 求值 / matchGrantPatterns)
	// 持有；confirmFunc 为 nil 时 handleConfirm 早退 —— PreToolUse 已先一步处理。
	policyChecker := ai.NewCommandPolicyChecker(nil)
	deps := aiagent.NewDeps(a.sshPool, policyChecker)
	a.mgr = aiagent.NewManager(aiagent.ManagerOptions{
		Provider:      prov,
		Model:         model,
		System:        aiagent.StaticSystemPrompt(a.lang),
		Tools:         aiagent.OpsTools(deps),
		Deps:          deps,
		MaxRounds:     50,
		Emitter:       emitter,
		Resolver:      resolver,
		LocalGrants:   aiagent.NewRepoLocalGrantStore(),
		AuditWriter:   newOpsAuditWriter(),
		PolicyChecker: newOpsPolicyChecker(),
		Mention:       newOpsMentionResolver(),
		TabOpener:     newOpsTabOpener(a),
	})
}

// InitAIProvider 启动时加载激活的 Provider
func (a *App) InitAIProvider() {
	p, err := ai_provider_svc.AIProvider().GetActive(a.langCtx())
	if err != nil {
		return // 无激活 provider，跳过
	}
	if err := a.activateProvider(p); err != nil {
		logger.Default().Warn("activate AI provider on startup", zap.Error(err))
	}
}

// --- AI 操作 ---

// ConversationDisplayMessage 返回给前端的会话消息（用于恢复显示）。
// SortOrder 是后端 conversation_messages 表的 sort_order，前端编辑/重生成时
// 必须传它给 EditAIMessage/RegenerateAIMessage —— 因为 buildDisplayMessages 会
// 合并多轮 assistant + 吸收 tool-result-only 行，前端 messages 数组的 index 和
// 后端 sort_order 不再 1:1。合并后的 assistant 取首条 assistant 的 sort_order。
type ConversationDisplayMessage struct {
	Role string `json:"role"`
	// PartialReason 是 cago agent.PartialState 的字符串值（"errored"/"canceled"/
	// "tokens_limit"/"timeout"）。空串表示消息正常完成。前端按此渲染气泡尾部
	// 的中断/错误提示，与运行时收到 "error"/"stopped" 事件的渲染路径一致。
	PartialReason string `json:"partialReason,omitempty"`
	// PartialDetail 是 PartialReason 对应的人类可读详情，例如 errored 时的
	// err.Error()、canceled 时的取消原因。空串时前端按 PartialReason 显示通用文案。
	PartialDetail string                             `json:"partialDetail,omitempty"`
	Content       string                             `json:"content"`
	Blocks        []conversation_entity.ContentBlock `json:"blocks"`
	Mentions      []conversation_entity.MentionRef   `json:"mentions"`
	TokenUsage    *conversation_entity.TokenUsage    `json:"tokenUsage,omitempty"`
	SortOrder     int                                `json:"sortOrder"`
}

// CreateConversation 创建新会话
func (a *App) CreateConversation() (*conversation_entity.Conversation, error) {
	if a.aiProvider == nil {
		return nil, fmt.Errorf("请先配置 AI Provider")
	}
	ctx := a.langCtx()

	// 获取激活 Provider ID
	activeProvider, _ := ai_provider_svc.AIProvider().GetActive(ctx)
	var providerID int64
	if activeProvider != nil {
		providerID = activeProvider.ID
	}

	workDir, err := defaultConversationWorkDir()
	if err != nil {
		return nil, err
	}
	conv := &conversation_entity.Conversation{
		Title:      "新对话",
		ProviderID: providerID,
		WorkDir:    workDir, // user-chosen via UpdateConversationCwd; default = ~/.opskat
	}
	if err := conversation_svc.Conversation().Create(ctx, conv); err != nil {
		return nil, err
	}
	a.currentConversationID = conv.ID
	return conv, nil
}

// ListConversations 获取会话列表
func (a *App) ListConversations() ([]*conversation_entity.Conversation, error) {
	return conversation_svc.Conversation().List(a.langCtx())
}

// UpdateConversationTitle 更新会话标题。
func (a *App) UpdateConversationTitle(id int64, title string) error {
	ctx := a.langCtx()
	err := conversation_svc.Conversation().UpdateTitle(ctx, id, normalizeConversationTitle(title))
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("会话不存在: %w", err)
	}
	return fmt.Errorf("更新会话标题失败: %w", err)
}

// UpdateConversationCwd 更新会话工作目录（cwd 由前端选择器设置）
func (a *App) UpdateConversationCwd(id int64, cwd string) error {
	ctx := a.langCtx()
	if err := conversation_svc.Conversation().UpdateWorkDir(ctx, id, cwd); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("会话不存在: %w", err)
		}
		return fmt.Errorf("更新会话工作目录失败: %w", err)
	}
	return nil
}

// PickConversationCwd opens a native folder dialog seeded with the conversation's
// current cwd (or ~/.opskat if unset), persists the selection, and returns the
// chosen path. Returns "" if the user canceled.
func (a *App) PickConversationCwd(id int64) (string, error) {
	conv, err := conversation_svc.Conversation().Get(a.langCtx(), id)
	if err != nil {
		return "", fmt.Errorf("会话不存在: %w", err)
	}
	defaultDir := conv.WorkDir
	if defaultDir == "" {
		var derr error
		defaultDir, derr = defaultConversationWorkDir()
		if derr != nil {
			return "", derr
		}
	}
	chosen, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                "选择工作目录",
		DefaultDirectory:     defaultDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if chosen == "" {
		return "", nil // user canceled
	}
	if err := a.UpdateConversationCwd(id, chosen); err != nil {
		return "", err
	}
	return chosen, nil
}

// SwitchConversation 切换到指定会话，返回显示消息
func (a *App) SwitchConversation(id int64) ([]ConversationDisplayMessage, error) {
	ctx := a.langCtx()
	conv, err := conversation_svc.Conversation().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}

	a.switchToConversation(conv)
	return a.loadConversationDisplayMessages(ctx, id)
}

// LoadConversationMessages 只读加载会话消息，不修改 currentConversationID。
// 用于侧边栏等场景只需读取历史而不切换当前会话。
func (a *App) LoadConversationMessages(id int64) ([]ConversationDisplayMessage, error) {
	ctx := a.langCtx()
	if _, err := conversation_svc.Conversation().Get(ctx, id); err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}
	return a.loadConversationDisplayMessages(ctx, id)
}

// buildDisplayMessages 把 cago v2 schema 的 conversation_messages 行聚合成前端
// 展示用的 ConversationDisplayMessage 列表。
//
// 还原原则（与前端流式状态机对齐）：
//   - assistant 行 → 一条 assistant 消息，blocks 包含 text/thinking/tool（tool
//     状态默认 completed、content 为空、稍后被 tool_result 行回填）。
//   - user 行 → 若 blocks 全是 tool_result，则把 result content 按 toolCallId
//     回填到上一条 assistant 的 tool block，不单独产出消息（前端 tool 块本来
//     就把 call+result 合并展示）。否则当成普通 user 消息。
//   - display_text 块（cago Audience 体系）用作 user 消息的 raw 显示文本（mention
//     扩展后保 raw）；老数据兼容 metadata{key:"display"}。
//   - 紧跟在 tool_result-only user 行之后的 assistant 行，会折叠合并到上一条
//     assistant 上：cago 每次 LLM 调用记一条 assistant，多轮 tool 后的最终
//     格式化回复在 DB 是第 N+2 条；前端流式 reducer 把整轮都贴到同一气泡，
//     历史回放要对齐这个体验，否则刷新后会变成两个 assistant 气泡。
func buildDisplayMessages(msgs []*conversation_entity.Message) []ConversationDisplayMessage {
	out := make([]ConversationDisplayMessage, 0, len(msgs))
	// mergePending：刚处理了一条 tool_result-only user 行——下一条 assistant
	// 应该 fold 进 out 末尾的 assistant，而不是独立成一条新消息。
	mergePending := false
	for _, m := range msgs {
		var raw []map[string]any
		if m.Blocks != "" {
			if err := json.Unmarshal([]byte(m.Blocks), &raw); err != nil {
				raw = nil
			}
		}
		// Legacy ContentBlock 形状兜底（status 字段命中前端 schema 才匹配）。
		if raw == nil || !looksLikeV2Blocks(raw) {
			if blocks, err := m.GetBlocks(); err == nil && len(blocks) > 0 {
				text := ""
				for _, b := range blocks {
					if b.Type == "text" {
						text += b.Content
					}
				}
				if text == "" {
					continue
				}
				mentions, _ := m.GetMentions()
				tokenUsage, _ := m.GetTokenUsage()
				out = append(out, ConversationDisplayMessage{
					Role:          m.Role,
					PartialReason: m.PartialReason,
					PartialDetail: m.PartialDetail,
					Content:       text,
					Blocks:        blocks,
					Mentions:      mentions,
					TokenUsage:    tokenUsage,
					SortOrder:     m.SortOrder,
				})
				mergePending = false
				continue
			}
		}

		// v2 path.
		// tool-role 行（cago v2 provider/types.go:12 RoleTool = "tool"）整条就是
		// tool_result——回填到上条 assistant 的 tool block，并 mergePending 让下
		// 一条 assistant fold 进来。老 v1 数据用 role="user" + onlyToolResults 表达
		// 同样语义，一起兼容。
		if m.Role == "tool" || (m.Role == "user" && onlyToolResults(raw)) {
			mergeToolResultsIntoLastAssistant(out, raw)
			mergePending = true
			continue
		}
		if m.Role == "user" {
			displayText := extractV2UserDisplay(raw)
			if displayText == "" {
				// 空 user 行（连 display 都没有）跳过；不动 mergePending——
				// 它代表的是 "上一条 assistant 已结束 tool result 等待"，被空消息打断不合理。
				continue
			}
			mentions, _ := m.GetMentions()
			// user 消息没有 partial 概念（cago 只在 assistant 上打 PartialReason）；
			// 这里不带 PartialReason/Detail，前端零值降级。
			out = append(out, ConversationDisplayMessage{
				Role:      "user",
				Content:   displayText,
				Blocks:    []conversation_entity.ContentBlock{{Type: "text", Content: displayText}},
				Mentions:  mentions,
				SortOrder: m.SortOrder,
			})
			mergePending = false
			continue
		}

		// assistant / system.
		blocks, content := decodeV2AssistantBlocks(raw)
		if len(blocks) == 0 && content == "" {
			continue
		}
		tokenUsage, _ := m.GetTokenUsage()
		if mergePending && m.Role == "assistant" && len(out) > 0 && out[len(out)-1].Role == "assistant" {
			prev := &out[len(out)-1]
			prev.Blocks = append(prev.Blocks, blocks...)
			if content != "" {
				if prev.Content != "" {
					prev.Content += "\n"
				}
				prev.Content += content
			}
			prev.TokenUsage = sumTokenUsage(prev.TokenUsage, tokenUsage)
			// 多轮 assistant 折叠：partial 状态以**当前这条**为准——前面几条
			// 必然 finalized（否则 cago 不会启动下一轮），最后一条才可能 errored/
			// canceled/timeout。零值表示这一条正常完成，整组也按完成渲染。
			prev.PartialReason = m.PartialReason
			prev.PartialDetail = m.PartialDetail
			mergePending = false
			continue
		}
		out = append(out, ConversationDisplayMessage{
			Role:          m.Role,
			PartialReason: m.PartialReason,
			PartialDetail: m.PartialDetail,
			Content:       content,
			Blocks:        blocks,
			TokenUsage:    tokenUsage,
			SortOrder:     m.SortOrder,
		})
		mergePending = false
	}
	return out
}

// sumTokenUsage 把两个 round 的 token 用量相加。任一为 nil 时返回另一个（不深拷贝，
// 调用者持有所有权）；都为 nil 返回 nil。对齐 aiStore live "usage" handler 的累加语义。
func sumTokenUsage(a, b *conversation_entity.TokenUsage) *conversation_entity.TokenUsage {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &conversation_entity.TokenUsage{
		InputTokens:         a.InputTokens + b.InputTokens,
		OutputTokens:        a.OutputTokens + b.OutputTokens,
		CacheCreationTokens: a.CacheCreationTokens + b.CacheCreationTokens,
		CacheReadTokens:     a.CacheReadTokens + b.CacheReadTokens,
	}
}

// looksLikeV2Blocks 判断 raw 是否是 cago v2 形状（type ∈ {text,display_text,
// tool_use,tool_result,thinking,metadata,image}）。
//
// v1 ContentBlock 有 toolName/toolInput/content/status 等字段；v2 用 name/input/text 等。
// metadata 留作老 DB 行兼容（pre-DisplayTextBlock 写入的 {"type":"metadata",
// "key":"display"} 行）。
func looksLikeV2Blocks(raw []map[string]any) bool {
	for _, b := range raw {
		t, _ := b["type"].(string)
		switch t {
		case "tool_use", "tool_result", "thinking", "metadata", "image", "display_text":
			return true
		case "text":
			if _, ok := b["text"]; ok {
				return true
			}
		}
	}
	return false
}

func onlyToolResults(raw []map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for _, b := range raw {
		t, _ := b["type"].(string)
		if t != "tool_result" {
			return false
		}
	}
	return true
}

// extractV2UserDisplay 优先返回 display_text 块的 text（mention 扩展前的用户原文，
// cago Audience 体系下 LLM 不可见 + UI/储存可见的官方载体）；否则降级到老格式
// metadata{key:"display"}（pre-cago-rename DB 行兼容）；都没有时拼接所有 text 块。
func extractV2UserDisplay(raw []map[string]any) string {
	for _, b := range raw {
		if t, _ := b["type"].(string); t == "display_text" {
			if s, _ := b["text"].(string); s != "" {
				return s
			}
		}
	}
	// 兼容：老行写的是 metadata{key:"display"}。
	for _, b := range raw {
		if t, _ := b["type"].(string); t == "metadata" {
			if k, _ := b["key"].(string); k == "display" {
				if s, _ := b["value"].(string); s != "" {
					return s
				}
			}
		}
	}
	var sb strings.Builder
	for _, b := range raw {
		if t, _ := b["type"].(string); t == "text" {
			if s, _ := b["text"].(string); s != "" {
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

// decodeV2AssistantBlocks 把 v2 raw blocks 转成前端 ContentBlock 序列。
// content 是拼接出来的纯文本（消息 plain text 回退用，包含所有 text 块的文本）。
func decodeV2AssistantBlocks(raw []map[string]any) ([]conversation_entity.ContentBlock, string) {
	blocks := make([]conversation_entity.ContentBlock, 0, len(raw))
	var sb strings.Builder
	for _, b := range raw {
		t, _ := b["type"].(string)
		switch t {
		case "text":
			s, _ := b["text"].(string)
			if s == "" {
				continue
			}
			blocks = append(blocks, conversation_entity.ContentBlock{Type: "text", Content: s})
			sb.WriteString(s)
		case "thinking":
			s, _ := b["text"].(string)
			if s == "" {
				continue
			}
			blocks = append(blocks, conversation_entity.ContentBlock{Type: "thinking", Content: s, Status: "completed"})
		case "tool_use":
			id, _ := b["id"].(string)
			name, _ := b["name"].(string)
			var inputJSON string
			if inp, ok := b["input"]; ok && inp != nil {
				if mb, err := json.Marshal(inp); err == nil {
					inputJSON = string(mb)
				}
			}
			blocks = append(blocks, conversation_entity.ContentBlock{
				Type:       "tool",
				ToolName:   name,
				ToolInput:  inputJSON,
				ToolCallID: id,
				Status:     "completed",
			})
		}
	}
	return blocks, sb.String()
}

// mergeToolResultsIntoLastAssistant 把 tool_result 块的 content 回填到 out 中
// 最近一条 assistant 消息里 toolCallId 匹配的 tool block。
func mergeToolResultsIntoLastAssistant(out []ConversationDisplayMessage, raw []map[string]any) {
	if len(out) == 0 {
		return
	}
	// 反向找最后一条 assistant。
	lastIdx := -1
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role == "assistant" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return
	}
	target := &out[lastIdx]
	for _, b := range raw {
		if t, _ := b["type"].(string); t != "tool_result" {
			continue
		}
		callID, _ := b["tool_use_id"].(string)
		isErr, _ := b["is_error"].(bool)
		content := flattenToolResultContent(b["content"])
		for j := range target.Blocks {
			if target.Blocks[j].Type == "tool" && target.Blocks[j].ToolCallID == callID {
				target.Blocks[j].Content = content
				if isErr {
					target.Blocks[j].Status = "error"
				}
				break
			}
		}
	}
}

// flattenToolResultContent 把 tool_result.content（嵌套 ContentBlock 列表）
// 平铺成一段文本：text 块直接拼接，其余块用 [type] 占位。
func flattenToolResultContent(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		switch t {
		case "text":
			if s, _ := m["text"].(string); s != "" {
				sb.WriteString(s)
			}
		default:
			sb.WriteString("[")
			sb.WriteString(t)
			sb.WriteString("]")
		}
	}
	return sb.String()
}

func (a *App) loadConversationDisplayMessages(ctx context.Context, id int64) ([]ConversationDisplayMessage, error) {
	msgs, err := conversation_svc.Conversation().LoadMessages(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildDisplayMessages(msgs), nil
}

// switchToConversation 内部切换会话逻辑
func (a *App) switchToConversation(conv *conversation_entity.Conversation) {
	a.currentConversationID = conv.ID
}

// DeleteConversation 删除会话
func (a *App) DeleteConversation(id int64) error {
	if a.mgr != nil {
		_ = a.mgr.CloseHandle(id)
	}

	err := conversation_svc.Conversation().Delete(a.langCtx(), id)
	if err != nil {
		return err
	}
	if a.currentConversationID == id {
		a.currentConversationID = 0
	}
	return nil
}

// buildCagoProvider 用 OpsKat 的 AIProvider 配置构建 cago provider.Provider。
func (a *App) buildCagoProvider(p *ai_provider_entity.AIProvider) (provider.Provider, error) {
	apiKey, err := ai_provider_svc.AIProvider().DecryptAPIKey(p)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key: %w", err)
	}
	switch p.Type {
	case "anthropic":
		return anthropics.NewProvider(anthropics.Config{
			APIKey:  apiKey,
			BaseURL: p.APIBase,
		}), nil
	default:
		cfg := openaiSDK.DefaultConfig(apiKey)
		if p.APIBase != "" {
			cfg.BaseURL = p.APIBase
		}
		return openaiCago.NewProvider(cfg), nil
	}
}

// SendAIMessage 发送一轮对话到 cago Manager。首条消息时自动创建会话 + 设置标题。
// cago 自身持久化历史，因此只把最后一条 user 消息当作 prompt 传入。
//
// mention 元数据通过 inline XML 标签携带在 llmBody 里（ai.WrapMentions 包装），
// gormStore.AppendMessage 在 user role 时反向 ai.ParseMentions 落到 row.Mentions——
// 元数据与消息正文原子绑定，无需 stash 旁路。raw 保留用户原文用于前端展示。
func (a *App) SendAIMessage(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
	ctx := a.langCtx()

	if convID == 0 {
		conv, err := a.CreateConversation()
		if err != nil {
			return fmt.Errorf("创建会话失败: %w", err)
		}
		convID = conv.ID
	}

	if conv, err := conversation_svc.Conversation().Get(ctx, convID); err == nil && conv.Title == "新对话" {
		for _, msg := range messages {
			if msg.Role == ai.RoleUser {
				title := normalizeConversationTitle(string(msg.Content))
				if err := conversation_svc.Conversation().UpdateTitle(ctx, convID, title); err != nil {
					logger.Default().Error("update conversation title", zap.Error(err))
				}
				break
			}
		}
	}

	// Extract the last user turn — Manager-backed Conv has its own persisted history.
	var prompt string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleUser {
			prompt = string(messages[i].Content)
			break
		}
	}
	if prompt == "" {
		return fmt.Errorf("no user message in send")
	}

	if a.mgr == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}
	body := ai.WrapMentions(prompt, aiCtx.MentionedAssets)
	h, err := a.mgr.Handle(ctx, convID)
	if err != nil {
		return err
	}
	return h.Send(ctx, prompt, body)
}

// QueueAIMessage 在生成过程中通过 cago Steer 注入用户消息。
// mention 元数据通过 inline XML 标签携带在 llmBody 里，gormStore 反向解析落库。
func (a *App) QueueAIMessage(convID int64, content string, mentions []ai.MentionedAsset) error {
	if a.mgr == nil {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	body := ai.WrapMentions(content, mentions)
	h, err := a.mgr.Handle(a.langCtx(), convID)
	if err != nil {
		return err
	}
	return h.Send(a.langCtx(), content, body)
}

// EditAIMessage 截断 conv 在 idx 处，重新追加 user 消息并重生成。
// mention 元数据通过 inline XML 标签携带在 llmBody 里，与 SendAIMessage 一致。
func (a *App) EditAIMessage(convID int64, idx int, content string, mentions []ai.MentionedAsset) error {
	if a.mgr == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}
	body := ai.WrapMentions(content, mentions)
	h, err := a.mgr.Handle(a.langCtx(), convID)
	if err != nil {
		return err
	}
	return h.Edit(a.langCtx(), idx, content, body)
}

// RegenerateAIMessage 截断指定 assistant 消息及其后所有消息，从前一条 user
// 消息重新生成。不引入新的 user 消息，因此无需 mentions 参数。
func (a *App) RegenerateAIMessage(convID int64, assistIdx int) error {
	if a.mgr == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}
	h, err := a.mgr.Handle(a.langCtx(), convID)
	if err != nil {
		return err
	}
	return h.Regenerate(a.langCtx(), assistIdx)
}

// RunAISlashResult mirrors the pre-v2 slash result shape for Wails type generation.
type RunAISlashResult struct {
	IsSlash bool   `json:"isSlash"`
	Prompt  string `json:"prompt"`
	Notice  string `json:"notice"`
}

// RunAISlash 解析 /slash 命令。cago v2 还没有 slash 解析器，这里返回 stub —
// 非 / 开头视为普通消息，/ 开头返回带说明的 Notice。完整 slash 支持留待后续 Task。
func (a *App) RunAISlash(_ int64, line string) (*RunAISlashResult, error) {
	if !strings.HasPrefix(line, "/") {
		return &RunAISlashResult{IsSlash: false}, nil
	}
	return &RunAISlashResult{
		IsSlash: true,
		Notice:  fmt.Sprintf("slash 命令暂时不可用（cago v2 迁移中）：%s", line),
	}, nil
}

// StopAIGeneration 停止指定会话的 AI 生成。
//
// handle 存在 → h.Cancel 触发 cago EventCancelled，bridge 那边统一发 "stopped"
// （见 bridge.go EventCancelled case，单源原则避免双发）。
// handle 缺失 → 没有 cago 事件源，直接 emit 让前端清 UI。
func (a *App) StopAIGeneration(convID int64) error {
	if a.mgr != nil {
		if h, err := a.mgr.Handle(a.langCtx(), convID); err == nil {
			_ = h.Cancel("user")
			return nil
		}
	}
	eventName := fmt.Sprintf("ai:event:%d", convID)
	wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{Type: "stopped"})
	return nil
}

// GetCurrentConversationID 获取当前会话ID
func (a *App) GetCurrentConversationID() int64 {
	return a.currentConversationID
}

// WaitAIFlushAck 返回用于等待前端 flush 完成的 channel。
// main.go 的 OnBeforeClose 使用：先 drain 掉旧信号，再 emit 事件，最后 select 等待 ack 或超时。
func (a *App) WaitAIFlushAck() <-chan struct{} {
	return a.flushAckCh
}

// DrainAIFlushAck 清空可能残留的 ack 信号，避免误把上次的 ack 当作本次响应。
func (a *App) DrainAIFlushAck() {
	select {
	case <-a.flushAckCh:
	default:
	}
}

// subscribeAIFlushAck 在 Startup 中注册：前端完成会话落盘后会 EventsEmit("ai:flush-done")，
// 这里把信号推入 channel，供 OnBeforeClose 等待。
func (a *App) subscribeAIFlushAck() {
	wailsRuntime.EventsOn(a.ctx, "ai:flush-done", func(_ ...any) {
		select {
		case a.flushAckCh <- struct{}{}:
		default:
		}
	})
}
