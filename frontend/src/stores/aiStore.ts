import { create } from "zustand";
import {
  SendAIMessage,
  SetAIProvider,
  DetectLocalCLIs,
  GetInitContext,
  ResetAISession,
  CreateConversation,
  ListConversations,
  SwitchConversation,
  DeleteConversation,
  SaveConversationMessages,
} from "../../wailsjs/go/main/App";
import { ai, conversation_entity, main } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useAssetStore } from "./assetStore";
import i18n from "../i18n";

// 内容块：文本或工具调用
export interface ContentBlock {
  type: "text" | "tool";
  content: string;
  // tool 类型专用
  toolName?: string;
  toolInput?: string;
  status?: "running" | "completed" | "error";
}

export interface ChatMessage {
  role: "user" | "assistant" | "tool";
  content: string; // 纯文本回退（用户消息 / 旧兼容）
  blocks: ContentBlock[]; // 结构化内容块
  streaming?: boolean;
}

interface StreamEventData {
  type: string;
  content?: string;
  tool_name?: string;
  tool_input?: string;
  error?: string;
}

interface AIState {
  messages: ChatMessage[];
  conversations: conversation_entity.Conversation[];
  currentConversationId: number | null;
  configured: boolean;
  sending: boolean;
  localCLIs: ai.CLIInfo[];

  configure: (
    providerType: string,
    apiBase: string,
    apiKey: string,
    model: string
  ) => Promise<void>;
  send: (content: string) => Promise<void>;
  detectCLIs: () => Promise<void>;
  clear: () => void;
  fetchConversations: () => Promise<void>;
  createConversation: () => Promise<void>;
  switchConversation: (id: number) => Promise<void>;
  deleteConversation: (id: number) => Promise<void>;
}

let cancelEventListener: (() => void) | null = null;
let eventGeneration = 0;

// 辅助：获取最后一条 assistant 消息并更新
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

// 辅助：追加文本到最后一个文本块，或创建新文本块
function appendText(blocks: ContentBlock[], text: string): ContentBlock[] {
  const newBlocks = [...blocks];
  const last = newBlocks[newBlocks.length - 1];
  if (last && last.type === "text") {
    newBlocks[newBlocks.length - 1] = { ...last, content: last.content + text };
  } else {
    newBlocks.push({ type: "text", content: text });
  }
  return newBlocks;
}

// 辅助：将显示消息转换为后端格式用于持久化
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

export const useAIStore = create<AIState>((set, get) => ({
  messages: [],
  conversations: [],
  currentConversationId: null,
  configured: false,
  sending: false,
  localCLIs: [],

  configure: async (providerType, apiBase, apiKey, model) => {
    await SetAIProvider(providerType, apiBase, apiKey, model);
    set({ configured: true });
  },

  fetchConversations: async () => {
    try {
      const convs = await ListConversations();
      set({ conversations: convs || [] });
    } catch {
      set({ conversations: [] });
    }
  },

  createConversation: async () => {
    try {
      const conv = await CreateConversation();
      set({
        currentConversationId: conv.ID,
        messages: [],
      });
      // 刷新列表
      get().fetchConversations();
    } catch (e) {
      console.error("创建会话失败:", e);
    }
  },

  switchConversation: async (id: number) => {
    if (get().sending) return;
    const state = get();

    // 保存当前会话消息
    if (state.currentConversationId && state.messages.length > 0) {
      SaveConversationMessages(toDisplayMessages(state.messages)).catch(
        () => {}
      );
    }

    // 取消旧的事件监听
    if (cancelEventListener) {
      cancelEventListener();
      cancelEventListener = null;
    }

    try {
      const displayMsgs = await SwitchConversation(id);
      const messages: ChatMessage[] = (displayMsgs || []).map(
        (dm: main.ConversationDisplayMessage) => ({
          role: dm.role as "user" | "assistant" | "tool",
          content: dm.content,
          blocks: (dm.blocks || []).map((b: conversation_entity.ContentBlock) => ({
            type: b.type as "text" | "tool",
            content: b.content,
            toolName: b.toolName,
            toolInput: b.toolInput,
            status: b.status as "running" | "completed" | "error" | undefined,
          })),
          streaming: false,
        })
      );
      set({ currentConversationId: id, messages });
    } catch (e) {
      console.error("切换会话失败:", e);
    }
  },

  deleteConversation: async (id: number) => {
    try {
      await DeleteConversation(id);
      const state = get();
      if (state.currentConversationId === id) {
        set({ currentConversationId: null, messages: [] });
      }
      get().fetchConversations();
    } catch (e) {
      console.error("删除会话失败:", e);
    }
  },

  send: async (content) => {
    const state = get();
    if (state.sending) return;

    let actualContent = content;

    // 拦截 /init 命令
    if (content.trim() === "/init") {
      const { selectedAssetId, selectedGroupId } = useAssetStore.getState();
      if (!selectedAssetId && !selectedGroupId) {
        set({
          messages: [
            ...state.messages,
            { role: "user", content: "/init", blocks: [] },
            {
              role: "assistant",
              content: i18n.t("ai.initNoSelection"),
              blocks: [
                { type: "text", content: i18n.t("ai.initNoSelection") },
              ],
              streaming: false,
            },
          ],
        });
        return;
      }
      try {
        actualContent = await GetInitContext(
          selectedAssetId || 0,
          selectedGroupId || 0
        );
      } catch (e) {
        const errMsg = `${i18n.t("ai.initError")}: ${e}`;
        set({
          messages: [
            ...state.messages,
            { role: "user", content: "/init", blocks: [] },
            {
              role: "assistant",
              content: errMsg,
              blocks: [{ type: "text", content: errMsg }],
              streaming: false,
            },
          ],
        });
        return;
      }
    }

    // 添加用户消息
    const displayContent = content.trim() === "/init" ? "/init" : content;
    const userMsg: ChatMessage = {
      role: "user",
      content: displayContent,
      blocks: [],
    };
    const newMessages = [...state.messages, userMsg];
    set({ messages: newMessages, sending: true });

    // 添加空的 assistant 消息（用于流式填充）
    const assistantMsg: ChatMessage = {
      role: "assistant",
      content: "",
      blocks: [],
      streaming: true,
    };
    set({ messages: [...newMessages, assistantMsg] });

    // 确保有会话ID（没有则先创建）
    let convId = state.currentConversationId;
    if (!convId) {
      try {
        const conv = await CreateConversation();
        convId = conv.ID;
        set({ currentConversationId: convId });
        get().fetchConversations();
      } catch (e) {
        set({ sending: false });
        return;
      }
    }

    // 在发送消息前设置事件监听（避免竞态）
    {
      const eventConvId = convId;
      const eventName = "ai:event:" + eventConvId;

      // 递增代数，防止旧监听器残留导致重复
      eventGeneration++;
      const myGeneration = eventGeneration;

      if (cancelEventListener) {
        cancelEventListener();
        cancelEventListener = null;
      }

      cancelEventListener = EventsOn(
        eventName,
        (event: StreamEventData) => {
          if (myGeneration !== eventGeneration) return;

          const msgs = get().messages;

          switch (event.type) {
            case "content": {
              const updated = updateLastAssistant(msgs, (msg) => ({
                ...msg,
                content: msg.content + (event.content || ""),
                blocks: appendText(msg.blocks, event.content || ""),
              }));
              if (updated) set({ messages: updated });
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
              if (updated) set({ messages: updated });
              break;
            }

            case "tool_result": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = [...msg.blocks];
                for (let i = newBlocks.length - 1; i >= 0; i--) {
                  const b = newBlocks[i];
                  if (
                    b.type === "tool" &&
                    b.status === "running" &&
                    b.toolName === event.tool_name
                  ) {
                    newBlocks[i] = {
                      ...b,
                      content: event.content || "",
                      status: "completed",
                    };
                    break;
                  }
                }
                return { ...msg, blocks: newBlocks };
              });
              if (updated) set({ messages: updated });
              break;
            }

            case "done": {
              const updated = updateLastAssistant(msgs, (msg) => {
                const newBlocks = msg.blocks.map((b) =>
                  b.type === "tool" && b.status === "running"
                    ? { ...b, status: "completed" as const }
                    : b
                );
                return { ...msg, blocks: newBlocks, streaming: false };
              });
              if (updated) set({ messages: updated, sending: false });
              else set({ sending: false });

              // 持久化消息
              const finalMsgs = get().messages;
              SaveConversationMessages(toDisplayMessages(finalMsgs)).catch(
                () => {}
              );
              // 刷新会话列表（标题可能已更新）
              get().fetchConversations();
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
              if (updated) set({ messages: updated, sending: false });
              else set({ sending: false });
              break;
            }
          }
        }
      );
    }

    // 转换为后端消息格式
    const apiMessages = newMessages.map((m, idx) => {
      const msgContent =
        idx === newMessages.length - 1 ? actualContent : m.content;
      return new ai.Message({
        role: m.role,
        content: msgContent,
      });
    });

    try {
      await SendAIMessage(apiMessages);
    } catch (e) {
      set({ sending: false });
      if (cancelEventListener) {
        cancelEventListener();
        cancelEventListener = null;
      }
    }
  },

  detectCLIs: async () => {
    const clis = await DetectLocalCLIs();
    set({ localCLIs: clis || [] });
  },

  clear: () => {
    const state = get();
    // 保存当前会话消息
    if (state.currentConversationId && state.messages.length > 0) {
      SaveConversationMessages(toDisplayMessages(state.messages)).catch(
        () => {}
      );
    }
    set({ messages: [], sending: false, currentConversationId: null });
    if (cancelEventListener) {
      cancelEventListener();
      cancelEventListener = null;
    }
    ResetAISession().catch(() => {});
  },
}));

// 应用启动时自动恢复 AI 配置
const providerType = localStorage.getItem("ai_provider_type");
if (providerType) {
  const apiBase = localStorage.getItem("ai_api_base") || "";
  const apiKey = localStorage.getItem("ai_api_key") || "";
  const model = localStorage.getItem("ai_model") || "";
  useAIStore
    .getState()
    .configure(providerType, apiBase, apiKey, model)
    .then(() => {
      // 配置完成后加载会话列表
      useAIStore.getState().fetchConversations();
    })
    .catch(() => {});
}
