/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { useAIStore } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { SideAssistantPanel } from "../components/ai/SideAssistantPanel";
import { CreateConversation, ListConversations, SwitchConversation } from "../../wailsjs/go/app/App";

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

  it("collapsed state shows edge rail", () => {
    render(<SideAssistantPanel collapsed={true} onToggle={() => {}} />);
    // With the mocked t() returning keys, the title of the rail is "ai.sidebar.expand".
    // The header title "ai.sidebar.title" is NOT rendered in collapsed mode.
    expect(screen.queryByText("ai.sidebar.title")).not.toBeInTheDocument();
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

  it("clicking promote button promotes sidebar conversation and clears sidebar binding", async () => {
    vi.mocked(SwitchConversation).mockResolvedValue([] as any);
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
