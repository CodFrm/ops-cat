import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import { SideAssistantPanel } from "../SideAssistantPanel";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore } from "@/stores/tabStore";

describe("SideAssistantPanel rail persistence", () => {
  beforeEach(() => {
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      configured: true,
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarTabs: [
        {
          id: "t1",
          conversationId: 1,
          title: "写迁移",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
        } as any,
      ],
      activeSidebarTabId: "t1",
      tabStates: {},
    });
  });

  afterEach(() => {
    cleanup();
  });

  it("defaults to collapsed (no localStorage value → expandRail toggle visible)", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    // 当前 collapsed=true，⇄ 按钮 aria-label 是 "ai.sidebar.expandRail"
    expect(screen.getByLabelText("ai.sidebar.expandRail")).toBeInTheDocument();
  });

  it("persists rail-collapsed flip to localStorage on toggle", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    fireEvent.click(screen.getByLabelText("ai.sidebar.expandRail"));
    expect(localStorage.getItem("ai_sidebar_rail_collapsed")).toBe("false");
  });

  it("reads rail-collapsed = false from localStorage on mount", () => {
    localStorage.setItem("ai_sidebar_rail_collapsed", "false");
    render(<SideAssistantPanel collapsed={false} onToggle={vi.fn()} />);
    // 当前是 expanded 状态，按钮应该是 collapseRail（收起）
    expect(screen.getByLabelText("ai.sidebar.collapseRail")).toBeInTheDocument();
  });
});
