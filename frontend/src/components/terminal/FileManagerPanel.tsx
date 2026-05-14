import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, GitMerge, Upload, X } from "lucide-react";
import type * as MonacoNS from "monaco-editor";
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
import { CodeDiffViewer } from "@/components/CodeDiffViewer";
import { CodeEditor } from "@/components/CodeEditor";
import { openExternalEdit, type ExternalEditMergePrepareResult, type ExternalEditSession } from "@/lib/externalEditApi";
import { buildTextDiffBlocks, type TextDiffBlock } from "@/lib/textDiffBlocks";
import {
  buildExternalEditAttentionItems,
  type ExternalEditAttentionItem,
  isExternalEditClipboardResidueSession,
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
  isActive?: boolean;
  isOpen: boolean;
  width: number;
  onWidthChange: (width: number) => void;
}

const EXTERNAL_EDIT_SAFE_ERROR_KEY = "externalEdit.error.safeActionFailed";
const EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS =
  "!h-auto max-w-full justify-start !whitespace-normal break-words px-2 py-1.5 text-left leading-4";

type ExternalEditPendingItem =
  | ExternalEditAttentionItem
  | {
      id: string;
      type: "pending" | "conflict" | "remote_missing";
      session: ExternalEditSession;
      decisionType?: "pending" | "conflict";
      sourceType?: "runtime" | "recovery";
    };

type MergePaneRole = "local" | "final" | "remote";
type MergeEditorRefs = Record<
  MergePaneRole,
  { editor: MonacoNS.editor.IStandaloneCodeEditor | null; monaco: typeof MonacoNS | null }
>;
type MergeDecorationRefs = Record<MergePaneRole, MonacoNS.editor.IEditorDecorationsCollection | null>;

function blockLineRange(block: TextDiffBlock, pane: MergePaneRole) {
  const startLine = pane === "remote" ? block.originalStartLine : block.modifiedStartLine;
  const endLine = pane === "remote" ? block.originalEndLine : block.modifiedEndLine;
  return { startLine, endLine };
}

function mergePaneLineClass(block: TextDiffBlock, pane: MergePaneRole, active: boolean) {
  if (active) return "external-edit-merge-line-current";
  if (pane === "remote") {
    return block.kind === "insert" ? "" : "external-edit-merge-line-remote-change";
  }
  if (pane === "local") {
    return block.kind === "delete" ? "" : "external-edit-merge-line-local-change";
  }
  if (block.kind === "delete") return "external-edit-merge-line-final-local";
  if (block.kind === "insert") return "external-edit-merge-line-final-remote";
  return "external-edit-merge-line-final-combined";
}

function mergePaneGutterClass(block: TextDiffBlock, pane: MergePaneRole, active: boolean) {
  if (active) return "external-edit-merge-gutter-current";
  if (pane === "remote") {
    return block.kind === "insert" ? "" : "external-edit-merge-gutter-remote";
  }
  if (pane === "local") {
    return block.kind === "delete" ? "" : "external-edit-merge-gutter-local";
  }
  if (block.kind === "insert") return "external-edit-merge-gutter-remote";
  if (block.kind === "delete") return "external-edit-merge-gutter-local";
  return "external-edit-merge-gutter-combined";
}

function buildMergePaneDecorations(
  monaco: typeof MonacoNS,
  blocks: TextDiffBlock[],
  activeIndex: number,
  pane: MergePaneRole
): MonacoNS.editor.IModelDeltaDecoration[] {
  return blocks.flatMap((block, index) => {
    const { startLine, endLine } = blockLineRange(block, pane);
    if (endLine < startLine || startLine < 1) return [];
    const active = index === activeIndex;
    const className = mergePaneLineClass(block, pane, active);
    const glyphMarginClassName = mergePaneGutterClass(block, pane, active);
    if (!className && !glyphMarginClassName) return [];
    return [
      {
        range: new monaco.Range(startLine, 1, endLine, 1),
        options: {
          isWholeLine: true,
          className,
          glyphMarginClassName,
          overviewRuler: {
            color: block.kind === "insert" ? "#16a34a" : block.kind === "delete" ? "#dc2626" : "#d97706",
            position: monaco.editor.OverviewRulerLane.Full,
          },
        },
      },
    ];
  });
}

interface ExternalEditIdeaFrameProps {
  actions?: ReactNode;
  children: ReactNode;
  fileName: string;
  helper: string;
  layoutLabel: string;
  mode: "compare" | "merge";
  remotePath: string;
  sidebarLabel: string;
  status: string;
  subtitle?: string;
  testId: string;
  title: string;
}

function ExternalEditIdeaFrame({
  actions,
  children,
  fileName,
  helper,
  layoutLabel,
  mode,
  remotePath,
  sidebarLabel,
  status,
  subtitle,
  testId,
  title,
}: ExternalEditIdeaFrameProps) {
  return (
    <div
      className={cn(
        "fixed z-50 flex overflow-hidden rounded-xl border border-slate-700 bg-[#1f2329] text-slate-100 shadow-2xl",
        mode === "compare" ? "inset-4" : "inset-3"
      )}
      data-idea-workbench={mode}
      data-testid={testId}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div className="flex w-56 shrink-0 flex-col border-r border-slate-700 bg-[#252a31]">
        <div className="border-b border-slate-700 px-4 py-3 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">
          {sidebarLabel}
        </div>
        <div className="flex-1 px-3 py-4">
          <div
            className={cn(
              "rounded-md px-3 py-2 text-sm font-medium",
              mode === "merge"
                ? "border border-amber-500/40 bg-amber-500/10 text-amber-100"
                : "bg-[#343b45] text-slate-100"
            )}
            data-testid={`external-edit-${mode}-idea-file`}
          >
            {fileName}
          </div>
          <div className="mt-3 break-all text-xs leading-5 text-slate-400">{remotePath}</div>
        </div>
        <div className="border-t border-slate-700 px-3 py-3 text-[11px] text-slate-400">{helper}</div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex h-12 items-center justify-between border-b border-slate-700 bg-[#2b3038] px-4">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">{title}</div>
            <div className="truncate text-[11px] text-slate-400">{subtitle || remotePath}</div>
          </div>
          {actions}
        </div>
        {children}
        <div className="flex h-8 items-center justify-between border-t border-slate-700 bg-[#252a31] px-4 text-[11px] text-slate-400">
          <span>{status}</span>
          <span>{layoutLabel}</span>
        </div>
      </div>
    </div>
  );
}

interface ExternalEditIdeaEditorPaneProps {
  badge: string;
  children: ReactNode;
  tone: "local" | "final" | "remote";
  title: string;
}

function ExternalEditIdeaEditorPane({ badge, children, tone, title }: ExternalEditIdeaEditorPaneProps) {
  return (
    <div
      className={cn("flex min-h-0 flex-col bg-[#1f2329]", tone === "final" && "ring-1 ring-amber-400/40")}
      data-idea-pane={tone}
      data-testid={`external-edit-idea-pane-${tone}`}
    >
      <div
        className={cn(
          "flex h-9 items-center justify-between border-b px-3 text-xs",
          tone === "final" ? "border-amber-400/30 bg-[#3a3324]" : "border-slate-700 bg-[#303640]"
        )}
      >
        <span
          className={cn(
            "font-semibold",
            tone === "local" && "text-emerald-200",
            tone === "remote" && "text-sky-200",
            tone === "final" && "text-amber-100"
          )}
        >
          {title}
        </span>
        <span
          className={cn(
            "rounded px-2 py-0.5 text-[10px] uppercase tracking-wide",
            tone === "final" ? "bg-amber-400/20 text-amber-100" : "bg-slate-800 text-slate-300"
          )}
        >
          {badge}
        </span>
      </div>
      {children}
    </div>
  );
}

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
  const continueEditLabel = (() => {
    const label = t("externalEdit.actions.continueEdit");
    return label === "externalEdit.actions.continueEdit" ? "继续修改" : label;
  })();
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
  const [mergeFinalContent, setMergeFinalContent] = useState("");
  const [mergeDirty, setMergeDirty] = useState(false);
  const [confirmCloseMerge, setConfirmCloseMerge] = useState(false);
  const [mergePrepareErrors, setMergePrepareErrors] = useState<Record<string, string>>({});
  const [preparedMergeResult, setPreparedMergeResult] = useState<ExternalEditMergePrepareResult | null>(null);
  const [pendingDialogOpen, setPendingDialogOpen] = useState(false);
  const [compareActiveBlockIndex, setCompareActiveBlockIndex] = useState(0);
  const [compareNavigationToken, setCompareNavigationToken] = useState(0);
  const [compareDiffTotal, setCompareDiffTotal] = useState(0);
  const [mergeActiveBlockIndex, setMergeActiveBlockIndex] = useState(0);
  const [mergeNavigationToken, setMergeNavigationToken] = useState(0);
  const mergeEditorRefs = useRef<MergeEditorRefs>({
    local: { editor: null, monaco: null },
    final: { editor: null, monaco: null },
    remote: { editor: null, monaco: null },
  });
  const mergeDecorationRefs = useRef<MergeDecorationRefs>({ local: null, final: null, remote: null });

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
  const applyMerge = useExternalEditStore((s) => s.applyMerge);
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
  const mergeConflictBlocks = useMemo(
    () =>
      safeMergeResult
        ? buildTextDiffBlocks(safeMergeResult.remoteContent || "", safeMergeResult.localContent || "")
        : [],
    [safeMergeResult?.localContent, safeMergeResult?.remoteContent]
  );
  const mergeConflictTotal = mergeConflictBlocks.length;

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
    if (!safeMergeResult) {
      setMergeFinalContent("");
      setMergeDirty(false);
      setMergeActiveBlockIndex(0);
      return;
    }
    setMergeFinalContent(safeMergeResult.finalContent);
    setMergeDirty(false);
    setMergeActiveBlockIndex(0);
    setMergeNavigationToken((token) => token + 1);
  }, [safeMergeResult]);

  useEffect(() => {
    if (!safeCompareResult) {
      setCompareActiveBlockIndex(0);
      setCompareDiffTotal(0);
      return;
    }
    setCompareActiveBlockIndex(0);
    setCompareNavigationToken((token) => token + 1);
  }, [safeCompareResult]);

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

  const handleApplyMerge = useCallback(async () => {
    if (!safeMergeResult) return;
    try {
      await applyMerge(safeMergeResult.primaryDraftSessionId, mergeFinalContent, safeMergeResult.remoteHash);
      setMergeDirty(false);
      setPreparedMergeResult(null);
    } catch (error) {
      setError(t(EXTERNAL_EDIT_SAFE_ERROR_KEY));
    }
  }, [applyMerge, mergeFinalContent, safeMergeResult, setError, t]);

  const handleMergeOpenChange = useCallback(
    (open: boolean) => {
      if (open) return;
      if (mergeDirty) {
        setConfirmCloseMerge(true);
        return;
      }
      setPreparedMergeResult(null);
      dismissMerge();
    },
    [dismissMerge, mergeDirty]
  );

  const navigateCompareBlock = useCallback(
    (direction: -1 | 1) => {
      if (compareDiffTotal === 0) return;
      setCompareActiveBlockIndex((current) => {
        const next = Math.min(Math.max(current + direction, 0), compareDiffTotal - 1);
        if (next !== current) {
          setCompareNavigationToken((token) => token + 1);
        }
        return next;
      });
    },
    [compareDiffTotal]
  );

  const navigateMergeBlock = useCallback(
    (direction: -1 | 1) => {
      if (mergeConflictTotal === 0) return;
      setMergeActiveBlockIndex((current) => {
        const next = Math.min(Math.max(current + direction, 0), mergeConflictTotal - 1);
        if (next !== current) {
          setMergeNavigationToken((token) => token + 1);
        }
        return next;
      });
    },
    [mergeConflictTotal]
  );

  const handleMergeEditorMount = useCallback(
    (pane: MergePaneRole) => (editor: MonacoNS.editor.IStandaloneCodeEditor, monaco: typeof MonacoNS) => {
      mergeEditorRefs.current[pane] = { editor, monaco };
    },
    []
  );

  useEffect(() => {
    (["local", "final", "remote"] as MergePaneRole[]).forEach((pane) => {
      const { editor, monaco } = mergeEditorRefs.current[pane];
      if (!editor || !monaco) return;
      const decorations = buildMergePaneDecorations(monaco, mergeConflictBlocks, mergeActiveBlockIndex, pane);
      mergeDecorationRefs.current[pane]?.clear();
      mergeDecorationRefs.current[pane] = editor.createDecorationsCollection(decorations);
    });
  }, [mergeActiveBlockIndex, mergeConflictBlocks, mergeFinalContent]);

  useEffect(() => {
    if (mergeConflictBlocks.length === 0) return;
    const block = mergeConflictBlocks[Math.min(Math.max(mergeActiveBlockIndex, 0), mergeConflictBlocks.length - 1)];
    (["local", "final", "remote"] as MergePaneRole[]).forEach((pane) => {
      const editor = mergeEditorRefs.current[pane].editor;
      if (!editor) return;
      const { startLine } = blockLineRange(block, pane);
      const lineNumber = Math.max(1, startLine);
      editor.revealLineInCenter(lineNumber);
      editor.setPosition({ lineNumber, column: 1 });
    });
  }, [mergeActiveBlockIndex, mergeConflictBlocks, mergeNavigationToken]);

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

      <Dialog open={pendingDialogOpen} onOpenChange={setPendingDialogOpen}>
        <DialogContent
          className="max-h-[82vh] max-w-3xl grid-rows-[auto,minmax(0,1fr),auto] gap-0 overflow-hidden p-0"
          data-testid="external-edit-pending-dialog"
        >
          <DialogHeader className="shrink-0 border-b px-6 py-4" data-testid="external-edit-pending-dialog-header">
            <DialogTitle>{t("externalEdit.pending.title")}</DialogTitle>
            <DialogDescription>{t("externalEdit.pending.description")}</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 space-y-3 overflow-y-auto px-6 py-4" data-testid="external-edit-pending-dialog-body">
            {pendingItems.length === 0 ? (
              <div className="rounded border bg-muted/20 px-3 py-4 text-sm text-muted-foreground">
                {t("externalEdit.pending.empty")}
              </div>
            ) : (
              pendingItems.map((item) => {
                const session = item.session;
                const fileName = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
                const isPendingDecision = item.decisionType === "pending";
                const isConflictDecision = item.decisionType === "conflict";
                const isError = item.type === "error";
                const isRemoteMissing = item.type === "remote_missing" || session.state === "remote_missing";
                const cardTone = isConflictDecision
                  ? "border-amber-400/30 bg-amber-500/5"
                  : isPendingDecision
                    ? "border-sky-400/30 bg-sky-500/5"
                    : isError
                      ? "border-rose-400/30 bg-rose-500/5"
                      : "border-amber-400/30 bg-amber-500/5";
                return (
                  <div
                    key={item.id}
                    className={cn("rounded border px-4 py-4 text-sm", cardTone)}
                    data-testid={`external-edit-pending-${item.type}`}
                  >
                    <div className="flex flex-col gap-3">
                      <div className="min-w-0 space-y-1.5" data-testid={`external-edit-pending-content-${session.id}`}>
                        <div
                          className="break-words font-medium text-foreground"
                          data-testid={`external-edit-pending-file-${session.id}`}
                        >
                          {fileName}
                        </div>
                        <div
                          className="break-all whitespace-normal text-xs text-muted-foreground"
                          data-testid={`external-edit-pending-path-${session.id}`}
                        >
                          {session.remotePath}
                        </div>
                        <div
                          className="break-words whitespace-normal text-xs leading-5 text-muted-foreground"
                          data-testid={`external-edit-pending-summary-${session.id}`}
                        >
                          {isConflictDecision && t("externalEdit.conflict.remoteChangedTitle")}
                          {isPendingDecision && t("externalEdit.recovery.summary")}
                          {!isConflictDecision &&
                            !isPendingDecision &&
                            !isError &&
                            isRemoteMissing &&
                            t("externalEdit.conflict.remoteMissingTitle")}
                          {isError && (session.lastError?.summary || t("externalEdit.error.title"))}
                        </div>
                      </div>
                      <div
                        className="flex w-full flex-wrap items-start gap-2"
                        data-testid={`external-edit-pending-actions-${session.id}`}
                      >
                        {isConflictDecision && (
                          <>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id || session.state !== "conflict"}
                              onClick={() => void handlePendingMerge(session)}
                            >
                              <GitMerge className="mr-1 h-3 w-3" />
                              {t("externalEdit.actions.merge")}
                            </Button>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id || session.state !== "conflict"}
                              onClick={() => void handlePendingAcceptRemote(session)}
                            >
                              {t("externalEdit.actions.acceptRemote")}
                            </Button>
                            <Button
                              size="xs"
                              variant="destructive"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id || session.state !== "conflict"}
                              onClick={() => void handlePendingOverwrite(session)}
                            >
                              {t("externalEdit.actions.overwrite")}
                            </Button>
                          </>
                        )}
                        {isPendingDecision && (
                          <>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id}
                              onClick={() => void handlePendingContinueEdit(session, item.sourceType)}
                            >
                              {continueEditLabel}
                            </Button>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id}
                              onClick={() => void handlePendingAcceptRemote(session)}
                            >
                              {t("externalEdit.actions.reread")}
                            </Button>
                            <Button
                              size="xs"
                              variant="destructive"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id}
                              onClick={() => void handlePendingOverwrite(session)}
                            >
                              {t("externalEdit.actions.overwrite")}
                            </Button>
                          </>
                        )}
                        {!isConflictDecision && !isPendingDecision && isRemoteMissing && (
                          <>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              disabled={savingSessionId === session.id}
                              onClick={() => void handlePendingOverwrite(session)}
                            >
                              {t("externalEdit.actions.saveAgain")}
                            </Button>
                          </>
                        )}
                        {isError && (
                          <>
                            <Button
                              size="xs"
                              variant="outline"
                              className={EXTERNAL_EDIT_PENDING_ACTION_BUTTON_CLASS}
                              onClick={() => openErrorDetail(session.id)}
                            >
                              <AlertTriangle className="mr-1 h-3 w-3" />
                              {t("externalEdit.actions.viewError")}
                            </Button>
                          </>
                        )}
                      </div>
                    </div>
                    {mergePrepareErrors[session.id] && (
                      <div className="mt-2 rounded border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs text-destructive">
                        {mergePrepareErrors[session.id]}
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
          <div
            className="flex shrink-0 justify-end border-t px-6 py-3"
            data-testid="external-edit-pending-dialog-footer"
          >
            <Button
              variant="outline"
              onClick={() => {
                setPendingDialogOpen(false);
              }}
            >
              {t("action.close")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {safeCompareResult && (
        <ExternalEditIdeaFrame
          fileName={safeCompareResult.fileName}
          helper={t("externalEdit.compare.helper")}
          layoutLabel={t("externalEdit.compare.remoteLeftLocalRight")}
          mode="compare"
          remotePath={safeCompareResult.remotePath}
          sidebarLabel={t("externalEdit.compare.projectView")}
          status={t("externalEdit.compare.status")}
          testId="external-edit-compare-workbench"
          title={t("externalEdit.compare.title")}
          actions={
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="xs"
                className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
                disabled={compareDiffTotal === 0 || compareActiveBlockIndex === 0}
                onClick={() => navigateCompareBlock(-1)}
              >
                {t("externalEdit.compare.previous")}
              </Button>
              <div
                className="min-w-14 rounded border border-slate-600 bg-slate-800 px-2 py-1 text-center text-xs text-slate-200"
                data-testid="external-edit-compare-diff-count"
              >
                {compareDiffTotal === 0 ? "0 / 0" : `${compareActiveBlockIndex + 1} / ${compareDiffTotal}`}
              </div>
              <Button
                variant="outline"
                size="xs"
                className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
                disabled={compareDiffTotal === 0 || compareActiveBlockIndex >= compareDiffTotal - 1}
                onClick={() => navigateCompareBlock(1)}
              >
                {t("externalEdit.compare.next")}
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="text-slate-300 hover:bg-slate-700 hover:text-white"
                onClick={dismissCompare}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          }
        >
          <div
            className="min-h-0 flex-1 bg-[#1f2329] p-2"
            data-idea-layout="read-only-diff"
            data-testid="external-edit-compare-idea-layout"
          >
            <CodeDiffViewer
              activeBlockIndex={compareActiveBlockIndex}
              badge={t("externalEdit.compare.readOnly")}
              className="border-slate-700 bg-[#f8fafc] text-slate-950 dark:bg-[#1f2329] dark:text-slate-100"
              height="100%"
              language="plaintext"
              modified={safeCompareResult.localContent || ""}
              modifiedTitle={t("externalEdit.compare.localDraft")}
              navigationToken={compareNavigationToken}
              original={safeCompareResult.remoteContent || ""}
              originalTitle={t("externalEdit.compare.remoteSnapshot")}
              testId="external-edit-compare-diff-editor"
              onDiffStatsChange={({ total }) => {
                setCompareDiffTotal(total);
                setCompareActiveBlockIndex((current) => Math.min(current, Math.max(total - 1, 0)));
              }}
            />
          </div>
        </ExternalEditIdeaFrame>
      )}

      {safeMergeResult && (
        <ExternalEditIdeaFrame
          fileName={safeMergeResult.fileName}
          helper={t("externalEdit.merge.helper")}
          layoutLabel={t("externalEdit.merge.localCenterRemote")}
          mode="merge"
          remotePath={safeMergeResult.remotePath}
          sidebarLabel={t("externalEdit.merge.changelist")}
          status={t("externalEdit.merge.status")}
          testId="external-edit-merge-workbench"
          title={t("externalEdit.merge.title")}
          actions={
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="xs"
                className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
                disabled={mergeConflictTotal === 0 || mergeActiveBlockIndex === 0}
                onClick={() => navigateMergeBlock(-1)}
              >
                {t("externalEdit.merge.previous")}
              </Button>
              <div
                className="min-w-14 rounded border border-slate-600 bg-slate-800 px-2 py-1 text-center text-xs text-slate-200"
                data-testid="external-edit-merge-conflict-count"
              >
                {mergeConflictTotal === 0 ? "0 / 0" : `${mergeActiveBlockIndex + 1} / ${mergeConflictTotal}`}
              </div>
              <Button
                variant="outline"
                size="xs"
                className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
                disabled={mergeConflictTotal === 0 || mergeActiveBlockIndex >= mergeConflictTotal - 1}
                onClick={() => navigateMergeBlock(1)}
              >
                {t("externalEdit.merge.next")}
              </Button>
              <Button
                variant="outline"
                className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
                onClick={() => handleMergeOpenChange(false)}
              >
                {t("action.cancel")}
              </Button>
              <Button
                disabled={!safeMergeResult || savingSessionId === safeMergeResult.primaryDraftSessionId}
                onClick={() => void handleApplyMerge()}
              >
                {savingSessionId === safeMergeResult?.primaryDraftSessionId
                  ? t("action.saving")
                  : t("externalEdit.actions.saveMerge")}
              </Button>
            </div>
          }
        >
          <div
            className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)_minmax(0,1fr)] gap-px bg-slate-700"
            data-idea-layout="three-way-merge"
            data-testid="external-edit-merge-idea-layout"
          >
            <ExternalEditIdeaEditorPane
              badge={t("externalEdit.merge.readOnlySide")}
              title={t("externalEdit.merge.localDraft")}
              tone="local"
            >
              <CodeEditor
                className="min-h-0 flex-1 overflow-hidden"
                fontSize={12}
                height="100%"
                language="plaintext"
                options={{
                  lineNumbers: "on",
                  contextmenu: true,
                  glyphMargin: true,
                  minimap: { enabled: false },
                  overviewRulerLanes: 3,
                  readOnly: true,
                }}
                readOnly
                testId="external-edit-merge-local"
                value={safeMergeResult.localContent || ""}
                onMount={handleMergeEditorMount("local")}
              />
            </ExternalEditIdeaEditorPane>
            <ExternalEditIdeaEditorPane
              badge={t("externalEdit.merge.editableCenter")}
              title={t("externalEdit.merge.finalDraft")}
              tone="final"
            >
              <CodeEditor
                className="min-h-0 flex-1 overflow-hidden"
                fontSize={12}
                height="100%"
                language="plaintext"
                options={{
                  lineNumbers: "on",
                  contextmenu: true,
                  glyphMargin: true,
                  minimap: { enabled: false },
                  overviewRulerLanes: 3,
                }}
                testId="external-edit-merge-final"
                value={mergeFinalContent}
                onChange={(value) => {
                  setMergeFinalContent(value);
                  setMergeDirty(true);
                }}
                onMount={handleMergeEditorMount("final")}
              />
            </ExternalEditIdeaEditorPane>
            <ExternalEditIdeaEditorPane
              badge={t("externalEdit.merge.readOnlySide")}
              title={t("externalEdit.merge.remoteDraft")}
              tone="remote"
            >
              <CodeEditor
                className="min-h-0 flex-1 overflow-hidden"
                fontSize={12}
                height="100%"
                language="plaintext"
                options={{
                  lineNumbers: "on",
                  contextmenu: true,
                  glyphMargin: true,
                  minimap: { enabled: false },
                  overviewRulerLanes: 3,
                  readOnly: true,
                }}
                readOnly
                testId="external-edit-merge-remote"
                value={safeMergeResult.remoteContent || ""}
                onMount={handleMergeEditorMount("remote")}
              />
            </ExternalEditIdeaEditorPane>
          </div>
        </ExternalEditIdeaFrame>
      )}

      <ConfirmDialog
        open={confirmCloseMerge}
        onOpenChange={(open) => {
          if (!open) setConfirmCloseMerge(false);
        }}
        title={t("externalEdit.merge.closeDirtyTitle")}
        description={t("externalEdit.merge.closeDirtyDesc")}
        cancelText={t("action.cancel")}
        confirmText={t("externalEdit.merge.closeDirtyConfirm")}
        onConfirm={() => {
          setConfirmCloseMerge(false);
          setMergeDirty(false);
          setPreparedMergeResult(null);
          dismissMerge();
        }}
      />

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
