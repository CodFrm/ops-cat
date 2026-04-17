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
  SendAIMessage,
  StopAIGeneration,
} from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

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
    it("creates a new AI tab with a placeholder tabStates entry", () => {
      const tabId = useAIStore.getState().openNewConversationTab();

      expect(tabId).toMatch(/^ai-new-/);
      expect(useTabStore.getState().tabs).toHaveLength(1);
      expect(useTabStore.getState().tabs[0].type).toBe("ai");
      // tabStates entry exists as a UI placeholder (no more messages/sending/pendingQueue).
      expect(useAIStore.getState().tabStates[tabId]).toBeDefined();
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
      const msgs = useAIStore.getState().conversationMessages[2];
      expect(msgs).toHaveLength(1);
      expect(msgs[0].role).toBe("user");
    });
  });

  describe("isAnySending", () => {
    it("returns false when no conversations are sending", () => {
      useAIStore.setState({
        conversationStreaming: {
          1: { sending: false, pendingQueue: [] },
          2: { sending: false, pendingQueue: [] },
        },
      });
      expect(useAIStore.getState().isAnySending()).toBe(false);
    });

    it("returns true when any conversation is sending", () => {
      useAIStore.setState({
        conversationStreaming: {
          1: { sending: false, pendingQueue: [] },
          2: { sending: true, pendingQueue: [] },
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

  it("sendToTab writes only to conversationMessages for an existing conversation", async () => {
    const tabId = "ai-42";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 42, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
    });

    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    await useAIStore.getState().sendToTab(tabId, "hello");

    const cms = useAIStore.getState().conversationMessages[42];
    expect(cms.filter((m) => m.role === "user").map((m) => m.content)).toEqual(["hello"]);
  });

  it("event listener is keyed by conversationId, not tabId", async () => {
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(EventsOn).mockReturnValue(() => {});

    const tabId = "ai-77";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 77, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({ tabStates: { [tabId]: {} } });

    await useAIStore.getState().sendToTab(tabId, "hi");

    const onCalls = vi.mocked(EventsOn).mock.calls;
    const eventNames = onCalls.map((c) => c[0]);
    expect(eventNames).toContain("ai:event:77");
  });
});

describe("sidebar state", () => {
  beforeEach(() => {
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarConversationId: null,
      sidebarUIState: { inputDraft: "", scrollTop: 0 },
    });
  });

  it("bindSidebar sets sidebarConversationId", () => {
    useAIStore.getState().bindSidebar(10);
    expect(useAIStore.getState().sidebarConversationId).toBe(10);
  });

  it("bindSidebar persists to localStorage", () => {
    useAIStore.getState().bindSidebar(20);
    expect(localStorage.getItem("ai_sidebar_conversation_id")).toBe("20");
  });

  it("bindSidebar(null) removes the localStorage key", () => {
    localStorage.setItem("ai_sidebar_conversation_id", "30");
    useAIStore.getState().bindSidebar(null);
    expect(localStorage.getItem("ai_sidebar_conversation_id")).toBeNull();
  });

  it("validateSidebarConversation clears sidebar if bound conv was deleted", () => {
    useAIStore.setState({
      sidebarConversationId: 999,
      conversations: [{ ID: 1, Title: "t", Updatetime: 0 } as any],
    });
    localStorage.setItem("ai_sidebar_last_bound", "999");

    useAIStore.getState().validateSidebarConversation();

    expect(useAIStore.getState().sidebarConversationId).toBeNull();
    expect(localStorage.getItem("ai_sidebar_last_bound")).toBeNull();
  });

  it("validateSidebarConversation keeps binding if conv still exists", () => {
    useAIStore.setState({
      sidebarConversationId: 1,
      conversations: [{ ID: 1, Title: "t", Updatetime: 0 } as any],
    });
    useAIStore.getState().validateSidebarConversation();
    expect(useAIStore.getState().sidebarConversationId).toBe(1);
  });

  it("openConversationTab evicts sidebar when the same convId is in sidebar", () => {
    useAIStore.setState({
      sidebarConversationId: 42,
      conversations: [{ ID: 42, Title: "t", Updatetime: 0 } as any],
    });
    vi.mocked(SwitchConversation).mockResolvedValue([] as any);
    void useAIStore.getState().openConversationTab(42);
    expect(useAIStore.getState().sidebarConversationId).toBeNull();
  });

  it("sendFromSidebar appends user message and subscribes ai:event:${convId}", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    useAIStore.setState({
      sidebarConversationId: 88,
      conversationMessages: { 88: [] },
      conversationStreaming: { 88: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebar(88, "ping");

    const msgs = useAIStore.getState().conversationMessages[88];
    expect(msgs.some((m) => m.role === "user" && m.content === "ping")).toBe(true);
    expect(vi.mocked(EventsOn).mock.calls.some((c) => c[0] === "ai:event:88")).toBe(true);
  });

  it("stopConversation calls StopAIGeneration with the convId", async () => {
    vi.mocked(StopAIGeneration).mockResolvedValue(undefined as any);

    await useAIStore.getState().stopConversation(123);

    expect(StopAIGeneration).toHaveBeenCalledWith(123);
  });
});
