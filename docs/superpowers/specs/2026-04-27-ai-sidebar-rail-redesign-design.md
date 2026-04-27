# AI 侧边助手会话 Rail 双态重设计

**日期**：2026-04-27
**状态**：草案已批准，待实施
**作者**：Brainstorm 协作（用户 + Claude）

## 背景与问题

当前 AI 侧边助手面板默认宽 360px，右侧的纵向会话标签栏（`SideAssistantTabBar`）会根据面板宽度在 128 / 156 / 176px 之间伸缩。这个 rail 占走 ~130–180px，对话内容区（消息流 + 输入框）只剩 ~180–230px，被严重挤压。

代码位置：

- `frontend/src/components/ai/SideAssistantPanel.tsx:54-55` 计算 `sessionRailWidth`
- `frontend/src/components/ai/SideAssistantTabBar.tsx` 渲染纵向标签列表

最近相关提交：`cb9bb78`（重构 AI 侧边助手为右侧纵向会话栏）、`ce3e285`（折叠态对齐）。

## 用户场景

确认的常态使用：**同时开 2–4 个会话**，会话栏需要常驻可见、支持快速切换；列表标题文字不是必须时刻可见，多数情况下用户能凭"在哪个会话里"做出判断。

## 方案概述

把现在的"标签列表式" rail 替换为**双态图标 rail**：

- **窄态（默认）**：宽 36px。每个会话渲染为 28×28 圆角图标方块，标题首字 + 标题哈希配色，状态点在右下角。完整标题通过 hover tooltip 展示。
- **宽态（可选）**：rail 顶部 ⇄ 按钮一键切换。展开后宽度默认 150px，可由左侧拖拽手柄调整（120–220px），状态持久化到 localStorage。
- 历史下拉按钮保留在面板顶部 header；新建会话按钮（＋）放进 rail 顶部按钮组；移除原 "会话 (N)" 标题区。

收益：默认窄态下对话区从 ~232px → ~324px（+40%）。

## 组件与文件改动

全部在 `frontend/src/components/ai/` 下。

### `SideAssistantTabBar.tsx` — 重写

新 props：

```ts
interface SideAssistantTabBarProps {
  tabs: SidebarAITab[];
  activeTabId: string | null;
  getStatus: (tabId: string) => SidebarTabStatus;
  collapsed: boolean;          // 新：窄态/宽态切换
  width: number;               // 新：宽态下当前宽度
  onActivate: (tabId: string) => void;
  onClose: (tabId: string) => void;
  onNewChat: () => void;       // 新：rail 顶部 + 按钮
  onToggleCollapsed: () => void; // 新：rail 顶部 ⇄ 按钮
}
```

删除原 `compact` prop（被 `collapsed` 取代）。

**窄态渲染**：

- rail 顶部一排两按钮（⇄、＋），24×24，垂直堆叠，与下方会话图标用细分隔线分开
- 会话图标 28×28，圆角 7px，背景色由 `getSessionIconColor(title)` 决定，居中白字加粗显示首字
- 选中态：外圈 2px 主色（`primary`）描边，不改背景色
- 状态点：右下角 9px 圆点，2px 同 rail 背景描边，颜色映射见下文
- hover：右上角浮出 14×14 ✕ 关闭按钮；中键也触发 `onClose`
- tooltip：使用 Radix Tooltip，`delayDuration: 300`，左侧弹出，内容为 `<title>` + 状态后缀

**宽态渲染**：

- 同样有顶部 ⇄ + ＋ 按钮组，但水平排布在顶部一行
- 会话图标 28×28（与窄态一致），右侧显示标题（一行截断）+ 状态副标题（如有）
- 选中态：背景高亮（沿用现有 `bg-background/95` 样式）+ 左侧色条
- 整条 rail 左边缘是拖拽手柄，光标 `col-resize`

### `SideAssistantPanel.tsx` — 调整

- 删除 `isCompactSessionRail` 与 `sessionRailWidth` 的计算
- 新增两个状态：
  - `railCollapsed: boolean`（localStorage key `ai_sidebar_rail_collapsed`，默认 `true`）
  - `railWidth: number`（localStorage key `ai_sidebar_rail_width`，默认 `150`）
- 第二个 `useResizeHandle` 只在 `!railCollapsed` 时挂载，绑到 rail 自身的左边缘
- 渲染时：

```ts
const railRenderWidth = railCollapsed ? 36 : railWidth;
```

- `tabs.length === 0` 时整个 rail 不渲染（保留现有的 `emptyGuide` 居中提示）

### 新增 `frontend/src/components/ai/sessionIconColor.ts`

单一职责工具，便于单元测试：

```ts
// 8–10 色固定调色板，OKLCH 写法，深浅主题各自有 fg/bg 对
export function getSessionIconColor(title: string): { bg: string; fg: string };

// 提取首字符
// - 去前导空白和 ASCII 标点
// - emoji 开头：保留 emoji
// - 中文：返回首字
// - 英文：返回首字母大写
// - 空串：返回 "?"
export function getSessionIconLetter(title: string): string;
```

颜色用 OKLCH 调色板（避免 Tailwind 动态类问题），`hash(title) % palette.length` → 同一标题永远同一色。

### i18n 改动

`frontend/src/i18n/locales/{zh-CN,en}/common.json`：

新增 key：

- `ai.sidebar.expandRail` / `ai.sidebar.collapseRail`（⇄ 按钮 tooltip）
- `ai.sidebar.statusSuffix.running` / `done` / `error` / `waiting_approval`（拼到 tooltip 末尾）

删除 key：

- `ai.sidebar.sessions`（"会话 (N)" 标题已移除）

### 不变

- `aiStore`：tab 数据结构、状态机、所有导航与发送函数完全复用
- `SideAssistantHeader`：历史下拉按钮位置不动
- 面板自身的宽度与折叠（`ai_sidebar_width`、`collapsed`）逻辑不动

## 数据流与持久化

| Key | 类型 | 默认 | 说明 |
|---|---|---|---|
| `ai_sidebar_width` | number | 360 | 整个 AI 面板宽（已有，不动） |
| `ai_sidebar_rail_collapsed` | bool | `true` | rail 是窄态还是宽态 |
| `ai_sidebar_rail_width` | number | 150 | 宽态下 rail 的宽 |

**为什么把 rail 折叠状态独立存**：和面板宽解耦。用户可能把面板拖到 500px、rail 仍是窄态；也可能面板 360px、rail 展开。两个独立的人机决策，应分别记录。

**会话图标颜色稳定性**：`getSessionIconColor` 是纯函数，对同一 title 永远返回同一色。aiStore 现有逻辑会在第一条用户消息后生成会话标题，此时颜色会从"无标题灰底"切换到稳定色，acceptable。

**状态点颜色映射**（复用 `SideAssistantTabBar.tsx:15-20` 现有色，与图标背景色来源**不同**——图标背景走 OKLCH 自定义调色板，状态点走 Tailwind 现有色阶）：

| status | 颜色 |
|---|---|
| `running` | `bg-sky-500` |
| `done` | `bg-emerald-500` |
| `error` | `bg-rose-500` |
| `waiting_approval` | `bg-amber-500` |
| `null` | 不渲染 |

视觉变化：从"小色条/spinner 在标题左边" → "小圆点在图标右下角"。`running` 状态在窄态下不再渲染 spinner（28×28 太小），用 `sky-500` 圆点 + tooltip 文字"运行中"标识。

## 边界情况与可访问性

### 首字提取（`getSessionIconLetter`）

测试覆盖：

| 输入 | 输出 |
|---|---|
| `"写迁移"` | `"写"` |
| `"ssh debug"` | `"S"` |
| `"  ssh"` | `"S"` |
| `"🐛 调研"` | `"🐛"` |
| `""` | `"?"` |
| `"   "` | `"?"` |

### Tooltip

- Radix `Tooltip`，`delayDuration: 300`
- 移开图标立即关闭（避免快速切换叠 tooltip）
- 内容：`title + (status ? " · " + i18n.statusSuffix[status] : "")`

### 可访问性

- 每个会话图标 `<button role="tab" aria-selected aria-label="<title> · <status>">`，键盘 Tab 可达，Enter/Space 激活
- 状态不能仅靠颜色：`aria-label` 与 tooltip 都包含状态文字
- ⇄、＋、✕ 全部 icon-only button，必须有 `aria-label` + `title`

### Resize 边界

- 宽态拖到 < 80px → 强制吸附为窄态，`railCollapsed` 翻为 `true`，`railWidth` 还原为上次记录值（不写入小于 120 的值）
- 宽态拖到 > 220px → 钳制
- AI 面板自身被拖到 < 200px 时 rail 仍按 36px 渲染，对话区可压到 ~150px，与现在一致

### 中键 / 右键

- 中键点会话图标 = 关闭（浏览器标签习惯）
- 右键暂不做菜单（YAGNI）

### 阻塞确认

`closeSidebarTab`（aiStore）现有的运行中/未保存确认逻辑保留，UI 层仅改 ✕ 的位置。

### 键盘快捷键

不在本次范围（YAGNI，当前也没有）。

## 测试策略

### 单元测试（Vitest，`frontend/src/components/ai/__tests__/`）

**`sessionIconColor.test.ts`**：

- 同一 title 返回同一颜色（稳定性）
- 不同 title 在调色板内分布（不全部撞同一格）
- 首字提取覆盖：中文 / 英文 / emoji / 纯空白 / 空串 / 前导标点（6+ 用例）

**`SideAssistantTabBar.test.tsx`**：

- 窄态：渲染图标 + 首字，不渲染标题文字
- 窄态：4 种状态的 tab 各自渲染对应颜色的状态点
- 宽态：渲染图标 + 标题 + 副标题
- 选中态描边：active 有 ring，非 active 无
- 点击图标 → 调用 `onActivate`
- 中键点击图标 → 调用 `onClose`
- 悬停 ✕：用 hover 状态下的 className 断言（避免 jsdom 浮层模拟）

**`SideAssistantPanel.test.tsx`**（如已有，扩展）：

- localStorage 持久化：toggle ⇄ 后 `ai_sidebar_rail_collapsed` 写入正确
- 宽态下拖拽改宽度，`ai_sidebar_rail_width` 持久化
- `tabs.length === 0` 时整个 rail 不渲染

### 手动验证清单（`make dev`）

1. 默认打开 AI 助手 → rail 36px、对话区明显变宽
2. 开 4 个会话，颜色分散，状态点显示正确
3. 点 ⇄ → 展开宽态，标题完整可见，再点 ⇄ → 收回，状态被记住
4. 宽态左边拖拽改宽，重启 app 后宽度被记住
5. 中键点会话图标 → 关闭
6. 关闭最后一个会话 → rail 整体消失，`emptyGuide` 居中显示
7. 中文 / 英文 / emoji 首字、blank session "?" 都正确
8. 黑/白主题下颜色都不糊（调色板要在两个主题都过得去）
9. 键盘 Tab 能到每个图标，Enter 激活，aria-label 在屏幕阅读器里读得出

### 回归点（不该坏的）

- 历史下拉、新建、提升到主 Tab、面板自身的拖宽与折叠都保持不变
- aiStore 行为不变（包括 `closeSidebarTab` 中的运行中确认逻辑）

## 不在本次范围

- 用户可手动改会话图标 / emoji（YAGNI，等反馈再加）
- 关联资产图标（fallback 路径，等反馈）
- 键盘快捷键（Cmd+1/2/3 切会话）
- 拖拽重排会话顺序

## 开放风险

- **首字撞色**：4 个 SSH 会话首字都是 "S"。当前依赖背景色哈希区分（极大概率不同色），仍可能两个 hash 撞到相邻色 → 接受，靠 hover tooltip 兜底；后续可上手动改 emoji
- **OKLCH 浏览器兼容**：Wails 内嵌 Webkit 已支持，桌面端不是问题
- **Radix Tooltip 与现有 portal 的层级**：现在的"历史下拉"也用 portal，需在实施时确认 z-index 不打架
