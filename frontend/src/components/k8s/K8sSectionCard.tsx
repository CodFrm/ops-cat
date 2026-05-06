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
