import { useTranslation } from "react-i18next";
import { FolderOpen, Folder } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useSFTPStore } from "@/stores/sftpStore";
import { useTerminalStore } from "@/stores/terminalStore";

interface TerminalToolbarProps {
  tabId: string;
}

export function TerminalToolbar({ tabId }: TerminalToolbarProps) {
  const { t } = useTranslation();
  const tabData = useTerminalStore((s) => s.tabData[tabId]);
  const toggleFileManager = useSFTPStore((s) => s.toggleFileManager);
  const isOpen = useSFTPStore((s) => s.fileManagerOpenTabs[tabId]);

  if (!tabData) return null;
  if (Object.keys(tabData.panes).length === 0) return null;

  const Icon = isOpen ? FolderOpen : Folder;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-t bg-background shrink-0">
      <div className="flex-1" />
      <Button
        variant={isOpen ? "secondary" : "ghost"}
        size="icon-xs"
        title={t("sftp.fileManager")}
        onClick={() => toggleFileManager(tabId)}
      >
        <Icon className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
