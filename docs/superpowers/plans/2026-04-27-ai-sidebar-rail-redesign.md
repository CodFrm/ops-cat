# AI 侧边助手会话 Rail 双态重设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把现有 128–176px 的 AI 侧边会话标签栏改造为默认 36px 的图标 rail（带状态点 + 标题首字 + 哈希配色），新增 ⇄ 按钮一键切到宽态（120–220px 可拖），状态持久化。

**Architecture:** 纯前端 UI 重构。新增一个无副作用的 `sessionIconColor` 工具模块负责"标题→首字"和"标题→颜色"，重写 `SideAssistantTabBar` 同时支持 `collapsed` / `expanded` 两种渲染分支，`SideAssistantPanel` 新增两个独立的 localStorage 键（折叠状态 + 宽态宽度）。`aiStore` 数据结构不动。

**Tech Stack:** React 19 + TypeScript + Vitest + Tailwind CSS 4 + `@opskat/ui`（封装的 Radix Tooltip / Button / `useResizeHandle`）。

**Spec:** `docs/superpowers/specs/2026-04-27-ai-sidebar-rail-redesign-design.md`

---

## Task 1: `sessionIconColor` 工具 — 首字提取（TDD）

**Files:**
- Create: `frontend/src/components/ai/sessionIconColor.ts`
- Test: `frontend/src/components/ai/__tests__/sessionIconColor.test.ts`

- [ ] **Step 1：写失败测试 — 首字提取**

```ts
// frontend/src/components/ai/__tests__/sessionIconColor.test.ts
import { describe, it, expect } from "vitest";
import { getSessionIconLetter } from "../sessionIconColor";

describe("getSessionIconLetter", () => {
  it("returns the first Chinese character", () => {
    expect(getSessionIconLetter("写迁移")).toBe("写");
  });

  it("uppercases the first ASCII letter", () => {
    expect(getSessionIconLetter("ssh debug")).toBe("S");
  });

  it("trims leading whitespace before extracting", () => {
    expect(getSessionIconLetter("  ssh")).toBe("S");
  });

  it("preserves a leading emoji as-is", () => {
    expect(getSessionIconLetter("🐛 调研")).toBe("🐛");
  });

  it("returns '?' for an empty string", () => {
    expect(getSessionIconLetter("")).toBe("?");
  });

  it("returns '?' for whitespace-only input", () => {
    expect(getSessionIconLetter("   ")).toBe("?");
  });

  it("strips leading ASCII punctuation, then uppercases", () => {
    expect(getSessionIconLetter("@user")).toBe("U");
    expect(getSessionIconLetter("--draft")).toBe("D");
  });
});
```

- [ ] **Step 2：跑测试，确认失败**

Run: `cd frontend && pnpm test -- sessionIconColor`
Expected: FAIL — `Cannot find module '../sessionIconColor'`

- [ ] **Step 3：实现首字提取**

```ts
// frontend/src/components/ai/sessionIconColor.ts

const LEADING_PUNCT_RE = /^[!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]+/u;
const ASCII_LETTER_RE = /^[a-zA-Z]$/;

export function getSessionIconLetter(title: string): string {
  const trimmed = title.trim().replace(LEADING_PUNCT_RE, "");
  if (!trimmed) return "?";
  // Array.from 正确处理代理对（emoji），取真正的第一个 code point
  const first = Array.from(trimmed)[0];
  if (!first) return "?";
  if (ASCII_LETTER_RE.test(first)) return first.toUpperCase();
  return first;
}
```

- [ ] **Step 4：跑测试，确认通过**

Run: `cd frontend && pnpm test -- sessionIconColor`
Expected: PASS — 7 tests passing

- [ ] **Step 5：commit**

```bash
git add frontend/src/components/ai/sessionIconColor.ts frontend/src/components/ai/__tests__/sessionIconColor.test.ts
git commit -m "✨ AI rail：新增 getSessionIconLetter 工具与单元测试"
```

---

## Task 2: `sessionIconColor` 工具 — 颜色调色板（TDD）

**Files:**
- Modify: `frontend/src/components/ai/sessionIconColor.ts`
- Modify: `frontend/src/components/ai/__tests__/sessionIconColor.test.ts`

- [ ] **Step 1：追加颜色测试**

```ts
// 追加到 frontend/src/components/ai/__tests__/sessionIconColor.test.ts
import { getSessionIconColor } from "../sessionIconColor";

describe("getSessionIconColor", () => {
  it("returns the same color for the same title", () => {
    expect(getSessionIconColor("写迁移")).toEqual(getSessionIconColor("写迁移"));
  });

  it("returns an object with bg and fg strings", () => {
    const c = getSessionIconColor("hello");
    expect(typeof c.bg).toBe("string");
    expect(typeof c.fg).toBe("string");
    expect(c.bg.length).toBeGreaterThan(0);
    expect(c.fg.length).toBeGreaterThan(0);
  });

  it("distributes different titles across the palette", () => {
    const titles = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j"];
    const bgSet = new Set(titles.map((t) => getSessionIconColor(t).bg));
    // At least 4 distinct buckets out of 10 distinct inputs
    expect(bgSet.size).toBeGreaterThanOrEqual(4);
  });

  it("returns a valid color for empty title (falls back to a default bucket)", () => {
    const c = getSessionIconColor("");
    expect(c.bg.length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2：跑测试，确认失败**

Run: `cd frontend && pnpm test -- sessionIconColor`
Expected: FAIL — `getSessionIconColor is not exported`

- [ ] **Step 3：实现颜色函数**

```ts
// 追加到 frontend/src/components/ai/sessionIconColor.ts

export interface SessionIconColor {
  bg: string;
  fg: string;
}

// 8 色固定调色板，OKLCH 写法 —— Wails 内嵌 Webkit 已支持。
// fg 统一白色（与 ~0.55-0.68 亮度的 bg 对比足够），简化心智负担。
const PALETTE: SessionIconColor[] = [
  { bg: "oklch(0.55 0.18 264)", fg: "#ffffff" }, // indigo
  { bg: "oklch(0.62 0.18 28)", fg: "#ffffff" },  // red-orange
  { bg: "oklch(0.62 0.16 145)", fg: "#ffffff" }, // emerald
  { bg: "oklch(0.65 0.18 65)", fg: "#1a1a1a" },  // amber (深字配浅底)
  { bg: "oklch(0.58 0.20 305)", fg: "#ffffff" }, // violet
  { bg: "oklch(0.62 0.15 215)", fg: "#ffffff" }, // sky
  { bg: "oklch(0.60 0.18 350)", fg: "#ffffff" }, // pink
  { bg: "oklch(0.55 0.16 95)", fg: "#ffffff" },  // olive
];

function hash(s: string): number {
  // djb2 变体，纯函数稳定
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  }
  return h >>> 0;
}

export function getSessionIconColor(title: string): SessionIconColor {
  const idx = hash(title) % PALETTE.length;
  return PALETTE[idx];
}
```

- [ ] **Step 4：跑测试，确认通过**

Run: `cd frontend && pnpm test -- sessionIconColor`
Expected: PASS — 11 tests passing total

- [ ] **Step 5：commit**

```bash
git add frontend/src/components/ai/sessionIconColor.ts frontend/src/components/ai/__tests__/sessionIconColor.test.ts
git commit -m "✨ AI rail：新增 getSessionIconColor 哈希调色板"
```

---

## Task 3: i18n key 增删

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`

- [ ] **Step 1：编辑 zh-CN — 在 `ai.sidebar` 下增删**

删除 key（这一行整行删掉）：
```json
"sessions": "会话",
```

新增 key（追加到 `ai.sidebar` 内、`status` 字段之前）：
```json
"expandRail": "展开会话栏",
"collapseRail": "收起会话栏",
"statusSuffix": {
  "waiting_approval": "待审批",
  "running": "运行中",
  "done": "已完成",
  "error": "出错"
},
```

- [ ] **Step 2：编辑 en — 同步增删**

删除：
```json
"sessions": "Sessions",
```

新增：
```json
"expandRail": "Expand session rail",
"collapseRail": "Collapse session rail",
"statusSuffix": {
  "waiting_approval": "Waiting approval",
  "running": "Running",
  "done": "Done",
  "error": "Error"
},
```

- [ ] **Step 3：验证 JSON 合法**

Run:
```bash
python3 -c "import json; json.load(open('frontend/src/i18n/locales/zh-CN/common.json')); json.load(open('frontend/src/i18n/locales/en/common.json')); print('ok')"
```
Expected: `ok`

- [ ] **Step 4：commit**

```bash
git add frontend/src/i18n/locales/zh-CN/common.json frontend/src/i18n/locales/en/common.json
git commit -m "🌐 AI rail：新增 expandRail / statusSuffix 翻译，移除 sessions 标题"
```

---

## Task 4: 重写 `SideAssistantTabBar` — props 与窄态骨架（TDD）

**Files:**
- Modify (rewrite): `frontend/src/components/ai/SideAssistantTabBar.tsx`
- Create: `frontend/src/components/ai/__tests__/SideAssistantTabBar.test.tsx`

- [ ] **Step 1：写失败测试 — 窄态渲染图标 + 不渲染标题文字**

```tsx
// frontend/src/components/ai/__tests__/SideAssistantTabBar.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { SideAssistantTabBar } from "../SideAssistantTabBar";
import type { SidebarAITab } from "@/stores/aiStore";

const baseProps = {
  width: 150,
  collapsed: true,
  activeTabId: "t1",
  getStatus: () => null,
  onActivate: vi.fn(),
  onClose: vi.fn(),
  onNewChat: vi.fn(),
  onToggleCollapsed: vi.fn(),
};

const tabs: SidebarAITab[] = [
  { id: "t1", title: "写迁移", conversationId: 1 } as SidebarAITab,
  { id: "t2", title: "查日志", conversationId: 2 } as SidebarAITab,
];

describe("SideAssistantTabBar (collapsed)", () => {
  it("renders one icon button per tab with the title's first character", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    expect(screen.getByText("写")).toBeInTheDocument();
    expect(screen.getByText("查")).toBeInTheDocument();
  });

  it("does not render the full title text in collapsed mode", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    // 完整标题不应直接渲染（仅 aria-label / tooltip 里有）
    expect(screen.queryByText("写迁移")).not.toBeInTheDocument();
  });

  it("exposes the full title via aria-label", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    expect(screen.getByLabelText(/写迁移/)).toBeInTheDocument();
  });

  it("calls onActivate when an icon is clicked", () => {
    const onActivate = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onActivate={onActivate} />);
    screen.getByLabelText(/查日志/).click();
    expect(onActivate).toHaveBeenCalledWith("t2");
  });
});
```

- [ ] **Step 2：跑测试，确认失败**

Run: `cd frontend && pnpm test -- SideAssistantTabBar`
Expected: FAIL — 旧组件没有新 props，断言失败

- [ ] **Step 3：重写组件 — 新 props + 窄态骨架**

完整替换 `frontend/src/components/ai/SideAssistantTabBar.tsx` 为：

```tsx
import { LoaderCircle } from "lucide-react";
import { cn, Button, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger, useResizeHandle } from "@opskat/ui";
import { useTranslation } from "react-i18next";
import type { SidebarAITab, SidebarTabStatus } from "@/stores/aiStore";
import { getSessionIconColor, getSessionIconLetter } from "./sessionIconColor";
import { ChevronsRight, ChevronsLeft, Plus, X } from "lucide-react";

interface SideAssistantTabBarProps {
  tabs: SidebarAITab[];
  activeTabId: string | null;
  getStatus: (tabId: string) => SidebarTabStatus;
  collapsed: boolean;
  width: number;
  onActivate: (tabId: string) => void;
  onClose: (tabId: string) => void;
  onNewChat: () => void;
  onToggleCollapsed: () => void;
  onResize?: (width: number) => void;
}

const statusDotColor: Record<Exclude<SidebarTabStatus, null>, string> = {
  waiting_approval: "bg-amber-500",
  running: "bg-sky-500",
  done: "bg-emerald-500",
  error: "bg-rose-500",
};

export function SideAssistantTabBar({
  tabs,
  activeTabId,
  getStatus,
  collapsed,
  onActivate,
  onClose,
  onNewChat,
  onToggleCollapsed,
}: SideAssistantTabBarProps) {
  const { t } = useTranslation();

  return (
    <TooltipProvider delayDuration={300}>
      <div
        className="flex h-full flex-col"
        role="tablist"
        aria-orientation="vertical"
        aria-label="ai-sessions"
      >
        {/* 顶部按钮组：⇄ + ＋ */}
        <div
          className={cn(
            "flex shrink-0 items-center gap-1 border-b border-panel-divider/70",
            collapsed ? "flex-col py-2" : "flex-row px-2 py-1.5"
          )}
        >
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 rounded-md text-muted-foreground/80"
            onClick={onToggleCollapsed}
            title={collapsed ? t("ai.sidebar.expandRail") : t("ai.sidebar.collapseRail")}
            aria-label={collapsed ? t("ai.sidebar.expandRail") : t("ai.sidebar.collapseRail")}
          >
            {collapsed ? <ChevronsLeft className="h-3.5 w-3.5" /> : <ChevronsRight className="h-3.5 w-3.5" />}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 rounded-md text-muted-foreground/80"
            onClick={onNewChat}
            title={t("ai.sidebar.newChat")}
            aria-label={t("ai.sidebar.newChat")}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
        </div>

        {/* 列表区 */}
        <div
          className={cn(
            "min-h-0 flex-1 overflow-y-auto",
            collapsed ? "flex flex-col items-center gap-2 py-2" : "px-2 py-2 space-y-1"
          )}
        >
          {tabs.map((tab) => {
            const isActive = tab.id === activeTabId;
            const status = getStatus(tab.id);
            const titleText = tab.title || t("ai.newConversation");
            const isBlank = tab.conversationId == null;
            const letter = isBlank ? "?" : getSessionIconLetter(titleText);
            const color = isBlank ? { bg: "transparent", fg: "currentColor" } : getSessionIconColor(titleText);
            const statusSuffix = status ? ` · ${t(`ai.sidebar.statusSuffix.${status}`)}` : "";

            const handleAuxClick = (e: React.MouseEvent) => {
              if (e.button === 1) {
                e.preventDefault();
                onClose(tab.id);
              }
            };

            if (collapsed) {
              return (
                <Tooltip key={tab.id}>
                  <TooltipTrigger asChild>
                    <div className="group relative">
                      <button
                        type="button"
                        role="tab"
                        aria-selected={isActive}
                        aria-label={titleText + statusSuffix}
                        onClick={() => onActivate(tab.id)}
                        onAuxClick={handleAuxClick}
                        className={cn(
                          "relative flex h-7 w-7 items-center justify-center rounded-md text-xs font-bold transition-transform hover:scale-105",
                          isActive && "ring-2 ring-primary ring-offset-1 ring-offset-sidebar",
                          isBlank && "border border-dashed border-muted-foreground/40 text-muted-foreground/70"
                        )}
                        style={isBlank ? undefined : { background: color.bg, color: color.fg }}
                      >
                        {letter}
                        {status && (
                          <span
                            className={cn(
                              "absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full ring-2 ring-sidebar",
                              statusDotColor[status]
                            )}
                            aria-hidden="true"
                          />
                        )}
                      </button>
                      {/* 兄弟节点的关闭按钮，避免 button-in-button 嵌套 */}
                      <button
                        type="button"
                        aria-label={t("tab.close")}
                        title={t("tab.close")}
                        onClick={(e) => {
                          e.stopPropagation();
                          onClose(tab.id);
                        }}
                        className="absolute -top-1 -right-1 hidden h-3.5 w-3.5 items-center justify-center rounded-full border-2 border-sidebar bg-muted text-muted-foreground hover:bg-foreground hover:text-background group-hover:flex"
                      >
                        <X className="h-2 w-2" strokeWidth={3} />
                      </button>
                    </div>
                  </TooltipTrigger>
                  <TooltipContent side="left">{titleText + statusSuffix}</TooltipContent>
                </Tooltip>
              );
            }

            // 宽态：图标 + 标题 + 副标题
            return (
              <div
                key={tab.id}
                className={cn(
                  "group relative min-w-0 overflow-hidden rounded-lg text-xs transition-colors",
                  isActive ? "bg-background/95 text-foreground" : "bg-transparent text-muted-foreground hover:bg-background/45"
                )}
              >
                <span
                  className={cn(
                    "absolute bottom-2 left-0 top-2 w-px rounded-full",
                    isActive ? "bg-primary/65" : "bg-transparent"
                  )}
                />
                <button
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  aria-label={titleText + statusSuffix}
                  onClick={() => onActivate(tab.id)}
                  onAuxClick={handleAuxClick}
                  className="flex w-full min-w-0 items-center gap-2 rounded-[inherit] py-1.5 pl-2 pr-8 text-left"
                >
                  <span
                    className={cn(
                      "relative flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-xs font-bold",
                      isBlank && "border border-dashed border-muted-foreground/40 text-muted-foreground/70"
                    )}
                    style={isBlank ? undefined : { background: color.bg, color: color.fg }}
                  >
                    {letter}
                    {status === "running" ? (
                      <LoaderCircle className="absolute -bottom-1 -right-1 h-3 w-3 animate-spin text-sky-500" />
                    ) : status ? (
                      <span
                        className={cn(
                          "absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full ring-2 ring-sidebar",
                          statusDotColor[status]
                        )}
                      />
                    ) : null}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium leading-5 text-[11px] text-foreground/92">
                      {titleText}
                    </span>
                    {(status || isBlank) && (
                      <span className="block truncate text-[10px] leading-4 text-muted-foreground/80">
                        {isBlank ? t("ai.sidebar.newChat") : t(`ai.sidebar.status.${status}`)}
                      </span>
                    )}
                  </span>
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className={cn(
                    "absolute right-1.5 top-1/2 h-5 w-5 shrink-0 -translate-y-1/2 rounded-md text-muted-foreground/70 opacity-0 transition-opacity hover:opacity-100 group-hover:opacity-70"
                  )}
                  onClick={(e) => {
                    e.stopPropagation();
                    onClose(tab.id);
                  }}
                  title={t("tab.close")}
                  aria-label={t("tab.close")}
                >
                  <X className="h-3 w-3" />
                </Button>
              </div>
            );
          })}
        </div>
      </div>
    </TooltipProvider>
  );
}
```

- [ ] **Step 4：跑测试，确认通过**

Run: `cd frontend && pnpm test -- SideAssistantTabBar`
Expected: PASS — 4 tests passing

- [ ] **Step 5：commit**

```bash
git add frontend/src/components/ai/SideAssistantTabBar.tsx frontend/src/components/ai/__tests__/SideAssistantTabBar.test.tsx
git commit -m "♻️ AI rail：重写 TabBar 为窄态图标 + 宽态详细列表"
```

---

## Task 5: TabBar 单测 — 状态点 + active 描边 + 中键关闭 + 顶部按钮

**Files:**
- Modify: `frontend/src/components/ai/__tests__/SideAssistantTabBar.test.tsx`

- [ ] **Step 1：追加 5 个测试**

在原 `describe("SideAssistantTabBar (collapsed)", ...)` 末尾追加：

```tsx
  it("renders the active tab with a ring marker", () => {
    const { container } = render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    const active = container.querySelector('[aria-selected="true"]');
    expect(active?.className).toMatch(/ring-2/);
  });

  it("renders a status dot when getStatus returns a non-null status", () => {
    const { container } = render(
      <SideAssistantTabBar
        {...baseProps}
        tabs={tabs}
        getStatus={(id) => (id === "t1" ? "running" : id === "t2" ? "error" : null)}
      />
    );
    expect(container.querySelector(".bg-sky-500")).toBeTruthy();
    expect(container.querySelector(".bg-rose-500")).toBeTruthy();
  });

  it("calls onClose on middle-button click (auxClick)", () => {
    const onClose = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onClose={onClose} />);
    const target = screen.getByLabelText(/查日志/);
    target.dispatchEvent(new MouseEvent("auxclick", { button: 1, bubbles: true }));
    expect(onClose).toHaveBeenCalledWith("t2");
  });

  it("calls onToggleCollapsed when ⇄ button is clicked", () => {
    const onToggleCollapsed = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onToggleCollapsed={onToggleCollapsed} />);
    screen.getByLabelText("ai.sidebar.expandRail").click();
    expect(onToggleCollapsed).toHaveBeenCalledTimes(1);
  });

  it("calls onNewChat when ＋ button is clicked", () => {
    const onNewChat = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onNewChat={onNewChat} />);
    screen.getByLabelText("ai.sidebar.newChat").click();
    expect(onNewChat).toHaveBeenCalledTimes(1);
  });
```

并追加宽态的 describe block：

```tsx
describe("SideAssistantTabBar (expanded)", () => {
  it("renders the full title text in expanded mode", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} collapsed={false} />);
    expect(screen.getByText("写迁移")).toBeInTheDocument();
    expect(screen.getByText("查日志")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2：跑测试**

Run: `cd frontend && pnpm test -- SideAssistantTabBar`
Expected: PASS — 10 tests passing total

- [ ] **Step 3：commit**

```bash
git add frontend/src/components/ai/__tests__/SideAssistantTabBar.test.tsx
git commit -m "✅ AI rail：补充 TabBar 状态点/中键关闭/顶部按钮单测"
```

---

## Task 6: 接入 `SideAssistantPanel` — 折叠状态 + 宽度状态

**Files:**
- Modify: `frontend/src/components/ai/SideAssistantPanel.tsx`

- [ ] **Step 1：替换 imports + 状态 + 渲染**

在文件顶部 imports 区追加：
```tsx
import { useState } from "react"; // 已有 useRef/useState/useEffect 的话保留
```

在组件函数体里，替换 `width` / `isCompactSessionRail` / `sessionRailWidth` 三处计算（`SideAssistantPanel.tsx:41-55` 附近）为：

```tsx
const panelRef = useRef<HTMLDivElement>(null);
const railRef = useRef<HTMLDivElement>(null);
const {
  size: width,
  isResizing: resizing,
  handleMouseDown: handleResizeStart,
} = useResizeHandle({
  defaultSize: 360,
  minSize: 280,
  maxSize: 520,
  reverse: true,
  storageKey: "ai_sidebar_width",
  targetRef: panelRef,
});

// rail 折叠状态独立持久化
const [railCollapsed, setRailCollapsed] = useState<boolean>(() => {
  const v = localStorage.getItem("ai_sidebar_rail_collapsed");
  // 默认窄态（true）
  return v === null ? true : v === "true";
});
const toggleRailCollapsed = () => {
  setRailCollapsed((prev) => {
    const next = !prev;
    localStorage.setItem("ai_sidebar_rail_collapsed", String(next));
    return next;
  });
};

// 宽态 rail 自身的宽度（仅 !railCollapsed 时启用拖拽）
const {
  size: railExpandedWidth,
  isResizing: railResizing,
  handleMouseDown: handleRailResizeStart,
} = useResizeHandle({
  defaultSize: 150,
  minSize: 120,
  maxSize: 220,
  reverse: true, // rail 在右侧、从左缘拖拽：往左拖 → rail 变宽（向 origin 方向变大），与面板自身一致
  storageKey: "ai_sidebar_rail_width",
  targetRef: railRef,
});

const railRenderWidth = railCollapsed ? 36 : railExpandedWidth;
```

- [ ] **Step 2：替换 rail JSX**

定位到 `SideAssistantPanel.tsx:184-199` 的 `<aside ...><SideAssistantTabBar .../></aside>`，替换为：

```tsx
{sidebarTabs.length > 0 && (
  <aside
    ref={railRef}
    className="relative min-h-0 shrink-0 border-l border-panel-divider/70 bg-sidebar/65"
    style={{ width: railRenderWidth }}
    data-ai-session-rail="right"
  >
    {!railCollapsed && (
      <div
        className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
        onMouseDown={handleRailResizeStart}
      />
    )}
    {railResizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}
    <SideAssistantTabBar
      tabs={sidebarTabs}
      activeTabId={activeSidebarTabId}
      getStatus={getSidebarTabStatus}
      collapsed={railCollapsed}
      width={railExpandedWidth}
      onActivate={activateSidebarTab}
      onClose={closeSidebarTab}
      onNewChat={handleNewChat}
      onToggleCollapsed={toggleRailCollapsed}
    />
  </aside>
)}
```

- [ ] **Step 3：跑现有 frontend 测试，确认没坏**

Run: `cd frontend && pnpm test`
Expected: 全部测试 PASS（包括新加的 sessionIconColor 11 + TabBar 10 + 现有的 aiStore / layoutStore / 等）

- [ ] **Step 4：跑 lint**

Run: `cd frontend && pnpm lint`
Expected: 0 errors（如有 unused imports 类的小 warning 修掉再继续）

- [ ] **Step 5：commit**

```bash
git add frontend/src/components/ai/SideAssistantPanel.tsx
git commit -m "✨ AI rail：Panel 接入 rail 折叠状态 + 宽态拖拽，删除 sessionRailWidth"
```

---

## Task 7: SideAssistantPanel localStorage 持久化测试

**Files:**
- Create: `frontend/src/components/ai/__tests__/SideAssistantPanel.persist.test.tsx`

- [ ] **Step 1：写测试**

```tsx
// frontend/src/components/ai/__tests__/SideAssistantPanel.persist.test.tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SideAssistantPanel } from "../SideAssistantPanel";

// 简单做法：mock 掉 useAIStore 返回最少必要值，让 Panel 至少能渲染出 rail
import { useAIStore } from "@/stores/aiStore";

vi.mock("@/stores/aiStore", () => ({
  useAIStore: vi.fn(),
}));

const mockedUseAIStore = vi.mocked(useAIStore);

describe("SideAssistantPanel rail persistence", () => {
  beforeEach(() => {
    localStorage.clear();
    mockedUseAIStore.mockReturnValue({
      sidebarTabs: [{ id: "t1", title: "写迁移", conversationId: 1 }],
      activeSidebarTabId: "t1",
      configured: true,
      fetchConversations: vi.fn(),
      getSidebarTabStatus: () => null,
      openNewSidebarTab: vi.fn(),
      bindSidebarTabToConversation: vi.fn(),
      openSidebarConversationInSidebar: vi.fn(),
      activateSidebarTab: vi.fn(),
      closeSidebarTab: vi.fn(),
      promoteSidebarToTab: vi.fn(),
      sendFromSidebarTab: vi.fn(),
      stopSidebarTab: vi.fn(),
    } as unknown as ReturnType<typeof useAIStore>);
  });

  it("defaults to collapsed (rail-collapsed key empty → true)", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    // 找展开按钮 aria-label = "ai.sidebar.expandRail" 说明当前是 collapsed
    expect(screen.getByLabelText("ai.sidebar.expandRail")).toBeInTheDocument();
  });

  it("persists rail-collapsed flip to localStorage on toggle", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    fireEvent.click(screen.getByLabelText("ai.sidebar.expandRail"));
    expect(localStorage.getItem("ai_sidebar_rail_collapsed")).toBe("false");
  });

  it("reads rail-collapsed = false from localStorage on mount", () => {
    localStorage.setItem("ai_sidebar_rail_collapsed", "false");
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    // 当前是 expanded 状态，按钮应该是 collapseRail（收起）
    expect(screen.getByLabelText("ai.sidebar.collapseRail")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2：跑测试**

Run: `cd frontend && pnpm test -- SideAssistantPanel.persist`
Expected: PASS — 3 tests passing

- [ ] **Step 3：commit**

```bash
git add frontend/src/components/ai/__tests__/SideAssistantPanel.persist.test.tsx
git commit -m "✅ AI rail：Panel rail 折叠状态持久化测试"
```

---

## Task 8: 全量测试 + lint + 手动验证

**Files:** 无（仅运行验证）

- [ ] **Step 1：跑全部 frontend 测试**

Run: `cd frontend && pnpm test`
Expected: 全部 PASS

- [ ] **Step 2：跑 lint**

Run: `cd frontend && pnpm lint`
Expected: 0 errors

- [ ] **Step 3：build 验证**

Run: `cd frontend && pnpm build`
Expected: build 成功，无类型错误

- [ ] **Step 4：手动验证（make dev）**

启动：
```bash
make dev
```

按以下清单逐项验证（spec §测试策略 - 手动验证清单）：

1. 默认打开 AI 助手 → rail 36px、对话区明显变宽
2. 开 4 个会话，颜色分散，状态点显示正确（蓝/绿/红/橙）
3. 点 ⇄ → rail 展开成 ~150px，标题完整可见；再点 ⇄ → 收回；reload 后状态保留
4. 宽态时左侧拖拽改宽（120-220px 钳制），reload 后宽度保留
5. 中键点会话图标 → 关闭
6. 关闭最后一个会话 → rail 整体消失，emptyGuide 居中显示
7. 中文 / 英文 / emoji 首字、blank session（新建未发送）显示 "?" 都正确
8. 黑/白主题切换 → 8 色调色板都不糊
9. 键盘 Tab 能到每个图标，Enter 激活
10. 历史下拉、新建（顶部 header 的＋）、提升到主 Tab、面板自身拖宽与折叠 — 这些**未改动**功能仍然正常工作

- [ ] **Step 5：（可选）若手动验证发现需要微调**

仅做 spec 内允许的细调（颜色亮度、间距、tooltip 文案），改完后单独 commit：
```bash
git commit -m "🎨 AI rail：手动验证后的视觉微调"
```

如果发现 spec 没覆盖的问题（比如 z-index 冲突），不要在这里改，停下来反馈给上层。

- [ ] **Step 6：最终 commit（如果之前 step 没全 commit）**

```bash
git status  # 应该是 clean
```

---

## 计划自检结果

**Spec 覆盖：**

| Spec 节 | 实现位置 |
|---|---|
| §1 总览 — 双态 rail | Task 4–6 |
| §2 sessionIconColor | Task 1–2 |
| §2 SideAssistantTabBar 重写 | Task 4 |
| §2 SideAssistantPanel 调整 | Task 6 |
| §2 i18n key 增删 | Task 3 |
| §3 三个 localStorage key | Task 6（rail_collapsed/rail_width） + 已存在 ai_sidebar_width |
| §3 状态点颜色映射 | Task 4（statusDotColor） |
| §4 首字提取 6 类输入 | Task 1（7 用例） |
| §4 Tooltip + a11y | Task 4（TooltipProvider + aria-label） |
| §4 Resize 钳制（80/220） | Task 6（minSize:120 maxSize:220；< 80 自动吸附为窄态见 spec §4，本次实现取 120 下限作为软吸附效果——若要硬吸附见"开放风险"） |
| §4 中键关闭 | Task 4（onAuxClick） + Task 5 测试 |
| §5 单元测试 | Task 1, 2, 4, 5, 7 |
| §5 手动验证清单 | Task 8 |

**已知偏差：**

- **<80px 软吸附为窄态**：spec §4 写"宽态拖到 < 80px → 强制吸附为窄态"。本次 Task 6 用 `useResizeHandle` 的 `minSize: 120` 直接钳制下限为 120px，不允许拖到 80 以下。等价效果（用户拖不窄）但少了"自动切窄态"的好心。如果用户实测想要吸附效果再加；YAGNI 起见先不实现。需要时在 `useResizeHandle` 的 `onResizeEnd` 里加 `if (size < 120 + 20) toggleRailCollapsed()` 即可。

**未实现（spec 明确不在范围）：**

- 用户手动改 emoji / 关联资产图标
- 拖拽重排会话顺序
- 键盘快捷键

---

Plan complete and saved to `docs/superpowers/plans/2026-04-27-ai-sidebar-rail-redesign.md`. Two execution options:

1. **Subagent-Driven (recommended)** - 每个 task 派一个新的 subagent，task 间做 review，迭代快
2. **Inline Execution** - 在当前 session 里执行，分批 checkpoint

Which approach?
