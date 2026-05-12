package ai

import (
	"fmt"
	"sort"
	"strings"
)

// TabInfo 当前打开的 Tab 信息
type TabInfo struct {
	Type      string `json:"type"` // "ssh" | "database" | "redis" | "sftp"
	AssetID   int64  `json:"assetId"`
	AssetName string `json:"assetName"`
}

// AIContext 前端传入的上下文信息
type AIContext struct {
	OpenTabs []TabInfo `json:"openTabs"`
}

// PromptBuilder 动态构建 System Prompt
type PromptBuilder struct {
	language          string
	context           AIContext
	extensionSkillMDs map[string]string // extName → SKILL.md content
}

// SetExtensionSkillMDs sets all extension SKILL.md contents to inject.
// Keys are extension names, values are the raw markdown.
func (b *PromptBuilder) SetExtensionSkillMDs(mds map[string]string) {
	b.extensionSkillMDs = mds
}

// NewPromptBuilder 创建 PromptBuilder
func NewPromptBuilder(language string, context AIContext) *PromptBuilder {
	return &PromptBuilder{
		language: language,
		context:  context,
	}
}

// Build 构建运行时上下文 prompt，注入到 cago 模板的 {{.AppendSystem}} 位。
// 角色身份已经搬到 internal/ai/system_template.go 的模板 intro，此处只输出
// 每次 Send 都可能变化的动态段（语言 / Tab / 知识 / 错误恢复 / extension SKILL.md）。
func (b *PromptBuilder) Build() string {
	var parts []string

	// 1. 用户语言
	parts = append(parts, b.buildLanguageHint())

	// 2. 当前 Tab 上下文
	if tabContext := b.buildTabContext(); tabContext != "" {
		parts = append(parts, tabContext)
	}

	// 3. 资产知识引导
	parts = append(parts, b.buildKnowledgeGuidance())

	// 4. 多资产 / 批量操作引导
	parts = append(parts, b.buildMultiAssetGuidance())

	// 5. 凭据与敏感信息引导
	parts = append(parts, b.buildSecretsGuidance())

	// 6. 错误恢复引导
	parts = append(parts, b.buildErrorRecoveryGuidance())

	// 7. 用户拒绝操作引导
	parts = append(parts, b.buildUserDenialGuidance())

	// 8. Extension tools guide
	if len(b.extensionSkillMDs) > 0 {
		names := make([]string, 0, len(b.extensionSkillMDs))
		for name := range b.extensionSkillMDs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("## From extension: %s\n%s", name, b.extensionSkillMDs[name]))
		}
	}

	return strings.Join(parts, "\n\n")
}

func (b *PromptBuilder) buildLanguageHint() string {
	switch b.language {
	case "zh-cn":
		return "The user's preferred language is Chinese (Simplified). Always respond in Chinese."
	case "en":
		return "The user's preferred language is English. Always respond in English."
	default:
		return "Respond in the same language the user uses."
	}
}

func (b *PromptBuilder) buildTabContext() string {
	if len(b.context.OpenTabs) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "The user currently has these tabs open (this helps you understand what they're working on):")
	for _, tab := range b.context.OpenTabs {
		typeName := tab.Type
		switch tab.Type {
		case "ssh":
			typeName = "SSH Terminal"
		case "database":
			typeName = "Database Query"
		case "redis":
			typeName = "Redis"
		case "sftp":
			typeName = "SFTP"
		}
		lines = append(lines, fmt.Sprintf("- %s: \"%s\" (ID: %d)", typeName, tab.AssetName, tab.AssetID))
	}
	return strings.Join(lines, "\n")
}

func (b *PromptBuilder) buildKnowledgeGuidance() string {
	return `Discover before acting: call list_assets / get_asset first, then operate. The asset Description often contains prior findings (OS, services, DB version) — read it to avoid redundant exploration. When you learn new non-secret facts about an asset during work, append them to the asset Description via update_asset.

Pick the dedicated tool for each asset type: exec_sql for databases, exec_redis for Redis, exec_mongo for MongoDB, exec_k8s for kubectl (do not invoke kubectl through run_command), kafka_* for Kafka. Use run_command only for plain SSH shell commands.

Local vs remote: the local tools (bash / write / edit) run on the user's own machine — never use them to act on a remote asset. When the scenario targets a specific server / database / Redis / Kafka / K8s asset (an SSH / Database / Redis / SFTP tab is open for it, or the user names the asset, or the request is clearly about that asset), use that asset's dedicated remote tool (run_command for SSH, exec_sql / exec_redis / exec_mongo / exec_k8s / kafka_* / sftp_*). Do not fall back to local bash even when the command looks identical — running ` + "`df -h`" + ` locally tells the user nothing about the server they asked about.`
}

func (b *PromptBuilder) buildMultiAssetGuidance() string {
	return `When the same operation targets 2 or more assets, prefer batch_command over a loop of run_command / exec_sql / exec_redis — it parallelizes execution and batches approval prompts. When you expect to issue several command patterns that will trigger approval, call request_permission upfront so the user grants them in a single review instead of one popup per call.`
}

func (b *PromptBuilder) buildSecretsGuidance() string {
	return `Never echo passwords, private keys, kubeconfig contents, or other credentials back to the user. The app stores them encrypted; treat anything that came from a password / private_key / kubeconfig field as write-only. If a tool result includes a secret, mask it before referencing it.`
}

func (b *PromptBuilder) buildErrorRecoveryGuidance() string {
	return `When a tool execution fails, analyze the error and try a different approach. If repeated attempts fail, explain the issue to the user and suggest alternatives. Do not give up after a single failure.`
}

func (b *PromptBuilder) buildUserDenialGuidance() string {
	return `IMPORTANT: When the user denies a command execution or permission request, you MUST immediately stop the current task. Do not attempt alternative commands, workarounds, or different approaches to achieve the same goal. Simply acknowledge the user's decision and ask if they need anything else. The user's denial is final and must be respected.`
}
