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
import { stripMentionTags } from "@/lib/mentionXml";

// 内容块：文本、工具调用、Sub Agent 或审批
export interface ContentBlock {
  type: "text" | "tool" | "agent" | "approval" | "thinking";
  content: string;
  toolName?: string;
  toolInput?: string;
  toolCallId?: string; // 跨 turn 还原 tool_calls 历史；老数据无此字段，发送时退化为塌缩消息
  status?: "running" | "completed" | "error" | "pending_confirm" | "cancelled";
  confirmId?: string;
  // agent 块专用
  agentRole?: string;
  agentTask?: string;
  childBlocks?: ContentBlock[];
  // approval 块专用
  approvalKind?: "single" | "batch" | "grant" | "local_tool";
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
  approvalToolName?: string; // local_tool: "bash" | "write" | "edit"
  approvalPatterns?: string[]; // local_tool: 默认 pattern 列表，本次会话允许时预填可编辑
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
  tokenUsage?: TokenUsage;
}

export interface PendingQueueItem {
  text: string;
}

interface StreamEventData {
  type: string;
  content?: string;
  tool_name?: string;
  tool_input?: string;
  tool_call_id?: string;
  confirm_id?: string;
  error?: string;
  agent_role?: string;
  agent_task?: string;
  // approval_request 专用
  kind?: "single" | "batch" | "grant" | "local_tool";
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
  patterns?: string[]; // local_tool: 默认 pattern 列表，前端在"本次会话允许"时预填到可编辑文本框
  // usage 事件：后端下发每轮 LLM 调用的 token 使用量
  usage?: {
    input_tokens?: number;
    output_tokens?: number;
    cache_creation_tokens?: number;
    cache_read_tokens?: number;
  };
}

// Sidebar 专用 UI 态（inputDraft / scrollTop / editTarget）。workspace tab 不使用这些字段，
// 工作区 tab 只在 tabStates 里放一个空占位对象标记 "该 tab 已注册"。
interface SidebarTabUIState {
  inputDraft: {
    content: string;
  };
  scrollTop: number;
  editTarget: {
    conversationId: number;
    messageIndex: number;
    draft: {
      content: string;
    };
  } | null;
}

export interface SidebarAITab {
  id: string;
  conversationId: number | null;
  title: string;
  createdAt: number;
  uiState: SidebarTabUIState;
}

export type SidebarTabStatus = "waiting_approval" | "error" | "running" | "done" | null;

const SIDEBAR_TABS_STORAGE_KEY = "ai_sidebar_tabs";
const SIDEBAR_ACTIVE_TAB_STORAGE_KEY = "ai_sidebar_active_tab_id";
const LEGACY_SIDEBAR_CONVERSATION_KEY = "ai_sidebar_conversation_id";
const LEGACY_SIDEBAR_INPUT_DRAFT_KEY = "ai_sidebar_input_draft";
const LEGACY_SIDEBAR_LAST_BOUND_KEY = "ai_sidebar_last_bound";

function createDefaultSidebarUiState(overrides?: Partial<SidebarTabUIState> | null): SidebarTabUIState {
  return {
    inputDraft: {
      content: overrides?.inputDraft?.content ?? "",
    },
    scrollTop: typeof overrides?.scrollTop === "number" ? overrides.scrollTop : 0,
    editTarget: overrides?.editTarget
      ? {
          conversationId: overrides.editTarget.conversationId,
          messageIndex: overrides.editTarget.messageIndex,
          draft: {
            content: overrides.editTarget.draft.content ?? "",
          },
        }
      : null,
  };
}

// 选项里 activate 明确为 false 才保留旧 active，否则把 active 切到新 tab。
function resolveNextActiveId(prevActive: string | null, candidate: string, activate: boolean | undefined): string {
  return activate === false && prevActive ? prevActive : candidate;
}

// 统一的"如果 host 没 conversation 就新建一个"小助手：只负责 CreateConversation + try/catch。
// 更新 store 的工作交给 attach 回调——sendToTab 需要更新 workspace tab meta，
// sendFromSidebarTab 需要更新 sidebar tab + 预置 conversations/messages/streaming。
async function createConversationForEmptyHost(
  attach: (conv: conversation_entity.Conversation) => void
): Promise<number | null> {
  try {
    const conv = await CreateConversation();
    attach(conv);
    return conv.ID;
  } catch {
    return null;
  }
}

function getDefaultSidebarTitle() {
  return i18n.t("ai.newConversation", "新对话");
}

function createSidebarTabId() {
  return `sidebar-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function createSidebarTab(overrides?: Partial<SidebarAITab>): SidebarAITab {
  return {
    id: overrides?.id ?? createSidebarTabId(),
    conversationId: overrides?.conversationId ?? null,
    title: overrides?.title ?? getDefaultSidebarTitle(),
    createdAt: overrides?.createdAt ?? Date.now(),
    uiState: createDefaultSidebarUiState(overrides?.uiState),
  };
}

// localStorage 是不可信存储，回填草稿时把 <mention> 标签降级为内部纯文本，
// 防止用户或第三方篡改 storage 后伪造发送时的资产引用。
function sanitizeSidebarUiStateForPersistence(
  uiState: Partial<SidebarTabUIState> | null | undefined
): SidebarTabUIState | undefined {
  if (!uiState) return undefined;
  return createDefaultSidebarUiState({
    ...uiState,
    inputDraft: {
      content: stripMentionTags(uiState.inputDraft?.content ?? ""),
    },
    editTarget: uiState.editTarget
      ? {
          conversationId: uiState.editTarget.conversationId,
          messageIndex: uiState.editTarget.messageIndex,
          draft: {
            content: stripMentionTags(uiState.editTarget.draft.content ?? ""),
          },
        }
      : null,
  });
}

function sanitizeSidebarTab(raw: unknown): SidebarAITab | null {
  if (!raw || typeof raw !== "object") return null;
  const tab = raw as Partial<SidebarAITab>;
  if (typeof tab.id !== "string" || tab.id.length === 0) return null;
  const conversationId =
    typeof tab.conversationId === "number" && Number.isFinite(tab.conversationId) ? tab.conversationId : null;
  return createSidebarTab({
    id: tab.id,
    conversationId,
    title: typeof tab.title === "string" && tab.title.length > 0 ? tab.title : undefined,
    createdAt: typeof tab.createdAt === "number" && Number.isFinite(tab.createdAt) ? tab.createdAt : undefined,
    uiState: sanitizeSidebarUiStateForPersistence(tab.uiState),
  });
}

function clearLegacySidebarKeys() {
  localStorage.removeItem(LEGACY_SIDEBAR_CONVERSATION_KEY);
  localStorage.removeItem(LEGACY_SIDEBAR_INPUT_DRAFT_KEY);
  localStorage.removeItem(LEGACY_SIDEBAR_LAST_BOUND_KEY);
}

function persistSidebarTabs(tabs: SidebarAITab[], activeSidebarTabId: string | null) {
  if (tabs.length === 0) {
    localStorage.removeItem(SIDEBAR_TABS_STORAGE_KEY);
    localStorage.removeItem(SIDEBAR_ACTIVE_TAB_STORAGE_KEY);
    clearLegacySidebarKeys();
    return;
  }
  localStorage.setItem(
    SIDEBAR_TABS_STORAGE_KEY,
    JSON.stringify(
      tabs.map((tab) => ({
        ...tab,
        uiState: sanitizeSidebarUiStateForPersistence(tab.uiState),
      }))
    )
  );
  localStorage.setItem(SIDEBAR_ACTIVE_TAB_STORAGE_KEY, activeSidebarTabId ?? tabs[0].id);
  clearLegacySidebarKeys();
}

function loadInitialSidebarState() {
  const storedTabs = localStorage.getItem(SIDEBAR_TABS_STORAGE_KEY);
  const storedActive = localStorage.getItem(SIDEBAR_ACTIVE_TAB_STORAGE_KEY);
  if (storedTabs) {
    try {
      const parsed = JSON.parse(storedTabs);
      const tabs = Array.isArray(parsed)
        ? parsed.map(sanitizeSidebarTab).filter((tab): tab is SidebarAITab => !!tab)
        : [];
      const activeSidebarTabId = tabs.some((tab) => tab.id === storedActive) ? storedActive : (tabs[0]?.id ?? null);
      return {
        tabs,
        activeSidebarTabId,
        migratedLegacy: false,
      };
    } catch {
      // ignore invalid persisted data and fall back to legacy migration
    }
  }

  const legacyConversation = localStorage.getItem(LEGACY_SIDEBAR_CONVERSATION_KEY);
  const legacyLastBound = localStorage.getItem(LEGACY_SIDEBAR_LAST_BOUND_KEY);
  const legacyDraft = localStorage.getItem(LEGACY_SIDEBAR_INPUT_DRAFT_KEY) || "";
  const parsedConversationId = legacyConversation ? Number.parseInt(legacyConversation, 10) : Number.NaN;
  const conversationId = Number.isFinite(parsedConversationId) ? parsedConversationId : null;
  const hasLegacyState =
    legacyConversation !== null ||
    legacyLastBound !== null ||
    localStorage.getItem(LEGACY_SIDEBAR_INPUT_DRAFT_KEY) !== null;

  // 从旧版单 sidebar key 平滑迁移到多 tab 模型：
  // 只要检测到任意旧 key，就恢复成一个初始侧边 tab，避免升级后直接丢上下文。
  if (!hasLegacyState) {
    return {
      tabs: [] as SidebarAITab[],
      activeSidebarTabId: null as string | null,
      migratedLegacy: false,
    };
  }

  const initialTab = createSidebarTab({
    conversationId,
    title: conversationId == null ? getDefaultSidebarTitle() : undefined,
    uiState: createDefaultSidebarUiState({
      inputDraft: {
        content: legacyDraft,
      },
    }),
  });

  return {
    tabs: [initialTab],
    activeSidebarTabId: initialTab.id,
    migratedLegacy: true,
  };
}

// 模块级 per-conversation 事件监听管理（不放 zustand，因为含函数引用）
const conversationListeners = new Map<number, { cancel: (() => void) | null; generation: number }>();
const conversationStopRequests = new Set<number>();

const conversationRenameVersions = new Map<number, number>();
const conversationRenameInFlight = new Set<number>();
let conversationListBarrier = 0;
let conversationListRequestId = 0;
let latestAppliedConversationListRequestId = 0;

type ConversationListRequestToken = {
  requestId: number;
  barrier: number;
};

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
  conversationStopRequests.delete(convId);
  cleanupStreamBuffer(convId);
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

// persistConversationSnapshot 将当前 conversation 的消息快照保存到持久化存储。
function persistConversationSnapshot(convId: number, includeStreaming = false) {
  const msgs = useAIStore.getState().conversationMessages[convId];
  if (!msgs) return;
  SaveConversationMessages(convId, toDisplayMessages(msgs, includeStreaming)).catch(() => {});
}

// persistNow 立即落盘一次。
// 用于低频关键事件（用户发送、工具调用、审批、终态等）。
function persistNow(convId: number, includeStreaming = false) {
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

function beginConversationRenameVersion(convId: number) {
  const nextVersion = (conversationRenameVersions.get(convId) || 0) + 1;
  conversationRenameVersions.set(convId, nextVersion);
  return nextVersion;
}

function isLatestConversationRename(convId: number, version: number) {
  return conversationRenameVersions.get(convId) === version;
}

function invalidateConversationListRequests() {
  conversationListBarrier += 1;
  return conversationListBarrier;
}

function beginConversationListRequest(): ConversationListRequestToken {
  conversationListRequestId += 1;
  return { requestId: conversationListRequestId, barrier: conversationListBarrier };
}

function canApplyConversationListRequest(request: ConversationListRequestToken) {
  if (request.barrier !== conversationListBarrier) {
    return false;
  }
  return request.requestId >= latestAppliedConversationListRequestId;
}

function markConversationListRequestApplied(request: ConversationListRequestToken) {
  latestAppliedConversationListRequestId = request.requestId;
}

function applyConversationListSnapshot(convs: conversation_entity.Conversation[]) {
  useAIStore.setState({ conversations: convs || [] });
  useAIStore.getState().validateSidebarTabs();
  const sidebarConversationIds = Array.from(
    new Set(
      useAIStore
        .getState()
        .sidebarTabs.map((tab) => tab.conversationId)
        .filter((convId): convId is number => convId != null)
    )
  );
  for (const convId of sidebarConversationIds) {
    void ensureConversationMessagesLoaded(convId);
  }
}

// 先同步当前内存里的会话列表和标签页标题，避免用户编辑后侧栏仍短暂显示旧标题。
function syncConversationTitleLocally(convId: number, title: string) {
  useAIStore.setState((state) => ({
    conversations: state.conversations.map((conv) => (conv.ID === convId ? { ...conv, Title: title } : conv)),
    sidebarTabs: state.sidebarTabs.map((tab) => (tab.conversationId === convId ? { ...tab, title } : tab)),
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

function getConversationTitleFallback(convId: number) {
  const sidebarTabTitle = useAIStore.getState().sidebarTabs.find((tab) => tab.conversationId === convId)?.title;
  if (sidebarTabTitle) return sidebarTabTitle;

  const tab = useTabStore
    .getState()
    .tabs.find((item) => item.type === "ai" && (item.meta as AITabMeta).conversationId === convId);
  if (!tab) return DEFAULT_CONVERSATION_TITLE;
  const meta = tab.meta as AITabMeta;
  return meta.title || tab.label || DEFAULT_CONVERSATION_TITLE;
}

function shouldAutoRenameAfterFirstTurnEdit(convId: number, previousFirstUserContent: string) {
  const currentTitle =
    useAIStore.getState().conversations.find((conv) => conv.ID === convId)?.Title ??
    getConversationTitleFallback(convId);
  return currentTitle === buildConversationTitle(previousFirstUserContent);
}

async function refreshConversationListAfterRename(convId: number, renameVersion: number) {
  try {
    const refreshRequest = beginConversationListRequest();
    const convs = await ListConversations();
    if (!isLatestConversationRename(convId, renameVersion)) {
      return;
    }
    if (!canApplyConversationListRequest(refreshRequest)) {
      return;
    }
    if (!Array.isArray(convs)) {
      return;
    }
    markConversationListRequestApplied(refreshRequest);
    applyConversationListSnapshot(convs || []);
  } catch (refreshError) {
    if (isLatestConversationRename(convId, renameVersion)) {
      console.warn("刷新会话列表失败，保留本地重命名结果:", refreshError);
    }
  }
}

async function renameConversationInternal(
  convId: number,
  title: string,
  options: { waitForListRefresh?: boolean } = {}
) {
  const normalizedTitle = buildConversationTitle(title);
  const previousTitle = useAIStore.getState().conversations.find((conv) => conv.ID === convId)?.Title;
  const rollbackTitle = previousTitle ?? getConversationTitleFallback(convId);
  if (previousTitle === normalizedTitle) {
    return true;
  }
  if (conversationRenameInFlight.has(convId)) {
    return false;
  }
  conversationRenameInFlight.add(convId);
  const renameVersion = beginConversationRenameVersion(convId);

  // 显式重命名先做本地乐观更新，避免历史列表 / 上下文条 / 已打开宿主短暂显示旧标题。
  invalidateConversationListRequests();
  syncConversationTitleLocally(convId, normalizedTitle);

  try {
    await UpdateConversationTitle(convId, normalizedTitle);
    if (!isLatestConversationRename(convId, renameVersion)) {
      return true;
    }
    const refreshPromise = refreshConversationListAfterRename(convId, renameVersion);
    if (options.waitForListRefresh === false) {
      void refreshPromise;
      return true;
    }
    await refreshPromise;
    return true;
  } catch (error) {
    if (isLatestConversationRename(convId, renameVersion)) {
      syncConversationTitleLocally(convId, rollbackTitle);
      console.error("重命名会话失败:", error);
    }
    return false;
  } finally {
    conversationRenameInFlight.delete(convId);
  }
}

function visitBlockStatuses(blocks: ContentBlock[] | undefined, visitor: (status: ContentBlock["status"]) => boolean) {
  for (const block of blocks || []) {
    if (visitor(block.status)) return true;
    if (visitBlockStatuses(block.childBlocks, visitor)) return true;
  }
  return false;
}

function getConversationStatus(convId: number | null): SidebarTabStatus {
  if (convId == null) return null;
  const state = useAIStore.getState();
  const messages = state.conversationMessages[convId] || [];
  const streaming = state.conversationStreaming[convId] || { sending: false, pendingQueue: [] };
  let lastAssistantMessage: (typeof messages)[number] | undefined;
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const message = messages[i];
    if (message.role === "assistant") {
      lastAssistantMessage = message;
      break;
    }
  }
  if (!lastAssistantMessage && messages.length === 0) return null;

  if (visitBlockStatuses(lastAssistantMessage?.blocks, (status) => status === "pending_confirm")) {
    return "waiting_approval";
  }
  if (visitBlockStatuses(lastAssistantMessage?.blocks, (status) => status === "error")) {
    return "error";
  }
  if (
    streaming.sending ||
    !!lastAssistantMessage?.streaming ||
    visitBlockStatuses(lastAssistantMessage?.blocks, (status) => status === "running")
  ) {
    return "running";
  }
  return messages.length > 0 ? "done" : null;
}

function getConversationHostCount(convId: number, options?: { ignoreSidebarTabId?: string; ignoreMainTabId?: string }) {
  const sidebarHostCount = useAIStore
    .getState()
    .sidebarTabs.filter((tab) => tab.id !== options?.ignoreSidebarTabId && tab.conversationId === convId).length;
  const mainTabHostCount = useTabStore
    .getState()
    .tabs.filter(
      (tab) =>
        tab.id !== options?.ignoreMainTabId && tab.type === "ai" && (tab.meta as AITabMeta).conversationId === convId
    ).length;
  return sidebarHostCount + mainTabHostCount;
}

function hasConversationHosts(convId: number, options?: { ignoreSidebarTabId?: string; ignoreMainTabId?: string }) {
  return getConversationHostCount(convId, options) > 0;
}

function cleanupConversationIfUnused(
  convId: number,
  options?: { ignoreSidebarTabId?: string; ignoreMainTabId?: string }
) {
  if (hasConversationHosts(convId, options)) {
    conversationStopRequests.delete(convId);
    return;
  }

  const streaming = useAIStore.getState().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
  if (streaming.sending) {
    // 最后一个宿主关闭/改绑时，不能立刻解绑 listener；
    // 否则 stop/done 等迟到事件会丢失，最后一段响应和终态快照也无法落盘。
    if (!conversationStopRequests.has(convId)) {
      conversationStopRequests.add(convId);
      StopAIGeneration(convId).catch(() => {
        conversationStopRequests.delete(convId);
      });
    }
    return;
  }

  cleanupConvListener(convId);
}

// 所有“首条消息决定标题”的场景都走同一条同步链路，避免首次发送和编辑重发出现两套规则。
async function updateConversationTitleForMessage(convId: number, content: string) {
  await renameConversationInternal(convId, content, { waitForListRefresh: false });
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
}

function drainQueue(convId: number) {
  const streaming = useAIStore.getState().conversationStreaming[convId];
  if (!streaming || streaming.pendingQueue.length === 0) return;
  if (!hasConversationHosts(convId)) return;

  const queue = [...streaming.pendingQueue];
  updateConversation(convId, { pendingQueue: [] });

  // 将队列中所有消息作为独立 user message 追加
  const currentMsgs = useAIStore.getState().conversationMessages[convId] || [];
  const newMsgs = [...currentMsgs];
  for (const item of queue) {
    newMsgs.push({
      role: "user" as const,
      content: item.text,
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
  conversationStopRequests.delete(convId);
  invalidateConvListenerForReplay(convId);
  updateConversation(convId, { sending: false, pendingQueue: [] });
}

// 先把会话消息截断到编辑点之前，再以统一发送链路重建 assistant 占位消息。
function resetConversationForReplay(convId: number, messages: ChatMessage[]) {
  updateConversation(convId, { messages, sending: false, pendingQueue: [] });
}

// 把“编辑并重发”和“重新生成”统一到一条 replay 流程里，避免两套截断逻辑继续分叉。
async function replayConversation(convId: number, nextMessages: ChatMessage[], content: string) {
  const streaming = useAIStore.getState().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
  prepareConversationForReplay(convId);
  if (streaming.sending) {
    await useAIStore.getState().stopConversation(convId);
  }

  resetConversationForReplay(convId, nextMessages);

  if (content.trim()) {
    await _sendForConversation(convId, content);
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
          toolCallId: event.tool_call_id,
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
        // 匹配优先级：toolCallId 精确匹配 > toolName + running > 任意 running
        const findToolMatch = (arr: ContentBlock[]): number => {
          if (event.tool_call_id) {
            for (let i = arr.length - 1; i >= 0; i--) {
              if (arr[i].type === "tool" && arr[i].toolCallId === event.tool_call_id) return i;
            }
          }
          for (let i = arr.length - 1; i >= 0; i--) {
            const b = arr[i];
            if (b.type === "tool" && b.status === "running" && b.toolName === event.tool_name) return i;
          }
          for (let i = arr.length - 1; i >= 0; i--) {
            if (arr[i].type === "tool" && arr[i].status === "running") return i;
          }
          return -1;
        };

        if (agentIdx !== -1 && newBlocks[agentIdx].childBlocks) {
          const agentBlock = { ...newBlocks[agentIdx] };
          const children = [...(agentBlock.childBlocks || [])];
          const matchIdx = findToolMatch(children);
          if (matchIdx !== -1) {
            children[matchIdx] = { ...children[matchIdx], content: event.content || "", status: "completed" };
            agentBlock.childBlocks = children;
            newBlocks[agentIdx] = agentBlock;
            return { ...msg, blocks: newBlocks };
          }
        }

        // 顶层工具块匹配
        const matchIdx = findToolMatch(newBlocks);
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
          approvalToolName: event.tool_name,
          approvalPatterns: event.patterns,
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
      // 结束当前 assistant 消息，插入 user 消息（含内联 <mention> XML），开启新 assistant 流
      //
      // 注意：cago 一次 drainSteer 可能批量消费多条 Steer，连续 emit N 个
      // queue_consumed 事件——LLM 此时还没开始新一轮，前一个新开的 assistant
      // 气泡是空壳。若直接续 streaming:false 留下，UI 会出现 N-1 个空气泡。
      // 因此遇到"前一个 assistant 没有任何内容"时直接丢弃它再插 user + 新流。
      const curQueue = useAIStore.getState().conversationStreaming[convId]?.pendingQueue || [];
      let nextMsgs = [...msgs];
      const lastIdx = nextMsgs.length - 1;
      if (lastIdx >= 0 && nextMsgs[lastIdx].role === "assistant") {
        const last = nextMsgs[lastIdx];
        const isEmpty = !last.content && (!last.blocks || last.blocks.length === 0);
        if (isEmpty) {
          nextMsgs = nextMsgs.slice(0, lastIdx);
        } else {
          nextMsgs[lastIdx] = { ...last, streaming: false };
        }
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
      persistConversationSnapshot(convId);
      useAIStore.getState().fetchConversations();

      if (hasConversationHosts(convId)) {
        drainQueue(convId);
      } else {
        cleanupConversationIfUnused(convId);
      }
      break;
    }

    case "retry": {
      const reason = event.error ? `: ${event.error}` : "";
      const updated = updateLastAssistant(msgs, (msg) => ({
        ...msg,
        blocks: appendText(msg.blocks, `\n\n*${i18n.t("ai.retrying", "重试中")} (${event.content})${reason}*`),
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

      if (hasConversationHosts(convId)) {
        drainQueue(convId);
      } else {
        cleanupConversationIfUnused(convId);
      }
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
      persistConversationSnapshot(convId, true);
      cleanupConversationIfUnused(convId);
      break;
    }
  }
}

// 把前端塌缩的 ChatMessage 还原成 OpenAI/Anthropic 标准的多条 LLM 消息：
//   - 一次 user turn 对应一条 ChatMessage(assistant)，blocks 顺序为
//     thinking_n -> tool_n(含 input/result) -> ... -> text(最终回复)
//   - 展开后变成：assistant(thinking + tool_calls) + tool(result) + ... + assistant(text)
//   - 前置约束：tool block 必须带 toolCallId 才能展开；缺失的（旧数据）直接忽略 tool 块，回退到塌缩
//
// 这样跨 turn 时 DeepSeek/OpenAI 能看到上一 turn 的中间 tool_calls 与结果，
// 同时也满足 DeepSeek thinking 模式"带 tool_calls 的 assistant 必须回传 reasoning_content"的强制要求。
function expandToAPIMessages(messages: ChatMessage[]): ai.Message[] {
  const out: ai.Message[] = [];
  for (const m of messages) {
    if (m.role !== "assistant") {
      out.push(new ai.Message({ role: m.role, content: m.content }));
      continue;
    }

    // assistant 累加器：在遇到 tool block 时刷出当前 assistant + 跟一条 tool 消息
    let thinking = "";
    let text = "";
    const pendingToolCalls: { id: string; type: string; function: { name: string; arguments: string } }[] = [];

    const flushAssistant = () => {
      if (!thinking && !text && pendingToolCalls.length === 0) return;
      const payload: Record<string, unknown> = { role: "assistant", content: text };
      if (thinking) {
        payload.thinking = thinking;
        payload.reasoning_content = thinking;
      }
      if (pendingToolCalls.length > 0) payload.tool_calls = pendingToolCalls.slice();
      out.push(new ai.Message(payload));
      thinking = "";
      text = "";
      pendingToolCalls.length = 0;
    };

    let canExpand = true; // 老数据若有 tool block 缺 toolCallId，全消息回退到塌缩
    for (const b of m.blocks) {
      if (b.type === "tool" && !b.toolCallId) {
        canExpand = false;
        break;
      }
    }

    if (!canExpand) {
      // 兼容旧数据：仅发最终 content + thinking 拼接（thinking 拼接也无害）
      const allThinking = m.blocks
        .filter((b) => b.type === "thinking")
        .map((b) => b.content)
        .join("");
      const payload: Record<string, unknown> = { role: "assistant", content: m.content };
      if (allThinking) {
        payload.thinking = allThinking;
        payload.reasoning_content = allThinking;
      }
      out.push(new ai.Message(payload));
      continue;
    }

    for (const b of m.blocks) {
      if (b.type === "thinking") {
        thinking += b.content;
      } else if (b.type === "text") {
        text += b.content;
      } else if (b.type === "tool" && b.toolCallId) {
        pendingToolCalls.push({
          id: b.toolCallId,
          type: "function",
          function: { name: b.toolName || "", arguments: b.toolInput || "{}" },
        });
        flushAssistant();
        out.push(
          new ai.Message({
            role: "tool",
            content: b.content,
            tool_call_id: b.toolCallId,
          })
        );
      }
      // approval / agent 块不参与 LLM 历史还原，跳过
    }
    flushAssistant();
  }
  return out;
}

// 核心发送：完全基于 convId，共享给 sendToTab / sendFromSidebar / regenerate
// content 中可能包含内联 <mention asset-id=...>@name</mention> 标签，前端不再单独维护 mentions 数组。
async function _sendForConversation(convId: number, content: string) {
  conversationStopRequests.delete(convId);
  const state = useAIStore.getState();
  const streaming = state.conversationStreaming[convId] || { sending: false, pendingQueue: [] };

  // 生成中时排队：推送到后端 runner 队列 + 本地队列（用于 UI 显示）
  if (streaming.sending) {
    if (content.trim()) {
      updateConversation(convId, {
        pendingQueue: [...streaming.pendingQueue, { text: content.trim() }],
      });
      QueueAIMessage(convId, content.trim()).catch(() => {});
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
      blocks: [],
    });
    updateConversation(convId, { messages: newMessages, sending: true });
    // 用户消息是用户亲手输入的内容，最不应该丢；发送后立即落盘。
    // includeStreaming=true 保留历史未完成 block（如有）。
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

  // 仅 DeepSeek-v4 thinking 模式强制要求"带 tool_calls 的 assistant 必须回传 reasoning_content"，
  // 且需要历史中间 tool_calls 可见才能跨 turn 继续推理；其他 provider（Anthropic / OpenAI / Kimi 等）
  // 保持原有塌缩行为，避免引入不必要的回归。
  const modelName = useAIStore.getState().modelName;
  const needExpand = modelName.startsWith("deepseek-v4");
  const apiMessages = needExpand
    ? expandToAPIMessages(newMessages)
    : newMessages.map((m) => new ai.Message({ role: m.role, content: m.content }));

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
    await SendAIMessage(convId, apiMessages, aiContext);
  } catch {
    updateConversation(convId, { sending: false });
    cleanupConvListener(convId);
  }
}

// === Store ===

// workspace tab 只需要一个空占位："此 tab 已注册"。实际 draft/scrollTop/editTarget 在组件内部管理。
type WorkspaceTabPlaceholder = Record<string, unknown>;

interface AIState {
  tabStates: Record<string, WorkspaceTabPlaceholder>;
  conversationMessages: Record<number, ChatMessage[]>;
  conversationStreaming: Record<number, { sending: boolean; pendingQueue: PendingQueueItem[] }>;

  // 全局状态
  conversations: conversation_entity.Conversation[];
  configured: boolean;
  providerName: string;
  modelName: string;

  // 侧边助手状态
  sidebarTabs: SidebarAITab[];
  activeSidebarTabId: string | null;

  // 配置
  checkConfigured: () => Promise<void>;

  // 发送
  send: (content: string) => Promise<void>;
  sendToTab: (tabId: string, content: string) => Promise<void>;
  sendFromSidebarTab: (tabId: string, content: string) => Promise<void>;
  editAndResendConversation: (convId: number, messageIndex: number, content: string) => Promise<void>;
  stopConversation: (convId: number) => Promise<void>;
  stopGeneration: (tabId: string) => Promise<void>;
  regenerate: (tabId: string, messageIndex: number) => Promise<void>;
  regenerateConversation: (convId: number, messageIndex: number) => Promise<void>;
  removeFromQueue: (convId: number, index: number) => void;
  clearQueue: (convId: number) => void;

  // Tab 管理 (delegates to tabStore)
  openConversationTab: (conversationId: number) => Promise<string>;
  openNewConversationTab: () => string;
  clear: () => void;

  // 会话管理
  fetchConversations: () => Promise<void>;
  renameConversation: (id: number, title: string) => Promise<boolean>;
  deleteConversation: (id: number) => Promise<void>;

  // 侧边助手 actions
  getActiveSidebarTab: () => SidebarAITab | null;
  getActiveSidebarConversationId: () => number | null;
  getSidebarTabState: (tabId: string) => SidebarTabUIState;
  getSidebarTabStatus: (tabId: string) => SidebarTabStatus;
  openNewSidebarTab: (options?: { activate?: boolean }) => string;
  bindSidebarTabToConversation: (tabId: string, conversationId: number, options?: { activate?: boolean }) => string;
  openSidebarConversationInSidebar: (
    conversationId: number,
    options?: { activate?: boolean; reuseIfOpen?: boolean }
  ) => string;
  activateSidebarTab: (tabId: string) => void;
  closeSidebarTab: (tabId: string) => void;
  promoteSidebarToTab: (tabId?: string) => Promise<string | null>;
  validateSidebarTabs: () => void;
  setSidebarTabInputDraft: (tabId: string, draft: { content: string }) => void;
  setSidebarTabScrollTop: (tabId: string, scrollTop: number) => void;
  setSidebarTabEditTarget: (
    tabId: string,
    editTarget: {
      conversationId: number;
      messageIndex: number;
      draft: { content: string };
    } | null
  ) => void;
  stopSidebarTab: (tabId: string) => Promise<void>;

  // 查询
  isAnySending: () => boolean;

  // NEW — 派生 getter
  getMessagesByConversationId: (convId: number) => ChatMessage[];
  getStreamingByConversationId: (convId: number) => { sending: boolean; pendingQueue: PendingQueueItem[] };
}

export const useAIStore = create<AIState>((set, get) => {
  const initialSidebarState = loadInitialSidebarState();

  // 侧边 tab uiState 的浅 merge：只替换 patch 里出现的字段，其余保持原引用。
  // 引用稳定性很重要——仅改动 inputDraft 时，editTarget/scrollTop 的引用不变，
  // 使得按 editTarget 订阅的组件不会因键盘输入而重渲染。
  const patchSidebarTabUiState = (tabId: string, patch: Partial<SidebarTabUIState>) => {
    set((state) => ({
      sidebarTabs: state.sidebarTabs.map((tab) =>
        tab.id === tabId ? { ...tab, uiState: { ...tab.uiState, ...patch } } : tab
      ),
    }));
  };

  return {
    tabStates: {},
    conversationMessages: {},
    conversationStreaming: {},

    conversations: [],
    configured: false,
    providerName: "",
    modelName: "",

    sidebarTabs: initialSidebarState.tabs,
    activeSidebarTabId: initialSidebarState.activeSidebarTabId,

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
      const request = beginConversationListRequest();
      try {
        const convs = await ListConversations();
        if (!canApplyConversationListRequest(request)) {
          return;
        }
        markConversationListRequestApplied(request);
        applyConversationListSnapshot(convs || []);
      } catch {
        // 临时失败保留现有列表，避免把刚完成的本地乐观标题或会话条目清空。
        if (!canApplyConversationListRequest(request)) {
          return;
        }
      }
    },

    renameConversation: async (id: number, title: string) => {
      return renameConversationInternal(id, title, { waitForListRefresh: true });
    },

    deleteConversation: async (id: number) => {
      try {
        await DeleteConversation(id);
        const tabStore = useTabStore.getState();
        const openAITabIds = tabStore.tabs
          .filter((tab) => tab.type === "ai" && (tab.meta as AITabMeta).conversationId === id)
          .map((tab) => tab.id);

        invalidateConversationListRequests();

        set((state) => {
          const nextSidebarTabs = state.sidebarTabs.filter((tab) => tab.conversationId !== id);
          const activeSidebarTabIndex = state.sidebarTabs.findIndex((tab) => tab.id === state.activeSidebarTabId);
          const activeSidebarTabWasRemoved =
            activeSidebarTabIndex !== -1 && state.sidebarTabs[activeSidebarTabIndex]?.conversationId === id;
          const findFallbackSidebarTabId = () => {
            if (activeSidebarTabIndex === -1) {
              return nextSidebarTabs[0]?.id ?? null;
            }
            // 以原数组里 active tab 的位置为基准，先向右再向左扫描第一个未被删除的 tab，
            // 避免 filter 后用旧 index 去索引新数组带来的偏移错误。
            for (let i = activeSidebarTabIndex + 1; i < state.sidebarTabs.length; i += 1) {
              const candidate = state.sidebarTabs[i];
              if (candidate && candidate.conversationId !== id) return candidate.id;
            }
            for (let i = activeSidebarTabIndex - 1; i >= 0; i -= 1) {
              const candidate = state.sidebarTabs[i];
              if (candidate && candidate.conversationId !== id) return candidate.id;
            }
            return null;
          };
          const nextActiveSidebarTabId = nextSidebarTabs.some((tab) => tab.id === state.activeSidebarTabId)
            ? state.activeSidebarTabId
            : activeSidebarTabWasRemoved
              ? findFallbackSidebarTabId()
              : (nextSidebarTabs[0]?.id ?? null);
          const { [id]: _removedMessages, ...conversationMessages } = state.conversationMessages;
          const { [id]: _removedStreaming, ...conversationStreaming } = state.conversationStreaming;
          return {
            conversations: state.conversations.filter((conv) => conv.ID !== id),
            sidebarTabs: nextSidebarTabs,
            activeSidebarTabId: nextActiveSidebarTabId,
            conversationMessages,
            conversationStreaming,
          };
        });

        for (const tabId of openAITabIds) {
          tabStore.closeTab(tabId);
        }
        cleanupConvListener(id);

        await get().fetchConversations();
      } catch (e) {
        console.error("删除会话失败:", e);
      }
    },

    // === 侧边助手 ===

    getActiveSidebarTab: () => {
      const { sidebarTabs, activeSidebarTabId } = get();
      return sidebarTabs.find((tab) => tab.id === activeSidebarTabId) ?? null;
    },

    getActiveSidebarConversationId: () => {
      return get().getActiveSidebarTab()?.conversationId ?? null;
    },

    getSidebarTabState: (tabId: string) => {
      return get().sidebarTabs.find((tab) => tab.id === tabId)?.uiState ?? createDefaultSidebarUiState();
    },

    getSidebarTabStatus: (tabId: string) => {
      const tab = get().sidebarTabs.find((item) => item.id === tabId);
      return getConversationStatus(tab?.conversationId ?? null);
    },

    openNewSidebarTab: (options) => {
      // 已经存在空白会话宿主时直接跳转，避免重复创建未使用的新会话 tab。
      const blankTab = get().sidebarTabs.find((tab) => tab.conversationId == null);
      if (blankTab) {
        if (options?.activate !== false || !get().activeSidebarTabId) {
          get().activateSidebarTab(blankTab.id);
        }
        return blankTab.id;
      }
      const nextTab = createSidebarTab();
      set((state) => ({
        sidebarTabs: [...state.sidebarTabs, nextTab],
        activeSidebarTabId: resolveNextActiveId(state.activeSidebarTabId, nextTab.id, options?.activate),
      }));
      return nextTab.id;
    },

    bindSidebarTabToConversation: (tabId: string, conversationId: number, options) => {
      const targetTab = get().sidebarTabs.find((tab) => tab.id === tabId);
      if (!targetTab) {
        return get().openSidebarConversationInSidebar(conversationId, {
          activate: options?.activate,
          reuseIfOpen: true,
        });
      }

      // 当前 blank tab 绑定到一个已在侧边打开的会话时，直接复用现有宿主，
      // 避免同一个 conversationId 在侧边产生重复 tab。
      const reusedTab = get().sidebarTabs.find((tab) => tab.id !== tabId && tab.conversationId === conversationId);
      if (reusedTab) {
        if (options?.activate !== false || !get().activeSidebarTabId) {
          get().activateSidebarTab(reusedTab.id);
        }
        return reusedTab.id;
      }

      const previousConversationId = targetTab.conversationId;
      const title = get().conversations.find((conv) => conv.ID === conversationId)?.Title || targetTab.title;
      set((state) => ({
        sidebarTabs: state.sidebarTabs.map((tab) =>
          tab.id === tabId
            ? {
                ...tab,
                conversationId,
                title,
                uiState: createDefaultSidebarUiState(),
              }
            : tab
        ),
        activeSidebarTabId: resolveNextActiveId(state.activeSidebarTabId, tabId, options?.activate),
      }));
      conversationStopRequests.delete(conversationId);
      void ensureConversationMessagesLoaded(conversationId);
      if (previousConversationId != null && previousConversationId !== conversationId) {
        cleanupConversationIfUnused(previousConversationId, { ignoreSidebarTabId: tabId });
      }
      return tabId;
    },

    openSidebarConversationInSidebar: (conversationId: number, options) => {
      const existingTab = get().sidebarTabs.find((tab) => tab.conversationId === conversationId);
      if (existingTab && options?.reuseIfOpen !== false) {
        if (options?.activate !== false || !get().activeSidebarTabId) {
          get().activateSidebarTab(existingTab.id);
        }
        void ensureConversationMessagesLoaded(conversationId);
        return existingTab.id;
      }

      const title = get().conversations.find((conv) => conv.ID === conversationId)?.Title || getDefaultSidebarTitle();
      const nextTab = createSidebarTab({ conversationId, title });
      set((state) => ({
        sidebarTabs: [...state.sidebarTabs, nextTab],
        activeSidebarTabId: resolveNextActiveId(state.activeSidebarTabId, nextTab.id, options?.activate),
      }));
      conversationStopRequests.delete(conversationId);
      void ensureConversationMessagesLoaded(conversationId);
      return nextTab.id;
    },

    activateSidebarTab: (tabId: string) => {
      if (!get().sidebarTabs.some((tab) => tab.id === tabId)) return;
      set({ activeSidebarTabId: tabId });
    },

    closeSidebarTab: (tabId: string) => {
      const state = get();
      const index = state.sidebarTabs.findIndex((tab) => tab.id === tabId);
      if (index === -1) return;

      const closingTab = state.sidebarTabs[index];
      if (closingTab.conversationId != null) {
        // 侧边宿主关闭前先把当前会话快照刷盘；
        // 但真正的 listener 回收要等到确认没有其它宿主仍引用该会话。
        persistConversationSnapshot(closingTab.conversationId, true);
      }

      set((currentState) => {
        const nextSidebarTabs = currentState.sidebarTabs.filter((tab) => tab.id !== tabId);
        let nextActiveSidebarTabId = currentState.activeSidebarTabId;
        if (currentState.activeSidebarTabId === tabId) {
          // 关闭当前 tab 时优先激活右邻居，其次左邻居，最后退化到首个剩余 tab。
          nextActiveSidebarTabId =
            nextSidebarTabs[index]?.id ?? nextSidebarTabs[index - 1]?.id ?? nextSidebarTabs[0]?.id ?? null;
        }
        return {
          sidebarTabs: nextSidebarTabs,
          activeSidebarTabId: nextActiveSidebarTabId,
        };
      });

      if (closingTab.conversationId != null) {
        cleanupConversationIfUnused(closingTab.conversationId, { ignoreSidebarTabId: tabId });
      }
    },

    promoteSidebarToTab: async (tabId?: string) => {
      const targetTab = tabId
        ? (get().sidebarTabs.find((tab) => tab.id === tabId) ?? null)
        : get().getActiveSidebarTab();
      if (!targetTab?.conversationId) return null;
      return get().openConversationTab(targetTab.conversationId);
    },

    validateSidebarTabs: () => {
      const conversationsById = new Map(
        get().conversations.map((conversation) => [conversation.ID, conversation] as const)
      );
      const removedConversationIds = get()
        .sidebarTabs.filter((tab) => tab.conversationId != null && !conversationsById.has(tab.conversationId))
        .map((tab) => tab.conversationId)
        .filter((conversationId): conversationId is number => conversationId != null);

      set((state) => {
        const nextSidebarTabs = state.sidebarTabs
          .filter((tab) => tab.conversationId == null || conversationsById.has(tab.conversationId))
          .map((tab) => {
            if (tab.conversationId == null) return tab;
            const conversation = conversationsById.get(tab.conversationId);
            return conversation && conversation.Title !== tab.title ? { ...tab, title: conversation.Title } : tab;
          });
        return {
          sidebarTabs: nextSidebarTabs,
          activeSidebarTabId: nextSidebarTabs.some((tab) => tab.id === state.activeSidebarTabId)
            ? state.activeSidebarTabId
            : (nextSidebarTabs[0]?.id ?? null),
        };
      });

      for (const conversationId of new Set(removedConversationIds)) {
        cleanupConversationIfUnused(conversationId);
      }
    },

    setSidebarTabInputDraft: (tabId: string, draft) => {
      patchSidebarTabUiState(tabId, {
        inputDraft: {
          content: draft.content ?? "",
        },
      });
    },

    setSidebarTabScrollTop: (tabId: string, scrollTop: number) => {
      patchSidebarTabUiState(tabId, { scrollTop });
    },

    setSidebarTabEditTarget: (tabId: string, editTarget) => {
      patchSidebarTabUiState(tabId, {
        editTarget: editTarget
          ? {
              conversationId: editTarget.conversationId,
              messageIndex: editTarget.messageIndex,
              draft: {
                content: editTarget.draft.content ?? "",
              },
            }
          : null,
      });
    },

    stopSidebarTab: async (tabId: string) => {
      const conversationId = get().sidebarTabs.find((tab) => tab.id === tabId)?.conversationId;
      if (conversationId != null) {
        await get().stopConversation(conversationId);
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
      let loadedMessages = state.conversationMessages[conversationId];

      // 同一 conversation 可能已经在侧边栏 live streaming。
      // 主工作区打开时优先复用内存里的消息/队列状态，避免重新加载后把 pendingQueue 和 sending 状态清空。
      if (loadedMessages === undefined) {
        try {
          const displayMsgs = await LoadConversationMessages(conversationId);
          loadedMessages = convertDisplayMessages(displayMsgs);
        } catch (e) {
          console.error("打开会话失败:", e);
          throw e;
        }
      }

      conversationStopRequests.delete(conversationId);
      tabStore.openTab({
        id: tabId,
        type: "ai",
        label: title,
        meta: { type: "ai", conversationId, title },
      });

      set((currentState) => ({
        tabStates: {
          ...currentState.tabStates,
          [tabId]: {},
        },
        conversationMessages:
          currentState.conversationMessages[conversationId] !== undefined || loadedMessages === undefined
            ? currentState.conversationMessages
            : { ...currentState.conversationMessages, [conversationId]: loadedMessages },
        conversationStreaming: currentState.conversationStreaming[conversationId]
          ? currentState.conversationStreaming
          : { ...currentState.conversationStreaming, [conversationId]: { sending: false, pendingQueue: [] } },
      }));

      return tabId;
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

      const existingTab = useTabStore.getState().tabs.find((t) => t.id === tabId);
      if (!existingTab) return;
      let convId = (existingTab.meta as AITabMeta).conversationId;

      // drain 模式 + 没有历史消息时直接返回，避免创建空会话
      const existingMessages = convId != null ? state.conversationMessages[convId] || [] : [];
      if (!content.trim() && existingMessages.length === 0) return;

      // Ensure tab has a conversation ID *before* writing any message state —
      // conversationMessages / conversationStreaming are keyed by convId.
      if (convId == null) {
        const newId = await createConversationForEmptyHost((conv) => {
          const curTab = useTabStore.getState().tabs.find((t) => t.id === tabId);
          useTabStore.getState().updateTab(tabId, {
            meta: {
              type: "ai",
              conversationId: conv.ID,
              title: curTab?.label || conv.Title || DEFAULT_CONVERSATION_TITLE,
            },
          });
          set((currentState) => ({
            conversations: currentState.conversations.some((item) => item.ID === conv.ID)
              ? currentState.conversations
              : [conv, ...currentState.conversations],
            conversationMessages: currentState.conversationMessages[conv.ID]
              ? currentState.conversationMessages
              : { ...currentState.conversationMessages, [conv.ID]: [] },
            conversationStreaming: currentState.conversationStreaming[conv.ID]
              ? currentState.conversationStreaming
              : {
                  ...currentState.conversationStreaming,
                  [conv.ID]: { sending: false, pendingQueue: [] },
                },
          }));
        });
        if (newId == null) return;
        convId = newId;
      }

      if (shouldSyncConversationTitleBeforeSend(convId, content)) {
        await updateConversationTitleForMessage(convId, content);
      }

      await _sendForConversation(convId, content);
    },

    sendFromSidebarTab: async (tabId: string, content: string) => {
      const sidebarTab = get().sidebarTabs.find((tab) => tab.id === tabId);
      if (!sidebarTab) return;
      let convId = sidebarTab.conversationId;
      const existingMessages = convId != null ? get().conversationMessages[convId] || [] : [];
      if (!content.trim() && existingMessages.length === 0) return;

      if (convId == null) {
        const newId = await createConversationForEmptyHost((conv) => {
          set((state) => ({
            conversations: state.conversations.some((item) => item.ID === conv.ID)
              ? state.conversations
              : [conv, ...state.conversations],
            conversationMessages: state.conversationMessages[conv.ID]
              ? state.conversationMessages
              : { ...state.conversationMessages, [conv.ID]: [] },
            conversationStreaming: state.conversationStreaming[conv.ID]
              ? state.conversationStreaming
              : {
                  ...state.conversationStreaming,
                  [conv.ID]: { sending: false, pendingQueue: [] },
                },
            sidebarTabs: state.sidebarTabs.map((tab) =>
              tab.id === tabId ? { ...tab, conversationId: conv.ID, title: conv.Title || tab.title } : tab
            ),
          }));
        });
        if (newId == null) return;
        convId = newId;
      }

      if (shouldSyncConversationTitleBeforeSend(convId, content)) {
        await updateConversationTitleForMessage(convId, content);
      }
      await _sendForConversation(convId, content);
    },

    editAndResendConversation: async (convId: number, messageIndex: number, content: string) => {
      if (!content.trim()) return;

      const messages = get().conversationMessages[convId] || [];
      if (messages.length === 0) return;
      if (messageIndex < 0 || messageIndex >= messages.length) return;

      const target = messages[messageIndex];
      if (!target || target.role !== "user") return;

      const firstUserIndex = messages.findIndex((message) => message.role === "user");
      if (firstUserIndex === messageIndex && shouldAutoRenameAfterFirstTurnEdit(convId, target.content)) {
        // 只有当前标题仍等于旧首条规范化标题时，首条编辑才继续自动改名；
        // 若用户已手工改名，则保留用户自定义标题。
        await updateConversationTitleForMessage(convId, content);
      }

      const truncated = messages.slice(0, messageIndex);
      // 从被编辑消息之前的稳定历史重新发送，确保后续分支完全替换。
      await replayConversation(convId, truncated, content);
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

      await get().regenerateConversation(convId, messageIndex);
    },

    regenerateConversation: async (convId: number, messageIndex: number) => {
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

    getMessagesByConversationId: (convId: number) => {
      return get().conversationMessages[convId] || [];
    },

    getStreamingByConversationId: (convId: number) => {
      return get().conversationStreaming[convId] || { sending: false, pendingQueue: [] };
    },
  };
});

const SIDEBAR_PERSIST_DEBOUNCE_MS = 300;
let sidebarPersistTimer: ReturnType<typeof setTimeout> | null = null;
let sidebarPersistPending: { tabs: SidebarAITab[]; activeSidebarTabId: string | null } | null = null;

function flushSidebarPersist() {
  if (sidebarPersistTimer) {
    clearTimeout(sidebarPersistTimer);
    sidebarPersistTimer = null;
  }
  if (sidebarPersistPending) {
    const { tabs, activeSidebarTabId } = sidebarPersistPending;
    sidebarPersistPending = null;
    persistSidebarTabs(tabs, activeSidebarTabId);
  }
}

function scheduleSidebarPersist(tabs: SidebarAITab[], activeSidebarTabId: string | null) {
  sidebarPersistPending = { tabs, activeSidebarTabId };
  if (sidebarPersistTimer) return;
  sidebarPersistTimer = setTimeout(() => {
    sidebarPersistTimer = null;
    flushSidebarPersist();
  }, SIDEBAR_PERSIST_DEBOUNCE_MS);
}

function didSidebarStructureChange(next: SidebarAITab[], prev: SidebarAITab[]) {
  if (next.length !== prev.length) return true;
  for (let i = 0; i < next.length; i += 1) {
    const a = next[i];
    const b = prev[i];
    if (a.id !== b.id || a.conversationId !== b.conversationId || a.title !== b.title) return true;
  }
  return false;
}

persistSidebarTabs(useAIStore.getState().sidebarTabs, useAIStore.getState().activeSidebarTabId);
useAIStore.subscribe((state, prevState) => {
  if (state.sidebarTabs === prevState.sidebarTabs && state.activeSidebarTabId === prevState.activeSidebarTabId) {
    return;
  }
  // 结构性变更（增删 tab/切换激活）需要立刻落盘，避免刷新丢失；
  // 仅 uiState 字段变化（滚动位置、输入草稿）走 debounce，避免高频 localStorage 写入阻塞主线程。
  if (
    state.activeSidebarTabId !== prevState.activeSidebarTabId ||
    didSidebarStructureChange(state.sidebarTabs, prevState.sidebarTabs)
  ) {
    flushSidebarPersist();
    persistSidebarTabs(state.sidebarTabs, state.activeSidebarTabId);
  } else {
    scheduleSidebarPersist(state.sidebarTabs, state.activeSidebarTabId);
  }
});

if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", flushSidebarPersist);
  window.addEventListener("pagehide", flushSidebarPersist);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flushSidebarPersist();
  });
}

// === Close Hook: clean up when tabStore closes an AI tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "ai") return;

  const meta = tab.meta as AITabMeta;

  if (meta.conversationId) {
    // 关闭标签前补一次最终快照，避免最后一段对话未落盘。
    // 关闭时流式输出可能仍在进行（streaming=true），必须传 includeStreaming=true 才能
    // 把最后一段 assistant 消息一起落盘；toDisplayMessages 会把未完结的 block 归一为 cancelled。
    persistConversationSnapshot(meta.conversationId, true);
    cleanupConversationIfUnused(meta.conversationId, { ignoreMainTabId: tab.id });
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
