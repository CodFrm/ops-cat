import { useState, useMemo } from "react";
import { MessageSquare, Plus, Search, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn, Button, Input, Popover, PopoverContent, PopoverTrigger } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";

function formatRelativeTime(timestamp: number, locale: string): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  if (diff < 60) return rtf.format(0, "second");
  if (diff < 3600) return rtf.format(-Math.floor(diff / 60), "minute");
  if (diff < 86400) return rtf.format(-Math.floor(diff / 3600), "hour");
  if (diff < 604800) return rtf.format(-Math.floor(diff / 86400), "day");
  const date = new Date(timestamp * 1000);
  return new Intl.DateTimeFormat(locale, { month: "numeric", day: "numeric" }).format(date);
}

interface SideAssistantHistoryDropdownProps {
  activeConversationId: number | null;
  onSelect: (conversationId: number) => void;
  onOpenInTab: (conversationId: number) => void;
  onClose: () => void;
}

export function SideAssistantHistoryDropdown({
  activeConversationId,
  onSelect,
  onOpenInTab,
  onClose,
}: SideAssistantHistoryDropdownProps) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language || "zh-CN";
  const { conversations, deleteConversation, sidebarTabs } = useAIStore();
  const openInTabIds = useMemo(
    () => new Set(sidebarTabs.map((tab) => tab.conversationId).filter((x): x is number => x != null)),
    [sidebarTabs]
  );

  const [query, setQuery] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  const filtered = useMemo(() => {
    if (!query) return conversations;
    const q = query.toLowerCase();
    return conversations.filter((c) => c.Title.toLowerCase().includes(q));
  }, [conversations, query]);

  return (
    <div
      data-history-dropdown=""
      className="absolute right-0 top-full z-40 mt-1 flex w-[280px] max-h-[400px] flex-col overflow-hidden rounded-md border border-panel-divider bg-popover shadow-lg"
    >
      <div className="p-2 border-b border-panel-divider shrink-0">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("ai.sidebar.historySearchPlaceholder")}
            className="pl-7 h-7 text-xs"
            autoFocus
          />
        </div>
      </div>
      {/* 用原生 overflow-y-auto，不走 Radix ScrollArea：
          Radix Viewport 依赖 height:100%，而外层只有 max-h 无具体 height 时
          百分比高度退化为 auto，永远不触发 overflow。 */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        {filtered.length === 0 ? (
          <p className="text-xs text-muted-foreground text-center py-4">{t("ai.sidebar.historyEmpty")}</p>
        ) : (
          filtered.map((conv) => {
            const isActive = activeConversationId === conv.ID;
            const isInTab = openInTabIds.has(conv.ID);
            const isDeleting = deleteTarget === conv.ID;
            return (
              <div
                key={conv.ID}
                className={cn(
                  "group flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-accent text-sm",
                  isActive && "bg-accent/50 border-l-2 border-primary"
                )}
                onClick={() => {
                  if (isDeleting) return;
                  onSelect(conv.ID);
                  onClose();
                }}
              >
                <MessageSquare className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="truncate">{conv.Title}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatRelativeTime(conv.Updatetime, locale)}
                    {isInTab && ` · ${t("ai.sidebar.promoteHint")}`}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100"
                  title={t("action.openInTab")}
                  aria-label={t("action.openInTab")}
                  onClick={(event) => {
                    event.stopPropagation();
                    onOpenInTab(conv.ID);
                  }}
                >
                  <Plus className="h-3 w-3" />
                </Button>
                <Popover open={isDeleting} onOpenChange={(open) => setDeleteTarget(open ? conv.ID : null)}>
                  <PopoverTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={cn("h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100", isDeleting && "opacity-100")}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    side="top"
                    align="end"
                    sideOffset={6}
                    className="w-auto p-3"
                    onOpenAutoFocus={(e) => e.preventDefault()}
                  >
                    <p className="text-xs mb-2">{t("ai.deleteConversationDesc")}</p>
                    <div className="flex gap-2 justify-end">
                      <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => setDeleteTarget(null)}>
                        {t("action.cancel")}
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        className="h-7 text-xs"
                        onClick={async () => {
                          await deleteConversation(conv.ID);
                          setDeleteTarget(null);
                        }}
                      >
                        {t("action.delete")}
                      </Button>
                    </div>
                  </PopoverContent>
                </Popover>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
