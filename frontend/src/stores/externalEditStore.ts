import { create } from "zustand";
import { toast } from "sonner";
import {
  type ExternalEditEvent,
  type ExternalEditSaveResult,
  type ExternalEditSession,
  listExternalEditSessions,
  resolveExternalEditConflict,
  saveExternalEditSession,
} from "@/lib/externalEditApi";

interface ExternalEditState {
  sessions: Record<string, ExternalEditSession>;
  loading: boolean;
  savingSessionId: string | null;
  // pendingConflict 只承载“需要用户二次决策”的保存结果，
  // 普通保存成功仍然通过 session 列表和 toast 反馈，避免把所有后端返回都升级成阻塞弹窗。
  pendingConflict: ExternalEditSaveResult | null;
  fetchSessions: () => Promise<void>;
  saveSession: (sessionId: string) => Promise<ExternalEditSaveResult>;
  resolveConflict: (
    sessionId: string,
    resolution: "overwrite" | "recreate" | "reread"
  ) => Promise<ExternalEditSaveResult>;
  dismissConflict: () => void;
  applyEvent: (event: ExternalEditEvent) => void;
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
            event.type === "session_conflict" && event.saveResult ? event.saveResult : state.pendingConflict,
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
