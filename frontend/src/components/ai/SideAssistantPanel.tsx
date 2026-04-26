import { useRef, useState, useEffect } from "react";
import { cn, useResizeHandle } from "@opskat/ui";
import { useAIStore, type MentionRef } from "@/stores/aiStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { SideAssistantHeader } from "./SideAssistantHeader";
import { SideAssistantContextBar } from "./SideAssistantContextBar";
import { SideAssistantHistoryDropdown } from "./SideAssistantHistoryDropdown";
import { SideAssistantTabBar } from "./SideAssistantTabBar";
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
    sidebarTabs,
    activeSidebarTabId,
    configured,
    fetchConversations,
    getSidebarTabStatus,
    openNewSidebarTab,
    bindSidebarTabToConversation,
    openSidebarConversationInSidebar,
    activateSidebarTab,
    closeSidebarTab,
    promoteSidebarToTab,
    sendFromSidebarTab,
    stopSidebarTab,
  } = useAIStore();
  const activeSidebarTab = sidebarTabs.find((tab) => tab.id === activeSidebarTabId) ?? null;
  const activeConversationId = activeSidebarTab?.conversationId ?? null;

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
  const railRef = useRef<HTMLDivElement>(null);

  // rail 折叠状态独立持久化（与面板宽解耦）
  const [railCollapsed, setRailCollapsed] = useState<boolean>(() => {
    // 默认窄态：仅当显式存了 "false" 才展开；null/损坏值都按 collapsed=true 处理
    return localStorage.getItem("ai_sidebar_rail_collapsed") !== "false";
  });
  const toggleRailCollapsed = () => {
    setRailCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem("ai_sidebar_rail_collapsed", String(next));
      return next;
    });
  };

  // 宽态 rail 自身的宽度（仅 !railCollapsed 时启用拖拽）
  const {
    size: railExpandedWidth,
    isResizing: railResizing,
    handleMouseDown: handleRailResizeStart,
  } = useResizeHandle({
    defaultSize: 150,
    minSize: 120,
    maxSize: 220,
    reverse: true,
    storageKey: "ai_sidebar_rail_width",
    targetRef: railRef,
  });

  const railRenderWidth = railCollapsed ? 36 : railExpandedWidth;

  useEffect(() => {
    if (configured) fetchConversations();
  }, [configured, fetchConversations]);

  // 点击历史下拉外部时关闭弹层，但触发按钮本身仍由自己的 toggle 控制。
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

  const handleNewChat = () => {
    openNewSidebarTab();
  };

  const handlePromote = async () => {
    if (activeSidebarTabId) {
      await promoteSidebarToTab(activeSidebarTabId);
    }
  };

  const handleHistorySelect = (convId: number) => {
    if (activeSidebarTabId) {
      bindSidebarTabToConversation(activeSidebarTabId, convId);
    } else {
      openSidebarConversationInSidebar(convId);
    }
    setHistoryOpen(false);
  };

  const handleHistoryOpenInTab = (convId: number) => {
    // 已经在侧边打开同会话时直接跳转，避免重复创建宿主。
    openSidebarConversationInSidebar(convId);
    setHistoryOpen(false);
  };

  const handleSendOverride = async (text: string, mentions?: MentionRef[]) => {
    if (!activeSidebarTabId) {
      return;
    }
    await sendFromSidebarTab(activeSidebarTabId, text, mentions);
  };

  const handleStopOverride = async () => {
    if (activeSidebarTabId) {
      await stopSidebarTab(activeSidebarTabId);
    }
  };

  return (
    <div
      ref={panelRef}
      className="relative overflow-hidden shrink-0 transition-[width] duration-200"
      style={{ width: collapsed ? 0 : width }}
    >
      <div
        className="relative flex h-full shrink-0 flex-col border-l border-panel-divider bg-sidebar"
        style={{ width }}
      >
        {!collapsed && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
            onMouseDown={handleResizeStart}
          />
        )}
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
            canPromote={activeConversationId != null}
          />
          {historyOpen && (
            <SideAssistantHistoryDropdown
              activeConversationId={activeConversationId}
              onSelect={handleHistorySelect}
              onOpenInTab={handleHistoryOpenInTab}
              onClose={() => setHistoryOpen(false)}
            />
          )}
        </div>

        <div className="flex min-h-0 flex-1" data-ai-session-layout="rail-right">
          <div className="flex min-w-0 flex-1 flex-col">
            <SideAssistantContextBar conversationId={activeConversationId} />

            {!activeSidebarTab ? (
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
                  sideTabId={activeSidebarTab.id}
                  conversationId={activeConversationId}
                  compact
                  onSendOverride={handleSendOverride}
                  onStopOverride={handleStopOverride}
                />
              </div>
            )}
          </div>

          {sidebarTabs.length > 0 && (
            <aside
              ref={railRef}
              className="relative min-h-0 shrink-0 border-l border-panel-divider/70 bg-sidebar/65"
              style={{ width: railRenderWidth }}
              data-ai-session-rail="right"
            >
              {!railCollapsed && (
                <div
                  className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
                  onMouseDown={handleRailResizeStart}
                />
              )}
              {railResizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}
              <SideAssistantTabBar
                tabs={sidebarTabs}
                activeTabId={activeSidebarTabId}
                getStatus={getSidebarTabStatus}
                collapsed={railCollapsed}
                onActivate={activateSidebarTab}
                onClose={closeSidebarTab}
                onNewChat={handleNewChat}
                onToggleCollapsed={toggleRailCollapsed}
              />
            </aside>
          )}
        </div>
      </div>
    </div>
  );
}
