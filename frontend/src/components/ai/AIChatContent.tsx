import { useState, useRef, useEffect, memo, useCallback, createContext, useContext } from "react";
import { Loader2, CornerDownLeft, Square, RefreshCw, X, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
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
} from "@opskat/ui";
import {
  useAIStore,
  useAISendOnEnter,
  type ChatMessage,
  type ContentBlock,
  type PendingQueueItem,
  type MentionRef,
} from "@/stores/aiStore";
import { AIChatInput, type AIChatInputHandle } from "@/components/ai/AIChatInput";
import { UserMessage } from "@/components/ai/UserMessage";
import { useTabStore, type AITabMeta } from "@/stores/tabStore";
import { ToolBlock } from "@/components/ai/ToolBlock";
import { ThinkingBlock } from "@/components/ai/ThinkingBlock";
import { AgentBlock } from "@/components/ai/AgentBlock";
import { ApprovalBlock } from "@/components/approval/ApprovalBlock";
import { AISetupWizard } from "@/components/ai/AISetupWizard";

// 常量化 Markdown 插件数组，避免每次渲染创建新引用导致 Markdown 重解析
const mdRemarkPlugins = [remarkGfm];
const mdRehypePlugins = [rehypeSanitize];

// 稳定引用的默认值，避免 zustand selector 每次返回新对象导致无限渲染
const EMPTY_MESSAGES: ChatMessage[] = [];
const DEFAULT_STREAMING = { sending: false, pendingQueue: [] as PendingQueueItem[] };

interface AIChatContentProps {
  tabId?: string;
  conversationId?: number | null;
  compact?: boolean;
  /** Optional: if provided, replaces the default sendToTab-based send path. */
  onSendOverride?: (content: string, mentions?: MentionRef[]) => Promise<void>;
  /** Optional: if provided, replaces the default stopGeneration-based stop path. */
  onStopOverride?: () => Promise<void>;
}

const CompactContext = createContext(false);
export function useCompact() {
  return useContext(CompactContext);
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
  conversationId: propConvId,
  compact = false,
  onSendOverride,
  onStopOverride,
}: AIChatContentProps) {
  const { t } = useTranslation();
  const { configured, sendToTab, stopGeneration, regenerate, removeFromQueue, clearQueue } = useAIStore();
  const derivedConvId = useTabStore((s) => {
    if (!tabId) return null;
    const tab = s.tabs.find((x) => x.id === tabId);
    return tab ? (tab.meta as AITabMeta).conversationId : null;
  });
  const conversationId = propConvId ?? derivedConvId;

  const messages = useAIStore((s) =>
    conversationId != null ? s.conversationMessages[conversationId] || EMPTY_MESSAGES : EMPTY_MESSAGES
  );
  const streaming = useAIStore((s) =>
    conversationId != null ? s.conversationStreaming[conversationId] || DEFAULT_STREAMING : DEFAULT_STREAMING
  );
  const { sending, pendingQueue } = streaming;

  const [regenerateTarget, setRegenerateTarget] = useState<number | null>(null);
  const [empty, setEmpty] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<AIChatInputHandle>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [tabId]);

  const handleSend = useCallback(
    (text: string, mentions: MentionRef[]) => {
      const trimmed = text.trim();
      if (!trimmed && mentions.length === 0) return;
      if (onSendOverride) {
        void onSendOverride(text, mentions.length > 0 ? mentions : undefined);
      } else if (tabId) {
        sendToTab(tabId, text, mentions.length > 0 ? mentions : undefined);
      }
    },
    [onSendOverride, sendToTab, tabId]
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

  const confirmRegenerate = () => {
    if (regenerateTarget !== null) {
      if (tabId) {
        regenerate(tabId, regenerateTarget);
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
        <ScrollArea className="flex-1 min-h-0 overflow-hidden">
          <div className="max-w-3xl mx-auto p-4 space-y-6">
            {messages.length === 0 && (
              <p className="text-sm text-muted-foreground text-center mt-16">{t("ai.placeholder")}</p>
            )}
            {messages.map((msg, i) => {
              const isLast = i === messages.length - 1;
              // 最后一条（流式目标）不启用 content-visibility，避免流式过程中被浏览器延迟渲染
              const cvStyle: React.CSSProperties | undefined = isLast
                ? undefined
                : { contentVisibility: "auto", containIntrinsicSize: "auto 120px" };
              return (
                <div key={i} className="text-sm" style={cvStyle}>
                  {msg.role === "user" ? (
                    <UserMessage msg={msg} />
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
              <AIChatInput
                ref={inputRef}
                onSubmit={handleSend}
                onEmptyChange={setEmpty}
                sendOnEnter={sendOnEnter}
                placeholder={t("ai.sendPlaceholder")}
              />
              <div className="flex items-center justify-between px-3 pb-2">
                <span className="text-xs text-muted-foreground/40 select-none">
                  {sendOnEnter
                    ? `Enter ${t("ai.sendShortcutHint")}`
                    : `${/mac/i.test(navigator.userAgent) ? "⌘+Enter" : "Ctrl+Enter"} ${t("ai.sendShortcutHint")}`}
                </span>
                {sending ? (
                  <Button size="icon" variant="destructive" className="h-7 w-7 shrink-0 rounded-lg" onClick={handleStop}>
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
  const { t } = useTranslation();
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

  const showRegenerate = !sending && !msg.streaming;

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
        {showRegenerate && (
          <button
            className="opacity-0 group-hover/assistant:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
            onClick={() => onRegenerate(index)}
            title={t("ai.regenerate", "重新生成")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-1.5 group/assistant">
      <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
      <div className="rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] min-w-0 overflow-hidden break-words prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto shadow-sm">
        <Markdown remarkPlugins={mdRemarkPlugins} rehypePlugins={mdRehypePlugins}>
          {msg.content}
        </Markdown>
        {msg.streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1" />}
      </div>
      {showRegenerate && (
        <button
          className="opacity-0 group-hover/assistant:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
          onClick={() => onRegenerate(index)}
          title={t("ai.regenerate", "重新生成")}
        >
          <RefreshCw className="h-3.5 w-3.5" />
        </button>
      )}
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
      className={`rounded-xl rounded-bl-sm bg-muted px-3.5 py-3 ${maxWidthClass} min-w-0 overflow-hidden shadow-sm space-y-2`}
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
