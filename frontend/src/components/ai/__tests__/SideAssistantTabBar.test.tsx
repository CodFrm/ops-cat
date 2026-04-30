import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { SideAssistantTabBar } from "../SideAssistantTabBar";
import type { SidebarAITab } from "@/stores/aiStore";

const baseProps = {
  collapsed: true,
  activeTabId: "t1",
  getStatus: () => null,
  onActivate: vi.fn(),
  onClose: vi.fn(),
  onNewChat: vi.fn(),
  onToggleCollapsed: vi.fn(),
};

const tabs: SidebarAITab[] = [
  { id: "t1", title: "写迁移", conversationId: 1 } as SidebarAITab,
  { id: "t2", title: "查日志", conversationId: 2 } as SidebarAITab,
];

describe("SideAssistantTabBar (collapsed)", () => {
  it("renders one icon button per tab with the title's first character", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    expect(screen.getByText("写")).toBeInTheDocument();
    expect(screen.getByText("查")).toBeInTheDocument();
  });

  it("does not render the full title text in collapsed mode", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    expect(screen.queryByText("写迁移")).not.toBeInTheDocument();
  });

  it("exposes the full title via aria-label", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    expect(screen.getByLabelText(/写迁移/)).toBeInTheDocument();
  });

  it("calls onActivate when an icon is clicked", () => {
    const onActivate = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onActivate={onActivate} />);
    screen.getByLabelText(/查日志/).click();
    expect(onActivate).toHaveBeenCalledWith("t2");
  });

  it("renders the active tab with a ring marker", () => {
    const { container } = render(<SideAssistantTabBar {...baseProps} tabs={tabs} />);
    const active = container.querySelector('[aria-selected="true"]');
    expect(active?.className).toMatch(/ring-2/);
  });

  it("renders a status dot when getStatus returns a non-null status", () => {
    const { container } = render(
      <SideAssistantTabBar
        {...baseProps}
        tabs={tabs}
        getStatus={(id) => (id === "t1" ? "running" : id === "t2" ? "error" : null)}
      />
    );
    expect(container.querySelector(".bg-sky-500")).toBeTruthy();
    expect(container.querySelector(".bg-rose-500")).toBeTruthy();
  });

  it("calls onClose on middle-button click (auxClick)", () => {
    const onClose = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onClose={onClose} />);
    const target = screen.getByLabelText(/查日志/);
    target.dispatchEvent(new MouseEvent("auxclick", { button: 1, bubbles: true }));
    expect(onClose).toHaveBeenCalledWith("t2");
  });

  it("calls onToggleCollapsed when ⇄ button is clicked", () => {
    const onToggleCollapsed = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onToggleCollapsed={onToggleCollapsed} />);
    screen.getByLabelText("ai.sidebar.expandRail").click();
    expect(onToggleCollapsed).toHaveBeenCalledTimes(1);
  });

  it("calls onNewChat when ＋ button is clicked", () => {
    const onNewChat = vi.fn();
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} onNewChat={onNewChat} />);
    screen.getByLabelText("ai.sidebar.newChat").click();
    expect(onNewChat).toHaveBeenCalledTimes(1);
  });
});

describe("SideAssistantTabBar (expanded)", () => {
  it("renders the full title text in expanded mode", () => {
    render(<SideAssistantTabBar {...baseProps} tabs={tabs} collapsed={false} />);
    expect(screen.getByText("写迁移")).toBeInTheDocument();
    expect(screen.getByText("查日志")).toBeInTheDocument();
  });
});
