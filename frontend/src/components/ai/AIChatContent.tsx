import { useState, useRef, useEffect, useMemo, memo, useCallback } from "react";
import {
  Loader2,
  CornerDownLeft,
  Square,
  RefreshCw,
  X,
  Trash2,
  Copy,
  ArrowUp,
  ArrowDown,
  Database,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import Markdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import {
  Button,
  ScrollArea,
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@opskat/ui";
import {
  useAIStore,
  useAISendOnEnter,
  type ChatMessage,
  type ContentBlock,
  type PendingQueueItem,
  type MentionRef,
  type TokenUsage,
} from "@/stores/aiStore";
import { AIChatInput, type AIChatInputDraft, type AIChatInputHandle } from "@/components/ai/AIChatInput";
import { UserMessage } from "@/components/ai/UserMessage";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";
import { formatModKey } from "@/stores/shortcutStore";
import { ToolBlock } from "@/components/ai/ToolBlock";
import { ThinkingBlock } from "@/components/ai/ThinkingBlock";
import { AgentBlock } from "@/components/ai/AgentBlock";
import { ApprovalBlock } from "@/components/approval/ApprovalBlock";
import { AISetupWizard } from "@/components/ai/AISetupWizard";
import { CompactContext, useCompact } from "@/components/ai/AIChatContentContext";

// 常量化 Markdown 插件数组，避免每次渲染创建新引用导致 Markdown 重解析
const mdRemarkPlugins = [remarkGfm];
const mdRehypePlugins = [rehypeSanitize];
// 统一助手消息选中态样式，避免不同气泡的选区反馈不一致。
const messageSelectionClass = "select-text selection:bg-primary/25 selection:text-foreground";

// 稳定引用的默认值，避免 zustand selector 每次返回新对象导致无限渲染
const EMPTY_MESSAGES: ChatMessage[] = [];
const DEFAULT_STREAMING = { sending: false, pendingQueue: [] as PendingQueueItem[] };

interface AIChatContentProps {
  tabId?: string;
  sideTabId?: string;
  conversationId?: number | null;
  compact?: boolean;
  /** Optional: if provided, replaces the default sendToTab-based send path. */
  onSendOverride?: (content: string, mentions?: MentionRef[]) => Promise<void>;
  /** Optional: if provided, replaces the default stopGeneration-based stop path. */
  onStopOverride?: () => Promise<void>;
}

interface EditTarget {
  conversationId: number;
  messageIndex: number;
  draft: AIChatInputDraft;
}

// 编辑态需要比较 mentions 是否还是同一条消息，避免 messageIndex 没变但消息内容已被刷新时误复用草稿。
function normalizeMentions(mentions: MentionRef[] | undefined): MentionRef[] {
  return mentions ?? [];
}

function areMentionsEqual(left: MentionRef[] | undefined, right: MentionRef[] | undefined) {
  const normalizedLeft = normalizeMentions(left);
  const normalizedRight = normalizeMentions(right);
  if (normalizedLeft.length !== normalizedRight.length) return false;
  return normalizedLeft.every((mention, index) => {
    const other = normalizedRight[index];
    return (
      mention.assetId === other.assetId &&
      mention.name === other.name &&
      mention.start === other.start &&
      mention.end === other.end
    );
  });
}

/** Split blocks into segments: consecutive non-approval blocks form a 'bubble' segment,
 *  each pending approval block becomes its own 'approval' segment.
 *  Resolved (non-pending) approval blocks are skipped so surrounding content merges into one bubble. */
function splitBlocksByApproval(blocks: ContentBlock[]): Array<{ type: "bubble" | "approval"; blocks: ContentBlock[] }> {
  const segments: Array<{ type: "bubble" | "approval"; blocks: ContentBlock[] }> = [];
  let currentBubble: ContentBlock[] = [];

  for (const block of blocks) {
    if (block.type === "approval" && block.status === "pending_confirm") {
      if (currentBubble.length > 0) {
        segments.push({ type: "bubble", blocks: currentBubble });
        currentBubble = [];
      }
      segments.push({ type: "approval", blocks: [block] });
    } else if (block.type === "approval") {
      // Resolved approval — skip, don't split
    } else {
      currentBubble.push(block);
    }
  }
  if (currentBubble.length > 0) {
    segments.push({ type: "bubble", blocks: currentBubble });
  }
  return segments;
}

export function AIChatContent({
  tabId,
  sideTabId,
  conversationId: propConvId,
  compact = false,
  onSendOverride,
  onStopOverride,
}: AIChatContentProps) {
  const { t } = useTranslation();
  // 拆成单字段 selector 拿稳定函数引用 / 仅订阅必要字段。
  // 之前 `useAIStore()` 整库订阅，每次按键写 inputDraft 都会重渲整棵消息树。
  const configured = useAIStore((s) => s.configured);
  const sendToTab = useAIStore((s) => s.sendToTab);
  const stopGeneration = useAIStore((s) => s.stopGeneration);
  const regenerate = useAIStore((s) => s.regenerate);
  const regenerateConversation = useAIStore((s) => s.regenerateConversation);
  const removeFromQueue = useAIStore((s) => s.removeFromQueue);
  const clearQueue = useAIStore((s) => s.clearQueue);
  const editAndResendConversation = useAIStore((s) => s.editAndResendConversation);
  const setSidebarTabInputDraft = useAIStore((s) => s.setSidebarTabInputDraft);
  const setSidebarTabEditTarget = useAIStore((s) => s.setSidebarTabEditTarget);
  const setSidebarTabScrollTop = useAIStore((s) => s.setSidebarTabScrollTop);
  const derivedConvId = useTabStore((s) => {
    if (!tabId) return null;
    const tab = s.tabs.find((x) => x.id === tabId);
    return tab ? (tab.meta as AITabMeta).conversationId : null;
  });
  // 只订阅 editTarget 字段，不订阅 inputDraft / scrollTop。
  // 得益于 store 侧 patchSidebarTabUiState 的浅 merge，仅输入草稿变化时 editTarget 引用保持不变，
  // 这里的 selector 会得到同一引用 → Object.is 通过 → 不触发 AIChatContent 重渲染。
  const sidebarEditTarget = useAIStore((s) =>
    sideTabId ? (s.sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState.editTarget ?? null) : null
  );
  const conversationId = propConvId ?? derivedConvId;

  const messages = useAIStore((s) =>
    conversationId != null ? s.conversationMessages[conversationId] || EMPTY_MESSAGES : EMPTY_MESSAGES
  );
  const streaming = useAIStore((s) =>
    conversationId != null ? s.conversationStreaming[conversationId] || DEFAULT_STREAMING : DEFAULT_STREAMING
  );
  const { sending, pendingQueue } = streaming;
  // 从最新到最旧收集非空用户消息，用 useMemo 避免无关渲染也构造新数组。
  const userMessageHistory = useMemo(() => {
    const history: string[] = [];
    for (let i = messages.length - 1; i >= 0; i--) {
      const msg = messages[i];
      if (msg.role === "user" && msg.content.trim()) {
        history.push(msg.content);
      }
    }
    return history;
  }, [messages]);

  const [regenerateTarget, setRegenerateTarget] = useState<number | null>(null);
  const [localEditTarget, setLocalEditTarget] = useState<EditTarget | null>(null);
  const [empty, setEmpty] = useState(true);
  const inputRef = useRef<AIChatInputHandle>(null);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const previousConversationIdRef = useRef<number | null | undefined>(conversationId);
  const editTarget = sideTabId ? sidebarEditTarget : localEditTarget;
  // 输入草稿的防抖落盘：每次 onUpdate 都直接写 store 会重建 sidebarTabs 数组，
  // 触发 SideAssistantPanel 等订阅者按键级重渲。这里在本地累积，250ms 静默后写一次。
  // tab 切换 / 卸载 / 退出编辑态会主动 flush，确保不丢草稿。
  const draftFlushTimerRef = useRef<number | null>(null);
  const pendingDraftRef = useRef<AIChatInputDraft | null>(null);
  // sticky-bottom 跟随：在底部时自动追内容，用户主动滚开后暂停跟随，再回到底部恢复跟随。
  // 不再用「每次 messages 变更测一次距底距离」——这种方式遇到流式 token 把内容推高、
  // 用户尚未触发滚动事件时会出现"在底部却跟不上"的状态。改成由 scroll 事件维护一个 ref。
  const isStickyBottomRef = useRef(true);
  const scrollPersistTimerRef = useRef<number | null>(null);

  // 滚动初始化 + sticky 状态追踪 + 侧边滚动位置防抖落盘 + 内容尺寸跟随。
  // 之前用 useEffect([messages]) 在 commit 帧设一次 scrollTop = scrollHeight；问题是审批卡片里
  // Textarea(rows 跟内容)/长 <code whitespace-pre-wrap>/Markdown 在 commit 后还会继续 reflow，
  // scrollHeight 又长一截但 effect 不再触发，用户停在"老底部"上、新卡片漏在视口下方。
  // 改用 ResizeObserver 跟随内容真实尺寸，只要还在 sticky 就一直追到底，避开布局 settle 的时序坑。
  useEffect(() => {
    const viewport = scrollAreaRef.current?.querySelector("[data-radix-scroll-area-viewport]") as HTMLDivElement | null;
    if (!viewport) return;
    if (sideTabId) {
      // 侧边模式恢复每个 tab 自己保存的滚动位置。
      viewport.scrollTop =
        useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState.scrollTop ?? 0;
    } else {
      // 主工作区 tab：进入会话直接落到底部，让用户看到最新消息。
      viewport.scrollTop = viewport.scrollHeight;
    }
    isStickyBottomRef.current = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 32;

    const handleScroll = () => {
      const distance = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
      // 32px 容差吸收子像素抖动 + 程序滚动的 round-off；离底超过容差视为用户已滚开。
      isStickyBottomRef.current = distance < 32;
      if (sideTabId) {
        // 落盘走 200ms 防抖，避免流式自动滚每帧写 store 触发 sidebarTabs 重建。
        if (scrollPersistTimerRef.current !== null) {
          window.clearTimeout(scrollPersistTimerRef.current);
        }
        scrollPersistTimerRef.current = window.setTimeout(() => {
          scrollPersistTimerRef.current = null;
          setSidebarTabScrollTop(sideTabId, viewport.scrollTop);
        }, 200);
      }
    };
    viewport.addEventListener("scroll", handleScroll, { passive: true });

    // 监听内容真实尺寸变化（审批卡片初始化、Textarea 自增、Remember 展开、流式 token 等）。
    // 仍 sticky 时无条件追到底；用户已上滑则 sticky=false，不打断。
    const content = viewport.firstElementChild as HTMLElement | null;
    let resizeObserver: ResizeObserver | null = null;
    if (content && typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(() => {
        if (!isStickyBottomRef.current) return;
        viewport.scrollTop = viewport.scrollHeight;
      });
      resizeObserver.observe(content);
    }

    return () => {
      viewport.removeEventListener("scroll", handleScroll);
      resizeObserver?.disconnect();
      if (scrollPersistTimerRef.current !== null) {
        window.clearTimeout(scrollPersistTimerRef.current);
        scrollPersistTimerRef.current = null;
      }
    };
  }, [conversationId, setSidebarTabScrollTop, sideTabId]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [sideTabId, tabId]);

  useEffect(() => {
    if (!sideTabId) return;
    // 侧边助手：切换 side tab 或刚绑定到新 conversation 时，恢复各自保存的 draft。
    // 通过 getState() 一次性读取，不订阅 inputDraft —— 否则每次按键都会重跑该 effect。
    const uiState = useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState;
    inputRef.current?.loadDraft(uiState?.inputDraft ?? { content: "", mentions: [] });
  }, [conversationId, sideTabId]);

  const flushPendingDraft = useCallback(() => {
    if (draftFlushTimerRef.current !== null) {
      window.clearTimeout(draftFlushTimerRef.current);
      draftFlushTimerRef.current = null;
    }
    const pending = pendingDraftRef.current;
    pendingDraftRef.current = null;
    if (sideTabId && pending) {
      setSidebarTabInputDraft(sideTabId, pending);
    }
  }, [setSidebarTabInputDraft, sideTabId]);

  const cancelPendingDraft = useCallback(() => {
    if (draftFlushTimerRef.current !== null) {
      window.clearTimeout(draftFlushTimerRef.current);
      draftFlushTimerRef.current = null;
    }
    pendingDraftRef.current = null;
  }, []);

  const handleDraftChange = useCallback(
    (draft: AIChatInputDraft) => {
      if (!sideTabId) return;
      pendingDraftRef.current = draft;
      if (draftFlushTimerRef.current !== null) return;
      draftFlushTimerRef.current = window.setTimeout(() => {
        draftFlushTimerRef.current = null;
        const pending = pendingDraftRef.current;
        pendingDraftRef.current = null;
        if (pending) setSidebarTabInputDraft(sideTabId, pending);
      }, 250);
    },
    [sideTabId, setSidebarTabInputDraft]
  );

  // 切换 side tab / 卸载时把还没落盘的最新草稿写回 store。
  useEffect(() => {
    return () => {
      flushPendingDraft();
    };
  }, [sideTabId, flushPendingDraft]);

  // 编辑态依赖 conversationId 和消息索引，切换会话时要显式清掉草稿，避免把旧草稿带到新会话。
  const resetEditMode = useCallback(
    (options?: { clearDraft?: boolean }) => {
      const hadEditTarget = sideTabId
        ? !!useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState.editTarget
        : !!localEditTarget;
      if (sideTabId) {
        setSidebarTabEditTarget(sideTabId, null);
      } else {
        setLocalEditTarget(null);
      }
      if (hadEditTarget && options?.clearDraft) {
        inputRef.current?.clear();
        if (sideTabId) {
          // 直接把 store 改成空，并丢掉 pending（它持有的是 clear 之前的旧草稿）。
          setSidebarTabInputDraft(sideTabId, { content: "", mentions: [] });
          cancelPendingDraft();
        }
      }
    },
    [cancelPendingDraft, localEditTarget, setSidebarTabEditTarget, setSidebarTabInputDraft, sideTabId]
  );

  useEffect(() => {
    if (sideTabId) return;
    if (previousConversationIdRef.current === conversationId) return;
    previousConversationIdRef.current = conversationId;
    if (editTarget) {
      resetEditMode({ clearDraft: true });
    }
  }, [conversationId, editTarget, resetEditMode, sideTabId]);

  useEffect(() => {
    // 会话消息被刷新、截断或替换后，如果编辑目标不再匹配当前消息，就立即退出编辑态。
    if (!editTarget) return;
    if (editTarget.conversationId !== conversationId) {
      resetEditMode({ clearDraft: true });
      return;
    }
    const targetMessage = messages[editTarget.messageIndex];
    if (
      !targetMessage ||
      targetMessage.role !== "user" ||
      targetMessage.content !== editTarget.draft.content ||
      !areMentionsEqual(targetMessage.mentions, editTarget.draft.mentions)
    ) {
      resetEditMode({ clearDraft: true });
    }
  }, [conversationId, editTarget, messages, resetEditMode]);

  const handleSend = useCallback(
    (text: string, mentions: MentionRef[]) => {
      const trimmed = text.trim();
      if (!trimmed && mentions.length === 0) return;
      const nextMentions = mentions.length > 0 ? mentions : undefined;
      // 提交时同步把侧边 tab 的 inputDraft 改空、并取消挂起的防抖 timer。
      // 否则 sendFromSidebarTab 异步新建会话后 conversationId 一变，
      // 第 251 行 loadDraft effect 会读到尚未被 250ms 防抖刷掉的旧草稿，
      // 把刚发送的消息回填到输入框，导致"消息已发出但输入框还有内容"。
      if (sideTabId) {
        cancelPendingDraft();
        setSidebarTabInputDraft(sideTabId, { content: "", mentions: [] });
      }
      if (editTarget && conversationId != null) {
        const activeTarget = editTarget;
        // 编辑模式改走 conversation 级 replay，提交成功后只在目标仍未变化时退出编辑态。
        void editAndResendConversation(conversationId, activeTarget.messageIndex, text, nextMentions).then(() => {
          if (sideTabId) {
            const currentEditTarget = useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)
              ?.uiState.editTarget;
            if (
              currentEditTarget &&
              currentEditTarget.conversationId === activeTarget.conversationId &&
              currentEditTarget.messageIndex === activeTarget.messageIndex
            ) {
              setSidebarTabEditTarget(sideTabId, null);
            }
            return;
          }
          setLocalEditTarget((current) =>
            current &&
            current.conversationId === activeTarget.conversationId &&
            current.messageIndex === activeTarget.messageIndex
              ? null
              : current
          );
        });
        return;
      }
      if (onSendOverride) {
        void onSendOverride(text, nextMentions);
      } else if (tabId) {
        sendToTab(tabId, text, nextMentions);
      }
    },
    [
      cancelPendingDraft,
      conversationId,
      editAndResendConversation,
      editTarget,
      onSendOverride,
      sendToTab,
      setSidebarTabEditTarget,
      setSidebarTabInputDraft,
      sideTabId,
      tabId,
    ]
  );

  const handleStop = () => {
    if (onStopOverride) {
      void onStopOverride();
    } else if (tabId) {
      stopGeneration(tabId);
    }
  };

  const handleRegenerate = useCallback((index: number) => {
    setRegenerateTarget(index);
  }, []);

  const handleEditMessage = useCallback(
    (index: number, msg: ChatMessage) => {
      if (conversationId == null || msg.role !== "user") return;
      const draft: AIChatInputDraft = {
        content: msg.content,
        mentions: msg.mentions,
      };
      // 进入编辑态时直接把原消息回填到输入框，保证 mention 和多段文本都按原样重发。
      inputRef.current?.loadDraft(draft);
      if (sideTabId) {
        setSidebarTabEditTarget(sideTabId, { conversationId, messageIndex: index, draft });
      } else {
        setLocalEditTarget({ conversationId, messageIndex: index, draft });
      }
    },
    [conversationId, setSidebarTabEditTarget, sideTabId]
  );

  const confirmRegenerate = () => {
    if (regenerateTarget !== null) {
      if (tabId) {
        regenerate(tabId, regenerateTarget);
      } else if (conversationId != null) {
        // 侧边助手没有主工作区 tabId，重生成必须直连 conversationId，
        // 否则 sidebar 内点击“重新生成”不会触发任何 replay。
        regenerateConversation(conversationId, regenerateTarget);
      }
      setRegenerateTarget(null);
    }
  };

  const sendOnEnter = useAISendOnEnter();

  if (!configured) {
    return <AISetupWizard />;
  }

  return (
    <CompactContext.Provider value={compact}>
      <div className="flex h-full flex-col" data-compact={compact}>
        {/* Messages */}
        <ScrollArea ref={scrollAreaRef} className="flex-1 min-h-0 overflow-hidden">
          <div className="max-w-3xl mx-auto p-4 space-y-6">
            {messages.length === 0 && (
              <p className="text-sm text-muted-foreground text-center mt-16">{t("ai.placeholder")}</p>
            )}
            {messages.map((msg, i) => (
              <div key={i} className="text-sm">
                {msg.role === "user" ? (
                  <UserMessage index={i} msg={msg} onEdit={handleEditMessage} />
                ) : (
                  <AssistantMessage msg={msg} index={i} sending={sending} onRegenerate={handleRegenerate} />
                )}
              </div>
            ))}
          </div>
        </ScrollArea>

        {/* Pending Queue */}
        {pendingQueue.length > 0 && (
          <div className="border-t px-3 py-2 bg-muted/30">
            <div className="max-w-3xl mx-auto">
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs text-muted-foreground">
                  {t("ai.pendingMessages", "等待发送")} ({pendingQueue.length})
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 px-1.5 text-xs text-muted-foreground hover:text-destructive"
                  onClick={() => {
                    if (conversationId != null) clearQueue(conversationId);
                  }}
                >
                  <Trash2 className="h-3 w-3 mr-1" />
                  {t("ai.clearQueue", "清空")}
                </Button>
              </div>
              <div className="space-y-1">
                {pendingQueue.map((item, i) => (
                  <div key={i} className="flex items-center gap-1.5 text-xs bg-background rounded px-2 py-1.5 border">
                    <span className="truncate flex-1 text-muted-foreground">
                      {item.text.length > 50 ? item.text.slice(0, 50) + "…" : item.text}
                    </span>
                    <button
                      className="shrink-0 text-muted-foreground/50 hover:text-destructive transition-colors"
                      onClick={() => {
                        if (conversationId != null) removeFromQueue(conversationId, i);
                      }}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Input */}
        <div className="border-t p-3">
          <div className="max-w-3xl mx-auto">
            <div className="rounded-xl border border-input bg-background transition-colors duration-150 focus-within:border-ring focus-within:ring-1 focus-within:ring-ring/50">
              {editTarget && (
                <div className="flex items-start justify-between gap-3 border-b px-3 py-2">
                  <div className="min-w-0">
                    <p className="text-xs font-medium text-foreground">{t("ai.editingMessage", "正在编辑消息")}</p>
                    <p className="text-xs text-muted-foreground">
                      {t("ai.editResendHint", "提交后会从这条消息重新发送后续对话。")}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 shrink-0 px-2 text-xs"
                    onClick={() => resetEditMode({ clearDraft: true })}
                  >
                    {t("ai.cancelEdit", "取消编辑")}
                  </Button>
                </div>
              )}
              <AIChatInput
                ref={inputRef}
                onSubmit={handleSend}
                onEmptyChange={setEmpty}
                onDraftChange={handleDraftChange}
                sendOnEnter={sendOnEnter}
                userMessageHistory={userMessageHistory}
                placeholder={t("ai.sendPlaceholder")}
              />
              <div className="flex items-center justify-between px-3 pb-2">
                <span className="text-xs text-muted-foreground/40 select-none">
                  {sendOnEnter
                    ? `Enter ${t("ai.sendShortcutHint")}`
                    : `${formatModKey("Enter")} ${t("ai.sendShortcutHint")}`}
                </span>
                {sending ? (
                  <Button
                    size="icon"
                    variant="destructive"
                    className="h-7 w-7 shrink-0 rounded-lg"
                    onClick={handleStop}
                  >
                    <Square className="h-3 w-3" />
                  </Button>
                ) : (
                  <Button
                    size="icon"
                    className="h-7 w-7 shrink-0 rounded-lg"
                    onClick={() => inputRef.current?.submit()}
                    disabled={empty}
                  >
                    <CornerDownLeft className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Regenerate confirmation dialog */}
        <AlertDialog open={regenerateTarget !== null} onOpenChange={(open) => !open && setRegenerateTarget(null)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("ai.regenerateTitle", "重新生成")}</AlertDialogTitle>
              <AlertDialogDescription>
                {t("ai.regenerateConfirm", "重新生成将删除此消息及之后的所有对话记录，确定要继续吗？")}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("common.cancel", "取消")}</AlertDialogCancel>
              <AlertDialogAction onClick={confirmRegenerate}>{t("common.confirm", "确定")}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </CompactContext.Provider>
  );
}

// 从 assistant 消息中提取纯文本内容供复制：优先取 block 内的 text 块，回退到 content。
// 工具调用/思考/Agent/审批块不进入复制结果，避免把 JSON 和执行日志塞进剪贴板。
function extractAssistantText(msg: ChatMessage): string {
  if (msg.blocks && msg.blocks.length > 0) {
    const parts: string[] = [];
    for (const b of msg.blocks) {
      if (b.type === "text" && b.content) parts.push(b.content);
    }
    if (parts.length > 0) return parts.join("\n\n").trim();
  }
  return (msg.content || "").trim();
}

// 人性化 token 数字：<1k 直显；<10k 保留 1 位小数；更大直接用 k/M 后缀。
function formatTokenCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  if (n < 1_000_000) return Math.round(n / 1000) + "k";
  return (n / 1_000_000).toFixed(1).replace(/\.0$/, "") + "M";
}

const TokenUsageBadge = memo(function TokenUsageBadge({ usage }: { usage: TokenUsage }) {
  const { t } = useTranslation();
  const input = usage.inputTokens || 0;
  const output = usage.outputTokens || 0;
  const cacheWrite = usage.cacheCreationTokens || 0;
  const cacheRead = usage.cacheReadTokens || 0;
  // 合计输入包含 cache write / read，让使用者一眼看到本轮实际消耗的 prompt 规模
  const totalInput = input + cacheWrite + cacheRead;
  if (totalInput === 0 && output === 0) return null;
  const hasCache = cacheRead > 0 || cacheWrite > 0;

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground/70 tabular-nums select-none cursor-default">
            <span className="inline-flex items-center gap-0.5">
              <ArrowUp className="h-3 w-3" />
              {formatTokenCount(totalInput)}
            </span>
            <span className="inline-flex items-center gap-0.5">
              <ArrowDown className="h-3 w-3" />
              {formatTokenCount(output)}
            </span>
            {hasCache && <Database className="h-3 w-3 text-primary/60" />}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" align="end" className="text-xs">
          <div className="space-y-0.5 tabular-nums min-w-[120px]">
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground">{t("ai.tokenUsage.input", "输入")}</span>
              <span>{input.toLocaleString()}</span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground">{t("ai.tokenUsage.output", "输出")}</span>
              <span>{output.toLocaleString()}</span>
            </div>
            {cacheWrite > 0 && (
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("ai.tokenUsage.cacheWrite", "缓存写入")}</span>
                <span>{cacheWrite.toLocaleString()}</span>
              </div>
            )}
            {cacheRead > 0 && (
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("ai.tokenUsage.cacheRead", "缓存命中")}</span>
                <span>{cacheRead.toLocaleString()}</span>
              </div>
            )}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
});

const AssistantToolbar = memo(function AssistantToolbar({
  msg,
  index,
  sending,
  onRegenerate,
}: {
  msg: ChatMessage;
  index: number;
  sending: boolean;
  onRegenerate: (index: number) => void;
}) {
  const { t } = useTranslation();
  const showActions = !sending && !msg.streaming;
  const copyText = extractAssistantText(msg);

  const handleCopy = useCallback(async () => {
    if (!copyText) return;
    try {
      await navigator.clipboard.writeText(copyText);
      toast.success(t("ai.copied", "已复制到剪贴板"), { duration: 1500, position: "top-center" });
    } catch {
      toast.error(t("ai.copyFailed", "复制失败"), { duration: 2000, position: "top-center" });
    }
  }, [copyText, t]);

  const hasUsage =
    !!msg.tokenUsage &&
    (msg.tokenUsage.inputTokens || 0) +
      (msg.tokenUsage.outputTokens || 0) +
      (msg.tokenUsage.cacheCreationTokens || 0) +
      (msg.tokenUsage.cacheReadTokens || 0) >
      0;

  if (!showActions && !hasUsage) return null;

  return (
    <div className="flex items-center w-full max-w-[95%] min-h-[18px] pl-0.5">
      <div className="flex items-center gap-2">
        {showActions && copyText && (
          <button
            type="button"
            className="opacity-0 group-hover/assistant:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
            onClick={handleCopy}
            title={t("action.copy", "复制")}
            aria-label={t("action.copy", "复制")}
          >
            <Copy className="h-3.5 w-3.5" />
          </button>
        )}
        {showActions && (
          <button
            type="button"
            className="opacity-0 group-hover/assistant:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
            onClick={() => onRegenerate(index)}
            title={t("ai.regenerate", "重新生成")}
            aria-label={t("ai.regenerate", "重新生成")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      {hasUsage && (
        <div className="ml-auto">
          <TokenUsageBadge usage={msg.tokenUsage!} />
        </div>
      )}
    </div>
  );
});

const AssistantMessage = memo(function AssistantMessage({
  msg,
  index,
  sending,
  onRegenerate,
}: {
  msg: ChatMessage;
  index: number;
  sending: boolean;
  onRegenerate: (index: number) => void;
}) {
  const hasBlocks = msg.blocks && msg.blocks.length > 0;
  const isEmpty = !hasBlocks && msg.content === "";

  if (msg.streaming && isEmpty) {
    return (
      <div className="flex flex-col items-start gap-1.5">
        <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
        <div className="rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] shadow-sm">
          <div className="flex items-center gap-1 py-1">
            <span
              className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
              style={{ animationDelay: "0ms" }}
            />
            <span
              className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
              style={{ animationDelay: "150ms" }}
            />
            <span
              className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
              style={{ animationDelay: "300ms" }}
            />
          </div>
        </div>
      </div>
    );
  }

  if (hasBlocks) {
    const segments = splitBlocksByApproval(msg.blocks);
    return (
      <div className="flex flex-col items-start gap-1.5 group/assistant">
        <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
        {segments.map((seg, si) =>
          seg.type === "approval" ? (
            <div key={si} className="w-full max-w-[95%]">
              <ApprovalBlock block={seg.blocks[0]} />
            </div>
          ) : (
            <BubbleSegment key={si} blocks={seg.blocks} streaming={msg.streaming && si === segments.length - 1} />
          )
        )}
        <AssistantToolbar msg={msg} index={index} sending={sending} onRegenerate={onRegenerate} />
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-1.5 group/assistant">
      <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
      <div
        className={`rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] min-w-0 overflow-hidden break-words prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto shadow-sm ${messageSelectionClass}`}
      >
        <Markdown remarkPlugins={mdRemarkPlugins} rehypePlugins={mdRehypePlugins}>
          {msg.content}
        </Markdown>
        {msg.streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1" />}
      </div>
      <AssistantToolbar msg={msg} index={index} sending={sending} onRegenerate={onRegenerate} />
    </div>
  );
});

const BubbleSegment = memo(function BubbleSegment({
  blocks,
  streaming,
}: {
  blocks: ContentBlock[];
  streaming?: boolean;
}) {
  const compactCtx = useCompact();
  const maxWidthClass = compactCtx ? "max-w-full" : "max-w-[95%]";
  return (
    <div
      className={`rounded-xl rounded-bl-sm bg-muted px-3.5 py-3 ${maxWidthClass} min-w-0 overflow-hidden shadow-sm space-y-2 ${messageSelectionClass}`}
    >
      {blocks.map((block, idx) =>
        block.type === "text" ? (
          <div
            key={idx}
            className="prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-pre:my-1 overflow-x-auto break-words"
          >
            <Markdown remarkPlugins={mdRemarkPlugins} rehypePlugins={mdRehypePlugins}>
              {block.content}
            </Markdown>
          </div>
        ) : block.type === "thinking" ? (
          <ThinkingBlock key={idx} block={block} />
        ) : block.type === "agent" ? (
          <AgentBlock key={idx} block={block} />
        ) : (
          <ToolBlock key={idx} block={block} />
        )
      )}
      {streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1 mb-1" />}
    </div>
  );
});
