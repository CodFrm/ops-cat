import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Upload } from "lucide-react";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  cn,
  ConfirmDialog,
} from "@opskat/ui";
import { SFTPDelete, SFTPGetwd } from "../../../wailsjs/go/app/App";
import { sftp_svc } from "../../../wailsjs/go/models";
import { openExternalEdit, type ExternalEditSession } from "@/lib/externalEditApi";
import { useExternalEditStore } from "@/stores/externalEditStore";
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
  assetId?: number;
  tabId: string;
  sessionId: string;
  isOpen: boolean;
  width: number;
  onWidthChange: (width: number) => void;
}

export function FileManagerPanel({ assetId, tabId, sessionId, isOpen, width, onWidthChange }: FileManagerPanelProps) {
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
  const allExternalSessions = useExternalEditStore((s) => s.sessions);
  const pendingConflict = useExternalEditStore((s) => s.pendingConflict);
  const dismissConflict = useExternalEditStore((s) => s.dismissConflict);
  const resolveConflict = useExternalEditStore((s) => s.resolveConflict);
  const savingSessionId = useExternalEditStore((s) => s.savingSessionId);
  const remoteChangedConflict = pendingConflict?.status === "conflict_remote_changed" ? pendingConflict : null;
  const remoteMissingConflict = pendingConflict?.status === "remote_missing" ? pendingConflict : null;

  const sessionTransfers = useMemo(
    () => Object.values(allTransfers).filter((transfer) => transfer.sessionId === sessionId),
    [allTransfers, sessionId]
  );
  const externalSessions = useMemo(
    () =>
      Object.values(allExternalSessions)
        .filter((session) => session.assetId === assetId)
        .sort((left, right) => right.updatedAt - left.updatedAt),
    [allExternalSessions, assetId]
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

  const canExternalEdit = useCallback((entry: sftp_svc.FileEntry) => !entry.isDir, []);

  const handleOpenExternalEdit = useCallback(
    async (remotePath: string) => {
      // 只有终端页真正绑定资产后才允许进入外部编辑链路，
      // 这样可以让历史测试和非终端场景继续复用组件，而不需要把 assetId 适配带回测试侧。
      if (!assetId) {
        return;
      }
      try {
        await openExternalEdit({
          assetId,
          sessionId,
          remotePath,
        });
      } catch (error) {
        setError(String(error));
      }
    },
    [assetId, sessionId, setError]
  );

  const handleSaveExternalEdit = useCallback(
    async (session: ExternalEditSession) => {
      try {
        await useExternalEditStore.getState().saveSession(session.id);
      } catch (error) {
        setError(String(error));
      }
    },
    [setError]
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
        case "externalEdit":
          if (entry) {
            void handleOpenExternalEdit(getFullPath(entry));
          }
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
      handleOpenExternalEdit,
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
              canExternalEdit={canExternalEdit}
              currentPath={currentPath}
              entries={entries}
              error={error}
              loading={loading}
              onExternalOpen={handleOpenExternalEdit}
              onGoUp={goUp}
              onNavigate={(path) => void navigateToPath(path)}
              onOpenContextMenu={(x, y, entry) =>
                setCtxMenu({ x, y, entry, canExternalEdit: entry ? canExternalEdit(entry) : false })
              }
              onRetry={() => void loadDir(currentPathRef.current)}
              selected={selected}
              setSelected={setSelected}
            />

            {externalSessions.length > 0 && (
              <div className="border-t px-2 py-2 space-y-2">
                <div className="text-[11px] font-medium text-muted-foreground">{t("externalEdit.panel.title")}</div>
                <div className="space-y-1">
                  {externalSessions.map((session) => {
                    const fileName = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
                    // stale 副本只用于保留冲突现场，不允许继续从面板直接覆盖回远端；
                    // 其余状态只有在本地确实有待处理变更时才展示可操作入口，避免误触发空保存。
                    const actionable =
                      session.state !== "stale" &&
                      (session.dirty || session.state === "conflict" || session.state === "remote_missing");
                    return (
                      <div
                        key={session.id}
                        className="rounded border bg-muted/20 px-2 py-1.5 text-[11px] flex items-center justify-between gap-2"
                      >
                        <div className="min-w-0">
                          <div className="truncate font-medium">{fileName}</div>
                          <div className="truncate text-muted-foreground">
                            {t(`externalEdit.state.${session.state}`)}
                          </div>
                        </div>
                        <Button
                          size="xs"
                          variant="outline"
                          disabled={!actionable || savingSessionId === session.id}
                          onClick={() => void handleSaveExternalEdit(session)}
                        >
                          {savingSessionId === session.id ? t("action.saving") : t("externalEdit.actions.sync")}
                        </Button>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            <TransferSection sessionId={sessionId} transfers={sessionTransfers} />
          </div>
        </div>
      </div>

      {ctxMenu && <FloatingMenu ctx={ctxMenu} onAction={handleCtxAction} onClose={() => setCtxMenu(null)} />}

      <AlertDialog
        open={!!remoteChangedConflict}
        onOpenChange={(open) => {
          if (!open) dismissConflict();
        }}
      >
        <AlertDialogContent onOverlayClick={dismissConflict}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("externalEdit.conflict.remoteChangedTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{remoteChangedConflict?.message || ""}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <Button variant="outline" onClick={dismissConflict}>
              {t("action.cancel")}
            </Button>
            <Button
              variant="outline"
              disabled={!remoteChangedConflict?.session || savingSessionId === remoteChangedConflict?.session?.id}
              onClick={async () => {
                if (!remoteChangedConflict?.session) return;
                try {
                  // reread 会创建一个新的 clean 会话，并把当前副本降级为 stale 副本；
                  // 弹窗在成功后立即关闭，避免用户对旧会话继续操作。
                  await resolveConflict(remoteChangedConflict.session.id, "reread");
                  dismissConflict();
                } catch (error) {
                  setError(String(error));
                }
              }}
            >
              {t("externalEdit.actions.reread")}
            </Button>
            <Button
              variant="destructive"
              disabled={!remoteChangedConflict?.session || savingSessionId === remoteChangedConflict?.session?.id}
              onClick={async () => {
                if (!remoteChangedConflict?.session) return;
                try {
                  // overwrite 直接把本地副本视为最终来源，因此要等后端返回成功后再清掉冲突态。
                  await resolveConflict(remoteChangedConflict.session.id, "overwrite");
                  dismissConflict();
                } catch (error) {
                  setError(String(error));
                }
              }}
            >
              {t("externalEdit.actions.overwrite")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <ConfirmDialog
        open={!!remoteMissingConflict}
        onOpenChange={(open) => {
          if (!open) dismissConflict();
        }}
        title={t("externalEdit.conflict.remoteMissingTitle")}
        description={remoteMissingConflict?.message || ""}
        cancelText={t("action.cancel")}
        confirmText={t("externalEdit.actions.saveAgain")}
        onConfirm={async () => {
          if (!remoteMissingConflict?.session) return;
          try {
            // recreate 只在远端缺失时开放，成功后会把当前会话重置为 clean，故该处清空前端挂起的冲突提示。
            await resolveConflict(remoteMissingConflict.session.id, "recreate");
            dismissConflict();
          } catch (error) {
            setError(String(error));
          }
        }}
      />

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
