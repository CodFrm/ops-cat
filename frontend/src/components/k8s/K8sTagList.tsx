import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { K8sSectionCard } from "./K8sSectionCard";

interface K8sTagListProps {
  tags: Record<string, string>;
  title?: string;
  defaultCollapsed?: boolean;
}

export function K8sTagList({ tags, title, defaultCollapsed = false }: K8sTagListProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const entries = Object.entries(tags);
  if (entries.length === 0) return null;

  return (
    <K8sSectionCard>
      <button
        className="flex items-center justify-between w-full text-xs font-semibold uppercase tracking-wider text-muted-foreground"
        onClick={() => setCollapsed((v) => !v)}
      >
        <span className="flex items-center gap-1.5">
          {collapsed ? <ChevronRight className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          {title || "Labels"}
        </span>
      </button>
      {!collapsed && (
        <div className="flex flex-wrap gap-2 mt-3">
          {entries.map(([k, v]) => (
            <span
              key={k}
              className="inline-flex items-center rounded-md border bg-muted/50 px-2 py-0.5 text-xs font-mono"
            >
              {k}: {v}
            </span>
          ))}
        </div>
      )}
    </K8sSectionCard>
  );
}
