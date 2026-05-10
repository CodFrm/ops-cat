import { create } from "zustand";
import { toast } from "sonner";
import {
  compareExternalEditSession,
  type ExternalEditCompareResult,
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
  latestSnapshot?: ExternalEditSession;
}

interface ExternalEditState {
  sessions: Record<string, ExternalEditSession>;
  loading: boolean;
  savingSessionId: string | null;
  // pendingConflict 只承载“需要用户二次决策”的保存结果，
  // 普通保存成功仍然通过 session 列表和 toast 反馈，避免把所有后端返回都升级成阻塞弹窗。
  pendingConflict: ExternalEditSaveResult | null;
  compareResult: ExternalEditCompareResult | null;
  fetchSessions: () => Promise<void>;
  saveSession: (sessionId: string) => Promise<ExternalEditSaveResult>;
  refreshSession: (sessionId: string) => Promise<ExternalEditSession>;
  compareSession: (sessionId: string) => Promise<ExternalEditCompareResult>;
  resolveConflict: (
    sessionId: string,
    resolution: "overwrite" | "recreate" | "reread"
  ) => Promise<ExternalEditSaveResult>;
  dismissConflict: () => void;
  dismissCompare: () => void;
  applyEvent: (event: ExternalEditEvent) => void;
}

export function buildExternalEditDocuments(sessions: Record<string, ExternalEditSession>): ExternalEditDocumentView[] {
  const byDocument = new Map<string, ExternalEditSession>();
  for (const session of Object.values(sessions)) {
    const current = byDocument.get(session.documentKey);
    if (!current || compareDocumentSession(session, current) < 0) {
      byDocument.set(session.documentKey, session);
    }
  }
  return Array.from(byDocument.entries())
    .map(([documentKey, session]) => ({ documentKey, session }))
    .sort((left, right) => right.session.updatedAt - left.session.updatedAt);
}

export function buildExternalEditConflicts(
  sessions: Record<string, ExternalEditSession>
): ExternalEditConflictView[] {
  const grouped = new Map<string, ExternalEditSession[]>();
  for (const session of Object.values(sessions)) {
    if (!session.documentKey) continue;
    if (session.state !== "conflict" && session.state !== "remote_missing" && session.state !== "stale") continue;
    const current = grouped.get(session.documentKey) || [];
    current.push(session);
    grouped.set(session.documentKey, current);
  }

  const conflicts: ExternalEditConflictView[] = [];
  for (const [documentKey, relatedSessions] of grouped.entries()) {
      const primaryDraft =
        relatedSessions
          .filter((session) => session.state === "conflict" || session.state === "remote_missing")
          .sort(compareDocumentSession)[0] ||
        relatedSessions.filter((session) => session.state === "stale").sort(compareDocumentSession)[0];
      if (!primaryDraft) continue;
      const latestSnapshot = Object.values(sessions)
        .filter((session) => session.documentKey === documentKey && session.sourceSessionId === primaryDraft.id && session.state === "clean")
        .sort(compareDocumentSession)[0];
      conflicts.push({
        documentKey,
        primaryDraft,
        latestSnapshot,
      });
    }
  return conflicts.sort((left, right) => right.primaryDraft.updatedAt - left.primaryDraft.updatedAt);
}

function compareDocumentSession(left: ExternalEditSession, right: ExternalEditSession): number {
  const rank = (session: ExternalEditSession) => {
    switch (session.state) {
      case "dirty":
        return 0;
      case "conflict":
      case "remote_missing":
        return 1;
      case "clean":
        return 2;
      case "expired":
        return 3;
      case "stale":
        return 4;
      default:
        return 5;
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

export const useExternalEditStore = create<ExternalEditState>((set) => ({
  sessions: {},
  loading: false,
  savingSessionId: null,
  pendingConflict: null,
  compareResult: null,

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
      set({ compareResult: result });
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
          pendingConflict:
            event.type === "session_conflict"
              ? event.saveResult || state.pendingConflict
              : event.type === "session_saved"
                ? null
                : state.pendingConflict,
        }));
        break;
      case "session_cleaned": {
        if (!event.session?.id) return;
        const sessionId = event.session.id;
        set((state) => {
          const next = { ...state.sessions };
          delete next[sessionId];
          return { sessions: next };
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
