import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, Upload } from "lucide-react";
import {
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
import { openExternalEdit, type ExternalEditMergePrepareResult, type ExternalEditSession } from "@/lib/externalEditApi";
import {
  buildExternalEditAttentionItems,
  isExternalEditClipboardResidueSession,
  useExternalEditStore,
} from "@/stores/externalEditStore";
import { useSFTPStore } from "@/stores/sftpStore";
import { ExternalEditCompareWorkbench } from "./external-edit/CompareWorkbench";
import { ExternalEditMergeWorkbench } from "./external-edit/MergeWorkbench";
import { ExternalEditPendingDialog, type ExternalEditPendingItem } from "./external-edit/PendingDialog";
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
  isActive?: boolean;
  isOpen: boolean;
  width: number;
  onWidthChange: (width: number) => void;
}

const EXTERNAL_EDIT_SAFE_ERROR_KEY = "externalEdit.error.safeActionFailed";

export function FileManagerPanel({
  assetId,
  tabId,
  sessionId,
  isActive = true,
  isOpen,
  width,
  onWidthChange,
}: FileManagerPanelProps) {
  const { t } = useTranslation();
  const continueEditLabel = t("externalEdit.actions.continueEdit");
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
  const [mergePrepareErrors, setMergePrepareErrors] = useState<Record<string, string>>({});
  const [preparedMergeResult, setPreparedMergeResult] = useState<ExternalEditMergePrepareResult | null>(null);
  const [pendingDialogOpen, setPendingDialogOpen] = useState(false);

  const startUpload = useSFTPStore((s) => s.startUpload);
  const startUploadDir = useSFTPStore((s) => s.startUploadDir);
  const startUploadFile = useSFTPStore((s) => s.startUploadFile);
  const startDownload = useSFTPStore((s) => s.startDownload);
  const startDownloadDir = useSFTPStore((s) => s.startDownloadDir);
  const allTransfers = useSFTPStore((s) => s.transfers);
  const allExternalSessions = useExternalEditStore((s) => s.sessions);
  const pendingConflict = useExternalEditStore((s) => s.pendingConflict);
  const dismissCompare = useExternalEditStore((s) => s.dismissCompare);
  const dismissMerge = useExternalEditStore((s) => s.dismissMerge);
  const dismissErrorDetail = useExternalEditStore((s) => s.dismissErrorDetail);
  const compareResult = useExternalEditStore((s) => s.compareResult);
  const mergeResult = useExternalEditStore((s) => s.mergeResult);
  const selectedError = useExternalEditStore((s) => s.selectedError);
  const openErrorDetail = useExternalEditStore((s) => s.openErrorDetail);
  const prepareMerge = useExternalEditStore((s) => s.prepareMerge);
  const resolveConflict = useExternalEditStore((s) => s.resolveConflict);
  const continuePendingSession = useExternalEditStore((s) => s.continuePendingSession);
  const savingSessionId = useExternalEditStore((s) => s.savingSessionId);
  const safePendingConflict = isExternalEditClipboardResidueSession(pendingConflict?.session) ? null : pendingConflict;
  const safeCompareResult = isExternalEditClipboardResidueSession(compareResult?.session) ? null : compareResult;
  const safeStoreMergeResult = isExternalEditClipboardResidueSession(mergeResult?.session) ? null : mergeResult;
  const safePreparedMergeResult = isExternalEditClipboardResidueSession(preparedMergeResult?.session)
    ? null
    : preparedMergeResult;
  const safeMergeResult = safeStoreMergeResult || safePreparedMergeResult;
  const safeSelectedError = isExternalEditClipboardResidueSession(selectedError) ? null : selectedError;

  const transferTarget = useMemo(() => ({ tabId, sessionId }), [tabId, sessionId]);
  const tabTransfers = useMemo(
    () => Object.values(allTransfers).filter((transfer) => transfer.tabId === tabId),
    [allTransfers, tabId]
  );
  const attentionItems = useMemo(
    () => buildExternalEditAttentionItems(allExternalSessions).filter((entry) => entry.session.assetId === assetId),
    [allExternalSessions, assetId]
  );
  const pendingItems = useMemo(() => {
    const items: ExternalEditPendingItem[] = [...attentionItems];
    const pendingSession = safePendingConflict?.session;
    if (pendingSession && pendingSession.assetId === assetId) {
      const runtimeType =
        safePendingConflict?.status === "remote_missing"
          ? "remote_missing"
          : safePendingConflict?.status === "conflict_remote_changed"
            ? "conflict"
            : null;
      if (!runtimeType) {
        return items.filter((item) => !isExternalEditClipboardResidueSession(item.session));
      }
      const decisionType = runtimeType === "remote_missing" ? undefined : runtimeType;
      const exists = items.some((item) => item.session.id === pendingSession.id && item.type === runtimeType);
      if (!exists) {
        items.unshift({
          id: `${runtimeType}:${pendingSession.id}`,
          type: runtimeType,
          session: pendingSession,
          decisionType,
          sourceType: "runtime",
        });
      }
    }
    return items.filter((item) => !isExternalEditClipboardResidueSession(item.session));
  }, [assetId, attentionItems, safePendingConflict]);
  const isDragOver = useNativeFileDrop({
    currentPathRef,
    isActive,
    isOpen,
    panelRef,
    sessionId,
    startUploadFile,
    tabId,
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

  useEffect(() => {
    if (safePendingConflict) {
      setPendingDialogOpen(true);
    }
  }, [safePendingConflict]);

  const doneUploadCount = tabTransfers.filter((transfer) => {
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

  const handlePrepareMerge = useCallback(
    async (session: ExternalEditSession) => {
      setMergePrepareErrors((current) => {
        const { [session.id]: _ignored, ...rest } = current;
        return rest;
      });
      try {
        const result = await prepareMerge(session.id);
        const acceptedResult = useExternalEditStore.getState().mergeResult;
        setPreparedMergeResult(
          acceptedResult?.primaryDraftSessionId === result.primaryDraftSessionId ? acceptedResult : null
        );
        return true;
      } catch (error) {
        const safeMessage = t(EXTERNAL_EDIT_SAFE_ERROR_KEY);
        setError(safeMessage);
        setMergePrepareErrors((current) => ({ ...current, [session.id]: safeMessage }));
        return false;
      }
    },
    [prepareMerge, setError, t]
  );

  const handlePendingMerge = useCallback(
    async (session: ExternalEditSession) => {
      const opened = await handlePrepareMerge(session);
      if (opened) {
        setPendingDialogOpen(false);
      }
    },
    [handlePrepareMerge]
  );

  const handlePendingAcceptRemote = useCallback(
    async (session: ExternalEditSession) => {
      try {
        await resolveConflict(session.id, "reread");
      } catch (error) {
        setError(t(EXTERNAL_EDIT_SAFE_ERROR_KEY));
      }
    },
    [resolveConflict, setError, t]
  );

  const handlePendingOverwrite = useCallback(
    async (session: ExternalEditSession) => {
      try {
        await resolveConflict(session.id, session.state === "remote_missing" ? "recreate" : "overwrite");
      } catch (error) {
        setError(t(EXTERNAL_EDIT_SAFE_ERROR_KEY));
      }
    },
    [resolveConflict, setError, t]
  );

  const handlePendingContinueEdit = useCallback(
    async (session: ExternalEditSession, sourceType?: "runtime" | "recovery") => {
      try {
        await continuePendingSession(session.id, sourceType);
        setPendingDialogOpen((open) => {
          if (!open) return open;
          const latestSessions = useExternalEditStore.getState().sessions;
          const latestPendingConflict = useExternalEditStore.getState().pendingConflict;
          const latestAttentionItems = buildExternalEditAttentionItems(latestSessions).filter(
            (entry) => entry.session.assetId === assetId
          );
          const latestPendingSession = latestPendingConflict?.session;
          const latestRuntimeType =
            latestPendingConflict?.status === "remote_missing"
              ? "remote_missing"
              : latestPendingConflict?.status === "conflict_remote_changed"
                ? "conflict"
                : null;
          const hasRuntimeItem =
            !!latestPendingSession &&
            latestPendingSession.assetId === assetId &&
            latestRuntimeType !== null &&
            !latestAttentionItems.some(
              (item) => item.session.id === latestPendingSession.id && item.type === latestRuntimeType
            );
          return latestAttentionItems.length > 0 || hasRuntimeItem;
        });
      } catch (error) {
        setError(t(EXTERNAL_EDIT_SAFE_ERROR_KEY));
      }
    },
    [assetId, continuePendingSession, setError, t]
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
          if (entry) startDownload(transferTarget, getFullPath(entry));
          break;
        case "externalEdit":
          if (entry) {
            void handleOpenExternalEdit(getFullPath(entry));
          }
          break;
        case "downloadDir":
          if (entry) startDownloadDir(transferTarget, getFullPath(entry));
          break;
        case "upload":
          startUpload(transferTarget, currentPath.endsWith("/") ? currentPath : currentPath + "/");
          break;
        case "uploadDir":
          startUploadDir(transferTarget, currentPath.endsWith("/") ? currentPath : currentPath + "/");
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
      startDownload,
      startDownloadDir,
      startUpload,
      startUploadDir,
      transferTarget,
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

            {pendingItems.length > 0 && (
              <div className="border-b bg-amber-500/5 px-3 py-2">
                <Button
                  className="w-full justify-between"
                  data-testid="external-edit-pending-entry"
                  size="sm"
                  variant="outline"
                  onClick={() => setPendingDialogOpen(true)}
                >
                  <span className="flex items-center gap-2">
                    <AlertTriangle className="h-4 w-4 text-amber-500" />
                    {t("externalEdit.pending.entry")}
                  </span>
                  <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs text-amber-700 dark:text-amber-300">
                    {pendingItems.length}
                  </span>
                </Button>
              </div>
            )}

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

            <TransferSection tabId={tabId} transfers={tabTransfers} />
          </div>
        </div>
      </div>

      {ctxMenu && <FloatingMenu ctx={ctxMenu} onAction={handleCtxAction} onClose={() => setCtxMenu(null)} />}

      <ExternalEditPendingDialog
        open={pendingDialogOpen}
        onOpenChange={setPendingDialogOpen}
        pendingItems={pendingItems}
        savingSessionId={savingSessionId}
        mergePrepareErrors={mergePrepareErrors}
        continueEditLabel={continueEditLabel}
        onOpenErrorDetail={openErrorDetail}
        onMerge={handlePendingMerge}
        onAcceptRemote={handlePendingAcceptRemote}
        onOverwrite={handlePendingOverwrite}
        onContinueEdit={handlePendingContinueEdit}
      />

      {safeCompareResult && (
        <ExternalEditCompareWorkbench compareResult={safeCompareResult} onDismiss={dismissCompare} />
      )}

      {safeMergeResult && (
        <ExternalEditMergeWorkbench
          mergeResult={safeMergeResult}
          savingSessionId={savingSessionId}
          onClose={() => {
            setPreparedMergeResult(null);
            dismissMerge();
          }}
          onError={() => setError(t(EXTERNAL_EDIT_SAFE_ERROR_KEY))}
        />
      )}

      <Dialog open={!!safeSelectedError} onOpenChange={(open) => !open && dismissErrorDetail()}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("externalEdit.error.title")}</DialogTitle>
            <DialogDescription>{safeSelectedError ? `${safeSelectedError.remotePath}` : ""}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm">
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.summaryLabel")}</div>
              <div>{safeSelectedError?.lastError?.summary || ""}</div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.stepLabel")}</div>
              <div>{safeSelectedError?.lastError?.step || ""}</div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">{t("externalEdit.error.suggestionLabel")}</div>
              <div>{safeSelectedError?.lastError?.suggestion || ""}</div>
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
