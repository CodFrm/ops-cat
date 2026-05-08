package ai

import "context"

// ProviderError wraps API errors with retry metadata
//
// Deprecated: Use github.com/cago-frame/agents/provider.ProviderError on the
// cago path. This type remains only for the legacy provider implementations
// (anthropic.go / openai.go) until they are removed.
type ProviderError struct {
	Err        error
	RetryAfter string // from HTTP Retry-After header
	StatusCode int
}

func (e *ProviderError) Error() string {
	return e.Err.Error()
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// Provider AI 服务提供者接口（legacy）
//
// Deprecated: Use cago providers (provider.Provider). This interface remains
// only because legacy AnthropicProvider / OpenAIProvider still satisfy it,
// pending their removal.
type Provider interface {
	// Chat 发送对话，返回流式事件 channel
	Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error)
	// Name 返回 provider 名称
	Name() string
	// Model 返回当前使用的模型 ID（用于 agent 层根据模型类型决定是否需要回传 reasoning_content 等）
	Model() string
}
