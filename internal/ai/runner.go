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
	"github.com/cago-frame/agents/tool/subagent"
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
//   - ProviderEntity 开启 reasoning 时走 coding.WithThinking；
//   - 非空 Tools 走 coding.WithExtraTools；
//   - LocalToolGate 非 nil 时为 bash/write/edit 加审批 middleware。注册顺序：
//     audit 在前（外层），gate 在后（内层）—— 这样 gate AbortWithDeny 后 audit
//     的 c.Next() 返回时 c.Output 已是 deny block，照样落审计。
//   - cago 的 subagent dispatch 工具不会把父 middleware 透传给 child agent。
//     Explore/Plan 工具集只读，无 bash/write/edit 路径；GeneralPurpose 含全套
//     coding 工具，因此显式替换默认 GP，把 audit + LocalToolGate 中间件挂上。
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
	if cfg.ProviderEntity != nil && cfg.ProviderEntity.ReasoningEnabled && cfg.ProviderEntity.ReasoningEffort != "" {
		opts = append(opts, coding.WithThinking(&cagoProvider.ThinkingConfig{
			Effort: cagoProvider.ThinkingEffort(cfg.ProviderEntity.ReasoningEffort),
		}))
	}
	if len(cfg.Tools) > 0 {
		opts = append(opts, coding.WithExtraTools(cfg.Tools...))
	}
	if cfg.LocalToolGate != nil {
		opts = append(opts, coding.WithAgentOpts(
			agent.Use(`^(bash|write|edit)$`, cfg.LocalToolGate.Middleware()),
		))
	}
	opts = append(opts, coding.WithExtraSubagents(
		buildGeneralPurposeEntry(prov, cfg.Cwd, cfg.LocalToolGate),
	))
	return coding.New(ctx, prov, cfg.Cwd, opts...)
}

// buildGeneralPurposeEntry 构造一个替换 coding 默认 GeneralPurpose 子 agent 的 Entry，
// 把父 agent 同款 middleware 显式注入到 child：
//   - auditMiddleware 无条件挂，保证子代理触发的 bash/write/edit 也落审计；
//   - LocalToolGate 非 nil 时挂 bash|write|edit 审批 gate，与父保持同一份白名单
//     （以 conversationID 索引）—— 用户在父 agent 里 allowAll 过的 pattern，
//     子 agent 调同样命令时复用，符合直觉。
func buildGeneralPurposeEntry(prov cagoProvider.Provider, cwd string, gate *LocalToolGate) subagent.Entry {
	subOpts := []coding.SubagentOption{
		coding.SubagentWithAgentOpts(agent.Use(".*", auditMiddleware)),
	}
	if gate != nil {
		subOpts = append(subOpts, coding.SubagentWithAgentOpts(
			agent.Use(`^(bash|write|edit)$`, gate.Middleware()),
		))
	}
	return coding.GeneralPurpose(prov, cwd, subOpts...)
}
