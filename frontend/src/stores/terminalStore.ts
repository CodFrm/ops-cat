import { create } from "zustand";
import {
  ConnectSSH,
  DisconnectSSH,
  SplitSSH,
} from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";

// Split tree types
export type SplitNode =
  | { type: "terminal"; sessionId: string }
  | { type: "pending"; pendingId: string }
  | {
      type: "split";
      direction: "horizontal" | "vertical";
      ratio: number;
      first: SplitNode;
      second: SplitNode;
    };

export interface TerminalPane {
  sessionId: string;
  connected: boolean;
}

export interface TerminalTab {
  id: string; // tab id (first session's ID)
  assetId: number;
  assetName: string;
  splitTree: SplitNode;
  activePaneId: string;
  panes: Record<string, TerminalPane>;
}

// Helper: get all session IDs from a split tree (skips pending)
export function getSessionIds(node: SplitNode): string[] {
  if (node.type === "terminal") return [node.sessionId];
  if (node.type === "pending") return [];
  return [...getSessionIds(node.first), ...getSessionIds(node.second)];
}

// Helper: replace a leaf node (terminal or pending) by ID
function replaceNode(
  tree: SplitNode,
  id: string,
  replacement: SplitNode
): SplitNode {
  if (tree.type === "terminal" && tree.sessionId === id) return replacement;
  if (tree.type === "pending" && tree.pendingId === id) return replacement;
  if (tree.type === "split") {
    return {
      ...tree,
      first: replaceNode(tree.first, id, replacement),
      second: replaceNode(tree.second, id, replacement),
    };
  }
  return tree;
}

// Helper: remove a leaf node, collapsing parent split
function removeNode(tree: SplitNode, id: string): SplitNode | null {
  if (tree.type === "terminal" && tree.sessionId === id) return null;
  if (tree.type === "pending" && tree.pendingId === id) return null;
  if (tree.type === "split") {
    const newFirst = removeNode(tree.first, id);
    const newSecond = removeNode(tree.second, id);
    if (newFirst === null) return newSecond;
    if (newSecond === null) return newFirst;
    if (newFirst === tree.first && newSecond === tree.second) return tree;
    return { ...tree, first: newFirst, second: newSecond };
  }
  return tree;
}

// Helper: update ratio at path
function setRatioAtPath(
  tree: SplitNode,
  path: number[],
  ratio: number
): SplitNode {
  if (path.length === 0 && tree.type === "split") {
    return { ...tree, ratio };
  }
  if (tree.type === "split" && path.length > 0) {
    const [head, ...rest] = path;
    if (head === 0)
      return { ...tree, first: setRatioAtPath(tree.first, rest, ratio) };
    return { ...tree, second: setRatioAtPath(tree.second, rest, ratio) };
  }
  return tree;
}

interface TerminalState {
  tabs: TerminalTab[];
  activeTabId: string | null;
  assetInfoOpen: boolean;
  connectingAssetIds: Set<number>;

  connect: (
    assetId: number,
    assetName: string,
    password: string,
    cols: number,
    rows: number
  ) => Promise<string>;
  disconnect: (sessionId: string) => void;
  setActiveTab: (id: string | null) => void;
  removeTab: (id: string) => void;
  markClosed: (id: string) => void;
  openAssetInfo: () => void;
  closeAssetInfo: () => void;

  // Split pane actions
  setActivePaneId: (tabId: string, paneId: string) => void;
  splitPane: (
    tabId: string,
    direction: "horizontal" | "vertical"
  ) => void;
  closePane: (tabId: string, sessionId: string) => void;
  setSplitRatio: (tabId: string, path: number[], ratio: number) => void;
}

export const useTerminalStore = create<TerminalState>((set, get) => ({
  tabs: [],
  activeTabId: null,
  assetInfoOpen: false,
  connectingAssetIds: new Set(),

  connect: async (assetId, assetName, password, cols, rows) => {
    if (get().connectingAssetIds.has(assetId)) {
      throw new Error("already connecting");
    }
    set((state) => ({
      connectingAssetIds: new Set(state.connectingAssetIds).add(assetId),
    }));
    try {
      const req = new main.SSHConnectRequest({
        assetId,
        password,
        key: "",
        cols,
        rows,
      });
      const sessionId = await ConnectSSH(req);

      const tab: TerminalTab = {
        id: sessionId,
        assetId,
        assetName,
        splitTree: { type: "terminal", sessionId },
        activePaneId: sessionId,
        panes: { [sessionId]: { sessionId, connected: true } },
      };
      set((state) => ({
        tabs: [...state.tabs, tab],
        activeTabId: sessionId,
      }));
      return sessionId;
    } finally {
      set((state) => {
        const next = new Set(state.connectingAssetIds);
        next.delete(assetId);
        return { connectingAssetIds: next };
      });
    }
  },

  disconnect: (sessionId) => {
    DisconnectSSH(sessionId);
    set((state) => ({
      tabs: state.tabs.map((tab) => {
        if (!tab.panes[sessionId]) return tab;
        return {
          ...tab,
          panes: {
            ...tab.panes,
            [sessionId]: { ...tab.panes[sessionId], connected: false },
          },
        };
      }),
    }));
  },

  setActiveTab: (id) => set({ activeTabId: id }),

  openAssetInfo: () => set({ assetInfoOpen: true, activeTabId: null }),

  closeAssetInfo: () => {
    const { tabs } = get();
    set({
      assetInfoOpen: false,
      activeTabId: tabs.length > 0 ? tabs[0].id : null,
    });
  },

  removeTab: (id) => {
    const tab = get().tabs.find((t) => t.id === id);
    if (tab) {
      for (const pane of Object.values(tab.panes)) {
        if (pane.connected) {
          DisconnectSSH(pane.sessionId);
        }
      }
    }
    set((state) => {
      const tabs = state.tabs.filter((t) => t.id !== id);
      const activeTabId =
        state.activeTabId === id
          ? tabs.length > 0
            ? tabs[tabs.length - 1].id
            : null
          : state.activeTabId;
      return { tabs, activeTabId };
    });
  },

  markClosed: (sessionId) => {
    set((state) => ({
      tabs: state.tabs.map((tab) => {
        if (!tab.panes[sessionId]) return tab;
        return {
          ...tab,
          panes: {
            ...tab.panes,
            [sessionId]: { ...tab.panes[sessionId], connected: false },
          },
        };
      }),
    }));
  },

  setActivePaneId: (tabId, paneId) => {
    set((state) => ({
      tabs: state.tabs.map((tab) =>
        tab.id === tabId ? { ...tab, activePaneId: paneId } : tab
      ),
    }));
  },

  splitPane: (tabId, direction) => {
    const tab = get().tabs.find((t) => t.id === tabId);
    if (!tab) return;

    const pendingId = `pending-${Date.now()}`;

    // Step 1: Immediately split UI with pending placeholder
    set((state) => {
      const tab = state.tabs.find((t) => t.id === tabId);
      if (!tab) return state;

      const newTree = replaceNode(tab.splitTree, tab.activePaneId, {
        type: "split",
        direction,
        ratio: 0.5,
        first: { type: "terminal", sessionId: tab.activePaneId },
        second: { type: "pending", pendingId },
      });

      return {
        tabs: state.tabs.map((t) =>
          t.id === tabId ? { ...t, splitTree: newTree } : t
        ),
      };
    });

    // Step 2: 在已有连接上创建新会话
    SplitSSH(tab.activePaneId, 80, 24)
      .then((sessionId) => {
        // Step 3: Replace pending with real terminal
        set((state) => {
          const tab = state.tabs.find((t) => t.id === tabId);
          if (!tab) return state;

          const newTree = replaceNode(tab.splitTree, pendingId, {
            type: "terminal",
            sessionId,
          });

          return {
            tabs: state.tabs.map((t) =>
              t.id === tabId
                ? {
                    ...t,
                    splitTree: newTree,
                    activePaneId: sessionId,
                    panes: {
                      ...t.panes,
                      [sessionId]: { sessionId, connected: true },
                    },
                  }
                : t
            ),
          };
        });
      })
      .catch((err) => {
        console.error("Split connection failed:", err);
        // Step 4: Remove pending node, collapse back
        set((state) => {
          const tab = state.tabs.find((t) => t.id === tabId);
          if (!tab) return state;

          const newTree = removeNode(tab.splitTree, pendingId);
          if (!newTree) return state;

          return {
            tabs: state.tabs.map((t) =>
              t.id === tabId ? { ...t, splitTree: newTree } : t
            ),
          };
        });
      });
  },

  closePane: (tabId, sessionId) => {
    const tab = get().tabs.find((t) => t.id === tabId);
    if (!tab) return;

    const pane = tab.panes[sessionId];
    if (pane?.connected) {
      DisconnectSSH(sessionId);
    }

    // If only one pane, remove entire tab
    const allSessions = getSessionIds(tab.splitTree);
    if (allSessions.length <= 1) {
      get().removeTab(tabId);
      return;
    }

    const newTree = removeNode(tab.splitTree, sessionId);
    if (!newTree) {
      get().removeTab(tabId);
      return;
    }

    const remaining = getSessionIds(newTree);
    const newActivePaneId =
      tab.activePaneId === sessionId ? remaining[0] : tab.activePaneId;

    const newPanes = { ...tab.panes };
    delete newPanes[sessionId];

    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId
          ? {
              ...t,
              splitTree: newTree,
              activePaneId: newActivePaneId,
              panes: newPanes,
            }
          : t
      ),
    }));
  },

  setSplitRatio: (tabId, path, ratio) => {
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId
          ? { ...t, splitTree: setRatioAtPath(t.splitTree, path, ratio) }
          : t
      ),
    }));
  },
}));
