import { create } from "zustand";
import {
  SendAIMessage,
  SetAIProvider,
  DetectLocalCLIs,
  ResetAISession,
  CreateConversation,
  ListConversations,
  SwitchConversation,
  DeleteConversation,
  SaveConversationMessages,
} from "../../wailsjs/go/main/App";
import { ai, conversation_entity, main } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import i18n from "../i18n";
import { useTabStore, registerTabCloseHook, type AITabMeta } from "./tabStore";

// 内容块：文本或工具调用
export interface ContentBlock {
  type: "text" | "tool";
  content: string;
  toolName?: string;
  toolInput?: string;
  status?: "running" | "completed" | "error" | "pending_confirm";
  confirmId?: string;
}

export interface ChatMessage {
  role: "user" | "assistant" | "tool";
  content: string;
  blocks: ContentBlock[];
  streaming?: boolean;
}

interface StreamEventData {
  type: string;
  content?: string;
  tool_name?: string;
  tool_input?: string;
  confirm_id?: string;
  error?: string;
}

interface TabState {
  messages: ChatMessage[];
  sending: boolean;
}

// 模块级 per-tab 事件监听管理（不放 zustand，因为含函数引用）
const tabEventListeners = new Map<
  string,
  { cancel: (() => void) | null; generation: number }
>();

function getOrCreateListener(tabId: string) {
  if (!tabEventListeners.has(tabId)) {
    tabEventListeners.set(tabId, { cancel: null, generation: 0 });
  }
  return tabEventListeners.get(tabId)!;
}

function cleanupListener(tabId: string) {
  const listener = tabEventListeners.get(tabId);
  if (listener?.cancel) listener.cancel();
  tabEventListeners.delete(tabId);
}

// === 辅助函数 ===

function updateLastAssistant(
  msgs: ChatMessage[],
  updater: (msg: ChatMessage) => ChatMessage
): ChatMessage[] | null {
  const lastIdx = msgs.length - 1;
  if (lastIdx < 0 || msgs[lastIdx].role !== "assistant") return null;
  const updated = [...msgs];
  updated[lastIdx] = updater(updated[lastIdx]);
  return updated;
}

function appendText(blocks: ContentBlock[], text: string): ContentBlock[] {
  const newBlocks = [...blocks];
  const last = newBlocks[newBlocks.length - 1];
  if (last && last.type === "text") {
    newBlocks[newBlocks.length - 1] = {
      ...last,
      content: last.content + text,
    };
  } else {
    newBlocks.push({ type: "text", content: text });
  }
  return newBlocks;
}

function toDisplayMessages(
  msgs: ChatMessage[]
): main.ConversationDisplayMessage[] {
  return msgs
    .filter((m) => !m.streaming)
    .map(
      (m) =>
        new main.ConversationDisplayMessage({
          role: m.role,
          content: m.content,
          blocks: m.blocks.map(
            (b) =>
              new conversation_entity.ContentBlock({
                type: b.type,
                content: b.content,
                toolName: b.toolName,
                toolInput: b.toolInput,
                status: b.status,
              })
          ),
        })
    );
}

function convertDisplayMessages(
  displayMsgs: main.ConversationDisplayMessage[]
): ChatMessage[] {
  return (displayMsgs || []).map(
    (dm: main.ConversationDisplayMessage) => ({
      role: dm.role as "user" | "assistant" | "tool",
      content: dm.content,
      blocks: (dm.blocks || []).map(
        (b: conversation_entity.ContentBlock) => ({
          type: b.type as "text" | "tool",
          content: b.content,
          toolName: b.toolName,
          toolInput: b.toolInput,
          status: b.status as
            | "running"
            | "completed"
            | "error"
            | undefined,
        })
      ),
      streaming: false,
    })
  );
}

// === Store ===

interface AIState {
  tabStates: Record<string, TabState>;

  // 全局状态
  conversations: conversation_entity.Conversation[];
  configured: boolean;
  localCLIs: ai.CLIInfo[];

  // 配置
  configure: (
    providerType: string,
    apiBase: string,
    apiKey: string,
    model: string
  ) => Promise<void>;
  detectCLIs: () => Promise<void>;

  // 发送
  send: (content: string) => Promise<void>;
  sendToTab: (tabId: string, content: string) => Promise<void>;

  // Tab 管理 (delegates to tabStore)
  openConversationTab: (conversationId: number) => Promise<string>;
  openNewConversationTab: () => string;
  clear: () => void;

  // 会话管理
  fetchConversations: () => Promise<void>;
  deleteConversation: (id: number) => Promise<void>;

  // 查询
  isAnySending: () => boolean;
  getTabState: (tabId: string) => TabState;
}

export const useAIStore = create<AIState>((set, get) => {
  function updateTab(
    tabId: string,
    updates: Partial<TabState>
  ) {
    set((state) => {
      const current = state.tabStates[tabId] || {
        messages: [],
        sending: false,
      };
      const newTabState = { ...current, ...updates };
      return { tabStates: { ...state.tabStates, [tabId]: newTabState } };
    });
  }

  return {
    tabStates: {},

    conversations: [],
    configured: false,
    localCLIs: [],

    configure: async (providerType, apiBase, apiKey, model) => {
      await SetAIProvider(providerType, apiBase, apiKey, model);
      set({ configured: true });
    },

    detectCLIs: async () => {
      const clis = await DetectLocalCLIs();
      set({ localCLIs: clis || [] });
    },

    fetchConversations: async () => {
      try {
        const convs = await ListConversations();
        set({ conversations: convs || [] });
      } catch {
        set({ conversations: [] });
      }
    },

    deleteConversation: async (id: number) => {
      try {
        await DeleteConversation(id);
        // If there's an open tab for this conversation, close it
        const tabStore = useTabStore.getState();
        const tab = tabStore.tabs.find(
          (t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === id
        );
        if (tab) {
          tabStore.closeTab(tab.id);
        }
        await get().fetchConversations();
      } catch (e) {
        console.error("删除会话失败:", e);
      }
    },

    // === Tab 管理 ===

    openConversationTab: async (conversationId: number) => {
      const tabStore = useTabStore.getState();

      // If already open, activate
      const existing = tabStore.tabs.find(
        (t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === conversationId
      );
      if (existing) {
        tabStore.activateTab(existing.id);
        return existing.id;
      }

      const tabId = `ai-${conversationId}`;
      const state = get();
      const conv = state.conversations.find(
        (c) => c.ID === conversationId
      );
      const title = conv?.Title || "对话";

      // Load messages
      try {
        const displayMsgs = await SwitchConversation(conversationId);
        const messages = convertDisplayMessages(displayMsgs);

        // Open tab in tabStore
        tabStore.openTab({
          id: tabId,
          type: "ai",
          label: title,
          meta: { type: "ai", conversationId, title },
        });

        // Set business data
        set((state) => ({
          tabStates: {
            ...state.tabStates,
            [tabId]: { messages, sending: false },
          },
        }));

        return tabId;
      } catch (e) {
        console.error("打开会话失败:", e);
        throw e;
      }
    },

    openNewConversationTab: () => {
      const tabId = `ai-new-${Date.now()}`;
      const title = i18n.t("ai.newConversation", "新对话");

      useTabStore.getState().openTab({
        id: tabId,
        type: "ai",
        label: title,
        meta: { type: "ai", conversationId: null, title },
      });

      set((state) => ({
        tabStates: {
          ...state.tabStates,
          [tabId]: { messages: [], sending: false },
        },
      }));

      return tabId;
    },

    // === 向后兼容 ===

    send: async (content: string) => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find(
        (t) => t.id === tabStore.activeTabId && t.type === "ai"
      );
      if (!activeTab) {
        const newTabId = get().openNewConversationTab();
        await get().sendToTab(newTabId, content);
        return;
      }
      await get().sendToTab(activeTab.id, content);
    },

    clear: () => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find(
        (t) => t.id === tabStore.activeTabId && t.type === "ai"
      );
      if (activeTab) {
        tabStore.closeTab(activeTab.id);
      }
      ResetAISession().catch(() => {});
    },

    // === 核心发送 ===

    sendToTab: async (tabId: string, content: string) => {
      const state = get();
      const tabState = state.tabStates[tabId];
      if (!tabState) return;
      if (tabState.sending) return;

      const displayContent = content;
      const userMsg: ChatMessage = {
        role: "user",
        content: displayContent,
        blocks: [],
      };
      const newMessages = [...tabState.messages, userMsg];
      updateTab(tabId, { messages: newMessages, sending: true });

      // First message becomes conversation title
      if (tabState.messages.length === 0) {
        const title =
          displayContent.length > 30
            ? displayContent.slice(0, 30) + "…"
            : displayContent;
        useTabStore.getState().updateTab(tabId, {
          label: title,
          meta: { ...useTabStore.getState().tabs.find((t) => t.id === tabId)!.meta, title } as AITabMeta,
        });
      }

      const assistantMsg: ChatMessage = {
        role: "assistant",
        content: "",
        blocks: [],
        streaming: true,
      };
      updateTab(tabId, {
        messages: [...newMessages, assistantMsg],
      });

      // Ensure tab has a conversation ID
      const tabStore = useTabStore.getState();
      const tab = tabStore.tabs.find((t) => t.id === tabId);
      let convId = tab ? (tab.meta as AITabMeta).conversationId : null;

      if (!convId) {
        try {
          const conv = await CreateConversation();
          convId = conv.ID;
          // Update tab meta with conversation ID
          useTabStore.getState().updateTab(tabId, {
            meta: { type: "ai", conversationId: convId, title: tab?.label || "对话" },
          });
          get().fetchConversations();
        } catch {
          updateTab(tabId, { sending: false });
          return;
        }
      }

      // Set up event listener
      const listener = getOrCreateListener(tabId);
      listener.generation++;
      const myGeneration = listener.generation;

      if (listener.cancel) {
        listener.cancel();
        listener.cancel = null;
      }

      const eventName = `ai:event:${convId}`;
      listener.cancel = EventsOn(
        eventName,
        (event: StreamEventData) => {
          if (myGeneration !== listener.generation) return;

          const currentTabState = get().tabStates[tabId];
          if (!currentTabState) return;
          const msgs = currentTabState.messages;

          switch (event.type) {
            case "content": {
              const updated = updateLastAssistant(msgs, (msg) => ({
                ...msg,
                content: msg.content + (event.content || ""),
                blocks: appendText(msg.blocks, event.content || ""),
              }));
              if (updated) updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_start": {
              const updated = updateLastAssistant(msgs, (msg) => ({
                ...msg,
                blocks: [
                  ...msg.blocks,
                  {
                    type: "tool" as const,
                    content: "",
                    toolName: event.tool_name || "Tool",
                    toolInput: event.tool_input || "",
                    status: "running" as const,
                  },
                ],
              }));
              if (updated) updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_result": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = [...msg.blocks];
                let matchIdx = -1;
                for (let i = newBlocks.length - 1; i >= 0; i--) {
                  const b = newBlocks[i];
                  if (b.type === "tool" && b.status === "running" && b.toolName === event.tool_name) {
                    matchIdx = i;
                    break;
                  }
                }
                if (matchIdx === -1) {
                  for (let i = newBlocks.length - 1; i >= 0; i--) {
                    const b = newBlocks[i];
                    if (b.type === "tool" && b.status === "running") {
                      matchIdx = i;
                      break;
                    }
                  }
                }
                if (matchIdx !== -1) {
                  newBlocks[matchIdx] = { ...newBlocks[matchIdx], content: event.content || "", status: "completed" };
                }
                return { ...msg, blocks: newBlocks };
              });
              if (updated) updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_confirm": {
              const confirmName = event.tool_name || "run_command";
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = [...msg.blocks];
                let existIdx = -1;
                for (let i = newBlocks.length - 1; i >= 0; i--) {
                  if (newBlocks[i].type === "tool" && newBlocks[i].status === "running") {
                    existIdx = i;
                    break;
                  }
                }
                if (existIdx !== -1) {
                  newBlocks[existIdx] = {
                    ...newBlocks[existIdx],
                    toolName: confirmName,
                    toolInput: event.tool_input || newBlocks[existIdx].toolInput,
                    status: "pending_confirm" as const,
                    confirmId: event.confirm_id,
                  };
                } else {
                  newBlocks.push({
                    type: "tool" as const,
                    content: "",
                    toolName: confirmName,
                    toolInput: event.tool_input || "",
                    status: "pending_confirm" as const,
                    confirmId: event.confirm_id,
                  });
                }
                return { ...msg, blocks: newBlocks };
              });
              if (updated) updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_confirm_result": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = msg.blocks.map((b) =>
                  b.confirmId === event.confirm_id && b.status === "pending_confirm"
                    ? { ...b, status: event.content === "deny" ? ("error" as const) : ("running" as const) }
                    : b
                );
                return { ...msg, blocks: newBlocks };
              });
              if (updated) updateTab(tabId, { messages: updated });
              break;
            }

            case "done": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = msg.blocks.map((b) =>
                  b.type === "tool" && (b.status === "running" || b.status === "pending_confirm")
                    ? { ...b, status: "completed" as const }
                    : b
                );
                return { ...msg, blocks: newBlocks, streaming: false };
              });
              if (updated) {
                updateTab(tabId, { messages: updated, sending: false });
              } else {
                updateTab(tabId, { sending: false });
              }

              // Persist messages
              const finalMsgs = get().tabStates[tabId]?.messages || [];
              if (convId) {
                SaveConversationMessages(convId, toDisplayMessages(finalMsgs)).catch(() => {});
              }
              // Refresh conversations (title may have updated)
              get().fetchConversations().then(() => {
                const convs = get().conversations;
                const currentTab = useTabStore.getState().tabs.find((t) => t.id === tabId);
                if (currentTab) {
                  const meta = currentTab.meta as AITabMeta;
                  if (meta.conversationId) {
                    const conv = convs.find((c) => c.ID === meta.conversationId);
                    if (conv && conv.Title !== currentTab.label) {
                      useTabStore.getState().updateTab(tabId, {
                        label: conv.Title,
                        meta: { ...meta, title: conv.Title },
                      });
                    }
                  }
                }
              });
              break;
            }

            case "error": {
              const updated = updateLastAssistant(msgs, (msg) => ({
                ...msg,
                blocks: appendText(msg.blocks, `\n\n**Error:** ${event.error}`),
                streaming: false,
              }));
              if (updated) {
                updateTab(tabId, { messages: updated, sending: false });
              } else {
                updateTab(tabId, { sending: false });
              }
              break;
            }
          }
        }
      );

      const apiMessages = newMessages.map((m) => {
        return new ai.Message({
          role: m.role,
          content: m.content,
        });
      });

      try {
        await SendAIMessage(convId!, apiMessages);
      } catch {
        updateTab(tabId, { sending: false });
        cleanupListener(tabId);
      }
    },

    // === 查询 ===

    isAnySending: () => {
      const { tabStates } = get();
      return Object.values(tabStates).some((ts) => ts.sending);
    },

    getTabState: (tabId: string) => {
      return (
        get().tabStates[tabId] || { messages: [], sending: false }
      );
    },
  };
});

// === Close Hook: clean up when tabStore closes an AI tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "ai") return;

  const state = useAIStore.getState();
  const tabState = state.tabStates[tab.id];
  const meta = tab.meta as AITabMeta;

  // Save messages
  if (meta.conversationId && tabState?.messages.length) {
    SaveConversationMessages(
      meta.conversationId,
      toDisplayMessages(tabState.messages)
    ).catch(() => {});
  }

  // Clean up event listener
  cleanupListener(tab.id);

  // Remove tab state
  useAIStore.setState((s) => {
    const { [tab.id]: _, ...newTabStates } = s.tabStates;
    return { tabStates: newTabStates };
  });
});

// === Startup: restore AI tab data for tabs already in tabStore ===

const providerType = localStorage.getItem("ai_provider_type");
if (providerType) {
  const apiBase = localStorage.getItem("ai_api_base") || "";
  const apiKey = localStorage.getItem("ai_api_key") || "";
  const model = localStorage.getItem("ai_model") || "";
  useAIStore
    .getState()
    .configure(providerType, apiBase, apiKey, model)
    .then(async () => {
      const store = useAIStore.getState();
      await store.fetchConversations();

      const tabStore = useTabStore.getState();
      const aiTabs = tabStore.tabs.filter((t) => t.type === "ai");

      if (aiTabs.length > 0) {
        // Load messages for existing AI tabs
        const { conversations } = store;
        for (const tab of aiTabs) {
          const meta = tab.meta as AITabMeta;
          if (meta.conversationId) {
            // Verify conversation still exists
            if (!conversations.some((c) => c.ID === meta.conversationId)) {
              tabStore.closeTab(tab.id);
              continue;
            }
            try {
              const displayMsgs = await SwitchConversation(meta.conversationId);
              const messages = convertDisplayMessages(displayMsgs);
              useAIStore.setState((s) => ({
                tabStates: { ...s.tabStates, [tab.id]: { messages, sending: false } },
              }));
              // Update label from conversation title
              const conv = conversations.find((c) => c.ID === meta.conversationId);
              if (conv && conv.Title !== tab.label) {
                tabStore.updateTab(tab.id, { label: conv.Title, meta: { ...meta, title: conv.Title } });
              }
            } catch {
              tabStore.closeTab(tab.id);
            }
          } else {
            // New conversation tab (unsaved) — just initialize empty state
            useAIStore.setState((s) => ({
              tabStates: { ...s.tabStates, [tab.id]: { messages: [], sending: false } },
            }));
          }
        }
      } else {
        // No saved AI tabs, open default
        const { conversations: convs } = store;
        if (convs.length > 0) {
          store.openConversationTab(convs[0].ID).catch(() => {});
        } else {
          store.openNewConversationTab();
        }
      }
    })
    .catch(() => {});
} else {
  // Not configured, open a new tab (shows setup prompt)
  useAIStore.getState().openNewConversationTab();
}
