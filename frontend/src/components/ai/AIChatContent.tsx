import { useState, useRef, useEffect } from "react";
import { useIMEComposing } from "@/hooks/useIMEComposing";
import { Bot, Loader2, CornerDownLeft, Settings2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import Markdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useAIStore, useAISendOnEnter, type ChatMessage, type ContentBlock } from "@/stores/aiStore";
import { ToolBlock } from "@/components/ai/ToolBlock";
import { AgentBlock } from "@/components/ai/AgentBlock";
import { ApprovalBlock } from "@/components/approval/ApprovalBlock";
import { AISetupWizard } from "@/components/ai/AISetupWizard";

interface AIChatContentProps {
  tabId: string;
}

/** Split blocks into segments: consecutive non-approval blocks form a 'bubble' segment,
 *  each approval block becomes its own 'approval' segment. */
function splitBlocksByApproval(blocks: ContentBlock[]): Array<{ type: "bubble" | "approval"; blocks: ContentBlock[] }> {
  const segments: Array<{ type: "bubble" | "approval"; blocks: ContentBlock[] }> = [];
  let currentBubble: ContentBlock[] = [];

  for (const block of blocks) {
    if (block.type === "approval") {
      if (currentBubble.length > 0) {
        segments.push({ type: "bubble", blocks: currentBubble });
        currentBubble = [];
      }
      segments.push({ type: "approval", blocks: [block] });
    } else {
      currentBubble.push(block);
    }
  }
  if (currentBubble.length > 0) {
    segments.push({ type: "bubble", blocks: currentBubble });
  }
  return segments;
}

export function AIChatContent({ tabId }: AIChatContentProps) {
  const { t } = useTranslation();
  const { configured, sendToTab } = useAIStore();
  const tabState = useAIStore((s) => s.tabStates[tabId]) || {
    messages: [],
    sending: false,
  };
  const { messages, sending } = tabState;
  const modelName = useAIStore((s) => s.modelName);

  const [input, setInput] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const { isComposing, onCompositionStart, onCompositionEnd } = useIMEComposing();

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  useEffect(() => {
    textareaRef.current?.focus();
  }, [tabId]);

  const handleSend = () => {
    const text = input.trim();
    if (!text || sending) return;
    setInput("");
    sendToTab(tabId, text);
  };

  const sendOnEnter = useAISendOnEnter();

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (isComposing()) return;
    if (sendOnEnter) {
      if (e.key === "Enter" && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    } else {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        handleSend();
      }
    }
  };

  if (!configured) {
    return <AISetupWizard />;
  }

  return (
    <div className="flex h-full flex-col">
      {/* Chat Header */}
      <div className="flex items-center justify-between px-5 h-12 border-b shrink-0">
        <div className="flex items-center gap-2.5">
          <Bot className="h-[18px] w-[18px] text-primary" />
          <span className="text-sm font-semibold">{t("ai.title")}</span>
          {modelName && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-muted px-2.5 h-[22px] text-[11px] text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-green-500" />
              {modelName}
            </span>
          )}
        </div>
        <Button variant="ghost" size="icon" className="h-8 w-8">
          <Settings2 className="h-4 w-4 text-muted-foreground" />
        </Button>
      </div>

      {/* Messages */}
      <ScrollArea className="flex-1 min-h-0 overflow-hidden">
        <div className="max-w-3xl mx-auto p-4 space-y-6">
          {messages.length === 0 && (
            <p className="text-sm text-muted-foreground text-center mt-16">{t("ai.placeholder")}</p>
          )}
          {messages.map((msg, i) => (
            <div key={i} className="text-sm">
              {msg.role === "user" ? (
                <UserMessage msg={msg} />
              ) : (
                <AssistantMessage msg={msg} />
              )}
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>

      {/* Input */}
      <div className="border-t p-3">
        <div className="max-w-3xl mx-auto">
          <div className="rounded-xl border border-input bg-background transition-colors duration-150 focus-within:border-ring focus-within:ring-1 focus-within:ring-ring/50">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              onCompositionStart={onCompositionStart}
              onCompositionEnd={onCompositionEnd}
              placeholder={t("ai.sendPlaceholder")}
              rows={2}
              className="w-full max-h-[25vh] rounded-t-xl bg-transparent px-3 pt-3 pb-1 text-sm outline-none resize-none placeholder:text-muted-foreground/60"
              style={{ fieldSizing: "content" } as React.CSSProperties}
            />
            <div className="flex items-center justify-between px-3 pb-2">
              <span className="text-xs text-muted-foreground/40 select-none">
                {sendOnEnter
                  ? `Enter ${t("ai.sendShortcutHint")}`
                  : `${/mac/i.test(navigator.userAgent) ? "⌘+Enter" : "Ctrl+Enter"} ${t("ai.sendShortcutHint")}`}
              </span>
              <Button
                size="icon"
                className="h-7 w-7 shrink-0 rounded-lg"
                onClick={handleSend}
                disabled={sending || !input.trim()}
              >
                {sending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <CornerDownLeft className="h-3.5 w-3.5" />
                )}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function UserMessage({ msg }: { msg: ChatMessage }) {
  return (
    <div className="flex flex-col items-end gap-1.5">
      <span className="text-xs font-semibold text-muted-foreground tracking-wide">You</span>
      <div className="inline-block rounded-xl rounded-br-sm bg-primary px-3.5 py-2.5 text-primary-foreground max-w-[85%] text-left shadow-sm break-words">
        {msg.content}
      </div>
    </div>
  );
}

function AssistantMessage({ msg }: { msg: ChatMessage }) {
  const hasBlocks = msg.blocks && msg.blocks.length > 0;
  const isEmpty = !hasBlocks && msg.content === "";

  if (msg.streaming && isEmpty) {
    return (
      <div className="flex flex-col items-start gap-1.5">
        <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
        <div className="rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] shadow-sm">
          <div className="flex items-center gap-1 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce" style={{ animationDelay: "0ms" }} />
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce" style={{ animationDelay: "150ms" }} />
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce" style={{ animationDelay: "300ms" }} />
          </div>
        </div>
      </div>
    );
  }

  if (hasBlocks) {
    const segments = splitBlocksByApproval(msg.blocks);
    return (
      <div className="flex flex-col items-start gap-1.5">
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
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-1.5">
      <span className="text-xs font-semibold text-primary tracking-wide">Assistant</span>
      <div className="rounded-xl rounded-bl-sm bg-muted px-3.5 py-2.5 max-w-[95%] min-w-0 overflow-hidden break-words prose prose-sm dark:prose-invert prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto shadow-sm">
        <Markdown rehypePlugins={[rehypeSanitize]}>{msg.content}</Markdown>
        {msg.streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1" />}
      </div>
    </div>
  );
}

function BubbleSegment({ blocks, streaming }: { blocks: ContentBlock[]; streaming?: boolean }) {
  return (
    <div className="rounded-xl rounded-bl-sm bg-muted px-3.5 py-3 max-w-[95%] min-w-0 overflow-hidden shadow-sm space-y-2">
      {blocks.map((block, idx) =>
        block.type === "text" ? (
          <div key={idx} className="prose prose-sm dark:prose-invert prose-p:my-1 prose-pre:my-1 overflow-x-auto break-words">
            <Markdown rehypePlugins={[rehypeSanitize]}>{block.content}</Markdown>
          </div>
        ) : block.type === "agent" ? (
          <AgentBlock key={idx} block={block} />
        ) : (
          <ToolBlock key={idx} block={block} />
        )
      )}
      {streaming && <Loader2 className="h-3 w-3 animate-spin inline-block ml-1 mb-1" />}
    </div>
  );
}
