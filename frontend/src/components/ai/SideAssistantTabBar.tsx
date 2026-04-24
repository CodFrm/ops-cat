import { LoaderCircle, X } from "lucide-react";
import { cn, Button } from "@opskat/ui";
import { useTranslation } from "react-i18next";
import type { SidebarAITab, SidebarTabStatus } from "@/stores/aiStore";

interface SideAssistantTabBarProps {
  tabs: SidebarAITab[];
  activeTabId: string | null;
  getStatus: (tabId: string) => SidebarTabStatus;
  compact?: boolean;
  onActivate: (tabId: string) => void;
  onClose: (tabId: string) => void;
}

const statusClassNames: Record<Exclude<SidebarTabStatus, null>, string> = {
  waiting_approval: "bg-amber-500",
  running: "bg-sky-500",
  done: "bg-emerald-500",
  error: "bg-rose-500",
};

export function SideAssistantTabBar({
  tabs,
  activeTabId,
  getStatus,
  compact = false,
  onActivate,
  onClose,
}: SideAssistantTabBarProps) {
  const { t } = useTranslation();
  const renderStatusIndicator = (status: SidebarTabStatus, isBlankSession: boolean) => {
    if (status === "running") {
      return (
        <span className="mt-1 flex h-3 w-3 shrink-0 items-center justify-center" title={t("ai.sidebar.status.running")}>
          <LoaderCircle className="h-3 w-3 animate-spin text-sky-500" aria-hidden="true" />
        </span>
      );
    }

    if (status) {
      return (
        <span
          className={cn("mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full", statusClassNames[status])}
          title={t(`ai.sidebar.status.${status}`)}
        />
      );
    }

    if (isBlankSession) {
      return <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground/35" />;
    }

    return <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-transparent" />;
  };

  return (
    <div
      className="flex h-full flex-col"
      role="tablist"
      aria-orientation="vertical"
      aria-label={t("ai.sidebar.sessions")}
    >
      <div className="flex items-center justify-between border-b border-panel-divider/70 px-3 py-2">
        <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground/65">
          {t("ai.sidebar.sessions")}
        </span>
        <span className="rounded-full border border-panel-divider/70 bg-background/60 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground/85">
          {tabs.length}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
        <div className="space-y-1">
          {tabs.map((tab) => {
            const isActive = tab.id === activeTabId;
            const status = getStatus(tab.id);
            const isBlankSession = tab.conversationId == null;
            const subtitle = isBlankSession
              ? t("ai.sidebar.newChat")
              : status
                ? t(`ai.sidebar.status.${status}`)
                : null;
            const showSubtitle = Boolean(subtitle && (!compact || isActive || isBlankSession));

            return (
              <div
                key={tab.id}
                className={cn(
                  "group relative min-w-0 overflow-hidden rounded-lg text-xs transition-colors",
                  isActive
                    ? "bg-background/95 text-foreground shadow-[inset_0_0_0_1px_rgba(255,255,255,0.05)]"
                    : "bg-transparent text-muted-foreground hover:bg-background/45"
                )}
                role="presentation"
              >
                <span
                  className={cn(
                    "absolute bottom-2 left-0 top-2 w-px rounded-full transition-colors",
                    isActive ? "bg-primary/65" : "bg-transparent"
                  )}
                />
                <button
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  className={cn(
                    "flex w-full min-w-0 items-start gap-2 rounded-[inherit] pl-3 pr-8 text-left",
                    compact ? "py-2" : "py-2.5"
                  )}
                  onClick={() => onActivate(tab.id)}
                  title={tab.title || t("ai.newConversation")}
                >
                  {renderStatusIndicator(status, isBlankSession)}
                  <span className="min-w-0 flex-1">
                    <span
                      className={cn(
                        "block truncate font-medium",
                        compact ? "text-[11px] leading-5" : "text-xs leading-5",
                        isActive ? "text-foreground" : "text-foreground/92"
                      )}
                    >
                      {tab.title || t("ai.newConversation")}
                    </span>
                    {showSubtitle && (
                      <span
                        className={cn(
                          "block truncate text-muted-foreground/80",
                          compact ? "text-[10px] leading-4" : "text-[11px] leading-4"
                        )}
                      >
                        {subtitle}
                      </span>
                    )}
                  </span>
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className={cn(
                    "absolute right-1.5 top-1/2 h-5 w-5 shrink-0 -translate-y-1/2 rounded-md text-muted-foreground/70 opacity-0 transition-opacity hover:opacity-100",
                    isActive && "opacity-70"
                  )}
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
    </div>
  );
}
