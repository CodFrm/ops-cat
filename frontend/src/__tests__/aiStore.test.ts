/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

vi.mock("../i18n", () => ({
  default: { t: (key: string, fallback: string) => fallback || key },
}));

import { useAIStore, getAISendOnEnter, setAISendOnEnter } from "../stores/aiStore";
import { useTabStore, type AITabMeta } from "../stores/tabStore";
import {
  CreateConversation,
  GetActiveAIProvider,
  ListConversations,
  DeleteConversation,
  LoadConversationMessages,
  SendAIMessage,
  StopAIGeneration,
  SaveConversationMessages,
  UpdateConversationTitle,
} from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

async function waitForStoreCondition(predicate: () => boolean, timeoutMs = 1000) {
  const start = Date.now();
  while (!predicate()) {
    if (Date.now() - start > timeoutMs) {
      throw new Error("waitForStoreCondition: timed out");
    }
    await new Promise((r) => setTimeout(r, 5));
  }
}

function createTabState() {
  return {
    inputDraft: { content: "", mentions: [] },
    scrollTop: 0,
    editTarget: null,
  };
}

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

    it("keeps the latest successful result when a later overlapping refresh fails", async () => {
      let resolveFirstFetch: ((value: any) => void) | undefined;
      vi.mocked(ListConversations)
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              resolveFirstFetch = resolve;
            }) as any
        )
        .mockRejectedValueOnce(new Error("later refresh failed"));

      useAIStore.setState({
        conversations: [{ ID: 0, Title: "旧会话", Updatetime: 0 } as any],
      });

      const firstFetch = useAIStore.getState().fetchConversations();
      const secondFetch = useAIStore.getState().fetchConversations();

      await secondFetch;
      resolveFirstFetch?.([{ ID: 8, Title: "新会话", Updatetime: 1 }] as any);
      await firstFetch;

      expect(useAIStore.getState().conversations.map((conv) => conv.ID)).toEqual([8]);
      expect(useAIStore.getState().conversations[0]?.Title).toBe("新会话");
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

    it("keeps a deleted conversation removed locally when the follow-up refresh fails", async () => {
      vi.mocked(DeleteConversation).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockRejectedValue(new Error("refresh failed"));

      useAIStore.setState({
        conversations: [
          { ID: 1, Title: "Chat 1", Updatetime: 0 } as any,
          { ID: 2, Title: "Chat 2", Updatetime: 0 } as any,
        ],
        conversationMessages: {
          1: [{ role: "user", content: "hello", blocks: [] }],
          2: [{ role: "user", content: "world", blocks: [] }],
        },
        conversationStreaming: {
          1: { sending: false, pendingQueue: [] },
          2: { sending: false, pendingQueue: [] },
        },
      });

      await useAIStore.getState().deleteConversation(1);

      expect(useAIStore.getState().conversations.map((conv) => conv.ID)).toEqual([2]);
      expect(useAIStore.getState().conversationMessages[1]).toBeUndefined();
      expect(useAIStore.getState().conversationStreaming[1]).toBeUndefined();
    });
  });

  describe("renameConversation", () => {
    it("normalizes the title, updates backend, and syncs any open AI tab", async () => {
      vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockResolvedValue([{ ID: 9, Title: "新标题", Updatetime: 10 }] as any);

      useTabStore.setState({
        tabs: [{ id: "ai-9", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 9, title: "旧标题" } }],
        activeTabId: "ai-9",
      });
      useAIStore.setState({
        conversations: [{ ID: 9, Title: "旧标题", Updatetime: 0 } as any],
      });

      const renamed = await useAIStore.getState().renameConversation(9, "  新标题  ");

      expect(renamed).toBe(true);
      expect(UpdateConversationTitle).toHaveBeenCalledWith(9, "新标题");
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 9)?.Title).toBe("新标题");
      expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-9")?.label).toBe("新标题");
      expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-9")?.meta as AITabMeta)?.title).toBe("新标题");
    });

    it("rolls back the optimistic title when backend rename fails", async () => {
      vi.mocked(UpdateConversationTitle).mockRejectedValue(new Error("fail"));

      useTabStore.setState({
        tabs: [{ id: "ai-3", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 3, title: "旧标题" } }],
        activeTabId: "ai-3",
      });
      useAIStore.setState({
        conversations: [{ ID: 3, Title: "旧标题", Updatetime: 0 } as any],
      });

      const renamed = await useAIStore.getState().renameConversation(3, "新标题");

      expect(renamed).toBe(false);
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 3)?.Title).toBe("旧标题");
      expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-3")?.label).toBe("旧标题");
      expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-3")?.meta as AITabMeta)?.title).toBe("旧标题");
    });

    it("keeps the optimistic title when backend rename succeeds but the refresh fails", async () => {
      vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockRejectedValue(new Error("refresh failed"));

      useTabStore.setState({
        tabs: [{ id: "ai-5", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 5, title: "旧标题" } }],
        activeTabId: "ai-5",
      });
      useAIStore.setState({
        conversations: [{ ID: 5, Title: "旧标题", Updatetime: 0 } as any],
      });

      const renamed = await useAIStore.getState().renameConversation(5, "新标题");

      expect(renamed).toBe(true);
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 5)?.Title).toBe("新标题");
      expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-5")?.label).toBe("新标题");
      expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-5")?.meta as AITabMeta)?.title).toBe("新标题");
    });

    it("rejects an overlapping rename for the same conversation while the first one is in flight", async () => {
      let resolveFirstRename: ((value: any) => void) | undefined;
      vi.mocked(UpdateConversationTitle).mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirstRename = resolve;
          }) as any
      );

      useTabStore.setState({
        tabs: [{ id: "ai-6", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 6, title: "旧标题" } }],
        activeTabId: "ai-6",
      });
      useAIStore.setState({
        conversations: [{ ID: 6, Title: "旧标题", Updatetime: 0 } as any],
      });

      const firstRename = useAIStore.getState().renameConversation(6, "第一次标题");
      const secondRename = useAIStore.getState().renameConversation(6, "第二次标题");

      expect(await secondRename).toBe(false);
      resolveFirstRename?.(undefined);
      await firstRename;

      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 6)?.Title).toBe("第一次标题");
      expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-6")?.label).toBe("第一次标题");
      expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-6")?.meta as AITabMeta)?.title).toBe(
        "第一次标题"
      );
    });

    it("ignores a stale fetchConversations result that returns after a rename starts", async () => {
      let resolveStaleFetch: ((value: any) => void) | undefined;
      vi.mocked(ListConversations)
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              resolveStaleFetch = resolve;
            }) as any
        )
        .mockResolvedValueOnce([{ ID: 7, Title: "新标题", Updatetime: 10 }] as any);
      vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);

      useTabStore.setState({
        tabs: [{ id: "ai-7", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 7, title: "旧标题" } }],
        activeTabId: "ai-7",
      });
      useAIStore.setState({
        conversations: [{ ID: 7, Title: "旧标题", Updatetime: 0 } as any],
      });

      const staleFetch = useAIStore.getState().fetchConversations();
      const rename = useAIStore.getState().renameConversation(7, "新标题");

      await rename;
      resolveStaleFetch?.([{ ID: 7, Title: "旧标题", Updatetime: 1 }]);
      await staleFetch;

      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 7)?.Title).toBe("新标题");
      expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-7")?.label).toBe("新标题");
      expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-7")?.meta as AITabMeta)?.title).toBe("新标题");
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
      vi.mocked(LoadConversationMessages).mockResolvedValue([{ role: "user", content: "Hello", blocks: [] }] as any);

      const tabId = await useAIStore.getState().openConversationTab(2);

      expect(tabId).toBe("ai-2");
      expect(useTabStore.getState().tabs).toHaveLength(1);
      const msgs = useAIStore.getState().conversationMessages[2];
      expect(msgs).toHaveLength(1);
      expect(msgs[0].role).toBe("user");
    });

    it("reuses in-memory live state without reloading or clearing the queue", async () => {
      useAIStore.setState({
        conversations: [{ ID: 3, Title: "Live Chat" }] as any,
        conversationMessages: {
          3: [
            { role: "user", content: "Hello", blocks: [] },
            { role: "assistant", content: "partial", blocks: [], streaming: true },
          ],
        },
        conversationStreaming: {
          3: { sending: true, pendingQueue: [{ text: "queued-1" }] },
        },
      });

      const tabId = await useAIStore.getState().openConversationTab(3);

      expect(tabId).toBe("ai-3");
      expect(LoadConversationMessages).not.toHaveBeenCalled();
      expect(useAIStore.getState().conversationMessages[3][1].content).toBe("partial");
      expect(useAIStore.getState().conversationStreaming[3]).toEqual({
        sending: true,
        pendingQueue: [{ text: "queued-1" }],
      });
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
        42: { sending: true, pendingQueue: [{ text: "q1" }, { text: "q2" }] },
      },
    });
    expect(useAIStore.getState().getStreamingByConversationId(42)).toEqual({
      sending: true,
      pendingQueue: [{ text: "q1" }, { text: "q2" }],
    });
  });

  it("sendToTab writes only to conversationMessages for an existing conversation", async () => {
    const tabId = "ai-42";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 42, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: createTabState() },
    });

    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    await useAIStore.getState().sendToTab(tabId, "hello");

    const cms = useAIStore.getState().conversationMessages[42];
    expect(cms.filter((m) => m.role === "user").map((m) => m.content)).toEqual(["hello"]);
  });

  it("sendToTab syncs local and backend titles for the first user message", async () => {
    const tabId = "ai-52";
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 52, title: "旧标题" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: createTabState() },
      conversations: [{ ID: 52, Title: "旧标题", Updatetime: 0 } as any],
      conversationMessages: { 52: [] },
      conversationStreaming: { 52: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendToTab(tabId, "first prompt");

    expect(UpdateConversationTitle).toHaveBeenCalledWith(52, "first prompt");
    expect(useAIStore.getState().conversations.find((conv) => conv.ID === 52)?.Title).toBe("first prompt");
    expect(useTabStore.getState().tabs.find((tab) => tab.id === tabId)?.label).toBe("first prompt");
  });

  it("sendToTab does not wait for list refresh before sending the first message", async () => {
    const tabId = "ai-54";
    let resolveRefresh: ((value: any) => void) | undefined;
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRefresh = resolve;
        }) as any
    );
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 54, title: "旧标题" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
      conversations: [{ ID: 54, Title: "旧标题", Updatetime: 0 } as any],
      conversationMessages: { 54: [] },
      conversationStreaming: { 54: { sending: false, pendingQueue: [] } },
    });

    const callsBeforeSend = vi.mocked(SendAIMessage).mock.calls.length;
    const sendPromise = useAIStore.getState().sendToTab(tabId, "first prompt");

    await waitForStoreCondition(() => vi.mocked(SendAIMessage).mock.calls.length === callsBeforeSend + 1);
    expect(UpdateConversationTitle).toHaveBeenCalledWith(54, "first prompt");

    resolveRefresh?.([{ ID: 54, Title: "first prompt", Updatetime: 1 }] as any);
    await sendPromise;
  });

  it("newly created AI tabs refresh conversations only once on the first send", async () => {
    const tabId = "ai-new-70";
    vi.mocked(CreateConversation).mockResolvedValue({ ID: 70, Title: "新对话", Updatetime: 0 } as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 70, Title: "first prompt", Updatetime: 1 }] as any);
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "新对话", meta: { type: "ai", conversationId: null, title: "新对话" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
      conversations: [],
    });

    const callsBeforeSend = vi.mocked(ListConversations).mock.calls.length;
    await useAIStore.getState().sendToTab(tabId, "first prompt");

    expect(vi.mocked(ListConversations).mock.calls.length - callsBeforeSend).toBe(1);
  });

  it("sendToTab rolls back the tab title when the first-send rename fails for a newly bound conversation", async () => {
    const tabId = "ai-new-53";
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockRejectedValue(new Error("rename failed"));
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "新对话", meta: { type: "ai", conversationId: 53, title: "新对话" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
      conversations: [],
      conversationMessages: { 53: [] },
      conversationStreaming: { 53: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendToTab(tabId, "first prompt");

    expect(UpdateConversationTitle).toHaveBeenCalledWith(53, "first prompt");
    expect(useTabStore.getState().tabs.find((tab) => tab.id === tabId)?.label).toBe("新对话");
    expect((useTabStore.getState().tabs.find((tab) => tab.id === tabId)?.meta as AITabMeta | undefined)?.title).toBe(
      "新对话"
    );
  });

  it("keeps a newly created conversation in the list when the first-send rename fails", async () => {
    const tabId = "ai-new-71";
    vi.mocked(CreateConversation).mockResolvedValue({ ID: 71, Title: "新对话", Updatetime: 0 } as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockRejectedValue(new Error("rename failed"));
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "新对话", meta: { type: "ai", conversationId: null, title: "新对话" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
      conversations: [],
    });

    await useAIStore.getState().sendToTab(tabId, "first prompt");

    expect(useAIStore.getState().conversations.find((conv) => conv.ID === 71)?.Title).toBe("新对话");
    expect(vi.mocked(SendAIMessage).mock.calls.at(-1)?.[0]).toBe(71);
  });

  it("event listener is keyed by conversationId, not tabId", async () => {
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(EventsOn).mockReturnValue(() => {});

    const tabId = "ai-77";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 77, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({ tabStates: { [tabId]: createTabState() } });

    await useAIStore.getState().sendToTab(tabId, "hi");

    const onCalls = vi.mocked(EventsOn).mock.calls;
    const eventNames = onCalls.map((c) => c[0]);
    expect(eventNames).toContain("ai:event:77");
  });
});

describe("sidebar state", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarTabs: [],
      activeSidebarTabId: null,
    });
  });

  const buildSidebarTab = (id: string, conversationId: number | null, title = "新对话") => ({
    id,
    conversationId,
    title,
    createdAt: 1,
    uiState: {
      inputDraft: { content: "", mentions: [] },
      scrollTop: 0,
      editTarget: null,
    },
  });

  it("openNewSidebarTab creates a blank active tab and persists the new storage keys", () => {
    const tabId = useAIStore.getState().openNewSidebarTab();
    expect(useAIStore.getState().activeSidebarTabId).toBe(tabId);
    expect(useAIStore.getState().sidebarTabs).toHaveLength(1);
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBeNull();
    expect(localStorage.getItem("ai_sidebar_tabs")).toContain(tabId);
    expect(localStorage.getItem("ai_sidebar_active_tab_id")).toBe(tabId);
  });

  it("openNewSidebarTab focuses the existing blank tab instead of creating a duplicate", () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-blank", null), buildSidebarTab("sidebar-42", 42, "Conv 42")],
      activeSidebarTabId: "sidebar-42",
    });

    const tabId = useAIStore.getState().openNewSidebarTab();

    expect(tabId).toBe("sidebar-blank");
    expect(useAIStore.getState().sidebarTabs).toHaveLength(2);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-blank");
  });

  it("openSidebarConversationInSidebar loads messages and reuses an existing host", async () => {
    useAIStore.setState({
      conversations: [{ ID: 42, Title: "Conv 42", Updatetime: 0 } as any],
    });
    vi.mocked(LoadConversationMessages).mockResolvedValue([
      { role: "user", content: "hi", blocks: [] },
      { role: "assistant", content: "hello", blocks: [] },
    ] as any);

    const firstTabId = useAIStore.getState().openSidebarConversationInSidebar(42);
    const reusedTabId = useAIStore
      .getState()
      .openSidebarConversationInSidebar(42, { activate: false, reuseIfOpen: true });

    await waitForStoreCondition(() => useAIStore.getState().conversationMessages[42] !== undefined);

    expect(reusedTabId).toBe(firstTabId);
    expect(useAIStore.getState().sidebarTabs).toHaveLength(1);
    expect(LoadConversationMessages).toHaveBeenCalledWith(42);
    expect(useAIStore.getState().conversationMessages[42]).toHaveLength(2);
    expect(useAIStore.getState().conversationStreaming[42]).toEqual({ sending: false, pendingQueue: [] });
  });

  it("openSidebarConversationInSidebar can create another host for the same conversation", async () => {
    useAIStore.setState({
      conversations: [{ ID: 42, Title: "Conv 42", Updatetime: 0 } as any],
      sidebarTabs: [buildSidebarTab("sidebar-42-a", 42, "Conv 42")],
      activeSidebarTabId: "sidebar-42-a",
    });
    vi.mocked(LoadConversationMessages).mockResolvedValue([{ role: "user", content: "hi", blocks: [] }] as any);

    const newTabId = useAIStore
      .getState()
      .openSidebarConversationInSidebar(42, { activate: false, reuseIfOpen: false });

    await waitForStoreCondition(() => useAIStore.getState().sidebarTabs.length === 2);

    expect(newTabId).not.toBe("sidebar-42-a");
    expect(useAIStore.getState().sidebarTabs.filter((tab) => tab.conversationId === 42)).toHaveLength(2);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-42-a");
  });

  it("fetchConversations loads messages for sidebar-bound conv restored from localStorage", async () => {
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 7, Title: "Restored", Updatetime: 0 }] as any);
    vi.mocked(LoadConversationMessages).mockResolvedValue([
      { role: "user", content: "from backend", blocks: [] },
    ] as any);
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-7", 7, "Restored")],
      activeSidebarTabId: "sidebar-7",
      conversationMessages: {},
      conversationStreaming: {},
    });

    await useAIStore.getState().fetchConversations();
    await waitForStoreCondition(() => useAIStore.getState().conversationMessages[7] !== undefined);

    expect(LoadConversationMessages).toHaveBeenCalledWith(7);
    expect(useAIStore.getState().conversationMessages[7]).toHaveLength(1);
  });

  it("validateSidebarTabs removes deleted conversations but keeps blank tabs", () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("dead", 999, "gone"), buildSidebarTab("blank", null)],
      activeSidebarTabId: "dead",
      conversations: [{ ID: 1, Title: "t", Updatetime: 0 } as any],
    });

    useAIStore.getState().validateSidebarTabs();

    expect(useAIStore.getState().sidebarTabs.map((tab) => tab.id)).toEqual(["blank"]);
    expect(useAIStore.getState().activeSidebarTabId).toBe("blank");
  });

  it("bindSidebarTabToConversation reuses an existing sidebar host instead of duplicating", () => {
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-a", 1, "A"), buildSidebarTab("sidebar-b", null)],
      activeSidebarTabId: "sidebar-b",
      conversations: [{ ID: 1, Title: "t", Updatetime: 0 } as any],
    });
    const reusedId = useAIStore.getState().bindSidebarTabToConversation("sidebar-b", 1);
    expect(reusedId).toBe("sidebar-a");
    expect(useAIStore.getState().sidebarTabs).toHaveLength(2);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-a");
  });

  it("sendFromSidebarTab lazily creates a conversation and syncs the title on first send", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(CreateConversation).mockResolvedValue({ ID: 89, Title: "旧标题", Updatetime: 0 } as any);
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 89, Title: "sidebar first", Updatetime: 0 }] as any);
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 89, Title: "sidebar first", Updatetime: 1 }] as any);
    useAIStore.setState({
      sidebarTabs: [buildSidebarTab("sidebar-89", null)],
      activeSidebarTabId: "sidebar-89",
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-89", "sidebar first");

    expect(UpdateConversationTitle).toHaveBeenCalledWith(89, "sidebar first");
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBe(89);
    expect(useAIStore.getState().sidebarTabs[0].title).toBe("sidebar first");
    expect(vi.mocked(EventsOn).mock.calls.some((c) => c[0] === "ai:event:89")).toBe(true);
  });

  it("getSidebarTabStatus applies waiting approval > error > running > done priority", () => {
    useAIStore.setState({
      sidebarTabs: [
        buildSidebarTab("approval", 10, "Approval"),
        buildSidebarTab("error", 11, "Error"),
        buildSidebarTab("running", 12, "Running"),
        buildSidebarTab("done", 13, "Done"),
      ],
      conversationMessages: {
        10: [
          { role: "assistant", content: "", blocks: [{ type: "approval", content: "", status: "pending_confirm" }] },
        ],
        11: [{ role: "assistant", content: "", blocks: [{ type: "tool", content: "", status: "error" }] }],
        12: [{ role: "assistant", content: "", blocks: [], streaming: true }],
        13: [{ role: "assistant", content: "done", blocks: [{ type: "text", content: "done", status: "completed" }] }],
      },
      conversationStreaming: {
        10: { sending: false, pendingQueue: [] },
        11: { sending: false, pendingQueue: [] },
        12: { sending: true, pendingQueue: [] },
        13: { sending: false, pendingQueue: [] },
      },
    });

    expect(useAIStore.getState().getSidebarTabStatus("approval")).toBe("waiting_approval");
    expect(useAIStore.getState().getSidebarTabStatus("error")).toBe("error");
    expect(useAIStore.getState().getSidebarTabStatus("running")).toBe("running");
    expect(useAIStore.getState().getSidebarTabStatus("done")).toBe("done");
  });

  it("stopConversation calls StopAIGeneration with the convId", async () => {
    vi.mocked(StopAIGeneration).mockResolvedValue(undefined as any);

    await useAIStore.getState().stopConversation(123);

    expect(StopAIGeneration).toHaveBeenCalledWith(123);
  });

  it("sidebar persistence strips mentions before writing to localStorage", () => {
    useAIStore.setState({
      sidebarTabs: [
        {
          id: "sidebar-secure",
          conversationId: 1,
          title: "Secure",
          createdAt: 1,
          uiState: {
            inputDraft: {
              content: "@prod-db",
              mentions: [{ assetId: 42, name: "prod-db", start: 0, end: 8 }],
            },
            scrollTop: 12,
            editTarget: {
              conversationId: 1,
              messageIndex: 0,
              draft: {
                content: "@prod-db again",
                mentions: [{ assetId: 42, name: "prod-db", start: 0, end: 8 }],
              },
            },
          },
        },
      ],
      activeSidebarTabId: "sidebar-secure",
    });

    const persisted = JSON.parse(localStorage.getItem("ai_sidebar_tabs") || "[]");
    expect(persisted[0]?.uiState?.inputDraft?.mentions).toEqual([]);
    expect(persisted[0]?.uiState?.editTarget?.draft?.mentions).toEqual([]);
  });
});

describe("editAndResendConversation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarTabs: [],
      activeSidebarTabId: null,
    });
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(SaveConversationMessages).mockResolvedValue(undefined as any);
    vi.mocked(StopAIGeneration).mockResolvedValue(undefined as any);
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("stops in-flight edits without letting stale stopped events drain the old queue", async () => {
    vi.useFakeTimers();
    const callbacks: Array<(event: any) => void> = [];
    const cancels: Array<ReturnType<typeof vi.fn>> = [];
    vi.mocked(EventsOn).mockImplementation(((_eventName: string, handler: (event: any) => void) => {
      callbacks.push(handler);
      const cancel = vi.fn();
      cancels.push(cancel);
      return cancel;
    }) as any);

    useAIStore.setState({
      sidebarTabs: [
        {
          id: "sidebar-55",
          conversationId: 55,
          title: "t",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-55",
      conversationMessages: { 55: [] },
      conversationStreaming: { 55: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-55", "original");
    useAIStore.setState({
      conversationStreaming: { 55: { sending: true, pendingQueue: [{ text: "queued-1" }, { text: "queued-2" }] } },
    });
    vi.mocked(StopAIGeneration).mockImplementation(async () => {
      callbacks[0]?.({ type: "stopped" });
    });

    await useAIStore.getState().editAndResendConversation(55, 0, "edited");
    await vi.runAllTimersAsync();

    const msgs = useAIStore.getState().conversationMessages[55];
    expect(msgs.map((m) => [m.role, m.content])).toEqual([
      ["user", "edited"],
      ["assistant", ""],
    ]);
    expect(useAIStore.getState().conversationStreaming[55]).toEqual({ sending: true, pendingQueue: [] });
    expect(StopAIGeneration).toHaveBeenCalledWith(55);
    expect(cancels[0]).toHaveBeenCalledTimes(1);
    expect(SendAIMessage).toHaveBeenCalledTimes(2);
    expect(
      (vi.mocked(SendAIMessage).mock.calls[1]?.[1] as Array<{ role: string; content: string }>).map((m) => [
        m.role,
        m.content,
      ])
    ).toEqual([["user", "edited"]]);
  });

  it("supports sidebar edits by conversationId with a sidebar host", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    useAIStore.setState({
      sidebarTabs: [
        {
          id: "sidebar-88",
          conversationId: 88,
          title: "t",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-88",
      conversationMessages: {
        88: [
          { role: "user", content: "sidebar old", blocks: [] },
          { role: "assistant", content: "sidebar answer", blocks: [] },
        ],
      },
      conversationStreaming: { 88: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().editAndResendConversation(88, 0, "sidebar edited");

    const msgs = useAIStore.getState().conversationMessages[88];
    expect(msgs.map((m) => [m.role, m.content])).toEqual([
      ["user", "sidebar edited"],
      ["assistant", ""],
    ]);
    expect(useTabStore.getState().tabs).toEqual([]);
    expect(vi.mocked(EventsOn).mock.calls.some((call) => call[0] === "ai:event:88")).toBe(true);
    expect(vi.mocked(SendAIMessage).mock.calls[0]?.[0]).toBe(88);
  });

  it("truncates messages after the edited user turn before resending", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    useAIStore.setState({
      conversationMessages: {
        90: [
          { role: "user", content: "first", blocks: [] },
          { role: "assistant", content: "first answer", blocks: [] },
          { role: "user", content: "second", blocks: [] },
          { role: "assistant", content: "second answer", blocks: [] },
        ],
      },
      conversationStreaming: { 90: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().editAndResendConversation(90, 2, "second revised");

    const msgs = useAIStore.getState().conversationMessages[90];
    expect(msgs.map((m) => [m.role, m.content])).toEqual([
      ["user", "first"],
      ["assistant", "first answer"],
      ["user", "second revised"],
      ["assistant", ""],
    ]);

    const sentMessages = vi.mocked(SendAIMessage).mock.calls[0]?.[1] as Array<{ role: string; content: string }>;
    expect(sentMessages.map((msg) => [msg.role, msg.content])).toEqual([
      ["user", "first"],
      ["assistant", "first answer"],
      ["user", "second revised"],
    ]);
  });

  it("ignores invalid indexes and non-user targets", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    useAIStore.setState({
      conversationMessages: {
        91: [
          { role: "user", content: "hello", blocks: [] },
          { role: "assistant", content: "world", blocks: [] },
        ],
      },
      conversationStreaming: { 91: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().editAndResendConversation(91, -1, "bad");
    await useAIStore.getState().editAndResendConversation(91, 1, "bad");
    await useAIStore.getState().editAndResendConversation(91, 99, "bad");
    await useAIStore.getState().editAndResendConversation(91, 0, "   ");

    expect(SendAIMessage).not.toHaveBeenCalled();
    expect(useAIStore.getState().conversationMessages[91].map((m) => [m.role, m.content])).toEqual([
      ["user", "hello"],
      ["assistant", "world"],
    ]);
  });

  it("updates local and backend titles when editing the first user turn", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 92, Title: "new first prompt", Updatetime: 1 }] as any);
    useTabStore.setState({
      tabs: [
        { id: "ai-92", type: "ai", label: "old title", meta: { type: "ai", conversationId: 92, title: "old title" } },
      ],
      activeTabId: "ai-92",
    });
    useAIStore.setState({
      conversations: [{ ID: 92, Title: "old title", Updatetime: 0 } as any],
      tabStates: { "ai-92": createTabState() },
      conversationMessages: {
        92: [
          { role: "user", content: "old title", blocks: [] },
          { role: "assistant", content: "answer", blocks: [] },
        ],
      },
      conversationStreaming: { 92: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().editAndResendConversation(92, 0, "new first prompt");

    expect(UpdateConversationTitle).toHaveBeenCalledWith(92, "new first prompt");
    expect(useAIStore.getState().conversations.find((conv) => conv.ID === 92)?.Title).toBe("new first prompt");
    expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-92")?.label).toBe("new first prompt");
    expect((useTabStore.getState().tabs.find((tab) => tab.id === "ai-92")?.meta as AITabMeta | undefined)?.title).toBe(
      "new first prompt"
    );
  });

  it("editAndResendConversation does not wait for list refresh before replaying the first turn", async () => {
    let resolveRefresh: ((value: any) => void) | undefined;
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(ListConversations).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRefresh = resolve;
        }) as any
    );
    useTabStore.setState({
      tabs: [
        { id: "ai-93", type: "ai", label: "old title", meta: { type: "ai", conversationId: 93, title: "old title" } },
      ],
      activeTabId: "ai-93",
    });
    useAIStore.setState({
      conversations: [{ ID: 93, Title: "old title", Updatetime: 0 } as any],
      tabStates: { "ai-93": {} },
      conversationMessages: {
        93: [
          { role: "user", content: "old title", blocks: [] },
          { role: "assistant", content: "answer", blocks: [] },
        ],
      },
      conversationStreaming: { 93: { sending: false, pendingQueue: [] } },
    });

    const replayPromise = useAIStore.getState().editAndResendConversation(93, 0, "new first prompt");

    await waitForStoreCondition(() => vi.mocked(SendAIMessage).mock.calls.length === 1);
    expect(UpdateConversationTitle).toHaveBeenCalledWith(93, "new first prompt");

    resolveRefresh?.([{ ID: 93, Title: "new first prompt", Updatetime: 1 }] as any);
    await replayPromise;
  });
});

describe("persistence debounce & streaming snapshot", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
    });
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(SaveConversationMessages).mockResolvedValue(undefined as any);
    vi.mocked(EventsOn).mockReturnValue(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("persists user message immediately and debounces follow-up streaming snapshot", async () => {
    const tabId = "ai-100";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 100, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({ tabStates: { [tabId]: createTabState() } });

    // sendToTab 新行为：用户消息立即落盘一次（避免防抖窗口内崩溃丢失用户输入），
    // 紧接着的 assistant placeholder 更新走 300ms 防抖。
    await useAIStore.getState().sendToTab(tabId, "hi");
    expect(SaveConversationMessages).toHaveBeenCalledTimes(1);
    expect(vi.mocked(SaveConversationMessages).mock.calls[0][0]).toBe(100);

    await vi.advanceTimersByTimeAsync(300);
    expect(SaveConversationMessages).toHaveBeenCalledTimes(2);
  });

  it("normalizes running/pending_confirm blocks when persisting a streaming snapshot", async () => {
    const tabId = "ai-101";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 101, title: "t" } }],
      activeTabId: tabId,
    });
    // Pre-seed an in-progress assistant message with blocks in transient states.
    useAIStore.setState({
      tabStates: { [tabId]: createTabState() },
      conversationMessages: {
        101: [
          {
            role: "assistant",
            content: "partial",
            streaming: true,
            blocks: [
              { type: "tool", content: "", status: "running", toolName: "ssh" },
              { type: "approval", content: "", status: "pending_confirm" },
              { type: "text", content: "ok", status: "completed" },
            ],
          },
        ],
      },
      conversationStreaming: { 101: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendToTab(tabId, "next");
    await vi.advanceTimersByTimeAsync(300);

    expect(SaveConversationMessages).toHaveBeenCalled();
    const [, displayMsgs] = vi.mocked(SaveConversationMessages).mock.calls[0];
    const assistant = (displayMsgs as any[]).find((m) => m.role === "assistant" && m.content === "partial");
    expect(assistant).toBeTruthy();
    expect(assistant.blocks.map((b: any) => b.status)).toEqual(["cancelled", "cancelled", "completed"]);
  });

  it("clears pending persist timer when closing the AI tab", async () => {
    const tabId = "ai-102";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 102, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({ tabStates: { [tabId]: createTabState() } });

    // sendToTab 会立即落盘一次（用户消息），后续 assistant placeholder 走 300ms 防抖。
    await useAIStore.getState().sendToTab(tabId, "hi");
    expect(SaveConversationMessages).toHaveBeenCalledTimes(1);

    // 关闭标签要取消待定的防抖定时器，并同步 flush 一次最终快照。
    useTabStore.getState().closeTab(tabId);
    expect(SaveConversationMessages).toHaveBeenCalledTimes(2);

    // 定时器已被清理，300ms 后不应再产生额外保存。
    await vi.advanceTimersByTimeAsync(300);
    expect(SaveConversationMessages).toHaveBeenCalledTimes(2);
  });

  it("preserves in-flight streaming assistant message when closing tab mid-stream", () => {
    const tabId = "ai-103";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 103, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: createTabState() },
      conversationMessages: {
        103: [
          { role: "user", content: "go", blocks: [] },
          {
            role: "assistant",
            content: "partial",
            streaming: true,
            blocks: [
              { type: "tool", content: "", status: "running", toolName: "ssh" },
              { type: "text", content: "ok", status: "completed" },
            ],
          },
        ],
      },
      conversationStreaming: { 103: { sending: true, pendingQueue: [] } },
    });

    useTabStore.getState().closeTab(tabId);

    expect(SaveConversationMessages).toHaveBeenCalledTimes(1);
    const [convIdArg, displayMsgs] = vi.mocked(SaveConversationMessages).mock.calls[0];
    expect(convIdArg).toBe(103);
    const assistant = (displayMsgs as any[]).find((m) => m.role === "assistant");
    expect(assistant).toBeTruthy();
    expect(assistant.content).toBe("partial");
    expect(assistant.blocks.map((b: any) => b.status)).toEqual(["cancelled", "completed"]);
  });
});

describe("sidebar and main tab multi-host behavior", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [{ ID: 50, Title: "t", Updatetime: 0 } as any],
      conversationMessages: { 50: [] },
      conversationStreaming: { 50: { sending: false, pendingQueue: [] } },
      sidebarTabs: [
        {
          id: "sidebar-50",
          conversationId: 50,
          title: "t",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-50",
    });
    vi.mocked(LoadConversationMessages).mockResolvedValue([] as any);
  });

  it("openConversationTab keeps the sidebar host for the same conversation", async () => {
    await useAIStore.getState().openConversationTab(50);
    expect(useAIStore.getState().sidebarTabs).toHaveLength(1);
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBe(50);
  });

  it("promoteSidebarToTab opens a main tab without clearing the sidebar host", async () => {
    const tabId = await useAIStore.getState().promoteSidebarToTab("sidebar-50");
    expect(tabId).toBeTruthy();
    expect(useAIStore.getState().sidebarTabs[0].conversationId).toBe(50);
    expect(useTabStore.getState().tabs.find((tab) => tab.id === tabId)?.type).toBe("ai");
  });

  it("deleteConversation removes every sidebar host for that conversation", async () => {
    vi.mocked(DeleteConversation).mockResolvedValue(undefined as any);
    await useAIStore.getState().deleteConversation(50);
    expect(useAIStore.getState().sidebarTabs).toEqual([]);
  });

  it("deleteConversation falls back to the right sidebar neighbor when the active host is removed", async () => {
    vi.mocked(DeleteConversation).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations).mockResolvedValue([
      { ID: 51, Title: "left", Updatetime: 0 },
      { ID: 53, Title: "right", Updatetime: 0 },
    ] as any);
    useAIStore.setState({
      conversations: [
        { ID: 51, Title: "left", Updatetime: 0 } as any,
        { ID: 52, Title: "middle", Updatetime: 0 } as any,
        { ID: 53, Title: "right", Updatetime: 0 } as any,
      ],
      sidebarTabs: [
        {
          id: "sidebar-left",
          conversationId: 51,
          title: "left",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
        {
          id: "sidebar-middle",
          conversationId: 52,
          title: "middle",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
        {
          id: "sidebar-right",
          conversationId: 53,
          title: "right",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-middle",
    });

    await useAIStore.getState().deleteConversation(52);

    expect(useAIStore.getState().sidebarTabs.map((tab) => tab.id)).toEqual(["sidebar-left", "sidebar-right"]);
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-right");
  });

  it("background-opened sidebar tabs do not steal the active sidebar tab", () => {
    useAIStore.getState().openSidebarConversationInSidebar(50, { activate: false, reuseIfOpen: true });
    const newTabId = useAIStore.getState().openSidebarConversationInSidebar(77, { activate: false, reuseIfOpen: true });
    expect(useAIStore.getState().activeSidebarTabId).toBe("sidebar-50");
    expect(useAIStore.getState().sidebarTabs.some((tab) => tab.id === newTabId)).toBe(true);
  });

  it("closing the last sidebar host stops the live conversation and keeps queued messages until stop completes", async () => {
    const callbacks: Array<(event: any) => void> = [];
    const cancels: Array<ReturnType<typeof vi.fn>> = [];
    vi.mocked(EventsOn).mockImplementation(((_eventName: string, handler: (event: any) => void) => {
      callbacks.push(handler);
      const cancel = vi.fn();
      cancels.push(cancel);
      return cancel;
    }) as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(StopAIGeneration).mockResolvedValue(undefined as any);

    useAIStore.setState({
      sidebarTabs: [
        {
          id: "sidebar-live",
          conversationId: 60,
          title: "Live",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-live",
      conversationMessages: { 60: [] },
      conversationStreaming: { 60: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-live", "first");
    useAIStore.setState({
      conversationStreaming: {
        60: { sending: true, pendingQueue: [{ text: "queued-1" }, { text: "queued-2" }] },
      },
    });

    useAIStore.getState().closeSidebarTab("sidebar-live");

    expect(StopAIGeneration).toHaveBeenCalledWith(60);
    expect(cancels[0]).not.toHaveBeenCalled();
    expect(useAIStore.getState().sidebarTabs).toEqual([]);

    callbacks[0]?.({ type: "stopped" });
    await waitForStoreCondition(() => cancels[0]?.mock.calls.length === 1);

    expect(useAIStore.getState().conversationStreaming[60]).toEqual({
      sending: false,
      pendingQueue: [{ text: "queued-1" }, { text: "queued-2" }],
    });
    expect(vi.mocked(SendAIMessage)).toHaveBeenCalledTimes(1);
  });
});

describe("DeepSeek-v4 多轮 tool 调用历史展开", () => {
  const buildHistory = () => [
    { role: "user" as const, content: "查 SSH 服务器", blocks: [], streaming: false },
    {
      role: "assistant" as const,
      content: "找到 2 台",
      streaming: false,
      blocks: [
        { type: "thinking" as const, content: "我先查一下" },
        {
          type: "tool" as const,
          content: '[{"id":1}]',
          toolName: "list_assets",
          toolInput: '{"asset_type":"ssh"}',
          toolCallId: "call_001",
          status: "completed" as const,
        },
        { type: "thinking" as const, content: "再过滤一下" },
        { type: "text" as const, content: "找到 2 台" },
      ],
    },
    { role: "user" as const, content: "再看 redis", blocks: [], streaming: false },
  ];

  const buildSidebarTabFor = (convId: number) => ({
    id: `sidebar-${convId}`,
    conversationId: convId,
    title: "新对话",
    createdAt: 1,
    uiState: {
      inputDraft: { content: "", mentions: [] },
      scrollTop: 0,
      editTarget: null,
    },
  });

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
  });

  it("DeepSeek-v4 模型：assistant blocks 展开为 assistant(tool_calls)+tool+assistant(text) 多条标准消息", async () => {
    useAIStore.setState({
      modelName: "deepseek-v4-pro",
      sidebarTabs: [buildSidebarTabFor(100)],
      activeSidebarTabId: "sidebar-100",
      conversationMessages: { 100: buildHistory() },
      conversationStreaming: { 100: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-100", "再看 redis");

    const args = vi.mocked(SendAIMessage).mock.calls.at(-1)!;
    const apiMsgs = args[1] as any[];

    // user / assistant(thinking+tool_calls) / tool / assistant(final text) / user / user
    // 注意 sendFromSidebarTab 会再追加一条 user 消息
    const roles = apiMsgs.map((m) => m.role);
    expect(roles).toEqual(["user", "assistant", "tool", "assistant", "user", "user"]);

    const toolCallAssistant = apiMsgs[1];
    expect(toolCallAssistant.thinking).toBe("我先查一下");
    expect(toolCallAssistant.reasoning_content).toBe("我先查一下");
    expect(toolCallAssistant.tool_calls).toHaveLength(1);
    expect(toolCallAssistant.tool_calls[0].id).toBe("call_001");
    expect(toolCallAssistant.tool_calls[0].function.name).toBe("list_assets");

    const toolMsg = apiMsgs[2];
    expect(toolMsg.tool_call_id).toBe("call_001");
    expect(toolMsg.content).toBe('[{"id":1}]');

    const finalAssistant = apiMsgs[3];
    expect(finalAssistant.thinking).toBe("再过滤一下");
    expect(finalAssistant.reasoning_content).toBe("再过滤一下");
    expect(finalAssistant.content).toBe("找到 2 台");
    expect(finalAssistant.tool_calls).toBeUndefined();
  });

  it("非 DeepSeek-v4 模型：保持原有塌缩行为，不展开 tool_calls，不带 reasoning_content", async () => {
    useAIStore.setState({
      modelName: "deepseek-chat",
      sidebarTabs: [buildSidebarTabFor(101)],
      activeSidebarTabId: "sidebar-101",
      conversationMessages: { 101: buildHistory() },
      conversationStreaming: { 101: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-101", "再看 redis");

    const args = vi.mocked(SendAIMessage).mock.calls.at(-1)!;
    const apiMsgs = args[1] as any[];

    // 只有 user / assistant / user / user（assistant 是塌缩后单条，不展开）
    const roles = apiMsgs.map((m) => m.role);
    expect(roles).toEqual(["user", "assistant", "user", "user"]);

    const assistantMsg = apiMsgs[1];
    expect(assistantMsg.content).toBe("找到 2 台");
    expect(assistantMsg.tool_calls).toBeUndefined();
    expect(assistantMsg.reasoning_content).toBeUndefined();
    expect(assistantMsg.thinking).toBeUndefined();
  });

  it("DeepSeek-v4 模型 + 老数据（tool block 缺 toolCallId）：兜底为塌缩消息，不抛错", async () => {
    const legacyHistory = [
      { role: "user" as const, content: "old turn", blocks: [], streaming: false },
      {
        role: "assistant" as const,
        content: "done",
        streaming: false,
        blocks: [
          { type: "thinking" as const, content: "thoughts" },
          // 缺 toolCallId 的旧持久化数据
          {
            type: "tool" as const,
            content: "result",
            toolName: "list_assets",
            toolInput: "{}",
            status: "completed" as const,
          },
          { type: "text" as const, content: "done" },
        ],
      },
      { role: "user" as const, content: "next", blocks: [], streaming: false },
    ];

    useAIStore.setState({
      modelName: "deepseek-v4-pro",
      sidebarTabs: [buildSidebarTabFor(102)],
      activeSidebarTabId: "sidebar-102",
      conversationMessages: { 102: legacyHistory },
      conversationStreaming: { 102: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab("sidebar-102", "next");

    const args = vi.mocked(SendAIMessage).mock.calls.at(-1)!;
    const apiMsgs = args[1] as any[];

    // 老数据回退到塌缩：user / assistant(单条，含 reasoning_content) / user / user
    expect(apiMsgs.map((m) => m.role)).toEqual(["user", "assistant", "user", "user"]);
    expect(apiMsgs[1].content).toBe("done");
    expect(apiMsgs[1].reasoning_content).toBe("thoughts");
    expect(apiMsgs[1].tool_calls).toBeUndefined();
  });
});
