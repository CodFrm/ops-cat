package ai

import (
	"context"
	"fmt"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/app/coding"
	cagoProvider "github.com/cago-frame/agents/provider"
	cagoAnthropics "github.com/cago-frame/agents/provider/anthropics"
	cagoOpenAI "github.com/cago-frame/agents/provider/openai"
	"github.com/cago-frame/agents/tool"
	"github.com/sashabaranov/go-openai"

	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
)

// BuildProvider 根据 AIProvider entity + 已解密 API Key 构造 cago provider.Provider。
// 后续对话全部走 cago provider；model / max_tokens / reasoning 在 request 阶段注入，
// 这里只负责"传输层"。
func BuildProvider(p *ai_provider_entity.AIProvider, apiKey string) (cagoProvider.Provider, error) {
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

// SystemConfig 组装一个 coding.System 需要的所有依赖。
// 调用方持有 *coding.System + *agent.Runner，自己处理 Send / Cancel / Steer / Close。
//
//   - Provider：可选；测试场景用 providertest.Mock 注入。非 nil 时优先使用。
//   - ProviderEntity / APIKey：生产路径——传给 BuildProvider 构造 cago provider。
//   - Cwd：coding.New 必填参数（read/write/bash 等工具的工作目录），通常为 ~/.opskat。
//   - SystemPrompt：动态系统提示词，附加到 coding 默认 system 之后。
//   - Model：覆盖 provider 默认模型。
//   - Tools：额外注册到父 agent 的工具集，通常是 Tools()。
//   - LocalToolGate：可选，非 nil 时为 bash/write/edit 挂 PreToolUseHook 走用户审批。
type SystemConfig struct {
	Provider       cagoProvider.Provider
	ProviderEntity *ai_provider_entity.AIProvider
	APIKey         string
	Cwd            string
	SystemPrompt   string
	Model          string
	Tools          []tool.Tool
	LocalToolGate  *LocalToolGate
}

// BuildSystem 拼装 coding.System：
//   - 关掉 ~/.claude 自动加载 / skills / slash commands；
//   - 用 OpsKat 自定义 system 模板；
//   - 注册 auditMiddleware（around 模式：跑前挂 *CheckResult slot，跑后落审计）；
//   - 非空 SystemPrompt 走 coding.AppendSystem 注入；
//   - 非空 Model 走 coding.WithModel；
//   - 非空 Tools 走 coding.WithExtraTools；
//   - LocalToolGate 非 nil 时为 bash/write/edit 加审批 middleware。注册顺序：
//     audit 在前（外层），gate 在后（内层）—— 这样 gate AbortWithDeny 后 audit
//     的 c.Next() 返回时 c.Output 已是 deny block，照样落审计。
//
// 注：cago 的 dispatch_subagent 不继承父 middleware —— 子代理若调用 bash/write/edit 暂不被拦截，
// 后续如需收紧再用 coding.WithExtraSubagents + SubagentWithAgentOpts 一并接线。
func BuildSystem(ctx context.Context, cfg SystemConfig) (*coding.System, error) {
	prov := cfg.Provider
	if prov == nil {
		built, err := BuildProvider(cfg.ProviderEntity, cfg.APIKey)
		if err != nil {
			return nil, err
		}
		prov = built
	}

	opts := []coding.Option{
		coding.WithoutContextFiles(),
		coding.WithoutSkills(),
		coding.WithoutSlashCommands(),
		coding.WithSystemTemplate(opskatSystemTemplate),
		coding.WithAgentOpts(agent.Use(".*", auditMiddleware)),
	}
	if cfg.SystemPrompt != "" {
		opts = append(opts, coding.AppendSystem(cfg.SystemPrompt))
	}
	if cfg.Model != "" {
		opts = append(opts, coding.WithModel(cfg.Model))
	}
	if len(cfg.Tools) > 0 {
		opts = append(opts, coding.WithExtraTools(cfg.Tools...))
	}
	if cfg.LocalToolGate != nil {
		opts = append(opts, coding.WithAgentOpts(
			agent.Use(`^(bash|write|edit)$`, cfg.LocalToolGate.Middleware()),
		))
	}
	return coding.New(ctx, prov, cfg.Cwd, opts...)
}
