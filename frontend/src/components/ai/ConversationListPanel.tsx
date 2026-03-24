import { useState, useCallback, useEffect } from "react";
import { Plus, Trash2, Bot, PanelRightClose, MessageSquare } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { cn } from "@/lib/utils";

const PANEL_MIN_WIDTH = 200;
const PANEL_MAX_WIDTH = 400;
const PANEL_DEFAULT_WIDTH = 240;

function formatRelativeTime(timestamp: number): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}天前`;
  const date = new Date(timestamp * 1000);
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

interface ConversationListPanelProps {
  collapsed: boolean;
  onToggle: () => void;
  onOpenConversation?: (tabId: string) => void;
}

export function ConversationListPanel({
  collapsed,
  onToggle,
  onOpenConversation,
}: ConversationListPanelProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const {
    conversations,
    configured,
    fetchConversations,
    openConversationTab,
    openNewConversationTab,
    deleteConversation,
    tabStates,
  } = useAIStore();
  const aiTabs = useTabStore((s) => s.tabs.filter((t) => t.type === "ai"));

  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  const [width, setWidth] = useState(() => {
    const saved = localStorage.getItem("ai_panel_width");
    return saved
      ? Math.max(PANEL_MIN_WIDTH, Math.min(PANEL_MAX_WIDTH, Number(saved)))
      : PANEL_DEFAULT_WIDTH;
  });
  const [resizing, setResizing] = useState(false);

  // 初始加载会话列表
  useEffect(() => {
    if (configured) {
      fetchConversations();
    }
  }, [configured, fetchConversations]);

  const handleResizeStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setResizing(true);
      const startX = e.clientX;
      const startWidth = width;

      const onMouseMove = (ev: MouseEvent) => {
        const delta = startX - ev.clientX;
        const newWidth = Math.max(
          PANEL_MIN_WIDTH,
          Math.min(PANEL_MAX_WIDTH, startWidth + delta)
        );
        setWidth(newWidth);
      };

      const onMouseUp = () => {
        setResizing(false);
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        setWidth((w) => {
          localStorage.setItem("ai_panel_width", String(w));
          return w;
        });
      };

      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    [width]
  );

  const handleOpenConversation = async (conversationId: number) => {
    try {
      const tabId = await openConversationTab(conversationId);
      onOpenConversation?.(tabId);
    } catch {
      // 打开失败
    }
  };

  const handleNewConversation = () => {
    const tabId = openNewConversationTab();
    onOpenConversation?.(tabId);
  };

  // 仅在该会话的 tab 正在发送时阻止删除
  const isConvSending = (convId: number) => {
    const tab = aiTabs.find((t) => (t.meta as AITabMeta).conversationId === convId);
    if (!tab) return false;
    return tabStates[tab.id]?.sending || false;
  };

  const handleDelete = (e: React.MouseEvent, id: number) => {
    e.stopPropagation();
    if (isConvSending(id)) return;
    setDeleteTarget(id);
  };

  const handleConfirmDelete = async () => {
    if (deleteTarget !== null) {
      await deleteConversation(deleteTarget);
      setDeleteTarget(null);
    }
  };

  // 已打开的会话 ID 集合
  const openConversationIds = new Set(
    aiTabs.map((t) => (t.meta as AITabMeta).conversationId).filter(Boolean)
  );

  return (
    <>
      <div
        className="relative overflow-hidden shrink-0 transition-[width] duration-200"
        style={{ width: collapsed ? 0 : width }}
      >
        <div
          className="relative flex h-full shrink-0 flex-col border-l border-panel-divider bg-sidebar"
          style={{ width }}
        >
          {/* Resize handle */}
          <div
            className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
            onMouseDown={handleResizeStart}
          />
          {resizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}

          {/* Drag region */}
          <div
            className={`${isFullscreen ? "h-2" : "h-10"} w-full shrink-0`}
            style={{ "--wails-draggable": "drag" } as React.CSSProperties}
          />

          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-panel-divider">
            <div className="flex items-center gap-1.5">
              <Bot className="h-3.5 w-3.5 text-primary" />
              <span className="text-sm font-medium">{t("ai.title")}</span>
            </div>
            <div className="flex gap-0.5">
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={handleNewConversation}
                title={t("ai.newConversation", "新对话")}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={onToggle}
              >
                <PanelRightClose className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          {/* 会话列表 */}
          <ScrollArea className="flex-1 min-h-0">
            {conversations.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-6">
                {t("ai.noConversations", "暂无对话")}
              </p>
            ) : (
              <div className="py-1">
                {conversations.map((conv) => {
                  const isOpen = openConversationIds.has(conv.ID);
                  return (
                    <div
                      key={conv.ID}
                      className={cn(
                        "group flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-sidebar-accent transition-colors",
                        isOpen && "bg-sidebar-accent/60"
                      )}
                      onClick={() => handleOpenConversation(conv.ID)}
                    >
                      <MessageSquare className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm truncate text-sidebar-foreground">
                          {conv.Title}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {formatRelativeTime(conv.Updatetime)}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
                        onClick={(e) => handleDelete(e, conv.ID)}
                        disabled={isConvSending(conv.ID)}
                      >
                        <Trash2 className="h-3 w-3" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
          </ScrollArea>
        </div>
      </div>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ai.deleteConversationTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("ai.deleteConversationDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("action.cancel")}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleConfirmDelete}>{t("action.delete")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
