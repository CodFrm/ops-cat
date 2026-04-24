package app

import (
	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
	"github.com/opskat/opskat/internal/service/snippet_svc"
)

// ListSnippetCategories 返回所有片段分类（内置 + 扩展提供）
func (a *App) ListSnippetCategories() []snippet_svc.Category {
	return snippet_svc.Snippet().ListCategories()
}

// ListSnippets 查询片段列表
func (a *App) ListSnippets(req snippet_svc.ListReq) ([]*snippet_entity.Snippet, error) {
	return snippet_svc.Snippet().List(a.langCtx(), req)
}

// GetSnippet 获取单个片段
func (a *App) GetSnippet(id int64) (*snippet_entity.Snippet, error) {
	return snippet_svc.Snippet().Get(a.langCtx(), id)
}

// CreateSnippet 创建片段
func (a *App) CreateSnippet(req snippet_svc.CreateReq) (*snippet_entity.Snippet, error) {
	return snippet_svc.Snippet().Create(a.langCtx(), req)
}

// UpdateSnippet 更新片段
func (a *App) UpdateSnippet(req snippet_svc.UpdateReq) (*snippet_entity.Snippet, error) {
	return snippet_svc.Snippet().Update(a.langCtx(), req)
}

// DeleteSnippet 软删除片段
func (a *App) DeleteSnippet(id int64) error {
	return snippet_svc.Snippet().Delete(a.langCtx(), id)
}

// DuplicateSnippet 复制片段
func (a *App) DuplicateSnippet(id int64) (*snippet_entity.Snippet, error) {
	return snippet_svc.Snippet().Duplicate(a.langCtx(), id)
}

// RecordSnippetUse 记录片段使用，原子更新 use_count / last_used_at
func (a *App) RecordSnippetUse(id int64) error {
	return snippet_svc.Snippet().RecordUse(a.langCtx(), id)
}

// SetSnippetLastAssets records the asset IDs most recently used to run a snippet.
func (a *App) SetSnippetLastAssets(id int64, assetIDs []int64) error {
	return snippet_svc.Snippet().SetLastAssets(a.langCtx(), id, assetIDs)
}

// GetSnippetLastAssets returns the (live-filtered) asset IDs last used to run a snippet.
func (a *App) GetSnippetLastAssets(id int64) ([]int64, error) {
	return snippet_svc.Snippet().GetLastAssets(a.langCtx(), id)
}
