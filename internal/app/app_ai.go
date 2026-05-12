package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/ai_provider_svc"
	"github.com/opskat/opskat/internal/service/conversation_svc"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// runnerEntry 持有一个活跃会话的 cago 运行栈：coding.System 控制工具集 + 父 agent，
// agent.Runner 承担实际的 Send / Steer / Cancel / Close；done channel 用于 Stop 时
// 等待事件消费 goroutine 退出。
type runnerEntry struct {
	sys        *coding.System
	runner     *agent.Runner
	done       chan struct{}
	sshCache   *ai.SSHClientCache
	dbCache    *ai.DatabaseClientCache
	redisCache *ai.RedisClientCache
	mongoCache *ai.MongoDBClientCache
}

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

// activateProvider 根据 Provider 配置准备 BuildSystem 所需的依赖。
// 每次 SendAIMessage 时按需 BuildSystem → cago.coding.New → agent.Runner。
func (a *App) activateProvider(p *ai_provider_entity.AIProvider) error {
	apiKey, err := ai_provider_svc.AIProvider().DecryptAPIKey(p)
	if err != nil {
		return fmt.Errorf("解密 API Key 失败: %w", err)
	}

	checker := ai.NewCommandPolicyChecker(a.makeCommandConfirmFunc())
	checker.SetGrantRequestFunc(a.makeGrantRequestFunc())
	a.policyChecker = checker

	cwd, err := defaultAICwd()
	if err != nil {
		return fmt.Errorf("准备 AI 工作目录失败: %w", err)
	}

	a.aiSystemCfg = &ai.SystemConfig{
		ProviderEntity: p,
		APIKey:         apiKey,
		Cwd:            cwd,
		Tools:          ai.Tools(),
		LocalToolGate:  ai.NewLocalToolGate(a.makeLocalToolConfirmFunc()),
	}
	a.resetRunners()
	return nil
}

// defaultAICwd 默认 AI 工作目录 = ~/.opskat。不存在时自动创建。
func defaultAICwd() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cwd := filepath.Join(home, ".opskat")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return "", err
	}
	return cwd, nil
}

// resetRunners 停止并清空所有缓存的 runnerEntry。
// Why: runner 创建时绑定的是当时 aiSystemCfg 构造出的 *coding.System，
// 供应商切换后若不清空，已有会话仍使用旧 provider，必须重启软件才生效。
// 并发调用 Cancel 避免串行累计阻塞 UI；另外设 3s 上限——若某个 entry 退出被卡住，
// 超时后放行，泄漏的 goroutine 只是持有旧 sys 引用，不会导致正确性问题。
func (a *App) resetRunners() {
	var wg sync.WaitGroup
	a.runners.Range(func(key, value any) bool {
		if e, ok := value.(*runnerEntry); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.stopEntry(e)
			}()
		}
		a.runners.Delete(key)
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
		logger.Default().Warn("resetRunners: 部分 runner 退出未在 3s 内完成，放行关闭")
	}
}

// stopEntry 取消正在跑的 turn 并等待事件消费 goroutine 退出，最后释放 Runner / System。
// 调用方负责把 entry 从 runners map 移除。
func (a *App) stopEntry(e *runnerEntry) {
	if e == nil {
		return
	}
	if e.runner != nil {
		_ = e.runner.Cancel("user_stop")
	}
	if e.done != nil {
		select {
		case <-e.done:
		case <-time.After(3 * time.Second):
		}
	}
	// 先关 cache 再关 runner/sys：此时正在跑的 tool.Call 已被 Cancel 并随 e.done 退出，
	// cache 内不会再有 in-flight 的 client 使用。ConnCache.Close 已过滤预期错误。
	if e.sshCache != nil {
		if err := e.sshCache.Close(); err != nil {
			logger.Default().Warn("close SSH cache", zap.Error(err))
		}
	}
	if e.dbCache != nil {
		if err := e.dbCache.Close(); err != nil {
			logger.Default().Warn("close database cache", zap.Error(err))
		}
	}
	if e.redisCache != nil {
		if err := e.redisCache.Close(); err != nil {
			logger.Default().Warn("close Redis cache", zap.Error(err))
		}
	}
	if e.mongoCache != nil {
		if err := e.mongoCache.Close(); err != nil {
			logger.Default().Warn("close MongoDB cache", zap.Error(err))
		}
	}
	if e.runner != nil {
		_ = e.runner.Close()
	}
	if e.sys != nil {
		if cerr := e.sys.Close(context.Background()); cerr != nil {
			logger.Default().Warn("close coding system", zap.Error(cerr))
		}
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
	TokenUsage *conversation_entity.TokenUsage    `json:"tokenUsage,omitempty"`
}

// CreateConversation 创建新会话
func (a *App) CreateConversation() (*conversation_entity.Conversation, error) {
	if a.aiSystemCfg == nil {
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
		usage, err := msg.GetTokenUsage()
		if err != nil {
			logger.Default().Warn("get message token usage", zap.Error(err))
		}
		displayMsgs = append(displayMsgs, ConversationDisplayMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Blocks:     blocks,
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
	// 先停止正在运行的生成
	if v, ok := a.runners.LoadAndDelete(id); ok {
		a.stopEntry(v.(*runnerEntry))
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
	if a.aiSystemCfg == nil {
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

	// cago 路径：system prompt 走 BuildSystem 配置注入到 coding.AppendSystem，
	// 不再以 RoleSystem 消息塞进 messages 列表。
	systemPrompt := builder.Build()

	// 注入审计上下文
	chatCtx := ai.WithAuditSource(a.ctx, "ai")
	chatCtx = ai.WithConversationID(chatCtx, convID)
	chatCtx = ai.WithSessionID(chatCtx, fmt.Sprintf("conv_%d", convID))
	chatCtx = logger.WithContextField(chatCtx, zap.Int64("conv_id", convID))
	if a.sshPool != nil {
		chatCtx = ai.WithSSHPool(chatCtx, a.sshPool)
	}

	// 同一次 Send 内复用连接：handler 内 getXxxCache(ctx)!=nil 时走缓存路径。
	// Kafka 沿用应用单例 a.kafkaService（内部 KafkaClientManager 自带缓存），
	// 不放进 runnerEntry，关闭时机归 a.sshPool.Close() 那条路径管。
	sshCache := ai.NewSSHClientCache()
	dbCache := ai.NewDatabaseClientCache()
	redisCache := ai.NewRedisClientCache()
	mongoCache := ai.NewMongoDBClientCache()
	chatCtx = ai.WithSSHCache(chatCtx, sshCache)
	chatCtx = ai.WithDatabaseCache(chatCtx, dbCache)
	chatCtx = ai.WithRedisCache(chatCtx, redisCache)
	chatCtx = ai.WithMongoDBCache(chatCtx, mongoCache)
	if a.kafkaService != nil {
		chatCtx = ai.WithKafkaService(chatCtx, a.kafkaService)
	}

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

	// 注入 policy checker（handler 内部走 GetPolicyChecker(ctx) → setCheckResult）
	if a.policyChecker != nil {
		chatCtx = ai.WithPolicyChecker(chatCtx, a.policyChecker)
	}

	// 每次发送都重建 runnerEntry：messages 是完整历史，需要回放，旧 conv 状态丢弃。
	// 旧 entry 若存在，先取消并释放。
	if v, ok := a.runners.LoadAndDelete(convID); ok {
		a.stopEntry(v.(*runnerEntry))
	}

	cfg := *a.aiSystemCfg
	cfg.SystemPrompt = systemPrompt
	if cfg.ProviderEntity != nil {
		cfg.Model = cfg.ProviderEntity.Model
	}
	sys, err := ai.BuildSystem(chatCtx, cfg)
	if err != nil {
		// 同时 emit error 事件 + 同步 return err：前端 catch 同步 err 清理 sending 状态，
		// 而流式监听器也能拿到 error 事件用于 UI 渲染。两条路径冗余但故意为之。
		onEvent(ai.StreamEvent{Type: "error", Error: fmt.Sprintf("build coding system: %s", err.Error())})
		return fmt.Errorf("build coding system: %w", err)
	}

	history, lastUserText := ai.SplitForReplay(messages)
	conv := agent.LoadConversation(fmt.Sprintf("opskat-conv-%d", convID), ai.ToAgentMessages(history))
	runner := sys.Agent().Runner(conv)

	entry := &runnerEntry{
		sys:        sys,
		runner:     runner,
		done:       make(chan struct{}),
		sshCache:   sshCache,
		dbCache:    dbCache,
		redisCache: redisCache,
		mongoCache: mongoCache,
	}
	a.runners.Store(convID, entry)

	events, err := runner.Send(chatCtx, lastUserText)
	if err != nil {
		close(entry.done)
		a.runners.Delete(convID)
		_ = runner.Close()
		_ = sys.Close(context.Background())
		if chatCtx.Err() != nil {
			// 用户取消：emit stopped 让前端走"已停止"分支，同步返回 nil 不视为错误。
			onEvent(ai.StreamEvent{Type: "stopped"})
			return nil //nolint:nilerr // 取消是用户主动行为，不是错误
		}
		onEvent(ai.StreamEvent{Type: "error", Error: err.Error()})
		return fmt.Errorf("send to LLM: %w", err)
	}

	go func() {
		defer close(entry.done)
		translator := ai.NewStreamTranslator()
		for ev := range events {
			translator.Translate(ev, onEvent)
		}
	}()
	return nil
}

// QueueAIMessage 在生成过程中通过 cago Runner.Steer 把用户消息注入当前 turn，
// 由 cago 在下一次 LLM 调用前把消息追加到 conversation。
// content 已包含内联 <mention> XML（前端构造），无需再行 prepend。
func (a *App) QueueAIMessage(convID int64, content string) error {
	v, ok := a.runners.Load(convID)
	if !ok {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	entry := v.(*runnerEntry)
	if entry.runner == nil {
		return fmt.Errorf("会话 %d 没有正在运行的生成", convID)
	}
	err := entry.runner.Steer(context.Background(), content, agent.WithSteerDisplay(content))
	if err != nil && !errors.Is(err, agent.ErrSteerNoActiveTurn) {
		logger.Default().Warn("cago Steer failed", zap.Error(err))
		return err
	}
	return nil
}

// StopAIGeneration 调用 cago Runner.Cancel 触发取消；事件消费 goroutine
// 收到 EventCancelled 后退出，stopEntry 负责清理 Runner / System。
func (a *App) StopAIGeneration(convID int64) error {
	v, ok := a.runners.LoadAndDelete(convID)
	if !ok {
		return nil
	}
	a.stopEntry(v.(*runnerEntry))
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
