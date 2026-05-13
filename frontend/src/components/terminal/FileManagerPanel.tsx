import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Upload } from "lucide-react";
import { cn, ConfirmDialog } from "@opskat/ui";
import {
  SFTPChmod,
  SFTPCreateFile,
  SFTPDelete,
  SFTPGetwd,
  SFTPMkdir,
  SFTPRename,
  SFTPStat,
} from "../../../wailsjs/go/app/App";
import { sftp_svc } from "../../../wailsjs/go/models";
import { useSFTPStore } from "@/stores/sftpStore";
import { FileList } from "./file-manager/FileList";
import { FloatingMenu } from "./file-manager/FloatingMenu";
import { PathToolbar } from "./file-manager/PathToolbar";
import { PermissionDialog } from "./file-manager/PermissionDialog";
import { TransferSection } from "./file-manager/TransferSection";
import { type CtxMenuState, type DeleteTarget, type EditingState, type PermissionTarget } from "./file-manager/types";
import { useFileManagerDirectory } from "./file-manager/useFileManagerDirectory";
import { useNativeFileDrop } from "./file-manager/useNativeFileDrop";
import { useResizeHandle } from "./file-manager/useResizeHandle";
import { useTerminalDirectorySync } from "./file-manager/useTerminalDirectorySync";
import { getEntryPath, getParentPath, HANDLE_PX, joinRemotePath, validateEntryName } from "./file-manager/utils";

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
  const [editingState, setEditingState] = useState<EditingState | null>(null);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSubmitting, setEditSubmitting] = useState(false);
  const [permTarget, setPermTarget] = useState<PermissionTarget | null>(null);
  const [permError, setPermError] = useState<string | null>(null);
  const [permSubmitting, setPermSubmitting] = useState(false);
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

  const beginNewFile = useCallback(() => {
    setEditError(null);
    setEditingState({ mode: "create-file" });
  }, []);
  const beginNewFolder = useCallback(() => {
    setEditError(null);
    setEditingState({ mode: "create-dir" });
  }, []);
  const beginRename = useCallback((entry: sftp_svc.FileEntry) => {
    setEditError(null);
    setEditingState({ mode: "rename", targetName: entry.name });
  }, []);

  const cancelEdit = useCallback(() => {
    setEditingState(null);
    setEditError(null);
    setEditSubmitting(false);
  }, []);

  const commitEdit = useCallback(
    async (rawName: string) => {
      if (!editingState) return;
      const name = rawName.trim();
      const validationKey = validateEntryName(name);
      if (validationKey) {
        setEditError(t("sftp." + validationKey));
        return;
      }
      const base = currentPathRef.current || "/";
      const newPath = joinRemotePath(base, name);

      setEditSubmitting(true);
      setEditError(null);
      try {
        if (editingState.mode === "rename") {
          if (name === editingState.targetName) {
            // 无变化，关闭编辑
            cancelEdit();
            return;
          }
          const oldPath = joinRemotePath(base, editingState.targetName);
          await SFTPRename(sessionId, oldPath, newPath);
        } else if (editingState.mode === "create-dir") {
          await SFTPMkdir(sessionId, newPath);
        } else {
          await SFTPCreateFile(sessionId, newPath);
        }
        await loadDir(currentPathRef.current);
        setSelected(newPath);
        setEditingState(null);
      } catch (e) {
        const msg = String(e);
        if (msg.includes("已存在")) {
          setEditError(t("sftp.errExists"));
        } else {
          setEditError(msg);
        }
      } finally {
        setEditSubmitting(false);
      }
    },
    [cancelEdit, currentPathRef, editingState, loadDir, sessionId, setSelected, t]
  );

  const openPermissions = useCallback(
    async (entry: sftp_svc.FileEntry) => {
      const fullPath = getFullPath(entry);
      try {
        const latest = await SFTPStat(sessionId, fullPath);
        setPermError(null);
        setPermTarget({
          path: fullPath,
          name: entry.name,
          mode: latest.mode,
          isDir: latest.isDir,
        });
      } catch (e) {
        setError(String(e));
      }
    },
    [getFullPath, sessionId, setError]
  );

  const handlePermConfirm = useCallback(
    async (mode: number, recursive: boolean) => {
      if (!permTarget) return;
      setPermSubmitting(true);
      setPermError(null);
      try {
        await SFTPChmod(sessionId, permTarget.path, mode, recursive);
        await loadDir(currentPathRef.current);
        setPermTarget(null);
      } catch (e) {
        setPermError(String(e));
      } finally {
        setPermSubmitting(false);
      }
    },
    [currentPathRef, loadDir, permTarget, sessionId]
  );

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
        case "rename":
          if (entry) beginRename(entry);
          break;
        case "chmod":
          if (entry) void openPermissions(entry);
          break;
        case "newFile":
          beginNewFile();
          break;
        case "newDir":
          beginNewFolder();
          break;
        case "goUp":
          goUp();
          break;
        case "refresh":
          void loadDir(currentPathRef.current);
          break;
      }
    },
    [
      beginNewFile,
      beginNewFolder,
      beginRename,
      ctxMenu,
      currentPath,
      currentPathRef,
      getFullPath,
      goUp,
      loadDir,
      navigateToPath,
      openPermissions,
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
              onNewFile={beginNewFile}
              onNewFolder={beginNewFolder}
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
              editingState={editingState}
              editError={editError}
              editSubmitting={editSubmitting}
              onEditCommit={(name) => void commitEdit(name)}
              onEditCancel={cancelEdit}
            />

            <TransferSection sessionId={sessionId} transfers={sessionTransfers} />
          </div>
        </div>
      </div>

      {ctxMenu && (
        <FloatingMenu
          ctx={ctxMenu}
          canGoUp={currentPath !== "/"}
          onAction={handleCtxAction}
          onClose={() => setCtxMenu(null)}
        />
      )}

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

      <PermissionDialog
        target={permTarget}
        submitting={permSubmitting}
        error={permError}
        onClose={() => {
          setPermTarget(null);
          setPermError(null);
        }}
        onConfirm={(mode, recursive) => void handlePermConfirm(mode, recursive)}
      />
    </>
  );
}
