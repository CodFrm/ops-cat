import { useState, useRef, useEffect, useMemo, memo, useCallback, useDeferredValue } from "react";
import { useShallow } from "zustand/shallow";
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
  type TokenUsage,
} from "@/stores/aiStore";
import { AIChatInput, type AIChatInputDraft, type AIChatInputHandle } from "@/components/ai/AIChatInput";
import { UserMessage } from "@/components/ai/UserMessage";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";
import { formatModKey } from "@/stores/shortcutStore";
import { ToolBlock } from "@/components/ai/ToolBlock";
import { ThinkingBlock } from "@/components/ai/ThinkingBlock";
import { AgentBlock } from "@/components/ai/AgentBlock";
import { ErrorBlock as ErrorBlockView } from "@/components/ai/ErrorBlock";
import { RetryBanner } from "@/components/ai/RetryBanner";
import { ApprovalBlock } from "@/components/approval/ApprovalBlock";
import { AISetupWizard } from "@/components/ai/AISetupWizard";
import { CompactContext, useCompact } from "@/components/ai/AIChatContentContext";

// 常量化 Markdown 插件数组，避免每次渲染创建新引用导致 Markdown 重解析
const mdRemarkPlugins = [remarkGfm];
const mdRehypePlugins = [rehypeSanitize];

// 流式输出时整段 markdown 会按 RAF 频率重新塞进 <Markdown>。
// react-markdown 是同步主线程解析，长文本下会直接卡住键盘 IME。
// 这里加两层保护：
//   1. memo 让"内容没变"的历史 block 一次也不重渲染（参数 content 是稳定字符串引用）。
//   2. useDeferredValue 让正在增长的最后一个 block 在主线程有更高优先级任务（按键）时
//      自动让位，等空闲帧再渲染——React 19 并发渲染会自然兜住延迟。
const MarkdownContent = memo(function MarkdownContent({ content }: { content: string }) {
  const deferred = useDeferredValue(content);
  return (
    <Markdown remarkPlugins={mdRemarkPlugins} rehypePlugins={mdRehypePlugins}>
      {deferred}
    </Markdown>
  );
});
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
  onSendOverride?: (content: string) => Promise<void>;
  /** Optional: if provided, replaces the default stopGeneration-based stop path. */
  onStopOverride?: () => Promise<void>;
}

interface EditTarget {
  conversationId: number;
  messageIndex: number;
  draft: AIChatInputDraft;
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
  // 只订阅必要的标量字段：之前 `useAIStore()` 整体解构会让本组件订阅整张 store，
  // 任何其它会话的流式 chunk、conversation list 增删、sidebar tab 抖动都会触发重渲染。
  const configured = useAIStore((s) => s.configured);
  // Actions 在 store 初始化后引用稳定；useShallow 让对象级浅比较恒为 true，永不触发重渲染。
  const {
    sendToTab,
    stopGeneration,
    regenerate,
    regenerateConversation,
    removeFromQueue,
    clearQueue,
    editAndResendConversation,
    setSidebarTabInputDraft,
    setSidebarTabEditTarget,
    setSidebarTabScrollTop,
  } = useAIStore(
    useShallow((s) => ({
      sendToTab: s.sendToTab,
      stopGeneration: s.stopGeneration,
      regenerate: s.regenerate,
      regenerateConversation: s.regenerateConversation,
      removeFromQueue: s.removeFromQueue,
      clearQueue: s.clearQueue,
      editAndResendConversation: s.editAndResendConversation,
      setSidebarTabInputDraft: s.setSidebarTabInputDraft,
      setSidebarTabEditTarget: s.setSidebarTabEditTarget,
      setSidebarTabScrollTop: s.setSidebarTabScrollTop,
    }))
  );
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
  // 直接走 zustand 选择器 + 浅比较：流式只改最后一条 assistant 消息时，
  // 用户消息序列保持引用稳定，AIChatInput 不会因为父组件每帧重渲染而被波及。
  const userMessageHistory = useAIStore(
    useShallow((s) => {
      const msgs = conversationId != null ? s.conversationMessages[conversationId] || EMPTY_MESSAGES : EMPTY_MESSAGES;
      const history: string[] = [];
      for (let i = msgs.length - 1; i >= 0; i--) {
        const msg = msgs[i];
        if (msg.role === "user" && msg.content.trim()) {
          history.push(msg.content);
        }
      }
      return history;
    })
  );

  const [regenerateTarget, setRegenerateTarget] = useState<number | null>(null);
  const [localEditTarget, setLocalEditTarget] = useState<EditTarget | null>(null);
  const [empty, setEmpty] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<AIChatInputHandle>(null);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  // 内层 messages 容器：ResizeObserver 挂在这里，捕获 useDeferredValue commit 撑高的那一帧。
  const contentRef = useRef<HTMLDivElement>(null);
  // Radix ScrollArea 的 viewport 是内部 div，原先每个 effect 都 querySelector 一次；
  // 流式时 messages effect 按帧触发，querySelector 累积成本不小。这里把首次查找结果缓存，
  // 同时用 isConnected 兜底 Radix 重建子树的边角场景。
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const getViewport = useCallback((): HTMLDivElement | null => {
    if (viewportRef.current?.isConnected) return viewportRef.current;
    const v = scrollAreaRef.current?.querySelector("[data-radix-scroll-area-viewport]") as HTMLDivElement | null;
    viewportRef.current = v;
    return v;
  }, []);
  const previousConversationIdRef = useRef<number | null | undefined>(conversationId);
  // 跟随滚动开关:用户在(接近)底部时为 true,流式追加内容会自动滚到底;
  // 用户主动向上滚动后变为 false,后续消息不再打断阅读位置。重新滚到底部即恢复跟随。
  const isAtBottomRef = useRef(true);
  // 已经因为出现而自动滚动过的 approval confirmId 集合,避免对同一个审批反复抢回视图。
  const seenApprovalIdsRef = useRef<Set<string>>(new Set());
  const editTarget = sideTabId ? sidebarEditTarget : localEditTarget;

  const enableScrollFollow = useCallback(() => {
    isAtBottomRef.current = true;
    const viewport = getViewport();
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight;
    } else {
      bottomRef.current?.scrollIntoView({ behavior: "auto" });
    }
  }, [getViewport]);

  useEffect(() => {
    seenApprovalIdsRef.current = new Set();
  }, [conversationId]);

  useEffect(() => {
    // 新出现待确认的审批一定要把视图拉回底部,审批是阻塞动作,用户必须看到。
    let hasNewApproval = false;
    for (const msg of messages) {
      if (!msg.blocks) continue;
      for (const block of msg.blocks) {
        if (block.type !== "approval" || block.status !== "pending_confirm" || !block.confirmId) continue;
        if (!seenApprovalIdsRef.current.has(block.confirmId)) {
          seenApprovalIdsRef.current.add(block.confirmId);
          hasNewApproval = true;
        }
      }
    }
    if (!hasNewApproval && !isAtBottomRef.current) return;
    const viewport = getViewport();
    if (viewport) {
      // 这里读到的 scrollHeight 可能还是 useDeferredValue commit 之前的旧值（MarkdownContent
      // 内部把 markdown 渲染挂到了低优先级 transition），所以这次滚到底只对齐到"旧底部"；
      // 下面那条 ResizeObserver effect 会在内容容器真正撑高的那一帧再把视图二次对齐到新底部。
      viewport.scrollTop = viewport.scrollHeight;
      isAtBottomRef.current = true;
    } else {
      bottomRef.current?.scrollIntoView({ behavior: "auto" });
    }
  }, [messages, getViewport]);

  useEffect(() => {
    // MarkdownContent 用 useDeferredValue 让长 markdown 走低优先级 commit，
    // 上面那条 messages effect 跑的时候 deferred 还没 commit、scrollHeight 是旧值；
    // 而 deferred 真正 commit 时既没有 scroll 事件、也不会改变 messages 引用，
    // 没有任何同步信号能把视图重新拉到底——只剩内容容器尺寸变化这个回调入口。
    // ResizeObserver 在内层 messages 容器尺寸变化时（包括每次 deferred commit 撑高）
    // 触发一次，加上 isAtBottomRef 闸门，保证用户上滑后跟随会被关掉，不与 scroll 处理器抢镜。
    const viewport = getViewport();
    const content = contentRef.current;
    if (!viewport || !content || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => {
      if (!isAtBottomRef.current) return;
      // scrollTop 的最大值是 scrollHeight - clientHeight，不是 scrollHeight；
      // 用 maxScrollTop 比较，避免每次尺寸变化都无效重写 scrollTop。
      const maxScrollTop = viewport.scrollHeight - viewport.clientHeight;
      if (viewport.scrollTop < maxScrollTop) {
        viewport.scrollTop = maxScrollTop;
      }
    });
    ro.observe(content);
    return () => ro.disconnect();
  }, [getViewport]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [sideTabId, tabId]);

  useEffect(() => {
    if (!sideTabId) return;
    // 侧边助手：切换 side tab 或刚绑定到新 conversation 时，恢复各自保存的 draft。
    // 通过 getState() 一次性读取，不订阅 inputDraft —— 否则每次按键都会重跑该 effect。
    const uiState = useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState;
    inputRef.current?.loadDraft(uiState?.inputDraft ?? { content: "" });
  }, [conversationId, sideTabId]);

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
          setSidebarTabInputDraft(sideTabId, { content: "" });
        }
      }
    },
    [localEditTarget, setSidebarTabEditTarget, setSidebarTabInputDraft, sideTabId]
  );

  useEffect(() => {
    if (sideTabId) return;
    if (previousConversationIdRef.current === conversationId) return;
    previousConversationIdRef.current = conversationId;
    if (!editTarget) return;
    let cancelled = false;
    queueMicrotask(() => {
      if (cancelled) return;
      resetEditMode({ clearDraft: true });
    });
    return () => {
      cancelled = true;
    };
  }, [conversationId, editTarget, resetEditMode, sideTabId]);

  useEffect(() => {
    // 会话消息被刷新、截断或替换后，如果编辑目标不再匹配当前消息，就立即退出编辑态。
    if (!editTarget) return;
    const targetMessage = messages[editTarget.messageIndex];
    const shouldReset =
      editTarget.conversationId !== conversationId ||
      !targetMessage ||
      targetMessage.role !== "user" ||
      targetMessage.content !== editTarget.draft.content;
    if (!shouldReset) return;
    let cancelled = false;
    queueMicrotask(() => {
      if (cancelled) return;
      resetEditMode({ clearDraft: true });
    });
    return () => {
      cancelled = true;
    };
  }, [conversationId, editTarget, messages, resetEditMode]);

  useEffect(() => {
    const viewport = getViewport();
    if (!viewport) return;
    // 滚动位置同样按宿主维度恢复，保证多个侧边 tab 来回切换时各自停在原来的阅读位置。
    if (sideTabId) {
      viewport.scrollTop =
        useAIStore.getState().sidebarTabs.find((tab) => tab.id === sideTabId)?.uiState.scrollTop ?? 0;
    }
    let lastScrollTop = viewport.scrollTop;
    let lastScrollHeight = viewport.scrollHeight;
    // 初始按几何判定一次：短内容/刚加载都视为在底部、开启跟随。
    isAtBottomRef.current = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= 50;
    const handleScroll = () => {
      const currentTop = viewport.scrollTop;
      const currentScrollHeight = viewport.scrollHeight;
      // 真正的"用户上滑"必须同时满足：scrollTop 减小 AND scrollHeight 没缩。
      // 内容收缩场景（典型：ThinkingBlock 从 running 完成时自动 collapse，max-h-64 展开内容消失）
      // 浏览器会把 scrollTop 钳到新 max → 看起来 scrollTop 减小但用户没动，
      // 误判会把跟随关掉、整段后续流式都无法回到底。
      const userScrolledUp = currentTop < lastScrollTop && currentScrollHeight >= lastScrollHeight;
      if (userScrolledUp) {
        isAtBottomRef.current = false;
      } else if (currentScrollHeight - currentTop - viewport.clientHeight <= 50) {
        isAtBottomRef.current = true;
      }
      lastScrollTop = currentTop;
      lastScrollHeight = currentScrollHeight;
      if (sideTabId) {
        setSidebarTabScrollTop(sideTabId, currentTop);
      }
    };
    viewport.addEventListener("scroll", handleScroll, { passive: true });
    return () => viewport.removeEventListener("scroll", handleScroll);
  }, [conversationId, getViewport, setSidebarTabScrollTop, sideTabId]);

  const handleSend = useCallback(
    (content: string) => {
      if (!content.trim()) return;
      if (editTarget && conversationId != null) {
        const activeTarget = editTarget;
        enableScrollFollow();
        // 编辑模式改走 conversation 级 replay，提交成功后只在目标仍未变化时退出编辑态。
        void editAndResendConversation(conversationId, activeTarget.messageIndex, content).then(() => {
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
      // 普通发送在当前会话空闲时会立即开启新一轮输出，视图应重新进入底部跟随。
      // 若 sending=true，本次提交只会进入 pending queue，保留用户当前阅读位置。
      if (!sending) {
        enableScrollFollow();
      }
      if (onSendOverride) {
        void onSendOverride(content);
      } else if (tabId) {
        sendToTab(tabId, content);
      }
    },
    [
      conversationId,
      editAndResendConversation,
      editTarget,
      enableScrollFollow,
      onSendOverride,
      sendToTab,
      setSidebarTabEditTarget,
      sideTabId,
      sending,
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
      const draft: AIChatInputDraft = { content: msg.content };
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

  // 草稿写回侧边态。useCallback 提供稳定引用，否则 AIChatInput memo 失效，
  // 父组件每次流式重渲染都会让输入框跟着重渲染。
  const handleDraftChange = useCallback(
    (draft: AIChatInputDraft) => {
      if (sideTabId) {
        setSidebarTabInputDraft(sideTabId, draft);
      }
    },
    [setSidebarTabInputDraft, sideTabId]
  );

  const confirmRegenerate = () => {
    if (regenerateTarget !== null) {
      enableScrollFollow();
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
          <div ref={contentRef} className="max-w-3xl mx-auto p-4 space-y-6">
            {messages.length === 0 && (
              <p className="text-sm text-muted-foreground text-center mt-16">{t("ai.placeholder")}</p>
            )}
            {messages.map((msg, i) => {
              const isLast = i === messages.length - 1;
              // 最后一条（流式目标）不启用 content-visibility，避免流式过程中被浏览器延迟渲染
              const cvStyle: React.CSSProperties | undefined = isLast
                ? undefined
                : { contentVisibility: "auto", containIntrinsicSize: "auto 120px" };
              // 必须用稳定的 msg.id 作 key：edit&resend / queue_consumed 会截断重插消息，
              // 用 index 作 key 时 React 会复用错误节点，ToolBlock/ThinkingBlock 的 expanded
              // 等本地 state 会串到别的消息。源头创建处均分配 id；测试 fixture 走 fallback。
              const key = msg.id ?? `idx-${i}-${msg.role}`;
              return (
                <div key={key} className="text-sm" style={cvStyle}>
                  {msg.role === "user" ? (
                    <UserMessage msg={msg} index={i} onEdit={handleEditMessage} />
                  ) : (
                    <AssistantMessage msg={msg} index={i} sending={sending} onRegenerate={handleRegenerate} />
                  )}
                </div>
              );
            })}
            <div ref={bottomRef} />
          </div>
        </ScrollArea>

        {/* Pending Queue */}
        {pendingQueue.length > 0 && (
          <div className="border-t px-3 py-2 bg-muted/30">
            <div className="max-w-3xl mx-auto">
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs text-muted-foreground">
                  {t("ai.pendingMessages")} ({pendingQueue.length})
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
                  {t("ai.clearQueue")}
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
                    <p className="text-xs font-medium text-foreground">{t("ai.editingMessage")}</p>
                    <p className="text-xs text-muted-foreground">{t("ai.editResendHint")}</p>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 shrink-0 px-2 text-xs"
                    onClick={() => resetEditMode({ clearDraft: true })}
                  >
                    {t("ai.cancelEdit")}
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
              <AlertDialogTitle>{t("ai.regenerateTitle")}</AlertDialogTitle>
              <AlertDialogDescription>{t("ai.regenerateConfirm")}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("action.cancel")}</AlertDialogCancel>
              <AlertDialogAction onClick={confirmRegenerate}>{t("action.confirm")}</AlertDialogAction>
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
  if (input === 0 && output === 0 && cacheWrite === 0 && cacheRead === 0) return null;
  const hasCache = cacheRead > 0 || cacheWrite > 0;

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground/70 tabular-nums select-none cursor-default">
            <span className="inline-flex items-center gap-0.5">
              <ArrowUp className="h-3 w-3" />
              {formatTokenCount(input)}
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
              <span className="text-muted-foreground">{t("ai.tokenUsage.input")}</span>
              <span>{input.toLocaleString()}</span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground">{t("ai.tokenUsage.output")}</span>
              <span>{output.toLocaleString()}</span>
            </div>
            {cacheWrite > 0 && (
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("ai.tokenUsage.cacheWrite")}</span>
                <span>{cacheWrite.toLocaleString()}</span>
              </div>
            )}
            {cacheRead > 0 && (
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("ai.tokenUsage.cacheRead")}</span>
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
  // 复制按钮即使不显示也会读 msg；流式时上层 toolbar 会因为 showActions=false 提前 return，
  // 不真正用到这里的结果。useMemo 让 toolbar 重渲染时不重做拼接，msg.blocks 引用稳定就直接命中。
  const copyText = useMemo(() => extractAssistantText(msg), [msg]);

  const handleCopy = useCallback(async () => {
    if (!copyText) return;
    try {
      await navigator.clipboard.writeText(copyText);
      toast.success(t("ai.copied"), { duration: 1500, position: "top-center" });
    } catch {
      toast.error(t("ai.copyFailed"), { duration: 2000, position: "top-center" });
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
            title={t("action.copy")}
            aria-label={t("action.copy")}
          >
            <Copy className="h-3.5 w-3.5" />
          </button>
        )}
        {showActions && (
          <button
            type="button"
            className="opacity-0 group-hover/assistant:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
            onClick={() => onRegenerate(index)}
            title={t("ai.regenerate")}
            aria-label={t("ai.regenerate")}
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

export const AssistantMessage = memo(function AssistantMessage({
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
  // Hooks 规则要求在条件分支之前调用：用空数组占位，下方按 hasBlocks 决定要不要取用。
  // 流式时 msg.blocks 引用每帧改变会重算，但 splitBlocksByApproval 本身是 O(blocks.length) 的纯遍历，
  // useMemo 至少避免 toolbar / 其他渲染触发的二次执行。
  const segments = useMemo(() => splitBlocksByApproval(msg.blocks ?? []), [msg.blocks]);

  if (msg.streaming && isEmpty) {
    return (
      <div className="flex flex-col items-start gap-1.5">
        <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
        {/* 503 等错误在 cago 同步路径下 retry，此时 assistant 占位 blocks 为空 +
            content="" → 命中本分支。RetryBanner 必须在这里也渲染，否则 retry 期间
            (1s/2s/4s 倒计时) UI 看不到任何"重试中"提示，最终只看到 ErrorBlock。 */}
        {msg.retryStatus && <RetryBanner status={msg.retryStatus} />}
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
    return (
      <div className="flex flex-col items-start gap-1.5 group/assistant">
        <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
        {msg.retryStatus && <RetryBanner status={msg.retryStatus} />}
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
      {msg.retryStatus && <RetryBanner status={msg.retryStatus} />}
      <div
        className={`rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] min-w-0 overflow-hidden break-words prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto shadow-sm ${messageSelectionClass}`}
      >
        <MarkdownContent content={msg.content} />
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
            <MarkdownContent content={block.content} />
          </div>
        ) : block.type === "thinking" ? (
          <ThinkingBlock key={idx} block={block} />
        ) : block.type === "agent" ? (
          <AgentBlock key={idx} block={block} />
        ) : block.type === "error" ? (
          <ErrorBlockView key={idx} block={block} />
        ) : (
          <ToolBlock key={idx} block={block} />
        )
      )}
      {streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1 mb-1" />}
    </div>
  );
});
