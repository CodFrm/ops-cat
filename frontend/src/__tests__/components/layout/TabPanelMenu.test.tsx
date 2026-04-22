import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TabPanelMenu } from "../../../components/layout/TabPanelMenu";
import { useLayoutStore } from "../../../stores/layoutStore";
import { useTabStore } from "../../../stores/tabStore";

/** Radix DropdownMenuTrigger listens on pointerdown (button=0) to open the menu. */
function openMenu(button: HTMLElement) {
  fireEvent.pointerDown(button, { button: 0, ctrlKey: false });
}

describe("TabPanelMenu", () => {
  beforeEach(() => {
    useLayoutStore.setState({
      tabBarLayout: "top",
      leftPanelWidth: 220,
      leftPanelVisible: true,
      activeSidePanel: "assets",
      filterOpen: false,
    });
    useTabStore.setState({ tabs: [], activeTabId: null });
  });

  afterEach(() => {
    cleanup();
  });

  it("shows 'switch to left' in top mode", () => {
    render(<TabPanelMenu mode="top" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    expect(screen.getByText("sideTabs.switchToLeft")).toBeInTheDocument();
  });

  it("shows 'switch to top' in side mode", () => {
    render(<TabPanelMenu mode="side" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    expect(screen.getByText("sideTabs.switchToTop")).toBeInTheDocument();
  });

  it("clicking 'switch to left' sets layout=left", () => {
    render(<TabPanelMenu mode="top" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    fireEvent.click(screen.getByText("sideTabs.switchToLeft"));
    expect(useLayoutStore.getState().tabBarLayout).toBe("left");
  });

  it("clicking 'switch to top' sets layout=top", () => {
    useLayoutStore.setState({ tabBarLayout: "left" });
    render(<TabPanelMenu mode="side" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    fireEvent.click(screen.getByText("sideTabs.switchToTop"));
    expect(useLayoutStore.getState().tabBarLayout).toBe("top");
  });

  it("clicking 'close all' clears tabs", () => {
    useTabStore.setState({
      tabs: [
        { id: "a", type: "page", label: "a", meta: { type: "page", pageId: "a" } },
        { id: "b", type: "page", label: "b", meta: { type: "page", pageId: "b" } },
      ],
      activeTabId: "a",
    });
    render(<TabPanelMenu mode="top" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    fireEvent.click(screen.getByText("sideTabs.closeAll"));
    expect(useTabStore.getState().tabs).toHaveLength(0);
  });

  it("clicking 'filter' calls onOpenFilter (deferred to escape DropdownMenu close)", async () => {
    const fn = vi.fn();
    render(<TabPanelMenu mode="top" onOpenFilter={fn} />);
    openMenu(screen.getByRole("button"));
    fireEvent.click(screen.getByText("shortcut.panel.filter"));
    await new Promise((r) => setTimeout(r, 0));
    expect(fn).toHaveBeenCalled();
  });

  it("side mode shows panel switch + hide actions", () => {
    render(<TabPanelMenu mode="side" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    expect(screen.getByText("sideTabs.tabsPanel")).toBeInTheDocument();
    expect(screen.getByText("sideTabs.hidePanel")).toBeInTheDocument();
  });

  it("side mode shows reverse switch when current is tabs", () => {
    useLayoutStore.setState({ activeSidePanel: "tabs" });
    render(<TabPanelMenu mode="side" onOpenFilter={vi.fn()} />);
    openMenu(screen.getByRole("button"));
    expect(screen.getByText("sideTabs.assetsPanel")).toBeInTheDocument();
  });
});
