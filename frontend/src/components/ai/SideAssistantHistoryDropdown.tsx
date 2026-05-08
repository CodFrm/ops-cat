import { useMemo, useRef, useState } from "react";
import { Check, MessageSquare, Pencil, Plus, Search, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn, Button, Input, Popover, PopoverContent, PopoverTrigger } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";

// 取路径最后一段作为简短标识（POSIX 与 Windows 分隔符兼容）。
function basename(path: string): string {
  if (!path) return "";
  const trimmed = path.replace(/[\\/]+$/, "");
  const idx = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return idx >= 0 ? trimmed.slice(idx + 1) : trimmed;
}

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
  // 单字段 selector：避免整库订阅，下拉打开期间不会因别处 store 改动反复重渲。
  const conversations = useAIStore((s) => s.conversations);
  const deleteConversation = useAIStore((s) => s.deleteConversation);
  const renameConversation = useAIStore((s) => s.renameConversation);
  const sidebarTabs = useAIStore((s) => s.sidebarTabs);
  const workspaceTabs = useTabStore((s) => s.tabs);
  const openInTabIds = useMemo(() => {
    const ids = new Set<number>();
    for (const tab of sidebarTabs) {
      if (tab.conversationId != null) ids.add(tab.conversationId);
    }
    for (const tab of workspaceTabs) {
      if (tab.type !== "ai") continue;
      const convId = (tab.meta as AITabMeta).conversationId;
      if (convId != null) ids.add(convId);
    }
    return ids;
  }, [sidebarTabs, workspaceTabs]);

  const [query, setQuery] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);
  const [renameTarget, setRenameTarget] = useState<number | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  const [renameSaving, setRenameSaving] = useState(false);
  const renameSavingRef = useRef(false);
  const renameSessionRef = useRef(0);

  const filtered = useMemo(() => {
    if (!query) return conversations;
    const q = query.toLowerCase();
    return conversations.filter((c) => c.Title.toLowerCase().includes(q));
  }, [conversations, query]);

  const cancelRename = () => {
    if (renameSavingRef.current) return;
    renameSessionRef.current += 1;
    setRenameTarget(null);
    setRenameDraft("");
    renameSavingRef.current = false;
    setRenameSaving(false);
  };

  const startRename = (conversationId: number, title: string) => {
    if (renameSavingRef.current) return;
    renameSessionRef.current += 1;
    setDeleteTarget(null);
    setRenameTarget(conversationId);
    setRenameDraft(title);
  };

  const submitRename = async (conversationId: number) => {
    if (renameSavingRef.current) return;
    const renameSession = renameSessionRef.current;
    renameSavingRef.current = true;
    setRenameSaving(true);
    const renamed = await renameConversation(conversationId, renameDraft);
    if (renameSessionRef.current !== renameSession) {
      return;
    }
    renameSavingRef.current = false;
    setRenameSaving(false);
    if (renamed) {
      renameSessionRef.current += 1;
      cancelRename();
    }
  };

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
            const isRenaming = renameTarget === conv.ID;
            return (
              <div
                key={conv.ID}
                className={cn(
                  "group flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-accent text-sm",
                  isActive && "bg-accent/50 border-l-2 border-primary"
                )}
                onClick={() => {
                  if (isDeleting || isRenaming) return;
                  onSelect(conv.ID);
                  onClose();
                }}
              >
                <MessageSquare className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  {isRenaming ? (
                    <div className="flex items-center gap-1" onClick={(event) => event.stopPropagation()}>
                      <Input
                        value={renameDraft}
                        onChange={(event) => setRenameDraft(event.target.value)}
                        onKeyDown={(event) => {
                          if ((event.nativeEvent as KeyboardEvent).isComposing) {
                            return;
                          }
                          if (event.key === "Enter") {
                            event.preventDefault();
                            void submitRename(conv.ID);
                          }
                          if (event.key === "Escape") {
                            event.preventDefault();
                            cancelRename();
                          }
                        }}
                        className="h-7 text-xs"
                        autoFocus
                        placeholder={t("ai.renameConversationPlaceholder")}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 shrink-0"
                        onClick={() => void submitRename(conv.ID)}
                        title={t("action.save")}
                        aria-label={t("action.save")}
                        disabled={renameSaving}
                      >
                        <Check className="h-3 w-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 shrink-0"
                        onClick={cancelRename}
                        title={t("action.cancel")}
                        aria-label={t("action.cancel")}
                        disabled={renameSaving}
                      >
                        <X className="h-3 w-3" />
                      </Button>
                    </div>
                  ) : (
                    <>
                      <p className="truncate">{conv.Title}</p>
                      <p className="text-xs text-muted-foreground truncate">
                        {formatRelativeTime(conv.Updatetime, locale)}
                        {conv.WorkDir && ` · ${basename(conv.WorkDir)}`}
                        {isInTab && ` · ${t("ai.sidebar.promoteHint")}`}
                      </p>
                    </>
                  )}
                </div>
                {!isRenaming && (
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
                )}
                {!isRenaming && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100"
                    onClick={(event) => {
                      event.stopPropagation();
                      startRename(conv.ID, conv.Title);
                    }}
                    title={t("ai.renameConversation")}
                    aria-label={t("ai.renameConversation")}
                  >
                    <Pencil className="h-3 w-3" />
                  </Button>
                )}
                {!isRenaming && (
                  <Popover open={isDeleting} onOpenChange={(open) => setDeleteTarget(open ? conv.ID : null)}>
                    <PopoverTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className={cn(
                          "h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100",
                          isDeleting && "opacity-100"
                        )}
                        title={t("action.delete")}
                        aria-label={t("action.delete")}
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
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => setDeleteTarget(null)}
                        >
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
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
