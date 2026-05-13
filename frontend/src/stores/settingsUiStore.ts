import { create } from "zustand";

export type SettingsTab =
  | "ai"
  | "import"
  | "backup"
  | "shortcuts"
  | "terminal"
  | "appearance"
  | "status"
  | "extensions"
  | "about";

interface SettingsUiState {
  activeTab: SettingsTab;
  setActiveTab: (tab: SettingsTab) => void;
}

export const useSettingsUiStore = create<SettingsUiState>((set) => ({
  activeTab: "ai",
  setActiveTab: (activeTab) => set({ activeTab }),
}));
