package snippet_repo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/cago/database/db"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
)

// snippetRepo 默认 GORM 实现
type snippetRepo struct{}

// NewSnippet 创建默认实现
func NewSnippet() SnippetRepo { return &snippetRepo{} }

func (r *snippetRepo) Create(ctx context.Context, s *snippet_entity.Snippet) error {
	// 调用方（service 层）必须在此之前完成 Validate；repo 不再填默认值，
	// 以避免契约模糊（"repo 是否会校验"）。
	return db.Ctx(ctx).Create(s).Error
}

func (r *snippetRepo) Update(ctx context.Context, s *snippet_entity.Snippet) error {
	if s.ID == 0 {
		return errors.New("snippet id is required for update")
	}
	// 只更新允许字段。
	// 故意不包含：
	//   - status / created_at / use_count / last_used_at：生命周期字段，由专用方法管理。
	//   - source / source_ref：扩展来源权威字段，改动即可能翻转 IsReadOnly 并影响
	//     HardDeleteBySource 作用域，属于敏感权限。PR 5 的扩展种子同步走独立路径。
	//   - category：片段分类不可变更（业务语义上变更分类 = 一个新的片段）。
	return db.Ctx(ctx).Model(&snippet_entity.Snippet{}).
		Where("id = ?", s.ID).
		Updates(map[string]interface{}{
			"name":        s.Name,
			"content":     s.Content,
			"description": s.Description,
			"updated_at":  time.Now(),
		}).Error
}

func (r *snippetRepo) SoftDelete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Model(&snippet_entity.Snippet{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     snippet_entity.StatusDeleted,
			"updated_at": time.Now(),
		}).Error
}

func (r *snippetRepo) HardDeleteBySource(ctx context.Context, source string) error {
	if source == "" {
		return errors.New("source is required")
	}
	return db.Ctx(ctx).
		Where("source = ?", source).
		Delete(&snippet_entity.Snippet{}).Error
}

func (r *snippetRepo) GetByID(ctx context.Context, id int64) (*snippet_entity.Snippet, error) {
	var s snippet_entity.Snippet
	if err := db.Ctx(ctx).
		Where("id = ? AND status = ?", id, snippet_entity.StatusActive).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *snippetRepo) Find(ctx context.Context, q SnippetQuery) ([]*snippet_entity.Snippet, error) {
	query := db.Ctx(ctx).Model(&snippet_entity.Snippet{}).
		Where("status = ?", snippet_entity.StatusActive)

	if len(q.Categories) > 0 {
		query = query.Where("category IN ?", q.Categories)
	}
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR content LIKE ?", kw, kw, kw)
	}
	if len(q.Sources) > 0 {
		query = query.Where("source IN ?", q.Sources)
	}

	switch q.OrderBy {
	case "use_count_desc":
		query = query.Order("use_count DESC").Order("updated_at DESC")
	case "updated_at_desc":
		query = query.Order("updated_at DESC")
	default:
		// NOTE: SQLite-only. "last_used_at IS NULL" as an ORDER BY term relies on
		// SQLite's boolean→0/1 coercion. Postgres requires "ORDER BY last_used_at
		// DESC NULLS LAST" instead. If multi-driver support is added, rewrite this.
		query = query.Order("last_used_at IS NULL").
			Order("last_used_at DESC").
			Order("updated_at DESC")
	}

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}

	var list []*snippet_entity.Snippet
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *snippetRepo) FindBySourceRef(ctx context.Context, source, ref string) (*snippet_entity.Snippet, error) {
	var s snippet_entity.Snippet
	err := db.Ctx(ctx).
		Where("source = ? AND source_ref = ? AND status = ?", source, ref, snippet_entity.StatusActive).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *snippetRepo) TouchUsage(ctx context.Context, id int64) error {
	now := time.Now()
	return db.Ctx(ctx).Exec(
		`UPDATE snippets SET use_count = use_count + 1, last_used_at = ?, updated_at = ? WHERE id = ? AND status = ?`,
		now, now, id, snippet_entity.StatusActive,
	).Error
}

// SetLastAssets overwrites last_asset_ids. Empty slice writes empty string.
func (r *snippetRepo) SetLastAssets(ctx context.Context, id int64, assetIDs []int64) error {
	if id == 0 {
		return errors.New("snippet id is required")
	}
	strs := make([]string, 0, len(assetIDs))
	for _, a := range assetIDs {
		strs = append(strs, strconv.FormatInt(a, 10))
	}
	joined := strings.Join(strs, ",")
	return db.Ctx(ctx).
		Model(&snippet_entity.Snippet{}).
		Where("id = ? AND status = ?", id, snippet_entity.StatusActive).
		Update("last_asset_ids", joined).Error
}

// UpsertExtensionSeed 幂等写入扩展 seed，以 (source, source_ref) 为联合键。
// 事务内先查后写：命中则更新允许字段，未命中则 Create 并回填 ID。
func (r *snippetRepo) UpsertExtensionSeed(ctx context.Context, src *snippet_entity.Snippet) error {
	if src == nil {
		return errors.New("snippet is required")
	}
	if src.Source == "" || src.SourceRef == "" {
		return errors.New("source and source_ref are required for extension seed upsert")
	}
	return db.Ctx(ctx).Transaction(func(tx *gorm.DB) error {
		var existing snippet_entity.Snippet
		err := tx.Where("source = ? AND source_ref = ?", src.Source, src.SourceRef).
			First(&existing).Error
		if err == nil {
			// 命中：仅覆盖允许字段；保留 use_count/last_used_at/status/created_at。
			updates := map[string]interface{}{
				"name":        src.Name,
				"category":    src.Category,
				"content":     src.Content,
				"description": src.Description,
				"updated_at":  time.Now(),
			}
			if err := tx.Model(&snippet_entity.Snippet{}).
				Where("id = ?", existing.ID).
				Updates(updates).Error; err != nil {
				return err
			}
			src.ID = existing.ID
			src.UseCount = existing.UseCount
			src.LastUsedAt = existing.LastUsedAt
			src.CreatedAt = existing.CreatedAt
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 未命中：插入。
		return tx.Create(src).Error
	})
}

// DeleteExtensionSeedsMissing 硬删 source 下 source_ref 不在 keepRefs 的行。
// keepRefs 为空时等价于清空该 source。
func (r *snippetRepo) DeleteExtensionSeedsMissing(ctx context.Context, source string, keepRefs []string) error {
	if source == "" {
		return errors.New("source is required")
	}
	q := db.Ctx(ctx).Where("source = ?", source)
	if len(keepRefs) > 0 {
		q = q.Where("source_ref NOT IN ?", keepRefs)
	}
	return q.Delete(&snippet_entity.Snippet{}).Error
}
