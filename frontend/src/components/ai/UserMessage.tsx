import { memo } from "react";
import type { ChatMessage, MentionRef } from "@/stores/aiStore";
import { useCompact } from "@/components/ai/AIChatContent";
import { openAssetInfoTab } from "@/lib/openAssetInfoTab";

interface Segment {
  type: "text" | "mention";
  text: string;
  mention?: MentionRef;
}

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

export const UserMessage = memo(function UserMessage({ msg }: { msg: ChatMessage }) {
  const compact = useCompact();
  const maxWidthClass = compact ? "max-w-[95%]" : "max-w-[85%]";
  const segments = buildSegments(msg.content, msg.mentions);
  return (
    <div className="flex flex-col items-end gap-1.5">
      <span className="text-xs font-semibold text-muted-foreground tracking-wide">You</span>
      <div
        className={`inline-block rounded-xl rounded-br-sm bg-primary px-3.5 py-2.5 text-primary-foreground ${maxWidthClass} text-left shadow-sm break-words whitespace-pre-wrap`}
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
    </div>
  );
});
