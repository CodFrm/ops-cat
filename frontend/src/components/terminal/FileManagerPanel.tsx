import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, ArrowRightLeft, RefreshCw, Trash2, Upload } from "lucide-react";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@opskat/ui";
import { SFTPDelete, SFTPGetwd } from "../../../wailsjs/go/app/App";
import { sftp_svc } from "../../../wailsjs/go/models";
import { CodeDiffViewer } from "@/components/CodeDiffViewer";
import { openExternalEdit, type ExternalEditSession } from "@/lib/externalEditApi";
import {
  buildExternalEditErrors,
  buildExternalEditConflicts,
  buildExternalEditDocuments,
  useExternalEditStore,
} from "@/stores/externalEditStore";
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
  const dismissCompare = useExternalEditStore((s) => s.dismissCompare);
  const deleteSession = useExternalEditStore((s) => s.deleteSession);
  const dismissErrorDetail = useExternalEditStore((s) => s.dismissErrorDetail);
  const compareResult = useExternalEditStore((s) => s.compareResult);
  const compareSession = useExternalEditStore((s) => s.compareSession);
  const selectedError = useExternalEditStore((s) => s.selectedError);
  const openErrorDetail = useExternalEditStore((s) => s.openErrorDetail);
  const refreshSession = useExternalEditStore((s) => s.refreshSession);
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
      buildExternalEditDocuments(allExternalSessions)
        .map((entry) => entry.session)
        .filter((session) => session.assetId === assetId),
    [allExternalSessions, assetId]
  );
  const conflictDocuments = useMemo(
    () => buildExternalEditConflicts(allExternalSessions).filter((entry) => entry.primaryDraft.assetId === assetId),
    [allExternalSessions, assetId]
  );
  const errorDocuments = useMemo(
    () => buildExternalEditErrors(allExternalSessions).filter((entry) => entry.session.assetId === assetId),
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
      // 兼容旧调用方：只有终端页真正绑定资产后才允许进入外部编辑链路，
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

  const handleRefreshExternalEdit = useCallback(
    async (session: ExternalEditSession) => {
      try {
        await refreshSession(session.id);
      } catch (error) {
        setError(String(error));
      }
    },
    [refreshSession, setError]
  );

  const handleCompareExternalEdit = useCallback(
    async (session: ExternalEditSession) => {
      try {
        await compareSession(session.id);
      } catch (error) {
        setError(String(error));
      }
    },
    [compareSession, setError]
  );

  const handleDeleteExternalEdit = useCallback(
    async (session: ExternalEditSession, removeLocal: boolean) => {
      try {
        await deleteSession(session.id, removeLocal);
      } catch (error) {
        setError(String(error));
      }
    },
    [deleteSession, setError]
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

            {(externalSessions.length > 0 || conflictDocuments.length > 0 || errorDocuments.length > 0) && (
              <div className="border-t px-2 py-2 space-y-2">
                <div className="text-[11px] font-medium text-muted-foreground">{t("externalEdit.panel.title")}</div>
                {conflictDocuments.length > 0 && (
                  <div className="space-y-1">
                    <div className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                      {t("externalEdit.panel.conflicts")}
                    </div>
                    {conflictDocuments.map(
                      ({ documentKey, primaryDraft, retainedDraft, activeDraft, latestSnapshot }) => {
                        const fileName =
                          primaryDraft.remotePath.split("/").filter(Boolean).pop() || primaryDraft.remotePath;
                        const sameNameCount = Object.values(allExternalSessions).filter((session) => {
                          const candidate = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
                          return candidate === fileName;
                        }).length;
                        const showPath = sameNameCount > 1;
                        const compareDisabled =
                          savingSessionId === primaryDraft.id ||
                          (primaryDraft.state !== "conflict" && primaryDraft.state !== "stale");
                        const rereadDisabled = savingSessionId === primaryDraft.id || primaryDraft.state !== "conflict";
                        const overwriteDisabled =
                          savingSessionId === primaryDraft.id || primaryDraft.state !== "conflict";
                        const recreateDisabled =
                          savingSessionId === primaryDraft.id || primaryDraft.state !== "remote_missing";
                        return (
                          <div
                            key={documentKey}
                            className="rounded border border-amber-400/30 bg-amber-500/5 px-2 py-2 text-[11px]"
                          >
                            <div className="flex items-start justify-between gap-2">
                              <div className="min-w-0">
                                <div className="truncate font-medium">{fileName}</div>
                                {showPath && (
                                  <div className="truncate text-muted-foreground">{primaryDraft.remotePath}</div>
                                )}
                                <div className="truncate text-muted-foreground">
                                  {t(`externalEdit.state.${primaryDraft.state}`)}
                                  {activeDraft || latestSnapshot
                                    ? ` · ${t("externalEdit.panel.remoteSnapshotReady")}`
                                    : ""}
                                </div>
                              </div>
                            </div>
                            <div className="mt-2 grid gap-2 lg:grid-cols-2">
                              <div className="rounded border bg-background/80 px-2 py-2">
                                <div className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                                  {primaryDraft.state === "stale"
                                    ? t("externalEdit.panel.retainedDraft")
                                    : t("externalEdit.panel.currentDraft")}
                                </div>
                                <div className="mt-1 truncate font-medium">{fileName}</div>
                                {showPath && (
                                  <div className="truncate text-muted-foreground">{primaryDraft.remotePath}</div>
                                )}
                                <div className="truncate text-muted-foreground">
                                  {t(`externalEdit.state.${primaryDraft.state}`)}
                                </div>
                              </div>
                              {retainedDraft && retainedDraft.id !== primaryDraft.id && (
                                <div className="rounded border bg-background/80 px-2 py-2">
                                  <div className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                                    {t("externalEdit.panel.retainedDraft")}
                                  </div>
                                  <div className="mt-1 truncate font-medium">{fileName}</div>
                                  {showPath && (
                                    <div className="truncate text-muted-foreground">{retainedDraft.remotePath}</div>
                                  )}
                                  <div className="truncate text-muted-foreground">
                                    {t(`externalEdit.state.${retainedDraft.state}`)}
                                  </div>
                                </div>
                              )}
                              {activeDraft && (
                                <div className="rounded border bg-background/80 px-2 py-2">
                                  <div className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                                    {t("externalEdit.panel.rereadDraft")}
                                  </div>
                                  <div className="mt-1 truncate font-medium">{fileName}</div>
                                  {showPath && (
                                    <div className="truncate text-muted-foreground">{activeDraft.remotePath}</div>
                                  )}
                                  <div className="truncate text-muted-foreground">
                                    {t(`externalEdit.state.${activeDraft.state}`)}
                                  </div>
                                </div>
                              )}
                            </div>
                            <div className="mt-2 flex flex-wrap gap-1">
                              <Button
                                size="xs"
                                variant="outline"
                                disabled={compareDisabled}
                                onClick={() => void handleCompareExternalEdit(primaryDraft)}
                              >
                                <ArrowRightLeft className="mr-1 h-3 w-3" />
                                {t("externalEdit.actions.compare")}
                              </Button>
                              <Button
                                size="xs"
                                variant="outline"
                                disabled={rereadDisabled}
                                onClick={async () => {
                                  try {
                                    await resolveConflict(primaryDraft.id, "reread");
                                    dismissConflict();
                                  } catch (error) {
                                    setError(String(error));
                                  }
                                }}
                              >
                                {t("externalEdit.actions.reread")}
                              </Button>
                              <Button
                                size="xs"
                                variant="destructive"
                                disabled={overwriteDisabled}
                                onClick={async () => {
                                  try {
                                    await resolveConflict(primaryDraft.id, "overwrite");
                                    dismissConflict();
                                  } catch (error) {
                                    setError(String(error));
                                  }
                                }}
                              >
                                {t("externalEdit.actions.overwrite")}
                              </Button>
                              <Button
                                size="xs"
                                variant="outline"
                                disabled={recreateDisabled}
                                onClick={async () => {
                                  try {
                                    await resolveConflict(primaryDraft.id, "recreate");
                                    dismissConflict();
                                  } catch (error) {
                                    setError(String(error));
                                  }
                                }}
                              >
                                {t("externalEdit.actions.saveAgain")}
                              </Button>
                            </div>
                          </div>
                        );
                      }
                    )}
                  </div>
                )}
                {errorDocuments.length > 0 && (
                  <div className="space-y-1">
                    <div className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                      {t("externalEdit.panel.errors")}
                    </div>
                    {errorDocuments.map(({ session }) => {
                      const fileName = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
                      return (
                        <div
                          key={session.id}
                          className="rounded border border-rose-400/30 bg-rose-500/5 px-2 py-2 text-[11px] flex items-start justify-between gap-2"
                        >
                          <div className="min-w-0">
                            <div className="truncate font-medium">{fileName}</div>
                            <div className="truncate text-muted-foreground">{session.lastError?.summary}</div>
                          </div>
                          <div className="flex items-center gap-1">
                            <Button size="xs" variant="outline" onClick={() => openErrorDetail(session.id)}>
                              <AlertTriangle className="mr-1 h-3 w-3" />
                              {t("externalEdit.actions.viewError")}
                            </Button>
                            <Button
                              size="xs"
                              variant="outline"
                              onClick={() => void handleDeleteExternalEdit(session, false)}
                            >
                              {t("externalEdit.actions.hideRecord")}
                            </Button>
                            <Button
                              size="xs"
                              variant="destructive"
                              onClick={() => void handleDeleteExternalEdit(session, true)}
                            >
                              <Trash2 className="mr-1 h-3 w-3" />
                              {t("externalEdit.actions.deleteLocal")}
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
                <div className="space-y-1">
                  {externalSessions.map((session) => {
                    const fileName = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
                    const isRereadDraft = !!session.sourceSessionId;
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
                          <div className="flex items-center gap-1.5">
                            <div className="truncate font-medium">{fileName}</div>
                            {isRereadDraft && (
                              <span className="rounded-full border border-border bg-background px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                                {t("externalEdit.panel.rereadDraft")}
                              </span>
                            )}
                          </div>
                          <div className="truncate text-muted-foreground">
                            {t(`externalEdit.state.${session.state}`)}
                            {isRereadDraft ? ` · ${t("externalEdit.panel.rereadDraftHint")}` : ""}
                          </div>
                        </div>
                        <div className="flex items-center gap-1">
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={savingSessionId === session.id}
                            onClick={() => void handleRefreshExternalEdit(session)}
                          >
                            <RefreshCw className="mr-1 h-3 w-3" />
                            {t("externalEdit.actions.refresh")}
                          </Button>
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={!actionable || savingSessionId === session.id}
                            onClick={() => void handleSaveExternalEdit(session)}
                          >
                            {savingSessionId === session.id ? t("action.saving") : t("externalEdit.actions.sync")}
                          </Button>
                          <Button
                            size="xs"
                            variant="ghost"
                            onClick={() => void handleDeleteExternalEdit(session, false)}
                          >
                            {t("externalEdit.actions.hideRecord")}
                          </Button>
                        </div>
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
            <Button variant="outline" onClick={dismissConflict}>
              {t("externalEdit.panel.reviewInList")}
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
        confirmText={t("externalEdit.panel.reviewInList")}
        onConfirm={dismissConflict}
      />

      <Dialog open={!!compareResult} onOpenChange={(open) => !open && dismissCompare()}>
        <DialogContent className="flex h-[88vh] max-w-[min(96vw,1600px)] flex-col overflow-hidden p-0">
          <DialogHeader className="border-b px-6 py-4">
            <DialogTitle>{t("externalEdit.compare.title")}</DialogTitle>
            <DialogDescription>
              {compareResult ? `${compareResult.fileName} · ${compareResult.remotePath}` : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="flex min-h-0 flex-1 flex-col px-6 py-4">
            <div className="mb-3 flex items-center justify-between gap-2 text-xs text-muted-foreground">
              <div>{t("externalEdit.compare.helper")}</div>
              <div className="rounded-full border border-border bg-background px-2 py-1 font-medium">
                {t("externalEdit.compare.readOnly")}
              </div>
            </div>
            <div className="min-h-0 flex-1">
              <CodeDiffViewer
                original={compareResult?.remoteContent || ""}
                modified={compareResult?.localContent || ""}
                originalTitle={t("externalEdit.compare.remoteSnapshot")}
                modifiedTitle={t("externalEdit.compare.localDraft")}
                badge={t("externalEdit.compare.readOnly")}
                language="plaintext"
                height="100%"
              />
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!selectedError} onOpenChange={(open) => !open && dismissErrorDetail()}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("externalEdit.error.title")}</DialogTitle>
            <DialogDescription>{selectedError ? `${selectedError.remotePath}` : ""}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm">
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.summaryLabel")}</div>
              <div>{selectedError?.lastError?.summary || ""}</div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.stepLabel")}</div>
              <div>{selectedError?.lastError?.step || ""}</div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.suggestionLabel")}</div>
              <div>{selectedError?.lastError?.suggestion || ""}</div>
            </div>
          </div>
        </DialogContent>
      </Dialog>

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
