import type { ReactNode } from "react";
import { cn } from "@opskat/ui";

interface IdeaFrameProps {
  actions?: ReactNode;
  children: ReactNode;
  fileName: string;
  helper: string;
  layoutLabel: string;
  mode: "compare" | "merge";
  remotePath: string;
  sidebarLabel: string;
  status: string;
  subtitle?: string;
  testId: string;
  title: string;
}

export function ExternalEditIdeaFrame({
  actions,
  children,
  fileName,
  helper,
  layoutLabel,
  mode,
  remotePath,
  sidebarLabel,
  status,
  subtitle,
  testId,
  title,
}: IdeaFrameProps) {
  return (
    <div
      className={cn(
        "fixed z-50 flex overflow-hidden rounded-xl border border-slate-700 bg-[#1f2329] text-slate-100 shadow-2xl",
        mode === "compare" ? "inset-4" : "inset-3"
      )}
      data-idea-workbench={mode}
      data-testid={testId}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div className="flex w-56 shrink-0 flex-col border-r border-slate-700 bg-[#252a31]">
        <div className="border-b border-slate-700 px-4 py-3 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">
          {sidebarLabel}
        </div>
        <div className="flex-1 px-3 py-4">
          <div
            className={cn(
              "rounded-md px-3 py-2 text-sm font-medium",
              mode === "merge"
                ? "border border-amber-500/40 bg-amber-500/10 text-amber-100"
                : "bg-[#343b45] text-slate-100"
            )}
            data-testid={`external-edit-${mode}-idea-file`}
          >
            {fileName}
          </div>
          <div className="mt-3 break-all text-xs leading-5 text-slate-400">{remotePath}</div>
        </div>
        <div className="border-t border-slate-700 px-3 py-3 text-[11px] text-slate-400">{helper}</div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex h-12 items-center justify-between border-b border-slate-700 bg-[#2b3038] px-4">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">{title}</div>
            <div className="truncate text-[11px] text-slate-400">{subtitle || remotePath}</div>
          </div>
          {actions}
        </div>
        {children}
        <div className="flex h-8 items-center justify-between border-t border-slate-700 bg-[#252a31] px-4 text-[11px] text-slate-400">
          <span>{status}</span>
          <span>{layoutLabel}</span>
        </div>
      </div>
    </div>
  );
}

interface IdeaEditorPaneProps {
  badge: string;
  children: ReactNode;
  tone: "local" | "final" | "remote";
  title: string;
}

export function ExternalEditIdeaEditorPane({ badge, children, tone, title }: IdeaEditorPaneProps) {
  return (
    <div
      className={cn("flex min-h-0 flex-col bg-[#1f2329]", tone === "final" && "ring-1 ring-amber-400/40")}
      data-idea-pane={tone}
      data-testid={`external-edit-idea-pane-${tone}`}
    >
      <div
        className={cn(
          "flex h-9 items-center justify-between border-b px-3 text-xs",
          tone === "final" ? "border-amber-400/30 bg-[#3a3324]" : "border-slate-700 bg-[#303640]"
        )}
      >
        <span
          className={cn(
            "font-semibold",
            tone === "local" && "text-emerald-200",
            tone === "remote" && "text-sky-200",
            tone === "final" && "text-amber-100"
          )}
        >
          {title}
        </span>
        <span
          className={cn(
            "rounded px-2 py-0.5 text-[10px] uppercase tracking-wide",
            tone === "final" ? "bg-amber-400/20 text-amber-100" : "bg-slate-800 text-slate-300"
          )}
        >
          {badge}
        </span>
      </div>
      {children}
    </div>
  );
}
