import { useState, useEffect } from "react";
import { create } from "zustand";
import {
  SendAIMessage,
  StopAIGeneration,
  QueueAIMessage,
  GetActiveAIProvider,
  CreateConversation,
  ListConversations,
  LoadConversationMessages,
  DeleteConversation,
  SaveConversationMessages,
  UpdateConversationTitle,
} from "../../wailsjs/go/app/App";
import { ai, conversation_entity, app } from "../../wailsjs/go/models";
import { EventsOn, EventsEmit } from "../../wailsjs/runtime/runtime";
import i18n from "../i18n";
import { useTabStore, registerTabCloseHook, registerTabRestoreHook, type AITabMeta, type Tab } from "./tabStore";
import { useAssetStore } from "./assetStore";
import { buildGroupPathMap } from "@/lib/assetSearch";

// 用户消息中的资产引用（对应前端 @ 提及）
export interface MentionRef {
  assetId: number;
  name: string; // 发送时刻的资产名快照
  start: number; // content 中字符起始索引（含 @ 符号，JS 字符串索引）
  end: number; // 结束索引（不含）
}

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

// Assistant 消息累计 token 使用量；单次用户 turn 可能跨多轮 LLM 调用，前端按 usage 事件累加。
export interface TokenUsage {
  inputTokens?: number;
  outputTokens?: number;
  cacheCreationTokens?: number;
  cacheReadTokens?: number;
}

export interface ChatMessage {
  role: "user" | "assistant" | "tool";
  content: string;
  blocks: ContentBlock[];
  streaming?: boolean;
  mentions?: MentionRef[];
  tokenUsage?: TokenUsage;
}

export interface PendingQueueItem {
  text: string;
  mentions?: MentionRef[];
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
  // usage 事件：后端下发每轮 LLM 调用的 token 使用量
  usage?: {
    input_tokens?: number;
    output_tokens?: number;
    cache_creation_tokens?: number;
    cache_read_tokens?: number;
  };
}

interface TabState {
  // Phase 1 清理后：tabStates 保留为 UI 态占位（将来放 scrollTop / inputDraft 等），
  // messages/sending/pendingQueue 已全部迁移到 conversationMessages / conversationStreaming。
  _marker?: "tab-state";
}

// 模块级 per-conversation 事件监听管理（不放 zustand，因为含函数引用）
const conversationListeners = new Map<number, { cancel: (() => void) | null; generation: number }>();

// 每个 conversation 只保留一个待执行的落盘定时器。
// 流式输出期间会持续增量更新消息，如果每次都立即保存，磁盘写入会过于频繁。
const persistTimers = new Map<number, number>();

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
  cleanupPersistTimer(convId);
}

function invalidateConvListenerForReplay(convId: number) {
  const listener = getOrCreateConvListener(convId);
  // 先 bump generation，再解绑旧订阅，避免停流后的迟到事件回写到新分支。
  listener.generation++;
  if (listener.cancel) {
    listener.cancel();
    listener.cancel = null;
  }
  cleanupStreamBuffer(convId);
}

// cleanupPersistTimer 清理指定 conversation 尚未执行的落盘定时器。
function cleanupPersistTimer(convId: number) {
  const timer = persistTimers.get(convId);
  if (timer !== undefined) {
    window.clearTimeout(timer);
    persistTimers.delete(convId);
  }
}

// persistConversationSnapshot 将当前 conversation 的消息快照保存到持久化存储。
function persistConversationSnapshot(convId: number, includeStreaming = false) {
  const msgs = useAIStore.getState().conversationMessages[convId];
  if (!msgs) return;
  SaveConversationMessages(convId, toDisplayMessages(msgs, includeStreaming)).catch(() => {});
}

// schedulePersist 为 conversation 安排一次短延迟的消息快照保存。
// 流式输出期间做一次短延迟防抖：
// 1. 降低高频写入带来的性能开销；
// 2. 让应用异常退出前，尽量已经落下最近一段对话快照。
function schedulePersist(convId: number, includeStreaming = false) {
  cleanupPersistTimer(convId);
  const timer = window.setTimeout(() => {
    persistTimers.delete(convId);
    persistConversationSnapshot(convId, includeStreaming);
  }, 300);
  persistTimers.set(convId, timer);
}

// persistNow 取消 schedulePersist 的防抖定时器，立即落盘一次。
// 用于低频关键事件（用户发送、工具调用、审批等），避免防抖窗口内崩溃丢数据。
function persistNow(convId: number, includeStreaming = false) {
  cleanupPersistTimer(convId);
  persistConversationSnapshot(convId, includeStreaming);
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

    return {
      conversationMessages: { ...state.conversationMessages, [convId]: updated },
    };
  });

  // 增量内容刷入消息列表后，同步安排一次防抖落盘。
  schedulePersist(convId, true);
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

// 持久化 streaming 快照时需要归一化的中间态 → 终态。
// 应用异常退出后重启，这些 block 不会再收到后端事件来结束，保留 running/pending_confirm
// 会让 UI 上长期显示"运行中/待确认"的 spinner 而没有任何进展。统一归一化为 cancelled。
const STREAMING_SNAPSHOT_STATUS_OVERRIDES: Record<string, ContentBlock["status"]> = {
  running: "cancelled",
  pending_confirm: "cancelled",
};

function normalizeSnapshotStatus(status: ContentBlock["status"]): ContentBlock["status"] {
  if (!status) return status;
  return STREAMING_SNAPSHOT_STATUS_OVERRIDES[status] ?? status;
}

function toDisplayMessages(msgs: ChatMessage[], includeStreaming = false): app.ConversationDisplayMessage[] {
  return msgs
    .filter((m) => includeStreaming || !m.streaming)
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
                status: includeStreaming ? normalizeSnapshotStatus(b.status) : b.status,
              })
          ),
          mentions: (m.mentions || []).map(
            (mr) =>
              new conversation_entity.MentionRef({
                assetId: mr.assetId,
                name: mr.name,
                start: mr.start,
                end: mr.end,
              })
          ),
          tokenUsage: m.tokenUsage ? new conversation_entity.TokenUsage(m.tokenUsage) : undefined,
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
    mentions: (dm.mentions || []).map((mr: conversation_entity.MentionRef) => ({
      assetId: mr.assetId,
      name: mr.name,
      start: mr.start,
      end: mr.end,
    })),
    tokenUsage: dm.tokenUsage
      ? {
          inputTokens: dm.tokenUsage.inputTokens,
          outputTokens: dm.tokenUsage.outputTokens,
          cacheCreationTokens: dm.tokenUsage.cacheCreationTokens,
          cacheReadTokens: dm.tokenUsage.cacheReadTokens,
        }
      : undefined,
    streaming: false,
  }));
}

// 单次加载会话历史消息：仅当当前 store 尚未持有该 convId 的消息时才触发后端拉取。
// 侧边绑定 / localStorage 恢复走这条路径，避免侧边显示已有会话时"消息为空"的表现。
// 走 LoadConversationMessages（只读），不会修改后端 currentConversationID。
async function ensureConversationMessagesLoaded(convId: number) {
  if (useAIStore.getState().conversationMessages[convId] !== undefined) return;
  try {
    const displayMsgs = await LoadConversationMessages(convId);
    const messages = convertDisplayMessages(displayMsgs);
    useAIStore.setState((s) => ({
      conversationMessages:
        s.conversationMessages[convId] !== undefined
          ? s.conversationMessages
          : { ...s.conversationMessages, [convId]: messages },
      conversationStreaming: s.conversationStreaming[convId]
        ? s.conversationStreaming
        : { ...s.conversationStreaming, [convId]: { sending: false, pendingQueue: [] } },
    }));
  } catch {
    // ignore — 保持原状态；下次再绑定时会重试
  }
}

// 与后端标题规范保持一致，避免本地乐观更新和持久化后的标题不一致。
const DEFAULT_CONVERSATION_TITLE = "新对话";
const CONVERSATION_TITLE_MAX_CHARS = 50;

// 编辑首条消息时要先得到最终标题，供前端和后端复用同一套裁剪规则。
function buildConversationTitle(content: string) {
  const trimmed = content.trim();
  if (!trimmed) return DEFAULT_CONVERSATION_TITLE;
  const chars = Array.from(trimmed);
  if (chars.length <= CONVERSATION_TITLE_MAX_CHARS) return trimmed;
  return chars.slice(0, CONVERSATION_TITLE_MAX_CHARS).join("");
}

// 先同步当前内存里的会话列表和标签页标题，避免用户编辑后侧栏仍短暂显示旧标题。
function syncConversationTitleLocally(convId: number, title: string) {
  useAIStore.setState((state) => ({
    conversations: state.conversations.map((conv) => (conv.ID === convId ? { ...conv, Title: title } : conv)),
  }));

  const tabStore = useTabStore.getState();
  for (const tab of tabStore.tabs) {
    if (tab.type !== "ai") continue;
    const meta = tab.meta as AITabMeta;
    if (meta.conversationId !== convId) continue;
    tabStore.updateTab(tab.id, {
      label: title,
      meta: { ...meta, title },
    });
  }
}

// 所有“首条消息决定标题”的场景都走同一条同步链路，避免首次发送和编辑重发出现两套规则。
async function updateConversationTitleForMessage(convId: number, content: string) {
  const title = buildConversationTitle(content);
  syncConversationTitleLocally(convId, title);
  try {
    await UpdateConversationTitle(convId, title);
  } catch {
    // ignore — 下一次 fetchConversations 仍会尝试从后端刷新标题
  }
}

function shouldSyncConversationTitleBeforeSend(convId: number, content: string) {
  if (!content.trim()) return false;
  const streaming = useAIStore.getState().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
  if (streaming.sending) return false;
  const messages = useAIStore.getState().conversationMessages[convId] || [];
  return messages.length === 0;
}

// === 模块级 conversation 操作（被 sendToTab / sendFromSidebar / regenerate 等共用）===

function updateConversation(
  convId: number,
  updates: { messages?: ChatMessage[]; sending?: boolean; pendingQueue?: PendingQueueItem[] }
) {
  useAIStore.setState((state) => {
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

    return {
      conversationMessages: newConvMessages,
      conversationStreaming: newConvStreaming,
    };
  });

  if (updates.messages !== undefined) {
    // 消息列表发生变化后，补一轮防抖持久化。
    schedulePersist(convId, true);
  }
}

function drainQueue(convId: number) {
  const streaming = useAIStore.getState().conversationStreaming[convId];
  if (!streaming || streaming.pendingQueue.length === 0) return;

  const queue = [...streaming.pendingQueue];
  updateConversation(convId, { pendingQueue: [] });

  // 将队列中所有消息作为独立 user message 追加
  const currentMsgs = useAIStore.getState().conversationMessages[convId] || [];
  const newMsgs = [...currentMsgs];
  for (const item of queue) {
    newMsgs.push({
      role: "user" as const,
      content: item.text,
      mentions: item.mentions,
      blocks: [],
      streaming: false,
    });
  }
  updateConversation(convId, { messages: newMsgs });

  // 触发新一轮发送（空内容表示使用已有消息）
  setTimeout(() => {
    _sendForConversation(convId, "").catch(() => {});
  }, 0);
}

// replay 前先让旧 listener 和 pending queue 失效，避免 stop 产生的迟到事件回放到新分支。
function prepareConversationForReplay(convId: number) {
  invalidateConvListenerForReplay(convId);
  cleanupPersistTimer(convId);
  updateConversation(convId, { sending: false, pendingQueue: [] });
}

// 先把会话消息截断到编辑点之前，再以统一发送链路重建 assistant 占位消息。
function resetConversationForReplay(convId: number, messages: ChatMessage[]) {
  updateConversation(convId, { messages, sending: false, pendingQueue: [] });
}

// 把“编辑并重发”和“重新生成”统一到一条 replay 流程里，避免两套截断逻辑继续分叉。
async function replayConversation(
  convId: number,
  nextMessages: ChatMessage[],
  content: string,
  mentions?: MentionRef[]
) {
  const streaming = useAIStore.getState().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
  prepareConversationForReplay(convId);
  if (streaming.sending) {
    await useAIStore.getState().stopConversation(convId);
  }

  resetConversationForReplay(convId, nextMessages);

  if (content.trim()) {
    await _sendForConversation(convId, content, mentions);
    return;
  }

  if (nextMessages.length === 0) return;
  await _sendForConversation(convId, "");
}

// 事件处理：核心流式状态机，完全基于 convId
function handleStreamEvent(convId: number, event: StreamEventData) {
  const currentMsgs = useAIStore.getState().conversationMessages[convId] || [];
  if (!currentMsgs) return;

  // 高频事件缓冲：content/thinking 通过 RAF 合并，每帧最多一次状态更新
  if (event.type === "content" || event.type === "thinking") {
    const buf = getOrCreateStreamBuffer(convId);
    if (event.type === "content") {
      buf.content += event.content || "";
    } else {
      buf.thinking += event.content || "";
    }
    if (buf.raf === null) {
      buf.raf = requestAnimationFrame(() => {
        const b = streamBuffers.get(convId);
        if (b) b.raf = null;
        flushStreamBuffer(convId);
      });
    }
    return;
  }

  // 非流式事件：先刷新缓冲区保证顺序，再处理
  flushStreamBuffer(convId);
  const msgs = useAIStore.getState().conversationMessages[convId] || currentMsgs;

  switch (event.type) {
    case "usage": {
      if (!event.usage) break;
      const delta = event.usage;
      const updated = updateLastAssistant(msgs, (msg) => {
        const prev = msg.tokenUsage || {};
        const merged: TokenUsage = {
          inputTokens: (prev.inputTokens || 0) + (delta.input_tokens || 0),
          outputTokens: (prev.outputTokens || 0) + (delta.output_tokens || 0),
          cacheCreationTokens: (prev.cacheCreationTokens || 0) + (delta.cache_creation_tokens || 0),
          cacheReadTokens: (prev.cacheReadTokens || 0) + (delta.cache_read_tokens || 0),
        };
        return { ...msg, tokenUsage: merged };
      });
      if (updated) {
        updateConversation(convId, { messages: updated });
      }
      break;
    }

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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }

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
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
      break;
    }

    case "thinking_done": {
      const updated = updateLastAssistant(msgs, (msg) => {
        const newBlocks = msg.blocks.map((b) =>
          b.type === "thinking" && b.status === "running" ? { ...b, status: "completed" as const } : b
        );
        return { ...msg, blocks: newBlocks };
      });
      if (updated) {
        updateConversation(convId, { messages: updated });
        persistNow(convId, true);
      }
      break;
    }

    case "queue_consumed": {
      // 后端在工具调用间隙消费了一条排队消息
      // 结束当前 assistant 消息，插入 user 消息，开启新 assistant 流
      // event.content 为后端分离出的展示原文；mentions 从本地队列读取用于高亮 chip
      const curQueue = useAIStore.getState().conversationStreaming[convId]?.pendingQueue || [];
      const consumedItem = curQueue[0];
      const nextMsgs = [...msgs];
      const lastIdx = nextMsgs.length - 1;
      if (lastIdx >= 0 && nextMsgs[lastIdx].role === "assistant") {
        nextMsgs[lastIdx] = { ...nextMsgs[lastIdx], streaming: false };
      }
      nextMsgs.push({
        role: "user" as const,
        content: event.content || "",
        mentions: consumedItem?.mentions,
        blocks: [],
        streaming: false,
      });
      nextMsgs.push({
        role: "assistant" as const,
        content: "",
        blocks: [],
        streaming: true,
      });
      const newQueue = curQueue.length > 0 ? curQueue.slice(1) : [];
      updateConversation(convId, { messages: nextMsgs, pendingQueue: newQueue });
      // 排队消息被消费（插入新 user + 开启新 assistant）属于低频关键事件，立即落盘。
      persistNow(convId, true);
      break;
    }

    case "stopped": {
      cleanupStreamBuffer(convId);
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
        updateConversation(convId, { messages: updated, sending: false });
      } else {
        updateConversation(convId, { sending: false });
      }

      // 终态立即落盘
      cleanupPersistTimer(convId);
      persistConversationSnapshot(convId);
      useAIStore.getState().fetchConversations();

      // 消费队列
      drainQueue(convId);
      break;
    }

    case "retry": {
      const updated = updateLastAssistant(msgs, (msg) => ({
        ...msg,
        blocks: appendText(msg.blocks, `\n\n*${i18n.t("ai.retrying", "重试中")} (${event.content})...*`),
      }));
      if (updated) updateConversation(convId, { messages: updated });
      break;
    }

    case "done": {
      cleanupStreamBuffer(convId);
      const updated = updateLastAssistant(msgs, (msg) => {
        const newBlocks = msg.blocks.map((b) =>
          b.type === "tool" && (b.status === "running" || b.status === "pending_confirm")
            ? { ...b, status: "completed" as const }
            : b
        );
        return { ...msg, blocks: newBlocks, streaming: false };
      });
      if (updated) {
        updateConversation(convId, { messages: updated, sending: false });
      } else {
        updateConversation(convId, { sending: false });
      }

      // 终态立即落盘，保证标题刷新前后都能恢复到完整会话内容。
      cleanupPersistTimer(convId);
      persistConversationSnapshot(convId);
      // Refresh conversations (title may have updated); sync any open tab bound to this conv.
      useAIStore
        .getState()
        .fetchConversations()
        .then(() => {
          const convs = useAIStore.getState().conversations;
          const currentTab = useTabStore
            .getState()
            .tabs.find((t) => t.type === "ai" && (t.meta as AITabMeta).conversationId === convId);
          if (currentTab) {
            const meta = currentTab.meta as AITabMeta;
            const conv = convs.find((c) => c.ID === convId);
            if (conv && conv.Title !== currentTab.label) {
              useTabStore.getState().updateTab(currentTab.id, {
                label: conv.Title,
                meta: { ...meta, title: conv.Title },
              });
            }
          }
        });

      // 消费队列
      drainQueue(convId);
      break;
    }

    case "error": {
      cleanupStreamBuffer(convId);
      const updated = updateLastAssistant(msgs, (msg) => ({
        ...msg,
        blocks: appendText(msg.blocks, `\n\n**Error:** ${event.error}`),
        streaming: false,
      }));
      if (updated) {
        updateConversation(convId, { messages: updated, sending: false });
      } else {
        updateConversation(convId, { sending: false });
      }

      // 错误态同样需要落盘，否则强制重启后会丢掉最后一次失败上下文。
      cleanupPersistTimer(convId);
      persistConversationSnapshot(convId, true);
      break;
    }
  }
}

// 将一组 MentionRef 解析为后端使用的 MentionedAsset（查 assetStore，按 assetId 去重，资产已删除跳过）
function resolveMentionedAssets(mentions: MentionRef[] | undefined): ai.MentionedAsset[] {
  if (!mentions || mentions.length === 0) return [];
  const assetStore = useAssetStore.getState();
  const groupPathMap = buildGroupPathMap(assetStore.groups);
  const seen = new Set<number>();
  const out: ai.MentionedAsset[] = [];
  for (const mr of mentions) {
    if (seen.has(mr.assetId)) continue;
    seen.add(mr.assetId);
    const asset = assetStore.assets.find((a) => a.ID === mr.assetId);
    if (!asset) continue;
    let host = "";
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      host = cfg.host || "";
    } catch {
      /* ignore */
    }
    out.push(
      new ai.MentionedAsset({
        assetId: asset.ID,
        name: asset.Name,
        type: asset.Type,
        host,
        groupPath: asset.GroupID ? groupPathMap.get(asset.GroupID) || "" : "",
      })
    );
  }
  return out;
}

// 核心发送：完全基于 convId，共享给 sendToTab / sendFromSidebar / regenerate
async function _sendForConversation(convId: number, content: string, mentions?: MentionRef[]) {
  const state = useAIStore.getState();
  const streaming = state.conversationStreaming[convId] || { sending: false, pendingQueue: [] };

  // 生成中时排队：推送到后端 runner 队列 + 本地队列（用于 UI 显示）
  if (streaming.sending) {
    if (content.trim()) {
      updateConversation(convId, {
        pendingQueue: [...streaming.pendingQueue, { text: content.trim(), mentions }],
      });
      QueueAIMessage(convId, content.trim(), resolveMentionedAssets(mentions)).catch(() => {});
    }
    return;
  }

  // 空内容 = drain/regen 发送（消息已经在 state 中）
  const isDrainSend = !content.trim();
  const currentMsgs = state.conversationMessages[convId] || [];
  const newMessages = [...currentMsgs];

  if (isDrainSend) {
    if (newMessages.length === 0) return;
    updateConversation(convId, { sending: true });
  } else {
    newMessages.push({
      role: "user",
      content,
      mentions,
      blocks: [],
    });
    updateConversation(convId, { messages: newMessages, sending: true });
    // 用户消息是用户亲手输入的内容，最不应该丢；绕过 300ms 防抖立即落盘。
    // includeStreaming=true 保留历史未完成 block（如有），与 schedulePersist 的默认行为一致。
    persistNow(convId, true);
  }

  const assistantMsg: ChatMessage = {
    role: "assistant",
    content: "",
    blocks: [],
    streaming: true,
  };
  updateConversation(convId, {
    messages: [...newMessages, assistantMsg],
  });

  // Set up event listener (keyed by conversationId)
  const listener = getOrCreateConvListener(convId);
  listener.generation++;
  const myGeneration = listener.generation;

  if (listener.cancel) {
    listener.cancel();
    listener.cancel = null;
  }

  const eventName = `ai:event:${convId}`;
  listener.cancel = EventsOn(eventName, (event: StreamEventData) => {
    if (myGeneration !== listener.generation) return;
    handleStreamEvent(convId, event);
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

  // 收集所有 user 消息的 mentions（按 assetId 去重、资产已删除跳过）
  const allMentions: MentionRef[] = [];
  for (const m of newMessages) {
    if (m.role !== "user" || !m.mentions) continue;
    allMentions.push(...m.mentions);
  }
  const mentionedAssets = resolveMentionedAssets(allMentions);

  const aiContext = new ai.AIContext({ openTabs, mentionedAssets });

  try {
    await SendAIMessage(convId, apiMessages, aiContext);
  } catch {
    updateConversation(convId, { sending: false });
    cleanupConvListener(convId);
  }
}

// === Store ===

interface AIState {
  tabStates: Record<string, TabState>;
  conversationMessages: Record<number, ChatMessage[]>;
  conversationStreaming: Record<number, { sending: boolean; pendingQueue: PendingQueueItem[] }>;

  // 全局状态
  conversations: conversation_entity.Conversation[];
  configured: boolean;
  providerName: string;
  modelName: string;

  // 侧边助手状态
  sidebarConversationId: number | null;
  sidebarUIState: { inputDraft: string; scrollTop: number };

  // 配置
  checkConfigured: () => Promise<void>;

  // 发送
  send: (content: string, mentions?: MentionRef[]) => Promise<void>;
  sendToTab: (tabId: string, content: string, mentions?: MentionRef[]) => Promise<void>;
  sendFromSidebar: (convId: number, content: string, mentions?: MentionRef[]) => Promise<void>;
  editAndResendConversation: (
    convId: number,
    messageIndex: number,
    content: string,
    mentions?: MentionRef[]
  ) => Promise<void>;
  stopConversation: (convId: number) => Promise<void>;
  stopGeneration: (tabId: string) => Promise<void>;
  regenerate: (tabId: string, messageIndex: number) => Promise<void>;
  removeFromQueue: (convId: number, index: number) => void;
  clearQueue: (convId: number) => void;

  // Tab 管理 (delegates to tabStore)
  openConversationTab: (conversationId: number) => Promise<string>;
  openNewConversationTab: () => string;
  clear: () => void;

  // 会话管理
  fetchConversations: () => Promise<void>;
  deleteConversation: (id: number) => Promise<void>;

  // 侧边助手 actions
  bindSidebar: (conversationId: number | null) => void;
  promoteSidebarToTab: () => Promise<string | null>;
  createAndBindSidebarConversation: () => Promise<number>;
  validateSidebarConversation: () => void;
  setSidebarInputDraft: (draft: string) => void;

  // 查询
  isAnySending: () => boolean;
  getTabState: (tabId: string) => TabState;

  // NEW — 派生 getter
  getMessagesByConversationId: (convId: number) => ChatMessage[];
  getStreamingByConversationId: (convId: number) => { sending: boolean; pendingQueue: PendingQueueItem[] };
}

export const useAIStore = create<AIState>((set, get) => {
  return {
    tabStates: {},
    conversationMessages: {},
    conversationStreaming: {},

    conversations: [],
    configured: false,
    providerName: "",
    modelName: "",

    sidebarConversationId: (() => {
      const saved = localStorage.getItem("ai_sidebar_conversation_id");
      return saved ? parseInt(saved, 10) : null;
    })(),
    sidebarUIState: { inputDraft: localStorage.getItem("ai_sidebar_input_draft") || "", scrollTop: 0 },

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
        get().validateSidebarConversation();
        // 启动时 sidebarConversationId 从 localStorage 恢复，但消息未加载。
        // 在验证通过（conv 仍存在）后触发一次历史消息拉取。
        const sidebarId = get().sidebarConversationId;
        if (sidebarId != null) {
          void ensureConversationMessagesLoaded(sidebarId);
        }
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

        // 侧边绑定联动：若侧边仍持有该会话，清除之
        if (get().sidebarConversationId === id) {
          get().bindSidebar(null);
        }
        // 清理最近绑定记录，避免关闭 Tab 时误恢复已删除会话
        const lastBound = localStorage.getItem("ai_sidebar_last_bound");
        if (lastBound && parseInt(lastBound, 10) === id) {
          localStorage.removeItem("ai_sidebar_last_bound");
        }

        await get().fetchConversations();
      } catch (e) {
        console.error("删除会话失败:", e);
      }
    },

    // === 侧边助手 ===

    bindSidebar: (conversationId: number | null) => {
      set({ sidebarConversationId: conversationId });
      if (conversationId === null) {
        localStorage.removeItem("ai_sidebar_conversation_id");
      } else {
        localStorage.setItem("ai_sidebar_conversation_id", String(conversationId));
        localStorage.setItem("ai_sidebar_last_bound", String(conversationId));
        // 侧边绑定的会话需要立即把历史消息加载进来，否则 AIChatContent 读到空数组会显示"无消息"。
        // fire-and-forget：helper 内部做了"已加载则跳过"判断。
        void ensureConversationMessagesLoaded(conversationId);
      }
    },

    createAndBindSidebarConversation: async () => {
      const conv = await CreateConversation();
      set((state) => ({
        conversations: [conv, ...state.conversations],
        conversationMessages: { ...state.conversationMessages, [conv.ID]: [] },
        conversationStreaming: {
          ...state.conversationStreaming,
          [conv.ID]: { sending: false, pendingQueue: [] },
        },
      }));
      get().bindSidebar(conv.ID);
      return conv.ID;
    },

    promoteSidebarToTab: async () => {
      const convId = get().sidebarConversationId;
      if (convId == null) return null;
      const tabId = await get().openConversationTab(convId);
      return tabId;
    },

    validateSidebarConversation: () => {
      const convId = get().sidebarConversationId;
      if (convId == null) return;
      const exists = get().conversations.some((c) => c.ID === convId);
      if (!exists) {
        get().bindSidebar(null);
        localStorage.removeItem("ai_sidebar_last_bound");
      }
    },

    setSidebarInputDraft: (draft: string) => {
      set({ sidebarUIState: { ...get().sidebarUIState, inputDraft: draft } });
      localStorage.setItem("ai_sidebar_input_draft", draft);
    },

    // === Tab 管理 ===

    openConversationTab: async (conversationId: number) => {
      // Single-host invariant: evict sidebar when it holds the same conv
      if (get().sidebarConversationId === conversationId) {
        get().bindSidebar(null);
      }

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

      // Load messages（只读加载，避免后端 currentConversationID 被多 Tab 竞争覆盖）
      try {
        const displayMsgs = await LoadConversationMessages(conversationId);
        const messages = convertDisplayMessages(displayMsgs);

        // Open tab in tabStore
        tabStore.openTab({
          id: tabId,
          type: "ai",
          label: title,
          meta: { type: "ai", conversationId, title },
        });

        // Register empty UI placeholder entry + write to conversation-keyed stores
        set((state) => ({
          tabStates: {
            ...state.tabStates,
            [tabId]: {},
          },
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
          [tabId]: {},
        },
      }));

      return tabId;
    },

    // === 向后兼容 ===

    send: async (content: string, mentions?: MentionRef[]) => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find((t) => t.id === tabStore.activeTabId && t.type === "ai");
      if (!activeTab) {
        const newTabId = get().openNewConversationTab();
        await get().sendToTab(newTabId, content, mentions);
        return;
      }
      await get().sendToTab(activeTab.id, content, mentions);
    },

    clear: () => {
      const tabStore = useTabStore.getState();
      const activeTab = tabStore.tabs.find((t) => t.id === tabStore.activeTabId && t.type === "ai");
      if (activeTab) {
        tabStore.closeTab(activeTab.id);
      }
    },

    // === 核心发送 ===

    sendToTab: async (tabId: string, content: string, mentions?: MentionRef[]) => {
      const state = get();
      const tabState = state.tabStates[tabId];
      if (!tabState) return;

      const existingTab = useTabStore.getState().tabs.find((t) => t.id === tabId);
      if (!existingTab) return;
      let convId = (existingTab.meta as AITabMeta).conversationId;

      // drain 模式 + 没有历史消息时直接返回，避免创建空会话
      const existingMessages = convId != null ? state.conversationMessages[convId] || [] : [];
      if (!content.trim() && existingMessages.length === 0) return;

      // Ensure tab has a conversation ID *before* writing any message state —
      // conversationMessages / conversationStreaming are keyed by convId.
      let createdConversation = false;
      if (!convId) {
        try {
          const conv = await CreateConversation();
          convId = conv.ID;
          createdConversation = true;
          const curTab = useTabStore.getState().tabs.find((t) => t.id === tabId);
          useTabStore.getState().updateTab(tabId, {
            meta: { type: "ai", conversationId: convId, title: curTab?.label || "对话" },
          });
        } catch {
          return;
        }
      }

      if (shouldSyncConversationTitleBeforeSend(convId, content)) {
        await updateConversationTitleForMessage(convId, content);
      }
      if (createdConversation) {
        void get().fetchConversations();
      }

      await _sendForConversation(convId, content, mentions);
    },

    sendFromSidebar: async (convId: number, content: string, mentions?: MentionRef[]) => {
      if (shouldSyncConversationTitleBeforeSend(convId, content)) {
        await updateConversationTitleForMessage(convId, content);
      }
      await _sendForConversation(convId, content, mentions);
    },

    editAndResendConversation: async (
      convId: number,
      messageIndex: number,
      content: string,
      mentions?: MentionRef[]
    ) => {
      if (!content.trim()) return;

      const messages = get().conversationMessages[convId] || [];
      if (messages.length === 0) return;
      if (messageIndex < 0 || messageIndex >= messages.length) return;

      const target = messages[messageIndex];
      if (!target || target.role !== "user") return;

      const firstUserIndex = messages.findIndex((message) => message.role === "user");
      if (firstUserIndex === messageIndex) {
        // 首条 user message 同时决定会话标题，编辑后要先同步标题再 replay。
        await updateConversationTitleForMessage(convId, content);
      }

      const truncated = messages.slice(0, messageIndex);
      // 从被编辑消息之前的稳定历史重新发送，确保后续分支完全替换。
      await replayConversation(convId, truncated, content, mentions);
    },

    stopConversation: async (convId: number) => {
      try {
        await StopAIGeneration(convId);
      } catch {
        // ignore
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
      const tab = useTabStore.getState().tabs.find((t) => t.id === tabId);
      if (!tab) return;
      const convId = (tab.meta as AITabMeta).conversationId;
      if (convId == null) return;

      const messages = get().conversationMessages[convId] || [];
      if (messageIndex < 0 || messageIndex >= messages.length) return;

      const truncated = messages.slice(0, messageIndex);
      await replayConversation(convId, truncated, "");
    },

    removeFromQueue: (convId: number, index: number) => {
      const streaming = get().conversationStreaming[convId];
      if (!streaming) return;
      const newQueue = streaming.pendingQueue.filter((_, i) => i !== index);
      updateConversation(convId, { pendingQueue: newQueue });
    },

    clearQueue: (convId: number) => {
      updateConversation(convId, { pendingQueue: [] });
    },

    // === 查询 ===

    isAnySending: () => {
      const { conversationStreaming } = get();
      return Object.values(conversationStreaming).some((s) => s.sending);
    },

    getTabState: (tabId: string) => {
      return get().tabStates[tabId] || {};
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

  const meta = tab.meta as AITabMeta;

  if (meta.conversationId) {
    // 关闭标签前补一次最终快照，避免最后一段对话未落盘。
    // 关闭时流式输出可能仍在进行（streaming=true），必须传 includeStreaming=true 才能
    // 把最后一段 assistant 消息一起落盘；toDisplayMessages 会把未完结的 block 归一为 cancelled。
    cleanupPersistTimer(meta.conversationId);
    persistConversationSnapshot(meta.conversationId, true);
    cleanupConvListener(meta.conversationId);
  }

  useAIStore.setState((s) => {
    const { [tab.id]: _, ...newTabStates } = s.tabStates;
    return { tabStates: newTabStates };
  });

  // 单宿主不变量：若关闭的 Tab 对应的 conversation 是最近的侧边绑定，且当前侧边为空，
  // 则恢复侧边绑定，使得会话继续以侧边形态存在。
  if (meta.conversationId) {
    const lastBound = localStorage.getItem("ai_sidebar_last_bound");
    if (
      lastBound &&
      parseInt(lastBound, 10) === meta.conversationId &&
      useAIStore.getState().sidebarConversationId == null
    ) {
      useAIStore.getState().bindSidebar(meta.conversationId);
    }
  }
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
          const displayMsgs = await LoadConversationMessages(meta.conversationId);
          const messages = convertDisplayMessages(displayMsgs);
          useAIStore.setState((s) => ({
            tabStates: { ...s.tabStates, [tab.id]: {} },
            conversationMessages: { ...s.conversationMessages, [meta.conversationId!]: messages },
            conversationStreaming: {
              ...s.conversationStreaming,
              [meta.conversationId!]: { sending: false, pendingQueue: [] },
            },
          }));
          const conv = conversations.find((c) => c.ID === meta.conversationId);
          if (conv && conv.Title !== tab.label) {
            tabStore.updateTab(tab.id, { label: conv.Title, meta: { ...meta, title: conv.Title } });
          }
        } catch {
          tabStore.closeTab(tab.id);
        }
      } else {
        useAIStore.setState((s) => ({
          tabStates: { ...s.tabStates, [tab.id]: {} },
        }));
      }
    }
  }
}

registerTabRestoreHook("ai", (tabs) => {
  restoreAITabs(tabs).catch(() => {});
});

// === Shutdown Flush: 关窗前兜底落盘所有活跃会话 ===

// flushAllConversations 同步发起所有会话落盘（fire-and-forget）。
// beforeunload 场景用这个：页面即将销毁，没法 await Promise，只能靠 postMessage 已经送达 Go 端。
function flushAllConversations() {
  const allMsgs = useAIStore.getState().conversationMessages;
  for (const convIdStr of Object.keys(allMsgs)) {
    const convId = Number(convIdStr);
    if (!convId) continue;
    persistNow(convId, true);
  }
}

// flushAllConversationsAsync 等待所有会话落盘完成后返回。
// Wails OnBeforeClose 场景用这个：后端在等 ai:flush-done 回执再放行。
async function flushAllConversationsAsync(): Promise<void> {
  const allMsgs = useAIStore.getState().conversationMessages;
  const promises: Promise<unknown>[] = [];
  for (const convIdStr of Object.keys(allMsgs)) {
    const convId = Number(convIdStr);
    if (!convId) continue;
    cleanupPersistTimer(convId);
    const msgs = allMsgs[convId];
    if (!msgs) continue;
    promises.push(SaveConversationMessages(convId, toDisplayMessages(msgs, true)).catch(() => {}));
  }
  await Promise.allSettled(promises);
}

// beforeunload 在 Wails v2 WebView 里通常不会触发（WebView 被直接销毁，不走 navigation 卸载流程），
// 但保留作为兜底：极端场景（开发者工具刷新、devserver 热更新、WebView 异常重启）下仍可能派发。
// 主路径是 OnBeforeClose 的 ai:flush-all/ai:flush-done ack 机制，不要依赖 beforeunload。
if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", flushAllConversations);
}

// Wails OnBeforeClose 会 emit ai:flush-all，前端等所有 IPC 回执到位后
// 再 emit ai:flush-done 通知后端放行，避免后端暴力 sleep。
EventsOn("ai:flush-all", () => {
  flushAllConversationsAsync().finally(() => {
    EventsEmit("ai:flush-done");
  });
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
