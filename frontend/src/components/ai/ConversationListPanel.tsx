import { useState, useEffect, useMemo } from "react";
import { Plus, Trash2, Bot, PanelRightClose, MessageSquare } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn, Button, ScrollArea, ConfirmDialog } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { useResizeHandle } from "@opskat/ui";

// resize constants kept near usage for clarity

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

export function ConversationListPanel({ collapsed, onToggle, onOpenConversation }: ConversationListPanelProps) {
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
  const tabs = useTabStore((s) => s.tabs);
  const aiTabs = useMemo(() => tabs.filter((t) => t.type === "ai"), [tabs]);

  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  const {
    width,
    isResizing: resizing,
    handleMouseDown: handleResizeStart,
  } = useResizeHandle({
    defaultWidth: 240,
    minWidth: 200,
    maxWidth: 400,
    reverse: true,
    storageKey: "ai_panel_width",
  });

  // 初始加载会话列表
  useEffect(() => {
    if (configured) {
      fetchConversations();
    }
  }, [configured, fetchConversations]);

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
  const openConversationIds = new Set(aiTabs.map((t) => (t.meta as AITabMeta).conversationId).filter(Boolean));

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
            className={`${isFullscreen ? "h-0" : "h-8"} w-full shrink-0`}
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
              <Button variant="ghost" size="icon" className="h-6 w-6" onClick={onToggle}>
                <PanelRightClose className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          {/* 会话列表 */}
          <ScrollArea className="flex-1 min-h-0">
            {conversations.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-6">{t("ai.noConversations", "暂无对话")}</p>
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
                        <p className="text-sm truncate text-sidebar-foreground">{conv.Title}</p>
                        <p className="text-xs text-muted-foreground">{formatRelativeTime(conv.Updatetime)}</p>
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

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("ai.deleteConversationTitle")}
        description={t("ai.deleteConversationDesc")}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={handleConfirmDelete}
      />
    </>
  );
}
