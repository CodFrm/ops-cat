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

// SetExtensionSkillMD sets a single extension SKILL.md content.
// Kept for backward compatibility — treats it as a single-entry map.
func (b *PromptBuilder) SetExtensionSkillMD(md string) {
	if md == "" {
		b.extensionSkillMDs = nil
		return
	}
	b.extensionSkillMDs = map[string]string{"extension": md}
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

	// 4. 错误恢复引导
	parts = append(parts, b.buildErrorRecoveryGuidance())

	// 5. 用户拒绝操作引导
	parts = append(parts, b.buildUserDenialGuidance())

	// 6. Extension tools guide
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
	return `When you discover valuable information about an asset during operations (OS version, running services, hardware specs, database version, etc.), proactively call update_asset to append these findings to the asset's Description field. When reading asset info, check the Description for existing knowledge to avoid redundant exploration.

For k8s assets, call get_asset before operating the cluster so you can inspect namespace, context, and ssh_tunnel_id. Prefer exec_k8s for kubectl work. exec_k8s will automatically use the SSH jump host when ssh_tunnel_id is set, and otherwise run kubectl locally with the asset kubeconfig. Do not use run_command or a generic local shell for kubectl when exec_k8s is available.`
}

func (b *PromptBuilder) buildErrorRecoveryGuidance() string {
	return `When a tool execution fails, analyze the error, and try a different approach. If repeated attempts fail, explain the issue to the user and suggest alternatives. Do not give up after a single failure.`
}

func (b *PromptBuilder) buildUserDenialGuidance() string {
	return `IMPORTANT: When the user denies a command execution or permission request, you MUST immediately stop the current task. Do not attempt alternative commands, workarounds, or different approaches to achieve the same goal. Simply acknowledge the user's decision and ask if they need anything else. The user's denial is final and must be respected.`
}
