import { LoaderCircle, X, ChevronsRight, ChevronsLeft, Plus } from "lucide-react";
import { cn, Button, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@opskat/ui";
import { useTranslation } from "react-i18next";
import type { SidebarAITab, SidebarTabStatus } from "@/stores/aiStore";
import { getSessionIconColor, getSessionIconLetter } from "./sessionIconColor";

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
