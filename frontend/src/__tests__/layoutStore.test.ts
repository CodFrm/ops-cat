import { describe, it, expect, beforeEach } from "vitest";
import { useLayoutStore, isCollapsed } from "../stores/layoutStore";

describe("layoutStore", () => {
  beforeEach(() => {
    localStorage.clear();
    useLayoutStore.setState({
      tabBarLayout: "top",
      leftPanelWidth: 220,
      leftPanelVisible: true,
      activeSidePanel: "assets",
      filterOpen: false,
    });
  });

  describe("defaults", () => {
    it("has top layout by default", () => {
      expect(useLayoutStore.getState().tabBarLayout).toBe("top");
    });
    it("has 220 default width", () => {
      expect(useLayoutStore.getState().leftPanelWidth).toBe(220);
    });
    it("is visible by default", () => {
      expect(useLayoutStore.getState().leftPanelVisible).toBe(true);
    });
    it("shows assets panel by default", () => {
      expect(useLayoutStore.getState().activeSidePanel).toBe("assets");
    });
    it("filter closed by default", () => {
      expect(useLayoutStore.getState().filterOpen).toBe(false);
    });
  });

  describe("setLayout", () => {
    it("switches to left", () => {
      useLayoutStore.getState().setLayout("left");
      expect(useLayoutStore.getState().tabBarLayout).toBe("left");
    });
    it("switches back to top", () => {
      useLayoutStore.getState().setLayout("left");
      useLayoutStore.getState().setLayout("top");
      expect(useLayoutStore.getState().tabBarLayout).toBe("top");
    });
  });

  describe("setPanelWidth", () => {
    it("sets width", () => {
      useLayoutStore.getState().setPanelWidth(300);
      expect(useLayoutStore.getState().leftPanelWidth).toBe(300);
    });
    it("clamps to minimum 48", () => {
      useLayoutStore.getState().setPanelWidth(20);
      expect(useLayoutStore.getState().leftPanelWidth).toBe(48);
    });
    it("clamps to 50vw max when window large", () => {
      Object.defineProperty(window, "innerWidth", { configurable: true, value: 2000 });
      useLayoutStore.getState().setPanelWidth(5000);
      expect(useLayoutStore.getState().leftPanelWidth).toBe(1000);
    });
  });

  describe("toggleVisible", () => {
    it("toggles visibility", () => {
      useLayoutStore.getState().toggleVisible();
      expect(useLayoutStore.getState().leftPanelVisible).toBe(false);
      useLayoutStore.getState().toggleVisible();
      expect(useLayoutStore.getState().leftPanelVisible).toBe(true);
    });
  });

  describe("switchPanel", () => {
    it("toggles between assets and tabs", () => {
      useLayoutStore.getState().switchPanel();
      expect(useLayoutStore.getState().activeSidePanel).toBe("tabs");
      useLayoutStore.getState().switchPanel();
      expect(useLayoutStore.getState().activeSidePanel).toBe("assets");
    });
  });

  describe("setActivePanel", () => {
    it("sets explicitly", () => {
      useLayoutStore.getState().setActivePanel("tabs");
      expect(useLayoutStore.getState().activeSidePanel).toBe("tabs");
    });
  });

  describe("isCollapsed selector", () => {
    it("returns true when width < 100", () => {
      expect(isCollapsed({ ...useLayoutStore.getState(), leftPanelWidth: 80 })).toBe(true);
    });
    it("returns false when width >= 100", () => {
      expect(isCollapsed({ ...useLayoutStore.getState(), leftPanelWidth: 100 })).toBe(false);
      expect(isCollapsed({ ...useLayoutStore.getState(), leftPanelWidth: 220 })).toBe(false);
    });
  });

  describe("requestOpenFilter", () => {
    it("opens filter in top mode without touching panel", () => {
      useLayoutStore.setState({ tabBarLayout: "top", activeSidePanel: "assets" });
      useLayoutStore.getState().requestOpenFilter();
      expect(useLayoutStore.getState().filterOpen).toBe(true);
      expect(useLayoutStore.getState().activeSidePanel).toBe("assets");
    });
    it("opens filter in left mode and activates tabs panel", () => {
      useLayoutStore.setState({ tabBarLayout: "left", activeSidePanel: "assets", leftPanelVisible: false });
      useLayoutStore.getState().requestOpenFilter();
      const s = useLayoutStore.getState();
      expect(s.filterOpen).toBe(true);
      expect(s.activeSidePanel).toBe("tabs");
      expect(s.leftPanelVisible).toBe(true);
    });
  });

  describe("persistence", () => {
    it("writes to localStorage when layout changes", () => {
      useLayoutStore.getState().setLayout("left");
      const raw = localStorage.getItem("layout_store");
      expect(raw).toBeTruthy();
      const data = JSON.parse(raw!);
      expect(data.tabBarLayout).toBe("left");
    });
    it("writes width changes", () => {
      useLayoutStore.getState().setPanelWidth(300);
      const data = JSON.parse(localStorage.getItem("layout_store")!);
      expect(data.leftPanelWidth).toBe(300);
    });
    it("does NOT persist filterOpen", () => {
      useLayoutStore.getState().setFilterOpen(true);
      const raw = localStorage.getItem("layout_store");
      if (raw) {
        const data = JSON.parse(raw);
        expect(data.filterOpen).toBeUndefined();
      }
    });
  });
});
