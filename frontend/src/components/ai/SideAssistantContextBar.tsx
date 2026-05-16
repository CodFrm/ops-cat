import { useState } from "react";
import { Button, Input } from "@opskat/ui";
import { Check, Pencil, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useAIStore } from "@/stores/aiStore";

interface SideAssistantContextBarProps {
  conversationId: number | null;
}

interface RenameState {
  conversationId: number | null;
  editing: boolean;
  draftTitle: string;
  saving: boolean;
  session: number;
}

export function SideAssistantContextBar({ conversationId }: SideAssistantContextBarProps) {
  const { t } = useTranslation();
  const conversations = useAIStore((s) => s.conversations);
  const renameConversation = useAIStore((s) => s.renameConversation);
  const conv = conversationId != null ? conversations.find((c) => c.ID === conversationId) : null;
  const conversationTitle = conv?.Title || "";
  const initialRenameState: RenameState = {
    conversationId,
    editing: false,
    draftTitle: conversationTitle,
    saving: false,
    session: 0,
  };
  const [renameState, setRenameState] = useState(initialRenameState);
  const currentRenameState = renameState.conversationId === conversationId ? renameState : initialRenameState;
  const { editing, draftTitle, saving } = currentRenameState;

  const updateRenameState = (patch: Partial<Omit<RenameState, "conversationId">>) => {
    setRenameState((current) => ({
      ...(current.conversationId === conversationId ? current : initialRenameState),
      ...patch,
      conversationId,
    }));
  };

  const startRename = () => {
    if (!conv || saving) return;
    updateRenameState({
      draftTitle: conversationTitle,
      editing: true,
      session: currentRenameState.session + 1,
    });
  };

  const cancelRename = () => {
    if (saving) return;
    updateRenameState({
      draftTitle: conversationTitle,
      editing: false,
      session: currentRenameState.session + 1,
    });
  };

  const submitRename = async () => {
    if (conversationId == null || !conv || saving) return;
    const editSession = currentRenameState.session;
    updateRenameState({ saving: true });
    const renamed = await renameConversation(conversationId, draftTitle);
    setRenameState((current) => {
      if (current.conversationId !== conversationId || current.session !== editSession) return current;
      return {
        ...current,
        saving: false,
        editing: renamed ? false : current.editing,
        session: renamed ? current.session + 1 : current.session,
      };
    });
  };

  if (!conversationId) {
    return (
      <div className="px-3 py-2 text-xs text-muted-foreground border-b border-panel-divider">
        {t("ai.sidebar.noConversation")}
      </div>
    );
  }

  if (editing) {
    return (
      <div className="flex items-center gap-1.5 px-3 py-1.5 text-xs border-b border-panel-divider">
        <Input
          value={draftTitle}
          onChange={(event) => updateRenameState({ draftTitle: event.target.value })}
          onKeyDown={(event) => {
            if ((event.nativeEvent as KeyboardEvent).isComposing) {
              return;
            }
            if (event.key === "Enter") {
              event.preventDefault();
              void submitRename();
            }
            if (event.key === "Escape") {
              event.preventDefault();
              cancelRename();
            }
          }}
          className="h-7 text-xs"
          autoFocus
          placeholder={t("ai.renameConversationPlaceholder")}
        />
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0"
          onClick={() => void submitRename()}
          title={t("action.save")}
          aria-label={t("action.save")}
          disabled={saving}
        >
          <Check className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0"
          onClick={cancelRename}
          title={t("action.cancel")}
          aria-label={t("action.cancel")}
          disabled={saving}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 text-xs text-muted-foreground border-b border-panel-divider">
      <span className="truncate flex-1 text-foreground" onDoubleClick={startRename}>
        {conversationTitle || t("ai.newConversation")}
      </span>
      <Button
        variant="ghost"
        size="icon"
        className="h-6 w-6 shrink-0"
        onClick={startRename}
        title={t("ai.renameConversation")}
        aria-label={t("ai.renameConversation")}
        disabled={!conv}
      >
        <Pencil className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
