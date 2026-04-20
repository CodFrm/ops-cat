import { useState, useMemo } from "react";
import { MessageSquare, Search, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn, ScrollArea, Button, Input, ConfirmDialog } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";

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
  onClose: () => void;
}

export function SideAssistantHistoryDropdown({
  activeConversationId,
  onSelect,
  onClose,
}: SideAssistantHistoryDropdownProps) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language || "zh-CN";
  const { conversations, deleteConversation } = useAIStore();
  const tabs = useTabStore((s) => s.tabs);
  const openInTabIds = useMemo(
    () =>
      new Set(
        tabs
          .filter((tb) => tb.type === "ai")
          .map((tb) => (tb.meta as AITabMeta).conversationId)
          .filter((x): x is number => x != null)
      ),
    [tabs]
  );

  const [query, setQuery] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  const filtered = useMemo(() => {
    if (!query) return conversations;
    const q = query.toLowerCase();
    return conversations.filter((c) => c.Title.toLowerCase().includes(q));
  }, [conversations, query]);

  return (
    <>
      <div
        data-history-dropdown=""
        className="absolute right-0 top-full z-40 mt-1 w-[280px] max-h-[400px] overflow-hidden rounded-md border border-panel-divider bg-popover shadow-lg"
      >
        <div className="p-2 border-b border-panel-divider">
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
        <ScrollArea className="max-h-[320px]">
          {filtered.length === 0 ? (
            <p className="text-xs text-muted-foreground text-center py-4">{t("ai.sidebar.historyEmpty")}</p>
          ) : (
            filtered.map((conv) => {
              const isActive = activeConversationId === conv.ID;
              const isInTab = openInTabIds.has(conv.ID);
              return (
                <div
                  key={conv.ID}
                  className={cn(
                    "group flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-accent text-sm",
                    isActive && "bg-accent/50 border-l-2 border-primary"
                  )}
                  onClick={() => {
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
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteTarget(conv.ID);
                    }}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              );
            })
          )}
        </ScrollArea>
      </div>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("ai.deleteConversationTitle")}
        description={t("ai.deleteConversationDesc")}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={async () => {
          if (deleteTarget !== null) {
            await deleteConversation(deleteTarget);
            setDeleteTarget(null);
          }
        }}
      />
    </>
  );
}
