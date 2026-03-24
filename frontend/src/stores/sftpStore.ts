import { create } from "zustand";
import {
  SFTPUpload,
  SFTPUploadDir,
  SFTPUploadFile,
  SFTPDownload,
  SFTPDownloadDir,
  SFTPCancelTransfer,
} from "../../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";

export interface SFTPTransfer {
  transferId: string;
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

const DEFAULT_FILE_MANAGER_WIDTH = 280;
const MIN_FILE_MANAGER_WIDTH = 200;
const MAX_FILE_MANAGER_WIDTH = 600;

interface SFTPState {
  transfers: Record<string, SFTPTransfer>;

  // File manager panel state
  fileManagerOpenTabs: Record<string, boolean>;
  fileManagerWidth: number;

  startUpload: (sessionId: string, remotePath: string) => Promise<string | null>;
  startUploadDir: (sessionId: string, remotePath: string) => Promise<string | null>;
  startUploadFile: (sessionId: string, localPath: string, remotePath: string) => Promise<string | null>;
  startDownload: (sessionId: string, remotePath: string) => Promise<string | null>;
  startDownloadDir: (sessionId: string, remotePath: string) => Promise<string | null>;
  cancelTransfer: (transferId: string) => void;
  clearTransfer: (transferId: string) => void;
  clearCompleted: () => void;
  getSessionTransfers: (sessionId: string) => SFTPTransfer[];

  toggleFileManager: (tabId: string) => void;
  setFileManagerWidth: (width: number) => void;
  isFileManagerOpen: (tabId: string) => boolean;
}

function subscribeProgress(
  transferId: string,
  sessionId: string,
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
        sessionId,
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
                status: event.error?.includes("context canceled")
                  ? "cancelled"
                  : "error",
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
  fileManagerWidth: DEFAULT_FILE_MANAGER_WIDTH,

  startUpload: async (sessionId, remotePath) => {
    const transferId = await SFTPUpload(sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, sessionId, "upload", set, get);
    return transferId;
  },

  startUploadDir: async (sessionId, remotePath) => {
    const transferId = await SFTPUploadDir(sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, sessionId, "upload", set, get);
    return transferId;
  },

  startUploadFile: async (sessionId, localPath, remotePath) => {
    const transferId = await SFTPUploadFile(sessionId, localPath, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, sessionId, "upload", set, get);
    return transferId;
  },

  startDownload: async (sessionId, remotePath) => {
    const transferId = await SFTPDownload(sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, sessionId, "download", set, get);
    return transferId;
  },

  startDownloadDir: async (sessionId, remotePath) => {
    const transferId = await SFTPDownloadDir(sessionId, remotePath);
    if (!transferId) return null;
    subscribeProgress(transferId, sessionId, "download", set, get);
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

  getSessionTransfers: (sessionId) => {
    return Object.values(get().transfers).filter(
      (t) => t.sessionId === sessionId
    );
  },

  toggleFileManager: (tabId) => {
    set((state) => ({
      fileManagerOpenTabs: {
        ...state.fileManagerOpenTabs,
        [tabId]: !state.fileManagerOpenTabs[tabId],
      },
    }));
  },

  setFileManagerWidth: (width) => {
    set({
      fileManagerWidth: Math.max(
        MIN_FILE_MANAGER_WIDTH,
        Math.min(MAX_FILE_MANAGER_WIDTH, width)
      ),
    });
  },

  isFileManagerOpen: (tabId) => {
    return !!get().fileManagerOpenTabs[tabId];
  },
}));
