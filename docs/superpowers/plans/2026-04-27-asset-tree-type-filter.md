# Asset Tree Type Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move asset type filtering from sidebar buttons into an in-tree multi-select popover that supports extension asset types, and restore PR #37's pre-existing "AssetTree visibility is explicit-only" behavior.

**Architecture:** The 4 sidebar type buttons are removed. AssetTree owns its own multi-select filter state (persisted to localStorage). A new `getAssetTypeOptions()` helper merges built-in types and `useExtensionStore.extensions[].manifest.assetTypes` into a single option list. The `homeSection` cross-cutting state, `normalizeAssetSection`, `tabBelongsToSection`, and `hideAssetListAfterConnect` are deleted; `App.tsx` and `SideTabList` decouple from the section concept.

**Tech Stack:** React 19, TypeScript, Zustand 5, shadcn/ui (Radix Popover), lucide-react icons, i18next, Vitest + Testing Library.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `frontend/src/lib/assetTypes/options.ts` | CREATE | `AssetTypeOption` type, `getAssetTypeOptions()`, `matchSelectedTypes()` |
| `frontend/src/lib/assetTypes/index.ts` | MODIFY | delete `HomeSection`, `normalizeAssetSection`; re-export from `options.ts` |
| `frontend/src/lib/tabSection.ts` | DELETE | dead code after section concept removed |
| `frontend/src/components/asset/AssetTypeFilterButton.tsx` | CREATE | funnel button + popover, takes `selectedTypes` + `onChange` |
| `frontend/src/components/layout/AssetTree.tsx` | MODIFY | drop `homeSection` prop; add internal filter state, persistence, FilterButton, type-aware filter logic |
| `frontend/src/components/layout/Sidebar.tsx` | MODIFY | drop second navGroup (4 type buttons), prune unused lucide imports |
| `frontend/src/components/layout/SideTabList.tsx` | MODIFY | drop `homeSection` prop and `tabBelongsToSection` usage |
| `frontend/src/App.tsx` | MODIFY | remove `homeSection` state, `hideAssetListAfterConnect`, section-driven branches in `handlePageChange`; restore pre-#37 simple form |
| `frontend/src/i18n/locales/zh-CN/common.json` | MODIFY | add `asset.filterByType*`, `asset.filterAllTypes`, `asset.filterExtensions` |
| `frontend/src/i18n/locales/en/common.json` | MODIFY | mirror zh-CN keys |
| `frontend/src/__tests__/assetTypeOptions.test.ts` | CREATE | unit tests for built-in + extension merging + `matchSelectedTypes` |
| `frontend/src/__tests__/AssetTreeTypeFilter.test.tsx` | CREATE | filter button toggle, popover behavior, last-item fallback, extension type listed |
| `frontend/src/__tests__/AssetTree.test.tsx` | MODIFY | drop any `homeSection`-related expectations (currently doesn't reference it directly — verify) |

---

## Task 1: Asset type options module — failing test

**Files:**
- Test: `frontend/src/__tests__/assetTypeOptions.test.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/__tests__/assetTypeOptions.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import { getAssetTypeOptions, matchSelectedTypes } from "@/lib/assetTypes/options";
import { asset_entity } from "../../wailsjs/go/models";

describe("getAssetTypeOptions", () => {
  it("returns built-in options when extensions registry is empty", () => {
    const opts = getAssetTypeOptions({});
    const values = opts.map((o) => o.value);
    expect(values).toEqual(["ssh", "database", "redis", "mongodb"]);
    expect(opts.every((o) => o.group === "builtin")).toBe(true);
  });

  it("aliases on database include mysql, postgresql, database", () => {
    const opts = getAssetTypeOptions({});
    const db = opts.find((o) => o.value === "database")!;
    expect(new Set(db.aliases)).toEqual(new Set(["database", "mysql", "postgresql"]));
  });

  it("merges extension assetTypes after built-ins", () => {
    const extensions = {
      k8sExt: {
        manifest: {
          name: "k8sExt",
          version: "1.0.0",
          icon: "Server",
          i18n: { displayName: "Kubernetes", description: "" },
          assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
        },
      },
    };
    const opts = getAssetTypeOptions(extensions as never);
    const ext = opts.find((o) => o.value === "kubernetes");
    expect(ext).toBeTruthy();
    expect(ext!.group).toBe("extension");
    expect(ext!.label).toBe("Kubernetes");
    expect(ext!.iconName).toBe("Server");
  });

  it("ignores extensions without assetTypes", () => {
    const extensions = {
      otherExt: {
        manifest: {
          name: "otherExt",
          version: "1.0.0",
          icon: "Box",
          i18n: { displayName: "Other", description: "" },
        },
      },
    };
    const opts = getAssetTypeOptions(extensions as never);
    expect(opts.filter((o) => o.group === "extension")).toEqual([]);
  });
});

describe("matchSelectedTypes", () => {
  const a = (id: number, type: string) => new asset_entity.Asset({ ID: id, Name: `n${id}`, Type: type });
  const assets = [a(1, "ssh"), a(2, "mysql"), a(3, "postgresql"), a(4, "redis"), a(5, "kubernetes")];
  const opts = getAssetTypeOptions({
    k8sExt: {
      manifest: {
        name: "k8sExt",
        version: "1",
        icon: "Server",
        i18n: { displayName: "Kubernetes", description: "" },
        assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
      },
    },
  } as never);

  it('returns all assets when selection is "all"', () => {
    expect(matchSelectedTypes(assets, "all", opts).map((x) => x.ID)).toEqual([1, 2, 3, 4, 5]);
  });

  it("matches database aliases (mysql, postgresql)", () => {
    expect(matchSelectedTypes(assets, ["database"], opts).map((x) => x.ID)).toEqual([2, 3]);
  });

  it("matches extension type", () => {
    expect(matchSelectedTypes(assets, ["kubernetes"], opts).map((x) => x.ID)).toEqual([5]);
  });

  it("treats empty selection as no filter (returns all)", () => {
    expect(matchSelectedTypes(assets, [], opts).map((x) => x.ID)).toEqual([1, 2, 3, 4, 5]);
  });

  it("matches case-insensitively", () => {
    const assetsMixed = [a(1, "SSH"), a(2, "MySQL")];
    expect(matchSelectedTypes(assetsMixed, ["ssh"], opts).map((x) => x.ID)).toEqual([1]);
    expect(matchSelectedTypes(assetsMixed, ["database"], opts).map((x) => x.ID)).toEqual([2]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && pnpm test src/__tests__/assetTypeOptions.test.ts`
Expected: FAIL with "Failed to resolve import @/lib/assetTypes/options".

---

## Task 2: Asset type options module — implement

**Files:**
- Create: `frontend/src/lib/assetTypes/options.ts`

- [ ] **Step 1: Create `options.ts`**

```ts
// frontend/src/lib/assetTypes/options.ts
import type { ComponentType } from "react";
import { Monitor, Database, Cylinder, Leaf, Server } from "lucide-react";
import { getIconComponent } from "@/components/asset/IconPicker";
import type { ExtManifest } from "@/extension/types";
import type { asset_entity } from "../../../wailsjs/go/models";

export interface AssetTypeOption {
  /** Stable identifier — used as the persisted "selected" value. */
  value: string;
  /** All `asset.Type` values that should match when this option is selected. */
  aliases: string[];
  /** Either an i18n key (built-in) or a literal display string (extension). */
  label: string;
  /** Marks `label` as i18n key vs literal. */
  labelIsI18nKey: boolean;
  /** Icon component for direct render. */
  icon: ComponentType<{ className?: string; style?: React.CSSProperties }>;
  /** For extension entries: the manifest icon name (resolved via getIconComponent). */
  iconName?: string;
  group: "builtin" | "extension";
}

interface ExtensionEntryLike {
  manifest: ExtManifest;
}

const BUILTIN_OPTIONS: AssetTypeOption[] = [
  {
    value: "ssh",
    aliases: ["ssh"],
    label: "nav.ssh",
    labelIsI18nKey: true,
    icon: Monitor,
    group: "builtin",
  },
  {
    value: "database",
    aliases: ["database", "mysql", "postgresql"],
    label: "nav.database",
    labelIsI18nKey: true,
    icon: Database,
    group: "builtin",
  },
  {
    value: "redis",
    aliases: ["redis"],
    label: "nav.redis",
    labelIsI18nKey: true,
    icon: Cylinder,
    group: "builtin",
  },
  {
    value: "mongodb",
    aliases: ["mongodb", "mongo"],
    label: "nav.mongodb",
    labelIsI18nKey: true,
    icon: Leaf,
    group: "builtin",
  },
];

export function getAssetTypeOptions(
  extensions: Record<string, ExtensionEntryLike>,
): AssetTypeOption[] {
  const out: AssetTypeOption[] = [...BUILTIN_OPTIONS];
  for (const entry of Object.values(extensions)) {
    const m = entry.manifest;
    if (!m.assetTypes?.length) continue;
    for (const at of m.assetTypes) {
      out.push({
        value: at.type,
        aliases: [at.type],
        label: at.i18n?.name ?? at.type,
        labelIsI18nKey: false,
        icon: m.icon ? getIconComponent(m.icon) : Server,
        iconName: m.icon,
        group: "extension",
      });
    }
  }
  return out;
}

export function matchSelectedTypes(
  assets: asset_entity.Asset[],
  selectedTypes: string[] | "all",
  options: AssetTypeOption[],
): asset_entity.Asset[] {
  if (selectedTypes === "all" || selectedTypes.length === 0) return assets;
  const aliasSet = new Set<string>();
  for (const value of selectedTypes) {
    const opt = options.find((o) => o.value === value);
    if (opt) opt.aliases.forEach((a) => aliasSet.add(a.toLowerCase()));
    else aliasSet.add(value.toLowerCase());
  }
  return assets.filter((a) => aliasSet.has((a.Type || "").trim().toLowerCase()));
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd frontend && pnpm test src/__tests__/assetTypeOptions.test.ts`
Expected: PASS, all 8 tests green.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/assetTypes/options.ts frontend/src/__tests__/assetTypeOptions.test.ts
git commit -m "$(cat <<'EOF'
✨ 新增 assetTypes options 模块支持扩展资产筛选

- getAssetTypeOptions 合并内置类型与扩展 assetTypes
- matchSelectedTypes 支持 "all"、aliases、大小写不敏感
EOF
)"
```

---

## Task 3: i18n keys

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`

- [ ] **Step 1: Add zh-CN keys**

Edit `frontend/src/i18n/locales/zh-CN/common.json`. Find:

```json
    "search": "搜索...",
```

Replace with (insert 4 new keys after `"search": "搜索..."`):

```json
    "search": "搜索...",
    "filterByType": "按资产类型筛选",
    "filterByTypeActive": "已筛选 {{count}} 类",
    "filterAllTypes": "全部类型",
    "filterExtensions": "扩展资产",
```

- [ ] **Step 2: Add en keys**

Edit `frontend/src/i18n/locales/en/common.json`. Find:

```json
    "search": "Search...",
```

Replace with:

```json
    "search": "Search...",
    "filterByType": "Filter by asset type",
    "filterByTypeActive": "{{count}} type(s) selected",
    "filterAllTypes": "All types",
    "filterExtensions": "Extensions",
```

- [ ] **Step 3: Verify JSON parses**

Run: `cd frontend && node -e "JSON.parse(require('fs').readFileSync('src/i18n/locales/zh-CN/common.json','utf8')); JSON.parse(require('fs').readFileSync('src/i18n/locales/en/common.json','utf8'));console.log('ok')"`
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/i18n/locales/zh-CN/common.json frontend/src/i18n/locales/en/common.json
git commit -m "✨ 新增资产类型筛选相关 i18n key"
```

---

## Task 4: AssetTypeFilterButton — failing test

**Files:**
- Test: `frontend/src/__tests__/AssetTreeTypeFilter.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/__tests__/AssetTreeTypeFilter.test.tsx`:

```tsx
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AssetTypeFilterButton } from "@/components/asset/AssetTypeFilterButton";
import { getAssetTypeOptions } from "@/lib/assetTypes/options";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, vars?: Record<string, unknown>) => (vars ? `${k}:${JSON.stringify(vars)}` : k) }),
}));

describe("AssetTypeFilterButton", () => {
  const builtinOpts = getAssetTypeOptions({});

  beforeEach(() => {
    cleanup();
  });
  afterEach(() => {
    cleanup();
  });

  it('renders without active dot when value is "all"', async () => {
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    const btn = screen.getByRole("button", { name: /asset.filterByType/i });
    expect(btn).toBeTruthy();
    expect(btn.querySelector('[data-active="true"]')).toBeNull();
  });

  it("renders an active marker when partial selection", () => {
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={() => {}} />);
    const btn = screen.getByRole("button", { name: /asset.filterByTypeActive/i });
    expect(btn.querySelector('[data-active="true"]')).not.toBeNull();
  });

  it("opens popover and lists built-in options", async () => {
    const user = userEvent.setup();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterAllTypes")).toBeTruthy();
    expect(screen.getByText("nav.ssh")).toBeTruthy();
    expect(screen.getByText("nav.database")).toBeTruthy();
    expect(screen.getByText("nav.redis")).toBeTruthy();
    expect(screen.getByText("nav.mongodb")).toBeTruthy();
  });

  it('toggling "All types" while not all switches selection to "all"', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("asset.filterAllTypes"));
    expect(onChange).toHaveBeenCalledWith("all");
  });

  it("toggling a single type from all narrows selection to other 3", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith(["database", "redis", "mongodb"]);
  });

  it('unchecking the last selected type falls back to "all"', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith("all");
  });

  it("renders Extensions section header when extension options are present", async () => {
    const user = userEvent.setup();
    const opts = getAssetTypeOptions({
      k8sExt: {
        manifest: {
          name: "k8sExt",
          version: "1",
          icon: "Server",
          i18n: { displayName: "Kubernetes", description: "" },
          assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
        },
      },
    } as never);
    render(<AssetTypeFilterButton value="all" options={opts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterExtensions")).toBeTruthy();
    expect(screen.getByText("Kubernetes")).toBeTruthy();
  });

  it("does not render Extensions section header when no extension options", async () => {
    const user = userEvent.setup();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.queryByText("asset.filterExtensions")).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && pnpm test src/__tests__/AssetTreeTypeFilter.test.tsx`
Expected: FAIL with `Failed to resolve import @/components/asset/AssetTypeFilterButton`.

---

## Task 5: AssetTypeFilterButton — implement

**Files:**
- Create: `frontend/src/components/asset/AssetTypeFilterButton.tsx`

- [ ] **Step 1: Create the component**

```tsx
// frontend/src/components/asset/AssetTypeFilterButton.tsx
import { useState } from "react";
import { Filter, Check } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button, Popover, PopoverContent, PopoverTrigger, ScrollArea, Tooltip, TooltipContent, TooltipTrigger } from "@opskat/ui";
import type { AssetTypeOption } from "@/lib/assetTypes/options";

interface AssetTypeFilterButtonProps {
  value: string[] | "all";
  options: AssetTypeOption[];
  onChange: (next: string[] | "all") => void;
}

export function AssetTypeFilterButton({ value, options, onChange }: AssetTypeFilterButtonProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const isAll = value === "all";
  const selectedSet = isAll ? null : new Set(value);
  const activeCount = isAll ? 0 : value.length;

  const builtin = options.filter((o) => o.group === "builtin");
  const extensions = options.filter((o) => o.group === "extension");

  const tooltipLabel = isAll
    ? t("asset.filterByType")
    : t("asset.filterByTypeActive", { count: activeCount });

  const toggleAll = () => {
    onChange("all");
  };

  const toggleOne = (opt: AssetTypeOption) => {
    if (isAll) {
      // Currently "all" → remove just this one, leaving the others.
      onChange(options.filter((o) => o.value !== opt.value).map((o) => o.value));
      return;
    }
    const next = selectedSet!.has(opt.value)
      ? value.filter((v) => v !== opt.value)
      : [...value, opt.value];
    if (next.length === 0) {
      onChange("all");
      return;
    }
    onChange(next);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 relative"
              aria-label={tooltipLabel}
            >
              <Filter className="h-3.5 w-3.5" />
              {!isAll && (
                <span
                  data-active="true"
                  className="absolute top-1 right-1 h-1.5 w-1.5 rounded-full bg-primary"
                />
              )}
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{tooltipLabel}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" side="bottom" sideOffset={4} className="w-[240px] p-0">
        <ScrollArea className="max-h-[360px]">
          <div className="py-1">
            <FilterRow
              label={t("asset.filterAllTypes")}
              checked={isAll}
              onClick={toggleAll}
            />
            <div className="my-1 mx-2 h-px bg-border" />
            {builtin.map((opt) => {
              const Icon = opt.icon;
              return (
                <FilterRow
                  key={opt.value}
                  label={opt.labelIsI18nKey ? t(opt.label) : opt.label}
                  icon={<Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                  checked={isAll || selectedSet!.has(opt.value)}
                  onClick={() => toggleOne(opt)}
                />
              );
            })}
            {extensions.length > 0 && (
              <>
                <div className="px-3 pt-2 pb-1 text-[10px] uppercase tracking-wider text-muted-foreground">
                  {t("asset.filterExtensions")}
                </div>
                {extensions.map((opt) => {
                  const Icon = opt.icon;
                  return (
                    <FilterRow
                      key={opt.value}
                      label={opt.labelIsI18nKey ? t(opt.label) : opt.label}
                      icon={<Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                      checked={isAll || selectedSet!.has(opt.value)}
                      onClick={() => toggleOne(opt)}
                    />
                  );
                })}
              </>
            )}
          </div>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  );
}

function FilterRow({
  label,
  icon,
  checked,
  onClick,
}: {
  label: string;
  icon?: React.ReactNode;
  checked: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-2 px-3 py-1.5 text-xs hover:bg-accent text-left"
    >
      <span className="flex h-3.5 w-3.5 items-center justify-center shrink-0">
        {checked ? <Check className="h-3.5 w-3.5 text-primary" /> : null}
      </span>
      {icon}
      <span className="flex-1 truncate">{label}</span>
    </button>
  );
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd frontend && pnpm test src/__tests__/AssetTreeTypeFilter.test.tsx`
Expected: PASS, all 8 tests green.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/asset/AssetTypeFilterButton.tsx frontend/src/__tests__/AssetTreeTypeFilter.test.tsx
git commit -m "$(cat <<'EOF'
✨ 新增 AssetTypeFilterButton 组件

- funnel 图标 + popover 展示内置与扩展资产类型
- 多选切换，全部取消自动回到全选
- 当前为"全部"时不显示激活红点
EOF
)"
```

---

## Task 6: Integrate filter into AssetTree (drop homeSection)

**Files:**
- Modify: `frontend/src/components/layout/AssetTree.tsx`

- [ ] **Step 1: Update imports and props**

Edit `frontend/src/components/layout/AssetTree.tsx`. Find lines 42-50:

```ts
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { filterAssets } from "@/lib/assetSearch";
import { getAssetType, normalizeAssetSection, type HomeSection } from "@/lib/assetTypes";
import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useActiveAssetIds } from "@/hooks/useActiveAssetIds";
import { MoveAsset, MoveGroup } from "../../../wailsjs/go/app/App";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";
```

Replace with:

```ts
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { filterAssets } from "@/lib/assetSearch";
import { getAssetType } from "@/lib/assetTypes";
import { getAssetTypeOptions, matchSelectedTypes } from "@/lib/assetTypes/options";
import { AssetTypeFilterButton } from "@/components/asset/AssetTypeFilterButton";
import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useExtensionStore } from "@/extension";
import { useActiveAssetIds } from "@/hooks/useActiveAssetIds";
import { MoveAsset, MoveGroup } from "../../../wailsjs/go/app/App";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";
```

- [ ] **Step 2: Remove homeSection from props**

Find lines 51-66:

```ts
interface AssetTreeProps {
  collapsed: boolean;
  homeSection?: HomeSection;
  sidebarHidden?: boolean;
  onShowSidebar?: () => void;
  onAddAsset: (groupId?: number) => void;
  onAddGroup: () => void;
  onEditGroup: (group: group_entity.Group) => void;
  onGroupDetail: (group: group_entity.Group) => void;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onCopyAsset: (asset: asset_entity.Asset) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onConnectAssetInNewTab?: (asset: asset_entity.Asset) => void;
  onSelectAsset: (asset: asset_entity.Asset) => void;
  onOpenInfoTab?: (type: "asset" | "group", id: number, name: string, icon?: string) => void;
}
```

Replace with:

```ts
interface AssetTreeProps {
  collapsed: boolean;
  sidebarHidden?: boolean;
  onShowSidebar?: () => void;
  onAddAsset: (groupId?: number) => void;
  onAddGroup: () => void;
  onEditGroup: (group: group_entity.Group) => void;
  onGroupDetail: (group: group_entity.Group) => void;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onCopyAsset: (asset: asset_entity.Asset) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onConnectAssetInNewTab?: (asset: asset_entity.Asset) => void;
  onSelectAsset: (asset: asset_entity.Asset) => void;
  onOpenInfoTab?: (type: "asset" | "group", id: number, name: string, icon?: string) => void;
}
```

- [ ] **Step 3: Update component signature**

Find lines 68-83:

```ts
export function AssetTree({
  collapsed,
  homeSection = "home",
  sidebarHidden,
  onShowSidebar,
  onAddAsset,
  onAddGroup,
  onEditGroup,
  onGroupDetail,
  onEditAsset,
  onCopyAsset,
  onConnectAsset,
  onConnectAssetInNewTab,
  onSelectAsset,
  onOpenInfoTab,
}: AssetTreeProps) {
```

Replace with:

```ts
const FILTER_LS_KEY = "asset_tree_type_filter";

function loadFilter(): string[] | "all" {
  try {
    const raw = localStorage.getItem(FILTER_LS_KEY);
    if (!raw) return "all";
    if (raw === '"all"' || raw === "all") return "all";
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.every((v) => typeof v === "string")) {
      return parsed.length === 0 ? "all" : (parsed as string[]);
    }
    return "all";
  } catch {
    return "all";
  }
}

function saveFilter(value: string[] | "all") {
  localStorage.setItem(FILTER_LS_KEY, value === "all" ? '"all"' : JSON.stringify(value));
}

export function AssetTree({
  collapsed,
  sidebarHidden,
  onShowSidebar,
  onAddAsset,
  onAddGroup,
  onEditGroup,
  onGroupDetail,
  onEditAsset,
  onCopyAsset,
  onConnectAsset,
  onConnectAssetInNewTab,
  onSelectAsset,
  onOpenInfoTab,
}: AssetTreeProps) {
```

- [ ] **Step 4: Add filter state and update filter logic**

Find lines 84-108 (the existing body up to `groupedAssets`):

```ts
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { assets, groups, selectedAssetId, fetchAssets, fetchGroups, deleteAsset, deleteGroup, refresh } =
    useAssetStore();
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);
  const activeAssetIds = useActiveAssetIds();
  const [filter, setFilter] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<{
    id: number;
    assetCount: number;
  } | null>(null);
  const [deleteAssetConfirm, setDeleteAssetConfirm] = useState<asset_entity.Asset | null>(null);

  useEffect(() => {
    fetchAssets();
    fetchGroups();
  }, [fetchAssets, fetchGroups]);

  if (collapsed) return null;

  const sectionAssets =
    homeSection === "home" ? assets : assets.filter((asset) => normalizeAssetSection(asset.Type) === homeSection);
  const filteredAssets = filter
    ? filterAssets(sectionAssets, groups, { query: filter }).map((r) => r.asset)
    : sectionAssets;
```

Replace with:

```ts
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { assets, groups, selectedAssetId, fetchAssets, fetchGroups, deleteAsset, deleteGroup, refresh } =
    useAssetStore();
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);
  const extensions = useExtensionStore((s) => s.extensions);
  const activeAssetIds = useActiveAssetIds();
  const [filter, setFilter] = useState("");
  const [selectedTypes, setSelectedTypes] = useState<string[] | "all">(loadFilter);
  const [deleteConfirm, setDeleteConfirm] = useState<{
    id: number;
    assetCount: number;
  } | null>(null);
  const [deleteAssetConfirm, setDeleteAssetConfirm] = useState<asset_entity.Asset | null>(null);

  useEffect(() => {
    fetchAssets();
    fetchGroups();
  }, [fetchAssets, fetchGroups]);

  useEffect(() => {
    saveFilter(selectedTypes);
  }, [selectedTypes]);

  const typeOptions = useMemo(() => getAssetTypeOptions(extensions), [extensions]);

  if (collapsed) return null;

  const typeFilteredAssets = matchSelectedTypes(assets, selectedTypes, typeOptions);
  const filteredAssets = filter
    ? filterAssets(typeFilteredAssets, groups, { query: filter }).map((r) => r.asset)
    : typeFilteredAssets;
```

- [ ] **Step 5: Add useMemo to React import**

Find line 1:

```ts
import React, { useEffect, useRef, useState } from "react";
```

Replace with:

```ts
import React, { useEffect, useMemo, useRef, useState } from "react";
```

- [ ] **Step 6: Add filter button to search row**

Find lines 213-221:

```tsx
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t("asset.search") || "Search..."}
            className="h-7 w-full rounded-md border border-sidebar-border bg-sidebar pl-7 pr-2 text-xs outline-none focus:border-ring focus:ring-1 focus:ring-ring/50 placeholder:text-muted-foreground/60 transition-colors duration-150"
          />
        </div>
```

Replace with:

```tsx
        <div className="flex items-center gap-1">
          <div className="relative flex-1">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder={t("asset.search") || "Search..."}
              className="h-7 w-full rounded-md border border-sidebar-border bg-sidebar pl-7 pr-2 text-xs outline-none focus:border-ring focus:ring-1 focus:ring-ring/50 placeholder:text-muted-foreground/60 transition-colors duration-150"
            />
          </div>
          <AssetTypeFilterButton value={selectedTypes} options={typeOptions} onChange={setSelectedTypes} />
        </div>
```

- [ ] **Step 7: Type-check**

Run: `cd frontend && pnpm exec tsc --noEmit`
Expected: PASS (no errors related to AssetTree).

Note: the call sites in `App.tsx` still pass `homeSection={homeSection}`. TS will error there. **Continue to Task 7 first** which removes those, then re-run typecheck. If running this step in isolation, accept that App.tsx prop errors will exist temporarily.

- [ ] **Step 8: Run AssetTree test to ensure no regression**

Run: `cd frontend && pnpm test src/__tests__/AssetTree.test.tsx`
Expected: PASS (this test uses an internal `AssetList` mock, not real `AssetTree`).

- [ ] **Step 9: Stage changes (commit deferred until App.tsx call sites updated)**

```bash
git add frontend/src/components/layout/AssetTree.tsx
```

Do NOT commit yet — App.tsx still passes the removed prop. Continue to Task 7.

---

## Task 7: Restore pre-#37 App.tsx + drop homeSection plumbing

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Update imports**

Find lines 24-26:

```ts
import { getAssetType, type HomeSection } from "@/lib/assetTypes";
import { tabBelongsToSection } from "@/lib/tabSection";
import { useTabStore, type PageTabMeta } from "@/stores/tabStore";
```

Replace with:

```ts
import { getAssetType } from "@/lib/assetTypes";
import { useTabStore } from "@/stores/tabStore";
```

- [ ] **Step 2: Remove homeSection state and related state**

Find lines 117-129:

```ts
  const [sidebarHidden, setSidebarHidden] = useState(() => localStorage.getItem("sidebar_hidden") === "true");
  const [assetTreeCollapsed, setAssetTreeCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true"
  );
  const [aiPanelCollapsed, setAiPanelCollapsed] = useState(() => localStorage.getItem("ai_panel_collapsed") === "true");
  const [assetTreeWidth, setAssetTreeWidth] = useState(() => {
    const saved = localStorage.getItem("asset_tree_width");
    return saved ? Math.max(160, Math.min(480, Number(saved))) : 224;
  });
  const [assetTreeResizing, setAssetTreeResizing] = useState(false);
  const assetTreeWidthRef = useRef(assetTreeWidth);
  const [homeSection, setHomeSection] = useState<HomeSection>("home");
  const assets = useAssetStore((s) => s.assets);
```

Replace with:

```ts
  const [sidebarHidden, setSidebarHidden] = useState(() => localStorage.getItem("sidebar_hidden") === "true");
  const [assetTreeCollapsed, setAssetTreeCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true"
  );
  const [aiPanelCollapsed, setAiPanelCollapsed] = useState(() => localStorage.getItem("ai_panel_collapsed") === "true");
  const [assetTreeWidth, setAssetTreeWidth] = useState(() => {
    const saved = localStorage.getItem("asset_tree_width");
    return saved ? Math.max(160, Math.min(480, Number(saved))) : 224;
  });
  const [assetTreeResizing, setAssetTreeResizing] = useState(false);
  const assetTreeWidthRef = useRef(assetTreeWidth);
```

(Removes `homeSection` state and the now-unused `assets` selector.)

- [ ] **Step 3: Remove `hideAssetListAfterConnect`**

Find lines 152-163:

```ts
  const hideAssetListAfterConnect = useCallback(() => {
    const layout = useLayoutStore.getState();
    if (layout.tabBarLayout === "left") {
      if (layout.leftPanelVisible) {
        layout.toggleVisible();
      }
      return;
    }

    setAssetTreeCollapsed(true);
    localStorage.setItem("sidebar_collapsed", "true");
  }, []);
```

Delete the entire `hideAssetListAfterConnect` function (12 lines).

- [ ] **Step 4: Strip `hideAssetListAfterConnect()` calls from `handleConnectAsset`**

Find lines 256-293:

```ts
  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    const def = getAssetType(asset.Type);
    if (def?.connectAction === "query") {
      useQueryStore.getState().openQueryTab(asset);
      hideAssetListAfterConnect();
      return;
    }

    // Check if this is an extension asset type
    const ext = useExtensionStore.getState().getExtensionForAssetType(asset.Type);
    if (ext) {
      const connectPage = ext.manifest.frontend?.pages.find((p) => p.slot === "asset.connect");
      if (connectPage) {
        useTabStore.getState().openTab({
          id: `ext-${asset.ID}-${connectPage.id}`,
          type: "page",
          label: asset.Name,
          icon: ext.manifest.icon,
          meta: {
            type: "page",
            pageId: connectPage.id,
            extensionName: ext.name,
            assetId: asset.ID,
          },
        });
        hideAssetListAfterConnect();
        return;
      }
    }

    if (def?.connectAction !== "terminal") return;
    try {
      await connect(asset);
      hideAssetListAfterConnect();
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };
```

Replace with:

```ts
  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    const def = getAssetType(asset.Type);
    if (def?.connectAction === "query") {
      useQueryStore.getState().openQueryTab(asset);
      return;
    }

    // Check if this is an extension asset type
    const ext = useExtensionStore.getState().getExtensionForAssetType(asset.Type);
    if (ext) {
      const connectPage = ext.manifest.frontend?.pages.find((p) => p.slot === "asset.connect");
      if (connectPage) {
        useTabStore.getState().openTab({
          id: `ext-${asset.ID}-${connectPage.id}`,
          type: "page",
          label: asset.Name,
          icon: ext.manifest.icon,
          meta: {
            type: "page",
            pageId: connectPage.id,
            extensionName: ext.name,
            assetId: asset.ID,
          },
        });
        return;
      }
    }

    if (def?.connectAction !== "terminal") return;
    try {
      await connect(asset);
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };
```

- [ ] **Step 5: Strip `hideAssetListAfterConnect()` from `handleConnectAssetInNewTab`**

Find lines 295-303:

```ts
  const handleConnectAssetInNewTab = async (asset: asset_entity.Asset) => {
    if (!getAssetType(asset.Type)?.canConnectInNewTab) return;
    try {
      await connect(asset, "", true);
      hideAssetListAfterConnect();
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };
```

Replace with:

```ts
  const handleConnectAssetInNewTab = async (asset: asset_entity.Asset) => {
    if (!getAssetType(asset.Type)?.canConnectInNewTab) return;
    try {
      await connect(asset, "", true);
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };
```

- [ ] **Step 6: Restore pre-#37 `handlePageChange`**

Find lines 305-377:

```ts
  // Sidebar page navigation
  const handlePageChange = useCallback(
    (page: string) => {
      const tabStore = useTabStore.getState();
      if (page === "home" || page === "database" || page === "ssh" || page === "redis" || page === "mongodb") {
        const section = page as HomeSection;
        setHomeSection(section);

        const terminalConnectedCount = Object.values(useTerminalStore.getState().tabData).reduce((count, tabData) => {
          return count + Object.values(tabData.panes).filter((pane) => pane.connected).length;
        }, 0);
        const queryConnectionCount = tabStore.tabs.filter((tab) => tab.type === "query").length;
        const extensionConnectionCount = tabStore.tabs.filter((tab) => {
          if (tab.type !== "page") return false;
          const meta = tab.meta as PageTabMeta;
          return Boolean(meta.extensionName && typeof meta.assetId === "number");
        }).length;
        const openAssetConnections = terminalConnectedCount + queryConnectionCount + extensionConnectionCount;

        const layout = useLayoutStore.getState();
        const clickedSameSection = section === homeSection;

        if (clickedSameSection && openAssetConnections > 1) {
          if (layout.tabBarLayout === "left") {
            layout.toggleVisible();
          } else {
            setAssetTreeCollapsed((prev) => {
              const next = !prev;
              localStorage.setItem("sidebar_collapsed", String(next));
              return next;
            });
          }
        } else {
          if (layout.tabBarLayout === "left") {
            if (!layout.leftPanelVisible) {
              layout.toggleVisible();
            }
          } else {
            setAssetTreeCollapsed((prev) => {
              if (!prev) return prev;
              localStorage.setItem("sidebar_collapsed", "false");
              return false;
            });
          }
        }

        const candidateTabs = tabStore.tabs.filter(
          (t) => t.type === "terminal" || t.type === "query" || t.type === "info"
        );
        const target = candidateTabs.find((t) => tabBelongsToSection(t, section, assets));
        if (target) {
          tabStore.activateTab(target.id);
        } else if (section === "home") {
          tabStore.activateTab(tabStore.tabs[0]?.id || "");
        }
        return;
      }
      // Page tabs: settings, forward, sshkeys, audit
      const existing = tabStore.tabs.find((t) => t.id === page);
      if (existing) {
        tabStore.activateTab(page);
      } else {
        tabStore.openTab({
          id: page,
          type: "page",
          label: page,
          meta: { type: "page", pageId: page },
        });
      }
      hideAssetListAfterConnect();
    },
    [homeSection, hideAssetListAfterConnect, assets]
  );
```

Replace with:

```ts
  // Sidebar page navigation
  const handlePageChange = useCallback((page: string) => {
    const tabStore = useTabStore.getState();
    if (page === "home") {
      const homeTab = tabStore.tabs.find(
        (t) => t.type === "terminal" || t.type === "info" || t.type === "query"
      );
      tabStore.activateTab(homeTab?.id || tabStore.tabs[0]?.id || "");
      return;
    }
    // Page tabs: settings, forward, sshkeys, audit, snippets
    const existing = tabStore.tabs.find((t) => t.id === page);
    if (existing) {
      tabStore.activateTab(page);
    } else {
      tabStore.openTab({
        id: page,
        type: "page",
        label: page,
        meta: { type: "page", pageId: page },
      });
    }
  }, []);
```

- [ ] **Step 7: Update `activePage` derivation**

Find lines 379-385:

```ts
  const tabBarLayout = useLayoutStore((s) => s.tabBarLayout);
  const leftPanelVisible = useLayoutStore((s) => s.leftPanelVisible);
  const activeSidePanel = useLayoutStore((s) => s.activeSidePanel);

  // Derive active page for sidebar highlighting
  const activeTab = useTabStore((s) => s.tabs.find((t) => t.id === s.activeTabId));
  const activePage = activeTab?.type === "page" ? activeTab.id : homeSection;
```

Replace with:

```ts
  const tabBarLayout = useLayoutStore((s) => s.tabBarLayout);
  const leftPanelVisible = useLayoutStore((s) => s.leftPanelVisible);
  const activeSidePanel = useLayoutStore((s) => s.activeSidePanel);

  // Derive active page for sidebar highlighting: page tabs use their id; everything else (terminal/info/query/none) is "home".
  const activeTab = useTabStore((s) => s.tabs.find((t) => t.id === s.activeTabId));
  const activePage = activeTab?.type === "page" ? activeTab.id : "home";
```

- [ ] **Step 8: Remove `homeSection` from JSX call sites (3 places)**

Find line 410-414 (left-tab-bar branch AssetTree):

```tsx
                      <AssetTree
                        collapsed={false}
                        homeSection={homeSection}
                        sidebarHidden={sidebarHidden}
                        onShowSidebar={toggleSidebarHidden}
                        onAddAsset={handleAddAsset}
```

Replace with:

```tsx
                      <AssetTree
                        collapsed={false}
                        sidebarHidden={sidebarHidden}
                        onShowSidebar={toggleSidebarHidden}
                        onAddAsset={handleAddAsset}
```

Find line 437 (SideTabList):

```tsx
                      <SideTabList homeSection={homeSection} />
```

Replace with:

```tsx
                      <SideTabList />
```

Find lines 460-464 (top-tab-bar branch AssetTree):

```tsx
                  <AssetTree
                    collapsed={false}
                    homeSection={homeSection}
                    sidebarHidden={sidebarHidden}
                    onShowSidebar={toggleSidebarHidden}
                    onAddAsset={handleAddAsset}
```

Replace with:

```tsx
                  <AssetTree
                    collapsed={false}
                    sidebarHidden={sidebarHidden}
                    onShowSidebar={toggleSidebarHidden}
                    onAddAsset={handleAddAsset}
```

- [ ] **Step 9: Verify imports — drop unused `useTerminalStore` selector usage if any**

Run: `cd frontend && pnpm exec tsc --noEmit 2>&1 | head -40`
Expected: AssetTree-related errors gone; only the SideTabList prop error may remain (handled in Task 8). If `useTerminalStore` import becomes unused, remove it; if `PageTabMeta` import becomes unused, it was already replaced in Step 1.

---

## Task 8: Decouple SideTabList from homeSection

**Files:**
- Modify: `frontend/src/components/layout/SideTabList.tsx`

- [ ] **Step 1: Inspect current file**

Run: `cat frontend/src/components/layout/SideTabList.tsx | head -40` to confirm current shape:

```tsx
import { type HomeSection } from "@/lib/assetTypes";
import { tabBelongsToSection } from "@/lib/tabSection";
// ...
export function SideTabList({ homeSection = "home" }: { homeSection?: HomeSection }) {
  // ...
  const filteredTabs = useMemo(
    () => tabs.filter((tab) => tabBelongsToSection(tab, homeSection, assets)),
    [tabs, homeSection, assets]
  );
```

- [ ] **Step 2: Strip homeSection wiring**

Edit `frontend/src/components/layout/SideTabList.tsx`:

- Remove the import `import { type HomeSection } from "@/lib/assetTypes";` if no other use.
- Remove the import `import { tabBelongsToSection } from "@/lib/tabSection";`.
- Remove the `useAssetStore` import line if `assets` becomes unused.
- Change the function signature from `export function SideTabList({ homeSection = "home" }: { homeSection?: HomeSection }) {` to `export function SideTabList() {`.
- Replace the `filteredTabs` useMemo with a passthrough:
  ```ts
  const filteredTabs = tabs;
  ```
  (or rename to `tabs` directly downstream — keeping `filteredTabs` minimizes diff churn.)

After edits the file should not import `HomeSection`, `tabBelongsToSection`, or reference `homeSection`.

- [ ] **Step 3: Type-check**

Run: `cd frontend && pnpm exec tsc --noEmit`
Expected: Zero errors.

- [ ] **Step 4: Run frontend tests**

Run: `cd frontend && pnpm test`
Expected: All existing tests pass; new `assetTypeOptions.test.ts` and `AssetTreeTypeFilter.test.tsx` pass.

- [ ] **Step 5: Commit Tasks 6 + 7 + 8 together**

These three tasks are interlocked (AssetTree drops the prop, App.tsx + SideTabList stop passing/consuming it). Commit them as one atomic change:

```bash
git add frontend/src/components/layout/AssetTree.tsx frontend/src/App.tsx frontend/src/components/layout/SideTabList.tsx
git commit -m "$(cat <<'EOF'
♻️ AssetTree 内置类型筛选并恢复 pre-#37 显隐语义

- AssetTree 接管类型筛选状态（localStorage 持久化），不再接受 homeSection prop
- App.tsx 移除 homeSection 状态、hideAssetListAfterConnect 与所有自动折叠/激活逻辑
- handlePageChange 还原为 pre-#37 简化版：home 仅激活首个 terminal/info/query tab
- SideTabList 解除与 homeSection 的耦合，直接展示全部 tab
EOF
)"
```

---

## Task 9: Drop sidebar type buttons

**Files:**
- Modify: `frontend/src/components/layout/Sidebar.tsx`

- [ ] **Step 1: Update imports**

Find lines 1-18:

```ts
import {
  Home,
  Settings,
  KeyRound,
  PanelLeftClose,
  PanelLeftOpen,
  EyeOff,
  Bot,
  ScrollText,
  ArrowRightLeft,
  Server,
  LayoutList,
  Database,
  Monitor,
  Cylinder,
  Leaf,
  FileCode,
} from "lucide-react";
```

Replace with:

```ts
import {
  Home,
  Settings,
  KeyRound,
  PanelLeftClose,
  PanelLeftOpen,
  EyeOff,
  Bot,
  ScrollText,
  ArrowRightLeft,
  Server,
  LayoutList,
  FileCode,
} from "lucide-react";
```

- [ ] **Step 2: Drop the type button group from `navGroups`**

Find lines 57-71:

```ts
  const navGroups: NavItem[][] = [
    [{ id: "home", icon: Home, label: t("nav.home") }],
    [
      { id: "database", icon: Database, label: t("nav.database") },
      { id: "ssh", icon: Monitor, label: t("nav.ssh") },
      { id: "redis", icon: Cylinder, label: t("nav.redis") },
      { id: "mongodb", icon: Leaf, label: t("nav.mongodb") },
    ],
    [
      { id: "forward", icon: ArrowRightLeft, label: t("nav.forward") },
      { id: "sshkeys", icon: KeyRound, label: t("nav.sshKeys") },
      { id: "snippets", icon: FileCode, label: t("nav.snippets") },
      { id: "audit", icon: ScrollText, label: t("nav.audit") },
    ],
  ];
```

Replace with:

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

- [ ] **Step 3: Type-check**

Run: `cd frontend && pnpm exec tsc --noEmit`
Expected: Zero errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/Sidebar.tsx
git commit -m "🔧 移除 Sidebar 资产类型快捷按钮"
```

---

## Task 10: Delete dead code (HomeSection, normalizeAssetSection, tabSection.ts)

**Files:**
- Modify: `frontend/src/lib/assetTypes/index.ts`
- Delete: `frontend/src/lib/tabSection.ts`

- [ ] **Step 1: Strip HomeSection / normalizeAssetSection from `lib/assetTypes/index.ts`**

Read current `frontend/src/lib/assetTypes/index.ts`:

```ts
import type { AssetTypeDefinition } from "./types";
import { registry } from "./_register";
export { registerAssetType } from "./_register";

export function getAssetType(type: string): AssetTypeDefinition | undefined {
  return registry.get(type);
}

export function isBuiltinType(type: string): boolean {
  return registry.has(type);
}

export function getBuiltinTypes(): AssetTypeDefinition[] {
  return [...registry.values()];
}

export type HomeSection = "home" | "database" | "ssh" | "redis" | "mongodb";

export function normalizeAssetSection(type: string): "database" | "ssh" | "redis" | "mongodb" | undefined {
  const normalized = type.trim().toLowerCase();
  if (!normalized) return undefined;
  if (normalized === "mysql" || normalized === "postgresql") return "database";
  if (normalized === "mongo") return "mongodb";
  if (normalized === "database" || normalized === "ssh" || normalized === "redis" || normalized === "mongodb") {
    return normalized;
  }
  return undefined;
}

// Side-effect imports — register all built-in types
import "./ssh";
import "./database";
import "./redis";
import "./mongodb";

export type { AssetTypeDefinition, DetailInfoCardProps, PolicyDefinition, PolicyFieldDef } from "./types";
```

Replace with:

```ts
import type { AssetTypeDefinition } from "./types";
import { registry } from "./_register";
export { registerAssetType } from "./_register";

export function getAssetType(type: string): AssetTypeDefinition | undefined {
  return registry.get(type);
}

export function isBuiltinType(type: string): boolean {
  return registry.has(type);
}

export function getBuiltinTypes(): AssetTypeDefinition[] {
  return [...registry.values()];
}

// Side-effect imports — register all built-in types
import "./ssh";
import "./database";
import "./redis";
import "./mongodb";

export type { AssetTypeDefinition, DetailInfoCardProps, PolicyDefinition, PolicyFieldDef } from "./types";
```

- [ ] **Step 2: Delete `lib/tabSection.ts`**

Run: `rm frontend/src/lib/tabSection.ts`

- [ ] **Step 3: Verify nothing else references the removed symbols**

Run: `cd frontend && grep -rn "HomeSection\|normalizeAssetSection\|tabBelongsToSection\|hideAssetListAfterConnect" src/ 2>&1 | grep -v "options.ts" | grep -v ".test."`
Expected: empty output. (Hits in `options.ts` related comments are fine if any; .test files have been updated.)

- [ ] **Step 4: Type-check**

Run: `cd frontend && pnpm exec tsc --noEmit`
Expected: Zero errors.

- [ ] **Step 5: Run all frontend tests**

Run: `cd frontend && pnpm test`
Expected: All green.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/assetTypes/index.ts frontend/src/lib/tabSection.ts
git commit -m "♻️ 删除 HomeSection / normalizeAssetSection / tabSection 死代码"
```

---

## Task 11: Lint + final verification

**Files:** N/A (verification only)

- [ ] **Step 1: Run frontend lint**

Run: `cd frontend && pnpm lint`
Expected: Zero errors. Fix any lint warnings introduced (typically: unused imports). If a fix is needed, run `pnpm lint:fix`, then `git add` + amend or commit fixup.

- [ ] **Step 2: Run frontend tests once more**

Run: `cd frontend && pnpm test`
Expected: All green, including the 2 new test files.

- [ ] **Step 3: Build production bundle**

Run: `cd frontend && pnpm build`
Expected: Succeeds, no type errors.

- [ ] **Step 4: Manual smoke (`make dev`)**

Run: `make dev` (from repo root). Verify each item:

1. Sidebar: no Database / SSH / Redis / MongoDB icons. Home button still present.
2. AssetTree: search bar has a funnel icon to its right. Default state — no red dot.
3. Click funnel → popover shows `All types`, `SSH`, `Database`, `Redis`, `MongoDB` (all checked).
4. Click `SSH` → popover stays open, SSH unchecked, AssetTree hides SSH assets, funnel shows red dot.
5. Click `Database`, `Redis`, `MongoDB` to uncheck them all → AssetTree returns to "all" state automatically (red dot disappears).
6. Re-uncheck `SSH` → close popover. Restart `make dev`. AssetTree should still hide SSH assets (persistence).
7. With an extension that registers `assetTypes` loaded (e.g., `make devserver EXT=<name>`), open popover → "Extensions" header appears with the extension's type listed. Filter respects it.
8. Connect an SSH asset (double-click) → AssetTree remains visible, no auto-collapse.
9. Open a query tab on a database asset → AssetTree remains visible.
10. Click sidebar's Home icon → activates first terminal/info/query tab, AssetTree visibility unchanged.
11. Click `PanelLeftClose` button (bottom-left of sidebar) → AssetTree collapses. Click again → expands.
12. Click `EyeOff` button (sidebar) → entire sidebar+tree hides; reveal strip on left edge brings it back.

- [ ] **Step 5: Final commit (if any lint fixes were needed)**

```bash
git status
# If clean, skip. Otherwise:
git add <fixed files>
git commit -m "🎨 lint fix"
```

---

## Self-Review Notes

**Spec coverage check:**
- 数据层 (`AssetTypeOption`, `getAssetTypeOptions`, `matchSelectedTypes`) → Task 1, 2.
- 删除 `HomeSection`/`normalizeAssetSection` → Task 10.
- 删除 `lib/tabSection.ts` → Task 10.
- 状态层（内部 state + localStorage） → Task 6.
- UI 层（搜索行 + funnel button + popover） → Task 4, 5, 6.
- i18n 新增 keys → Task 3.
- 过滤逻辑 (`matchSelectedTypes` 集成) → Task 6.
- App.tsx 清理（state、hideAssetListAfterConnect、handlePageChange、activePage、call sites） → Task 7.
- Sidebar.tsx 清理 → Task 9.
- SideTabList.tsx 清理 → Task 8.
- AssetTree.tsx Props 调整 → Task 6.
- 单元测试 (`assetTypeOptions.test.ts`, `AssetTreeTypeFilter.test.tsx`) → Task 1, 4, 5.
- 手测脚本（10 项） → Task 11 Step 4.

**Type consistency:** `selectedTypes: string[] | "all"` is the single representation throughout. `AssetTypeOption.value` is a `string`, with `aliases: string[]`. `onChange` always receives the same type as `value`.

**Placeholder scan:** No TBDs, no "implement later", every code step contains the exact code or exact diff lines.
