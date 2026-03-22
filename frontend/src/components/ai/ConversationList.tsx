import { Trash2, MessageSquare } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useAIStore } from "@/stores/aiStore";
import { useState } from "react";

function formatRelativeTime(timestamp: number): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}天前`;
  const date = new Date(timestamp * 1000);
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

export function ConversationList() {
  const { t } = useTranslation();
  const {
    conversations,
    currentConversationId,
    switchConversation,
    deleteConversation,
    sending,
  } = useAIStore();
  const [open, setOpen] = useState(false);

  const handleSwitch = async (id: number) => {
    if (id === currentConversationId) return;
    await switchConversation(id);
    setOpen(false);
  };

  const handleDelete = async (e: React.MouseEvent, id: number) => {
    e.stopPropagation();
    await deleteConversation(id);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          title={t("ai.conversations", "对话列表")}
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-64 p-0"
        align="end"
        side="bottom"
        sideOffset={4}
      >
        <ScrollArea className="max-h-80">
          {conversations.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-6">
              {t("ai.noConversations", "暂无对话")}
            </p>
          ) : (
            <div className="py-1">
              {conversations.map((conv) => (
                <div
                  key={conv.ID}
                  className={`flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-muted/50 transition-colors ${
                    conv.ID === currentConversationId
                      ? "bg-muted/80"
                      : ""
                  }`}
                  onClick={() => !sending && handleSwitch(conv.ID)}
                >
                  <div className="flex-1 min-w-0">
                    <p className="text-sm truncate">{conv.Title}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatRelativeTime(conv.Updatetime)}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-5 w-5 shrink-0 opacity-0 group-hover:opacity-100 hover:opacity-100"
                    onClick={(e) => handleDelete(e, conv.ID)}
                    disabled={sending}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </ScrollArea>
      </PopoverContent>
    </Popover>
  );
}
