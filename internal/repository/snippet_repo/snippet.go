package snippet_repo

//go:generate mockgen -source=snippet.go -destination=mock_snippet_repo/snippet.go -package=mock_snippet_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
)

// SnippetQuery 查询参数
type SnippetQuery struct {
	Categories []string // 空表示不过滤
	Keyword    string   // 对 name / description / content 做 LIKE
	Sources    []string // 空表示不过滤；可选 "user" / "ext:foo"
	Limit      int      // 0 表示不限制
	Offset     int
	OrderBy    string // "use_count_desc" | "updated_at_desc" | ""（默认：last_used_at DESC NULLS LAST, updated_at DESC）
}

// SnippetRepo 数据访问接口
type SnippetRepo interface {
	Create(ctx context.Context, s *snippet_entity.Snippet) error
	Update(ctx context.Context, s *snippet_entity.Snippet) error
	SoftDelete(ctx context.Context, id int64) error
	HardDeleteBySource(ctx context.Context, source string) error
	GetByID(ctx context.Context, id int64) (*snippet_entity.Snippet, error)
	Find(ctx context.Context, q SnippetQuery) ([]*snippet_entity.Snippet, error)
	FindBySourceRef(ctx context.Context, source, ref string) (*snippet_entity.Snippet, error)
	TouchUsage(ctx context.Context, id int64) error
	// SetLastAssets overwrites last_asset_ids for an active snippet. No dedupe,
	// no reorder — UI is authoritative. Service caps length before calling.
	SetLastAssets(ctx context.Context, id int64, assetIDs []int64) error

	// UpsertExtensionSeed 以 (source, source_ref) 为联合键幂等写入扩展 seed。
	// 命中已有行则覆盖 name/category/content/description + updated_at，
	// 保留 use_count/last_used_at/status/created_at。
	// 不存在则插入并为 src.ID 赋值。仅供扩展 seed 同步路径使用，不对用户侧暴露。
	UpsertExtensionSeed(ctx context.Context, src *snippet_entity.Snippet) error
	// DeleteExtensionSeedsMissing 硬删除 source 下 source_ref 不在 keepRefs 内的记录。
	// keepRefs 为空等价于 HardDeleteBySource(source)（清空该扩展全部 seed）。
	DeleteExtensionSeedsMissing(ctx context.Context, source string, keepRefs []string) error
}

var defaultSnippet SnippetRepo

// Snippet 获取 SnippetRepo 实例
func Snippet() SnippetRepo { return defaultSnippet }

// RegisterSnippet 注册 SnippetRepo 实现
func RegisterSnippet(r SnippetRepo) { defaultSnippet = r }
