import { create } from "zustand";
import {
  ConnectSSHAsync,
  CancelSSHConnect,
  RespondAuthChallenge,
  RespondHostKeyVerify,
  DisconnectSSH,
  GetSSHSyncState,
  SplitSSH,
  UpdateAssetPassword,
  WriteSSH,
  WriteSerial,
  ConnectSerialAsync,
  DisconnectSerial,
} from "../../wailsjs/go/app/App";
import { app, asset_entity } from "../../wailsjs/go/models";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import { bytesToBase64 } from "../lib/terminalEncode";
import { useTabStore, registerTabCloseHook, registerTabRestoreHook, type TerminalTabMeta } from "./tabStore";
import { useAssetStore } from "./assetStore";

function disposeTerminalInstance(sessionId: string): void {
  import("@/components/terminal/terminalRegistry")
    .then(({ disposeTerminal }) => disposeTerminal(sessionId))
    .catch((error) => console.error(`Failed to dispose terminal instance ${sessionId}:`, error));
}

export type TerminalTransport = "ssh" | "serial";

// 仅在恢复（restore）时用到——那时 pane 状态还没有 transport，只能从 session id 前缀
// 反推；连接/断开/分屏等运行时路径都直接读 pane.transport，不再依赖前缀。
function inferTransportFromSessionId(sessionId: string): TerminalTransport {
  return sessionId.startsWith("serial-") ? "serial" : "ssh";
}

function disconnectSession(sessionId: string, transport: TerminalTransport): void {
  if (transport === "serial") {
    DisconnectSerial(sessionId);
  } else {
    DisconnectSSH(sessionId);
  }
}

function writeSessionInput(sessionId: string, transport: TerminalTransport, dataB64: string): Promise<void> {
  return transport === "serial" ? WriteSerial(sessionId, dataB64) : WriteSSH(sessionId, dataB64);
}

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
  transport: TerminalTransport;
  connected: boolean;
  connectedAt: number;
}

export interface TerminalDirectorySyncState {
  sessionId: string;
  cwd?: string;
  cwdKnown: boolean;
  shell?: string;
  shellType?: string;
  supported: boolean;
  promptReady: boolean;
  promptClean: boolean;
  busy: boolean;
  status: "initializing" | "ready" | "unsupported";
  lastError?: string;
}

export type TerminalDirectoryFollowMode = "off" | "always";

// Business data per terminal tab (split tree, panes, connection state)
export interface TerminalTabData {
  splitTree: SplitNode;
  activePaneId: string;
  panes: Record<string, TerminalPane>;
  directoryFollowMode: TerminalDirectoryFollowMode;
  /** Snippet content to send once the first pane becomes connected. Cleared after write. */
  pendingInput?: string;
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

export type ConnectionStep = "resolve" | "open" | "connect" | "auth" | "shell";

export interface ConnectionState {
  connectionId: string;
  assetId: number;
  assetName: string;
  transport: "ssh" | "serial";
  password: string;
  logs: ConnectionLogEntry[];
  status: "connecting" | "auth_challenge" | "host_key_verify" | "connected" | "error";
  currentStep: ConnectionStep;
  error?: string;
  authFailed?: boolean;
  challenge?: {
    challengeId: string;
    prompts: string[];
    echo: boolean[];
  };
  hostKeyVerify?: {
    verifyId: string;
    host: string;
    port: number;
    keyType: string;
    fingerprint: string;
    isChanged: boolean;
    oldFingerprint?: string;
  };
}

// Helper: get all session IDs from a split tree (skips pending/connecting)
export function getSessionIds(node: SplitNode): string[] {
  if (node.type === "terminal") return [node.sessionId];
  if (node.type === "pending" || node.type === "connecting") return [];
  return [...getSessionIds(node.first), ...getSessionIds(node.second)];
}

// Helper: replace a leaf node (terminal, pending, or connecting) by ID
function replaceNode(tree: SplitNode, id: string, replacement: SplitNode): SplitNode {
  if (tree.type === "terminal" && tree.sessionId === id) return replacement;
  if (tree.type === "pending" && tree.pendingId === id) return replacement;
  if (tree.type === "connecting" && tree.connectionId === id) return replacement;
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
function setRatioAtPath(tree: SplitNode, path: number[], ratio: number): SplitNode {
  if (path.length === 0 && tree.type === "split") {
    return { ...tree, ratio };
  }
  if (tree.type === "split" && path.length > 0) {
    const [head, ...rest] = path;
    if (head === 0) return { ...tree, first: setRatioAtPath(tree.first, rest, ratio) };
    return { ...tree, second: setRatioAtPath(tree.second, rest, ratio) };
  }
  return tree;
}

/** Returns the set of asset IDs that have at least one connected terminal pane. */
export function getTerminalActiveAssetIds(): Set<number> {
  const { tabData } = useTerminalStore.getState();
  const tabs = useTabStore.getState().tabs;
  const ids = new Set<number>();
  for (const tab of tabs) {
    if (tab.type !== "terminal") continue;
    const d = tabData[tab.id];
    if (d && Object.values(d.panes).some((p) => p.connected)) {
      ids.add((tab.meta as TerminalTabMeta).assetId);
    }
  }
  return ids;
}

const syncListeners = new Set<string>();

export function __resetTerminalSyncListenersForTest() {
  syncListeners.clear();
}

function registerSessionSyncListener(sessionId: string) {
  if (!sessionId || syncListeners.has(sessionId)) return;
  syncListeners.add(sessionId);

  const eventName = `ssh:sync:${sessionId}`;
  EventsOn(eventName, (state: TerminalDirectorySyncState) => {
    useTerminalStore.getState().setSessionSyncState(sessionId, state);
  });

  GetSSHSyncState(sessionId)
    .then((state) => {
      if (!syncListeners.has(sessionId)) return;
      useTerminalStore.getState().setSessionSyncState(sessionId, state as TerminalDirectorySyncState);
    })
    .catch(() => {
      /* ignore initial sync fetch errors; live event will reconcile */
    });
}

function unregisterSessionSyncListener(sessionId: string) {
  if (!syncListeners.delete(sessionId)) return;
  EventsOff(`ssh:sync:${sessionId}`);
  useTerminalStore.setState((state) => {
    const next = { ...state.sessionSync };
    delete next[sessionId];
    return { sessionSync: next };
  });
}

// === Connection event listener (shared by connect/reconnect/restore) ===

/**
 * Sets up event listeners for a connection's progress events.
 * Handles progress/error/auth_challenge uniformly; delegates "connected" to callback.
 *
 * @param connectionId - The connection ID from ConnectSSHAsync / ConnectSerialAsync
 * @param onConnected - Called when session is established (receives sessionId)
 * @param onFinished - Optional cleanup called on both "connected" and "error"
 * @param transport - "ssh" or "serial" (determines event name prefix)
 */
function setupConnectionListener(
  connectionId: string,
  onConnected: (sessionId: string) => void,
  onFinished?: () => void,
  transport: "ssh" | "serial" = "ssh"
) {
  const eventName = `${transport}:connect:${connectionId}`;
  EventsOn(
    eventName,
    (event: {
      type: string;
      step?: string;
      message?: string;
      sessionId?: string;
      error?: string;
      authFailed?: boolean;
      challengeId?: string;
      prompts?: string[];
      echo?: boolean[];
      hostKeyVerifyId?: string;
      hostKeyEvent?: {
        host: string;
        port: number;
        keyType: string;
        fingerprint: string;
        isChanged: boolean;
        oldFingerprint?: string;
      };
    }) => {
      const state = useTerminalStore.getState();
      const conn = state.connections[connectionId];
      if (!conn) return;

      switch (event.type) {
        case "progress":
          useTerminalStore.setState((s) => ({
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

        case "connected":
          onConnected(event.sessionId!);
          EventsOff(eventName);
          onFinished?.();
          break;

        case "error":
          useTerminalStore.setState((s) => ({
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
          onFinished?.();
          break;

        case "auth_challenge":
          useTerminalStore.setState((s) => ({
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

        case "host_key_verify":
          useTerminalStore.setState((s) => ({
            connections: {
              ...s.connections,
              [connectionId]: {
                ...s.connections[connectionId],
                status: "host_key_verify",
                hostKeyVerify: {
                  verifyId: event.hostKeyVerifyId!,
                  host: event.hostKeyEvent!.host,
                  port: event.hostKeyEvent!.port,
                  keyType: event.hostKeyEvent!.keyType,
                  fingerprint: event.hostKeyEvent!.fingerprint,
                  isChanged: event.hostKeyEvent!.isChanged,
                  oldFingerprint: event.hostKeyEvent!.oldFingerprint,
                },
                logs: [
                  ...s.connections[connectionId].logs,
                  {
                    message: event.hostKeyEvent!.isChanged ? "警告：主机密钥已变更！" : "等待确认主机密钥...",
                    timestamp: Date.now(),
                    type: event.hostKeyEvent!.isChanged ? ("error" as const) : ("info" as const),
                  },
                ],
              },
            },
          }));
          break;
      }
    }
  );
}

interface TerminalState {
  // Business data keyed by tab id
  tabData: Record<string, TerminalTabData>;
  sessionSync: Record<string, TerminalDirectorySyncState>;
  connectingAssetIds: Set<number>;
  connections: Record<string, ConnectionState>;

  connect: (
    asset: asset_entity.Asset,
    password?: string,
    forceNew?: boolean,
    opts?: { initialInput?: string }
  ) => Promise<string>;
  reconnect: (tabId: string) => void;
  reconnectBySession: (sessionId: string) => void;
  disconnect: (sessionId: string) => void;
  markClosed: (sessionId: string) => void;

  // Connection progress actions
  retryConnect: (connectionId: string, password?: string) => void;
  respondChallenge: (connectionId: string, answers: string[]) => void;
  respondHostKeyVerify: (connectionId: string, action: number) => void;
  cancelConnect: (connectionId: string) => void;

  // Split pane actions
  setActivePaneId: (tabId: string, paneId: string) => void;
  setSessionSyncState: (sessionId: string, state: TerminalDirectorySyncState) => void;
  setDirectoryFollowMode: (tabId: string, mode: TerminalDirectoryFollowMode) => void;
  splitPane: (tabId: string, direction: "horizontal" | "vertical") => void;
  closePane: (tabId: string, sessionId: string) => void;
  setSplitRatio: (tabId: string, path: number[], ratio: number) => void;
}

export const useTerminalStore = create<TerminalState>((set, get) => ({
  tabData: {},
  sessionSync: {},
  connectingAssetIds: new Set(),
  connections: {},

  connect: async (asset, password = "", forceNew = false, opts) => {
    const assetId = asset.ID;
    const assetPath = useAssetStore.getState().getAssetPath(asset);
    const assetIcon = asset.Icon || "";
    let metadata: SSHConnectMetadata | undefined;
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      metadata = { host: cfg.host || "", port: cfg.port || 22, username: cfg.username || "" };
    } catch {
      /* ignore */
    }

    const tabStore = useTabStore.getState();

    // If there's already a tab for this asset (connected or connecting), switch to it
    if (!forceNew) {
      const existingTab = tabStore.tabs.find((t) => {
        if (t.type !== "terminal") return false;
        const m = t.meta as TerminalTabMeta;
        return m.assetId === assetId;
      });
      if (existingTab) {
        tabStore.activateTab(existingTab.id);
        return existingTab.id;
      }
    }

    set((state) => ({
      connectingAssetIds: new Set(state.connectingAssetIds).add(assetId),
    }));

    try {
      let connectionId: string;
      const isSerial = asset.Type === "serial";

      if (isSerial) {
        connectionId = await ConnectSerialAsync({ assetId });
      } else {
        const req = new app.SSHConnectRequest({
          assetId,
          password,
          key: "",
          cols: 80,
          rows: 24,
        });
        connectionId = await ConnectSSHAsync(req);
      }

      // Create tab in tabStore
      tabStore.openTab({
        id: connectionId,
        type: "terminal",
        label: assetPath,
        icon: assetIcon || undefined,
        meta: {
          type: "terminal",
          assetId,
          assetName: assetPath,
          assetIcon: assetIcon || "",
          host: metadata?.host || "",
          port: metadata?.port || 22,
          username: metadata?.username || "",
        },
      });

      // Create business data
      set((state) => ({
        tabData: {
          ...state.tabData,
          [connectionId]: {
            splitTree: { type: "connecting", connectionId },
            activePaneId: connectionId,
            panes: {},
            directoryFollowMode: "off",
            pendingInput: opts?.initialInput,
          },
        },
        connections: {
          ...state.connections,
          [connectionId]: {
            connectionId,
            assetId,
            assetName: assetPath,
            transport: isSerial ? "serial" : "ssh",
            password,
            logs: [],
            status: "connecting",
            currentStep: "resolve",
          },
        },
      }));

      setupConnectionListener(
        connectionId,
        (sessionId) => {
          // Migrate tabData from connectionId key to sessionId key
          let pendingInput: string | undefined;
          set((s) => {
            const data = s.tabData[connectionId];
            if (!data) return s;

            pendingInput = data.pendingInput;

            const newTree = replaceNode(data.splitTree, connectionId, {
              type: "terminal",
              sessionId,
            });

            const newTabData = { ...s.tabData };
            delete newTabData[connectionId];
            newTabData[sessionId] = {
              splitTree: newTree,
              activePaneId: sessionId,
              panes: {
                [sessionId]: {
                  sessionId,
                  transport: isSerial ? "serial" : "ssh",
                  connected: true,
                  connectedAt: Date.now(),
                },
              },
              directoryFollowMode: data.directoryFollowMode,
              // pendingInput intentionally not forwarded — write happens below
            };

            const newConnections = { ...s.connections };
            delete newConnections[connectionId];

            return { tabData: newTabData, connections: newConnections };
          });

          // Update tab id in tabStore
          tabStore.replaceTabId(connectionId, sessionId);
          if (!isSerial) {
            registerSessionSyncListener(sessionId);
          }

          // Write pending snippet input (no trailing \r — user sees content and decides to execute)
          if (pendingInput) {
            writeSessionInput(
              sessionId,
              isSerial ? "serial" : "ssh",
              bytesToBase64(new TextEncoder().encode(pendingInput))
            ).catch(console.error);
          }
        },
        () => {
          // Clear connectingAssetIds on connected or error
          set((s) => {
            const next = new Set(s.connectingAssetIds);
            next.delete(assetId);
            return { connectingAssetIds: next };
          });
        },
        isSerial ? "serial" : "ssh"
      );

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

    const meta = tab.meta as TerminalTabMeta;

    const sessionId = data.activePaneId;
    const pane = data.panes[sessionId];
    const asset = useAssetStore.getState().assets.find((a) => a.ID === meta.assetId);
    // 优先用 pane.transport（运行时权威），否则按资产类型，再退回 session id 前缀（restore 后的兜底）。
    const transport: TerminalTransport =
      pane?.transport ?? (asset?.Type === "serial" ? "serial" : inferTransportFromSessionId(sessionId));
    const isSerial = transport === "serial";

    unregisterSessionSyncListener(sessionId);
    if (pane?.connected) {
      disconnectSession(sessionId, transport);
    }

    const connectPromise = isSerial
      ? ConnectSerialAsync({ assetId: meta.assetId })
      : ConnectSSHAsync(
          new app.SSHConnectRequest({
            assetId: meta.assetId,
            password: "",
            key: "",
            cols: 80,
            rows: 24,
          })
        );

    connectPromise
      .then((connectionId: string) => {
        // Dispose the old persistent xterm; the slot is replaced by a "connecting" node.
        disposeTerminalInstance(sessionId);
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
                transport: isSerial ? "serial" : "ssh",
                password: "",
                logs: [],
                status: "connecting" as const,
                currentStep: "resolve" as const,
              },
            },
          };
        });

        setupConnectionListener(
          connectionId,
          (newSessionId) => {
            set((s) => {
              const d = s.tabData[tabId];
              if (!d) return s;

              const newTree = replaceNode(d.splitTree, connectionId, {
                type: "terminal",
                sessionId: newSessionId,
              });

              const newConnections = { ...s.connections };
              delete newConnections[connectionId];

              return {
                tabData: {
                  ...s.tabData,
                  [tabId]: {
                    ...d,
                    splitTree: newTree,
                    activePaneId: newSessionId,
                    panes: {
                      ...d.panes,
                      [newSessionId]: {
                        sessionId: newSessionId,
                        transport,
                        connected: true,
                        connectedAt: Date.now(),
                      },
                    },
                  },
                },
                connections: newConnections,
              };
            });
            if (!isSerial) {
              registerSessionSyncListener(newSessionId);
            }
          },
          undefined,
          isSerial ? "serial" : "ssh"
        );
      })
      .catch((err: unknown) => {
        console.error("Reconnect failed:", err);
      });
  },

  reconnectBySession: (sessionId) => {
    const { tabData } = get();
    const tabId = Object.keys(tabData).find((id) => Boolean(tabData[id]?.panes[sessionId]));
    if (tabId) get().reconnect(tabId);
  },

  retryConnect: (connectionId, password) => {
    const conn = get().connections[connectionId];
    if (!conn) return;

    // Find the asset from assetStore
    const assetStore = useAssetStore.getState();
    const asset = assetStore.assets.find((a) => a.ID === conn.assetId);
    if (!asset) return;

    // Clean up old event listeners and connection state
    EventsOff(`${conn.transport}:connect:${connectionId}`);

    // Remove old tab and tabData
    const tabStore = useTabStore.getState();
    set((s) => {
      const newConnections = { ...s.connections };
      delete newConnections[connectionId];
      const newTabData = { ...s.tabData };
      delete newTabData[connectionId];
      return { connections: newConnections, tabData: newTabData };
    });
    tabStore.closeTab(connectionId);

    // Reconnect with new or empty password
    get().connect(asset, password !== undefined ? password : "");

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

  respondHostKeyVerify: (connectionId, action) => {
    const conn = get().connections[connectionId];
    if (!conn?.hostKeyVerify) return;

    RespondHostKeyVerify(conn.hostKeyVerify.verifyId, action);

    set((s) => ({
      connections: {
        ...s.connections,
        [connectionId]: {
          ...s.connections[connectionId],
          status: "connecting",
          hostKeyVerify: undefined,
          logs: [
            ...s.connections[connectionId].logs,
            {
              message: action === 2 ? "用户拒绝连接" : "主机密钥已确认",
              timestamp: Date.now(),
              type: "info" as const,
            },
          ],
        },
      },
    }));
  },

  cancelConnect: (connectionId) => {
    const conn = get().connections[connectionId];
    if (!conn) return;

    CancelSSHConnect(connectionId);
    EventsOff(`${conn.transport}:connect:${connectionId}`);

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
    unregisterSessionSyncListener(sessionId);
    const transport =
      Object.values(get().tabData)
        .map((d) => d.panes[sessionId]?.transport)
        .find((t): t is TerminalTransport => !!t) ?? inferTransportFromSessionId(sessionId);
    disconnectSession(sessionId, transport);
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
    unregisterSessionSyncListener(sessionId);
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

  setSessionSyncState: (sessionId, syncState) => {
    set((state) => ({
      sessionSync: {
        ...state.sessionSync,
        [sessionId]: syncState,
      },
    }));
  },

  setDirectoryFollowMode: (tabId, mode) => {
    set((state) => {
      const data = state.tabData[tabId];
      if (!data) return state;
      return {
        tabData: {
          ...state.tabData,
          [tabId]: {
            ...data,
            directoryFollowMode: mode,
          },
        },
      };
    });
  },

  splitPane: (tabId, direction) => {
    const data = get().tabData[tabId];
    if (!data) return;
    // 串口 session 不支持 split（同一物理端口不能被多 pane 复用），上游菜单也已 disable，
    // 这里再防一道，避免外部直接调用绕过。
    if (data.panes[data.activePaneId]?.transport === "serial") return;

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
      .then((sessionId: string) => {
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
                  // split 仅适用于 SSH（serial 已在入口阻断），新 pane 始终是 ssh。
                  [sessionId]: { sessionId, transport: "ssh", connected: true, connectedAt: Date.now() },
                },
              },
            },
          };
        });
        registerSessionSyncListener(sessionId);
      })
      .catch((err: unknown) => {
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
    unregisterSessionSyncListener(sessionId);
    if (pane?.connected) {
      disconnectSession(sessionId, pane.transport);
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
    const newActivePaneId = data.activePaneId === sessionId ? remaining[0] : data.activePaneId;

    const newPanes = { ...data.panes };
    delete newPanes[sessionId];

    set((state) => ({
      tabData: {
        ...state.tabData,
        [tabId]: {
          ...data,
          splitTree: newTree,
          activePaneId: newActivePaneId,
          panes: newPanes,
        },
      },
    }));

    // Drop the persistent xterm now that it's no longer in the tree.
    disposeTerminalInstance(sessionId);
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
    EventsOff(`${conn.transport}:connect:${tab.id}`);
  }

  // Disconnect all panes and drop their persistent xterm instances.
  if (data) {
    for (const pane of Object.values(data.panes)) {
      unregisterSessionSyncListener(pane.sessionId);
      if (pane.connected) {
        disconnectSession(pane.sessionId, pane.transport);
      }
      disposeTerminalInstance(pane.sessionId);
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

// === Restore Hook: initialize tabData + auto-reconnect ===

registerTabRestoreHook("terminal", (tabs) => {
  if (tabs.length === 0) return;

  // Initialize tabData as disconnected (reconnect will transition to connecting)
  const tabData: Record<string, TerminalTabData> = {};
  for (const tab of tabs) {
    // 恢复阶段还没有 pane.transport，只能从 session id 前缀反推；reconnect 跑完后
    // 新 pane 会带上权威 transport，这个值随之被覆盖。
    const transport = inferTransportFromSessionId(tab.id);
    tabData[tab.id] = {
      splitTree: { type: "terminal", sessionId: tab.id },
      activePaneId: tab.id,
      panes: { [tab.id]: { sessionId: tab.id, transport, connected: false, connectedAt: 0 } },
      directoryFollowMode: "off",
    };
  }
  useTerminalStore.setState({ tabData });

  // Auto-reconnect each terminal tab
  for (const tab of tabs) {
    useTerminalStore.getState().reconnect(tab.id);
  }
});
