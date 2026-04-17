import { Pin } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useAIStore } from "@/stores/aiStore";
import { useTabStore } from "@/stores/tabStore";

interface SideAssistantContextBarProps {
  conversationId: number | null;
}

export function SideAssistantContextBar({ conversationId }: SideAssistantContextBarProps) {
  const { t } = useTranslation();
  const conversations = useAIStore((s) => s.conversations);
  const conv = conversationId != null ? conversations.find((c) => c.ID === conversationId) : null;

  // Follow main tab: if the active non-AI, non-page tab has an asset, show its name as a badge.
  const followedAsset = useTabStore((s) => {
    const active = s.tabs.find((t) => t.id === s.activeTabId);
    if (!active || active.type === "ai" || active.type === "page") return null;
    const meta = active.meta as { assetName?: string; assetId?: number };
    return meta.assetName || null;
  });

  if (!conversationId) {
    return (
      <div className="px-3 py-2 text-xs text-muted-foreground border-b border-panel-divider">
        {t("ai.sidebar.noConversation")}
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 text-xs text-muted-foreground border-b border-panel-divider">
      <span className="truncate flex-1 text-foreground">{conv?.Title || t("ai.newConversation")}</span>
      {followedAsset && (
        <span
          className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-primary/10 text-primary"
          title={t("ai.sidebar.followMainTab")}
        >
          <Pin className="h-2.5 w-2.5" />
          {followedAsset}
        </span>
      )}
    </div>
  );
}
