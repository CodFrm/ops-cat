import { create } from "zustand";
import {
  ConnectSSHAsync,
  CancelSSHConnect,
  RespondAuthChallenge,
  DisconnectSSH,
  SplitSSH,
  UpdateAssetPassword,
} from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import { useTabStore, registerTabCloseHook, type TerminalTabMeta } from "./tabStore";

// Split tree types
export type SplitNode =
  | { type: "terminal"; sessionId: string }
  | { type: "pending"; pendingId: string }
  | { type: "connecting"; connectionId: string }
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
  connectedAt: number;
}

// Business data per terminal tab (split tree, panes, connection state)
export interface TerminalTabData {
  splitTree: SplitNode;
  activePaneId: string;
  panes: Record<string, TerminalPane>;
}

export interface SSHConnectMetadata {
  host: string;
  port: number;
  username: string;
}

export interface ConnectionLogEntry {
  message: string;
  timestamp: number;
  type: "info" | "error";
}

export type ConnectionStep = "resolve" | "connect" | "auth" | "shell";

export interface ConnectionState {
  connectionId: string;
  assetId: number;
  assetName: string;
  password: string;
  logs: ConnectionLogEntry[];
  status: "connecting" | "auth_challenge" | "connected" | "error";
  currentStep: ConnectionStep;
  error?: string;
  authFailed?: boolean;
  challenge?: {
    challengeId: string;
    prompts: string[];
    echo: boolean[];
  };
}

// Helper: get all session IDs from a split tree (skips pending/connecting)
export function getSessionIds(node: SplitNode): string[] {
  if (node.type === "terminal") return [node.sessionId];
  if (node.type === "pending" || node.type === "connecting") return [];
  return [...getSessionIds(node.first), ...getSessionIds(node.second)];
}

// Helper: replace a leaf node (terminal, pending, or connecting) by ID
function replaceNode(
  tree: SplitNode,
  id: string,
  replacement: SplitNode
): SplitNode {
  if (tree.type === "terminal" && tree.sessionId === id) return replacement;
  if (tree.type === "pending" && tree.pendingId === id) return replacement;
  if (tree.type === "connecting" && tree.connectionId === id)
    return replacement;
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
  if (tree.type === "connecting" && tree.connectionId === id) return null;
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
  // Business data keyed by tab id
  tabData: Record<string, TerminalTabData>;
  connectingAssetIds: Set<number>;
  connections: Record<string, ConnectionState>;

  connect: (
    assetId: number,
    assetName: string,
    assetIcon: string,
    password: string,
    cols: number,
    rows: number,
    metadata?: SSHConnectMetadata
  ) => Promise<string>;
  reconnect: (tabId: string) => void;
  disconnect: (sessionId: string) => void;
  markClosed: (sessionId: string) => void;

  // Connection progress actions
  retryConnect: (connectionId: string, password?: string) => void;
  respondChallenge: (connectionId: string, answers: string[]) => void;
  cancelConnect: (connectionId: string) => void;

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
  tabData: {},
  connectingAssetIds: new Set(),
  connections: {},

  connect: async (assetId, assetName, assetIcon, password, cols, rows, metadata) => {
    const tabStore = useTabStore.getState();

    // If there's already a connecting/error tab for this asset, switch to it
    const existingTab = tabStore.tabs.find((t) => {
      if (t.type !== "terminal") return false;
      const m = t.meta as TerminalTabMeta;
      if (m.assetId !== assetId) return false;
      const conn = get().connections[t.id];
      return conn && (conn.status === "connecting" || conn.status === "error" || conn.status === "auth_challenge");
    });
    if (existingTab) {
      tabStore.activateTab(existingTab.id);
      return existingTab.id;
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

      const connectionId = await ConnectSSHAsync(req);

      // Create tab in tabStore
      tabStore.openTab({
        id: connectionId,
        type: "terminal",
        label: assetName,
        icon: assetIcon || undefined,
        meta: { type: "terminal", assetId, assetName, assetIcon: assetIcon || "", host: metadata?.host || "", port: metadata?.port || 22, username: metadata?.username || "" },
      });

      // Create business data
      const connState: ConnectionState = {
        connectionId,
        assetId,
        assetName,
        password,
        logs: [],
        status: "connecting",
        currentStep: "resolve",
      };

      set((state) => ({
        tabData: {
          ...state.tabData,
          [connectionId]: {
            splitTree: { type: "connecting", connectionId },
            activePaneId: connectionId,
            panes: {},
          },
        },
        connections: { ...state.connections, [connectionId]: connState },
      }));

      // Listen for connection progress
      const eventName = `ssh:connect:${connectionId}`;
      EventsOn(eventName, (event: {
        type: string;
        step?: string;
        message?: string;
        sessionId?: string;
        error?: string;
        authFailed?: boolean;
        challengeId?: string;
        prompts?: string[];
        echo?: boolean[];
      }) => {
        const state = get();
        const conn = state.connections[connectionId];
        if (!conn) return;

        switch (event.type) {
          case "progress":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  currentStep: (event.step as ConnectionStep) || s.connections[connectionId].currentStep,
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: event.message || "", timestamp: Date.now(), type: "info" as const },
                  ],
                },
              },
            }));
            break;

          case "connected": {
            const sessionId = event.sessionId!;

            // Update business data: replace connecting node with terminal node
            set((s) => {
              const data = s.tabData[connectionId];
              if (!data) return s;

              const newTree = replaceNode(data.splitTree, connectionId, {
                type: "terminal",
                sessionId,
              });

              const newTabData = { ...s.tabData };
              delete newTabData[connectionId];
              newTabData[sessionId] = {
                splitTree: newTree,
                activePaneId: sessionId,
                panes: { [sessionId]: { sessionId, connected: true, connectedAt: Date.now() } },
              };

              const newConnections = { ...s.connections };
              delete newConnections[connectionId];

              return { tabData: newTabData, connections: newConnections };
            });

            // Update tab id in tabStore
            tabStore.replaceTabId(connectionId, sessionId);

            EventsOff(eventName);

            set((s) => {
              const next = new Set(s.connectingAssetIds);
              next.delete(assetId);
              return { connectingAssetIds: next };
            });
            break;
          }

          case "error":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  status: "error",
                  error: event.error,
                  authFailed: event.authFailed,
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: event.error || "连接失败", timestamp: Date.now(), type: "error" as const },
                  ],
                },
              },
            }));

            set((s) => {
              const next = new Set(s.connectingAssetIds);
              next.delete(assetId);
              return { connectingAssetIds: next };
            });
            break;

          case "auth_challenge":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  status: "auth_challenge",
                  challenge: {
                    challengeId: event.challengeId!,
                    prompts: event.prompts || [],
                    echo: event.echo || [],
                  },
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: "等待用户输入认证信息...", timestamp: Date.now(), type: "info" as const },
                  ],
                },
              },
            }));
            break;
        }
      });

      return connectionId;
    } catch (e) {
      set((state) => {
        const next = new Set(state.connectingAssetIds);
        next.delete(assetId);
        return { connectingAssetIds: next };
      });
      throw e;
    }
  },

  reconnect: (tabId) => {
    const tabStore = useTabStore.getState();
    const tab = tabStore.tabs.find((t) => t.id === tabId);
    if (!tab || tab.type !== "terminal") return;

    const data = get().tabData[tabId];
    if (!data) return;

    const sessionId = data.activePaneId;
    const pane = data.panes[sessionId];

    if (pane?.connected) {
      DisconnectSSH(sessionId);
    }

    const meta = tab.meta as TerminalTabMeta;
    const req = new main.SSHConnectRequest({
      assetId: meta.assetId,
      password: "",
      key: "",
      cols: 80,
      rows: 24,
    });

    ConnectSSHAsync(req).then((connectionId) => {
      set((s) => {
        const d = s.tabData[tabId];
        if (!d) return s;

        const newTree = replaceNode(d.splitTree, sessionId, {
          type: "connecting",
          connectionId,
        });

        const newPanes = { ...d.panes };
        delete newPanes[sessionId];

        return {
          tabData: {
            ...s.tabData,
            [tabId]: { ...d, splitTree: newTree, activePaneId: connectionId, panes: newPanes },
          },
          connections: {
            ...s.connections,
            [connectionId]: {
              connectionId,
              assetId: meta.assetId,
              assetName: meta.assetName,
              password: "",
              logs: [],
              status: "connecting" as const,
              currentStep: "resolve" as const,
            },
          },
        };
      });

      const eventName = `ssh:connect:${connectionId}`;
      EventsOn(eventName, (event: {
        type: string;
        step?: string;
        message?: string;
        sessionId?: string;
        error?: string;
        authFailed?: boolean;
        challengeId?: string;
        prompts?: string[];
        echo?: boolean[];
      }) => {
        const state = get();
        const conn = state.connections[connectionId];
        if (!conn) return;

        switch (event.type) {
          case "progress":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  currentStep: (event.step as ConnectionStep) || s.connections[connectionId].currentStep,
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: event.message || "", timestamp: Date.now(), type: "info" as const },
                  ],
                },
              },
            }));
            break;

          case "connected": {
            const newSessionId = event.sessionId!;
            set((s) => {
              const d = s.tabData[tabId];
              if (!d) return s;

              const newTree = replaceNode(d.splitTree, connectionId, {
                type: "terminal",
                sessionId: newSessionId,
              });

              const newPanes = {
                ...d.panes,
                [newSessionId]: { sessionId: newSessionId, connected: true, connectedAt: Date.now() },
              };

              const newConnections = { ...s.connections };
              delete newConnections[connectionId];

              return {
                tabData: {
                  ...s.tabData,
                  [tabId]: { ...d, splitTree: newTree, activePaneId: newSessionId, panes: newPanes },
                },
                connections: newConnections,
              };
            });
            EventsOff(eventName);
            break;
          }

          case "error":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  status: "error",
                  error: event.error,
                  authFailed: event.authFailed,
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: event.error || "连接失败", timestamp: Date.now(), type: "error" as const },
                  ],
                },
              },
            }));
            break;

          case "auth_challenge":
            set((s) => ({
              connections: {
                ...s.connections,
                [connectionId]: {
                  ...s.connections[connectionId],
                  status: "auth_challenge",
                  challenge: {
                    challengeId: event.challengeId!,
                    prompts: event.prompts || [],
                    echo: event.echo || [],
                  },
                  logs: [
                    ...s.connections[connectionId].logs,
                    { message: "等待用户输入认证信息...", timestamp: Date.now(), type: "info" as const },
                  ],
                },
              },
            }));
            break;
        }
      });
    }).catch((err) => {
      console.error("Reconnect failed:", err);
    });
  },

  retryConnect: (connectionId, password) => {
    const conn = get().connections[connectionId];
    if (!conn) return;

    const tabStore = useTabStore.getState();
    const tab = tabStore.tabs.find((t) => t.id === connectionId);
    const meta = tab?.meta as TerminalTabMeta | undefined;
    const metadata: SSHConnectMetadata | undefined = meta ? {
      host: meta.host,
      port: meta.port,
      username: meta.username,
    } : undefined;

    // Clean up old event listeners and connection state
    EventsOff(`ssh:connect:${connectionId}`);

    // Remove old tab and tabData
    set((s) => {
      const newConnections = { ...s.connections };
      delete newConnections[connectionId];
      const newTabData = { ...s.tabData };
      delete newTabData[connectionId];
      return { connections: newConnections, tabData: newTabData };
    });
    tabStore.closeTab(connectionId);

    // Reconnect with new or empty password
    const newPassword = password !== undefined ? password : "";
    get().connect(conn.assetId, conn.assetName, meta?.assetIcon || "", newPassword, 80, 24, metadata);

    if (password) {
      UpdateAssetPassword(conn.assetId, password).catch(() => {});
    }
  },

  respondChallenge: (connectionId, answers) => {
    const conn = get().connections[connectionId];
    if (!conn?.challenge) return;

    RespondAuthChallenge(conn.challenge.challengeId, answers);

    set((s) => ({
      connections: {
        ...s.connections,
        [connectionId]: {
          ...s.connections[connectionId],
          status: "connecting",
          challenge: undefined,
        },
      },
    }));
  },

  cancelConnect: (connectionId) => {
    const conn = get().connections[connectionId];
    if (!conn) return;

    CancelSSHConnect(connectionId);
    EventsOff(`ssh:connect:${connectionId}`);

    set((s) => {
      const next = new Set(s.connectingAssetIds);
      next.delete(conn.assetId);
      return { connectingAssetIds: next };
    });

    // Clean up tabData and connection
    set((s) => {
      const newConnections = { ...s.connections };
      delete newConnections[connectionId];
      const newTabData = { ...s.tabData };
      delete newTabData[connectionId];
      return { connections: newConnections, tabData: newTabData };
    });

    // Close tab via tabStore
    useTabStore.getState().closeTab(connectionId);
  },

  disconnect: (sessionId) => {
    DisconnectSSH(sessionId);
    set((state) => {
      const newTabData = { ...state.tabData };
      for (const [tabId, data] of Object.entries(newTabData)) {
        if (data.panes[sessionId]) {
          newTabData[tabId] = {
            ...data,
            panes: {
              ...data.panes,
              [sessionId]: { ...data.panes[sessionId], connected: false },
            },
          };
        }
      }
      return { tabData: newTabData };
    });
  },

  markClosed: (sessionId) => {
    set((state) => {
      const newTabData = { ...state.tabData };
      for (const [tabId, data] of Object.entries(newTabData)) {
        if (data.panes[sessionId]) {
          newTabData[tabId] = {
            ...data,
            panes: {
              ...data.panes,
              [sessionId]: { ...data.panes[sessionId], connected: false },
            },
          };
        }
      }
      return { tabData: newTabData };
    });
  },

  setActivePaneId: (tabId, paneId) => {
    set((state) => {
      const data = state.tabData[tabId];
      if (!data) return state;
      return {
        tabData: { ...state.tabData, [tabId]: { ...data, activePaneId: paneId } },
      };
    });
  },

  splitPane: (tabId, direction) => {
    const data = get().tabData[tabId];
    if (!data) return;

    const pendingId = `pending-${Date.now()}`;

    // Step 1: Split UI with pending placeholder
    set((state) => {
      const d = state.tabData[tabId];
      if (!d) return state;

      const newTree = replaceNode(d.splitTree, d.activePaneId, {
        type: "split",
        direction,
        ratio: 0.5,
        first: { type: "terminal", sessionId: d.activePaneId },
        second: { type: "pending", pendingId },
      });

      return {
        tabData: { ...state.tabData, [tabId]: { ...d, splitTree: newTree } },
      };
    });

    // Step 2: Create new session on existing connection
    SplitSSH(data.activePaneId, 80, 24)
      .then((sessionId) => {
        set((state) => {
          const d = state.tabData[tabId];
          if (!d) return state;

          const newTree = replaceNode(d.splitTree, pendingId, {
            type: "terminal",
            sessionId,
          });

          return {
            tabData: {
              ...state.tabData,
              [tabId]: {
                ...d,
                splitTree: newTree,
                activePaneId: sessionId,
                panes: {
                  ...d.panes,
                  [sessionId]: { sessionId, connected: true, connectedAt: Date.now() },
                },
              },
            },
          };
        });
      })
      .catch((err) => {
        console.error("Split connection failed:", err);
        set((state) => {
          const d = state.tabData[tabId];
          if (!d) return state;

          const newTree = removeNode(d.splitTree, pendingId);
          if (!newTree) return state;

          return {
            tabData: { ...state.tabData, [tabId]: { ...d, splitTree: newTree } },
          };
        });
      });
  },

  closePane: (tabId, sessionId) => {
    const data = get().tabData[tabId];
    if (!data) return;

    const pane = data.panes[sessionId];
    if (pane?.connected) {
      DisconnectSSH(sessionId);
    }

    // If only one pane, close entire tab
    const allSessions = getSessionIds(data.splitTree);
    if (allSessions.length <= 1) {
      useTabStore.getState().closeTab(tabId);
      return;
    }

    const newTree = removeNode(data.splitTree, sessionId);
    if (!newTree) {
      useTabStore.getState().closeTab(tabId);
      return;
    }

    const remaining = getSessionIds(newTree);
    const newActivePaneId =
      data.activePaneId === sessionId ? remaining[0] : data.activePaneId;

    const newPanes = { ...data.panes };
    delete newPanes[sessionId];

    set((state) => ({
      tabData: {
        ...state.tabData,
        [tabId]: { splitTree: newTree, activePaneId: newActivePaneId, panes: newPanes },
      },
    }));
  },

  setSplitRatio: (tabId, path, ratio) => {
    set((state) => {
      const data = state.tabData[tabId];
      if (!data) return state;
      return {
        tabData: {
          ...state.tabData,
          [tabId]: { ...data, splitTree: setRatioAtPath(data.splitTree, path, ratio) },
        },
      };
    });
  },
}));

// === Close Hook: clean up when tabStore closes a terminal tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "terminal") return;

  const state = useTerminalStore.getState();
  const data = state.tabData[tab.id];

  // Cancel if still connecting
  const conn = state.connections[tab.id];
  if (conn) {
    CancelSSHConnect(tab.id);
    EventsOff(`ssh:connect:${tab.id}`);
  }

  // Disconnect all panes
  if (data) {
    for (const pane of Object.values(data.panes)) {
      if (pane.connected) {
        DisconnectSSH(pane.sessionId);
      }
    }
  }

  // Clean up state
  useTerminalStore.setState((s) => {
    const newTabData = { ...s.tabData };
    delete newTabData[tab.id];
    const newConnections = { ...s.connections };
    delete newConnections[tab.id];
    const next = new Set(s.connectingAssetIds);
    if (conn) next.delete(conn.assetId);
    return { tabData: newTabData, connections: newConnections, connectingAssetIds: next };
  });
});

// === Restore terminal tabData for tabs already in tabStore ===

(function _restoreTerminalTabData() {
  const tabs = useTabStore.getState().tabs.filter((t) => t.type === "terminal");
  if (tabs.length === 0) return;

  const tabData: Record<string, TerminalTabData> = {};
  for (const tab of tabs) {
    // Restored terminal tabs start as disconnected
    tabData[tab.id] = {
      splitTree: { type: "terminal", sessionId: tab.id },
      activePaneId: tab.id,
      panes: { [tab.id]: { sessionId: tab.id, connected: false, connectedAt: 0 } },
    };
  }
  useTerminalStore.setState({ tabData });
})();
