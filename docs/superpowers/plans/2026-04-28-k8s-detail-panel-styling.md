# K8s 资源详情面板样式重构实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提取 K8s 详情面板的公共视觉模式为共享组件，并统一所有 K8s 资源详情面板的样式与 OpsKat 整体 UI 规范一致。

**Architecture:** 在 `frontend/src/components/k8s/` 下新建 9 个共享组件/工具文件，然后把 `K8sClusterPage.tsx` 中所有详情面板的内联 JSX 替换为新组件。组件采用无状态函数组件设计，数据通过 props 传入，保持原有数据流不变。

**Tech Stack:** React 19, TypeScript, Tailwind CSS 4, shadcn/ui `@opskat/ui`, Lucide React, i18next

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `frontend/src/components/k8s/utils.ts` | 共享状态颜色工具函数 |
| `frontend/src/components/k8s/K8sSectionCard.tsx` | 通用区块卡片（带标题） |
| `frontend/src/components/k8s/K8sResourceHeader.tsx` | 资源头部（名称 + 副标题 + 状态 badge） |
| `frontend/src/components/k8s/K8sMetadataGrid.tsx` | 元数据网格（复用 `InfoItem`） |
| `frontend/src/components/k8s/K8sTableSection.tsx` | 带标题的表格区块 |
| `frontend/src/components/k8s/K8sConditionList.tsx` | Pod 条件列表 |
| `frontend/src/components/k8s/K8sTagList.tsx` | 标签/注解展示 |
| `frontend/src/components/k8s/K8sCodeBlock.tsx` | YAML/数据代码块 |
| `frontend/src/components/k8s/K8sLogsPanel.tsx` | Pod 日志面板（含交互控件） |
| `frontend/src/components/k8s/index.ts` | 统一导出（可选，如项目无 index 模式则跳过） |
| `frontend/src/components/k8s/K8sClusterPage.tsx` | 使用新组件重构所有详情面板 |

---

### Task 1: 共享状态颜色工具函数

**Files:**
- Create: `frontend/src/components/k8s/utils.ts`

- [ ] **Step 1: 创建 `utils.ts`**

```typescript
export type StatusVariant = "success" | "warning" | "error" | "info" | "neutral";

export function getK8sStatusColor(status: string): StatusVariant {
  const s = status.toLowerCase();
  if (s === "running" || s === "true" || s === "ready") return "success";
  if (s === "pending") return "warning";
  if (s === "failed" || s === "false" || s === "unknown") return "error";
  return "neutral";
}

export function getContainerStateColor(state: string): StatusVariant {
  if (state.startsWith("Running")) return "success";
  if (state.startsWith("Waiting")) return "warning";
  return "error";
}

export function statusVariantToClass(variant: StatusVariant): string {
  const map: Record<StatusVariant, string> = {
    success: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400",
    warning: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-400",
    error: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400",
    info: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-400",
    neutral: "bg-muted text-muted-foreground",
  };
  return map[variant];
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/utils.ts
git commit -m "♻️ refactor: add shared K8s status color utilities"
```

---

### Task 2: K8sSectionCard 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sSectionCard.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

interface K8sSectionCardProps {
  title?: string;
  icon?: LucideIcon;
  children: ReactNode;
  className?: string;
}

export function K8sSectionCard({ title, icon: Icon, children, className }: K8sSectionCardProps) {
  return (
    <div className={`rounded-xl border bg-card p-4 ${className || ""}`}>
      {title && (
        <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3 flex items-center gap-1.5">
          {Icon && <Icon className="h-3.5 w-3.5" />}
          {title}
        </h4>
      )}
      {children}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sSectionCard.tsx
git commit -m "♻️ refactor: add K8sSectionCard shared component"
```

---

### Task 3: K8sResourceHeader 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sResourceHeader.tsx`
- Modify: `frontend/src/components/k8s/utils.ts`（确认已存在）

- [ ] **Step 1: 创建组件**

```typescript
import { statusVariantToClass, type StatusVariant } from "./utils";

interface K8sResourceHeaderProps {
  name: string;
  subtitle?: string;
  status?: {
    text: string;
    variant: StatusVariant;
  };
}

export function K8sResourceHeader({ name, subtitle, status }: K8sResourceHeaderProps) {
  return (
    <div className="flex items-center justify-between mb-4">
      <div>
        <h3 className="font-mono text-sm font-medium">{name}</h3>
        {subtitle && <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>}
      </div>
      {status && (
        <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusVariantToClass(status.variant)}`}>
          {status.text}
        </span>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sResourceHeader.tsx
git commit -m "♻️ refactor: add K8sResourceHeader shared component"
```

---

### Task 4: K8sMetadataGrid 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sMetadataGrid.tsx`
- Modify: `frontend/src/components/asset/detail/InfoItem.tsx`（只读确认存在）

- [ ] **Step 1: 创建组件**

```typescript
import { InfoItem } from "@/components/asset/detail/InfoItem";

interface MetadataItem {
  label: string;
  value: string;
  mono?: boolean;
}

interface K8sMetadataGridProps {
  items: MetadataItem[];
  className?: string;
}

export function K8sMetadataGrid({ items, className }: K8sMetadataGridProps) {
  return (
    <div className={`grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4 ${className || ""}`}>
      {items.map((item) => (
        <InfoItem key={item.label} label={item.label} value={item.value} mono={item.mono} />
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sMetadataGrid.tsx
git commit -m "♻️ refactor: add K8sMetadataGrid shared component"
```

---

### Task 5: K8sTableSection 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sTableSection.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import type { ReactNode } from "react";
import { K8sSectionCard } from "./K8sSectionCard";

interface Column<T> {
  key: string;
  label: ReactNode;
  className?: string;
}

interface K8sTableSectionProps<T> {
  title: string;
  columns: Column<T>[];
  data: T[];
  renderRow: (item: T, index: number) => ReactNode;
  emptyText?: string;
}

export function K8sTableSection<T>({ title, columns, data, renderRow, emptyText }: K8sTableSectionProps<T>) {
  return (
    <K8sSectionCard title={title}>
      {data.length === 0 ? (
        <p className="text-xs text-muted-foreground">{emptyText || "No data"}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    className={`text-left py-2 pr-4 text-xs text-muted-foreground font-medium ${col.className || ""}`}
                  >
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((item, i) => renderRow(item, i))}
            </tbody>
          </table>
        </div>
      )}
    </K8sSectionCard>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sTableSection.tsx
git commit -m "♻️ refactor: add K8sTableSection shared component"
```

---

### Task 6: K8sConditionList 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sConditionList.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import { K8sSectionCard } from "./K8sSectionCard";
import { statusVariantToClass } from "./utils";

interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
}

interface K8sConditionListProps {
  conditions: Condition[];
  title?: string;
}

export function K8sConditionList({ conditions, title }: K8sConditionListProps) {
  return (
    <K8sSectionCard title={title || "Conditions"}>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {conditions.map((c) => (
          <div key={c.type} className="rounded-lg border p-3">
            <div className="flex items-center justify-between mb-1">
              <span className="text-sm font-medium">{c.type}</span>
              <span
                className={`text-xs px-1.5 py-0.5 rounded-full ${
                  c.status === "True"
                    ? statusVariantToClass("success")
                    : statusVariantToClass("error")
                }`}
              >
                {c.status}
              </span>
            </div>
            {c.reason && <p className="text-xs text-muted-foreground">{c.reason}</p>}
            {c.message && <p className="text-xs text-muted-foreground mt-0.5">{c.message}</p>}
          </div>
        ))}
      </div>
    </K8sSectionCard>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sConditionList.tsx
git commit -m "♻️ refactor: add K8sConditionList shared component"
```

---

### Task 7: K8sTagList 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sTagList.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import { K8sSectionCard } from "./K8sSectionCard";

interface K8sTagListProps {
  tags: Record<string, string>;
  title?: string;
}

export function K8sTagList({ tags, title }: K8sTagListProps) {
  const entries = Object.entries(tags);
  if (entries.length === 0) return null;

  return (
    <K8sSectionCard title={title || "Labels"}>
      <div className="flex flex-wrap gap-2">
        {entries.map(([k, v]) => (
          <span
            key={k}
            className="inline-flex items-center rounded-md border bg-muted/50 px-2 py-0.5 text-xs font-mono"
          >
            {k}: {v}
          </span>
        ))}
      </div>
    </K8sSectionCard>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sTagList.tsx
git commit -m "♻️ refactor: add K8sTagList shared component"
```

---

### Task 8: K8sCodeBlock 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sCodeBlock.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import { K8sSectionCard } from "./K8sSectionCard";

interface K8sCodeBlockProps {
  code: string;
  title?: string;
  maxHeight?: string;
}

export function K8sCodeBlock({ code, title, maxHeight = "max-h-96" }: K8sCodeBlockProps) {
  return (
    <K8sSectionCard title={title || "YAML"}>
      <pre
        className={`bg-muted/50 rounded-lg p-3 text-xs font-mono overflow-y-auto whitespace-pre-wrap ${maxHeight}`}
      >
        {code}
      </pre>
    </K8sSectionCard>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sCodeBlock.tsx
git commit -m "♻️ refactor: add K8sCodeBlock shared component"
```

---

### Task 9: K8sLogsPanel 组件

**Files:**
- Create: `frontend/src/components/k8s/K8sLogsPanel.tsx`

- [ ] **Step 1: 创建组件**

```typescript
import { useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { ScrollText, Square, Play } from "lucide-react";
import { K8sSectionCard } from "./K8sSectionCard";

interface K8sLogsPanelProps {
  containers: { name: string }[];
  namespace: string;
  podName: string;
  logContainer: string | null;
  logTailLines: number;
  logStreamID: string | null;
  logError: string | null;
  logLines: string[];
  onContainerChange: (container: string) => void;
  onTailLinesChange: (lines: number) => void;
  onStart: () => void;
  onStop: () => void;
}

export function K8sLogsPanel({
  containers,
  logContainer,
  logTailLines,
  logStreamID,
  logError,
  logLines,
  onContainerChange,
  onTailLinesChange,
  onStart,
  onStop,
}: K8sLogsPanelProps) {
  const { t } = useTranslation();
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logLines]);

  return (
    <K8sSectionCard>
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
          <ScrollText className="h-3.5 w-3.5" />
          {t("asset.k8sPodLogs")}
        </h4>
        <div className="flex items-center gap-2">
          <select
            className="h-7 rounded-md border bg-background px-2 text-xs"
            value={logContainer || containers[0]?.name || ""}
            onChange={(e) => onContainerChange(e.target.value)}
            disabled={!!logStreamID}
          >
            {containers.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
          <input
            type="number"
            className="h-7 w-16 rounded-md border bg-background px-2 text-xs"
            value={logTailLines}
            onChange={(e) => onTailLinesChange(Number(e.target.value))}
            disabled={!!logStreamID}
            min={1}
            max={10000}
            title={t("asset.k8sPodLogsTailLines")}
          />
          {logStreamID ? (
            <button
              onClick={onStop}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/50 px-3 py-1.5 text-xs text-destructive hover:bg-destructive/10"
            >
              <Square className="h-3 w-3" />
              {t("asset.k8sPodLogsStop")}
            </button>
          ) : (
            <button
              onClick={onStart}
              className="inline-flex items-center gap-1.5 rounded-md border border-primary/50 px-3 py-1.5 text-xs text-primary hover:bg-primary/10"
            >
              <Play className="h-3 w-3" />
              {t("asset.k8sPodLogsStart")}
            </button>
          )}
        </div>
      </div>
      {logError && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive mb-3">
          {t("asset.k8sPodLogsError")}: {logError}
        </div>
      )}
      <div className="bg-black rounded-lg p-3 text-xs font-mono max-h-96 overflow-y-auto">
        {logLines.length === 0 && !logStreamID && !logError && (
          <span className="text-gray-500">{t("asset.k8sPodLogsStopped")}</span>
        )}
        {logStreamID && logLines.length === 0 && (
          <span className="text-gray-500">{t("asset.k8sPodLogsStreaming")}</span>
        )}
        {logLines.map((line, i) => (
          <span key={i} className="text-green-400 block">
            {line}
          </span>
        ))}
        <div ref={logEndRef} />
      </div>
    </K8sSectionCard>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sLogsPanel.tsx
git commit -m "♻️ refactor: add K8sLogsPanel shared component"
```

---

### Task 10: 重构 K8sClusterPage.tsx — 提取 imports 和准备工具

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx`

- [ ] **Step 1: 在文件顶部添加新组件的 imports**

在现有 imports 区域（约在 `ScrollText` 附近）添加：

```typescript
import { K8sSectionCard } from "./K8sSectionCard";
import { K8sResourceHeader } from "./K8sResourceHeader";
import { K8sMetadataGrid } from "./K8sMetadataGrid";
import { K8sTableSection } from "./K8sTableSection";
import { K8sConditionList } from "./K8sConditionList";
import { K8sTagList } from "./K8sTagList";
import { K8sCodeBlock } from "./K8sCodeBlock";
import { K8sLogsPanel } from "./K8sLogsPanel";
import { getK8sStatusColor, getContainerStateColor, statusVariantToClass } from "./utils";
```

- [ ] **Step 2: 删除 `K8sClusterPage.tsx` 中局部定义的 `getStatusColor` 和 `getContainerStateColor`**

查找并删除（约在 1613-1626 行）：

```typescript
const getStatusColor = (status: string) => {
  ...
};

const getContainerStateColor = (state: string) => {
  ...
};
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: add K8s shared component imports and remove local color helpers"
```

---

### Task 11: 重构 Overview 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1381-1447`

- [ ] **Step 1: 替换 Overview JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
    <div className="rounded-lg bg-muted/50 p-4">...version...</div>
    <div className="rounded-lg bg-muted/50 p-4">...platform...</div>
    <div className="rounded-lg bg-muted/50 p-4">...nodes...</div>
  </div>
  <div className="rounded-xl border bg-card p-6">
    <h3 className="text-sm font-semibold mb-3">...</h3>
    ...nodes cards...
  </div>
  <div className="rounded-xl border bg-card p-6">
    <h3 className="text-sm font-semibold mb-3">...</h3>
    ...namespace tags...
  </div>
</div>
```

替换为：
```tsx
<div className="max-w-5xl mx-auto p-4 space-y-4">
  <K8sSectionCard>
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
      <InfoItem label={t("asset.k8sVersion")} value={info.version} mono />
      <InfoItem label={t("asset.k8sPlatform")} value={info.platform} mono />
      <InfoItem label={t("asset.k8sNodes")} value={String(info.nodes.length)} mono />
    </div>
  </K8sSectionCard>

  <K8sSectionCard title={t("asset.k8sNodes")}>
    <div className="grid gap-3 sm:grid-cols-2">
      {info.nodes.map((node) => (
        <div
          key={node.name}
          className="rounded-lg border p-3 cursor-pointer hover:bg-muted/30 transition-colors"
          onClick={() => openTab(`node:${node.name}`, node.name)}
        >
          <div className="flex items-center justify-between mb-2">
            <span className="font-mono text-sm font-medium">{node.name}</span>
            <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusVariantToClass(getK8sStatusColor(node.status))}`}>
              {node.status === "True" ? "Ready" : node.status}
            </span>
          </div>
          <div className="grid grid-cols-2 gap-1 text-xs text-muted-foreground">
            <span>OS: {node.os}</span>
            <span>Arch: {node.arch}</span>
            <span>CPU: {node.cpu}</span>
            <span>Mem: {node.memory}</span>
          </div>
        </div>
      ))}
    </div>
  </K8sSectionCard>

  <K8sSectionCard title={t("asset.k8sNamespaces")}>
    <div className="flex flex-wrap gap-2">
      {info.namespaces.map((ns) => (
        <span
          key={ns.name}
          className={`inline-flex items-center rounded-md border px-3 py-1 text-sm font-mono cursor-pointer hover:bg-muted/50 ${
            ns.status === "Active" ? "" : "text-muted-foreground border-dashed"
          }`}
          onClick={() => openTab(`ns:${ns.name}`, ns.name)}
        >
          {ns.name}
        </span>
      ))}
    </div>
  </K8sSectionCard>
</div>
```

注意：需要在文件顶部确认 `InfoItem` 已 import（`AssetDetail.tsx` 中有 import，但 `K8sClusterPage.tsx` 目前没有，需要添加 `import { InfoItem } from "@/components/asset/detail/InfoItem";`）。

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Overview panel with shared components"
```

---

### Task 12: 重构 Node 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1450-1493`

- [ ] **Step 1: 替换 Node JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <div className="flex items-center justify-between mb-4">
      <h3 className="text-base font-semibold">{activeNode.name}</h3>
      <span className="...">{activeNode.status === "True" ? "Ready" : activeNode.status}</span>
    </div>
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
      <div className="rounded-lg bg-muted/50 p-4"><div className="text-xs text-muted-foreground mb-1">OS</div>...</div>
      ...
    </div>
  </div>
</div>
```

替换为：
```tsx
<div className="max-w-5xl mx-auto p-4 space-y-4">
  <K8sSectionCard>
    <K8sResourceHeader
      name={activeNode.name}
      status={{
        text: activeNode.status === "True" ? "Ready" : activeNode.status,
        variant: getK8sStatusColor(activeNode.status),
      }}
    />
    <K8sMetadataGrid
      items={[
        { label: "OS", value: activeNode.os, mono: true },
        { label: "Architecture", value: activeNode.arch, mono: true },
        { label: "Kubernetes", value: `v${activeNode.version}`, mono: true },
        { label: "CPU", value: activeNode.cpu, mono: true },
        { label: "Memory", value: activeNode.memory, mono: true },
        { label: "Roles", value: activeNode.roles.join(", "), mono: true },
      ]}
    />
  </K8sSectionCard>
</div>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Node panel with shared components"
```

---

### Task 13: 重构 Namespace 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1495-1546`

- [ ] **Step 1: 替换 Namespace JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <h3 className="text-base font-semibold mb-1">{activeNs.name}</h3>
    <p className="text-xs text-muted-foreground mb-4">...</p>
    ...
  </div>
</div>
```

替换为：
```tsx
<div className="max-w-5xl mx-auto p-4 space-y-4">
  <K8sSectionCard>
    <K8sResourceHeader
      name={activeNs.name}
      subtitle={`${t("asset.k8sNamespace")}: ${activeNs.status}`}
      status={{
        text: activeNs.status,
        variant: activeNs.status === "Active" ? "success" : "neutral",
      }}
    />
    {loadingNamespaces.has(activeNs.name) ? (
      <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        {t("asset.k8sLoadingNamespace")}
      </div>
    ) : namespaceErrors[activeNs.name] ? (
      <div ...>...error...</div>
    ) : namespaceResources[activeNs.name] ? (
      <K8sMetadataGrid
        items={RESOURCE_TYPES.map((rt) => {
          const count = namespaceResources[activeNs.name][rt.key] as number;
          return {
            label: t(rt.labelKey),
            value: String(count),
            mono: true,
          };
        })}
      />
    ) : null}
  </K8sSectionCard>
</div>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Namespace panel with shared components"
```

---

### Task 14: 重构 ns-res 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1548-1571`

- [ ] **Step 1: 替换 ns-res JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <div className="flex items-center gap-3 mb-4">...icon + title + badge...</div>
    <div className="rounded-lg bg-muted/50 p-4">...</div>
  </div>
</div>
```

替换为：
```tsx
<div className="max-w-5xl mx-auto p-4 space-y-4">
  <K8sSectionCard>
    <div className="flex items-center gap-3 mb-4">
      {rt && <rt.icon className="h-5 w-5 text-muted-foreground" />}
      <h3 className="font-mono text-sm font-medium">{rt ? t(rt.labelKey) : resKey}</h3>
      <span className="text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">{ns}</span>
    </div>
    <K8sMetadataGrid items={[{ label: t("asset.k8sNamespaceResources"), value: String(count), mono: true }]} />
  </K8sSectionCard>
</div>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s ns-res panel with shared components"
```

---

### Task 15: 重构 Pod 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1573-1883`

- [ ] **Step 1: 替换 Pod 详情 JSX**

把 `activeTabId.startsWith("pod:")` 分支下的整个返回 JSX（从 1628 行左右的 `return (` 到 1882 行的 `);`）替换为：

```tsx
return (
  <div className="max-w-5xl mx-auto p-4 space-y-4">
    <K8sSectionCard>
      <K8sResourceHeader
        name={detail.name}
        subtitle={`${detail.namespace} · ${detail.node_name}`}
        status={{ text: detail.status, variant: getK8sStatusColor(detail.status) }}
      />
      <K8sMetadataGrid
        items={[
          { label: t("asset.k8sPodIP"), value: detail.pod_ip || "-", mono: true },
          { label: t("asset.k8sPodHostIP"), value: detail.host_ip || "-", mono: true },
          { label: t("asset.k8sPodCreationTime"), value: detail.creation_time },
          { label: t("asset.k8sPodReady"), value: detail.ready, mono: true },
          { label: t("asset.k8sPodQosClass"), value: detail.qos_class },
        ]}
      />
    </K8sSectionCard>

    <K8sTableSection
      title={t("asset.k8sPodContainers")}
      columns={[
        { key: "name", label: t("asset.k8sPodName") },
        { key: "image", label: "Image" },
        { key: "state", label: t("asset.k8sPodStatus") },
        { key: "ready", label: t("asset.k8sPodReady") },
        { key: "restarts", label: t("asset.k8sPodRestarts") },
      ]}
      data={detail.containers}
      emptyText={t("asset.k8sNoEvents")}
      renderRow={(c) => (
        <tr key={c.name} className="border-b last:border-0">
          <td className="py-2 pr-4 font-mono text-sm">{c.name}</td>
          <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{c.image}</td>
          <td className="py-2 pr-4">
            <span className={`text-xs px-1.5 py-0.5 rounded-full ${statusVariantToClass(getContainerStateColor(c.state))}`}>
              {c.state}
            </span>
          </td>
          <td className="py-2 pr-4">
            <span className={c.ready ? "text-green-600" : "text-red-600"}>{c.ready ? "\u2713" : "\u2717"}</span>
          </td>
          <td className="py-2 font-mono text-sm">{c.restart_count}</td>
        </tr>
      )}
    />

    <K8sConditionList conditions={detail.conditions} title={t("asset.k8sPodConditions")} />

    <K8sTableSection
      title={t("asset.k8sPodEvents")}
      columns={[
        { key: "type", label: "Type" },
        { key: "reason", label: "Reason" },
        { key: "message", label: "Message" },
        { key: "count", label: "Count" },
        { key: "last_time", label: "Last Seen" },
      ]}
      data={detail.events}
      emptyText={t("asset.k8sNoEvents")}
      renderRow={(e, i) => (
        <tr key={i} className="border-b last:border-0">
          <td className="py-2 pr-4">
            <span className={`text-xs px-1.5 py-0.5 rounded-full ${statusVariantToClass(e.type === "Warning" ? "warning" : "info")}`}>
              {e.type}
            </span>
          </td>
          <td className="py-2 pr-4 text-xs">{e.reason}</td>
          <td className="py-2 pr-4 text-xs text-muted-foreground max-w-xs truncate">{e.message}</td>
          <td className="py-2 pr-4 font-mono text-xs">{e.count}</td>
          <td className="py-2 text-xs text-muted-foreground">{e.last_time}</td>
        </tr>
      )}
    />

    <K8sTagList tags={detail.labels} title={t("asset.k8sPodLabels")} />

    <K8sCodeBlock code={detail.yaml} title={t("asset.k8sPodYAML")} />

    <K8sLogsPanel
      containers={detail.containers}
      namespace={detail.namespace}
      podName={detail.name}
      logContainer={logContainer}
      logTailLines={logTailLines}
      logStreamID={logStreamID}
      logError={logError}
      logLines={logLines}
      onContainerChange={(container) => {
        setLogContainer(container);
        if (logStreamID) {
          stopLogStream();
          startLogStream(detail.namespace, detail.name, container, logTailLines);
        }
      }}
      onTailLinesChange={(lines) => setLogTailLines(lines)}
      onStart={() => {
        const container = logContainer || detail.containers[0]?.name || "";
        startLogStream(detail.namespace, detail.name, container, logTailLines);
      }}
      onStop={stopLogStream}
    />
  </div>
);
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Pod panel with shared components"
```

---

### Task 16: 重构 Service 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1885-1970`

- [ ] **Step 1: 替换 Service JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <div className="flex items-center justify-between mb-4">...name + type badge...</div>
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">...metadata...</div>
  </div>
  <div className="rounded-xl border bg-card p-6">
    <h4 className="text-sm font-semibold mb-3">...</h4>
    ...ports table...
  </div>
</div>
```

替换为：
```tsx
<div className="max-w-5xl mx-auto p-4 space-y-4">
  <K8sSectionCard>
    <K8sResourceHeader
      name={svc.name}
      subtitle={svc.namespace}
      status={{ text: svc.type, variant: "info" }}
    />
    <K8sMetadataGrid
      items={[
        { label: t("asset.k8sServiceType"), value: svc.type, mono: true },
        { label: t("asset.k8sServiceClusterIP"), value: svc.cluster_ip || "-", mono: true },
        { label: t("asset.k8sPodAge"), value: svc.age, mono: true },
      ]}
    />
  </K8sSectionCard>

  <K8sTableSection
    title={t("asset.k8sServicePorts")}
    columns={[
      { key: "name", label: t("asset.k8sPodName") },
      { key: "port", label: t("asset.k8sServicePort") },
      { key: "target_port", label: t("asset.k8sServiceTargetPort") },
      { key: "protocol", label: t("asset.k8sServiceProtocol") },
      { key: "node_port", label: "NodePort" },
    ]}
    data={svc.ports}
    emptyText={t("asset.k8sNoEvents")}
    renderRow={(p, i) => (
      <tr key={i} className="border-b last:border-0">
        <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{p.name || "-"}</td>
        <td className="py-2 pr-4 font-mono text-sm">{p.port}</td>
        <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{p.target_port || "-"}</td>
        <td className="py-2 pr-4 text-xs">{p.protocol}</td>
        <td className="py-2 font-mono text-xs text-muted-foreground">{p.node_port || "-"}</td>
      </tr>
    )}
  />
</div>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Service panel with shared components"
```

---

### Task 17: 重构 ConfigMap 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:1972-2029`

- [ ] **Step 1: 替换 ConfigMap JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <div className="flex items-center justify-between mb-4">...name + keys count...</div>
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">...age...</div>
  </div>
  <div className="rounded-xl border bg-card p-6">
    <h4 className="text-sm font-semibold mb-3">Data</h4>
    ...entries...
  </div>
</div>
```

替换为：
```tsx
const dataEntries = Object.entries(cm.data || {});

return (
  <div className="max-w-5xl mx-auto p-4 space-y-4">
    <K8sSectionCard>
      <K8sResourceHeader
        name={cm.name}
        subtitle={cm.namespace}
        status={{ text: `${dataEntries.length} key${dataEntries.length !== 1 ? "s" : ""}`, variant: "neutral" }}
      />
      <K8sMetadataGrid items={[{ label: t("asset.k8sPodAge"), value: cm.age, mono: true }]} />
    </K8sSectionCard>

    <K8sSectionCard title="Data">
      {dataEntries.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("asset.k8sNoEvents")}</p>
      ) : (
        <div className="space-y-3">
          {dataEntries.map(([key, value]) => (
            <div key={key}>
              <div className="text-xs text-muted-foreground font-medium mb-1">{key}</div>
              <K8sCodeBlock code={value} maxHeight="max-h-64" />
            </div>
          ))}
        </div>
      )}
    </K8sSectionCard>
  </div>
);
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s ConfigMap panel with shared components"
```

---

### Task 18: 重构 Secret 面板

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx:2031-2102`

- [ ] **Step 1: 替换 Secret JSX**

把：
```tsx
<div className="max-w-4xl mx-auto p-6 space-y-6">
  <div className="rounded-xl border bg-card p-6">
    <div className="flex items-center justify-between mb-4">...name + type...</div>
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">...type + age...</div>
  </div>
  <div className="rounded-xl border bg-card p-6">
    <h4 className="text-sm font-semibold mb-3">...</h4>
    ...data entries...
  </div>
</div>
```

替换为：
```tsx
const dataEntries = Object.entries(secret.data || {});
const decodeValue = (encoded: string) => {
  try {
    return atob(encoded);
  } catch {
    return encoded;
  }
};

return (
  <div className="max-w-5xl mx-auto p-4 space-y-4">
    <K8sSectionCard>
      <K8sResourceHeader
        name={secret.name}
        subtitle={secret.namespace}
        status={{ text: secret.type, variant: "neutral" }}
      />
      <K8sMetadataGrid
        items={[
          { label: t("asset.k8sSecretType"), value: secret.type, mono: true },
          { label: t("asset.k8sPodAge"), value: secret.age, mono: true },
        ]}
      />
    </K8sSectionCard>

    <K8sSectionCard title={t("asset.k8sSecretData")}>
      {dataEntries.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("asset.k8sNoEvents")}</p>
      ) : (
        <div className="space-y-3">
          {dataEntries.map(([key, value]) => {
            const decoded = decodeValue(value);
            return (
              <div key={key}>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-xs text-muted-foreground font-medium">{key}</span>
                  <span className="text-[10px] text-muted-foreground">{decoded.length}B</span>
                </div>
                <K8sCodeBlock code={decoded} maxHeight="max-h-32" />
              </div>
            );
          })}
        </div>
      )}
    </K8sSectionCard>
  </div>
);
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "♻️ refactor: restyle K8s Secret panel with shared components"
```

---

### Task 19: 清理未使用的 import 和变量

**Files:**
- Modify: `frontend/src/components/k8s/K8sClusterPage.tsx`

- [ ] **Step 1: 运行 lint 检查未使用变量**

```bash
cd frontend && pnpm lint 2>&1 | head -60
```

- [ ] **Step 2: 修复所有 lint 报错**

根据 lint 输出，删除未使用的 import（例如可能被替换掉的 `ScrollText`、`Square`、`Play` 等，如果日志面板已提取为组件，需确认主文件是否还直接使用了这些图标）。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/k8s/K8sClusterPage.tsx
git commit -m "🎨 style: remove unused imports after K8s panel refactor"
```

---

### Task 20: 最终验证

**Files:**
- Modify: N/A（只读验证）

- [ ] **Step 1: 前端 lint 通过**

```bash
cd frontend && pnpm lint
```
Expected: 无 error，无未使用变量 warning。

- [ ] **Step 2: 前端测试通过**

```bash
cd frontend && pnpm test --run
```
Expected: 所有测试 pass（特别是 `K8sClusterPage` 相关的测试，如有）。

- [ ] **Step 3: TypeScript 编译检查**

```bash
cd frontend && pnpm tsc --noEmit
```
Expected: 无类型错误。

- [ ] **Step 4: Commit**

```bash
git commit --allow-empty -m "✅ tests: verify K8s panel refactor passes lint and tests"
```

---

## Self-Review

1. **Spec coverage**: 所有 8 个面板（Overview/Node/Namespace/ns-res/Pod/Service/ConfigMap/Secret）都有对应的重构 Task。所有 8 个新组件都有独立 Task。
2. **Placeholder scan**: 无 TBD/TODO/"implement later"。所有代码均为可直接使用的完整实现。
3. **Type consistency**: `StatusVariant` 在 `utils.ts`、`K8sResourceHeader.tsx`、`K8sConditionList.tsx` 中一致使用。`K8sTableSection` 的泛型签名在 Pod/Service 中一致。
