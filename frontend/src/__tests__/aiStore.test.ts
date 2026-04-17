/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../i18n", () => ({
  default: { t: (key: string, fallback: string) => fallback || key },
}));

import { useAIStore, getAISendOnEnter, setAISendOnEnter } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import {
  GetActiveAIProvider,
  ListConversations,
  DeleteConversation,
  SwitchConversation,
} from "../../wailsjs/go/app/App";

describe("aiStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      configured: false,
    });
  });

  describe("checkConfigured", () => {
    it("sets configured=true when active provider exists", async () => {
      vi.mocked(GetActiveAIProvider).mockResolvedValue({ id: 1, name: "test", type: "openai" } as any);

      await useAIStore.getState().checkConfigured();

      expect(useAIStore.getState().configured).toBe(true);
    });

    it("sets configured=false when no active provider", async () => {
      vi.mocked(GetActiveAIProvider).mockResolvedValue(null as any);

      await useAIStore.getState().checkConfigured();

      expect(useAIStore.getState().configured).toBe(false);
    });

    it("sets configured=false on error", async () => {
      vi.mocked(GetActiveAIProvider).mockRejectedValue(new Error("fail"));

      await useAIStore.getState().checkConfigured();

      expect(useAIStore.getState().configured).toBe(false);
    });
  });

  describe("fetchConversations", () => {
    it("stores conversations from backend", async () => {
      vi.mocked(ListConversations).mockResolvedValue([{ ID: 1, Title: "Chat 1" }] as any);

      await useAIStore.getState().fetchConversations();

      expect(useAIStore.getState().conversations).toHaveLength(1);
    });

    it("handles error gracefully", async () => {
      vi.mocked(ListConversations).mockRejectedValue(new Error("fail"));

      await useAIStore.getState().fetchConversations();

      expect(useAIStore.getState().conversations).toEqual([]);
    });
  });

  describe("deleteConversation", () => {
    it("calls backend and refreshes conversations", async () => {
      vi.mocked(DeleteConversation).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockResolvedValue([]);

      useAIStore.setState({ conversations: [{ ID: 1, Title: "Chat 1" }] as any });

      await useAIStore.getState().deleteConversation(1);

      expect(DeleteConversation).toHaveBeenCalledWith(1);
      expect(ListConversations).toHaveBeenCalled();
    });

    it("closes associated tab if open", async () => {
      vi.mocked(DeleteConversation).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockResolvedValue([]);

      useTabStore.setState({
        tabs: [{ id: "ai-1", type: "ai", label: "Chat 1", meta: { type: "ai", conversationId: 1, title: "Chat 1" } }],
        activeTabId: "ai-1",
      });

      await useAIStore.getState().deleteConversation(1);

      expect(useTabStore.getState().tabs).toHaveLength(0);
    });
  });

  describe("openNewConversationTab", () => {
    it("creates a new AI tab with empty messages", () => {
      const tabId = useAIStore.getState().openNewConversationTab();

      expect(tabId).toMatch(/^ai-new-/);
      expect(useTabStore.getState().tabs).toHaveLength(1);
      expect(useTabStore.getState().tabs[0].type).toBe("ai");
      expect(useAIStore.getState().tabStates[tabId]).toEqual({ messages: [], sending: false, pendingQueue: [] });
    });
  });

  describe("openConversationTab", () => {
    it("activates existing tab if conversation is already open", async () => {
      useTabStore.setState({
        tabs: [{ id: "ai-1", type: "ai", label: "Chat", meta: { type: "ai", conversationId: 1, title: "Chat" } }],
        activeTabId: null,
      });

      const tabId = await useAIStore.getState().openConversationTab(1);

      expect(tabId).toBe("ai-1");
      expect(useTabStore.getState().activeTabId).toBe("ai-1");
    });

    it("creates new tab and loads messages for new conversation", async () => {
      useAIStore.setState({
        conversations: [{ ID: 2, Title: "Old Chat" }] as any,
      });
      vi.mocked(SwitchConversation).mockResolvedValue([{ role: "user", content: "Hello", blocks: [] }] as any);

      const tabId = await useAIStore.getState().openConversationTab(2);

      expect(tabId).toBe("ai-2");
      expect(useTabStore.getState().tabs).toHaveLength(1);
      const tabState = useAIStore.getState().tabStates[tabId];
      expect(tabState.messages).toHaveLength(1);
      expect(tabState.messages[0].role).toBe("user");
    });
  });

  describe("getTabState", () => {
    it("returns tab state for existing tab", () => {
      useAIStore.setState({
        tabStates: {
          "ai-1": { messages: [{ role: "user", content: "Hi", blocks: [] }], sending: false, pendingQueue: [] },
        },
      });

      const state = useAIStore.getState().getTabState("ai-1");
      expect(state.messages).toHaveLength(1);
    });

    it("returns default state for unknown tab", () => {
      const state = useAIStore.getState().getTabState("unknown");
      expect(state).toEqual({ messages: [], sending: false, pendingQueue: [] });
    });
  });

  describe("isAnySending", () => {
    it("returns false when no tabs are sending", () => {
      useAIStore.setState({
        tabStates: {
          "ai-1": { messages: [], sending: false, pendingQueue: [] },
          "ai-2": { messages: [], sending: false, pendingQueue: [] },
        },
      });
      expect(useAIStore.getState().isAnySending()).toBe(false);
    });

    it("returns true when any tab is sending", () => {
      useAIStore.setState({
        tabStates: {
          "ai-1": { messages: [], sending: false, pendingQueue: [] },
          "ai-2": { messages: [], sending: true, pendingQueue: [] },
        },
      });
      expect(useAIStore.getState().isAnySending()).toBe(true);
    });
  });
});

describe("AI Send on Enter settings", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("defaults to true when no localStorage value", () => {
    expect(getAISendOnEnter()).toBe(true);
  });

  it("returns stored value", () => {
    localStorage.setItem("ai_send_on_enter", "false");
    expect(getAISendOnEnter()).toBe(false);
  });

  it("setAISendOnEnter persists and dispatches event", () => {
    const handler = vi.fn();
    window.addEventListener("ai-send-on-enter-change", handler);

    setAISendOnEnter(false);

    expect(localStorage.getItem("ai_send_on_enter")).toBe("false");
    expect(handler).toHaveBeenCalledTimes(1);

    window.removeEventListener("ai-send-on-enter-change", handler);
  });
});

describe("conversationMessages (Phase 1)", () => {
  beforeEach(() => {
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      configured: false,
      conversationMessages: {},
      conversationStreaming: {},
    });
  });

  it("getMessagesByConversationId returns empty array when no conversation", () => {
    const store = useAIStore.getState();
    expect(store.getMessagesByConversationId(999)).toEqual([]);
  });

  it("getMessagesByConversationId returns messages when set", () => {
    useAIStore.setState({
      conversationMessages: {
        42: [{ role: "user", content: "hi", blocks: [] }],
      },
    });
    const store = useAIStore.getState();
    expect(store.getMessagesByConversationId(42)).toHaveLength(1);
    expect(store.getMessagesByConversationId(42)[0].content).toBe("hi");
  });

  it("getStreamingByConversationId returns default when not sending", () => {
    const store = useAIStore.getState();
    expect(store.getStreamingByConversationId(42)).toEqual({ sending: false, pendingQueue: [] });
  });

  it("getStreamingByConversationId reflects streaming state", () => {
    useAIStore.setState({
      conversationStreaming: {
        42: { sending: true, pendingQueue: ["q1", "q2"] },
      },
    });
    expect(useAIStore.getState().getStreamingByConversationId(42)).toEqual({
      sending: true,
      pendingQueue: ["q1", "q2"],
    });
  });
});
