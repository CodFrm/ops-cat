# Asset Tree Type Filter — Design

Date: 2026-04-27
Status: Approved (pending implementation plan)

## 背景与动机

PR #37 (`c1cee81`) 引入了首页分区按钮（Home / Database / SSH / Redis / MongoDB），把"按资产类型筛选"以 sidebar 图标的形式暴露，并附带了一系列隐式的"切换 section 时折叠/展开 AssetTree、激活匹配 tab"行为。

这个方案有两处问题：

1. **不支持扩展资产**。`normalizeAssetSection` 硬编码了 5 种内置 type，扩展资产（如 kubernetes、docker）的资产 type 不会落入任何 section，只能在 "Home" 视图里看到，无法与内置类型一样被快速筛选。
2. **隐式联动让用户困惑**。点击 section 按钮会同时改变 sidebar 折叠状态、激活某个 tab；连接资产、打开 page tab 也会自动收起 AssetTree。这些"贴心"行为反而让 AssetTree 的可见性变得不可预测。

本次改造把"类型筛选"从 sidebar 移入 AssetTree 内部，作为列表自身的过滤维度（与"按名称搜索"同级），并恢复 PR #37 之前"AssetTree 仅由显式按钮控制显隐"的语义。

## 目标

- 类型筛选 UI 内聚到 AssetTree，支持内置类型与扩展资产类型，多选。
- 删除 sidebar 上的 4 个类型按钮（Database/SSH/Redis/MongoDB），保留 Home。
- 删除 `homeSection` / `normalizeAssetSection` / `tabBelongsToSection` / `hideAssetListAfterConnect` 等与 section 概念绑定的状态和函数。
- 恢复 pre-#37 行为：连接资产、点击 page tab、点击 Home 都不再隐式折叠 AssetTree。

## 非目标

- 不引入 SideTabList 自己的类型筛选；本次仅解除它和 `homeSection` 的耦合。
- 不变更资产连接、tab 打开、信息面板等其它流程的语义。
- 不重新设计 AssetTree 的搜索、分组树展示。

## 设计

### 数据层

#### 类型选项汇总

新增 `frontend/src/lib/assetTypes/options.ts`：

```ts
export interface AssetTypeOption {
  // 选中时用于匹配 asset.Type。对内置 "database" 这种聚合项，aliases 列出所有等价值。
  value: string;
  aliases: string[];        // 默认 [value]，"database" 时为 ["database", "mysql", "postgresql"]
  labelKey: string;          // 内置: "nav.ssh" 等；扩展: 直接放 i18n.name 字符串
  iconName?: string;         // 用于 getIconComponent 解析；内置写死 lucide 图标名
  group: "builtin" | "extension";
}

export function getAssetTypeOptions(
  extensions: ReturnType<typeof useExtensionStore.getState>["extensions"],
): AssetTypeOption[];
```

- 内置项静态返回 4 个：ssh / database / redis / mongodb（聚合 mysql/postgresql 到 database 的逻辑放进 `aliases`）。
- 扩展项遍历 `extensions[].manifest.assetTypes`，每个 `assetType` 产出一个 option，icon 取 `manifest.icon`。
- 扩展中如果声明了与内置同名的 type（如某扩展也叫 `ssh`），仍按各自来源列出，使用方自己处理潜在重名（短期不强加去重）。

#### 删除清单

- `lib/assetTypes/index.ts`：删除 `HomeSection` 类型与 `normalizeAssetSection` 函数。
- `lib/tabSection.ts`：整个文件删除。
- 对应测试用例删除或迁移。

### 状态层

筛选状态完全内聚到 `AssetTree.tsx`，不上提到 store 或 App.tsx。

```ts
// 内部 state
const [selectedTypes, setSelectedTypes] = useState<string[] | "all">(
  () => loadFromLocalStorage()
);

useEffect(() => {
  saveToLocalStorage(selectedTypes);
}, [selectedTypes]);
```

- localStorage key：`asset_tree_type_filter`。
- 序列化：`"all"` 字面量保存为字符串 `"all"`；具体选择保存为 JSON 数组。
- 默认值：`"all"`。
- "全部取消"自动回弹为 `"all"`，避免出现"什么都不显示"的空筛选状态。

### UI 层

#### 顶部布局调整

`AssetTree.tsx` 第 213-221 行的搜索框行从

```tsx
<div className="relative">
  <Search ... />
  <input ... />
</div>
```

改为搜索框 + 右侧 funnel 按钮：

```tsx
<div className="flex items-center gap-1">
  <div className="relative flex-1">
    <Search ... />
    <input ... />
  </div>
  <FilterButton selectedTypes={selectedTypes} onChange={setSelectedTypes} />
</div>
```

#### `<FilterButton>`

放在 AssetTree 文件内部或拆到 `frontend/src/components/asset/AssetTypeFilterButton.tsx`：

- 按钮 h-7 w-7 ghost icon，`Filter` 图标。
- 当 `selectedTypes !== "all"` 时，按钮右上角叠加 1.5×1.5 的 `bg-primary` 圆点。
- 点击展开 `Popover`：

```
┌────────────────────────────────┐
│ ☑ All types                    │
│ ───────────────────────────── │
│ <icon> ☑ SSH                   │
│ <icon> ☑ Database (MySQL/PG)   │
│ <icon> ☑ Redis                 │
│ <icon> ☑ MongoDB               │
│ ─── EXTENSIONS ───────────────│  ← 仅当存在扩展类型时
│ <icon> ☑ Kubernetes            │
│ <icon> ☑ Docker                │
└────────────────────────────────┘
```

- "All types" 行的 checkbox：当前为 `"all"` 时 checked；否则不 checked。点击：`"all"` → `[]`（空数组立即视为重新规约为 `"all"`，等价于"重置"），其它 → `"all"`。简化版：点击始终切换为 `"all"`。
- 单项 checkbox：点击 toggle 该 option `value`。toggle 后若 `selectedTypes` 落空（长度 0），自动回到 `"all"`。
- popover 内 `ScrollArea`，max-h 360px，扩展类型多时滚动。

i18n 新增：

| key | zh-CN | en |
|---|---|---|
| `asset.filterByType` | 按资产类型筛选 | Filter by asset type |
| `asset.filterByTypeActive` | 已筛选 {{count}} 类 | {{count}} type(s) selected |
| `asset.filterAllTypes` | 全部类型 | All types |
| `asset.filterExtensions` | 扩展资产 | Extensions |

`nav.ssh / nav.database / nav.redis / nav.mongodb` 保留，作为 popover 内置项 label 复用。Database 项的 label 直接用 `nav.database`，不在文案里附加 "(MySQL/PG)" 之类的子说明（aliases 仅在过滤逻辑层生效，不暴露给 UI）。

#### 过滤逻辑

`AssetTree.tsx` 第 105 行：

```ts
const sectionAssets = matchSelectedTypes(assets, selectedTypes);
```

`matchSelectedTypes` 实现要点：
- `selectedTypes === "all"` → 直接返回 `assets`。
- 否则把每个 selected `value` 展开成 `aliases` 集合，按 `asset.Type ∈ aliasesSet` 匹配。
- 大小写规范：与现有 `normalizeAssetSection` 保持一致（`type.trim().toLowerCase()` 后比对）。

### App.tsx 清理

| 操作 | 位置 | 内容 |
|---|---|---|
| 删除 | L24-25 | `HomeSection` import、`tabBelongsToSection` import |
| 删除 | L128 | `homeSection` state |
| 删除 | L152-163 | `hideAssetListAfterConnect` 函数 |
| 删除 | L260, 281, 289, 299, 374 | 全部 `hideAssetListAfterConnect()` 调用 |
| 简化 | L306-377 | `handlePageChange` 还原 pre-#37 形态：`page === "home"` 仅激活第一个 terminal/info tab；其它 page 走 page tab 打开/激活；不再切 section、不再触碰 `assetTreeCollapsed` / `leftPanelVisible` |
| 简化 | L385 | `activePage` 派生：active tab 是 page 用 page id，否则归 `"home"` |
| 删除 | L412, 437, 462 | 给 `AssetTree` 和 `SideTabList` 传递的 `homeSection` prop |

### Sidebar.tsx 清理

`navGroups` 第二组（4 个类型按钮）整体删除：

```ts
const navGroups: NavItem[][] = [
  [{ id: "home", icon: Home, label: t("nav.home") }],
  [
    { id: "forward", icon: ArrowRightLeft, label: t("nav.forward") },
    { id: "sshkeys", icon: KeyRound, label: t("nav.sshKeys") },
    { id: "snippets", icon: FileCode, label: t("nav.snippets") },
    { id: "audit", icon: ScrollText, label: t("nav.audit") },
  ],
];
```

未使用的 lucide 图标 `Database / Monitor / Cylinder / Leaf` 在 Sidebar 内删除 import；这些图标在 `lib/assetTypes/options.ts` 内置项里改为重新 import。

### SideTabList.tsx 清理

- 删除 `homeSection` prop、`HomeSection` import、`tabBelongsToSection` import 与调用。
- `tabs` 全集直接交给已有的内部筛选 popover/搜索处理。

### AssetTree.tsx Props 调整

- 移除 `homeSection?: HomeSection` 参数。
- 移除 `normalizeAssetSection` import。
- 内部新增筛选状态、`<FilterButton>`、过滤逻辑。

## 测试

### 单元测试

- 新增 `__tests__/assetTypeOptions.test.ts`：
  - 内置 4 项稳定输出。
  - mock 一个扩展，验证扩展类型出现在 `extension` 分组。
  - `database` option 的 `aliases` 包含 `mysql`、`postgresql`、`database`。
- 更新 `__tests__/AssetTree.test.tsx`：
  - 删除 `homeSection` 相关用例。
  - 新增："filter button toggles popover"、"checking SSH only filters out non-SSH assets"、"unchecking last item falls back to all"、"extension asset type appears in filter and filters correctly"。
- 删除 `lib/tabSection.ts` 相关测试（如有）。

### 手测脚本

1. 启动 `make dev`，sidebar 上不应再看到 Database/SSH/Redis/MongoDB 4 个按钮。
2. AssetTree 顶部搜索框右侧出现 funnel 图标。
3. 点击 funnel，popover 打开，看到 4 个内置选项（默认全选）。
4. 取消勾选 SSH，AssetTree 中 SSH 类型资产消失，funnel 出现红点。
5. 把所有项取消，AssetTree 自动回到全选状态。
6. 关闭并重启应用，筛选状态恢复。
7. 在 `../extensions` 加载一个有 `assetTypes` 的扩展，重新进入 popover，看到 "Extensions" 分组与该类型；勾选/取消生效。
8. 连接一个资产、打开 page tab：AssetTree 不再自动折叠。
9. 点击 Home 按钮：仅切换到第一个 terminal/info tab，AssetTree 显隐不变。
10. PanelLeftClose / EyeOff 按钮仍能正常折叠/隐藏 AssetTree。

## 影响面

- **变更文件**：
  - `frontend/src/App.tsx`
  - `frontend/src/components/layout/Sidebar.tsx`
  - `frontend/src/components/layout/AssetTree.tsx`
  - `frontend/src/components/layout/SideTabList.tsx`
  - `frontend/src/lib/assetTypes/index.ts`
  - `frontend/src/lib/assetTypes/options.ts`（新增）
  - `frontend/src/lib/tabSection.ts`（删除）
  - `frontend/src/i18n/locales/{zh-CN,en}/common.json`
  - 对应 `__tests__/`
- **后端 / Wails 绑定 / opsctl**：无影响。
- **用户可见变化**：sidebar 少 4 个按钮；AssetTree 多了一个 funnel 按钮；连接/翻页时 AssetTree 不再自动隐藏（行为回归）。

## 兼容性

- `localStorage["asset_tree_type_filter"]` 是新键，老用户首次进入读到 `null` → 默认 `"all"`。
- PR #37 引入的 `localStorage` 副作用键（如有）保持不动。
- 由于 Sidebar 的 4 个 type 按钮被删，原本依赖它们触发 `hideAssetListAfterConnect` 副作用的用户行为不再适用 —— 这是显式恢复 pre-#37 语义的一部分，不属于 regression。
