package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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

// activateProvider 根据激活的 AI Provider 配置构建 cago provider 并清空旧 Systems。
// 切换 provider 时若不重置 aiAgentSystems，旧 *aiagent.System 会继续持有旧 provider
// 引用，必须重启软件才能生效。
func (a *App) activateProvider(p *ai_provider_entity.AIProvider) error {
	prov, err := a.buildCagoProvider(p)
	if err != nil {
		return err
	}
	a.aiProvider = prov
	a.aiModel = p.Model
	a.resetAIAgentSystems()
	return nil
}

// resetAIAgentSystems 关闭并清空所有缓存的 *aiagent.System。
// 并发 Close 配 3s 超时上限：cago Close 是幂等的且通常很快，但若某个 Close 因
// goroutine 调度问题卡住，超时后放行避免阻塞 UI；泄漏的 System 只持有旧 provider
// 引用，不影响正确性。
func (a *App) resetAIAgentSystems() {
	var wg sync.WaitGroup
	a.aiAgentSystems.Range(func(key, value any) bool {
		if sys, ok := value.(*aiagent.System); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = sys.Close(a.ctx)
			}()
		}
		a.aiAgentSystems.Delete(key)
		return true
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.Default().Warn("resetAIAgentSystems: 部分 System Close 未在 3s 内完成，放行关闭")
	}
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

// ConversationDisplayMessage 返回给前端的会话消息（用于恢复显示）
type ConversationDisplayMessage struct {
	Role       string                             `json:"role"`
	Content    string                             `json:"content"`
	Blocks     []conversation_entity.ContentBlock `json:"blocks"`
	Mentions   []conversation_entity.MentionRef   `json:"mentions"`
	TokenUsage *conversation_entity.TokenUsage    `json:"tokenUsage,omitempty"`
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

// buildDisplayMessages 把按 sort_order 排好的 cago-shape conversation_messages 行
// 聚合成前端展示用的 ConversationDisplayMessage 列表。
//
// TODO(Task 20): v1 path removed; await Phase 3 rewrite with v2 Message struct.
// This stub returns an empty slice to keep the build green.
func buildDisplayMessages(_ []*conversation_entity.Message) []ConversationDisplayMessage {
	// v1 path removed; await Task 20 rewrite
	return nil
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
	if v, ok := a.aiAgentSystems.LoadAndDelete(id); ok {
		_ = v.(*aiagent.System).Close(a.ctx)
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

// SendAIMessage 发送一轮对话到 cago 后端。首条消息时自动创建会话 + 设置标题；
// cago Session 持久化历史，因此只把最后一条 user 消息当作 prompt 传入。
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

	// Extract the last user turn — cago Session has its own persisted history.
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

	sys, err := a.getOrCreateAIAgentSystem(convID)
	if err != nil {
		return err
	}

	// Per-turn extension skill MD: same logic as legacy path.
	ext := make(map[string]string)
	if a.extSvc != nil {
		bridge := a.extSvc.Bridge()
		seen := make(map[string]bool)
		for _, tab := range aiCtx.OpenTabs {
			if seen[tab.Type] {
				continue
			}
			seen[tab.Type] = true
			if skillMD := bridge.GetSkillMDWithExtension(tab.Type); skillMD.Content != "" {
				ext[skillMD.ExtensionName] = skillMD.Content
			}
		}
	}

	return sys.Stream(ctx, prompt, aiCtx, ext)
}

// getOrCreateAIAgentSystem returns a cached *aiagent.System for convID or
// builds a new one. activateProvider eagerly builds aiProvider, so we just
// guard against the no-active-provider case here.
func (a *App) getOrCreateAIAgentSystem(convID int64) (*aiagent.System, error) {
	if v, ok := a.aiAgentSystems.Load(convID); ok {
		return v.(*aiagent.System), nil
	}

	if a.aiProvider == nil {
		return nil, fmt.Errorf("请先配置 AI Provider")
	}

	conv, err := conversation_svc.Conversation().Get(a.langCtx(), convID)
	if err != nil {
		return nil, fmt.Errorf("conversation %d not found: %w", convID, err)
	}
	cwd := conv.WorkDir
	if cwd == "" {
		var derr error
		cwd, derr = defaultConversationWorkDir()
		if derr != nil {
			return nil, derr
		}
	}

	lang := "en"
	if a.lang == "zh-cn" {
		lang = "zh-cn"
	}

	checker := ai.NewCommandPolicyChecker(a.makeCommandConfirmFunc())
	checker.SetGrantRequestFunc(a.makeGrantRequestFunc())

	deps := &aiagent.Deps{
		SSHPool:       a.sshPool,
		KafkaService:  a.kafkaService,
		PolicyChecker: checker,
	}

	eventName := fmt.Sprintf("ai:event:%d", convID)
	emitter := aiagent.EmitterFunc(func(_ int64, event ai.StreamEvent) {
		wailsRuntime.EventsEmit(a.ctx, eventName, event)
	})

	resolver := aiagent.PendingResolver(func(confirmID string) (chan ai.ApprovalResponse, func()) {
		ch := make(chan ai.ApprovalResponse, 1)
		a.pendingAIApprovals.Store(confirmID, ch)
		return ch, func() { a.pendingAIApprovals.Delete(confirmID) }
	})

	sys, err := aiagent.NewSystem(a.ctx, aiagent.SystemOptions{
		Provider: a.aiProvider,
		Model:    a.aiModel,
		Cwd:      cwd,
		ConvID:   convID,
		Lang:     lang,
		Deps:     deps,
		Emitter:  emitter,
		// CheckPerm 默认走 ai.CheckPermission（静态策略，不触发审批回调），审批弹卡走
		// gw.RequestSingle 通过 emitter 闭包发出，对得上当前会话的 ai:event:<convID>。
		// 老路径上 deps.PolicyChecker / makeCommandConfirmFunc 仍保留，给 batch_command
		// 和 tool handler 内部冗余 check 用。
		Resolver: resolver,
		Activate: func() { a.activateWindow() },
	})
	if err != nil {
		return nil, fmt.Errorf("create aiagent.System: %w", err)
	}

	if existing, loaded := a.aiAgentSystems.LoadOrStore(convID, sys); loaded {
		// Race: another goroutine created one first. Discard ours, return the winner.
		_ = sys.Close(a.ctx)
		return existing.(*aiagent.System), nil
	}
	return sys, nil
}

// QueueAIMessage 在生成过程中通过 cago Steer 注入用户消息。
// 若消息带 @ 提及的资产，将资产上下文渲染后 prepend 到消息正文。
func (a *App) QueueAIMessage(convID int64, content string, mentions []ai.MentionedAsset) error {
	body := content
	if mentionCtx := ai.RenderMentionContext(mentions); mentionCtx != "" {
		body = mentionCtx + "\n\n" + content
	}
	v, ok := a.aiAgentSystems.Load(convID)
	if !ok {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	return v.(*aiagent.System).Steer(a.ctx, body, content)
}

// RunAISlashResult mirrors aiagent.SlashResult shape for Wails type generation.
type RunAISlashResult struct {
	IsSlash bool   `json:"isSlash"`
	Prompt  string `json:"prompt"`
	Notice  string `json:"notice"`
}

// RunAISlash 解析 /slash 命令。
//   - IsSlash=false → 前端把 line 当普通消息走 SendAIMessage。
//   - Prompt 非空 → 前端用 Prompt 调 SendAIMessage（模板展开）。
//   - Notice 非空 → 前端把它作为合成的 system 消息渲染（/help、/compact 摘要）。
//
// 未识别的 /command 返回 aiagent.ErrUnknownSlashCommand。
func (a *App) RunAISlash(convID int64, line string) (*RunAISlashResult, error) {
	v, ok := a.aiAgentSystems.Load(convID)
	if !ok {
		return nil, fmt.Errorf("会话 %d 没有活跃的 AI System", convID)
	}
	sys := v.(*aiagent.System)
	res, err := sys.RunSlash(a.langCtx(), line)
	if err != nil {
		return nil, err
	}
	return &RunAISlashResult{
		IsSlash: res.IsSlash,
		Prompt:  res.Prompt,
		Notice:  res.Notice,
	}, nil
}

// StopAIGeneration 停止指定会话的 AI 生成。
// 即使 sys 不存在也会 emit stopped，让前端清掉进行中的 tool block。
func (a *App) StopAIGeneration(convID int64) error {
	if v, ok := a.aiAgentSystems.Load(convID); ok {
		v.(*aiagent.System).StopStream()
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

// RespondPermission 前端响应权限确认请求
func (a *App) RespondPermission(behavior, message string) {
	resp := ai.PermissionResponse{Behavior: behavior, Message: message}
	select {
	case a.permissionChan <- resp:
	default:
	}
}
