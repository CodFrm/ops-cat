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

const defaultAIActions = {
  renameConversation: useAIStore.getState().renameConversation,
};

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

  it("collapsed state collapses outer width to 0 (panel stays in DOM for width animation)", () => {
    const { container } = render(<SideAssistantPanel collapsed={true} onToggle={() => {}} />);
    // Outer wrapper animates via width; collapsed means width: 0.
    const outer = container.firstChild as HTMLElement;
    expect(outer).toBeTruthy();
    expect(outer.style.width).toBe("0px");
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

    const tablist = screen.getByRole("tablist");
    expect(tablist).toHaveAttribute("aria-orientation", "vertical");
    expect(tablist.closest('[data-ai-session-rail="right"]')).not.toBeNull();
    expect(document.querySelector(".bg-sky-500")).toBeTruthy();
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

  it("history open-in-tab opens a new sidebar tab and jumps to it when the conversation is not yet open", async () => {
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

    expect(useAIStore.getState().sidebarTabs).toHaveLength(2);
    const newTab = useAIStore.getState().sidebarTabs.find((tab) => tab.conversationId === 2);
    expect(newTab).toBeDefined();
    expect(useAIStore.getState().activeSidebarTabId).toBe(newTab!.id);
  });

  it("history open-in-tab focuses the existing sidebar host when the conversation is already open", async () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-1", 1, "Conv A"), buildSidebarTab("sidebar-blank", null)],
      activeSidebarTabId: "sidebar-blank",
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    fireEvent.click((await screen.findAllByTitle("action.openInTab"))[0]);

    expect(useAIStore.getState().sidebarTabs.filter((tab) => tab.conversationId === 1)).toHaveLength(1);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-1");
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

  it("current conversation can be renamed from the context bar", async () => {
    const renameConversation = vi.fn().mockResolvedValue(true);
    useAIStore.setState({
      conversations: [{ ID: 7, Title: "旧标题", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 7: [] },
      conversationStreaming: { 7: { sending: false, pendingQueue: [] } },
      sidebarConversationId: 7,
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "新标题" },
    });
    fireEvent.click(screen.getByTitle("action.save"));

    await waitFor(() => {
      expect(renameConversation).toHaveBeenCalledWith(7, "新标题");
    });
  });

  it("context-bar rename ignores Enter while IME composition is active", () => {
    const renameConversation = vi.fn().mockResolvedValue(true);
    useAIStore.setState({
      conversations: [{ ID: 71, Title: "旧标题", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 71: [] },
      conversationStreaming: { 71: { sending: false, pendingQueue: [] } },
      sidebarConversationId: 71,
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    const input = screen.getByPlaceholderText("ai.renameConversationPlaceholder");
    fireEvent.change(input, { target: { value: "输入中" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });

    expect(renameConversation).not.toHaveBeenCalled();
  });

  it("context-bar rename stays disabled until the conversation metadata is loaded", () => {
    useAIStore.setState({
      conversationMessages: { 11: [] },
      conversationStreaming: { 11: { sending: false, pendingQueue: [] } },
      sidebarConversationId: 11,
      conversations: [],
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    expect(screen.getByTitle("ai.renameConversation")).toBeDisabled();
  });

  it("context-bar rename keeps edit mode open when the save fails", async () => {
    const renameConversation = vi.fn().mockResolvedValue(false);
    useAIStore.setState({
      conversations: [{ ID: 8, Title: "旧标题", Updatetime: Math.floor(Date.now() / 1000) } as any],
      conversationMessages: { 8: [] },
      conversationStreaming: { 8: { sending: false, pendingQueue: [] } },
      sidebarConversationId: 8,
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "失败标题" },
    });
    fireEvent.click(screen.getByTitle("action.save"));

    await waitFor(() => {
      expect(renameConversation).toHaveBeenCalledWith(8, "失败标题");
    });
    expect(screen.getByPlaceholderText("ai.renameConversationPlaceholder")).toBeInTheDocument();
  });

  it("a stale context-bar rename completion does not close the next conversation editor", async () => {
    let resolveFirstRename: ((value: boolean) => void) | undefined;
    const renameConversation = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<boolean>((resolve) => {
            resolveFirstRename = resolve;
          })
      )
      .mockResolvedValue(false);

    useAIStore.setState({
      conversations: [
        { ID: 21, Title: "会话 A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 22, Title: "会话 B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      conversationMessages: {
        21: [],
        22: [],
      },
      conversationStreaming: {
        21: { sending: false, pendingQueue: [] },
        22: { sending: false, pendingQueue: [] },
      },
      sidebarConversationId: 21,
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "会话 A 新标题" },
    });
    fireEvent.click(screen.getByTitle("action.save"));

    useAIStore.setState({ sidebarConversationId: 22 } as any);
    await waitFor(() => {
      expect(screen.getByTitle("ai.renameConversation")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    expect(screen.getByPlaceholderText("ai.renameConversationPlaceholder")).toBeInTheDocument();

    resolveFirstRename?.(true);

    await waitFor(() => {
      expect(screen.getByPlaceholderText("ai.renameConversationPlaceholder")).toBeInTheDocument();
    });
  });

  it("history rename edits the conversation without rebinding the sidebar", async () => {
    const renameConversation = vi.fn().mockResolvedValue(true);
    useAIStore.setState({
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    await screen.findByText("Conv A");
    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "Conv Renamed" },
    });
    fireEvent.click(screen.getByTitle("action.save"));

    await waitFor(() => {
      expect(renameConversation).toHaveBeenCalledWith(1, "Conv Renamed");
    });
    expect(useAIStore.getState().sidebarConversationId).toBeNull();
  });

  it("history rename ignores Enter while IME composition is active", async () => {
    const renameConversation = vi.fn().mockResolvedValue(true);
    useAIStore.setState({
      conversations: [{ ID: 72, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    await screen.findByText("Conv A");
    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    const input = screen.getByPlaceholderText("ai.renameConversationPlaceholder");
    fireEvent.change(input, { target: { value: "输入中" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });

    expect(renameConversation).not.toHaveBeenCalled();
  });

  it("history rename ignores repeated saves while a rename is in flight", async () => {
    let resolveRename: ((value: boolean) => void) | undefined;
    const renameConversation = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          resolveRename = resolve;
        })
    );
    useAIStore.setState({
      conversations: [{ ID: 12, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    await screen.findByText("Conv A");
    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "Conv Once" },
    });
    fireEvent.click(screen.getByTitle("action.save"));
    fireEvent.click(screen.getByTitle("action.save"));

    expect(renameConversation).toHaveBeenCalledTimes(1);

    resolveRename?.(true);
    await waitFor(() => {
      expect(screen.queryByPlaceholderText("ai.renameConversationPlaceholder")).not.toBeInTheDocument();
    });
  });

  it("history rename does not switch to another row while the current save is in flight", async () => {
    let resolveRename: ((value: boolean) => void) | undefined;
    const renameConversation = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          resolveRename = resolve;
        })
    );
    useAIStore.setState({
      conversations: [
        { ID: 31, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 32, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
      renameConversation,
    } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    await screen.findByText("Conv A");
    fireEvent.click(screen.getAllByTitle("ai.renameConversation")[0]);
    fireEvent.change(screen.getByPlaceholderText("ai.renameConversationPlaceholder"), {
      target: { value: "Conv A Renamed" },
    });
    fireEvent.click(screen.getByTitle("action.save"));

    fireEvent.click(screen.getByTitle("ai.renameConversation"));
    expect(screen.getByDisplayValue("Conv A Renamed")).toBeInTheDocument();

    resolveRename?.(true);
    await waitFor(() => {
      expect(screen.queryByPlaceholderText("ai.renameConversationPlaceholder")).not.toBeInTheDocument();
    });
  });
});
