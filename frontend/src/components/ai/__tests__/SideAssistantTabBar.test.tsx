import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { SideAssistantTabBar } from "../SideAssistantTabBar";
import type { SidebarAITab } from "@/stores/aiStore";

const baseProps = {
  width: 150,
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
});
