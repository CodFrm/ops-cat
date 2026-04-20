import { useTranslation } from "react-i18next";
import { useAIStore } from "@/stores/aiStore";

interface SideAssistantContextBarProps {
  conversationId: number | null;
}

export function SideAssistantContextBar({ conversationId }: SideAssistantContextBarProps) {
  const { t } = useTranslation();
  const conversations = useAIStore((s) => s.conversations);
  const conv = conversationId != null ? conversations.find((c) => c.ID === conversationId) : null;

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
    </div>
  );
}
