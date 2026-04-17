import { create } from "zustand";

export type TabBarLayout = "top" | "left";
export type SidePanel = "assets" | "tabs";

export interface LayoutState {
  tabBarLayout: TabBarLayout;
  leftPanelWidth: number;
  leftPanelVisible: boolean;
  activeSidePanel: SidePanel;
  filterOpen: boolean;

  setLayout: (layout: TabBarLayout) => void;
  setPanelWidth: (w: number) => void;
  toggleVisible: () => void;
  switchPanel: () => void;
  setActivePanel: (p: SidePanel) => void;
  setFilterOpen: (open: boolean) => void;
  /** 打开 filter：侧边模式下顺带激活 tabs 面板并展开 */
  requestOpenFilter: () => void;
}

export const MIN_PANEL_WIDTH = 48;
export const COLLAPSE_THRESHOLD = 100;
export const DEFAULT_PANEL_WIDTH = 220;

export function isCollapsed(s: Pick<LayoutState, "leftPanelWidth">): boolean {
  return s.leftPanelWidth < COLLAPSE_THRESHOLD;
}

function clampWidth(w: number): number {
  const maxW = typeof window !== "undefined" ? Math.floor(window.innerWidth / 2) : 10000;
  return Math.max(MIN_PANEL_WIDTH, Math.min(maxW, Math.round(w)));
}

export const useLayoutStore = create<LayoutState>((set) => ({
  tabBarLayout: "top",
  leftPanelWidth: DEFAULT_PANEL_WIDTH,
  leftPanelVisible: true,
  activeSidePanel: "assets",
  filterOpen: false,

  setLayout: (layout) => set({ tabBarLayout: layout, filterOpen: false }),
  setPanelWidth: (w) => set({ leftPanelWidth: clampWidth(w) }),
  toggleVisible: () => set((s) => ({ leftPanelVisible: !s.leftPanelVisible })),
  switchPanel: () => set((s) => ({ activeSidePanel: s.activeSidePanel === "assets" ? "tabs" : "assets" })),
  setActivePanel: (p) => set({ activeSidePanel: p }),
  setFilterOpen: (open) => set({ filterOpen: open }),
  requestOpenFilter: () =>
    set((s) =>
      s.tabBarLayout === "left"
        ? { filterOpen: true, activeSidePanel: "tabs", leftPanelVisible: true }
        : { filterOpen: true }
    ),
}));

const STORAGE_KEY = "layout_store";

interface SavedLayout {
  tabBarLayout: TabBarLayout;
  leftPanelWidth: number;
  leftPanelVisible: boolean;
  activeSidePanel: SidePanel;
}

let _persistReady = false;

useLayoutStore.subscribe((state, prev) => {
  if (!_persistReady) return;
  if (
    state.tabBarLayout !== prev.tabBarLayout ||
    state.leftPanelWidth !== prev.leftPanelWidth ||
    state.leftPanelVisible !== prev.leftPanelVisible ||
    state.activeSidePanel !== prev.activeSidePanel
  ) {
    const data: SavedLayout = {
      tabBarLayout: state.tabBarLayout,
      leftPanelWidth: state.leftPanelWidth,
      leftPanelVisible: state.leftPanelVisible,
      activeSidePanel: state.activeSidePanel,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  }
});

(function restore() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const data = JSON.parse(raw) as Partial<SavedLayout>;
      useLayoutStore.setState({
        tabBarLayout: data.tabBarLayout ?? "top",
        leftPanelWidth: clampWidth(data.leftPanelWidth ?? DEFAULT_PANEL_WIDTH),
        leftPanelVisible: data.leftPanelVisible ?? true,
        activeSidePanel: data.activeSidePanel ?? "assets",
      });
    }
  } catch {
    /* ignore */
  }
  _persistReady = true;
})();
