package app

import (
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/ai_provider_svc"
	"github.com/opskat/opskat/internal/service/conversation_svc"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
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
		provider = ai.NewAnthropicProvider(p.Name, p.APIBase, apiKey, p.Model, maxOutputTokens)
	default: // "openai"
		provider = ai.NewOpenAIProvider(p.Name, p.APIBase, apiKey, p.Model, maxOutputTokens)
	}

	config := ai.NewDefaultConfig()
	config.ContextWindow = contextWindow
	a.aiAgent = ai.NewAgent(provider, func() ai.ToolExecutor {
		return ai.NewAuditingExecutor(ai.NewDefaultToolExecutor(), ai.NewDefaultAuditWriter())
	}, checker, config)
	return nil
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
	Role    string                             `json:"role"`
	Content string                             `json:"content"`
	Blocks  []conversation_entity.ContentBlock `json:"blocks"`
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

	conv := &conversation_entity.Conversation{
		Title:      "新对话",
		ProviderID: providerID,
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

// SwitchConversation 切换到指定会话，返回显示消息
func (a *App) SwitchConversation(id int64) ([]ConversationDisplayMessage, error) {
	ctx := a.langCtx()
	conv, err := conversation_svc.Conversation().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}

	a.switchToConversation(conv)

	// 加载消息用于前端显示
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
		displayMsgs = append(displayMsgs, ConversationDisplayMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Blocks:  blocks,
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
	// 先停止正在运行的生成
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

// SendAIMessage 发送 AI 消息，通过 Wails Events 流式返回
// convID 指定目标会话，支持多会话并发发送
func (a *App) SendAIMessage(convID int64, messages []ai.Message, aiCtx ai.AIContext) error {
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
				title := string(msg.Content)
				if len([]rune(title)) > 50 {
					title = string([]rune(title)[:50])
				}
				conv.Title = title
				if err := conversation_svc.Conversation().Update(ctx, conv); err != nil {
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
	if a.extBridge != nil {
		for _, tab := range aiCtx.OpenTabs {
			if skillMD := a.extBridge.GetSkillMD(tab.Type); skillMD != "" {
				builder.SetExtensionSkillMD(skillMD)
				break
			}
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

// QueueAIMessage 在生成过程中追加用户消息到队列，
// 会在下一次工具调用结束后被注入到对话上下文
func (a *App) QueueAIMessage(convID int64, content string) error {
	v, ok := a.runners.Load(convID)
	if !ok {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	runner := v.(*ai.ConversationRunner)
	runner.QueueMessage(ai.Message{
		Role:    ai.RoleUser,
		Content: content,
	})
	return nil
}

// StopAIGeneration 停止指定会话的 AI 生成
func (a *App) StopAIGeneration(convID int64) error {
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
		msgs = append(msgs, msg)
	}
	return conversation_svc.Conversation().SaveMessages(ctx, convID, msgs)
}

// GetCurrentConversationID 获取当前会话ID
func (a *App) GetCurrentConversationID() int64 {
	return a.currentConversationID
}

// RespondPermission 前端响应权限确认请求
func (a *App) RespondPermission(behavior, message string) {
	resp := ai.PermissionResponse{Behavior: behavior, Message: message}
	select {
	case a.permissionChan <- resp:
	default:
	}
}
