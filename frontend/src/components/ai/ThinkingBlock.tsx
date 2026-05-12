import { memo, useState, useEffect } from "react";
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

  // Auto-collapse when thinking completes
  useEffect(() => {
    if (!isRunning) {
      setExpanded(false);
    }
  }, [isRunning]);

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
        <div className="border-t border-purple-500/15 px-3 py-2 max-h-64 overflow-auto">
          <pre className="whitespace-pre-wrap break-words font-mono text-[11px] text-muted-foreground/80 leading-relaxed italic">
            {block.content}
          </pre>
        </div>
      )}
    </div>
  );
});
