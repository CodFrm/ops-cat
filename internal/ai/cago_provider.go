package ai

import (
	"fmt"

	cagoProvider "github.com/cago-frame/agents/provider"
	cagoAnthropics "github.com/cago-frame/agents/provider/anthropics"
	cagoOpenAI "github.com/cago-frame/agents/provider/openai"
	"github.com/sashabaranov/go-openai"

	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
)

// BuildCagoProvider 根据 OpsKat AIProvider entity + 已解密 API Key 构造 cago provider.Provider。
// 后续 agent loop 全部走 cago provider；model / reasoning / max_tokens 在 request 阶段注入，
// 这里只负责"传输层"。
func BuildCagoProvider(p *ai_provider_entity.AIProvider, apiKey string) (cagoProvider.Provider, error) {
	if p == nil {
		return nil, fmt.Errorf("provider 配置为空")
	}
	switch p.Type {
	case "anthropic":
		return cagoAnthropics.NewProvider(cagoAnthropics.Config{
			BaseURL: p.APIBase,
			APIKey:  apiKey,
		}), nil
	case "openai", "":
		cfg := openai.DefaultConfig(apiKey)
		if p.APIBase != "" {
			cfg.BaseURL = p.APIBase
		}
		return cagoOpenAI.NewProvider(cfg), nil
	default:
		return nil, fmt.Errorf("不支持的 provider 类型: %s", p.Type)
	}
}
