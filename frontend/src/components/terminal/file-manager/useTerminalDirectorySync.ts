import { useCallback, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ChangeSSHDirectory, EnableSSHSync, GetSSHSyncState } from "../../../../wailsjs/go/app/App";
import { DIRSYNC_ERROR_CODES, type DirSyncErrorCode, formatDirSyncError } from "@/lib/dirSyncErrors";
import { useTerminalStore, type TerminalDirectorySyncState } from "@/stores/terminalStore";
import { normalizeRemotePath } from "./utils";

interface UseTerminalDirectorySyncOptions {
  currentPathRef: MutableRefObject<string>;
  loadDir: (dirPath: string) => Promise<boolean>;
  sessionId: string;
  tabId: string;
}

export function useTerminalDirectorySync({
  currentPathRef,
  loadDir,
  sessionId,
  tabId,
}: UseTerminalDirectorySyncOptions) {
  const { t } = useTranslation();
  const directoryFollowMode = useTerminalStore((s) => s.tabData[tabId]?.directoryFollowMode ?? "off");
  const setDirectoryFollowMode = useTerminalStore((s) => s.setDirectoryFollowMode);
  const sessionSync = useTerminalStore((s) => s.sessionSync[sessionId]);
  const paneConnected = useTerminalStore((s) => s.tabData[tabId]?.panes[sessionId]?.connected ?? false);

  const showSyncError = useCallback(
    (err: unknown) => {
      toast.error(formatDirSyncError(t, err));
    },
    [t]
  );

  const showSyncCode = useCallback(
    (code: DirSyncErrorCode) => {
      showSyncError(code);
    },
    [showSyncError]
  );

  const syncPanelFromTerminal = useCallback(async () => {
    let sync = sessionSync;
    if (!sync || !sync.supported) {
      try {
        await EnableSSHSync(sessionId);
        const latest = (await GetSSHSyncState(sessionId)) as TerminalDirectorySyncState;
        useTerminalStore.getState().setSessionSyncState(sessionId, latest);
        sync = latest;
      } catch (e) {
        showSyncError(e);
        return false;
      }
    }
    if (!sync) {
      showSyncCode(DIRSYNC_ERROR_CODES.CWD_UNKNOWN);
      return false;
    }
    if (!sync.supported) {
      showSyncCode(DIRSYNC_ERROR_CODES.UNSUPPORTED);
      return false;
    }
    if (!sync.cwdKnown || !sync.cwd) {
      showSyncCode(DIRSYNC_ERROR_CODES.CWD_UNKNOWN);
      return false;
    }
    return loadDir(sync.cwd);
  }, [loadDir, sessionId, sessionSync, showSyncCode, showSyncError]);

  const syncTerminalToPath = useCallback(
    async (targetPath: string) => {
      try {
        await ChangeSSHDirectory(sessionId, targetPath);
        return true;
      } catch (e) {
        showSyncError(e);
        return false;
      }
    },
    [sessionId, showSyncError]
  );

  const navigateToPath = useCallback(
    async (dirPath: string) => {
      const targetPath = normalizeRemotePath(currentPathRef.current, dirPath);
      if (directoryFollowMode === "always") {
        const changed = await syncTerminalToPath(targetPath);
        if (!changed) return false;
      }
      return loadDir(targetPath);
    },
    [currentPathRef, directoryFollowMode, loadDir, syncTerminalToPath]
  );

  const toggleFollowMode = useCallback(async () => {
    if (directoryFollowMode === "always") {
      setDirectoryFollowMode(tabId, "off");
      return;
    }

    if (sessionSync?.busy) {
      showSyncCode(DIRSYNC_ERROR_CODES.BUSY);
      return;
    }

    const synced = await syncPanelFromTerminal();
    if (!synced) return;
    setDirectoryFollowMode(tabId, "always");
  }, [directoryFollowMode, sessionSync?.busy, setDirectoryFollowMode, showSyncCode, syncPanelFromTerminal, tabId]);

  return {
    directoryFollowMode,
    navigateToPath,
    paneConnected,
    sessionSync,
    syncPanelFromTerminal,
    syncTerminalToPath,
    toggleFollowMode,
  };
}
