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

export interface TerminalTab {
  id: string; // tab id (connectionId initially, then sessionId)
  assetId: number;
  assetName: string;
  host: string;
  port: number;
  username: string;
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
  tabs: TerminalTab[];
  activeTabId: string | null;
  assetInfoOpen: boolean;
  connectingAssetIds: Set<number>;
  connections: Record<string, ConnectionState>;

  connect: (
    assetId: number,
    assetName: string,
    password: string,
    cols: number,
    rows: number,
    metadata?: SSHConnectMetadata
  ) => Promise<string>;
  reconnect: (tabId: string) => void;
  disconnect: (sessionId: string) => void;
  setActiveTab: (id: string | null) => void;
  removeTab: (id: string) => void;
  markClosed: (id: string) => void;
  openAssetInfo: () => void;
  closeAssetInfo: () => void;

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
  tabs: [],
  activeTabId: null,
  assetInfoOpen: false,
  connectingAssetIds: new Set(),
  connections: {},

  connect: async (assetId, assetName, password, cols, rows, metadata) => {
    // 如果已有正在连接或连接失败的 tab，直接切换过去
    const existingTab = get().tabs.find((t) => {
      if (t.assetId !== assetId) return false;
      const conn = get().connections[t.id];
      return conn && (conn.status === "connecting" || conn.status === "error" || conn.status === "auth_challenge");
    });
    if (existingTab) {
      set({ activeTabId: existingTab.id });
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

      // 异步连接，立即返回 connectionId
      const connectionId = await ConnectSSHAsync(req);

      // 立即创建 Tab（"connecting" 节点）
      const tab: TerminalTab = {
        id: connectionId,
        assetId,
        assetName,
        host: metadata?.host || "",
        port: metadata?.port || 22,
        username: metadata?.username || "",
        splitTree: { type: "connecting", connectionId },
        activePaneId: connectionId,
        panes: {},
      };

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
        tabs: [...state.tabs, tab],
        activeTabId: connectionId,
        connections: { ...state.connections, [connectionId]: connState },
      }));

      // 监听连接进度事件
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
                    {
                      message: event.message || "",
                      timestamp: Date.now(),
                      type: "info" as const,
                    },
                  ],
                },
              },
            }));
            break;

          case "connected": {
            const sessionId = event.sessionId!;
            // 替换 connecting 节点为 terminal 节点
            set((s) => {
              const tab = s.tabs.find((t) => t.id === connectionId);
              if (!tab) return s;

              const newTree = replaceNode(tab.splitTree, connectionId, {
                type: "terminal",
                sessionId,
              });

              const newTabs = s.tabs.map((t) =>
                t.id === connectionId
                  ? {
                      ...t,
                      id: sessionId,
                      splitTree: newTree,
                      activePaneId: sessionId,
                      panes: { [sessionId]: { sessionId, connected: true, connectedAt: Date.now() } },
                    }
                  : t
              );

              const newConnections = { ...s.connections };
              delete newConnections[connectionId];

              return {
                tabs: newTabs,
                activeTabId:
                  s.activeTabId === connectionId ? sessionId : s.activeTabId,
                connections: newConnections,
              };
            });

            // 清理事件监听
            EventsOff(eventName);

            // 清理 connectingAssetIds
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
                    {
                      message: event.error || "连接失败",
                      timestamp: Date.now(),
                      type: "error" as const,
                    },
                  ],
                },
              },
            }));

            // 清理 connectingAssetIds
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
                    {
                      message: "等待用户输入认证信息...",
                      timestamp: Date.now(),
                      type: "info" as const,
                    },
                  ],
                },
              },
            }));
            break;
        }
      });

      return connectionId;
    } catch (e) {
      // ConnectSSHAsync 本身的校验错误（资产不存在等）
      set((state) => {
        const next = new Set(state.connectingAssetIds);
        next.delete(assetId);
        return { connectingAssetIds: next };
      });
      throw e;
    }
  },

  reconnect: (tabId) => {
    const tab = get().tabs.find((t) => t.id === tabId);
    if (!tab) return;

    const sessionId = tab.activePaneId;
    const pane = tab.panes[sessionId];

    // 断开旧会话（如果还连着的话）
    if (pane?.connected) {
      DisconnectSSH(sessionId);
    }

    // 发起新的异步连接
    const req = new main.SSHConnectRequest({
      assetId: tab.assetId,
      password: "",
      key: "",
      cols: 80,
      rows: 24,
    });

    ConnectSSHAsync(req).then((connectionId) => {
      // 替换活跃 pane 为 connecting 节点
      set((s) => {
        const t = s.tabs.find((t) => t.id === tabId);
        if (!t) return s;

        const newTree = replaceNode(t.splitTree, sessionId, {
          type: "connecting",
          connectionId,
        });

        const newPanes = { ...t.panes };
        delete newPanes[sessionId];

        return {
          tabs: s.tabs.map((tab) =>
            tab.id === tabId
              ? { ...tab, splitTree: newTree, activePaneId: connectionId, panes: newPanes }
              : tab
          ),
          connections: {
            ...s.connections,
            [connectionId]: {
              connectionId,
              assetId: t.assetId,
              assetName: t.assetName,
              password: "",
              logs: [],
              status: "connecting" as const,
              currentStep: "resolve" as const,
            },
          },
        };
      });

      // 监听连接进度事件
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
              const t = s.tabs.find((t) => t.id === tabId);
              if (!t) return s;

              const newTree = replaceNode(t.splitTree, connectionId, {
                type: "terminal",
                sessionId: newSessionId,
              });

              const newPanes = {
                ...t.panes,
                [newSessionId]: { sessionId: newSessionId, connected: true, connectedAt: Date.now() },
              };

              const newConnections = { ...s.connections };
              delete newConnections[connectionId];

              return {
                tabs: s.tabs.map((tab) =>
                  tab.id === tabId
                    ? { ...tab, splitTree: newTree, activePaneId: newSessionId, panes: newPanes }
                    : tab
                ),
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

    // 获取旧 tab 的元数据
    const oldTab = get().tabs.find((t) => t.id === connectionId);
    const metadata: SSHConnectMetadata | undefined = oldTab ? {
      host: oldTab.host,
      port: oldTab.port,
      username: oldTab.username,
    } : undefined;

    // 清理旧的事件监听和连接状态
    EventsOff(`ssh:connect:${connectionId}`);

    // 移除旧 tab
    set((s) => {
      const newConnections = { ...s.connections };
      delete newConnections[connectionId];
      return {
        tabs: s.tabs.filter((t) => t.id !== connectionId),
        connections: newConnections,
        activeTabId:
          s.activeTabId === connectionId ? null : s.activeTabId,
      };
    });

    // 重新连接
    const newPassword = password !== undefined ? password : conn.password;
    get().connect(conn.assetId, conn.assetName, newPassword, 80, 24, metadata);

    // 如果提供了新密码，保存到资产
    if (password && password !== conn.password) {
      UpdateAssetPassword(conn.assetId, password).catch(() => {});
    }
  },

  respondChallenge: (connectionId, answers) => {
    const conn = get().connections[connectionId];
    if (!conn?.challenge) return;

    RespondAuthChallenge(conn.challenge.challengeId, answers);

    // 恢复连接中状态
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

    // 清理 connectingAssetIds
    set((s) => {
      const next = new Set(s.connectingAssetIds);
      next.delete(conn.assetId);
      return { connectingAssetIds: next };
    });

    // 移除 tab 和连接状态
    get().removeTab(connectionId);

    set((s) => {
      const newConnections = { ...s.connections };
      delete newConnections[connectionId];
      return { connections: newConnections };
    });
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
      // 如果是 connecting 状态，取消连接
      const conn = get().connections[id];
      if (conn) {
        CancelSSHConnect(id);
        EventsOff(`ssh:connect:${id}`);
        set((s) => {
          const next = new Set(s.connectingAssetIds);
          next.delete(conn.assetId);
          const newConnections = { ...s.connections };
          delete newConnections[id];
          return { connectingAssetIds: next, connections: newConnections };
        });
      }

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
                      [sessionId]: { sessionId, connected: true, connectedAt: Date.now() },
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
