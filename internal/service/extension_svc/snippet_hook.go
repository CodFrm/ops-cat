package extension_svc

import (
	"context"

	"github.com/opskat/opskat/internal/service/snippet_svc"
)

// SnippetExtensionHook 扩展生命周期与 snippet_svc 的边界接口。
// 由 bootstrap 注入 snippet_svc.Snippet() 的默认实现；参数为 nil 时 extension_svc 跳过所有调用。
type SnippetExtensionHook interface {
	// SyncExtensionSeeds 幂等同步扩展 seed。
	SyncExtensionSeeds(ctx context.Context, extName string, seeds []snippet_svc.SeedDef) error
	// RemoveExtensionSeeds 清除扩展 seed（源：ext:<extName>）。用户自建片段保留。
	RemoveExtensionSeeds(ctx context.Context, extName string) error
	// RefreshCategories 重建 snippet 分类表。
	RefreshCategories()
	// KnownCategoryIDs 当前注册表已知分类 ID（用于跨扩展冲突检测）。
	KnownCategoryIDs() []string
}
