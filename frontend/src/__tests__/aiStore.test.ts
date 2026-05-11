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
        sidebarTabs: [
          {
            id: "sidebar-1",
            conversationId: 1,
            title: "Chat 1",
            createdAt: 1,
            uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          },
        ],
        activeSidebarTabId: "sidebar-1",
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
      expect(useAIStore.getState().sidebarTabs).toEqual([]);
      expect(useAIStore.getState().conversationMessages[1]).toBeUndefined();
      expect(useAIStore.getState().conversationStreaming[1]).toBeUndefined();
    });
  });

  describe("renameConversation", () => {
    it("normalizes the title and syncs sidebar/main AI hosts optimistically", async () => {
      vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
      vi.mocked(ListConversations).mockResolvedValue([{ ID: 9, Title: "新标题", Updatetime: 10 }] as any);

      useTabStore.setState({
        tabs: [{ id: "ai-9", type: "ai", label: "旧标题", meta: { type: "ai", conversationId: 9, title: "旧标题" } }],
        activeTabId: "ai-9",
      });
      useAIStore.setState({
        conversations: [{ ID: 9, Title: "旧标题", Updatetime: 0 } as any],
        sidebarTabs: [
          {
            id: "sidebar-9",
            conversationId: 9,
            title: "旧标题",
            createdAt: 1,
            uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          },
        ],
        activeSidebarTabId: "sidebar-9",
      });

      const renamed = await useAIStore.getState().renameConversation(9, "  新标题  ");

      expect(renamed).toBe(true);
      expect(UpdateConversationTitle).toHaveBeenCalledWith(9, "新标题");
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 9)?.Title).toBe("新标题");
      expect(useAIStore.getState().sidebarTabs.find((tab) => tab.id === "sidebar-9")?.title).toBe("新标题");
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
        sidebarTabs: [
          {
            id: "sidebar-3",
            conversationId: 3,
            title: "旧标题",
            createdAt: 1,
            uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          },
        ],
      });

      const renamed = await useAIStore.getState().renameConversation(3, "新标题");

      expect(renamed).toBe(false);
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 3)?.Title).toBe("旧标题");
      expect(useAIStore.getState().sidebarTabs.find((tab) => tab.id === "sidebar-3")?.title).toBe("旧标题");
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
        sidebarTabs: [
          {
            id: "sidebar-5",
            conversationId: 5,
            title: "旧标题",
            createdAt: 1,
            uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          },
        ],
      });

      const renamed = await useAIStore.getState().renameConversation(5, "新标题");

      expect(renamed).toBe(true);
      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 5)?.Title).toBe("新标题");
      expect(useAIStore.getState().sidebarTabs.find((tab) => tab.id === "sidebar-5")?.title).toBe("新标题");
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
        sidebarTabs: [
          {
            id: "sidebar-7",
            conversationId: 7,
            title: "旧标题",
            createdAt: 1,
            uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
          },
        ],
      });

      const staleFetch = useAIStore.getState().fetchConversations();
      const rename = useAIStore.getState().renameConversation(7, "新标题");

      await rename;
      resolveStaleFetch?.([{ ID: 7, Title: "旧标题", Updatetime: 1 }] as any);
      await staleFetch;

      expect(useAIStore.getState().conversations.find((conv) => conv.ID === 7)?.Title).toBe("新标题");
      expect(useAIStore.getState().sidebarTabs.find((tab) => tab.id === "sidebar-7")?.title).toBe("新标题");
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

  // 回归 "继续" 被渲染成 chip 按钮的 bug：第二条无 mention 的消息不能聚合历史 mentions。
  // 否则后端 ai.WrapMentions 会拿到旧 mention 元数据并 stash 到下一条 row 上。
  it("sendToTab 不聚合历史 mentions —— 后续无 mention 消息的 aiContext.mentionedAssets 为空", async () => {
    const tabId = "ai-77";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 77, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: createTabState() },
      conversationMessages: {
        77: [
          {
            role: "user",
            content: "@prod-db 看",
            mentions: [{ assetId: 1, name: "prod-db", start: 0, end: 8 }],
            blocks: [],
          },
          { role: "assistant", content: "ok", blocks: [] },
        ],
      },
      conversationStreaming: { 77: { sending: false, pendingQueue: [] } },
    });
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    await useAIStore.getState().sendToTab(tabId, "继续");

    expect(SendAIMessage).toHaveBeenCalled();
    const lastCall = vi.mocked(SendAIMessage).mock.calls[vi.mocked(SendAIMessage).mock.calls.length - 1];
    const aiCtx: any = lastCall[2];
    expect(aiCtx?.mentionedAssets ?? []).toEqual([]);
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
      tabStates: { [tabId]: createTabState() },
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
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations)
      .mockResolvedValueOnce([{ ID: 89, Title: "sidebar first", Updatetime: 0 }] as any)
      .mockResolvedValue([{ ID: 89, Title: "sidebar first", Updatetime: 1 }] as any);
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

  it("updates local and backend titles when editing the first user turn if the current title still matches the old first prompt", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
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

  it("keeps a user-customized title when editing the first user turn", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    useTabStore.setState({
      tabs: [
        {
          id: "ai-94",
          type: "ai",
          label: "custom title",
          meta: { type: "ai", conversationId: 94, title: "custom title" },
        },
      ],
      activeTabId: "ai-94",
    });
    useAIStore.setState({
      conversations: [{ ID: 94, Title: "custom title", Updatetime: 0 } as any],
      tabStates: { "ai-94": createTabState() },
      conversationMessages: {
        94: [
          { role: "user", content: "old title", blocks: [] },
          { role: "assistant", content: "answer", blocks: [] },
        ],
      },
      conversationStreaming: { 94: { sending: false, pendingQueue: [] } },
      sidebarTabs: [
        {
          id: "sidebar-94",
          conversationId: 94,
          title: "custom title",
          createdAt: 1,
          uiState: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null },
        },
      ],
      activeSidebarTabId: "sidebar-94",
    });

    await useAIStore.getState().editAndResendConversation(94, 0, "new first prompt");

    expect(UpdateConversationTitle).not.toHaveBeenCalled();
    expect(useAIStore.getState().conversations.find((conv) => conv.ID === 94)?.Title).toBe("custom title");
    expect(useAIStore.getState().sidebarTabs.find((tab) => tab.id === "sidebar-94")?.title).toBe("custom title");
    expect(useTabStore.getState().tabs.find((tab) => tab.id === "ai-94")?.label).toBe("custom title");
    expect(
      (vi.mocked(SendAIMessage).mock.calls[0]?.[1] as Array<{ role: string; content: string }>).map((m) => [
        m.role,
        m.content,
      ])
    ).toEqual([["user", "new first prompt"]]);
  });

  it("editAndResendConversation does not wait for list refresh before replaying the first turn", async () => {
    vi.mocked(EventsOn).mockReturnValue(() => {});
    let resolveRefresh: ((value: any) => void) | undefined;
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
      tabStates: { "ai-93": createTabState() },
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
    vi.mocked(UpdateConversationTitle).mockResolvedValue(undefined as any);
    vi.mocked(ListConversations).mockResolvedValue([{ ID: 60, Title: "first", Updatetime: 1 }] as any);

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

// 回归：tool_start 与 approval_request 在 Wails 投递层的到达顺序竞态。
// 后端 cago 把 EventPreToolUse 推 ring buffer（goroutine A 生产，goroutine B 消费 → tool_start），
// PreToolUse hook 紧接着同步跑 RequestSingle 直接打 Wails 发 approval_request，绕过 ring buffer。
// 当 B 抢不到调度，approval_request 先到 → 旧实现把 approval block 挤到 tool 前面，
// splitBlocksByApproval 会切出 bubble[thinking] | approval | bubble[tool]，把思考与工具劈成两段气泡。
// 修复：tool_start handler 看到末尾是 pending_confirm approval 时，把 tool 插到它前面而非追加。
describe("aiStore tool_start vs approval_request 投递竞态", () => {
  const buildSidebarTab = (convId: number) => ({
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

  type StreamEvt = { type: string; [k: string]: unknown };

  async function captureHandler(convId: number) {
    const callbacks: Array<(event: StreamEvt) => void> = [];
    vi.mocked(EventsOn).mockImplementation(((eventName: string, handler: (event: StreamEvt) => void) => {
      if (eventName === `ai:event:${convId}`) callbacks.push(handler);
      return () => {};
    }) as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    useAIStore.setState({
      sidebarTabs: [buildSidebarTab(convId)],
      activeSidebarTabId: `sidebar-${convId}`,
      conversationMessages: { [convId]: [] },
      conversationStreaming: { [convId]: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab(`sidebar-${convId}`, "执行命令");
    if (callbacks.length === 0) throw new Error("EventsOn handler not registered");
    return callbacks[0];
  }

  function lastAssistantBlocks(convId: number) {
    const msgs = useAIStore.getState().conversationMessages[convId] || [];
    const last = msgs[msgs.length - 1];
    return last?.role === "assistant" ? last.blocks : [];
  }

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

  it("到达顺序 tool_start → approval_request：blocks 末尾为 [tool, approval(pending)]", async () => {
    const convId = 200;
    const fire = await captureHandler(convId);

    fire({ type: "thinking", content: "我先看一下" });
    fire({
      type: "tool_start",
      tool_name: "bash",
      tool_input: '{"command":"ls -la"}',
      tool_call_id: "call_bash_1",
    });
    fire({
      type: "approval_request",
      kind: "local_bash",
      confirm_id: "ai_200_42",
      items: [{ Type: "local_bash", Command: "ls -la" }],
    });

    const blocks = lastAssistantBlocks(convId);
    expect(blocks.map((b) => b.type)).toEqual(["thinking", "tool", "approval"]);
    expect(blocks[1]).toMatchObject({ type: "tool", toolCallId: "call_bash_1", status: "running" });
    expect(blocks[2]).toMatchObject({ type: "approval", confirmId: "ai_200_42", status: "pending_confirm" });
  });

  it("到达顺序 approval_request → tool_start（竞态）：tool 仍被插到 pending approval 前面", async () => {
    const convId = 201;
    const fire = await captureHandler(convId);

    fire({ type: "thinking", content: "我先看一下" });
    fire({
      type: "approval_request",
      kind: "local_bash",
      confirm_id: "ai_201_77",
      items: [{ Type: "local_bash", Command: "ls -la" }],
    });
    fire({
      type: "tool_start",
      tool_name: "bash",
      tool_input: '{"command":"ls -la"}',
      tool_call_id: "call_bash_2",
    });

    const blocks = lastAssistantBlocks(convId);
    expect(blocks.map((b) => b.type)).toEqual(["thinking", "tool", "approval"]);
    expect(blocks[1]).toMatchObject({ type: "tool", toolCallId: "call_bash_2", status: "running" });
    expect(blocks[2]).toMatchObject({ type: "approval", confirmId: "ai_201_77", status: "pending_confirm" });
  });

  it("approval_result(allow) 后 approval block 转 running，blocks 顺序保持 [thinking, tool, approval]", async () => {
    const convId = 202;
    const fire = await captureHandler(convId);

    fire({ type: "thinking", content: "thinking" });
    fire({ type: "approval_request", kind: "local_bash", confirm_id: "cid_1", items: [] });
    fire({ type: "tool_start", tool_name: "bash", tool_input: "{}", tool_call_id: "tc_1" });
    fire({ type: "approval_result", confirm_id: "cid_1", content: "allow" });

    const blocks = lastAssistantBlocks(convId);
    expect(blocks.map((b) => b.type)).toEqual(["thinking", "tool", "approval"]);
    expect(blocks[2]).toMatchObject({ type: "approval", confirmId: "cid_1", status: "running" });
  });

  it("非 approval 末尾不受影响：tool_start 仍按原 push 到末尾", async () => {
    const convId = 203;
    const fire = await captureHandler(convId);

    fire({ type: "thinking", content: "t" });
    fire({ type: "tool_start", tool_name: "bash", tool_input: "{}", tool_call_id: "tc_first" });
    fire({ type: "tool_result", tool_name: "bash", tool_call_id: "tc_first", content: "ok" });
    fire({ type: "tool_start", tool_name: "bash", tool_input: "{}", tool_call_id: "tc_second" });

    const blocks = lastAssistantBlocks(convId);
    expect(blocks.map((b) => b.type)).toEqual(["thinking", "tool", "tool"]);
    expect(blocks[1]).toMatchObject({ toolCallId: "tc_first", status: "completed" });
    expect(blocks[2]).toMatchObject({ toolCallId: "tc_second", status: "running" });
  });
});

// 回归：cago drainInjections 在 safe point 一次 drain 出多条 follow-up，bridge 合并
// 成单条 queue_consumed_batch；前端必须按 FIFO 一次性追加 N 条 user 消息，并只保留
// 一个尾部 assistant placeholder。逐条 queue_consumed 走的话，drain 期间不会有 token
// delta 进来填中间的 asst 占位，最终会留下一串空气泡，所以 batch 是必要路径。
describe("aiStore queue_consumed_batch", () => {
  const buildSidebarTab = (convId: number) => ({
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

  type StreamEvt = { type: string; [k: string]: unknown };

  async function captureHandler(convId: number) {
    const callbacks: Array<(event: StreamEvt) => void> = [];
    vi.mocked(EventsOn).mockImplementation(((eventName: string, handler: (event: StreamEvt) => void) => {
      if (eventName === `ai:event:${convId}`) callbacks.push(handler);
      return () => {};
    }) as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);

    useAIStore.setState({
      sidebarTabs: [buildSidebarTab(convId)],
      activeSidebarTabId: `sidebar-${convId}`,
      conversationMessages: { [convId]: [] },
      conversationStreaming: { [convId]: { sending: false, pendingQueue: [] } },
    });

    await useAIStore.getState().sendFromSidebarTab(`sidebar-${convId}`, "首条消息");
    if (callbacks.length === 0) throw new Error("EventsOn handler not registered");
    return callbacks[0];
  }

  function getMessages(convId: number) {
    return useAIStore.getState().conversationMessages[convId] || [];
  }

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

  it("3 条 batch：依次追加 3 条 user + 1 条尾部 streaming asst（无中间空气泡）", async () => {
    const convId = 300;
    const fire = await captureHandler(convId);

    // 模拟用户已经把 m1/m2/m3 排进 pendingQueue（_sendForConversation 在 sending=true
    // 时会做这件事；这里直接置位绕过具体调用路径）。
    useAIStore.setState((s) => ({
      conversationStreaming: {
        ...s.conversationStreaming,
        [convId]: {
          sending: true,
          pendingQueue: [
            { text: "m1", mentions: [{ assetId: 1, name: "a", start: 0, end: 2 }] },
            { text: "m2" },
            { text: "m3" },
          ],
        },
      },
    }));

    // 起一些工具/思考事件，给上一个 asst 至少塞一个 block，确保它会被收尾而不是被压扁
    fire({ type: "thinking", content: "考虑中" });

    fire({
      type: "queue_consumed_batch",
      queue_contents: ["m1", "m2", "m3"],
    });

    const msgs = getMessages(convId);
    // 期望布局：[首条 user, 上轮 asst(closed), user(m1), user(m2), user(m3), asst(streaming)]
    const roles = msgs.map((m) => m.role);
    expect(roles).toEqual(["user", "assistant", "user", "user", "user", "assistant"]);
    expect(msgs[1].streaming).toBe(false);
    expect(msgs[2]).toMatchObject({ role: "user", content: "m1" });
    expect(msgs[2].mentions).toEqual([{ assetId: 1, name: "a", start: 0, end: 2 }]);
    expect(msgs[3]).toMatchObject({ role: "user", content: "m2" });
    expect(msgs[4]).toMatchObject({ role: "user", content: "m3" });
    expect(msgs[5]).toMatchObject({ role: "assistant", content: "" });
    expect(msgs[5].streaming).toBe(true);

    // pendingQueue 应被清空（3 条都被消费）
    const queue = useAIStore.getState().conversationStreaming[convId]?.pendingQueue || [];
    expect(queue).toEqual([]);
  });

  it("空 queue_contents：忽略事件（防止 bridge bug 写出空 user）", async () => {
    const convId = 301;
    const fire = await captureHandler(convId);

    const before = getMessages(convId).length;
    fire({ type: "queue_consumed_batch", queue_contents: [] });
    expect(getMessages(convId).length).toBe(before);
  });

  // 防御：bridge 已修过历史 follow-up 回放（cago runloop 每次 Stream 都会重放
  // history 中的 MessageKindFollowUp，旧实现 popDisplay 返回 "" 就会把 N 条空串
  // 累积 emit 出来，前端按 FIFO 写出 N 条空 user 气泡）。即使 bridge 兜不住，
  // 前端也不应把空字符串项当成"用户消息"渲染。
  it("queue_contents 含空字符串：跳过空项，仍能正确收尾上一个 asst placeholder", async () => {
    const convId = 303;
    const fire = await captureHandler(convId);

    useAIStore.setState((s) => ({
      conversationStreaming: {
        ...s.conversationStreaming,
        [convId]: { sending: true, pendingQueue: [{ text: "live1" }] },
      },
    }));
    fire({ type: "thinking", content: "x" });

    fire({ type: "queue_consumed_batch", queue_contents: ["", "live1", ""] });

    const msgs = getMessages(convId);
    const roles = msgs.map((m) => m.role);
    // 只追加 1 条非空 user（"live1"）+ 1 条尾部 streaming asst；不出现空 user 气泡。
    expect(roles).toEqual(["user", "assistant", "user", "assistant"]);
    expect(msgs[2]).toMatchObject({ role: "user", content: "live1" });
    expect(msgs[3]).toMatchObject({ role: "assistant", content: "" });
    expect(msgs[3].streaming).toBe(true);
  });

  // 兜底极端：所有 queue_contents 均为空（bridge 老 bug 的典型症状），前端必须
  // 完全无操作 —— 不追加 user、不刷新 asst placeholder（继续保留当前流的 streaming
  // 标志）。
  it("queue_contents 全为空：完全忽略事件", async () => {
    const convId = 304;
    const fire = await captureHandler(convId);

    fire({ type: "thinking", content: "x" });
    const before = [...getMessages(convId)];

    fire({ type: "queue_consumed_batch", queue_contents: ["", "", ""] });

    const after = getMessages(convId);
    expect(after.length).toBe(before.length);
    // 上一条 asst 占位仍处于 streaming，而非被错误收尾。
    expect(after[after.length - 1]).toMatchObject({ role: "assistant", streaming: true });
  });

  it("queue_consumed_batch 后续真正流式 token 落到尾部 asst（中间 asst 不被填）", async () => {
    const convId = 302;
    const fire = await captureHandler(convId);

    useAIStore.setState((s) => ({
      conversationStreaming: {
        ...s.conversationStreaming,
        [convId]: { sending: true, pendingQueue: [{ text: "m1" }, { text: "m2" }] },
      },
    }));

    fire({ type: "queue_consumed_batch", queue_contents: ["m1", "m2"] });
    fire({ type: "content", content: "回应内容" });

    const msgs = getMessages(convId);
    const tail = msgs[msgs.length - 1];
    expect(tail.role).toBe("assistant");
    // content 走 RAF 缓冲；这里只断言尾部 asst 的 blocks/content 还在 streaming，
    // 至少把 batch 的尾 asst 视为活跃流目标即可。
    expect(tail.streaming).toBe(true);
  });
});
