package snippet_svc

import (
	"sort"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
)

// Category 片段分类元信息
type Category struct {
	ID        string `json:"id"`        // shell / sql / redis / mongo / prompt / <ext-declared>
	AssetType string `json:"assetType"` // ssh / database / redis / mongodb / "" / <ext 声明>
	Label     string `json:"label"`     // 英文 fallback；前端自行做 i18n
	Source    string `json:"source"`    // "builtin" 或 "ext:<name>"
}

// CategorySourceBuiltin 内置分类来源
const CategorySourceBuiltin = "builtin"

// ExtensionCategory 描述一个扩展贡献的分类条目。
// 注入 CategoryRegistry 时使用；service 层不依赖 pkg/extension 的类型，以避免循环依赖。
type ExtensionCategory struct {
	ID           string // 分类 ID
	AssetType    string // 绑定的资产类型
	Label        string // 本地化 fallback 标签
	ExtensionRef string // 源扩展名（作为 Source="ext:<name>" 的组成）
}

// ExtensionCategoryProvider 返回当前已加载扩展声明的所有分类。
// 实现方（extension_svc）需要在扩展启/停/装/卸后重新被查询；见 CategoryRegistry.RefreshFromExtensions。
type ExtensionCategoryProvider interface {
	ListExtensionCategories() []ExtensionCategory
}

// ExtensionCategoryProviderFunc 以函数形式实现 ExtensionCategoryProvider。
type ExtensionCategoryProviderFunc func() []ExtensionCategory

// ListExtensionCategories 实现 ExtensionCategoryProvider。
func (f ExtensionCategoryProviderFunc) ListExtensionCategories() []ExtensionCategory {
	if f == nil {
		return nil
	}
	return f()
}

// CategoryRegistry 分类注册表。包含 5 个内置分类 + 运行时已加载扩展的分类。
type CategoryRegistry struct {
	mu         sync.RWMutex
	builtins   []Category
	categories []Category
	index      map[string]Category
	provider   ExtensionCategoryProvider
}

// NewCategoryRegistry 预加载内置分类。provider 可为 nil（仅内置；扩展集成 PR 5 之前的行为）。
func NewCategoryRegistry() *CategoryRegistry {
	builtins := []Category{
		{ID: snippet_entity.CategoryShell, AssetType: "ssh", Label: "Shell", Source: CategorySourceBuiltin},
		{ID: snippet_entity.CategorySQL, AssetType: "database", Label: "SQL", Source: CategorySourceBuiltin},
		{ID: snippet_entity.CategoryRedis, AssetType: "redis", Label: "Redis", Source: CategorySourceBuiltin},
		{ID: snippet_entity.CategoryMongo, AssetType: "mongodb", Label: "Mongo", Source: CategorySourceBuiltin},
		{ID: snippet_entity.CategoryK8s, AssetType: "k8s", Label: "K8S", Source: CategorySourceBuiltin},
		{ID: snippet_entity.CategoryPrompt, AssetType: "", Label: "Prompt", Source: CategorySourceBuiltin},
	}
	r := &CategoryRegistry{builtins: builtins}
	r.rebuildLocked() // initial: 仅内置
	return r
}

// SetExtensionProvider 注入扩展分类来源。必须在 bootstrap 期（extension_svc 构造后）调用。
// 调用后需要显式 RefreshFromExtensions 才会实际拉取一次。
func (r *CategoryRegistry) SetExtensionProvider(p ExtensionCategoryProvider) {
	r.mu.Lock()
	r.provider = p
	r.mu.Unlock()
}

// RefreshFromExtensions 从 provider 重新拉取扩展分类并与内置合并。
// 内置分类永远存在且不可被扩展覆盖：若扩展分类 ID 命中内置则静默跳过（manifest 校验应已拦截）。
func (r *CategoryRegistry) RefreshFromExtensions() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rebuildLocked()
}

// rebuildLocked 重建 categories/index。调用方必须持有写锁。
func (r *CategoryRegistry) rebuildLocked() {
	// 内置分类先入
	next := make([]Category, 0, len(r.builtins))
	next = append(next, r.builtins...)
	builtinIDs := make(map[string]struct{}, len(r.builtins))
	for _, b := range r.builtins {
		builtinIDs[b.ID] = struct{}{}
	}

	if r.provider != nil {
		seen := make(map[string]struct{})
		exts := r.provider.ListExtensionCategories()
		// 稳定排序：按 ID 升序，避免多扩展顺序跳变。
		sort.Slice(exts, func(i, j int) bool { return exts[i].ID < exts[j].ID })
		for _, ec := range exts {
			if ec.ID == "" {
				continue
			}
			if _, isBuiltin := builtinIDs[ec.ID]; isBuiltin {
				continue // 防御性：不允许覆盖内置
			}
			if _, dup := seen[ec.ID]; dup {
				continue
			}
			seen[ec.ID] = struct{}{}
			next = append(next, Category{
				ID:        ec.ID,
				AssetType: ec.AssetType,
				Label:     ec.Label,
				Source:    snippet_entity.SourceExtPrefix + ec.ExtensionRef,
			})
		}
	}

	idx := make(map[string]Category, len(next))
	for _, c := range next {
		idx[c.ID] = c
	}
	r.categories = next
	r.index = idx
}

// List 返回所有已注册分类（顺序稳定）。
func (r *CategoryRegistry) List() []Category {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Category, len(r.categories))
	copy(out, r.categories)
	return out
}

// Get 查找指定 ID 的分类。
func (r *CategoryRegistry) Get(id string) (Category, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.index[id]
	return c, ok
}

// IDs 返回所有已注册分类的 ID 列表（稳定顺序，与 List 一致）。
func (r *CategoryRegistry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, len(r.categories))
	for i, c := range r.categories {
		ids[i] = c.ID
	}
	return ids
}
