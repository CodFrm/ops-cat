package ai

import (
	"fmt"
	"strings"
)

// 此文件保留与前端共享的上下文类型 + RenderMentionContext。
// 旧的 PromptBuilder 在 cago 接管后已删除（系统提示由 aiagent.StaticSystemPrompt
// + 各 hook 注入构建）；MentionedAsset 渲染仍由 App.QueueAIMessage 复用。

// TabInfo 当前打开的 Tab 信息
type TabInfo struct {
	Type      string `json:"type"` // "ssh" | "database" | "redis" | "sftp"
	AssetID   int64  `json:"assetId"`
	AssetName string `json:"assetName"`
}

// MentionedAsset 用户本次消息引用的资产（对应前端 @ 提及）
//
// Start/End 是 @-mention 在原 prompt 字符串中的字符偏移（JS 字符串索引）。前端
// AIChatInput 提交时算出，gormStore 旁路落库到 conversation_messages.mentions JSON，
// 刷新后 UserMessage.buildSegments 据此切片渲染 chip——丢了就会渲染成空 button。
type MentionedAsset struct {
	AssetID   int64  `json:"assetId"`
	Name      string `json:"name"`
	Type      string `json:"type"` // ssh/mysql/redis/mongo/...
	Host      string `json:"host"`
	GroupPath string `json:"groupPath"` // 完整路径 "生产/数据库"，无分组时为空
	Start     int    `json:"start"`     // content 中字符起始索引（含 @ 符号）
	End       int    `json:"end"`       // 结束索引（不含）
}

// AIContext 前端传入的上下文信息
type AIContext struct {
	OpenTabs        []TabInfo        `json:"openTabs"`
	MentionedAssets []MentionedAsset `json:"mentionedAssets"`
}

// RenderMentionContext 渲染一组被 @ 提及的资产为 prompt 片段。
// 由 aiagent UserPromptSubmit hook 与 App.QueueAIMessage 复用。
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
