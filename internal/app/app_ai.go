package app

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// activateProvider 根据 Provider 配置创建 AI Agent
func (a *App) activateProvider(p *ai_provider_entity.AIProvider) error {
	apiKey, err := ai_provider_svc.AIProvider().DecryptAPIKey(p)
	if err != nil {
		return fmt.Errorf("解密 API Key 失败: %w", err)
	}

	checker := ai.NewCommandPolicyChecker(a.makeCommandConfirmFunc())
	checker.SetGrantRequestFunc(a.makeGrantRequestFunc())

	maxOutputTokens := ai.ResolveMaxOutputTokens(p.MaxOutputTokens, p.Model)
	contextWindow := ai.ResolveContextWindow(p.ContextWindow, p.Model)

	var provider ai.Provider
	switch p.Type {
	case "anthropic":
		provider = ai.NewAnthropicProvider(
			p.Name,
			p.APIBase,
			apiKey,
			p.Model,
			maxOutputTokens,
			p.ReasoningEnabled,
			p.ReasoningEffort,
		)
	default: // "openai"
		provider = ai.NewOpenAIProvider(
			p.Name,
			p.APIBase,
			apiKey,
			p.Model,
			maxOutputTokens,
			p.ReasoningEnabled,
			p.ReasoningEffort,
		)
	}

	config := ai.NewDefaultConfig()
	config.ContextWindow = contextWindow
	a.aiAgent = ai.NewAgent(provider, func() ai.ToolExecutor {
		return ai.NewAuditingExecutor(ai.NewDefaultToolExecutor(), ai.NewDefaultAuditWriter())
	}, checker, config)
	a.resetRunners()
	return nil
}

// resetRunners 停止并清空所有缓存的 ConversationRunner。
// Why: runner 在创建时会捕获当时的 aiAgent 引用，供应商切换后若不清空，
// 已有会话会继续使用旧 provider，必须重启软件才生效。
// 并发调用 Stop 避免串行累计阻塞 UI；另外设 3s 上限——Runner.Stop 会阻塞等
// goroutine 退出，若某个 Stop 被卡住（ctx 不被及时响应等），
// 超时后放行，泄漏的 goroutine 只是持有旧 agent 引用，不会导致正确性问题。
func (a *App) resetRunners() {
	var wg sync.WaitGroup
	a.runners.Range(func(key, value any) bool {
		if r, ok := value.(*ai.ConversationRunner); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.Stop()
			}()
		}
		a.runners.Delete(key)
		return true
	})

	// Cago systems: close in parallel, on the same 3s budget as runners.
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
	// Invalidate the cached cago provider — next dispatch will rebuild from
	// the (potentially newly activated) AI provider config.
	a.aiCagoProvider = nil

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.Default().Warn("resetRunners: 部分 runner Stop 未在 3s 内完成，放行关闭")
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
	if a.aiAgent == nil {
		return nil, fmt.Errorf("请先配置 AI Provider")
	}
	ctx := a.langCtx()

	// 获取激活 Provider ID
	activeProvider, _ := ai_provider_svc.AIProvider().GetActive(ctx)
	var providerID int64
	if activeProvider != nil {
		providerID = activeProvider.ID
	}

	home, _ := os.UserHomeDir()
	conv := &conversation_entity.Conversation{
		Title:      "新对话",
		ProviderID: providerID,
		WorkDir:    home, // user-chosen via UpdateConversationCwd; default = home
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

func (a *App) loadConversationDisplayMessages(ctx context.Context, id int64) ([]ConversationDisplayMessage, error) {
	msgs, err := conversation_svc.Conversation().LoadMessages(ctx, id)
	if err != nil {
		return nil, err
	}

	var displayMsgs []ConversationDisplayMessage
	for _, msg := range msgs {
		blocks, err := msg.GetBlocks()
		if err != nil {
			logger.Default().Warn("get message blocks", zap.Error(err))
		}
		mentions, err := msg.GetMentions()
		if err != nil {
			logger.Default().Warn("get message mentions", zap.Error(err))
		}
		usage, err := msg.GetTokenUsage()
		if err != nil {
			logger.Default().Warn("get message token usage", zap.Error(err))
		}
		displayMsgs = append(displayMsgs, ConversationDisplayMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Blocks:     blocks,
			Mentions:   mentions,
			TokenUsage: usage,
		})
	}
	return displayMsgs, nil
}

// switchToConversation 内部切换会话逻辑
func (a *App) switchToConversation(conv *conversation_entity.Conversation) {
	a.currentConversationID = conv.ID
}

// DeleteConversation 删除会话
func (a *App) DeleteConversation(id int64) error {
	// Cago path: close the system, removing it from the cache.
	// Not gated on aiagent.Enabled() — LoadAndDelete is a no-op on an empty map,
	// and we want to handle the case where someone toggles the flag mid-session.
	if v, ok := a.aiAgentSystems.LoadAndDelete(id); ok {
		_ = v.(*aiagent.System).Close(a.ctx)
	}

	// Legacy path: existing logic, unchanged
	if v, ok := a.runners.Load(id); ok {
		v.(*ai.ConversationRunner).Stop()
		a.runners.Delete(id)
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

// SendAIMessage routes to legacy or cago backend depending on the feature flag.
func (a *App) SendAIMessage(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
	if aiagent.Enabled() {
		return a.sendAIMessageCago(convID, messages, aiCtx)
	}
	return a.sendAIMessageLegacy(convID, messages, aiCtx)
}

// sendAIMessageLegacy is the existing SendAIMessage body, unchanged.
func (a *App) sendAIMessageLegacy(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
	if a.aiAgent == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}

	ctx := a.langCtx()

	// 自动创建会话（首次发消息时）
	if convID == 0 {
		conv, err := a.CreateConversation()
		if err != nil {
			return fmt.Errorf("创建会话失败: %w", err)
		}
		convID = conv.ID
	}

	// 更新会话标题（如果仍是默认标题"新对话"）
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

	eventName := fmt.Sprintf("ai:event:%d", convID)

	// 构建动态系统提示
	lang := "en"
	if a.lang == "zh-cn" {
		lang = "zh-cn"
	}
	builder := ai.NewPromptBuilder(lang, aiCtx)

	// Inject extension SKILL.md based on connected asset types
	if a.extSvc != nil {
		bridge := a.extSvc.Bridge()
		mds := make(map[string]string)
		seen := make(map[string]bool)
		for _, tab := range aiCtx.OpenTabs {
			if seen[tab.Type] {
				continue
			}
			seen[tab.Type] = true
			if skillMD := bridge.GetSkillMDWithExtension(tab.Type); skillMD.Content != "" {
				mds[skillMD.ExtensionName] = skillMD.Content
			}
		}
		if len(mds) > 0 {
			builder.SetExtensionSkillMDs(mds)
		}
	}

	fullMessages := make([]ai.Message, 0, 1+len(messages))
	fullMessages = append(fullMessages, ai.Message{
		Role:    ai.RoleSystem,
		Content: builder.Build(),
	})
	fullMessages = append(fullMessages, messages...)

	// 注入审计上下文
	chatCtx := ai.WithAuditSource(a.ctx, "ai")
	chatCtx = ai.WithConversationID(chatCtx, convID)
	chatCtx = ai.WithSessionID(chatCtx, fmt.Sprintf("conv_%d", convID))
	chatCtx = logger.WithContextField(chatCtx, zap.Int64("conv_id", convID))
	if a.sshPool != nil {
		chatCtx = ai.WithSSHPool(chatCtx, a.sshPool)
	}

	// 注入 Sub Agent 依赖
	chatCtx = ai.WithSpawnAgentDeps(chatCtx, &ai.SpawnAgentDeps{
		Provider: a.aiAgent.GetProvider(),
		Checker:  a.aiAgent.GetPolicyChecker(),
		OnEvent: func(event ai.StreamEvent) {
			wailsRuntime.EventsEmit(a.ctx, eventName, event)
		},
		NewExecutor: func() ai.ToolExecutor {
			return ai.NewAuditingExecutor(ai.NewDefaultToolExecutor(), ai.NewDefaultAuditWriter())
		},
	})

	onEvent := func(event ai.StreamEvent) {
		wailsRuntime.EventsEmit(a.ctx, eventName, event)

		// done/stopped 时更新会话时间
		if event.Type == "done" || event.Type == "stopped" {
			if conv, err := conversation_svc.Conversation().Get(a.ctx, convID); err == nil {
				if err := conversation_svc.Conversation().Update(a.ctx, conv); err != nil {
					logger.Default().Warn("update conversation time", zap.Error(err))
				}
			}
		}
	}

	runner := a.getOrCreateRunner(convID)
	return runner.Start(chatCtx, fullMessages, onEvent)
}

func (a *App) getOrCreateRunner(convID int64) *ai.ConversationRunner {
	v, _ := a.runners.LoadOrStore(convID, ai.NewConversationRunner(a.aiAgent))
	return v.(*ai.ConversationRunner)
}

// buildCagoProvider constructs a cago-side provider.Provider from an OpsKat
// AIProvider config. Used only on the cago path — the legacy *ai.Agent stays
// driven by ai.Provider.
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

// sendAIMessageCago routes a Send through the cago backend. Mirrors
// sendAIMessageLegacy's effects (title set on first turn, Wails event emission)
// but uses *aiagent.System under the hood.
func (a *App) sendAIMessageCago(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
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
// builds a new one. The cago provider is built lazily on first call after
// activateProvider; it's invalidated by resetRunners.
func (a *App) getOrCreateAIAgentSystem(convID int64) (*aiagent.System, error) {
	if v, ok := a.aiAgentSystems.Load(convID); ok {
		return v.(*aiagent.System), nil
	}

	if a.aiCagoProvider == nil {
		active, err := ai_provider_svc.AIProvider().GetActive(a.langCtx())
		if err != nil || active == nil {
			return nil, fmt.Errorf("请先配置 AI Provider")
		}
		prov, err := a.buildCagoProvider(active)
		if err != nil {
			return nil, fmt.Errorf("build cago provider: %w", err)
		}
		a.aiCagoProvider = prov
	}

	conv, err := conversation_svc.Conversation().Get(a.langCtx(), convID)
	if err != nil {
		return nil, fmt.Errorf("conversation %d not found: %w", convID, err)
	}
	cwd := conv.WorkDir
	if cwd == "" {
		if home, herr := os.UserHomeDir(); herr == nil {
			cwd = home
		} else {
			cwd = os.TempDir()
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
		Provider:      a.aiCagoProvider,
		Cwd:           cwd,
		ConvID:        convID,
		Lang:          lang,
		Deps:          deps,
		Emitter:       emitter,
		PolicyChecker: checker,
		Resolver:      resolver,
		Activate:      func() { a.activateWindow() },
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

// QueueAIMessage 在生成过程中追加用户消息到队列，
// 会在下一次工具调用结束后被注入到对话上下文。
// 若消息带 @ 提及的资产，将资产上下文渲染后 prepend 到消息正文，
// 以便 agent 在后续轮次看到 mention 信息（系统提示在 Start 时已定型，无法再次重建）。
func (a *App) QueueAIMessage(convID int64, content string, mentions []ai.MentionedAsset) error {
	body := content
	if mentionCtx := ai.RenderMentionContext(mentions); mentionCtx != "" {
		body = mentionCtx + "\n\n" + content
	}

	if aiagent.Enabled() {
		v, ok := a.aiAgentSystems.Load(convID)
		if !ok {
			return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
		}
		return v.(*aiagent.System).Steer(a.ctx, body, content)
	}

	// Legacy path
	v, ok := a.runners.Load(convID)
	if !ok {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	runner := v.(*ai.ConversationRunner)
	runner.QueueMessage(content, ai.Message{
		Role:    ai.RoleUser,
		Content: body,
	})
	return nil
}

// StopAIGeneration 停止指定会话的 AI 生成
func (a *App) StopAIGeneration(convID int64) error {
	if aiagent.Enabled() {
		if v, ok := a.aiAgentSystems.Load(convID); ok {
			v.(*aiagent.System).StopStream()
		}
		// Emit "stopped" so the frontend can clear the in-flight tool block UI.
		// Idempotent: a cancelled Stream also emits its own done/error events;
		// this just guarantees the legacy contract on the cago path.
		eventName := fmt.Sprintf("ai:event:%d", convID)
		wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{Type: "stopped"})
		return nil
	}

	// Legacy path
	v, ok := a.runners.Load(convID)
	if !ok {
		return nil
	}
	runner := v.(*ai.ConversationRunner)
	runner.Stop()
	return nil
}

// SaveConversationMessages 前端调用，保存显示消息到数据库
// convID 指定目标会话，支持多会话独立保存
func (a *App) SaveConversationMessages(convID int64, displayMsgs []ConversationDisplayMessage) error {
	if convID == 0 {
		return nil
	}
	ctx := a.langCtx()
	var msgs []*conversation_entity.Message
	for i, dm := range displayMsgs {
		msg := &conversation_entity.Message{
			ConversationID: convID,
			Role:           dm.Role,
			Content:        dm.Content,
			SortOrder:      i,
			Createtime:     time.Now().Unix(),
		}
		if err := msg.SetBlocks(dm.Blocks); err != nil {
			logger.Default().Error("set message blocks", zap.Error(err))
		}
		if err := msg.SetMentions(dm.Mentions); err != nil {
			logger.Default().Error("set message mentions", zap.Error(err))
		}
		if err := msg.SetTokenUsage(dm.TokenUsage); err != nil {
			logger.Default().Error("set message token usage", zap.Error(err))
		}
		msgs = append(msgs, msg)
	}
	return conversation_svc.Conversation().SaveMessages(ctx, convID, msgs)
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
