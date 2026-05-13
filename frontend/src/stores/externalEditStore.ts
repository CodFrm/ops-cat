import { create } from "zustand";
import { toast } from "sonner";
import {
  applyExternalEditMerge,
  compareExternalEditSession,
  deleteExternalEditSession,
  type ExternalEditCompareResult,
  type ExternalEditDeleteResult,
  type ExternalEditEvent,
  type ExternalEditMergePrepareResult,
  type ExternalEditSaveResult,
  type ExternalEditSession,
  listExternalEditSessions,
  prepareExternalEditMerge,
  recoverExternalEditSession,
  refreshExternalEditSession,
  resolveExternalEditConflict,
  saveExternalEditSession,
} from "@/lib/externalEditApi";

export interface ExternalEditDocumentView {
  documentKey: string;
  session: ExternalEditSession;
}

export interface ExternalEditConflictView {
  documentKey: string;
  primaryDraft: ExternalEditSession;
  retainedDraft?: ExternalEditSession;
  activeDraft?: ExternalEditSession;
  latestSnapshot?: ExternalEditSession;
  showRetainedDrafts: boolean;
}

export interface ExternalEditErrorView {
  documentKey: string;
  session: ExternalEditSession;
}

export interface ExternalEditRecoveryView {
  documentKey: string;
  session: ExternalEditSession;
}

export interface ExternalEditAttentionItem {
  id: string;
  type: "conflict" | "error" | "recovery";
  documentKey: string;
  session: ExternalEditSession;
}

const EXTERNAL_EDIT_CLIPBOARD_RESIDUE_MARKERS = ["clipboard-images", "folder/clipboard", "folder\\clipboard"];

interface ExternalEditState {
  sessions: Record<string, ExternalEditSession>;
  loading: boolean;
  savingSessionId: string | null;
  autoSavePhases: Record<string, "pending" | "running">;
  // pendingConflict 只承载“需要用户二次决策”的保存结果，
  // 普通保存成功仍然通过 session 列表和 toast 反馈，避免把所有后端返回都升级成阻塞弹窗。
  pendingConflict: ExternalEditSaveResult | null;
  compareResult: ExternalEditCompareResult | null;
  mergeResult: ExternalEditMergePrepareResult | null;
  selectedError: ExternalEditSession | null;
  selectedRecovery: ExternalEditSession | null;
  fetchSessions: () => Promise<void>;
  saveSession: (sessionId: string) => Promise<ExternalEditSaveResult>;
  refreshSession: (sessionId: string) => Promise<ExternalEditSession>;
  compareSession: (sessionId: string) => Promise<ExternalEditCompareResult>;
  prepareMerge: (sessionId: string) => Promise<ExternalEditMergePrepareResult>;
  applyMerge: (sessionId: string, finalContent: string, remoteHash: string) => Promise<ExternalEditSaveResult>;
  recoverSession: (sessionId: string) => Promise<ExternalEditSession>;
  deleteSession: (sessionId: string, removeLocal: boolean) => Promise<ExternalEditDeleteResult>;
  resolveConflict: (
    sessionId: string,
    resolution: "overwrite" | "recreate" | "reread"
  ) => Promise<ExternalEditSaveResult>;
  dismissConflict: () => void;
  dismissCompare: () => void;
  dismissMerge: () => void;
  openErrorDetail: (sessionId: string) => void;
  dismissErrorDetail: () => void;
  openRecoveryDetail: (sessionId: string) => void;
  dismissRecoveryDetail: () => void;
  applyEvent: (event: ExternalEditEvent) => void;
}

export function buildExternalEditDocuments(sessions: Record<string, ExternalEditSession>): ExternalEditDocumentView[] {
  const grouped = new Map<string, ExternalEditSession[]>();
  for (const session of Object.values(sessions)) {
    if (isExternalEditClipboardResidueSession(session)) continue;
    if (session.hidden || session.recordState === "completed" || session.recordState === "abandoned") continue;
    if (session.recordState === "error") continue;
    if (!session.documentKey) continue;
    const current = grouped.get(session.documentKey) || [];
    current.push(session);
    grouped.set(session.documentKey, current);
  }

  const byDocument = new Map<string, ExternalEditSession>();
  for (const [documentKey, relatedSessions] of grouped.entries()) {
    const rereadDraft = relatedSessions
      .filter((session) => session.sourceSessionId && session.state !== "stale" && session.recordState !== "error")
      .sort(compareDocumentSession)[0];
    if (rereadDraft) {
      byDocument.set(documentKey, rereadDraft);
      continue;
    }

    const current = relatedSessions.sort(compareDocumentSession)[0];
    if (current) {
      byDocument.set(documentKey, current);
    }
  }
  return Array.from(byDocument.entries())
    .map(([documentKey, session]) => ({ documentKey, session }))
    .sort((left, right) => right.session.updatedAt - left.session.updatedAt);
}

export function buildExternalEditConflicts(sessions: Record<string, ExternalEditSession>): ExternalEditConflictView[] {
  const grouped = new Map<string, ExternalEditSession[]>();
  for (const session of Object.values(sessions)) {
    if (isExternalEditClipboardResidueSession(session)) continue;
    if (!session.documentKey) continue;
    if (session.hidden) continue;
    const current = grouped.get(session.documentKey) || [];
    current.push(session);
    grouped.set(session.documentKey, current);
  }

  const conflicts: ExternalEditConflictView[] = [];
  for (const [documentKey, relatedSessions] of grouped.entries()) {
    const retainedDraft = relatedSessions
      .filter((session) => session.state === "stale")
      .sort(compareDocumentSession)[0];
    const livePrimaryDraft = relatedSessions
      .filter((session) => session.state === "conflict" || session.state === "remote_missing")
      .sort(compareDocumentSession)[0];
    const activeDraft =
      (retainedDraft?.supersededBySessionId
        ? relatedSessions.find((session) => session.id === retainedDraft.supersededBySessionId)
        : undefined) ||
      relatedSessions
        .filter((session) => session.sourceSessionId && session.state !== "stale" && session.recordState !== "error")
        .sort(compareDocumentSession)[0];
    const primaryDraft = livePrimaryDraft;
    if (!primaryDraft) continue;

    conflicts.push({
      documentKey,
      primaryDraft,
      retainedDraft,
      activeDraft,
      latestSnapshot: activeDraft,
      showRetainedDrafts: true,
    });
  }
  return conflicts.sort((left, right) => right.primaryDraft.updatedAt - left.primaryDraft.updatedAt);
}

export function buildExternalEditErrors(sessions: Record<string, ExternalEditSession>): ExternalEditErrorView[] {
  return Object.values(sessions)
    .filter(
      (session) =>
        !isExternalEditClipboardResidueSession(session) &&
        !session.hidden &&
        session.recordState === "error" &&
        session.lastError
    )
    .sort((left, right) => right.updatedAt - left.updatedAt)
    .map((session) => ({ documentKey: session.documentKey, session }));
}

function compareDocumentSession(left: ExternalEditSession, right: ExternalEditSession): number {
  const rank = (session: ExternalEditSession) => {
    switch (session.state) {
      case "dirty":
        return 0;
      case "conflict":
      case "remote_missing":
        return 1;
      case "error":
        return 2;
      case "clean":
        return 3;
      case "expired":
        return 4;
      case "stale":
        return 5;
      default:
        return 6;
    }
  };
  const rankDiff = rank(left) - rank(right);
  if (rankDiff !== 0) {
    return rankDiff;
  }
  return right.updatedAt - left.updatedAt;
}

function upsertSession(state: ExternalEditState, session?: ExternalEditSession): Record<string, ExternalEditSession> {
  if (!session) {
    return state.sessions;
  }
  if (isExternalEditClipboardResidueSession(session)) {
    const next = { ...state.sessions };
    delete next[session.id];
    return next;
  }
  return {
    ...state.sessions,
    [session.id]: session,
  };
}

export function buildExternalEditRecoveries(sessions: Record<string, ExternalEditSession>): ExternalEditRecoveryView[] {
  return visiblePrimarySessionsByDocument(sessions)
    .filter(
      (session) =>
        !!session.resumeRequired &&
        session.state !== "conflict" &&
        session.state !== "remote_missing" &&
        session.recordState !== "error"
    )
    .map((session) => ({ documentKey: session.documentKey, session }));
}

export function isExternalEditClipboardResidueSession(session?: ExternalEditSession | null): boolean {
  if (!session) return false;
  return [
    session.documentKey,
    session.remotePath,
    session.remoteRealPath,
    session.localPath,
    session.workspaceDir,
  ].some(isExternalEditClipboardResidueText);
}

function isExternalEditClipboardResidueText(value?: string): boolean {
  const normalized = (value || "").trim().replace(/\\/g, "/").toLowerCase();
  if (!normalized) return false;
  return EXTERNAL_EDIT_CLIPBOARD_RESIDUE_MARKERS.some((marker) =>
    normalized.includes(marker.replace(/\\/g, "/").toLowerCase())
  );
}

function isExternalEditClipboardResidueSaveResult(result?: ExternalEditSaveResult | null): boolean {
  if (!result) return false;
  return (
    isExternalEditClipboardResidueSession(result.session) ||
    isExternalEditClipboardResidueText(result.conflict?.documentKey) ||
    isExternalEditClipboardResidueText(result.conflict?.primaryDraftSessionId) ||
    isExternalEditClipboardResidueText(result.conflict?.latestSnapshotSessionId)
  );
}

function isExternalEditClipboardResidueCompareResult(result?: ExternalEditCompareResult | null): boolean {
  if (!result) return false;
  return (
    isExternalEditClipboardResidueSession(result.session) ||
    isExternalEditClipboardResidueText(result.documentKey) ||
    isExternalEditClipboardResidueText(result.remotePath) ||
    isExternalEditClipboardResidueText(result.conflict?.documentKey)
  );
}

function isExternalEditClipboardResidueMergeResult(result?: ExternalEditMergePrepareResult | null): boolean {
  if (!result) return false;
  return (
    isExternalEditClipboardResidueSession(result.session) ||
    isExternalEditClipboardResidueText(result.documentKey) ||
    isExternalEditClipboardResidueText(result.remotePath)
  );
}

function scrubExternalEditRuntimeState(
  state: Pick<
    ExternalEditState,
    "pendingConflict" | "compareResult" | "mergeResult" | "selectedError" | "selectedRecovery" | "autoSavePhases"
  >,
  residueSession?: ExternalEditSession | null
) {
  const residueDocumentKey = residueSession?.documentKey;
  const nextPhases = { ...state.autoSavePhases };
  if (residueDocumentKey) {
    delete nextPhases[residueDocumentKey];
  }
  return {
    autoSavePhases: nextPhases,
    pendingConflict:
      isExternalEditClipboardResidueSaveResult(state.pendingConflict) ||
      (residueSession?.id && state.pendingConflict?.session?.id === residueSession.id)
        ? null
        : state.pendingConflict,
    compareResult:
      isExternalEditClipboardResidueCompareResult(state.compareResult) ||
      (residueSession?.id && state.compareResult?.session?.id === residueSession.id)
        ? null
        : state.compareResult,
    mergeResult:
      isExternalEditClipboardResidueMergeResult(state.mergeResult) ||
      (residueSession?.id && state.mergeResult?.session?.id === residueSession.id)
        ? null
        : state.mergeResult,
    selectedError:
      isExternalEditClipboardResidueSession(state.selectedError) ||
      (residueSession?.id && state.selectedError?.id === residueSession.id)
        ? null
        : state.selectedError,
    selectedRecovery:
      isExternalEditClipboardResidueSession(state.selectedRecovery) ||
      (residueSession?.id && state.selectedRecovery?.id === residueSession.id)
        ? null
        : state.selectedRecovery,
  };
}

export function buildExternalEditAttentionItems(
  sessions: Record<string, ExternalEditSession>
): ExternalEditAttentionItem[] {
  return visiblePrimarySessionsByDocument(sessions)
    .flatMap((session): ExternalEditAttentionItem[] => {
      if (session.state === "conflict" || session.state === "remote_missing") {
        return [{ id: `conflict:${session.documentKey}`, type: "conflict", documentKey: session.documentKey, session }];
      }
      if (session.recordState === "error" && session.lastError) {
        return [{ id: `error:${session.documentKey}`, type: "error", documentKey: session.documentKey, session }];
      }
      if (session.resumeRequired) {
        return [{ id: `recovery:${session.documentKey}`, type: "recovery", documentKey: session.documentKey, session }];
      }
      return [];
    })
    .sort((left, right) => right.session.updatedAt - left.session.updatedAt);
}

function visiblePrimarySessionsByDocument(sessions: Record<string, ExternalEditSession>): ExternalEditSession[] {
  const grouped = new Map<string, ExternalEditSession[]>();
  for (const session of Object.values(sessions)) {
    if (isExternalEditClipboardResidueSession(session)) continue;
    if (!session.documentKey || session.hidden) continue;
    if (session.recordState === "completed" || session.recordState === "abandoned") continue;
    const current = grouped.get(session.documentKey) || [];
    current.push(session);
    grouped.set(session.documentKey, current);
  }
  return Array.from(grouped.values())
    .map((family) => family.sort(compareDocumentFamilyPriority)[0])
    .filter(Boolean)
    .sort((left, right) => right.updatedAt - left.updatedAt);
}

function compareDocumentFamilyPriority(left: ExternalEditSession, right: ExternalEditSession): number {
  const rank = (session: ExternalEditSession) => {
    if (session.state === "conflict" || session.state === "remote_missing") return 0;
    if (session.recordState === "error") return 1;
    if (session.resumeRequired) return 2;
    return 3;
  };
  const rankDiff = rank(left) - rank(right);
  return rankDiff !== 0 ? rankDiff : right.updatedAt - left.updatedAt;
}

function compareRemoteMissingResultToSaveResult(result: ExternalEditCompareResult): ExternalEditSaveResult {
  return {
    status: "remote_missing",
    message: result.message,
    session: result.session,
    conflict: result.conflict,
    automatic: false,
  };
}

function sessionToRefreshConflictResult(session: ExternalEditSession): ExternalEditSaveResult {
  const remoteMissing = session.state === "remote_missing";
  return {
    status: remoteMissing ? "remote_missing" : "conflict_remote_changed",
    message: remoteMissing
      ? "远程文件不存在，请先确认是否需要重新创建远程文件"
      : "远程文件已有新版本，请先比对差异，再决定重新读取或强制覆盖",
    session,
    conflict: {
      documentKey: session.documentKey,
      primaryDraftSessionId: session.id,
    },
    automatic: false,
  };
}

export const useExternalEditStore = create<ExternalEditState>((set) => ({
  sessions: {},
  loading: false,
  savingSessionId: null,
  autoSavePhases: {},
  pendingConflict: null,
  compareResult: null,
  mergeResult: null,
  selectedError: null,
  selectedRecovery: null,

  fetchSessions: async () => {
    set({ loading: true });
    try {
      const sessions = await listExternalEditSessions();
      const next: Record<string, ExternalEditSession> = {};
      for (const session of sessions || []) {
        if (isExternalEditClipboardResidueSession(session)) continue;
        next[session.id] = session;
      }
      set((state) => ({ sessions: next, ...scrubExternalEditRuntimeState(state) }));
    } finally {
      set({ loading: false });
    }
  },

  saveSession: async (sessionId) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await saveExternalEditSession(sessionId);
      set((state) => ({
        sessions: upsertSession(state, result.session),
        ...scrubExternalEditRuntimeState(state, result.session),
        pendingConflict:
          !isExternalEditClipboardResidueSaveResult(result) &&
          (result.status === "conflict_remote_changed" || result.status === "remote_missing")
            ? result
            : null,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  refreshSession: async (sessionId) => {
    set({ savingSessionId: sessionId });
    try {
      const session = await refreshExternalEditSession(sessionId);
      set((state) => ({
        sessions: upsertSession(state, session),
        ...scrubExternalEditRuntimeState(state, session),
        pendingConflict:
          !isExternalEditClipboardResidueSession(session) &&
          (session.state === "conflict" || session.state === "remote_missing")
            ? sessionToRefreshConflictResult(session)
            : state.pendingConflict,
      }));
      return session;
    } finally {
      set({ savingSessionId: null });
    }
  },

  compareSession: async (sessionId) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await compareExternalEditSession(sessionId);
      set((state) => ({
        sessions: upsertSession(state, result.session),
        ...scrubExternalEditRuntimeState(state, result.session),
        compareResult:
          result.status === "remote_missing" || isExternalEditClipboardResidueCompareResult(result) ? null : result,
        pendingConflict:
          result.status === "remote_missing" && !isExternalEditClipboardResidueCompareResult(result)
            ? compareRemoteMissingResultToSaveResult(result)
            : state.pendingConflict,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  prepareMerge: async (sessionId) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await prepareExternalEditMerge(sessionId);
      set((state) => ({
        sessions: upsertSession(state, result.session),
        ...scrubExternalEditRuntimeState(state, result.session),
        mergeResult: isExternalEditClipboardResidueMergeResult(result) ? null : result,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  applyMerge: async (sessionId, finalContent, remoteHash) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await applyExternalEditMerge({ sessionId, finalContent, remoteHash });
      set((state) => ({
        sessions: upsertSession(state, result.session),
        ...scrubExternalEditRuntimeState(state, result.session),
        mergeResult:
          result.status === "saved" || isExternalEditClipboardResidueSaveResult(result) ? null : state.mergeResult,
        pendingConflict:
          !isExternalEditClipboardResidueSaveResult(result) &&
          (result.status === "conflict_remote_changed" || result.status === "remote_missing")
            ? result
            : state.pendingConflict,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  recoverSession: async (sessionId) => {
    set({ savingSessionId: sessionId });
    try {
      const session = await recoverExternalEditSession(sessionId);
      set((state) => ({
        sessions: upsertSession(state, session),
        ...scrubExternalEditRuntimeState(state, session),
        selectedRecovery: isExternalEditClipboardResidueSession(session) ? null : session,
      }));
      return session;
    } finally {
      set({ savingSessionId: null });
    }
  },

  deleteSession: async (sessionId, removeLocal) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await deleteExternalEditSession(sessionId, removeLocal);
      set((state) => {
        const next = { ...state.sessions };
        const resultDocumentKey = result.session?.documentKey || state.sessions[sessionId]?.documentKey;
        if (removeLocal && resultDocumentKey) {
          for (const [id, session] of Object.entries(next)) {
            if (session.documentKey === resultDocumentKey) {
              delete next[id];
            }
          }
        } else if (removeLocal || result.session?.id === sessionId) {
          if (removeLocal) {
            delete next[sessionId];
          } else if (result.session) {
            next[result.session.id] = result.session;
          }
        }
        const scrubbed = scrubExternalEditRuntimeState(state, result.session);
        return {
          sessions: next,
          ...scrubbed,
          selectedError: state.selectedError?.id === sessionId ? null : scrubbed.selectedError,
          selectedRecovery: state.selectedRecovery?.id === sessionId ? null : scrubbed.selectedRecovery,
        };
      });
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  resolveConflict: async (sessionId, resolution) => {
    set({ savingSessionId: sessionId });
    try {
      const result = await resolveExternalEditConflict(sessionId, resolution);
      set((state) => ({
        sessions: upsertSession(state, result.session),
        ...scrubExternalEditRuntimeState(state, result.session),
        pendingConflict:
          !isExternalEditClipboardResidueSaveResult(result) &&
          (result.status === "conflict_remote_changed" || result.status === "remote_missing")
            ? result
            : null,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  dismissConflict: () => set({ pendingConflict: null }),
  dismissCompare: () => set({ compareResult: null }),
  dismissMerge: () => set({ mergeResult: null }),
  openErrorDetail: (sessionId) =>
    set((state) => ({
      selectedError: isExternalEditClipboardResidueSession(state.sessions[sessionId])
        ? null
        : state.sessions[sessionId] || null,
    })),
  dismissErrorDetail: () => set({ selectedError: null }),
  openRecoveryDetail: (sessionId) =>
    set((state) => ({
      selectedRecovery: isExternalEditClipboardResidueSession(state.sessions[sessionId])
        ? null
        : state.sessions[sessionId] || null,
    })),
  dismissRecoveryDetail: () => set({ selectedRecovery: null }),

  applyEvent: (event) => {
    // 前端把 external-edit:event 当成后端状态机的单一事实来源：
    // 会话面板、冲突弹窗、toast 都从这里派生，避免多个组件各自猜测保存结果。
    switch (event.type) {
      case "session_opened":
      case "session_restored":
      case "session_changed":
      case "session_saved":
      case "session_conflict":
        set((state) => {
          const scrubbed = scrubExternalEditRuntimeState(state, event.session);
          if (
            isExternalEditClipboardResidueSession(event.session) ||
            isExternalEditClipboardResidueSaveResult(event.saveResult)
          ) {
            return {
              sessions: upsertSession(state, event.session),
              ...scrubbed,
            };
          }
          const nextPhases =
            event.session?.documentKey && scrubbed.autoSavePhases[event.session.documentKey]
              ? (() => {
                  const next = { ...scrubbed.autoSavePhases };
                  delete next[event.session.documentKey];
                  return next;
                })()
              : scrubbed.autoSavePhases;
          return {
            sessions: upsertSession(state, event.session),
            ...scrubbed,
            autoSavePhases: nextPhases,
            pendingConflict:
              event.type === "session_conflict"
                ? event.saveResult || scrubbed.pendingConflict
                : event.type === "session_saved"
                  ? null
                  : scrubbed.pendingConflict,
            selectedError:
              event.session && scrubbed.selectedError?.id === event.session.id ? event.session : scrubbed.selectedError,
            selectedRecovery:
              event.session && scrubbed.selectedRecovery?.id === event.session.id
                ? event.session
                : scrubbed.selectedRecovery,
          };
        });
        break;
      case "session_auto_save": {
        const documentKey = event.autoSave?.documentKey;
        if (!documentKey) return;
        if (isExternalEditClipboardResidueText(documentKey)) {
          set((state) => {
            const next = { ...state.autoSavePhases };
            delete next[documentKey];
            return { autoSavePhases: next };
          });
          return;
        }
        set((state) => {
          const next = { ...state.autoSavePhases };
          if (event.autoSave?.phase === "pending" || event.autoSave?.phase === "running") {
            next[documentKey] = event.autoSave.phase;
          } else {
            delete next[documentKey];
          }
          return { autoSavePhases: next };
        });
        break;
      }
      case "session_cleaned": {
        if (!event.session?.id) return;
        const sessionId = event.session.id;
        set((state) => {
          const next = { ...state.sessions };
          delete next[sessionId];
          const nextPhases = { ...state.autoSavePhases };
          if (event.session?.documentKey) {
            delete nextPhases[event.session.documentKey];
          }
          return {
            sessions: next,
            ...scrubExternalEditRuntimeState({ ...state, autoSavePhases: nextPhases }, event.session),
          };
        });
        break;
      }
      default:
        break;
    }

    if (event.type === "session_saved" && event.saveResult?.message) {
      toast.success(event.saveResult.message);
    }
  },
}));
