package app

import (
	"context"

	"github.com/opskat/opskat/internal/service/snippet_svc"
	"github.com/opskat/opskat/pkg/extension"
)

// collectExtensionCategories 遍历扩展管理器当前已加载扩展，收集其 manifest 声明的 snippet 分类。
// 返回结果喂给 CategoryRegistry 的 provider。
func collectExtensionCategories(mgr *extension.Manager) []snippet_svc.ExtensionCategory {
	if mgr == nil {
		return nil
	}
	var out []snippet_svc.ExtensionCategory
	for _, ext := range mgr.ListExtensions() {
		if ext == nil || ext.Manifest == nil {
			continue
		}
		for _, c := range ext.Manifest.Snippets.Categories {
			label := c.ID
			if c.I18n.Name != "" {
				label = c.I18n.Name
			}
			out = append(out, snippet_svc.ExtensionCategory{
				ID:           c.ID,
				AssetType:    c.AssetType,
				Label:        label,
				ExtensionRef: ext.Name,
			})
		}
	}
	return out
}

// snippetExtensionHook 代理 extension_svc 的 SnippetExtensionHook 到全局 snippet_svc 单例。
// 单独为代理对象的原因：snippet_svc 单例在 bootstrap.Init 中注册；App 构造时可能尚未就绪，
// 延迟查表可避免启动顺序耦合。
type snippetExtensionHook struct{}

// SyncExtensionSeeds 实现 extension_svc.SnippetExtensionHook
func (snippetExtensionHook) SyncExtensionSeeds(ctx context.Context, extName string, seeds []snippet_svc.SeedDef) error {
	svc := snippet_svc.Snippet()
	if svc == nil {
		return nil
	}
	return svc.SyncExtensionSeeds(ctx, extName, seeds)
}

// RemoveExtensionSeeds 实现 extension_svc.SnippetExtensionHook
func (snippetExtensionHook) RemoveExtensionSeeds(ctx context.Context, extName string) error {
	svc := snippet_svc.Snippet()
	if svc == nil {
		return nil
	}
	return svc.RemoveExtensionSeeds(ctx, extName)
}

// RefreshCategories 实现 extension_svc.SnippetExtensionHook
func (snippetExtensionHook) RefreshCategories() {
	svc := snippet_svc.Snippet()
	if svc == nil {
		return
	}
	svc.RefreshCategories()
}

// KnownCategoryIDs 实现 extension_svc.SnippetExtensionHook
func (snippetExtensionHook) KnownCategoryIDs() []string {
	svc := snippet_svc.Snippet()
	if svc == nil {
		return nil
	}
	return svc.KnownCategoryIDs()
}
