import { create } from "zustand";
import {
  SFTPUpload,
  SFTPUploadDir,
  SFTPUploadFile,
  SFTPDownload,
  SFTPDownloadDir,
  SFTPCancelTransfer,
} from "../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import { registerTabCloseHook, registerTabReplaceHook } from "./tabStore";

export interface SFTPTransfer {
  transferId: string;
  tabId: string;
  sessionId: string;
  direction: "upload" | "download";
  currentFile: string;
  filesCompleted: number;
  filesTotal: number;
  bytesDone: number;
  bytesTotal: number;
  speed: number;
  status: "active" | "done" | "error" | "cancelled";
  error?: string;
}

export interface SFTPTransferTarget {
  tabId: string;
  sessionId: string;
}

const DEFAULT_FILE_MANAGER_WIDTH = 280;
const MIN_FILE_MANAGER_WIDTH = 200;
const MAX_FILE_MANAGER_WIDTH = 600;

interface SFTPState {
  transfers: Record<string, SFTPTransfer>;

  // File manager panel state
  fileManagerOpenTabs: Record<string, boolean>;
  fileManagerPaths: Record<string, string>;
  fileManagerWidth: number;

  startUpload: (target: SFTPTransferTarget, remotePath: string) => Promise<string | null>;
  startUploadDir: (target: SFTPTransferTarget, remotePath: string) => Promise<string | null>;
  startUploadFile: (target: SFTPTransferTarget, localPath: string, remotePath: string) => Promise<string | null>;
  startDownload: (target: SFTPTransferTarget, remotePath: string) => Promise<string | null>;
  startDownloadDir: (target: SFTPTransferTarget, remotePath: string) => Promise<string | null>;
  cancelTransfer: (transferId: string) => void;
  clearTransfer: (transferId: string) => void;
  clearCompleted: () => void;
  clearCompletedForSession: (sessionId: string) => void;
  clearCompletedForTab: (tabId: string) => void;
  getSessionTransfers: (sessionId: string) => SFTPTransfer[];
  getTabTransfers: (tabId: string) => SFTPTransfer[];

  toggleFileManager: (tabId: string) => void;
  setFileManagerPath: (tabId: string, path: string) => void;
  setFileManagerWidth: (width: number) => void;
}

function subscribeProgress(
  transferId: string,
  target: SFTPTransferTarget,
  direction: "upload" | "download",
  set: (fn: (state: SFTPState) => Partial<SFTPState>) => void,
  get: () => SFTPState
) {
  // Initialize transfer in store
  set((state) => ({
    transfers: {
      ...state.transfers,
      [transferId]: {
        transferId,
        tabId: target.tabId,
        sessionId: target.sessionId,
        direction,
        currentFile: "",
        filesCompleted: 0,
        filesTotal: 0,
        bytesDone: 0,
        bytesTotal: 0,
        speed: 0,
        status: "active",
      },
    },
  }));

  const eventName = "sftp:progress:" + transferId;
  EventsOn(
    eventName,
    (event: {
      transferId: string;
      status: string;
      currentFile?: string;
      filesCompleted?: number;
      filesTotal?: number;
      bytesDone?: number;
      bytesTotal?: number;
      speed?: number;
      error?: string;
    }) => {
      const transfers = get().transfers;
      const existing = transfers[transferId];
      if (!existing) return;

      switch (event.status) {
        case "progress":
          set((state) => ({
            transfers: {
              ...state.transfers,
              [transferId]: {
                ...existing,
                currentFile: event.currentFile || existing.currentFile,
                filesCompleted: event.filesCompleted ?? existing.filesCompleted,
                filesTotal: event.filesTotal ?? existing.filesTotal,
                bytesDone: event.bytesDone ?? existing.bytesDone,
                bytesTotal: event.bytesTotal ?? existing.bytesTotal,
                speed: event.speed ?? existing.speed,
              },
            },
          }));
          break;
        case "done":
          set((state) => ({
            transfers: {
              ...state.transfers,
              [transferId]: { ...existing, status: "done" },
            },
          }));
          EventsOff(eventName);
          // 5 秒后自动清除已完成的传输
          setTimeout(() => {
            const current = get().transfers[transferId];
            if (current && current.status === "done") {
              set((state) => {
                const { [transferId]: _, ...rest } = state.transfers;
                return { transfers: rest };
              });
            }
          }, 5000);
          break;
        case "error":
          set((state) => ({
            transfers: {
              ...state.transfers,
              [transferId]: {
                ...existing,
                status: event.error?.includes("context canceled") ? "cancelled" : "error",
                error: event.error,
              },
            },
          }));
          EventsOff(eventName);
          break;
      }
    }
  );
}

export const useSFTPStore = create<SFTPState>((set, get) => ({
  transfers: {},
  fileManagerOpenTabs: {},
  fileManagerPaths: {},
  fileManagerWidth: DEFAULT_FILE_MANAGER_WIDTH,

  startUpload: async (target, remotePath) => {
    const transferId = await SFTPUpload(target.sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, target, "upload", set, get);
    return transferId;
  },

  startUploadDir: async (target, remotePath) => {
    const transferId = await SFTPUploadDir(target.sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, target, "upload", set, get);
    return transferId;
  },

  startUploadFile: async (target, localPath, remotePath) => {
    const transferId = await SFTPUploadFile(target.sessionId, localPath, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, target, "upload", set, get);
    return transferId;
  },

  startDownload: async (target, remotePath) => {
    const transferId = await SFTPDownload(target.sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, target, "download", set, get);
    return transferId;
  },

  startDownloadDir: async (target, remotePath) => {
    const transferId = await SFTPDownloadDir(target.sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, target, "download", set, get);
    return transferId;
  },

  cancelTransfer: (transferId) => {
    SFTPCancelTransfer(transferId);
  },

  clearTransfer: (transferId) => {
    set((state) => {
      const { [transferId]: _, ...rest } = state.transfers;
      return { transfers: rest };
    });
  },

  clearCompleted: () => {
    set((state) => {
      const active: Record<string, SFTPTransfer> = {};
      for (const [id, t] of Object.entries(state.transfers)) {
        if (t.status === "active") {
          active[id] = t;
        }
      }
      return { transfers: active };
    });
  },

  clearCompletedForSession: (sessionId) => {
    set((state) => {
      const kept: Record<string, SFTPTransfer> = {};
      for (const [id, t] of Object.entries(state.transfers)) {
        if (t.sessionId !== sessionId || t.status === "active") {
          kept[id] = t;
        }
      }
      return { transfers: kept };
    });
  },

  clearCompletedForTab: (tabId) => {
    set((state) => {
      const kept: Record<string, SFTPTransfer> = {};
      for (const [id, t] of Object.entries(state.transfers)) {
        if (t.tabId !== tabId || t.status === "active") {
          kept[id] = t;
        }
      }
      return { transfers: kept };
    });
  },

  getSessionTransfers: (sessionId) => {
    return Object.values(get().transfers).filter((t) => t.sessionId === sessionId);
  },

  getTabTransfers: (tabId) => {
    return Object.values(get().transfers).filter((t) => t.tabId === tabId);
  },

  toggleFileManager: (tabId) => {
    set((state) => ({
      fileManagerOpenTabs: {
        ...state.fileManagerOpenTabs,
        [tabId]: !state.fileManagerOpenTabs[tabId],
      },
    }));
  },

  setFileManagerPath: (tabId, path) => {
    set((state) => ({
      fileManagerPaths: {
        ...state.fileManagerPaths,
        [tabId]: path,
      },
    }));
  },

  setFileManagerWidth: (width) => {
    set({
      fileManagerWidth: Math.max(MIN_FILE_MANAGER_WIDTH, Math.min(MAX_FILE_MANAGER_WIDTH, width)),
    });
  },
}));

registerTabCloseHook((tab) => {
  if (tab.type !== "terminal") return;
  useSFTPStore.setState((state) => {
    const nextOpenTabs = { ...state.fileManagerOpenTabs };
    delete nextOpenTabs[tab.id];
    const nextPaths = { ...state.fileManagerPaths };
    delete nextPaths[tab.id];
    return {
      fileManagerOpenTabs: nextOpenTabs,
      fileManagerPaths: nextPaths,
    };
  });
});

// SSH terminal tabs flip from connectionId → sessionId once the session establishes.
// Migrate keyed file-manager state across that rename so panels opened during
// connecting stay open after.
registerTabReplaceHook((oldId, newId) => {
  useSFTPStore.setState((state) => {
    const hasOpen = Object.prototype.hasOwnProperty.call(state.fileManagerOpenTabs, oldId);
    const hasPath = Object.prototype.hasOwnProperty.call(state.fileManagerPaths, oldId);
    if (!hasOpen && !hasPath) return state;
    const nextOpenTabs = { ...state.fileManagerOpenTabs };
    if (hasOpen) {
      nextOpenTabs[newId] = nextOpenTabs[oldId];
      delete nextOpenTabs[oldId];
    }
    const nextPaths = { ...state.fileManagerPaths };
    if (hasPath) {
      nextPaths[newId] = nextPaths[oldId];
      delete nextPaths[oldId];
    }
    return {
      fileManagerOpenTabs: nextOpenTabs,
      fileManagerPaths: nextPaths,
    };
  });
});
