package ai

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// RunnerState 表示 ConversationRunner 的当前状态
type RunnerState int

const (
	RunnerIdle RunnerState = iota
	RunnerRunning
	RunnerRetrying
	RunnerStopping
)

const (
	MaxRetries    = 10
	MaxRetryDelay = 15 * time.Second
)

// retryDelays 定义每次重试的基础延迟（1-indexed）
var retryDelays = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
}

// pendingMessage 排队的用户消息：msg 注入到 LLM 上下文，displayContent 用于通知前端展示。
// 两者可不同 —— 例如带 @ 提及时，msg.Content 会附加 mention 上下文，displayContent 仍是原始用户输入。
type pendingMessage struct {
	msg            Message
	displayContent string
}

// ConversationRunner 管理单个 AI 会话的生命周期（启动、停止、重试）
type ConversationRunner struct {
	agent       *Agent
	state       RunnerState
	cancel      context.CancelFunc
	retry       int
	done        chan struct{}
	mu          sync.Mutex
	pendingMsgs []pendingMessage
}

// NewConversationRunner 创建新的 runner
func NewConversationRunner(agent *Agent) *ConversationRunner {
	return &ConversationRunner{
		agent: agent,
		state: RunnerIdle,
	}
}

// State 返回当前状态
func (r *ConversationRunner) State() RunnerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Start 启动对话，如果已在运行则返回错误
func (r *ConversationRunner) Start(ctx context.Context, messages []Message, onEvent func(StreamEvent)) error {
	r.mu.Lock()
	if r.state != RunnerIdle {
		r.mu.Unlock()
		return errors.New("runner is already active")
	}
	r.state = RunnerRunning
	chatCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.retry = 0
	r.done = make(chan struct{})
	r.mu.Unlock()

	go r.run(chatCtx, messages, onEvent)
	return nil
}

// Stop 取消当前生成并等待 goroutine 退出
func (r *ConversationRunner) Stop() {
	r.mu.Lock()
	if r.state == RunnerIdle {
		r.mu.Unlock()
		return
	}
	r.state = RunnerStopping
	if r.cancel != nil {
		r.cancel()
	}
	done := r.done
	r.mu.Unlock()

	if done != nil {
		<-done
	}
}

// QueueMessage 向正在运行的会话追加一条用户消息，
// 会在下一次工具调用结束后、下一轮 LLM 调用前被消费。
// displayContent 是给前端展示的原始内容；为空时回退到 msg.Content。
func (r *ConversationRunner) QueueMessage(displayContent string, msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if displayContent == "" {
		displayContent = msg.Content
	}
	r.pendingMsgs = append(r.pendingMsgs, pendingMessage{msg: msg, displayContent: displayContent})
}

// drainPendingMessages 取出并清空所有待处理消息
func (r *ConversationRunner) drainPendingMessages() []pendingMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pendingMsgs) == 0 {
		return nil
	}
	msgs := r.pendingMsgs
	r.pendingMsgs = nil
	return msgs
}

func (r *ConversationRunner) run(ctx context.Context, messages []Message, onEvent func(StreamEvent)) {
	defer func() {
		r.mu.Lock()
		r.state = RunnerIdle
		r.cancel = nil
		// 清空残留的排队消息：本轮 Chat 已结束，未消费的队列归前端 drainQueue 重新发送，
		// 否则下一次 Start 复用同一个 runner 时会被误当作新一轮的 mid-cycle 消息再次注入。
		r.pendingMsgs = nil
		r.mu.Unlock()
		close(r.done)
	}()

	for {
		err := r.agent.Chat(ctx, messages, onEvent, r.drainPendingMessages)

		// 正常完成
		if err == nil {
			onEvent(StreamEvent{Type: "done"})
			return
		}

		// 用户停止
		if ctx.Err() != nil {
			onEvent(StreamEvent{Type: "stopped"})
			return
		}

		// 重试逻辑
		r.mu.Lock()
		r.retry++
		attempt := r.retry
		if attempt > MaxRetries {
			r.mu.Unlock()
			onEvent(StreamEvent{
				Type:  "error",
				Error: err.Error(),
			})
			return
		}
		r.state = RunnerRetrying
		r.mu.Unlock()

		delay := calcRetryDelay(attempt, err)

		// 通知前端重试状态
		onEvent(StreamEvent{
			Type:    "retry",
			Content: strconv.Itoa(attempt) + "/" + strconv.Itoa(MaxRetries),
			Error:   err.Error(),
		})

		// 等待退避时间，支持取消
		select {
		case <-time.After(delay):
			r.mu.Lock()
			r.state = RunnerRunning
			r.mu.Unlock()
			continue
		case <-ctx.Done():
			onEvent(StreamEvent{Type: "stopped"})
			return
		}
	}
}

// calcRetryDelay 计算重试延迟，优先使用 Retry-After 头，带 ±20% 抖动
func calcRetryDelay(attempt int, err error) time.Duration {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.RetryAfter != "" {
		if seconds, parseErr := strconv.Atoi(providerErr.RetryAfter); parseErr == nil && seconds > 0 {
			return addJitter(time.Duration(seconds) * time.Second)
		}
	}

	idx := attempt - 1
	if idx >= len(retryDelays) {
		idx = len(retryDelays) - 1
	}
	return addJitter(retryDelays[idx])
}

func addJitter(base time.Duration) time.Duration {
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(base) * jitter)
}
