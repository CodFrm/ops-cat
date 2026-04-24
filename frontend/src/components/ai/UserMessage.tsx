import { memo, useCallback } from "react";
import { Copy, Pencil } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from "@opskat/ui";
import type { ChatMessage, MentionRef } from "@/stores/aiStore";
import { useCompact } from "@/components/ai/AIChatContentContext";
import { openAssetInfoTab } from "@/lib/openAssetInfoTab";

interface Segment {
  type: "text" | "mention";
  text: string;
  mention?: MentionRef;
}

// 单独控制用户消息选中态，保证主色气泡里也能明确看到选区范围。
const userMessageSelectionClass = "select-text selection:bg-white/35 selection:text-primary-foreground";

function buildSegments(content: string, mentions: MentionRef[] | undefined): Segment[] {
  if (!mentions || mentions.length === 0) {
    return [{ type: "text", text: content }];
  }
  const sorted = [...mentions].sort((a, b) => a.start - b.start);
  const segs: Segment[] = [];
  let cursor = 0;
  for (const m of sorted) {
    if (m.start > cursor) segs.push({ type: "text", text: content.slice(cursor, m.start) });
    segs.push({ type: "mention", text: content.slice(m.start, m.end), mention: m });
    cursor = m.end;
  }
  if (cursor < content.length) segs.push({ type: "text", text: content.slice(cursor) });
  return segs;
}

// 统一复制提示，保证按钮复制和右键复制的反馈一致；剪贴板不可用时给出错误提示，避免 unhandled rejection。
async function copyUserMessageText(text: string, copiedText: string, failedText: string) {
  if (!text.trim()) return;
  try {
    await navigator.clipboard.writeText(text);
    toast.success(copiedText, { duration: 1500, position: "top-center" });
  } catch {
    toast.error(failedText, { duration: 2000, position: "top-center" });
  }
}

interface UserMessageProps {
  msg: ChatMessage;
  onEdit?: () => void;
}

export const UserMessage = memo(function UserMessage({ msg, onEdit }: UserMessageProps) {
  const compact = useCompact();
  const maxWidthClass = compact ? "max-w-[95%]" : "max-w-[85%]";
  const segments = buildSegments(msg.content, msg.mentions);
  const { t } = useTranslation();

  // 用户消息复制默认取整条内容，减少额外转换带来的偏差。
  const handleCopy = useCallback(() => {
    void copyUserMessageText(msg.content, t("ai.copied", "已复制到剪贴板"), t("ai.copyFailed", "复制失败"));
  }, [msg.content, t]);

  // 右键复制优先保留当前选区，没有选区时回退到整条消息。
  const handleContextCopy = useCallback(() => {
    const selectedText = window.getSelection?.()?.toString().trim() ?? "";
    const copyText = selectedText && msg.content.includes(selectedText) ? selectedText : msg.content;
    void copyUserMessageText(copyText, t("ai.copied", "已复制到剪贴板"), t("ai.copyFailed", "复制失败"));
  }, [msg.content, t]);

  const handleEdit = useCallback(() => {
    onEdit?.();
  }, [onEdit]);

  return (
    <div className="flex flex-col items-end gap-1.5 group/user">
      <span className="text-xs font-semibold text-muted-foreground tracking-wide">You</span>
      <ContextMenu>
        <ContextMenuTrigger className={`block ${maxWidthClass}`}>
          <div
            className={`rounded-xl rounded-br-sm bg-primary px-3.5 py-2.5 text-primary-foreground text-left shadow-sm break-words whitespace-pre-wrap ${userMessageSelectionClass}`}
          >
            {segments.map((s, i) =>
              s.type === "text" ? (
                <span key={i}>{s.text}</span>
              ) : (
                <button
                  key={i}
                  type="button"
                  onClick={() => openAssetInfoTab(s.mention!.assetId)}
                  className="inline-flex items-center rounded bg-primary-foreground/20 px-1 py-0.5 text-xs font-medium hover:bg-primary-foreground/30 hover:underline cursor-pointer"
                >
                  {s.text}
                </button>
              )
            )}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          {onEdit && <ContextMenuItem onClick={handleEdit}>{t("ai.editMessage", "编辑消息")}</ContextMenuItem>}
          <ContextMenuItem onClick={handleContextCopy}>{t("action.copy", "复制")}</ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
      <div className="flex items-center gap-2 min-h-[18px] pr-0.5">
        {onEdit && (
          <button
            type="button"
            className="opacity-0 group-hover/user:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
            onClick={handleEdit}
            title={t("ai.editMessage", "编辑消息")}
            aria-label={t("ai.editMessage", "编辑消息")}
          >
            <Pencil className="h-3.5 w-3.5" />
          </button>
        )}
        <button
          type="button"
          className="opacity-0 group-hover/user:opacity-100 transition-opacity text-muted-foreground/50 hover:text-primary"
          onClick={handleCopy}
          title={t("action.copy", "复制")}
          aria-label={t("action.copy", "复制")}
        >
          <Copy className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
});
