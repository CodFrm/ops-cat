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

// MentionedAsset 用户本次消息引用的资产（对应前端 @ 提及）
type MentionedAsset struct {
	AssetID   int64  `json:"assetId"`
	Name      string `json:"name"`
	Type      string `json:"type"` // ssh/mysql/redis/mongo/...
	Host      string `json:"host"`
	GroupPath string `json:"groupPath"` // 完整路径 "生产/数据库"，无分组时为空
}

// AIContext 前端传入的上下文信息
type AIContext struct {
	OpenTabs        []TabInfo        `json:"openTabs"`
	MentionedAssets []MentionedAsset `json:"mentionedAssets"`
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

// Build 构建完整的 System Prompt
func (b *PromptBuilder) Build() string {
	var parts []string

	// 1. 基础角色描述
	parts = append(parts, b.buildRoleDescription())

	// 2. 用户语言
	parts = append(parts, b.buildLanguageHint())

	// 3. 当前 Tab 上下文
	if tabContext := b.buildTabContext(); tabContext != "" {
		parts = append(parts, tabContext)
	}

	// 3.5. 本次消息引用的资产
	if mentionContext := b.buildMentionContext(); mentionContext != "" {
		parts = append(parts, mentionContext)
	}

	// 4. 资产知识引导
	parts = append(parts, b.buildKnowledgeGuidance())

	// 5. 错误恢复引导
	parts = append(parts, b.buildErrorRecoveryGuidance())

	// 6. 用户拒绝操作引导
	parts = append(parts, b.buildUserDenialGuidance())

	// 7. Extension tools guide
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

func (b *PromptBuilder) buildRoleDescription() string {
	return `You are the OpsKat AI assistant, a powerful IT operations agent. You can:
- List, view, add, and update remote server assets (SSH, databases, Redis, Kubernetes)
- Execute commands on SSH servers
- Execute SQL queries on databases (MySQL, PostgreSQL)
- Execute Redis commands
- Upload and download files via SFTP
- Request command execution permissions from the user
- Spawn sub-agents for complex multi-step tasks
- Execute batch commands across multiple assets simultaneously

You are proactive, thorough, and safety-conscious. Always verify before destructive operations.`
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

func (b *PromptBuilder) buildMentionContext() string {
	return RenderMentionContext(b.context.MentionedAssets)
}

// RenderMentionContext 渲染一组被 @ 提及的资产为 prompt 片段。
// 供 PromptBuilder 与 QueueAIMessage（生成过程中追加消息）复用。
func RenderMentionContext(mentions []MentionedAsset) string {
	if len(mentions) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, "# Assets referenced in the user's message")
	for _, a := range mentions {
		segs := []string{
			fmt.Sprintf("ID=%d", a.AssetID),
			fmt.Sprintf("type=%s", a.Type),
			fmt.Sprintf("host=%s", a.Host),
		}
		if a.GroupPath != "" {
			segs = append(segs, fmt.Sprintf("group=%s", a.GroupPath))
		}
		lines = append(lines, fmt.Sprintf("- @%s (%s)", a.Name, strings.Join(segs, ", ")))
	}
	lines = append(lines, "")
	lines = append(lines, "When the user refers to an asset by @name, use the information above — do not guess by name alone.")
	return strings.Join(lines, "\n")
}
