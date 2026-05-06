import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { K8sSectionCard } from "./K8sSectionCard";

interface K8sCodeBlockProps {
  code: string;
  title?: string;
  maxHeight?: string;
  defaultCollapsed?: boolean;
}

export function K8sCodeBlock({ code, title, maxHeight = "max-h-96", defaultCollapsed = false }: K8sCodeBlockProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <K8sSectionCard>
      <button
        className="flex items-center justify-between w-full text-xs font-semibold uppercase tracking-wider text-muted-foreground"
        onClick={() => setCollapsed((v) => !v)}
      >
        <span className="flex items-center gap-1.5">
          {collapsed ? <ChevronRight className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          {title || "YAML"}
        </span>
      </button>
      {!collapsed && (
        <pre
          className={`bg-muted/50 rounded-lg p-3 text-xs font-mono overflow-y-auto whitespace-pre-wrap mt-3 ${maxHeight}`}
        >
          {code}
        </pre>
      )}
    </K8sSectionCard>
  );
}
