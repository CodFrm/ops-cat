import { useEffect, useRef, useState } from "react";
import { Button, Input } from "@opskat/ui";
import { Check, Pencil, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useAIStore } from "@/stores/aiStore";

interface SideAssistantContextBarProps {
  conversationId: number | null;
}

export function SideAssistantContextBar({ conversationId }: SideAssistantContextBarProps) {
  const { t } = useTranslation();
  const conversations = useAIStore((s) => s.conversations);
  const renameConversation = useAIStore((s) => s.renameConversation);
  const conv = conversationId != null ? conversations.find((c) => c.ID === conversationId) : null;
  const [editing, setEditing] = useState(false);
  const [draftTitle, setDraftTitle] = useState("");
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const editSessionRef = useRef(0);

  useEffect(() => {
    editSessionRef.current += 1;
    setEditing(false);
    setSaving(false);
    savingRef.current = false;
    setDraftTitle(conv?.Title || "");
  }, [conversationId]);

  useEffect(() => {
    // 仅在非编辑态同步外部标题，避免乐观更新时把进行中的草稿提前冲掉。
    if (!editing) {
      setDraftTitle(conv?.Title || "");
    }
  }, [conv?.Title, editing]);

  const startRename = () => {
    if (!conv || savingRef.current) return;
    editSessionRef.current += 1;
    setDraftTitle(conv?.Title || "");
    setEditing(true);
  };

  const cancelRename = () => {
    if (savingRef.current) return;
    editSessionRef.current += 1;
    setDraftTitle(conv?.Title || "");
    setEditing(false);
  };

  const submitRename = async () => {
    if (conversationId == null || !conv || savingRef.current) return;
    const editSession = editSessionRef.current;
    savingRef.current = true;
    setSaving(true);
    const renamed = await renameConversation(conversationId, draftTitle);
    if (editSessionRef.current !== editSession) {
      return;
    }
    savingRef.current = false;
    setSaving(false);
    if (renamed) {
      editSessionRef.current += 1;
      setEditing(false);
    }
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
          onChange={(event) => setDraftTitle(event.target.value)}
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
        {conv?.Title || t("ai.newConversation")}
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
