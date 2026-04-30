import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Upload } from "lucide-react";
import { cn, ConfirmDialog } from "@opskat/ui";
import { SFTPDelete, SFTPGetwd } from "../../../wailsjs/go/app/App";
import { sftp_svc } from "../../../wailsjs/go/models";
import { useSFTPStore } from "@/stores/sftpStore";
import { FileList } from "./file-manager/FileList";
import { FloatingMenu } from "./file-manager/FloatingMenu";
import { PathToolbar } from "./file-manager/PathToolbar";
import { TransferSection } from "./file-manager/TransferSection";
import { type DeleteTarget, type CtxMenuState } from "./file-manager/types";
import { useFileManagerDirectory } from "./file-manager/useFileManagerDirectory";
import { useNativeFileDrop } from "./file-manager/useNativeFileDrop";
import { useResizeHandle } from "./file-manager/useResizeHandle";
import { useTerminalDirectorySync } from "./file-manager/useTerminalDirectorySync";
import { getEntryPath, getParentPath, HANDLE_PX } from "./file-manager/utils";

interface FileManagerPanelProps {
  tabId: string;
  sessionId: string;
  isOpen: boolean;
  width: number;
  onWidthChange: (width: number) => void;
}

export function FileManagerPanel({ tabId, sessionId, isOpen, width, onWidthChange }: FileManagerPanelProps) {
  const { t } = useTranslation();
  const {
    currentPath,
    currentPathRef,
    entries,
    error,
    loading,
    loadDir,
    pathInput,
    selected,
    setError,
    setPathInput,
    setSelected,
    storedPath,
  } = useFileManagerDirectory(tabId, sessionId);

  const {
    directoryFollowMode,
    navigateToPath,
    paneConnected,
    sessionSync,
    syncPanelFromTerminal,
    syncTerminalToPath,
    toggleFollowMode,
  } = useTerminalDirectorySync({
    currentPathRef,
    loadDir,
    sessionId,
    tabId,
  });

  const [ctxMenu, setCtxMenu] = useState<CtxMenuState | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const loadedRef = useRef(false);
  const lastSessionRef = useRef<string | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const startUpload = useSFTPStore((s) => s.startUpload);
  const startUploadDir = useSFTPStore((s) => s.startUploadDir);
  const startUploadFile = useSFTPStore((s) => s.startUploadFile);
  const startDownload = useSFTPStore((s) => s.startDownload);
  const startDownloadDir = useSFTPStore((s) => s.startDownloadDir);
  const allTransfers = useSFTPStore((s) => s.transfers);

  const sessionTransfers = useMemo(
    () => Object.values(allTransfers).filter((transfer) => transfer.sessionId === sessionId),
    [allTransfers, sessionId]
  );

  const isDragOver = useNativeFileDrop({
    currentPathRef,
    isOpen,
    panelRef,
    sessionId,
    startUploadFile,
  });
  const { handleResizeStart, isResizing, outerRef } = useResizeHandle({ onWidthChange, panelRef, width });

  useEffect(() => {
    if (!sessionId) return;
    if (lastSessionRef.current !== sessionId) {
      lastSessionRef.current = sessionId;
      loadedRef.current = false;
    }
    if (!isOpen || loadedRef.current) return;
    loadedRef.current = true;

    if (directoryFollowMode === "always" && sessionSync?.cwdKnown && sessionSync.cwd) {
      void loadDir(sessionSync.cwd);
      return;
    }
    if (storedPath) {
      void loadDir(storedPath);
      return;
    }

    SFTPGetwd(sessionId)
      .then((home) => loadDir(home || "/"))
      .catch(() => loadDir("/"));
  }, [sessionId, isOpen, directoryFollowMode, sessionSync?.cwdKnown, sessionSync?.cwd, storedPath, loadDir]);

  useEffect(() => {
    if (!isOpen || directoryFollowMode !== "always") return;
    if (!sessionSync?.cwdKnown || !sessionSync.cwd) return;
    if (sessionSync.cwd === currentPath) return;
    void loadDir(sessionSync.cwd);
  }, [currentPath, directoryFollowMode, isOpen, loadDir, sessionSync?.cwd, sessionSync?.cwdKnown]);

  const doneUploadCount = sessionTransfers.filter((transfer) => {
    return transfer.status === "done" && transfer.direction === "upload";
  }).length;
  const prevDoneCount = useRef(0);
  useEffect(() => {
    if (doneUploadCount > prevDoneCount.current) {
      void loadDir(currentPathRef.current);
    }
    prevDoneCount.current = doneUploadCount;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doneUploadCount]);

  const getFullPath = useCallback((entry: sftp_svc.FileEntry) => getEntryPath(currentPath, entry), [currentPath]);

  const goUp = useCallback(() => {
    if (currentPath === "/") return;
    void navigateToPath(getParentPath(currentPath));
  }, [currentPath, navigateToPath]);

  const goHome = useCallback(() => {
    SFTPGetwd(sessionId)
      .then((home) => navigateToPath(home || "/"))
      .catch(() => navigateToPath("/"));
  }, [navigateToPath, sessionId]);

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      await SFTPDelete(sessionId, deleteTarget.path, deleteTarget.isDir);
      await loadDir(currentPathRef.current);
    } catch (e) {
      setError(String(e));
    } finally {
      setDeleteTarget(null);
    }
  }, [currentPathRef, deleteTarget, loadDir, sessionId, setError]);

  const handleCtxAction = useCallback(
    (action: string) => {
      if (!ctxMenu) return;
      const entry = ctxMenu.entry;
      setCtxMenu(null);

      switch (action) {
        case "open":
          if (entry?.isDir) void navigateToPath(getFullPath(entry));
          break;
        case "download":
          if (entry) startDownload(sessionId, getFullPath(entry));
          break;
        case "downloadDir":
          if (entry) startDownloadDir(sessionId, getFullPath(entry));
          break;
        case "upload":
          startUpload(sessionId, currentPath.endsWith("/") ? currentPath : currentPath + "/");
          break;
        case "uploadDir":
          startUploadDir(sessionId, currentPath.endsWith("/") ? currentPath : currentPath + "/");
          break;
        case "delete":
          if (entry) {
            setDeleteTarget({
              path: getFullPath(entry),
              name: entry.name,
              isDir: entry.isDir,
            });
          }
          break;
        case "refresh":
          void loadDir(currentPathRef.current);
          break;
      }
    },
    [
      ctxMenu,
      currentPath,
      currentPathRef,
      getFullPath,
      loadDir,
      navigateToPath,
      sessionId,
      startDownload,
      startDownloadDir,
      startUpload,
      startUploadDir,
    ]
  );

  const totalWidth = width + HANDLE_PX;

  return (
    <>
      <div
        ref={outerRef}
        className="shrink-0 overflow-hidden transition-[width] duration-200 ease-out"
        style={{
          width: isOpen ? totalWidth : 0,
          pointerEvents: isOpen ? "auto" : "none",
        }}
      >
        <div className="flex h-full" style={{ minWidth: totalWidth }}>
          <div
            className={cn(
              "w-1 cursor-col-resize hover:bg-primary/20 transition-colors shrink-0",
              isResizing && "bg-primary/30"
            )}
            onMouseDown={handleResizeStart}
          />

          <div
            ref={panelRef}
            className="flex flex-col border-l bg-background relative overflow-hidden"
            style={
              {
                width,
                "--wails-drop-target": isOpen ? "drop" : undefined,
              } as CSSProperties
            }
          >
            {isDragOver && (
              <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center bg-primary/5 border-2 border-dashed border-primary/30 rounded animate-in fade-in-0 duration-150">
                <div className="flex flex-col items-center gap-1 text-primary/60">
                  <Upload className="h-5 w-5" />
                  <span className="text-xs">{t("sftp.dropToUpload")}</span>
                </div>
              </div>
            )}

            <PathToolbar
              currentPath={currentPath}
              directoryFollowMode={directoryFollowMode}
              onFollowToggle={() => void toggleFollowMode()}
              onGoHome={goHome}
              onGoUp={goUp}
              onPathInputChange={setPathInput}
              onPathSubmit={(nextPath) => void navigateToPath(nextPath)}
              onRefresh={() => void loadDir(currentPathRef.current)}
              onSyncPanelFromTerminal={() => void syncPanelFromTerminal()}
              onSyncTerminalToPath={() => void syncTerminalToPath(currentPath)}
              paneConnected={paneConnected}
              pathInput={pathInput}
            />

            <FileList
              currentPath={currentPath}
              entries={entries}
              error={error}
              loading={loading}
              onGoUp={goUp}
              onNavigate={(path) => void navigateToPath(path)}
              onOpenContextMenu={(x, y, entry) => setCtxMenu({ x, y, entry })}
              onRetry={() => void loadDir(currentPathRef.current)}
              selected={selected}
              setSelected={setSelected}
            />

            <TransferSection sessionId={sessionId} transfers={sessionTransfers} />
          </div>
        </div>
      </div>

      {ctxMenu && <FloatingMenu ctx={ctxMenu} onAction={handleCtxAction} onClose={() => setCtxMenu(null)} />}

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("sftp.deleteConfirmTitle")}
        description={t("sftp.deleteConfirmDesc", { name: deleteTarget?.name })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={handleDelete}
      />
    </>
  );
}
