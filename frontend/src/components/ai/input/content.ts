import { buildGroupPathMap } from "@/lib/groupPath";
import { buildMentionXml, escapeXmlText, parseMentionContent } from "@/lib/mentionXml";
import { useAssetStore } from "@/stores/assetStore";
import type {
  AIChatInputDraft,
  ProseMirrorLikeNode,
  TipTapDocNode,
  TipTapMentionNode,
  TipTapParagraphNode,
  TipTapTextNode,
} from "./types";

export function extractContentXml(doc: ProseMirrorLikeNode): string {
  const assetStore = useAssetStore.getState();
  const groupPathMap = buildGroupPathMap(assetStore.groups);
  const lookupAsset = (id: number) => assetStore.assets.find((asset) => asset.ID === id);
  const hostFromConfig = (cfg: string | undefined) => {
    if (!cfg) return undefined;
    try {
      const parsed = JSON.parse(cfg) as { host?: string };
      return parsed.host || undefined;
    } catch {
      return undefined;
    }
  };

  let out = "";
  doc.descendants((node) => {
    if (node.type.name === "text") {
      out += escapeXmlText(node.text ?? "");
    } else if (node.type.name === "mention") {
      const id = Number(node.attrs.id);
      const label = String(node.attrs.label ?? "");
      const asset = Number.isFinite(id) ? lookupAsset(id) : undefined;
      out += buildMentionXml({
        assetId: id,
        name: label,
        type: asset?.Type,
        host: asset ? hostFromConfig(asset.Config) : undefined,
        groupPath: asset?.GroupID ? groupPathMap.get(asset.GroupID) : undefined,
      });
    } else if (node.type.name === "paragraph" && out.length > 0) {
      out += "\n";
    }
    return true;
  });
  return out.replace(/\n+$/g, "");
}

function normalizeDraftMessage(draft: string | AIChatInputDraft): AIChatInputDraft {
  if (typeof draft === "string") {
    return { content: draft };
  }
  return { content: draft.content ?? "" };
}

function appendTextToParagraphs(
  paragraphs: TipTapParagraphNode[],
  text: string,
  currentParagraphContent: Array<TipTapTextNode | TipTapMentionNode>
) {
  const segments = text.split("\n");
  for (let index = 0; index < segments.length; index += 1) {
    const segment = segments[index];
    if (segment.length > 0) {
      currentParagraphContent.push({ type: "text", text: segment });
    }
    if (index < segments.length - 1) {
      paragraphs.push(
        currentParagraphContent.length > 0
          ? { type: "paragraph", content: currentParagraphContent }
          : { type: "paragraph" }
      );
      currentParagraphContent = [];
    }
  }
  return currentParagraphContent;
}

export function buildEditorDocFromMessage(message: string | AIChatInputDraft): TipTapDocNode {
  const { content } = normalizeDraftMessage(message);
  const segments = parseMentionContent(content);
  const paragraphs: TipTapParagraphNode[] = [];
  let currentParagraphContent: Array<TipTapTextNode | TipTapMentionNode> = [];

  for (const seg of segments) {
    if (seg.type === "text") {
      currentParagraphContent = appendTextToParagraphs(paragraphs, seg.text, currentParagraphContent);
    } else {
      currentParagraphContent.push({
        type: "mention",
        attrs: {
          id: String(seg.attrs.assetId),
          label: seg.attrs.name,
        },
      });
    }
  }

  paragraphs.push(
    currentParagraphContent.length > 0 ? { type: "paragraph", content: currentParagraphContent } : { type: "paragraph" }
  );

  return {
    type: "doc",
    content: paragraphs,
  };
}
