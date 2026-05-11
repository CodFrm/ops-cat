import { create } from "zustand";
import { toast } from "sonner";
import {
  compareExternalEditSession,
  deleteExternalEditSession,
  type ExternalEditCompareResult,
  type ExternalEditDeleteResult,
  type ExternalEditEvent,
  type ExternalEditSaveResult,
  type ExternalEditSession,
  listExternalEditSessions,
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

interface ExternalEditState {
  sessions: Record<string, ExternalEditSession>;
  loading: boolean;
  savingSessionId: string | null;
  autoSavePhases: Record<string, "pending" | "running">;
  // pendingConflict 只承载“需要用户二次决策”的保存结果，
  // 普通保存成功仍然通过 session 列表和 toast 反馈，避免把所有后端返回都升级成阻塞弹窗。
  pendingConflict: ExternalEditSaveResult | null;
  compareResult: ExternalEditCompareResult | null;
  selectedError: ExternalEditSession | null;
  fetchSessions: () => Promise<void>;
  saveSession: (sessionId: string) => Promise<ExternalEditSaveResult>;
  refreshSession: (sessionId: string) => Promise<ExternalEditSession>;
  compareSession: (sessionId: string) => Promise<ExternalEditCompareResult>;
  deleteSession: (sessionId: string, removeLocal: boolean) => Promise<ExternalEditDeleteResult>;
  resolveConflict: (
    sessionId: string,
    resolution: "overwrite" | "recreate" | "reread"
  ) => Promise<ExternalEditSaveResult>;
  dismissConflict: () => void;
  dismissCompare: () => void;
  openErrorDetail: (sessionId: string) => void;
  dismissErrorDetail: () => void;
  applyEvent: (event: ExternalEditEvent) => void;
}

export function buildExternalEditDocuments(sessions: Record<string, ExternalEditSession>): ExternalEditDocumentView[] {
  const grouped = new Map<string, ExternalEditSession[]>();
  for (const session of Object.values(sessions)) {
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
        .filter(
          (session) =>
            session.sourceSessionId &&
            session.state !== "stale" &&
            session.recordState !== "error"
        )
        .sort(compareDocumentSession)[0];
    if (activeDraft) continue;
    const primaryDraft = activeDraft || livePrimaryDraft || retainedDraft;
    if (!primaryDraft) continue;

    conflicts.push({
      documentKey,
      primaryDraft,
      retainedDraft,
      activeDraft: undefined,
      latestSnapshot: undefined,
      showRetainedDrafts: true,
    });
  }
  return conflicts.sort((left, right) => right.primaryDraft.updatedAt - left.primaryDraft.updatedAt);
}

export function buildExternalEditErrors(sessions: Record<string, ExternalEditSession>): ExternalEditErrorView[] {
  return Object.values(sessions)
    .filter((session) => !session.hidden && session.recordState === "error" && session.lastError)
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
  return {
    ...state.sessions,
    [session.id]: session,
  };
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

export const useExternalEditStore = create<ExternalEditState>((set) => ({
  sessions: {},
  loading: false,
  savingSessionId: null,
  autoSavePhases: {},
  pendingConflict: null,
  compareResult: null,
  selectedError: null,

  fetchSessions: async () => {
    set({ loading: true });
    try {
      const sessions = await listExternalEditSessions();
      const next: Record<string, ExternalEditSession> = {};
      for (const session of sessions || []) {
        next[session.id] = session;
      }
      set({ sessions: next });
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
        pendingConflict:
          result.status === "conflict_remote_changed" || result.status === "remote_missing" ? result : null,
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
        compareResult: result.status === "remote_missing" ? null : result,
        pendingConflict:
          result.status === "remote_missing" ? compareRemoteMissingResultToSaveResult(result) : state.pendingConflict,
      }));
      return result;
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
        if (removeLocal || result.session?.id === sessionId) {
          if (removeLocal) {
            delete next[sessionId];
          } else if (result.session) {
            next[result.session.id] = result.session;
          }
        }
        return {
          sessions: next,
          selectedError: state.selectedError?.id === sessionId ? null : state.selectedError,
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
        pendingConflict:
          result.status === "conflict_remote_changed" || result.status === "remote_missing" ? result : null,
      }));
      return result;
    } finally {
      set({ savingSessionId: null });
    }
  },

  dismissConflict: () => set({ pendingConflict: null }),
  dismissCompare: () => set({ compareResult: null }),
  openErrorDetail: (sessionId) =>
    set((state) => ({
      selectedError: state.sessions[sessionId] || null,
    })),
  dismissErrorDetail: () => set({ selectedError: null }),

  applyEvent: (event) => {
    // 前端把 external-edit:event 当成后端状态机的单一事实来源：
    // 会话面板、冲突弹窗、toast 都从这里派生，避免多个组件各自猜测保存结果。
    switch (event.type) {
      case "session_opened":
      case "session_restored":
      case "session_changed":
      case "session_saved":
      case "session_conflict":
        set((state) => ({
          sessions: upsertSession(state, event.session),
          autoSavePhases:
            event.session?.documentKey && state.autoSavePhases[event.session.documentKey]
              ? (() => {
                  const next = { ...state.autoSavePhases };
                  delete next[event.session.documentKey];
                  return next;
                })()
              : state.autoSavePhases,
          pendingConflict:
            event.type === "session_conflict"
              ? event.saveResult || state.pendingConflict
              : event.type === "session_saved"
                ? null
                : state.pendingConflict,
          selectedError:
            event.session && state.selectedError?.id === event.session.id ? event.session : state.selectedError,
        }));
        break;
      case "session_auto_save": {
        const documentKey = event.autoSave?.documentKey;
        if (!documentKey) return;
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
            autoSavePhases: nextPhases,
            selectedError: state.selectedError?.id === sessionId ? null : state.selectedError,
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
