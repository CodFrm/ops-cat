import { useState, useRef, useEffect, useCallback } from "react";
import {
  Send,
  PanelRightClose,
  Settings2,
  Loader2,
  Bot,
  Plus,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import Markdown from "react-markdown";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useAIStore, type ChatMessage } from "@/stores/aiStore";
import { useFullscreen } from "@/hooks/useFullscreen";
import { ToolBlock } from "@/components/ai/ToolBlock";
import { ConversationList } from "@/components/ai/ConversationList";

const AI_PANEL_MIN_WIDTH = 280;
const AI_PANEL_MAX_WIDTH = 640;
const AI_PANEL_DEFAULT_WIDTH = 320;

interface AIPanelProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function AIPanel({ collapsed, onToggle }: AIPanelProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { messages, sending, configured, send, clear } = useAIStore();
  const [input, setInput] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(() => {
    const saved = localStorage.getItem("ai_panel_width");
    return saved ? Math.max(AI_PANEL_MIN_WIDTH, Math.min(AI_PANEL_MAX_WIDTH, Number(saved))) : AI_PANEL_DEFAULT_WIDTH;
  });
  const [resizing, setResizing] = useState(false);

  const handleResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setResizing(true);
    const startX = e.clientX;
    const startWidth = width;

    const onMouseMove = (ev: MouseEvent) => {
      const delta = startX - ev.clientX;
      const newWidth = Math.max(AI_PANEL_MIN_WIDTH, Math.min(AI_PANEL_MAX_WIDTH, startWidth + delta));
      setWidth(newWidth);
    };

    const onMouseUp = () => {
      setResizing(false);
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      // persist on drag end
      setWidth((w) => {
        localStorage.setItem("ai_panel_width", String(w));
        return w;
      });
    };

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  }, [width]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  if (collapsed) return null;

  const handleSend = () => {
    const text = input.trim();
    if (!text || sending) return;
    setInput("");
    send(text);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div
      className="relative flex h-full shrink-0 flex-col border-l border-panel-divider"
      style={{ width }}
    >
      {/* Resize handle */}
      <div
        className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
        onMouseDown={handleResizeStart}
      />
      {/* Overlay to prevent iframe/selection interference while dragging */}
      {resizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}
      {/* Drag region for frameless window */}
      <div
        className={`${isFullscreen ? "h-2" : "h-10"} w-full shrink-0`}
        style={{ "--wails-draggable": "drag" } as React.CSSProperties}
      />
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-panel-divider">
        <div className="flex items-center gap-1.5">
          <Bot className="h-3.5 w-3.5 text-primary" />
          <span className="text-sm font-medium">{t("ai.title")}</span>
        </div>
        <div className="flex gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={clear}
            title={t("ai.newConversation", "新对话")}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <ConversationList />
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={onToggle}
          >
            <PanelRightClose className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Not configured */}
      {!configured && (
        <div className="flex-1 flex items-center justify-center p-4">
          <div className="text-center space-y-2">
            <Settings2 className="h-8 w-8 text-muted-foreground mx-auto" />
            <p className="text-sm text-muted-foreground">
              {t("ai.notConfigured")}
            </p>
          </div>
        </div>
      )}

      {/* Messages */}
      {configured && (
        <ScrollArea className="flex-1 min-h-0">
          <div className="p-3 space-y-3">
            {messages.length === 0 && (
              <p className="text-sm text-muted-foreground text-center mt-8">
                {t("ai.placeholder")}
              </p>
            )}
            {messages.map((msg, i) => (
              <div
                key={i}
                className={`text-sm ${
                  msg.role === "user" ? "text-right" : ""
                }`}
              >
                {msg.role === "user" ? (
                  <div className="inline-block rounded-xl rounded-br-sm bg-primary px-3 py-2 text-primary-foreground max-w-[85%] text-left shadow-sm">
                    {msg.content}
                  </div>
                ) : (
                  <AssistantMessage msg={msg} />
                )}
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        </ScrollArea>
      )}

      {/* Input */}
      {configured && (
        <div className="border-t p-3">
          <div className="flex gap-2">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t("ai.placeholder")}
              rows={1}
              className="flex-1 max-h-32 rounded-xl border border-input bg-background px-3 py-2 text-sm outline-none focus:border-ring focus:ring-1 focus:ring-ring/50 resize-none transition-colors duration-150 placeholder:text-muted-foreground/60"
              style={{ fieldSizing: "content" } as React.CSSProperties}
            />
            <Button
              size="icon"
              className="h-9 w-9 shrink-0 rounded-xl"
              onClick={handleSend}
              disabled={sending || !input.trim()}
            >
              {sending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Send className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

// Assistant 消息：渲染结构化内容块
function AssistantMessage({ msg }: { msg: ChatMessage }) {
  const hasBlocks = msg.blocks && msg.blocks.length > 0;
  const isEmpty = !hasBlocks && msg.content === "";

  // 等待中：三个跳动点
  if (msg.streaming && isEmpty) {
    return (
      <div className="rounded-xl rounded-bl-sm bg-muted/60 border border-border/50 px-3 py-2 max-w-[95%] shadow-sm">
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
    );
  }

  // 有 blocks 时渲染结构化内容
  if (hasBlocks) {
    return (
      <div className="rounded-xl rounded-bl-sm bg-muted/60 border border-border/50 px-3 py-2 max-w-[95%] overflow-hidden shadow-sm">
        {msg.blocks.map((block, idx) =>
          block.type === "text" ? (
            <div
              key={idx}
              className="prose prose-sm dark:prose-invert prose-p:my-1 prose-pre:my-1 overflow-x-auto"
            >
              <Markdown>{block.content}</Markdown>
            </div>
          ) : (
            <ToolBlock key={idx} block={block} />
          )
        )}
        {msg.streaming && (
          <Loader2 className="h-3 w-3 animate-spin inline-block ml-1 mb-1" />
        )}
      </div>
    );
  }

  // 回退：纯文本 markdown
  return (
    <div className="rounded-xl rounded-bl-sm bg-muted/60 border border-border/50 px-3 py-2 max-w-[95%] overflow-hidden prose prose-sm dark:prose-invert prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto shadow-sm">
      <Markdown>{msg.content}</Markdown>
      {msg.streaming && (
        <Loader2 className="h-3 w-3 animate-spin inline-block ml-1" />
      )}
    </div>
  );
}
