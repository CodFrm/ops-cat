/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { useAIStore } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { SideAssistantPanel } from "../components/ai/SideAssistantPanel";
import { CreateConversation, ListConversations, LoadConversationMessages } from "../../wailsjs/go/app/App";

// Note: setup.ts mocks react-i18next so `t(key)` returns the raw key.
// So button titles become the i18n keys themselves (e.g. "ai.sidebar.newChat").

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
      sidebarConversationId: null,
      sidebarUIState: { inputDraft: "", scrollTop: 0 },
      tabStates: {},
    });
    // Prevent fetchConversations (called on mount) from clobbering our seeded conversations.
    vi.mocked(ListConversations).mockImplementation(async () => {
      return useAIStore.getState().conversations as any;
    });
  });

  afterEach(() => {
    cleanup();
  });

  it("collapsed state renders nothing (triggered from left Sidebar / shortcut instead)", () => {
    const { container } = render(<SideAssistantPanel collapsed={true} onToggle={() => {}} />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("ai.sidebar.title")).not.toBeInTheDocument();
    expect(screen.queryByText("ai.sidebar.emptyGuide")).not.toBeInTheDocument();
  });

  it("expanded with no conversation shows empty guide", () => {
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);
    // With the mocked t() returning keys, the empty guide renders the raw key "ai.sidebar.emptyGuide".
    expect(screen.getByText("ai.sidebar.emptyGuide")).toBeInTheDocument();
  });

  it("clicking + new chat creates a conversation and binds sidebar", async () => {
    vi.mocked(CreateConversation).mockResolvedValue({ ID: 123, Title: "", Updatetime: 0 } as any);
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    const newBtn = screen.getByTitle("ai.sidebar.newChat");
    fireEvent.click(newBtn);

    await waitFor(() => {
      expect(useAIStore.getState().sidebarConversationId).toBe(123);
    });
  });

  it("history button opens dropdown and selecting binds sidebar", async () => {
    useAIStore.setState({
      conversations: [
        { ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any,
        { ID: 2, Title: "Conv B", Updatetime: Math.floor(Date.now() / 1000) } as any,
      ],
    });
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    expect(await screen.findByText("Conv A")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Conv A"));
    expect(useAIStore.getState().sidebarConversationId).toBe(1);
  });

  it("clicking outside the history popup closes it (within panel)", async () => {
    useAIStore.setState({
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
      sidebarConversationId: null,
    });
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    expect(await screen.findByText("Conv A")).toBeInTheDocument();

    // Click somewhere inside the panel but outside the dropdown popup.
    fireEvent.mouseDown(screen.getByText("ai.sidebar.emptyGuide"));

    await waitFor(() => {
      expect(screen.queryByText("Conv A")).not.toBeInTheDocument();
    });
  });

  it("clicking outside the history popup closes it (outside panel)", async () => {
    useAIStore.setState({
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
    });
    render(
      <div>
        <button data-testid="outside">outside</button>
        <SideAssistantPanel collapsed={false} onToggle={() => {}} />
      </div>
    );

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    expect(await screen.findByText("Conv A")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByTestId("outside"));

    await waitFor(() => {
      expect(screen.queryByText("Conv A")).not.toBeInTheDocument();
    });
  });

  it("clicking inside the history popup keeps it open", async () => {
    useAIStore.setState({
      conversations: [{ ID: 1, Title: "Conv A", Updatetime: Math.floor(Date.now() / 1000) } as any],
    });
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.history"));
    const item = await screen.findByText("Conv A");

    // mousedown inside the popup must not close it (the search input/list area).
    fireEvent.mouseDown(item);
    expect(screen.getByText("Conv A")).toBeInTheDocument();
  });

  it("clicking promote button promotes sidebar conversation and clears sidebar binding", async () => {
    vi.mocked(LoadConversationMessages).mockResolvedValue([] as any);
    useAIStore.setState({
      sidebarConversationId: 5,
      conversations: [{ ID: 5, Title: "Conv", Updatetime: 0 } as any],
      conversationMessages: { 5: [] },
      conversationStreaming: { 5: { sending: false, pendingQueue: [] } },
    });
    render(<SideAssistantPanel collapsed={false} onToggle={() => {}} />);

    fireEvent.click(screen.getByTitle("ai.sidebar.promoteToTab"));

    await waitFor(() => {
      expect(useAIStore.getState().sidebarConversationId).toBeNull();
    });
  });
});
