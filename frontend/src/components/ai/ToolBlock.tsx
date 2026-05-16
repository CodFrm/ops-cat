import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Terminal,
  FileText,
  FilePen,
  Search,
  ChevronRight,
  Loader2,
  CheckCircle2,
  XCircle,
  Shield,
} from "lucide-react";
import type { ContentBlock } from "@/stores/aiStore";

const toolIcons: Record<string, typeof Terminal> = {
  Bash: Terminal,
  Read: FileText,
  Write: FilePen,
  Edit: FilePen,
  Glob: Search,
  Grep: Search,
  run_command: Terminal,
  request_permission: Shield,
};

interface ToolBlockProps {
  block: ContentBlock;
}

function formatToolInput(input?: string): string {
  if (!input) return "";
  const trimmed = input.trim();
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return input;
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return input;
  }
}

export const ToolBlock = memo(function ToolBlock({ block }: ToolBlockProps) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const Icon = toolIcons[block.toolName || ""] || Terminal;
  const isRunning = block.status === "running";
  const isError = block.status === "error";
  const isCancelled = block.status === "cancelled";
  const hasOutput = block.content && block.content.length > 0;
  const hasInput = !!block.toolInput;
  const canExpand = hasOutput || hasInput;

  return (
    <div
      className={`my-1.5 rounded-lg border bg-background text-xs overflow-hidden ${
        isRunning ? "border-primary/30" : "border-border/60"
      }`}
    >
      <button
        className="flex items-center gap-2 w-full min-w-0 px-3 py-2 h-[34px] text-left hover:bg-muted/50 transition-colors"
        onClick={() => canExpand && setExpanded(!expanded)}
        disabled={!canExpand}
      >
        {canExpand && (
          <ChevronRight
            className={`h-3 w-3 shrink-0 text-muted-foreground transition-transform duration-150 ${
              expanded ? "rotate-90 opacity-100" : "opacity-50"
            }`}
          />
        )}
        {isRunning ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 text-primary animate-spin" />
        ) : (
          <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="font-medium text-foreground/80">{block.toolName}</span>
        {block.toolInput && (
          <code className="min-w-0 truncate text-muted-foreground font-mono text-[10px] ml-0.5">{block.toolInput}</code>
        )}
        <span className="ml-auto shrink-0">
          {isError && <XCircle className="h-3.5 w-3.5 text-destructive/70" />}
          {isCancelled && <XCircle className="h-3.5 w-3.5 text-muted-foreground/50" />}
          {!isRunning && !isError && !isCancelled && hasOutput && (
            <CheckCircle2 className="h-3.5 w-3.5 text-green-500/70" />
          )}
        </span>
      </button>

      {expanded && canExpand && (
        <div className="border-t border-border/40 max-h-96 overflow-auto">
          {hasInput && (
            <div className="px-3 py-2">
              <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">{t("toolBlock.arguments")}</div>
              <pre className="whitespace-pre-wrap break-all font-mono text-[11px] text-foreground/70 leading-relaxed">
                {formatToolInput(block.toolInput)}
              </pre>
            </div>
          )}
          {hasOutput && (
            <div className={`px-3 py-2 ${hasInput ? "border-t border-border/40" : ""}`}>
              {hasInput && (
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">{t("toolBlock.output")}</div>
              )}
              <pre className="whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground leading-relaxed">
                {block.content}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
});
