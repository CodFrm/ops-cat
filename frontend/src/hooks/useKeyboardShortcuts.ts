import { useEffect } from "react";
import { useTerminalStore } from "@/stores/terminalStore";
import { useTabStore } from "@/stores/tabStore";
import { useShortcutStore, matchShortcut } from "@/stores/shortcutStore";
import { useLayoutStore } from "@/stores/layoutStore";

interface ShortcutHandlers {
  onToggleAIPanel: () => void;
  onToggleSidebar: () => void;
}

export function useKeyboardShortcuts({ onToggleAIPanel, onToggleSidebar }: ShortcutHandlers) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const { shortcuts, isRecording } = useShortcutStore.getState();
      if (isRecording) return;

      const target = e.target as HTMLElement;
      if (
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable) &&
        !target.closest(".xterm")
      ) {
        return;
      }

      const action = matchShortcut(e, shortcuts);
      if (!action) return;

      // panel.filter 特殊处理：xterm 聚焦时透传，否则通过 store 触发 filter 面板
      if (action === "panel.filter") {
        const el = document.activeElement as HTMLElement | null;
        if (el?.closest(".xterm")) return;
        e.preventDefault();
        e.stopPropagation();
        useLayoutStore.getState().requestOpenFilter();
        return;
      }

      e.preventDefault();
      e.stopPropagation();

      const tabStore = useTabStore.getState();
      const { tabs, activeTabId } = tabStore;

      // Tab switching: tab.1 ~ tab.9
      const tabMatch = action.match(/^tab\.(\d)$/);
      if (tabMatch) {
        const idx = parseInt(tabMatch[1]) - 1;
        if (idx < tabs.length) {
          tabStore.activateTab(tabs[idx].id);
        }
        return;
      }

      switch (action) {
        case "tab.close": {
          if (activeTabId) {
            const activeTab = tabs.find((t) => t.id === activeTabId);
            if (activeTab?.type === "terminal") {
              // Close terminal pane (not tab) if there are splits
              const termStore = useTerminalStore.getState();
              const data = termStore.tabData[activeTabId];
              if (data) {
                termStore.closePane(activeTabId, data.activePaneId);
              }
            } else {
              tabStore.closeTab(activeTabId);
            }
          }
          break;
        }
        case "tab.prev": {
          if (tabs.length === 0) break;
          const curIdx = tabs.findIndex((t) => t.id === activeTabId);
          const prevIdx = curIdx <= 0 ? tabs.length - 1 : curIdx - 1;
          tabStore.activateTab(tabs[prevIdx].id);
          break;
        }
        case "tab.next": {
          if (tabs.length === 0) break;
          const curIdx = tabs.findIndex((t) => t.id === activeTabId);
          const nextIdx = curIdx >= tabs.length - 1 ? 0 : curIdx + 1;
          tabStore.activateTab(tabs[nextIdx].id);
          break;
        }
        case "split.vertical": {
          const activeTab = tabs.find((t) => t.id === activeTabId);
          if (activeTab?.type === "terminal") {
            useTerminalStore.getState().splitPane(activeTabId!, "vertical");
          }
          break;
        }
        case "split.horizontal": {
          const activeTab = tabs.find((t) => t.id === activeTabId);
          if (activeTab?.type === "terminal") {
            useTerminalStore.getState().splitPane(activeTabId!, "horizontal");
          }
          break;
        }
        case "panel.switch": {
          const st = useLayoutStore.getState();
          st.switchPanel();
          if (!st.leftPanelVisible) st.toggleVisible();
          break;
        }
        case "panel.ai":
          onToggleAIPanel();
          break;
        case "panel.sidebar": {
          const layout = useLayoutStore.getState().tabBarLayout;
          if (layout === "left") {
            useLayoutStore.getState().toggleVisible();
          } else {
            onToggleSidebar();
          }
          break;
        }
        case "page.home": {
          const homeTab = tabs.find((t) => t.type === "terminal" || t.type === "info");
          if (homeTab) tabStore.activateTab(homeTab.id);
          break;
        }
        case "page.settings": {
          const existing = tabs.find((t) => t.id === "settings");
          if (existing) {
            tabStore.activateTab("settings");
          } else {
            tabStore.openTab({
              id: "settings",
              type: "page",
              label: "settings",
              meta: { type: "page", pageId: "settings" },
            });
          }
          break;
        }
        case "page.sshkeys": {
          const existing = tabs.find((t) => t.id === "sshkeys");
          if (existing) {
            tabStore.activateTab("sshkeys");
          } else {
            tabStore.openTab({
              id: "sshkeys",
              type: "page",
              label: "sshkeys",
              meta: { type: "page", pageId: "sshkeys" },
            });
          }
          break;
        }
      }
    };

    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [onToggleAIPanel, onToggleSidebar]);
}
