import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
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
  defaultCollapsed?: boolean;
}

export function K8sConditionList({ conditions, title, defaultCollapsed = false }: K8sConditionListProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <K8sSectionCard>
      <button
        className="flex items-center justify-between w-full text-xs font-semibold uppercase tracking-wider text-muted-foreground"
        onClick={() => setCollapsed((v) => !v)}
      >
        <span className="flex items-center gap-1.5">
          {collapsed ? <ChevronRight className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          {title || "Conditions"}
        </span>
      </button>
      {!collapsed && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
          {conditions.map((c) => (
            <div key={c.type} className="rounded-lg border p-3">
              <div className="flex items-center justify-between mb-1">
                <span className="text-sm font-medium">{c.type}</span>
                <span
                  className={`text-xs px-1.5 py-0.5 rounded-full ${
                    c.status === "True" ? statusVariantToClass("success") : statusVariantToClass("error")
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
      )}
    </K8sSectionCard>
  );
}
