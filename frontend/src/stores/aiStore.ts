import { useState, useEffect } from "react";
import { create } from "zustand";
import {
  SendAIMessage,
  StopAIGeneration,
  QueueAIMessage,
  GetActiveAIProvider,
  CreateConversation,
  ListConversations,
  SwitchConversation,
  DeleteConversation,
  SaveConversationMessages,
} from "../../wailsjs/go/app/App";
import { ai, conversation_entity, app } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import i18n from "../i18n";
import { useTabStore, registerTabCloseHook, registerTabRestoreHook, type AITabMeta, type Tab } from "./tabStore";

// 内容块：文本、工具调用、Sub Agent 或审批
export interface ContentBlock {
  type: "text" | "tool" | "agent" | "approval" | "thinking";
  content: string;
  toolName?: string;
  toolInput?: string;
  status?: "running" | "completed" | "error" | "pending_confirm" | "cancelled";
  confirmId?: string;
  // agent 块专用
  agentRole?: string;
  agentTask?: string;
  childBlocks?: ContentBlock[];
  // approval 块专用
  approvalKind?: "single" | "batch" | "grant";
  approvalItems?: Array<{
    type: string;
    asset_id: number;
    asset_name: string;
    group_id?: number;
    group_name?: string;
    command: string;
    detail?: string;
  }>;
  approvalDescription?: string;
  approvalSessionId?: string;
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
  agent_role?: string;
  agent_task?: string;
  // approval_request 专用
  kind?: "single" | "batch" | "grant";
  items?: Array<{
    type: string;
    asset_id: number;
    asset_name: string;
    group_id?: number;
    group_name?: string;
    command: string;
    detail?: string;
  }>;
  description?: string;
  session_id?: string;
}

interface TabState {
  messages: ChatMessage[];
  sending: boolean;
  pendingQueue: string[];
}

// 模块级 per-conversation 事件监听管理（不放 zustand，因为含函数引用）
const conversationListeners = new Map<number, { cancel: (() => void) | null; generation: number }>();

function getOrCreateConvListener(convId: number) {
  if (!conversationListeners.has(convId)) {
    conversationListeners.set(convId, { cancel: null, generation: 0 });
  }
  return conversationListeners.get(convId)!;
}

function cleanupConvListener(convId: number) {
  const listener = conversationListeners.get(convId);
  if (listener?.cancel) listener.cancel();
  conversationListeners.delete(convId);
  cleanupStreamBuffer(convId);
}

// === 流式事件缓冲（性能优化：合并高频 content/thinking 事件，按帧刷新）===

const streamBuffers = new Map<number, { content: string; thinking: string; raf: number | null }>();

function getOrCreateStreamBuffer(convId: number) {
  let buf = streamBuffers.get(convId);
  if (!buf) {
    buf = { content: "", thinking: "", raf: null };
    streamBuffers.set(convId, buf);
  }
  return buf;
}

function flushStreamBuffer(convId: number) {
  const buf = streamBuffers.get(convId);
  if (!buf) return;
  if (buf.raf !== null) {
    cancelAnimationFrame(buf.raf);
    buf.raf = null;
  }
  const cd = buf.content;
  const td = buf.thinking;
  buf.content = "";
  buf.thinking = "";
  if (!cd && !td) return;

  useAIStore.setState((state) => {
    const msgs = state.conversationMessages[convId];
    if (!msgs) return state;
    const updated = updateLastAssistant(msgs, (msg) => {
      let blocks = msg.blocks;
      let content = msg.content;
      if (td) {
        blocks = [...blocks];
        const last = blocks[blocks.length - 1];
        if (last && last.type === "thinking" && last.status === "running") {
          blocks[blocks.length - 1] = { ...last, content: last.content + td };
        } else {
          blocks.push({ type: "thinking" as const, content: td, status: "running" as const });
        }
      }
      if (cd) {
        blocks = appendText(blocks, cd);
        content += cd;
      }
      return { ...msg, content, blocks };
    });
    if (!updated) return state;

    // Mirror back to tabStates (transition — Task 5 will remove the mirror).
    const newTabStates = { ...state.tabStates };
    const tab = useTabStore
      .getState()
      .tabs.find((t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === convId);
    if (tab && newTabStates[tab.id]) {
      newTabStates[tab.id] = { ...newTabStates[tab.id], messages: updated };
    }

    return {
      conversationMessages: { ...state.conversationMessages, [convId]: updated },
      tabStates: newTabStates,
    };
  });
}

function cleanupStreamBuffer(convId: number) {
  const buf = streamBuffers.get(convId);
  if (buf?.raf != null) cancelAnimationFrame(buf.raf);
  streamBuffers.delete(convId);
}

// === 辅助函数 ===

function updateLastAssistant(msgs: ChatMessage[], updater: (msg: ChatMessage) => ChatMessage): ChatMessage[] | null {
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

function toDisplayMessages(msgs: ChatMessage[]): app.ConversationDisplayMessage[] {
  return msgs
    .filter((m) => !m.streaming)
    .map(
      (m) =>
        new app.ConversationDisplayMessage({
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

function convertDisplayMessages(displayMsgs: app.ConversationDisplayMessage[]): ChatMessage[] {
  return (displayMsgs || []).map((dm: app.ConversationDisplayMessage) => ({
    role: dm.role as "user" | "assistant" | "tool",
    content: dm.content,
    blocks: (dm.blocks || []).map((b: conversation_entity.ContentBlock) => ({
      type: b.type as "text" | "tool" | "agent",
      content: b.content,
      toolName: b.toolName,
      toolInput: b.toolInput,
      status: b.status as "running" | "completed" | "error" | undefined,
    })),
    streaming: false,
  }));
}

// === Store ===

interface AIState {
  tabStates: Record<string, TabState>;
  conversationMessages: Record<number, ChatMessage[]>;
  conversationStreaming: Record<number, { sending: boolean; pendingQueue: string[] }>;

  // 全局状态
  conversations: conversation_entity.Conversation[];
  configured: boolean;
  providerName: string;
  modelName: string;

  // 配置
  checkConfigured: () => Promise<void>;

  // 发送
  send: (content: string) => Promise<void>;
  sendToTab: (tabId: string, content: string) => Promise<void>;
  stopGeneration: (tabId: string) => Promise<void>;
  regenerate: (tabId: string, messageIndex: number) => Promise<void>;
  removeFromQueue: (tabId: string, index: number) => void;
  clearQueue: (tabId: string) => void;

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

  // NEW — 派生 getter
  getMessagesByConversationId: (convId: number) => ChatMessage[];
  getStreamingByConversationId: (convId: number) => { sending: boolean; pendingQueue: string[] };
}

export const useAIStore = create<AIState>((set, get) => {
  function drainQueue(tabId: string, convId: number) {
    const streaming = get().conversationStreaming[convId];
    if (!streaming || streaming.pendingQueue.length === 0) return;

    const queue = [...streaming.pendingQueue];
    updateConversation(convId, { pendingQueue: [] });

    // 将队列中所有消息作为独立 user message 追加
    const currentMsgs = get().conversationMessages[convId] || [];
    const newMsgs = [...currentMsgs];
    for (const text of queue) {
      newMsgs.push({
        role: "user" as const,
        content: text,
        blocks: [],
        streaming: false,
      });
    }
    updateConversation(convId, { messages: newMsgs });

    // 触发新一轮发送（空内容表示使用已有消息）
    setTimeout(() => {
      get().sendToTab(tabId, "");
    }, 0);
  }

  function updateTab(tabId: string, updates: Partial<TabState>) {
    set((state) => {
      const current = state.tabStates[tabId] || {
        messages: [],
        sending: false,
        pendingQueue: [],
      };
      const newTabState = { ...current, ...updates };

      // Resolve convId from tab meta for dual-write
      const tab = useTabStore.getState().tabs.find((t) => t.id === tabId);
      const convId = tab ? (tab.meta as AITabMeta).conversationId : null;

      const newConvMessages =
        convId != null && updates.messages !== undefined
          ? { ...state.conversationMessages, [convId]: updates.messages }
          : state.conversationMessages;

      const newConvStreaming =
        convId != null && (updates.sending !== undefined || updates.pendingQueue !== undefined)
          ? {
              ...state.conversationStreaming,
              [convId]: {
                sending: updates.sending ?? state.conversationStreaming[convId]?.sending ?? false,
                pendingQueue: updates.pendingQueue ?? state.conversationStreaming[convId]?.pendingQueue ?? [],
              },
            }
          : state.conversationStreaming;

      return {
        tabStates: { ...state.tabStates, [tabId]: newTabState },
        conversationMessages: newConvMessages,
        conversationStreaming: newConvStreaming,
      };
    });
  }

  function updateConversation(
    convId: number,
    updates: { messages?: ChatMessage[]; sending?: boolean; pendingQueue?: string[] }
  ) {
    set((state) => {
      const newConvMessages =
        updates.messages !== undefined
          ? { ...state.conversationMessages, [convId]: updates.messages }
          : state.conversationMessages;

      const currentStreaming = state.conversationStreaming[convId] || { sending: false, pendingQueue: [] };
      const newConvStreaming =
        updates.sending !== undefined || updates.pendingQueue !== undefined
          ? {
              ...state.conversationStreaming,
              [convId]: {
                sending: updates.sending ?? currentStreaming.sending,
                pendingQueue: updates.pendingQueue ?? currentStreaming.pendingQueue,
              },
            }
          : state.conversationStreaming;

      // Mirror tabStates (transition only — Task 5 removes this)
      const tab = useTabStore
        .getState()
        .tabs.find((t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === convId);
      let newTabStates = state.tabStates;
      if (tab) {
        const curTabState = state.tabStates[tab.id] || { messages: [], sending: false, pendingQueue: [] };
        newTabStates = {
          ...state.tabStates,
          [tab.id]: {
            messages: updates.messages ?? curTabState.messages,
            sending: updates.sending ?? curTabState.sending,
            pendingQueue: updates.pendingQueue ?? curTabState.pendingQueue,
          },
        };
      }

      return {
        conversationMessages: newConvMessages,
        conversationStreaming: newConvStreaming,
        tabStates: newTabStates,
      };
    });
  }

  return {
    tabStates: {},
    conversationMessages: {},
    conversationStreaming: {},

    conversations: [],
    configured: false,
    providerName: "",
    modelName: "",

    checkConfigured: async () => {
      try {
        const active = await GetActiveAIProvider();
        if (active) {
          set({ configured: true, providerName: active.name, modelName: active.model });
        } else {
          set({ configured: false, providerName: "", modelName: "" });
        }
      } catch {
        set({ configured: false });
      }
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
        const tab = tabStore.tabs.find((t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === id);
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
      const conv = state.conversations.find((c) => c.ID === conversationId);
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
            [tabId]: { messages, sending: false, pendingQueue: [] },
          },
        }));

        // Dual-write to conversation-keyed stores (Phase 1 transition)
        useAIStore.setState((state) => ({
          conversationMessages: { ...state.conversationMessages, [conversationId]: messages },
          conversationStreaming: {
            ...state.conversationStreaming,
            [conversationId]: { sending: false, pendingQueue: [] },
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
          [tabId]: { messages: [], sending: false, pendingQueue: [] },
        },
      }));

      return tabId;
    },

    // === 向后兼容 ===

    send: async (content: string) => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find((t) => t.id === tabStore.activeTabId && t.type === "ai");
      if (!activeTab) {
        const newTabId = get().openNewConversationTab();
        await get().sendToTab(newTabId, content);
        return;
      }
      await get().sendToTab(activeTab.id, content);
    },

    clear: () => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find((t) => t.id === tabStore.activeTabId && t.type === "ai");
      if (activeTab) {
        tabStore.closeTab(activeTab.id);
      }
    },

    // === 核心发送 ===

    sendToTab: async (tabId: string, content: string) => {
      const state = get();
      const tabState = state.tabStates[tabId];
      if (!tabState) return;

      // 生成中时排队：推送到后端 runner 队列 + 本地队列（用于 UI 显示）
      if (tabState.sending) {
        if (content.trim()) {
          updateTab(tabId, {
            pendingQueue: [...tabState.pendingQueue, content.trim()],
          });
          // 推送到后端，工具调用间隙会被注入
          const tab = useTabStore.getState().tabs.find((t) => t.id === tabId);
          const queueConvId = tab ? (tab.meta as AITabMeta).conversationId : null;
          if (queueConvId) {
            QueueAIMessage(queueConvId, content.trim()).catch(() => {});
          }
        }
        return;
      }

      // 空内容 = drain/regen 发送（消息已经在 state 中）
      const isDrainSend = !content.trim();
      let newMessages = [...tabState.messages];
      const hadEmptyMessages = tabState.messages.length === 0;

      if (!isDrainSend) {
        const displayContent = content;
        const userMsg: ChatMessage = {
          role: "user",
          content: displayContent,
          blocks: [],
        };
        newMessages = [...newMessages, userMsg];
        updateTab(tabId, { messages: newMessages, sending: true });

        // First message becomes conversation title
        if (hadEmptyMessages) {
          const title = displayContent.length > 30 ? displayContent.slice(0, 30) + "…" : displayContent;
          useTabStore.getState().updateTab(tabId, {
            label: title,
            meta: { ...useTabStore.getState().tabs.find((t) => t.id === tabId)!.meta, title } as AITabMeta,
          });
        }
      } else {
        if (newMessages.length === 0) return;
        updateTab(tabId, { sending: true });
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

      // Set up event listener (keyed by conversationId)
      const listener = getOrCreateConvListener(convId!);
      listener.generation++;
      const myGeneration = listener.generation;

      if (listener.cancel) {
        listener.cancel();
        listener.cancel = null;
      }

      const eventName = `ai:event:${convId}`;
      listener.cancel = EventsOn(eventName, (event: StreamEventData) => {
        if (myGeneration !== listener.generation) return;

        const currentMsgs = get().conversationMessages[convId!] || [];
        if (!currentMsgs) return;

        // 高频事件缓冲：content/thinking 通过 RAF 合并，每帧最多一次状态更新
        if (event.type === "content" || event.type === "thinking") {
          const buf = getOrCreateStreamBuffer(convId!);
          if (event.type === "content") {
            buf.content += event.content || "";
          } else {
            buf.thinking += event.content || "";
          }
          if (buf.raf === null) {
            buf.raf = requestAnimationFrame(() => {
              const b = streamBuffers.get(convId!);
              if (b) b.raf = null;
              flushStreamBuffer(convId!);
            });
          }
          return;
        }

        // 非流式事件：先刷新缓冲区保证顺序，再处理
        flushStreamBuffer(convId!);
        const msgs = get().conversationMessages[convId!] || currentMsgs;

        switch (event.type) {
          case "agent_start": {
            const updated = updateLastAssistant(msgs, (msg) => ({
              ...msg,
              blocks: [
                ...msg.blocks,
                {
                  type: "agent" as const,
                  content: "",
                  agentRole: event.agent_role || "",
                  agentTask: event.agent_task || "",
                  status: "running" as const,
                  childBlocks: [],
                },
              ],
            }));
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "agent_end": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = [...msg.blocks];
              for (let i = newBlocks.length - 1; i >= 0; i--) {
                if (newBlocks[i].type === "agent" && newBlocks[i].status === "running") {
                  newBlocks[i] = { ...newBlocks[i], content: event.content || "", status: "completed" };
                  break;
                }
              }
              return { ...msg, blocks: newBlocks };
            });
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "tool_start": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = [...msg.blocks];
              const toolBlock: ContentBlock = {
                type: "tool" as const,
                content: "",
                toolName: event.tool_name || "Tool",
                toolInput: event.tool_input || "",
                status: "running" as const,
              };

              // 如果有 running 的 agent 块，嵌套到 childBlocks
              let agentIdx = -1;
              for (let i = newBlocks.length - 1; i >= 0; i--) {
                if (newBlocks[i].type === "agent" && newBlocks[i].status === "running") {
                  agentIdx = i;
                  break;
                }
              }
              if (agentIdx !== -1) {
                const agentBlock = { ...newBlocks[agentIdx] };
                agentBlock.childBlocks = [...(agentBlock.childBlocks || []), toolBlock];
                newBlocks[agentIdx] = agentBlock;
              } else {
                newBlocks.push(toolBlock);
              }

              return { ...msg, blocks: newBlocks };
            });
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "tool_result": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = [...msg.blocks];

              // 先检查是否在 running 的 agent 块内
              let agentIdx = -1;
              for (let i = newBlocks.length - 1; i >= 0; i--) {
                if (newBlocks[i].type === "agent" && newBlocks[i].status === "running") {
                  agentIdx = i;
                  break;
                }
              }
              if (agentIdx !== -1 && newBlocks[agentIdx].childBlocks) {
                const agentBlock = { ...newBlocks[agentIdx] };
                const children = [...(agentBlock.childBlocks || [])];
                let matchIdx = -1;
                for (let i = children.length - 1; i >= 0; i--) {
                  if (
                    children[i].type === "tool" &&
                    children[i].status === "running" &&
                    children[i].toolName === event.tool_name
                  ) {
                    matchIdx = i;
                    break;
                  }
                }
                if (matchIdx === -1) {
                  for (let i = children.length - 1; i >= 0; i--) {
                    if (children[i].type === "tool" && children[i].status === "running") {
                      matchIdx = i;
                      break;
                    }
                  }
                }
                if (matchIdx !== -1) {
                  children[matchIdx] = { ...children[matchIdx], content: event.content || "", status: "completed" };
                  agentBlock.childBlocks = children;
                  newBlocks[agentIdx] = agentBlock;
                  return { ...msg, blocks: newBlocks };
                }
              }

              // 顶层工具块匹配
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
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "approval_request": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = [...msg.blocks];
              newBlocks.push({
                type: "approval" as const,
                content: "",
                status: "pending_confirm" as const,
                confirmId: event.confirm_id,
                agentRole: event.agent_role,
                approvalKind: event.kind,
                approvalItems: event.items,
                approvalDescription: event.description,
                approvalSessionId: event.session_id,
              });
              return { ...msg, blocks: newBlocks };
            });
            if (updated) updateConversation(convId!, { messages: updated });

            if (document.hidden) {
              try {
                new Notification("OpsKat", {
                  body: i18n.t("ai.notificationPermissionNeeded"),
                  tag: `confirm-${event.confirm_id}`,
                });
              } catch {
                // 通知权限未授予，忽略
              }
            }
            break;
          }

          case "approval_result": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = msg.blocks.map((b) =>
                b.confirmId === event.confirm_id && b.status === "pending_confirm"
                  ? { ...b, status: event.content === "deny" ? ("error" as const) : ("running" as const) }
                  : b
              );
              return { ...msg, blocks: newBlocks };
            });
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "thinking_done": {
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = msg.blocks.map((b) =>
                b.type === "thinking" && b.status === "running" ? { ...b, status: "completed" as const } : b
              );
              return { ...msg, blocks: newBlocks };
            });
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "queue_consumed": {
            // 后端在工具调用间隙消费了一条排队消息
            // 结束当前 assistant 消息，插入 user 消息，开启新 assistant 流
            const nextMsgs = [...msgs];
            const lastIdx = nextMsgs.length - 1;
            if (lastIdx >= 0 && nextMsgs[lastIdx].role === "assistant") {
              nextMsgs[lastIdx] = { ...nextMsgs[lastIdx], streaming: false };
            }
            nextMsgs.push({
              role: "user" as const,
              content: event.content || "",
              blocks: [],
              streaming: false,
            });
            nextMsgs.push({
              role: "assistant" as const,
              content: "",
              blocks: [],
              streaming: true,
            });
            // 从本地队列头部移除已消费的消息
            const curQueue = get().conversationStreaming[convId!]?.pendingQueue || [];
            const newQueue = curQueue.length > 0 ? curQueue.slice(1) : [];
            updateConversation(convId!, { messages: nextMsgs, pendingQueue: newQueue });
            break;
          }

          case "stopped": {
            cleanupStreamBuffer(convId!);
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = msg.blocks.map((b) => {
                if (b.type === "tool" && b.status === "running") {
                  return { ...b, status: "cancelled" as const };
                }
                if (b.type === "thinking" && b.status === "running") {
                  return { ...b, status: "completed" as const };
                }
                if (b.type === "agent" && b.status === "running") {
                  return {
                    ...b,
                    status: "cancelled" as const,
                    childBlocks: b.childBlocks?.map((c) =>
                      c.status === "running" ? { ...c, status: "cancelled" as const } : c
                    ),
                  };
                }
                return b;
              });
              return { ...msg, blocks: newBlocks, streaming: false };
            });
            if (updated) {
              updateConversation(convId!, { messages: updated, sending: false });
            } else {
              updateConversation(convId!, { sending: false });
            }

            // 保存消息
            const finalMsgs = get().conversationMessages[convId!] || [];
            if (convId) {
              SaveConversationMessages(convId, toDisplayMessages(finalMsgs)).catch(() => {});
            }
            get().fetchConversations();

            // 消费队列
            drainQueue(tabId, convId!);
            break;
          }

          case "retry": {
            const updated = updateLastAssistant(msgs, (msg) => ({
              ...msg,
              blocks: appendText(msg.blocks, `\n\n*${i18n.t("ai.retrying", "重试中")} (${event.content})...*`),
            }));
            if (updated) updateConversation(convId!, { messages: updated });
            break;
          }

          case "done": {
            cleanupStreamBuffer(convId!);
            const updated = updateLastAssistant(msgs, (msg) => {
              const newBlocks = msg.blocks.map((b) =>
                b.type === "tool" && (b.status === "running" || b.status === "pending_confirm")
                  ? { ...b, status: "completed" as const }
                  : b
              );
              return { ...msg, blocks: newBlocks, streaming: false };
            });
            if (updated) {
              updateConversation(convId!, { messages: updated, sending: false });
            } else {
              updateConversation(convId!, { sending: false });
            }

            // Persist messages
            const finalMsgs = get().conversationMessages[convId!] || [];
            if (convId) {
              SaveConversationMessages(convId, toDisplayMessages(finalMsgs)).catch(() => {});
            }
            // Refresh conversations (title may have updated)
            get()
              .fetchConversations()
              .then(() => {
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

            // 消费队列
            drainQueue(tabId, convId!);
            break;
          }

          case "error": {
            cleanupStreamBuffer(convId!);
            const updated = updateLastAssistant(msgs, (msg) => ({
              ...msg,
              blocks: appendText(msg.blocks, `\n\n**Error:** ${event.error}`),
              streaming: false,
            }));
            if (updated) {
              updateConversation(convId!, { messages: updated, sending: false });
            } else {
              updateConversation(convId!, { sending: false });
            }
            break;
          }
        }
      });

      const apiMessages = newMessages.map((m) => {
        return new ai.Message({
          role: m.role,
          content: m.content,
        });
      });

      // 收集当前 Tab 上下文
      const allTabs = useTabStore.getState().tabs;
      const openTabs = allTabs
        .filter(
          (t): t is Tab & { meta: { assetId: number; assetName?: string } } =>
            t.type !== "ai" && t.type !== "page" && t.meta != null && "assetId" in t.meta
        )
        .map(
          (t) =>
            new ai.TabInfo({
              type: t.type,
              assetId: t.meta.assetId || 0,
              assetName: t.meta.assetName || t.label || "",
            })
        );
      const aiContext = new ai.AIContext({ openTabs });

      try {
        await SendAIMessage(convId!, apiMessages, aiContext);
      } catch {
        updateConversation(convId!, { sending: false });
        cleanupConvListener(convId!);
      }
    },

    // === 停止/重新生成/队列 ===

    stopGeneration: async (tabId: string) => {
      const tabStore = useTabStore.getState();
      const tab = tabStore.tabs.find((t) => t.id === tabId);
      if (!tab) return;
      const convId = (tab.meta as AITabMeta).conversationId;
      if (convId) {
        await StopAIGeneration(convId);
      }
    },

    regenerate: async (tabId: string, messageIndex: number) => {
      const tabState = get().tabStates[tabId];
      if (!tabState) return;

      // 正在生成时先停止
      if (tabState.sending) {
        await get().stopGeneration(tabId);
        await new Promise((r) => setTimeout(r, 200));
      }

      // 截断到指定消息之前
      const truncated = tabState.messages.slice(0, messageIndex);
      updateTab(tabId, { messages: truncated, sending: false, pendingQueue: [] });

      if (truncated.length === 0) return;

      // 用空内容触发 sendToTab（drain 模式，使用已有消息）
      await get().sendToTab(tabId, "");
    },

    removeFromQueue: (tabId: string, index: number) => {
      const tabState = get().tabStates[tabId];
      if (!tabState) return;
      const newQueue = tabState.pendingQueue.filter((_, i) => i !== index);
      updateTab(tabId, { pendingQueue: newQueue });
    },

    clearQueue: (tabId: string) => {
      updateTab(tabId, { pendingQueue: [] });
    },

    // === 查询 ===

    isAnySending: () => {
      const { tabStates } = get();
      return Object.values(tabStates).some((ts) => ts.sending);
    },

    getTabState: (tabId: string) => {
      return get().tabStates[tabId] || { messages: [], sending: false, pendingQueue: [] };
    },

    getMessagesByConversationId: (convId: number) => {
      return get().conversationMessages[convId] || [];
    },

    getStreamingByConversationId: (convId: number) => {
      return get().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
    },
  };
});

// === Close Hook: clean up when tabStore closes an AI tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "ai") return;

  const state = useAIStore.getState();
  const meta = tab.meta as AITabMeta;

  if (meta.conversationId) {
    const msgs = state.conversationMessages[meta.conversationId] || [];
    if (msgs.length) {
      SaveConversationMessages(meta.conversationId, toDisplayMessages(msgs)).catch(() => {});
    }
    cleanupConvListener(meta.conversationId);
  }

  useAIStore.setState((s) => {
    const { [tab.id]: _, ...newTabStates } = s.tabStates;
    return { tabStates: newTabStates };
  });
});

// === Restore Hook: load AI settings and restore conversation tabs ===

async function restoreAITabs(tabs: Tab[]) {
  try {
    const active = await GetActiveAIProvider();
    if (!active) {
      return;
    }
    useAIStore.setState({ configured: true });
  } catch {
    return;
  }

  const store = useAIStore.getState();
  await store.fetchConversations();

  if (tabs.length > 0) {
    const { conversations } = store;
    const tabStore = useTabStore.getState();
    for (const tab of tabs) {
      const meta = tab.meta as AITabMeta;
      if (meta.conversationId) {
        if (!conversations.some((c) => c.ID === meta.conversationId)) {
          tabStore.closeTab(tab.id);
          continue;
        }
        try {
          const displayMsgs = await SwitchConversation(meta.conversationId);
          const messages = convertDisplayMessages(displayMsgs);
          useAIStore.setState((s) => ({
            tabStates: { ...s.tabStates, [tab.id]: { messages, sending: false, pendingQueue: [] } },
          }));
          if (meta.conversationId) {
            useAIStore.setState((s) => ({
              conversationMessages: { ...s.conversationMessages, [meta.conversationId!]: messages },
              conversationStreaming: {
                ...s.conversationStreaming,
                [meta.conversationId!]: { sending: false, pendingQueue: [] },
              },
            }));
          }
          const conv = conversations.find((c) => c.ID === meta.conversationId);
          if (conv && conv.Title !== tab.label) {
            tabStore.updateTab(tab.id, { label: conv.Title, meta: { ...meta, title: conv.Title } });
          }
        } catch {
          tabStore.closeTab(tab.id);
        }
      } else {
        useAIStore.setState((s) => ({
          tabStates: { ...s.tabStates, [tab.id]: { messages: [], sending: false, pendingQueue: [] } },
        }));
      }
    }
  }
}

registerTabRestoreHook("ai", (tabs) => {
  restoreAITabs(tabs).catch(() => {});
});

// === AI Send on Enter 设置 ===

const SEND_ON_ENTER_KEY = "ai_send_on_enter";

export function getAISendOnEnter(): boolean {
  const val = localStorage.getItem(SEND_ON_ENTER_KEY);
  return val === null ? true : val === "true";
}

export function setAISendOnEnter(value: boolean) {
  localStorage.setItem(SEND_ON_ENTER_KEY, String(value));
  window.dispatchEvent(new Event("ai-send-on-enter-change"));
}

export function useAISendOnEnter(): boolean {
  const [value, setValue] = useState(getAISendOnEnter);
  useEffect(() => {
    const handler = () => setValue(getAISendOnEnter());
    window.addEventListener("ai-send-on-enter-change", handler);
    return () => window.removeEventListener("ai-send-on-enter-change", handler);
  }, []);
  return value;
}
