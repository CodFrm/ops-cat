/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { useAIStore } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { SideAssistantPanel } from "../components/ai/SideAssistantPanel";
import { ListConversations, LoadConversationMessages, DeleteConversation } from "../../wailsjs/go/app/App";

function buildSidebarTab(id: string, conversationId: number | null, title = "New conversation") {
  return {
    id,
    conversationId,
    title,
    createdAt: 1,
    uiState: {
      inputDraft: { content: "", mentions: [] },
      scrollTop: 0,
      editTarget: null,
    },
  };
}

describe("SideAssistantPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      configured: true,
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarTabs: [],
      activeSidebarTabId: null,
      tabStates: {},
    });
    vi.mocked(ListConversations).mockImplementation(async () => {
      return useAIStore.getState().conversations as any;
    });
    vi.mocked(LoadConversationMessages).mockResolvedValue([] as any);
  });

  afterEach(() => {
    cleanup();
  });

  it("collapsed state renders nothing", () => {
    const { container } = render(<SideAssistantPanel collapsed={true} onToggle={() => {}} />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("ai.sidebar.title")).not.toBeInTheDocument();
  });

  it("expanded with no sidebar tabs shows the empty guide", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);
    expect(screen.getByText("ai.sidebar.emptyGuide")).toBeInTheDocument();
  });

  it("clicking new chat creates a new blank sidebar tab", async () => {
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.newChat"));

    await waitFor(() => {
      expect(useAIStore.getState().sidebarTabs).toHaveLength(1);
    });
    expect(useAIStore.getState().activeSidebarTabId).toBe(useAIStore.getState().sidebarTabs[0].id);
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBeNull();
    expect(screen.queryByText("ai.sidebar.emptyGuide")).not.toBeInTheDocument();
  });

  it("renders the session selector as a right-side vertical rail", () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A"), buildSidebarTab("sidebar-2", 2, "Conv B")],
      activeSidebarTabId: "sidebar-1",
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      conversationMessages: {
        1: [{ role: "user", content: "hello", blocks: [], streaming: false } as any],
        2: [],
      },
      conversationStreaming: {
        1: { sending: true, pendingQueue: [] },
        2: { sending: false, pendingQueue: [] },
      },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    const tablist = screen.getByRole("tablist", { name: "ai.sidebar.sessions" });
    expect(tablist).toHaveAttribute("aria-orientation", "vertical");
    expect(tablist.closest('[data-ai-session-rail="right"]')).not.toBeNull();
    expect(screen.getByTitle("ai.sidebar.status.running")).toBeInTheDocument();
  });

  it("history selection binds the active blank tab instead of opening a duplicate", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-blank", null)],
      activeSidebarTabId: "sidebar-blank",
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    fireEvent.click(await screen.findByText("Conv A"));

    expect(useAIStore.getState().sidebarTabs).toHaveLength(1);
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBe(1);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-blank");
  });

  it("history open-in-tab adds a background sidebar tab without stealing focus", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A")],
      activeSidebarTabId: "sidebar-1",
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    const openButtons = await screen.findAllByTitle("action.openInTab");
    fireEvent.click(openButtons[1]);

    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-1");
    expect(useAIStore.getState().sidebarTabs).toHaveLength(2);
    expect(useAIStore.getState().sidebarTabs.some((tab) => tab.conversationId === 2)).toBe(true);
  });

  it("history open-in-tab creates a second host even when the conversation is already open", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A")],
      activeSidebarTabId: "sidebar-1",
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    fireEvent.click((await screen.findAllByTitle("action.openInTab"))[0]);

    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-1");
    expect(useAIStore.getState().sidebarTabs.filter((tab) => tab.conversationId === 1)).toHaveLength(2);
  });

  it("closing an inactive sidebar tab keeps the current active tab", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A"), buildSidebarTab("sidebar-2", 2, "Conv B")],
      activeSidebarTabId: "sidebar-2",
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      conversationMessages: { 1: [], 2: [] },
      conversationStreaming: {
        1: { sending: false, pendingQueue: [] },
        2: { sending: false, pendingQueue: [] },
      },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getAllByLabelText("tab.close")[0]);

    await waitFor(() => {
      expect(useAIStore.getState().sidebarTabs.map((tab) => tab.id)).toEqual(["sidebar-2"]);
    });
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-2");
  });

  it("closing the active sidebar tab activates the right neighbor first", async () => {
    useAIStore.setState({
      sidebarTabs: [
        buildSidebarTab("sidebar-1", 1, "Conv A"),
        buildSidebarTab("sidebar-2", 2, "Conv B"),
        buildSidebarTab("sidebar-3", 3, "Conv C"),
      ],
      activeSidebarTabId: "sidebar-2",
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 3, Title: "Conv C", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      conversationMessages: { 1: [], 2: [], 3: [] },
      conversationStreaming: {
        1: { sending: false, pendingQueue: [] },
        2: { sending: false, pendingQueue: [] },
        3: { sending: false, pendingQueue: [] },
      },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getAllByLabelText("tab.close")[1]);

    await waitFor(() => {
      expect(useAIStore.getState().sidebarTabs.map((tab) => tab.id)).toEqual(["sidebar-1", "sidebar-3"]);
    });
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-3");
  });

  it("closing the last sidebar tab falls back to the empty guide", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A")],
      activeSidebarTabId: "sidebar-1",
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByLabelText("tab.close"));

    await waitFor(() => {
      expect(useAIStore.getState().sidebarTabs).toHaveLength(0);
    });
    expect(useAIStore.getState().activeSidebarTabId).toBeNull();
    expect(screen.getByText("ai.sidebar.emptyGuide")).toBeInTheDocument();
  });

  it("confirming delete in history triggers DeleteConversation", async () => {
    vi.mocked(DeleteConversation).mockResolvedValue(undefined);
    useAIStore.setState({
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    const item = await screen.findByText("Conv A");
    const row = item.closest("div")!.parentElement!;
    const trashBtn = row.querySelector('button[aria-label="action.openInTab"]')
      ? row.querySelectorAll("button")[1]
      : row.querySelector("button");
    fireEvent.click(trashBtn as Element);

    fireEvent.click(await screen.findByText("action.delete"));

    await waitFor(() => {
      expect(DeleteConversation).toHaveBeenCalledWith(1);
    });
  });

  it("promote keeps the sidebar tab and opens a main workspace AI tab", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-5", 5, "Conv")],
      activeSidebarTabId: "sidebar-5",
      conversations: [{ ID: 5, Title: "Conv", Updatetime: 0 } as any],
      conversationMessages: { 5: [] },
      conversationStreaming: { 5: { sending: false, pendingQueue: [] } },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.promoteToTab"));

    await waitFor(() => {
      expect(
        useTabStore.getState().tabs.some((tab) => tab.type === "ai" && (tab.meta as any).conversationId === 5)
      ).toBe(true);
    });
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBe(5);
  });
});
