package ai

// ApprovalItem 统一审批项，AI 和 opsctl 共用
type ApprovalItem struct {
	Type      string `json:"type"` // "exec", "k8s", "sql", "redis", "cp", "grant"
	AssetID   int64  `json:"asset_id"`
	AssetName string `json:"asset_name"`
	GroupID   int64  `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	Command   string `json:"command"`
	Detail    string `json:"detail,omitempty"`
}

// ApprovalResponse 统一审批响应
type ApprovalResponse struct {
	Decision    string         `json:"decision"`               // "allow", "deny", "allowAll"
	EditedItems []ApprovalItem `json:"edited_items,omitempty"` // grant: 用户可能编辑了 items
}
