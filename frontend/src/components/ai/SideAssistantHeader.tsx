import { Bot, History, Plus, PanelRightOpen, ArrowUpRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@opskat/ui";

interface SideAssistantHeaderProps {
  onToggleCollapse: () => void;
  onOpenHistory: () => void;
  onNewChat: () => void;
  onPromoteToTab: () => void;
  canPromote: boolean;
}

export function SideAssistantHeader({
  onToggleCollapse,
  onOpenHistory,
  onNewChat,
  onPromoteToTab,
  canPromote,
}: SideAssistantHeaderProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between px-3 py-2 border-b border-panel-divider">
      <div className="flex items-center gap-1.5">
        <Bot className="h-3.5 w-3.5 text-primary" />
        <span className="text-sm font-medium">{t("ai.sidebar.title")}</span>
      </div>
      <div className="flex gap-0.5">
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onOpenHistory}
          title={t("ai.sidebar.history")}
          data-history-trigger=""
        >
          <History className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onPromoteToTab}
          disabled={!canPromote}
          title={t("ai.sidebar.promoteToTab")}
        >
          <ArrowUpRight className="h-3.5 w-3.5" />
        </Button>
        <Button variant="ghost" size="icon" className="h-6 w-6" onClick={onNewChat} title={t("ai.sidebar.newChat")}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onToggleCollapse}
          title={t("ai.sidebar.collapse")}
        >
          <PanelRightOpen className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}
