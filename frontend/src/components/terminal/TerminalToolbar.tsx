import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { FolderOpen, Folder, FileCode } from "lucide-react";
import { Button } from "@opskat/ui";
import { useSFTPStore } from "@/stores/sftpStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { SnippetPopover } from "@/components/snippet/SnippetPopover";
import { WriteSSH, WriteSerial } from "../../../wailsjs/go/app/App";
import { bytesToBase64 } from "@/lib/terminalEncode";

interface TerminalToolbarProps {
  tabId: string;
}

export function TerminalToolbar({ tabId }: TerminalToolbarProps) {
  const { t } = useTranslation();
  const tabData = useTerminalStore((s) => s.tabData[tabId]);
  const toggleFileManager = useSFTPStore((s) => s.toggleFileManager);
  const isOpen = useSFTPStore((s) => s.fileManagerOpenTabs[tabId]);

  const activePaneId = tabData?.activePaneId;
  const activePane = activePaneId ? tabData?.panes[activePaneId] : undefined;
  const activePaneConnected = activePane?.connected ?? false;
  const isSerialActivePane = activePane?.transport === "serial";

  const handleSnippetInsert = useCallback(
    (content: string, { withEnter }: { withEnter: boolean }) => {
      if (!activePaneId) return;
      const payload = withEnter ? content + "\r" : content;
      const writeFn = isSerialActivePane ? WriteSerial : WriteSSH;
      writeFn(activePaneId, bytesToBase64(new TextEncoder().encode(payload))).catch(console.error);
    },
    [activePaneId, isSerialActivePane]
  );

  if (!tabData) return null;
  if (Object.keys(tabData.panes).length === 0) return null;

  const Icon = isOpen ? FolderOpen : Folder;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-t bg-background shrink-0">
      <div className="flex-1" />
      <SnippetPopover
        category="shell"
        showSendWithEnter
        onInsert={handleSnippetInsert}
        trigger={
          <Button
            variant="ghost"
            size="icon-xs"
            title={t("snippet.popover.triggerButton")}
            aria-label={t("snippet.popover.triggerButton")}
            disabled={!activePaneConnected}
          >
            <FileCode className="h-3.5 w-3.5" />
          </Button>
        }
      />
      {!isSerialActivePane && (
        <Button
          variant={isOpen ? "secondary" : "ghost"}
          size="icon-xs"
          title={t("sftp.fileManager")}
          onClick={() => toggleFileManager(tabId)}
        >
          <Icon className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}
