import { useState, useEffect, useRef } from "react";
import { Bot } from "lucide-react";
import { cn, useResizeHandle } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore } from "@/stores/tabStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { SideAssistantHeader } from "./SideAssistantHeader";
import { SideAssistantContextBar } from "./SideAssistantContextBar";
import { SideAssistantHistoryDropdown } from "./SideAssistantHistoryDropdown";
import { AIChatContent } from "./AIChatContent";
import { useTranslation } from "react-i18next";

interface SideAssistantPanelProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function SideAssistantPanel({ collapsed, onToggle }: SideAssistantPanelProps) {
  const { t } = useTranslation();
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
  const rootRef = useRef<HTMLDivElement>(null);

  const {
    width,
    isResizing: resizing,
    handleMouseDown: handleResizeStart,
  } = useResizeHandle({
    defaultWidth: 360,
    minWidth: 280,
    maxWidth: 520,
    reverse: true,
    storageKey: "ai_sidebar_width",
  });

  useEffect(() => {
    if (configured) fetchConversations();
  }, [configured, fetchConversations]);

  // Close history dropdown on click outside
  useEffect(() => {
    if (!historyOpen) return;
    const handler = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setHistoryOpen(false);
      }
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

  const handleSendOverride = async (text: string) => {
    let convId = sidebarConversationId;
    if (convId == null) {
      convId = await createAndBindSidebarConversation();
    }
    await sendFromSidebar(convId, text);
  };

  const handleStopOverride = async () => {
    if (sidebarConversationId != null) {
      await stopConversation(sidebarConversationId);
    }
  };

  if (collapsed) {
    return (
      <div
        className="shrink-0 border-l border-panel-divider bg-sidebar flex flex-col items-center py-2 cursor-pointer hover:bg-sidebar-accent transition-colors"
        style={{ width: 32 }}
        onClick={onToggle}
        title={t("ai.sidebar.expand")}
      >
        <Bot className="h-4 w-4 text-primary mt-1" />
      </div>
    );
  }

  return (
    <div ref={rootRef} className="relative overflow-visible shrink-0 transition-[width] duration-200" style={{ width }}>
      <div
        className="relative flex h-full shrink-0 flex-col border-l border-panel-divider bg-sidebar"
        style={{ width }}
      >
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
            {t("ai.sidebar.emptyGuide")}
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
