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

// === 多 Tab 类型 ===

export interface AITab {
  id: string; // "ai-{convId}" 或 "ai-new-{timestamp}"
  conversationId: number | null;
  title: string;
  openedAt: number;
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
  // 多 Tab 状态
  openTabs: AITab[];
  activeAITabId: string | null;
  tabStates: Record<string, TabState>;

  // 全局状态
  conversations: conversation_entity.Conversation[];
  configured: boolean;
  localCLIs: ai.CLIInfo[];

  // 向后兼容（指向活跃 tab，供旧组件过渡用）
  messages: ChatMessage[];
  sending: boolean;
  currentConversationId: number | null;

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

  // Tab 管理
  openConversationTab: (conversationId: number) => Promise<string>;
  openNewConversationTab: () => string;
  closeConversationTab: (tabId: string) => void;
  setActiveAITab: (tabId: string | null) => void;
  reorderOpenTabs: (fromIndex: number, toIndex: number) => void;
  clear: () => void;

  // 会话管理
  fetchConversations: () => Promise<void>;
  deleteConversation: (id: number) => Promise<void>;

  // 查询
  isAnySending: () => boolean;
  getTabState: (tabId: string) => TabState;
}

export const useAIStore = create<AIState>((set, get) => {
  // 更新指定 tab 的 state，并同步向后兼容字段
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
      const newTabStates = { ...state.tabStates, [tabId]: newTabState };

      // 如果是活跃 tab，同步向后兼容字段
      const compat =
        state.activeAITabId === tabId
          ? {
              messages: newTabState.messages,
              sending: newTabState.sending,
            }
          : {};

      return { tabStates: newTabStates, ...compat };
    });
  }

  // 同步向后兼容字段
  function syncCompat(activeTabId: string | null) {
    const state = get();
    const tab = state.openTabs.find((t) => t.id === activeTabId);
    const tabState = activeTabId
      ? state.tabStates[activeTabId]
      : null;
    set({
      messages: tabState?.messages || [],
      sending: tabState?.sending || false,
      currentConversationId: tab?.conversationId || null,
    });
  }

  return {
    openTabs: [],
    activeAITabId: null,
    tabStates: {},

    conversations: [],
    configured: false,
    localCLIs: [],

    // 向后兼容
    messages: [],
    sending: false,
    currentConversationId: null,

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
        // 如果有打开的 tab 对应这个会话，关闭它
        const state = get();
        const tab = state.openTabs.find(
          (t) => t.conversationId === id
        );
        if (tab) {
          get().closeConversationTab(tab.id);
        }
        await get().fetchConversations();
      } catch (e) {
        console.error("删除会话失败:", e);
      }
    },

    // === Tab 管理 ===

    openConversationTab: async (conversationId: number) => {
      const state = get();
      // 如果已打开，直接切换
      const existing = state.openTabs.find(
        (t) => t.conversationId === conversationId
      );
      if (existing) {
        get().setActiveAITab(existing.id);
        return existing.id;
      }

      const tabId = `ai-${conversationId}`;
      const conv = state.conversations.find(
        (c) => c.ID === conversationId
      );
      const title = conv?.Title || "对话";

      // 加载消息
      try {
        const displayMsgs = await SwitchConversation(conversationId);
        const messages = convertDisplayMessages(displayMsgs);

        const tab: AITab = {
          id: tabId,
          conversationId,
          title,
          openedAt: Date.now(),
        };

        set((state) => ({
          openTabs: [...state.openTabs, tab],
          activeAITabId: tabId,
          tabStates: {
            ...state.tabStates,
            [tabId]: { messages, sending: false },
          },
          // 同步向后兼容
          messages,
          sending: false,
          currentConversationId: conversationId,
        }));

        return tabId;
      } catch (e) {
        console.error("打开会话失败:", e);
        throw e;
      }
    },

    openNewConversationTab: () => {
      const tabId = `ai-new-${Date.now()}`;
      const tab: AITab = {
        id: tabId,
        conversationId: null,
        title: i18n.t("ai.newConversation", "新对话"),
        openedAt: Date.now(),
      };

      set((state) => ({
        openTabs: [...state.openTabs, tab],
        activeAITabId: tabId,
        tabStates: {
          ...state.tabStates,
          [tabId]: { messages: [], sending: false },
        },
        // 同步向后兼容
        messages: [],
        sending: false,
        currentConversationId: null,
      }));

      return tabId;
    },

    closeConversationTab: (tabId: string) => {
      const state = get();
      const tab = state.openTabs.find((t) => t.id === tabId);
      const tabState = state.tabStates[tabId];

      // 保存消息（直接传 convID，无需先切换会话）
      if (tab?.conversationId && tabState?.messages.length) {
        SaveConversationMessages(
          tab.conversationId,
          toDisplayMessages(tabState.messages)
        ).catch(() => {});
      }

      // 清理事件监听
      cleanupListener(tabId);

      // 移除 tab
      set((state) => {
        const newTabs = state.openTabs.filter((t) => t.id !== tabId);
        const { [tabId]: _, ...newTabStates } = state.tabStates;

        // 如果关闭的是活跃 tab，切换到最后一个
        let newActiveId = state.activeAITabId;
        if (state.activeAITabId === tabId) {
          newActiveId =
            newTabs.length > 0 ? newTabs[newTabs.length - 1].id : null;
        }

        const activeTabState = newActiveId
          ? newTabStates[newActiveId]
          : null;
        const activeTab = newTabs.find((t) => t.id === newActiveId);

        return {
          openTabs: newTabs,
          activeAITabId: newActiveId,
          tabStates: newTabStates,
          messages: activeTabState?.messages || [],
          sending: activeTabState?.sending || false,
          currentConversationId: activeTab?.conversationId || null,
        };
      });
    },

    setActiveAITab: (tabId: string | null) => {
      set({ activeAITabId: tabId });
      syncCompat(tabId);
    },

    reorderOpenTabs: (fromIndex: number, toIndex: number) => {
      set((state) => {
        if (fromIndex === toIndex) return state;
        const newTabs = [...state.openTabs];
        const [moved] = newTabs.splice(fromIndex, 1);
        newTabs.splice(toIndex, 0, moved);
        return { openTabs: newTabs };
      });
    },

    // === 向后兼容 ===

    send: async (content: string) => {
      const { activeAITabId } = get();
      if (!activeAITabId) {
        // 自动创建新 tab
        const newTabId = get().openNewConversationTab();
        await get().sendToTab(newTabId, content);
        return;
      }
      await get().sendToTab(activeAITabId, content);
    },

    clear: () => {
      const { activeAITabId } = get();
      if (activeAITabId) {
        get().closeConversationTab(activeAITabId);
      }
      ResetAISession().catch(() => {});
    },

    // === 核心发送 ===

    sendToTab: async (tabId: string, content: string) => {
      const state = get();
      const tabState = state.tabStates[tabId];
      if (!tabState) return;
      // 仅检查当前 tab 是否正在发送，不阻塞其他 tab
      if (tabState.sending) return;

      // 添加用户消息
      const displayContent = content;
      const userMsg: ChatMessage = {
        role: "user",
        content: displayContent,
        blocks: [],
      };
      const newMessages = [...tabState.messages, userMsg];
      updateTab(tabId, { messages: newMessages, sending: true });

      // 第一条消息作为会话标题
      if (tabState.messages.length === 0) {
        const title =
          displayContent.length > 30
            ? displayContent.slice(0, 30) + "…"
            : displayContent;
        set((state) => ({
          openTabs: state.openTabs.map((t) =>
            t.id === tabId ? { ...t, title } : t
          ),
        }));
      }

      // 添加空的 assistant 消息（用于流式填充）
      const assistantMsg: ChatMessage = {
        role: "assistant",
        content: "",
        blocks: [],
        streaming: true,
      };
      updateTab(tabId, {
        messages: [...newMessages, assistantMsg],
      });

      // 确保 tab 有会话 ID
      let tab = get().openTabs.find((t) => t.id === tabId);
      let convId = tab?.conversationId || null;

      if (!convId) {
        try {
          const conv = await CreateConversation();
          convId = conv.ID;
          // 更新 tab 的 conversationId
          set((state) => ({
            openTabs: state.openTabs.map((t) =>
              t.id === tabId
                ? { ...t, conversationId: convId }
                : t
            ),
            currentConversationId:
              state.activeAITabId === tabId
                ? convId
                : state.currentConversationId,
          }));
          get().fetchConversations();
        } catch {
          updateTab(tabId, { sending: false });
          return;
        }
      }

      // 设置事件监听
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
                blocks: appendText(
                  msg.blocks,
                  event.content || ""
                ),
              }));
              if (updated)
                updateTab(tabId, { messages: updated });
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
              if (updated)
                updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_result": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = [...msg.blocks];
                // 先按 toolName 精确匹配
                let matchIdx = -1;
                for (let i = newBlocks.length - 1; i >= 0; i--) {
                  const b = newBlocks[i];
                  if (
                    b.type === "tool" &&
                    b.status === "running" &&
                    b.toolName === event.tool_name
                  ) {
                    matchIdx = i;
                    break;
                  }
                }
                // 匹配不到时 fallback 到最后一个 running 的 tool block
                // （工具的 tool_start 和 tool_result 可能 toolName 不同）
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
                  newBlocks[matchIdx] = {
                    ...newBlocks[matchIdx],
                    content: event.content || "",
                    status: "completed",
                  };
                }
                return { ...msg, blocks: newBlocks };
              });
              if (updated)
                updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_confirm": {
              const confirmName = event.tool_name || "run_command";
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = [...msg.blocks];
                // Codex 顺序执行工具，tool_confirm 一定对应最后一个 running block
                let existIdx = -1;
                for (let i = newBlocks.length - 1; i >= 0; i--) {
                  if (
                    newBlocks[i].type === "tool" &&
                    newBlocks[i].status === "running"
                  ) {
                    existIdx = i;
                    break;
                  }
                }
                if (existIdx !== -1) {
                  // 复用已有 block，更新为 pending_confirm
                  newBlocks[existIdx] = {
                    ...newBlocks[existIdx],
                    toolName: confirmName,
                    toolInput: event.tool_input || newBlocks[existIdx].toolInput,
                    status: "pending_confirm" as const,
                    confirmId: event.confirm_id,
                  };
                } else {
                  // 没有 running block（item/started 未触发），新建
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
              if (updated)
                updateTab(tabId, { messages: updated });
              break;
            }

            case "tool_confirm_result": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = msg.blocks.map((b) =>
                  b.confirmId === event.confirm_id &&
                  b.status === "pending_confirm"
                    ? {
                        ...b,
                        status:
                          event.content === "deny"
                            ? ("error" as const)
                            : ("running" as const),
                      }
                    : b
                );
                return { ...msg, blocks: newBlocks };
              });
              if (updated)
                updateTab(tabId, { messages: updated });
              break;
            }

            case "done": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = msg.blocks.map((b) =>
                  b.type === "tool" && (b.status === "running" || b.status === "pending_confirm")
                    ? { ...b, status: "completed" as const }
                    : b
                );
                return {
                  ...msg,
                  blocks: newBlocks,
                  streaming: false,
                };
              });
              if (updated) {
                updateTab(tabId, {
                  messages: updated,
                  sending: false,
                });
              } else {
                updateTab(tabId, { sending: false });
              }

              // 持久化消息
              const finalMsgs =
                get().tabStates[tabId]?.messages || [];
              if (convId) {
                SaveConversationMessages(
                  convId,
                  toDisplayMessages(finalMsgs)
                ).catch(() => {});
              }
              // 刷新会话列表（标题可能更新），完成后同步 tab 标题
              get().fetchConversations().then(() => {
                const convs = get().conversations;
                const currentTab = get().openTabs.find(
                  (t) => t.id === tabId
                );
                if (currentTab?.conversationId) {
                  const conv = convs.find(
                    (c) => c.ID === currentTab.conversationId
                  );
                  if (conv && conv.Title !== currentTab.title) {
                    set((state) => ({
                      openTabs: state.openTabs.map((t) =>
                        t.id === tabId
                          ? { ...t, title: conv.Title }
                          : t
                      ),
                    }));
                  }
                }
              });
              break;
            }

            case "error": {
              const updated = updateLastAssistant(msgs, (msg) => ({
                ...msg,
                blocks: appendText(
                  msg.blocks,
                  `\n\n**Error:** ${event.error}`
                ),
                streaming: false,
              }));
              if (updated) {
                updateTab(tabId, {
                  messages: updated,
                  sending: false,
                });
              } else {
                updateTab(tabId, { sending: false });
              }
              break;
            }
          }
        }
      );

      // 转换为后端消息格式
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

// 应用启动时自动恢复 AI 配置并打开默认 tab
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
      // 自动打开最近的会话或新建一个
      const { conversations } = useAIStore.getState();
      if (conversations.length > 0) {
        store.openConversationTab(conversations[0].ID).catch(() => {});
      } else {
        store.openNewConversationTab();
      }
    })
    .catch(() => {});
} else {
  // 未配置也打开一个新 tab（显示未配置提示）
  useAIStore.getState().openNewConversationTab();
}
