import { create } from "zustand";
import { useRecentAssetStore } from "./recentAssetStore";

// === Tab Types ===

export type TabType = "terminal" | "ai" | "query" | "page" | "info";

export interface TerminalTabMeta {
  type: "terminal";
  assetId: number;
  assetName: string;
  assetIcon: string;
  host: string;
  port: number;
  username: string;
}

export interface AITabMeta {
  type: "ai";
  conversationId: number | null;
  title: string;
}

export interface QueryTabMeta {
  type: "query";
  assetId: number;
  assetName: string;
  assetIcon: string;
  assetType: "database" | "redis" | "mongodb" | "kafka" | "k8s";
  driver?: string;
  defaultDatabase?: string;
  redisDatabase?: number;
  redisScanPageSize?: number;
  redisKeySeparator?: string;
}

export interface PageTabMeta {
  type: "page";
  pageId: string;
  extensionName?: string;
  assetId?: number;
}

export interface InfoTabMeta {
  type: "info";
  targetType: "asset" | "group";
  targetId: number;
  name: string;
  icon?: string;
}

export type TabMeta = TerminalTabMeta | AITabMeta | QueryTabMeta | PageTabMeta | InfoTabMeta;

export interface Tab {
  id: string;
  type: TabType;
  label: string;
  icon?: string;
  iconColor?: string;
  meta: TabMeta;
}

// === Close Hook ===

type TabCloseHook = (tab: Tab) => void;
const closeHooks: TabCloseHook[] = [];

export function registerTabCloseHook(hook: TabCloseHook) {
  closeHooks.push(hook);
}

// === Replace Hook ===

type TabReplaceHook = (oldId: string, newId: string) => void;
const replaceHooks: TabReplaceHook[] = [];

export function registerTabReplaceHook(hook: TabReplaceHook) {
  replaceHooks.push(hook);
}

// === Restore Hook ===

type TabRestoreHook = (tabs: Tab[]) => void;
const restoreHooks: Map<TabType, TabRestoreHook[]> = new Map();
let _restoreComplete = false;

/**
 * Register a hook that fires after tabs are restored from localStorage.
 * The hook is always called (even with an empty array if no tabs of that type exist).
 * If restore already completed, the hook is called immediately.
 */
export function registerTabRestoreHook(tabType: TabType, hook: TabRestoreHook) {
  if (!restoreHooks.has(tabType)) {
    restoreHooks.set(tabType, []);
  }
  restoreHooks.get(tabType)!.push(hook);

  // If restore already happened, call immediately
  if (_restoreComplete) {
    const tabs = useTabStore.getState().tabs.filter((t) => t.type === tabType);
    hook(tabs);
  }
}

function _fireRestoreHooks() {
  _restoreComplete = true;
  const allTabs = useTabStore.getState().tabs;
  for (const [type, hooks] of restoreHooks) {
    const tabs = allTabs.filter((t) => t.type === type);
    for (const hook of hooks) {
      hook(tabs);
    }
  }
}

// === Store ===

interface TabStoreState {
  tabs: Tab[];
  activeTabId: string | null;

  openTab: (tab: Tab, activate?: boolean) => void;
  closeTab: (id: string) => void;
  activateTab: (id: string) => void;
  updateTab: (id: string, patch: Partial<Pick<Tab, "label" | "icon" | "iconColor" | "meta">>) => void;
  replaceTabId: (oldId: string, newId: string) => void;
  reorderTab: (fromId: string, toId: string) => void;
  moveTabTo: (id: string, toIndex: number) => void;
  closeOtherTabs: (id: string) => void;
  closeLeftTabs: (id: string) => void;
  closeRightTabs: (id: string) => void;
}

export const useTabStore = create<TabStoreState>((set, get) => ({
  tabs: [],
  activeTabId: null,

  openTab: (tab, activate = true) => {
    const { tabs } = get();
    const existing = tabs.find((t) => t.id === tab.id);
    if (existing) {
      if (activate) set({ activeTabId: tab.id });
      return;
    }
    set((s) => ({
      tabs: [...s.tabs, tab],
      activeTabId: activate ? tab.id : s.activeTabId,
    }));
    // Record recently opened asset (terminal/query/page carry assetId; info uses targetId for assets)
    const { meta } = tab;
    let assetId = 0;
    if (meta.type === "terminal" || meta.type === "query") {
      assetId = meta.assetId;
    } else if (meta.type === "page" && meta.assetId != null) {
      assetId = meta.assetId;
    } else if (meta.type === "info" && meta.targetType === "asset") {
      assetId = meta.targetId;
    }
    if (assetId > 0) {
      useRecentAssetStore.getState().touch(assetId);
    }
  },

  closeTab: (id) => {
    const { tabs, activeTabId } = get();
    const idx = tabs.findIndex((t) => t.id === id);
    if (idx === -1) return;

    const tab = tabs[idx];
    // Notify business stores
    for (const hook of closeHooks) {
      hook(tab);
    }

    const newTabs = tabs.filter((t) => t.id !== id);
    let newActiveId = activeTabId;
    if (activeTabId === id) {
      // Activate neighbor
      if (newTabs.length === 0) {
        newActiveId = null;
      } else if (idx < newTabs.length) {
        newActiveId = newTabs[idx].id;
      } else {
        newActiveId = newTabs[newTabs.length - 1].id;
      }
    }
    set({ tabs: newTabs, activeTabId: newActiveId });
  },

  activateTab: (id) => {
    set({ activeTabId: id });
  },

  updateTab: (id, patch) => {
    set((s) => ({
      tabs: s.tabs.map((t) => (t.id === id ? { ...t, ...patch } : t)),
    }));
  },

  replaceTabId: (oldId, newId) => {
    set((s) => ({
      tabs: s.tabs.map((t) => (t.id === oldId ? { ...t, id: newId } : t)),
      activeTabId: s.activeTabId === oldId ? newId : s.activeTabId,
    }));
    for (const hook of replaceHooks) hook(oldId, newId);
  },

  reorderTab: (fromId, toId) => {
    const { tabs } = get();
    const fromIdx = tabs.findIndex((t) => t.id === fromId);
    const toIdx = tabs.findIndex((t) => t.id === toId);
    if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return;
    const next = [...tabs];
    const [moved] = next.splice(fromIdx, 1);
    next.splice(toIdx, 0, moved);
    set({ tabs: next });
  },

  moveTabTo: (id, toIndex) => {
    const { tabs } = get();
    const fromIdx = tabs.findIndex((t) => t.id === id);
    if (fromIdx === -1 || fromIdx === toIndex) return;
    const clamped = Math.max(0, Math.min(toIndex, tabs.length - 1));
    const next = [...tabs];
    const [moved] = next.splice(fromIdx, 1);
    next.splice(clamped, 0, moved);
    set({ tabs: next });
  },

  closeOtherTabs: (id) => {
    const { tabs } = get();
    const closingTabs = tabs.filter((t) => t.id !== id);
    for (const tab of closingTabs) {
      for (const hook of closeHooks) hook(tab);
    }
    set({ tabs: tabs.filter((t) => t.id === id), activeTabId: id });
  },

  closeLeftTabs: (id) => {
    const { tabs, activeTabId } = get();
    const idx = tabs.findIndex((t) => t.id === id);
    if (idx <= 0) return;
    const closingTabs = tabs.slice(0, idx);
    for (const tab of closingTabs) {
      for (const hook of closeHooks) hook(tab);
    }
    const newTabs = tabs.slice(idx);
    const activeStillOpen = newTabs.some((t) => t.id === activeTabId);
    set({ tabs: newTabs, activeTabId: activeStillOpen ? activeTabId : id });
  },

  closeRightTabs: (id) => {
    const { tabs, activeTabId } = get();
    const idx = tabs.findIndex((t) => t.id === id);
    if (idx === -1 || idx >= tabs.length - 1) return;
    const closingTabs = tabs.slice(idx + 1);
    for (const tab of closingTabs) {
      for (const hook of closeHooks) hook(tab);
    }
    const newTabs = tabs.slice(0, idx + 1);
    const activeStillOpen = newTabs.some((t) => t.id === activeTabId);
    set({ tabs: newTabs, activeTabId: activeStillOpen ? activeTabId : id });
  },
}));

// === Persistence ===

const STORAGE_KEY = "tab_store";

interface SavedTabStore {
  tabs: Tab[];
  activeTabId: string | null;
}

let _persistReady = false;

useTabStore.subscribe((state, prevState) => {
  if (!_persistReady) return;
  if (state.tabs !== prevState.tabs || state.activeTabId !== prevState.activeTabId) {
    const data: SavedTabStore = { tabs: state.tabs, activeTabId: state.activeTabId };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  }
});

// Migration from old localStorage keys
function _migrateOldKeys(): SavedTabStore | null {
  const hasOldKeys =
    localStorage.getItem("open_page_tabs") !== null ||
    localStorage.getItem("terminal_open_tabs") !== null ||
    localStorage.getItem("ai_open_tabs") !== null ||
    localStorage.getItem("query_open_tabs") !== null;

  if (!hasOldKeys) return null;

  const tabs: Tab[] = [];

  // Migrate terminal tabs
  try {
    const raw = localStorage.getItem("terminal_open_tabs");
    if (raw) {
      const saved = JSON.parse(raw);
      if (Array.isArray(saved)) {
        saved.forEach(
          (
            st: { assetId: number; assetName: string; assetIcon: string; host: string; port: number; username: string },
            i: number
          ) => {
            tabs.push({
              id: `restored-${st.assetId}-${i}`,
              type: "terminal",
              label: st.assetName,
              icon: st.assetIcon || undefined,
              meta: {
                type: "terminal",
                assetId: st.assetId,
                assetName: st.assetName,
                assetIcon: st.assetIcon,
                host: st.host,
                port: st.port,
                username: st.username,
              },
            });
          }
        );
      }
    }
  } catch {
    /* ignore */
  }

  // Migrate info tabs
  try {
    const raw = localStorage.getItem("terminal_info_tabs");
    if (raw) {
      const saved = JSON.parse(raw);
      if (Array.isArray(saved)) {
        saved.forEach((it: { id: string; type: "asset" | "group"; targetId: number; name: string; icon?: string }) => {
          tabs.push({
            id: it.id,
            type: "info",
            label: it.name,
            icon: it.icon,
            meta: { type: "info", targetType: it.type, targetId: it.targetId, name: it.name, icon: it.icon },
          });
        });
      }
    }
  } catch {
    /* ignore */
  }

  // Migrate AI tabs (just conversation IDs — aiStore will load messages)
  try {
    const raw = localStorage.getItem("ai_open_tabs");
    if (raw) {
      const convIds = JSON.parse(raw);
      if (Array.isArray(convIds)) {
        convIds.forEach((convId: number) => {
          tabs.push({
            id: `ai-${convId}`,
            type: "ai",
            label: `Conversation ${convId}`,
            meta: { type: "ai", conversationId: convId, title: `Conversation ${convId}` },
          });
        });
      }
    }
  } catch {
    /* ignore */
  }

  // Migrate query tabs
  try {
    const raw = localStorage.getItem("query_open_tabs");
    if (raw) {
      const saved = JSON.parse(raw);
      if (Array.isArray(saved)) {
        saved.forEach(
          (st: {
            assetId: number;
            assetName: string;
            assetIcon: string;
            assetType: "database" | "redis" | "mongodb" | "kafka" | "k8s";
            driver?: string;
            defaultDatabase?: string;
          }) => {
            tabs.push({
              id: `query-${st.assetId}`,
              type: "query",
              label: st.assetName,
              icon: st.assetIcon || undefined,
              meta: {
                type: "query",
                assetId: st.assetId,
                assetName: st.assetName,
                assetIcon: st.assetIcon,
                assetType: st.assetType,
                driver: st.driver,
                defaultDatabase: st.defaultDatabase,
              },
            });
          }
        );
      }
    }
  } catch {
    /* ignore */
  }

  // Migrate page tabs
  try {
    const raw = localStorage.getItem("open_page_tabs");
    if (raw) {
      const saved = JSON.parse(raw);
      if (Array.isArray(saved)) {
        saved.forEach((pageId: string) => {
          tabs.push({
            id: pageId,
            type: "page",
            label: pageId,
            meta: { type: "page", pageId },
          });
        });
      }
    }
  } catch {
    /* ignore */
  }

  // Determine active tab
  let activeTabId: string | null = null;
  const lastActive = localStorage.getItem("last_active_tab");
  if (lastActive) {
    if (lastActive.startsWith("ai:")) {
      // old format: "ai:{tabId}" where tabId is like "ai-{convId}"
      const aiTabId = lastActive.slice(3);
      if (tabs.some((t) => t.id === aiTabId)) activeTabId = aiTabId;
    } else if (lastActive.startsWith("query:")) {
      const assetId = lastActive.slice(6);
      const qId = `query-${assetId}`;
      if (tabs.some((t) => t.id === qId)) activeTabId = qId;
    } else if (lastActive && !lastActive.startsWith("ai:") && !lastActive.startsWith("query:")) {
      // Page tab or empty
      if (tabs.some((t) => t.id === lastActive)) activeTabId = lastActive;
    }
  }
  // Check terminal active
  if (!activeTabId && (lastActive === "" || lastActive === null)) {
    try {
      const savedIdx = Number(localStorage.getItem("terminal_active_tab_idx"));
      const terminalTabs = tabs.filter((t) => t.type === "terminal");
      if (savedIdx >= 0 && savedIdx < terminalTabs.length) {
        activeTabId = terminalTabs[savedIdx].id;
      }
    } catch {
      /* ignore */
    }
  }
  // AI active
  if (!activeTabId) {
    try {
      const savedConv = Number(localStorage.getItem("ai_active_tab_conv"));
      if (savedConv) {
        const aiTab = tabs.find((t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === savedConv);
        if (aiTab) activeTabId = aiTab.id;
      }
    } catch {
      /* ignore */
    }
  }

  // Clean up old keys
  const oldKeys = [
    "open_page_tabs",
    "last_active_tab",
    "terminal_open_tabs",
    "terminal_active_tab_idx",
    "terminal_info_tabs",
    "ai_open_tabs",
    "ai_active_tab_conv",
    "query_open_tabs",
    "tab_order",
  ];
  for (const key of oldKeys) localStorage.removeItem(key);

  return { tabs, activeTabId };
}

// Restore on startup
(function _restoreTabStore() {
  // Try new format first
  const raw = localStorage.getItem(STORAGE_KEY);
  if (raw) {
    try {
      const data: SavedTabStore = JSON.parse(raw);
      if (data.tabs && Array.isArray(data.tabs)) {
        useTabStore.setState({ tabs: data.tabs, activeTabId: data.activeTabId });
        _persistReady = true;
        _fireRestoreHooks();
        return;
      }
    } catch {
      /* ignore */
    }
  }

  // Try migration from old format
  const migrated = _migrateOldKeys();
  if (migrated) {
    useTabStore.setState(migrated);
    // Save in new format immediately
    localStorage.setItem(STORAGE_KEY, JSON.stringify(migrated));
  }

  _persistReady = true;
  _fireRestoreHooks();
})();
