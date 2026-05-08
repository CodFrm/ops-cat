import { memo, useEffect, useRef, useState } from "react";
import { Brain, ChevronRight, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { ContentBlock } from "@/stores/aiStore";

interface ThinkingBlockProps {
  block: ContentBlock;
}

export const ThinkingBlock = memo(function ThinkingBlock({ block }: ThinkingBlockProps) {
  const { t } = useTranslation();
  const isRunning = block.status === "running";
  const [expanded, setExpanded] = useState(isRunning);
  const contentRef = useRef<HTMLDivElement>(null);
  // 思考面板内部的 sticky-bottom 跟随：思考中保持追到最新，用户主动滚开后暂停，回到底部恢复。
  // 与外层消息列表是独立的两个滚动区，所以各自维护一个状态。
  const isStickyBottomRef = useRef(true);

  // Auto-collapse when thinking completes
  useEffect(() => {
    if (!isRunning) {
      setExpanded(false);
    }
  }, [isRunning]);

  // 每次（重新）展开思考中面板时重置为 sticky；用户要主动滚开才暂停跟随。
  useEffect(() => {
    if (expanded && isRunning) {
      isStickyBottomRef.current = true;
    }
  }, [expanded, isRunning]);

  // 监听内部滚动维护 sticky 状态。展开期间挂监听，折叠时清掉。
  useEffect(() => {
    if (!expanded) return;
    const el = contentRef.current;
    if (!el) return;
    const handleScroll = () => {
      isStickyBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 32;
    };
    el.addEventListener("scroll", handleScroll, { passive: true });
    return () => el.removeEventListener("scroll", handleScroll);
  }, [expanded]);

  // 思考中且 sticky 时跟随到底部；非 running 状态下不自动滚（用户手动展开希望从头读）。
  useEffect(() => {
    if (!expanded || !isRunning || !isStickyBottomRef.current) return;
    const el = contentRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [expanded, isRunning, block.content]);

  const charCount = block.content.length;
  const summary = isRunning
    ? t("ai.thinking", "思考中...")
    : `${t("ai.thinkingProcess", "思考过程")} · ${charCount} ${t("ai.chars", "字")}`;

  return (
    <div className="my-1.5 rounded-lg border border-purple-500/20 bg-purple-500/5 text-xs overflow-hidden">
      <button
        className="flex items-center gap-2 w-full min-w-0 px-3 py-2 h-[34px] text-left hover:bg-purple-500/10 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <ChevronRight
          className={`h-3 w-3 shrink-0 text-purple-500/60 transition-transform duration-150 ${
            expanded ? "rotate-90" : ""
          }`}
        />
        {isRunning ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 text-purple-500 animate-spin" />
        ) : (
          <Brain className="h-3.5 w-3.5 shrink-0 text-purple-500" />
        )}
        <span className="text-muted-foreground italic truncate">{summary}</span>
      </button>

      {expanded && block.content && (
        <div ref={contentRef} className="border-t border-purple-500/15 px-3 py-2 max-h-64 overflow-auto">
          <pre className="whitespace-pre-wrap break-words font-mono text-[11px] text-muted-foreground/80 leading-relaxed italic">
            {block.content}
          </pre>
        </div>
      )}
    </div>
  );
});
