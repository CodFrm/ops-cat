package app

import (
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
	"github.com/opskat/opskat/internal/service/ai_provider_svc"
)

// AIProviderInfo 返回给前端的 Provider 信息
type AIProviderInfo struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	APIBase          string `json:"apiBase"`
	APIKey           string `json:"apiKey"`
	MaskedAPIKey     string `json:"maskedApiKey"`
	Model            string `json:"model"`
	MaxOutputTokens  int    `json:"maxOutputTokens"`
	ContextWindow    int    `json:"contextWindow"`
	ReasoningEnabled bool   `json:"reasoningEnabled"`
	ReasoningEffort  string `json:"reasoningEffort"`
	IsActive         bool   `json:"isActive"`
}

func toProviderInfo(p *ai_provider_entity.AIProvider, apiKey string) AIProviderInfo {
	enabled, effort := normalizeProviderReasoningConfig(p.Type, p.ReasoningEnabled, p.ReasoningEffort)
	return AIProviderInfo{
		ID:               p.ID,
		Name:             p.Name,
		Type:             p.Type,
		APIBase:          p.APIBase,
		APIKey:           apiKey,
		MaskedAPIKey:     maskAPIKey(apiKey),
		Model:            p.Model,
		MaxOutputTokens:  p.MaxOutputTokens,
		ContextWindow:    p.ContextWindow,
		ReasoningEnabled: enabled,
		ReasoningEffort:  effort,
		IsActive:         p.IsActive,
	}
}

func normalizeProviderReasoningConfig(providerType string, reasoningEnabled bool, reasoningEffort string) (bool, string) {
	switch providerType {
	case "openai", "anthropic":
	default:
		return false, ""
	}
	effort := strings.ToLower(strings.TrimSpace(reasoningEffort))
	// max 仅 Anthropic 暴露；OpenAI 收到 max 视为非法，落回 medium
	if effort == "max" && providerType != "anthropic" {
		effort = "medium"
	}
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return true, effort
	case "none":
		return false, ""
	default:
		// 老 OpenAI 数据：toggle=true 但 effort 空 → medium。
		// Anthropic legacy 兜底由 migration 202605070001 一次性处理，这里无需特例。
		if reasoningEnabled {
			return true, "medium"
		}
		return false, ""
	}
}

// ListAIProviders 列出所有 Provider
func (a *App) ListAIProviders() ([]AIProviderInfo, error) {
	list, err := ai_provider_svc.AIProvider().List(a.langCtx())
	if err != nil {
		return nil, err
	}
	result := make([]AIProviderInfo, 0, len(list))
	for _, p := range list {
		decrypted, _ := ai_provider_svc.AIProvider().DecryptAPIKey(p)
		result = append(result, toProviderInfo(p, decrypted))
	}
	return result, nil
}

// GetActiveAIProvider 获取当前激活的 Provider
func (a *App) GetActiveAIProvider() (*AIProviderInfo, error) {
	p, err := ai_provider_svc.AIProvider().GetActive(a.langCtx())
	if err != nil {
		return nil, nil //nolint:nilerr // 无激活 provider 时返回 nil 表示未配置
	}
	decrypted, _ := ai_provider_svc.AIProvider().DecryptAPIKey(p)
	info := toProviderInfo(p, decrypted)
	return &info, nil
}

// CreateAIProvider 创建新 Provider
func (a *App) CreateAIProvider(name, providerType, apiBase, apiKey, model string, maxOutputTokens, contextWindow int, reasoningEnabled bool, reasoningEffort string) (*AIProviderInfo, error) {
	reasoningEnabled, reasoningEffort = normalizeProviderReasoningConfig(providerType, reasoningEnabled, reasoningEffort)
	p := &ai_provider_entity.AIProvider{
		Name:             name,
		Type:             providerType,
		APIBase:          apiBase,
		Model:            model,
		MaxOutputTokens:  maxOutputTokens,
		ContextWindow:    contextWindow,
		ReasoningEnabled: reasoningEnabled,
		ReasoningEffort:  reasoningEffort,
	}
	if err := ai_provider_svc.AIProvider().Create(a.langCtx(), p, apiKey); err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}
	info := toProviderInfo(p, apiKey)
	return &info, nil
}

// UpdateAIProvider 更新 Provider
func (a *App) UpdateAIProvider(id int64, name, providerType, apiBase, apiKey, model string, maxOutputTokens, contextWindow int, reasoningEnabled bool, reasoningEffort string) error {
	p, err := ai_provider_svc.AIProvider().Get(a.langCtx(), id)
	if err != nil {
		return fmt.Errorf("provider 不存在: %w", err)
	}
	reasoningEnabled, reasoningEffort = normalizeProviderReasoningConfig(providerType, reasoningEnabled, reasoningEffort)
	p.Name = name
	p.Type = providerType
	p.APIBase = apiBase
	p.Model = model
	p.MaxOutputTokens = maxOutputTokens
	p.ContextWindow = contextWindow
	p.ReasoningEnabled = reasoningEnabled
	p.ReasoningEffort = reasoningEffort
	if err := ai_provider_svc.AIProvider().Update(a.langCtx(), p, apiKey); err != nil {
		return fmt.Errorf("更新 Provider 失败: %w", err)
	}

	// 如果更新的是激活的 Provider，重新加载
	if p.IsActive {
		return a.activateProvider(p)
	}
	return nil
}

// DeleteAIProvider 删除 Provider
func (a *App) DeleteAIProvider(id int64) error {
	p, err := ai_provider_svc.AIProvider().Get(a.langCtx(), id)
	if err != nil {
		return fmt.Errorf("provider 不存在: %w", err)
	}
	if p.IsActive {
		a.aiProvider = nil
		a.aiModel = ""
		if a.mgr != nil {
			_ = a.mgr.Close()
			a.mgr = nil
		}
	}
	return ai_provider_svc.AIProvider().Delete(a.langCtx(), id)
}

// SetActiveAIProvider 切换激活 Provider 并创建 Agent
func (a *App) SetActiveAIProvider(id int64) error {
	if err := ai_provider_svc.AIProvider().SetActive(a.langCtx(), id); err != nil {
		return fmt.Errorf("激活 Provider 失败: %w", err)
	}
	p, err := ai_provider_svc.AIProvider().Get(a.langCtx(), id)
	if err != nil {
		return err
	}
	return a.activateProvider(p)
}

// AIModelInfo 模型信息
type AIModelInfo struct {
	ID              string `json:"id"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
	ContextWindow   int    `json:"contextWindow"`
}

// FetchAIModels 从 API 获取可用模型列表
func (a *App) FetchAIModels(providerType, apiBase, apiKey string) ([]AIModelInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API Key 不能为空")
	}

	models, err := ai.FetchModels(providerType, apiBase, apiKey)
	if err != nil {
		return nil, fmt.Errorf("获取模型列表失败: %w", err)
	}

	result := make([]AIModelInfo, 0, len(models))
	for _, m := range models {
		info := AIModelInfo{ID: m.ID}
		if d := ai.GetModelDefaults(m.ID); d != nil {
			info.MaxOutputTokens = d.MaxOutputTokens
			info.ContextWindow = d.ContextWindow
		} else {
			info.MaxOutputTokens = ai.FallbackMaxOutputTokens
			info.ContextWindow = ai.FallbackContextWindow
		}
		result = append(result, info)
	}
	return result, nil
}

// GetModelDefaults 获取模型的默认参数，未知模型返回 fallback 默认值
func (a *App) GetModelDefaults(model string) *AIModelInfo {
	d := ai.GetModelDefaults(model)
	if d != nil {
		return &AIModelInfo{
			ID:              model,
			MaxOutputTokens: d.MaxOutputTokens,
			ContextWindow:   d.ContextWindow,
		}
	}
	return &AIModelInfo{
		ID:              model,
		MaxOutputTokens: ai.FallbackMaxOutputTokens,
		ContextWindow:   ai.FallbackContextWindow,
	}
}
