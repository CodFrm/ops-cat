import { useState } from "react";
import {
  Terminal,
  FileText,
  FilePen,
  Search,
  ChevronRight,
  Loader2,
  CheckCircle2,
} from "lucide-react";
import type { ContentBlock } from "@/stores/aiStore";

const toolIcons: Record<string, typeof Terminal> = {
  Bash: Terminal,
  Read: FileText,
  Write: FilePen,
  Edit: FilePen,
  Glob: Search,
  Grep: Search,
};

interface ToolBlockProps {
  block: ContentBlock;
}

export function ToolBlock({ block }: ToolBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const Icon = toolIcons[block.toolName || ""] || Terminal;
  const isRunning = block.status === "running";
  const hasOutput = block.content && block.content.length > 0;

  return (
    <div className="my-1.5 rounded-lg border border-border/60 bg-muted/30 text-xs overflow-hidden">
      {/* Header: 工具名 + 输入摘要 */}
      <button
        className="flex items-center gap-1.5 w-full min-w-0 px-2.5 py-1.5 text-left hover:bg-muted/50 transition-colors"
        onClick={() => hasOutput && setExpanded(!expanded)}
        disabled={!hasOutput}
      >
        <ChevronRight
          className={`h-3 w-3 shrink-0 text-muted-foreground/60 transition-transform duration-150 ${
            expanded ? "rotate-90" : ""
          } ${!hasOutput ? "invisible" : ""}`}
        />
        {isRunning ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 text-blue-500 animate-spin" />
        ) : (
          <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="font-medium text-foreground/80">
          {block.toolName}
        </span>
        {block.toolInput && (
          <code className="truncate min-w-0 text-muted-foreground font-mono text-[11px] ml-0.5">
            {block.toolInput}
          </code>
        )}
        <span className="ml-auto shrink-0">
          {!isRunning && hasOutput && (
            <CheckCircle2 className="h-3 w-3 text-green-500/70" />
          )}
        </span>
      </button>

      {/* Output: 可展开的结果 */}
      {expanded && hasOutput && (
        <div className="border-t border-border/40 px-2.5 py-1.5 max-h-48 overflow-auto">
          <pre className="whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground leading-relaxed">
            {block.content}
          </pre>
        </div>
      )}
    </div>
  );
}
