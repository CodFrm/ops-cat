package ai

// 此文件保留与前端共享的上下文类型。旧的 PromptBuilder 在 cago 接管后已删除
// （系统提示由 aiagent.StaticSystemPrompt + 各 hook 注入构建）。
// Mention 注入：以前用 RenderMentionContext 拼 markdown header prepend 到 prompt + 旁路
// stash 落库；现在改成 ai.WrapMentions 在 LLM body 里 inline <mention> XML，
// gormStore.AppendMessage 反向 ai.ParseMentions 写 row.Mentions——元数据与正文原子绑定。

// TabInfo 当前打开的 Tab 信息
type TabInfo struct {
	Type      string `json:"type"` // "ssh" | "database" | "redis" | "sftp"
	AssetID   int64  `json:"assetId"`
	AssetName string `json:"assetName"`
}

// MentionedAsset 用户本次消息引用的资产（对应前端 @ 提及）
//
// Start/End 是 @-mention 在原 prompt 字符串中的 JS UTF-16 偏移。前端 AIChatInput
// 提交时算出，ai.WrapMentions 包成 inline <mention> XML 写到 LLM body，
// gormStore.AppendMessage 反向 ai.ParseMentions 拿回来落到 conversation_messages.mentions
// JSON 列。刷新后 UserMessage.buildSegments 据此切片渲染 chip——丢了就会渲染成空 button。
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
