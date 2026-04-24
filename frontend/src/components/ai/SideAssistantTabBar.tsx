import { X } from "lucide-react";
import { cn, Button } from "@opskat/ui";
import { useTranslation } from "react-i18next";
import type { SidebarAITab, SidebarTabStatus } from "@/stores/aiStore";

interface SideAssistantTabBarProps {
  tabs: SidebarAITab[];
  activeTabId: string | null;
  getStatus: (tabId: string) => SidebarTabStatus;
  onActivate: (tabId: string) => void;
  onClose: (tabId: string) => void;
}

const statusClassNames: Record<Exclude<SidebarTabStatus, null>, string> = {
  waiting_approval: "bg-amber-500",
  running: "bg-sky-500",
  done: "bg-emerald-500",
  error: "bg-rose-500",
};

export function SideAssistantTabBar({ tabs, activeTabId, getStatus, onActivate, onClose }: SideAssistantTabBarProps) {
  const { t } = useTranslation();

  return (
    <div className="border-b border-panel-divider px-2 py-1.5" role="tablist" aria-label={t("ai.sidebar.title")}>
      <div className="flex gap-1 overflow-x-auto pb-0.5">
        {tabs.map((tab) => {
          const isActive = tab.id === activeTabId;
          const status = getStatus(tab.id);
          return (
            <div
              key={tab.id}
              className={cn(
                "group flex min-w-0 items-center gap-2 rounded-md border px-2.5 py-1.5 text-xs transition-colors",
                isActive
                  ? "border-primary/40 bg-background text-foreground shadow-sm"
                  : "border-transparent bg-muted/50 text-muted-foreground hover:bg-muted"
              )}
              role="presentation"
            >
              <button
                type="button"
                role="tab"
                aria-selected={isActive}
                className="flex min-w-0 flex-1 items-center gap-2 text-left"
                onClick={() => onActivate(tab.id)}
              >
                {status ? (
                  <span
                    className={cn("h-1.5 w-1.5 shrink-0 rounded-full", statusClassNames[status])}
                    title={t(`ai.sidebar.status.${status}`)}
                  />
                ) : (
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-transparent" />
                )}
                <span className="truncate">{tab.title || t("ai.newConversation")}</span>
              </button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-4 w-4 shrink-0 opacity-60 hover:opacity-100"
                onClick={(event) => {
                  event.stopPropagation();
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
  );
}
