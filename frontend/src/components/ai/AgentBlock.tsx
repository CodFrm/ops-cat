import { memo, useState } from "react";
import { Bot, ChevronDown, ChevronRight, Loader2 } from "lucide-react";
import { ToolBlock } from "./ToolBlock";
import type { ContentBlock } from "@/stores/aiStore";

interface AgentBlockProps {
  block: ContentBlock;
}

export const AgentBlock = memo(function AgentBlock({ block }: AgentBlockProps) {
  const [expanded, setExpanded] = useState(block.status === "running");

  const isRunning = block.status === "running";

  return (
    <div className="rounded-lg border bg-muted/30 my-1">
      <button
        className="flex items-center gap-2 w-full px-3 py-2 h-[34px] text-left text-sm hover:bg-muted/50 rounded-t-lg"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? (
          <ChevronDown className="h-3.5 w-3.5 shrink-0" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 shrink-0" />
        )}
        <Bot className="h-3.5 w-3.5 shrink-0 text-primary" />
        <span className="font-medium truncate">{block.agentRole || "Sub Agent"}</span>
        {isRunning && <Loader2 className="h-3.5 w-3.5 animate-spin ml-auto shrink-0" />}
      </button>

      {expanded && (
        <div className="px-3 pb-2 space-y-1">
          {block.agentTask && <p className="text-xs text-muted-foreground">{block.agentTask}</p>}

          {block.childBlocks?.map((child, idx) => (
            <div key={idx} className="ml-2 border-l-2 border-primary/20 pl-2">
              <ToolBlock block={child} />
            </div>
          ))}

          {block.status === "completed" && block.content && (
            <div className="text-xs bg-background rounded p-2 mt-1 whitespace-pre-wrap">{block.content}</div>
          )}
        </div>
      )}
    </div>
  );
});
