import { useRef, useState, useEffect } from "react";
import { cn, useResizeHandle } from "@opskat/ui";
import { useAIStore, type MentionRef } from "@/stores/aiStore";
import { useTabStore } from "@/stores/tabStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { SideAssistantHeader } from "./SideAssistantHeader";
import { SideAssistantContextBar } from "./SideAssistantContextBar";
import { SideAssistantHistoryDropdown } from "./SideAssistantHistoryDropdown";
import { AIChatContent } from "./AIChatContent";
import { Trans } from "react-i18next";
import { History } from "lucide-react";

interface SideAssistantPanelProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function SideAssistantPanel({ collapsed, onToggle }: SideAssistantPanelProps) {
  const isFullscreen = useFullscreen();
  const {
    sidebarConversationId,
    configured,
    fetchConversations,
    bindSidebar,
    createAndBindSidebarConversation,
    promoteSidebarToTab,
    sendFromSidebar,
    stopConversation,
  } = useAIStore();

  const [historyOpen, setHistoryOpen] = useState(false);

  const panelRef = useRef<HTMLDivElement>(null);
  const {
    size: width,
    isResizing: resizing,
    handleMouseDown: handleResizeStart,
  } = useResizeHandle({
    defaultSize: 360,
    minSize: 280,
    maxSize: 520,
    reverse: true,
    storageKey: "ai_sidebar_width",
    targetRef: panelRef,
  });

  useEffect(() => {
    if (configured) fetchConversations();
  }, [configured, fetchConversations]);

  // Close history dropdown on click outside the popup (but not the trigger,
  // which manages its own toggle).
  useEffect(() => {
    if (!historyOpen) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (!target) return;
      if (target.closest("[data-history-dropdown]")) return;
      if (target.closest("[data-history-trigger]")) return;
      // 忽略从下拉中弹出的 Popover / Dialog —— 它们通过 portal 渲染到 body，
      // 否则会被判为"外部点击"：mousedown 关闭下拉 → 确认按钮随下拉卸载 →
      // click 事件不派发，删除永远触发不了。
      if (target.closest('[data-slot^="popover"]')) return;
      if (target.closest('[data-slot^="alert-dialog"]')) return;
      setHistoryOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [historyOpen]);

  const handleNewChat = async () => {
    await createAndBindSidebarConversation();
  };

  const handlePromote = async () => {
    await promoteSidebarToTab();
  };

  const handleHistorySelect = (convId: number) => {
    const tabStore = useTabStore.getState();
    const existingTab = tabStore.tabs.find(
      (tb) => tb.type === "ai" && (tb.meta as { conversationId: number | null }).conversationId === convId
    );
    if (existingTab) {
      tabStore.activateTab(existingTab.id);
      return;
    }
    bindSidebar(convId);
  };

  const handleSendOverride = async (text: string, mentions?: MentionRef[]) => {
    let convId = sidebarConversationId;
    if (convId == null) {
      convId = await createAndBindSidebarConversation();
    }
    await sendFromSidebar(convId, text, mentions);
  };

  const handleStopOverride = async () => {
    if (sidebarConversationId != null) {
      await stopConversation(sidebarConversationId);
    }
  };

  if (collapsed) return null;

  return (
    <div
      ref={panelRef}
      className="relative overflow-visible shrink-0 transition-[width] duration-200"
      style={{ width }}
    >
      <div className="relative flex h-full w-full shrink-0 flex-col border-l border-panel-divider bg-sidebar">
        <div
          className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
          onMouseDown={handleResizeStart}
        />
        {resizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}

        <div
          className={cn("w-full shrink-0", isFullscreen ? "h-0" : "h-8")}
          style={{ "--wails-draggable": "drag" } as React.CSSProperties}
        />

        <div className="relative">
          <SideAssistantHeader
            onToggleCollapse={onToggle}
            onOpenHistory={() => setHistoryOpen((x) => !x)}
            onNewChat={handleNewChat}
            onPromoteToTab={handlePromote}
            canPromote={sidebarConversationId != null}
          />
          {historyOpen && (
            <SideAssistantHistoryDropdown
              activeConversationId={sidebarConversationId}
              onSelect={handleHistorySelect}
              onClose={() => setHistoryOpen(false)}
            />
          )}
        </div>

        <SideAssistantContextBar conversationId={sidebarConversationId} />

        {sidebarConversationId == null ? (
          <div className="flex-1 flex items-center justify-center p-4 text-center text-sm text-muted-foreground">
            <Trans
              i18nKey="ai.sidebar.emptyGuide"
              components={{
                history: <History className="inline-block h-3.5 w-3.5 mx-0.5 align-text-bottom" />,
              }}
            />
          </div>
        ) : (
          <div className="flex-1 min-h-0 flex flex-col">
            <AIChatContent
              conversationId={sidebarConversationId}
              compact
              onSendOverride={handleSendOverride}
              onStopOverride={handleStopOverride}
            />
          </div>
        )}
      </div>
    </div>
  );
}
