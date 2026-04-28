# Cmd+P Quick Open 设计文档

**日期**: 2026-04-28
**状态**: 待实现
**作者**: 头脑风暴产物（codfrm + Claude）

## 背景与目标

OpsKat 桌面端目前要打开资产或在已开标签间跳转，只能在左侧 AssetTree / TopTabBar 用鼠标操作。借鉴 VSCode `Cmd+P` 体验，提供一个键盘驱动的 Quick Open 浮层，让用户：

1. 一键唤起搜索框，输入资产名（支持中文 / 拼音）回车直接打开
2. 在搜索框为空时，看到当前已打开的 tab 与最近用过的资产，方便快速跳转

非目标（明确 YAGNI）：

- **不**做 VSCode `Cmd+Shift+P` 命令面板（执行命令、改设置等）
- **不**搜索 group / snippet / AI 会话 / 设置页 — 只搜资产 + 已打开 tab
- **不**支持模糊正则、`@` `:` 等前缀符号

## 用户体验

### 触发
- 默认快捷键 `⌘P` (Mac) / `Ctrl+P` (Win)，注册在 `shortcutStore`，用户可改键
- 浮层打开时再按一次 → 关闭（toggle）
- `Esc` 关闭

### 布局
- Radix `Dialog` 居中浮层，宽度 `max-w-2xl`，高度自适应、内列表 `max-h-96` 滚动
- 顶部：单行 `Input` 输入框，autofocus，placeholder：`搜索资产或已打开的标签...`
- 中部：分组结果列表
- 底部：键位提示 `↑↓ 选择 · ↵ 打开 · Esc 关闭`

### 结果分组规则

| query 状态 | Section 顺序与内容 |
|---|---|
| 空 | ① **已打开**：当前所有 tab（不限制条数，单 section 内最多 50 行）<br>② **最近**：从 `recentAssetStore.recentIds`（最多 20）按顺序取，先过滤掉"在已打开 section 中已出现的 assetId"和"在 `assetStore.assets` 里已不存在的 assetId"，再取前 **5** 行 |
| 非空 | ① **已打开**：tab.label 大小写不敏感包含 query；对 terminal/query/info 类 tab 同时匹配其所属资产的 name（包括拼音）和 host<br>② **资产**：`filterAssets(assets, groups, {query, limit:50})` 结果（去掉已在 ① 中的 assetId） |

### 排序
- 已打开 tab section：按 `tabStore` 中的原顺序
- 资产 section：复用 `lib/assetSearch.ts` 的 rank（前缀 > 包含 > 拼音 > groupPath）

### 行内展示
单行：`[图标] 名称 [groupPath 灰字] ............... [类型 badge]`
- 名称用 `lib/highlightMatch.ts` 给命中字符加底色
- 已打开 section 的行不显示 groupPath，显示一个绿点 indicator
- badge 文案 i18n：`终端 / 查询 / 信息 / 页面`

### 键盘
- `↑/↓`：activeIndex 在 flat 化的所有可选行间上下移动；到边界不循环
- `Enter`：触发 activeIndex 行的动作
- `Esc`：关闭
- 鼠标 hover：同步 activeIndex
- query 变化：activeIndex 重置为 0
- IME 中文输入中（`e.isComposing`）：忽略 ↑↓Enter

## 架构

### 新增 / 修改文件

```
frontend/src/
├── components/command/
│   └── CommandPalette.tsx          ← 新增
├── lib/
│   └── openAssetDefault.ts         ← 新增（canConnect → onConnectAsset；否则 → openAssetInfoTab）
├── stores/
│   ├── recentAssetStore.ts         ← 新增
│   ├── shortcutStore.ts            ← 修改：新增 action "command.quickopen"
│   └── tabStore.ts                 ← 修改：openTab 内调用 recentAssetStore.touch
├── components/layout/
│   └── MainPanel.tsx               ← 修改：注册快捷键、挂载 <CommandPalette />
└── i18n/locales/{zh-CN,en}/common.json   ← 修改：新增 commandPalette.* + shortcutsList.commandQuickopen
```

挂载点选 `MainPanel`：与 `panel.ai` / `panel.filter` / `page.settings` 等同级快捷键的现有监听点保持一致。

### 数据流

```
用户按 ⌘P
   │
   ▼
MainPanel handleKeyDown ─ matchShortcut ─→ "command.quickopen"
   │                                                │
   │  e.preventDefault()                            │
   ▼                                                ▼
setCommandOpen(true)                          (toggle if already open)
   │
   ▼
<CommandPalette open onOpenChange onConnectAsset/>
   │
   ├─ 读取 useTabStore.tabs            ─┐
   ├─ 读取 useAssetStore.assets/groups  ├─→ 组装 sections
   ├─ 读取 useRecentAssetStore.recentIds┘
   ▼
用户在 Input 输入 query
   │
   ▼
filterAssets(...) + tab name 过滤 → 重新组装 sections
   │
   ▼
用户按 Enter
   │
   ├─ 行 = 已打开 tab → useTabStore.activateTab(tab.id)
   │
   └─ 行 = 资产 → openAssetDefault(asset, onConnectAsset)
                     │
                     ├─ canConnect       → onConnectAsset(asset)
                     │                       (App.tsx 内 handleConnectAsset 再分派
                     │                        terminal / query / extension)
                     │
                     └─ !canConnect      → openAssetInfoTab(asset.ID)
                            │
                            ▼
                     useTabStore.openTab(...)
                            │
                            ▼
                     openTab 内部：若 meta.assetId 存在
                       → useRecentAssetStore.getState().touch(assetId)
   │
   ▼
onOpenChange(false)
```

## 模块详细设计

### `recentAssetStore.ts`

```ts
const STORAGE_KEY = "recent_assets";
const MAX_RECENT = 20;

interface RecentAssetState {
  recentIds: number[];                   // 最近在前
  touch: (id: number) => void;           // 移到首位，去重，截断到 20
  remove: (id: number) => void;          // 删除指定 id（资产被删除时调用）
}
```

- 持久化：localStorage `recent_assets`，写时序列化 `JSON.stringify(recentIds)`
- 读取容错：JSON 解析失败 → 重置为 `[]`
- `touch(id)`：`recentIds = [id, ...recentIds.filter(x => x !== id)].slice(0, 20)`

### `tabStore.ts` 改动

`openTab(tab)` 函数内，在添加完 tab 后追加一行：

```ts
const meta = tab.meta as Partial<{ assetId: number }>;
if (meta?.assetId) {
  useRecentAssetStore.getState().touch(meta.assetId);
}
```

注意：terminal/query meta 有 `assetId`；page meta 可选 `assetId`（扩展页的资产关联）；**info meta 的字段名是 `targetId` 且只有 `targetType === "asset"` 时才是资产 ID** —— 这种情况下也要 touch。AI tab 没有资产，跳过。

### `lib/openAssetDefault.ts`

签名：

```ts
export function openAssetDefault(
  asset: asset_entity.Asset,
  onConnectAsset: (asset: asset_entity.Asset) => void
): void
```

行为：

```ts
const def = getAssetType(asset.Type);
if (def?.canConnect) {
  onConnectAsset(asset);    // App.tsx 的 handleConnectAsset 已含 query/terminal/extension 三向分派
} else {
  openAssetInfoTab(asset.ID);
}
```

**为什么不复用 AssetTree 双击逻辑**：AssetTree 双击只在 `canConnect` 时才有动作（不可连接资产双击什么都不发生）。CommandPalette 期望任何资产都能"打开"，所以非连接资产 fallback 到 info tab。两边语义不同，但 CommandPalette 这套 3 行逻辑本身简单到不必跨边界，**不**修改 AssetTree 的双击行为，避免越权改动。

`openAssetInfoTab(assetId)` 已存在（接收的是 `assetId: number`，不是 asset 对象）。

### `CommandPalette.tsx`

Props:
```ts
interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
}
```

内部 hook：
```ts
const tabs = useTabStore((s) => s.tabs);
const assets = useAssetStore((s) => s.assets);
const groups = useAssetStore((s) => s.groups);
const recentIds = useRecentAssetStore((s) => s.recentIds);
const [query, setQuery] = useState("");
const [activeIndex, setActiveIndex] = useState(0);
```

`sections` 用 `useMemo` 派生（依赖 `query / tabs / assets / groups / recentIds`）。`flatRows` 也 useMemo 一次，键盘导航的 index 在它上面走。

### `shortcutStore.ts` 改动

```ts
SHORTCUT_ACTIONS: 末尾加 "command.quickopen"
DEFAULT_SHORTCUTS: 加 "command.quickopen": { code: "KeyP", mod: true, shift: false, alt: false }
```

设置页快捷键列表会自动渲染（项目已有遍历 SHORTCUT_ACTIONS 的逻辑），只需补 i18n 文案 `shortcutsList.commandQuickopen`。

### `MainPanel.tsx` 改动

```ts
const [commandOpen, setCommandOpen] = useState(false);

// 已有 handleKeyDown switch 内增加：
case "command.quickopen":
  e.preventDefault();
  setCommandOpen((v) => !v);
  break;

// 渲染树最末端追加：
<CommandPalette
  open={commandOpen}
  onOpenChange={setCommandOpen}
  onConnectAsset={onConnectAsset}
/>
```

## 边界 / 错误处理

| 场景 | 处理 |
|---|---|
| 资产列表为空 | 仅显示已打开 tab section；都没有 → EmptyState：`暂无内容，先在左侧添加资产` |
| query 无命中 | EmptyState：`没有匹配的结果` |
| recent 含已删资产 | 渲染前用 `assetStore.assets` 过滤；同时在删资产入口调用 `recentAssetStore.remove(id)` |
| IME 输入中按 ↑/↓/Enter | `e.isComposing === true` 直接 return |
| Cmd+P 时浮层已开 | toggle 关闭 |
| 输入框内按 ↑/↓ | `e.preventDefault()` 后转 activeIndex（避免移动光标） |
| localStorage 解析失败 | recentIds 重置为 `[]`，不抛错 |
| 资产无 GroupID | groupPath 为空字符串，行内不显示 |

## 测试

新增 vitest 文件（按项目 `src/__tests__/setup.ts` 已 mock Wails runtime 的方式）：

### `__tests__/recentAssetStore.test.ts`
- `touch` 新 id 加到首位
- `touch` 已存在 id 移到首位，不重复
- 超过 20 时截断尾部
- `remove` 删除指定 id
- 跨实例从 localStorage 恢复
- localStorage 损坏数据 → 重置为 `[]`

### `__tests__/openAssetDefault.test.ts`
- canConnect 资产（如 ssh / database / redis） → 调用 `onConnectAsset`
- 不可连接资产（registry 中无 type 或 canConnect=false） → 调用 `openAssetInfoTab(asset.ID)`

### `__tests__/commandPalette.test.tsx`
- 空 query 渲染 opened + recent 两个 section（mock 各 2 个 tab、3 个 recent）
- 输入 query：opened-matched 与 assets section 同时出现，且重复 assetId 不在 assets section
- ↑↓ 改变 activeIndex，到边界不循环
- Enter 选中已打开 tab → `activateTab` 被调用、浮层关闭
- Enter 选中资产 → `openAssetDefault` 被调用、浮层关闭
- Esc → `onOpenChange(false)`
- IME composing 期间 ↑↓Enter 被忽略

## i18n 文案

新增 keys（`zh-CN` / `en` 都加）：

```
commandPalette.placeholder       搜索资产或已打开的标签...        Search assets or open tabs...
commandPalette.section.opened    已打开                          Open
commandPalette.section.recent    最近                            Recent
commandPalette.section.assets    资产                            Assets
commandPalette.empty.noContent   暂无内容，先在左侧添加资产        No items. Add an asset first.
commandPalette.empty.noMatch     没有匹配的结果                  No matching results
commandPalette.footer.navigate   选择                            Navigate
commandPalette.footer.open       打开                            Open
commandPalette.footer.close      关闭                            Close
commandPalette.badge.terminal    终端                            Terminal
commandPalette.badge.query       查询                            Query
commandPalette.badge.info        信息                            Info
commandPalette.badge.page        页面                            Page
shortcut.command.quickopen       快速打开                        Quick Open
```

## 实施清单（提示给 writing-plans）

1. 新增 `recentAssetStore.ts` + 测试
2. 新增 `lib/openAssetDefault.ts` + 测试（CommandPalette 专用，**不**修改 AssetTree）
3. `tabStore.openTab` 内挂 `recentAssetStore.touch`，删资产入口挂 `recentAssetStore.remove`
4. `shortcutStore` 增加 `command.quickopen`
5. 新增 `components/command/CommandPalette.tsx` + 测试
6. `MainPanel.tsx` 注册快捷键 + 挂载浮层
7. i18n 文案补全（zh-CN / en）
8. 跑一遍 `pnpm test` + `pnpm lint`，最后 `make dev` 手测一次（resize / 空状态 / IME）
