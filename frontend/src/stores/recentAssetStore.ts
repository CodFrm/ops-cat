import { create } from "zustand";

const STORAGE_KEY = "recent_assets";
const MAX_RECENT = 20;

interface RecentAssetState {
  recentIds: number[];
  touch: (id: number) => void;
  remove: (id: number) => void;
}

function loadRecent(): number[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) && parsed.every((item) => typeof item === "number" && Number.isFinite(item))
      ? parsed
      : [];
  } catch {
    return [];
  }
}

function saveRecent(ids: number[]): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
}

export const useRecentAssetStore = create<RecentAssetState>((set) => ({
  recentIds: loadRecent(),

  touch: (id) => {
    set((state) => {
      const updated = [id, ...state.recentIds.filter((x) => x !== id)].slice(0, MAX_RECENT);
      saveRecent(updated);
      return { recentIds: updated };
    });
  },

  remove: (id) => {
    set((state) => {
      const updated = state.recentIds.filter((x) => x !== id);
      saveRecent(updated);
      return { recentIds: updated };
    });
  },
}));
