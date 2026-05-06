package snippet_entity

import (
	"errors"
	"strings"
	"time"
)

// 片段分类常量
const (
	CategoryShell  = "shell"
	CategorySQL    = "sql"
	CategoryRedis  = "redis"
	CategoryMongo  = "mongo"
	CategoryK8s    = "k8s"
	CategoryPrompt = "prompt"
)

// 片段来源常量
const (
	// SourceUser 用户自创建片段
	SourceUser = "user"
	// SourceExtPrefix 扩展来源前缀，完整 source 形如 "ext:<extension-name>"
	SourceExtPrefix = "ext:"
)

// 状态常量（遵循仓库软删除约定）
const (
	StatusActive  int8 = 1
	StatusDeleted int8 = 2
)

// AllCategories 返回所有内置片段分类的 ID 列表
func AllCategories() []string {
	return []string{CategoryShell, CategorySQL, CategoryRedis, CategoryMongo, CategoryK8s, CategoryPrompt}
}

// IsValidCategory 判断分类是否为内置合法分类
func IsValidCategory(s string) bool {
	switch s {
	case CategoryShell, CategorySQL, CategoryRedis, CategoryMongo, CategoryK8s, CategoryPrompt:
		return true
	}
	return false
}

// CategoryAssetType 返回分类绑定的资产类型（asset_entity.AssetType*）
// prompt 不绑定资产；未知分类返回空字符串。
func CategoryAssetType(cat string) string {
	switch cat {
	case CategoryShell:
		return "ssh"
	case CategorySQL:
		return "database"
	case CategoryRedis:
		return "redis"
	case CategoryMongo:
		return "mongodb"
	case CategoryK8s:
		return "k8s"
	case CategoryPrompt:
		return ""
	}
	return ""
}

// Snippet 片段实体
type Snippet struct {
	ID           int64      `gorm:"primaryKey"`
	Name         string     `gorm:"size:128;not null;index"`
	Category     string     `gorm:"size:32;not null;index"`
	Content      string     `gorm:"type:text;not null"`
	Description  string     `gorm:"size:512;not null;default:''"`
	LastAssetIDs string     `gorm:"size:1024;not null;default:''"` // comma-separated int64; UI-authoritative
	Source       string     `gorm:"size:64;not null;default:user;index"`
	SourceRef    string     `gorm:"size:128;not null;default:''"`
	UseCount     uint       `gorm:"not null;default:0"`
	LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	Status       int8       `gorm:"not null;default:1;index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName GORM 表名
func (Snippet) TableName() string { return "snippets" }

// Validate 校验必填字段。
//
// 注意：category 仅做"非空"校验；"是否为已注册分类"由 service 层查询 CategoryRegistry
// 决定（注册表包含内置 5 类 + 运行时已加载扩展声明的分类）。entity 层若直接判定会
// 拒绝掉扩展分类，破坏 PR 5 的扩展集成。
func (s *Snippet) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("snippet name is required")
	}
	if strings.TrimSpace(s.Category) == "" {
		return errors.New("snippet category is required")
	}
	if strings.TrimSpace(s.Content) == "" {
		return errors.New("snippet content is required")
	}
	if s.Source == "" {
		return errors.New("snippet source is required")
	}
	return nil
}

// IsReadOnly 扩展来源的片段不可由用户编辑
func (s *Snippet) IsReadOnly() bool {
	return strings.HasPrefix(s.Source, SourceExtPrefix)
}
