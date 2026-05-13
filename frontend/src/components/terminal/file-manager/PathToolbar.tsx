import { useTranslation } from "react-i18next";
import { ArrowUp, FilePlus, FolderPlus, Home, RefreshCw } from "lucide-react";
import { Button, Input } from "@opskat/ui";
import { type TerminalDirectoryFollowMode } from "@/stores/terminalStore";

interface PathToolbarProps {
  currentPath: string;
  directoryFollowMode: TerminalDirectoryFollowMode;
  onFollowToggle: () => void;
  onGoHome: () => void;
  onGoUp: () => void;
  onNewFile: () => void;
  onNewFolder: () => void;
  onPathInputChange: (path: string) => void;
  onPathSubmit: (path: string) => void;
  onRefresh: () => void;
  onSyncPanelFromTerminal: () => void;
  onSyncTerminalToPath: () => void;
  paneConnected: boolean;
  pathInput: string;
}

export function PathToolbar({
  currentPath,
  directoryFollowMode,
  onFollowToggle,
  onGoHome,
  onGoUp,
  onNewFile,
  onNewFolder,
  onPathInputChange,
  onPathSubmit,
  onRefresh,
  onSyncPanelFromTerminal,
  onSyncTerminalToPath,
  paneConnected,
  pathInput,
}: PathToolbarProps) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-0.5 px-1 py-1 border-b shrink-0">
      <Button
        variant="ghost"
        size="xs"
        className="px-1.5 text-[10px] font-mono"
        onClick={onSyncPanelFromTerminal}
        title={t("sftp.sync.panelFromTerminal")}
        aria-label={t("sftp.sync.panelFromTerminal")}
        disabled={!paneConnected}
      >
        {"T→F"}
      </Button>
      <Button
        variant="ghost"
        size="xs"
        className="px-1.5 text-[10px] font-mono"
        onClick={onSyncTerminalToPath}
        title={t("sftp.sync.terminalFromPanel")}
        aria-label={t("sftp.sync.terminalFromPanel")}
        disabled={!paneConnected}
      >
        {"F→T"}
      </Button>
      <Button
        variant={directoryFollowMode === "always" ? "secondary" : "ghost"}
        size="xs"
        className="px-1.5 text-[10px] font-medium"
        onClick={onFollowToggle}
        title={t("sftp.sync.followToggle")}
        aria-label={t("sftp.sync.followToggle")}
        disabled={!paneConnected}
      >
        {t("sftp.sync.followShort")}
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={onGoUp}
        disabled={currentPath === "/"}
        title={t("sftp.parentDir")}
      >
        <ArrowUp className="h-3.5 w-3.5" />
      </Button>
      <Button variant="ghost" size="icon-xs" onClick={onGoHome} title={t("sftp.home")}>
        <Home className="h-3.5 w-3.5" />
      </Button>
      <Button variant="ghost" size="icon-xs" onClick={onNewFile} title={t("sftp.newFile")}>
        <FilePlus className="h-3.5 w-3.5" />
      </Button>
      <Button variant="ghost" size="icon-xs" onClick={onNewFolder} title={t("sftp.newFolder")}>
        <FolderPlus className="h-3.5 w-3.5" />
      </Button>
      <Input
        className="h-6 text-xs flex-1 min-w-0"
        value={pathInput}
        onChange={(e) => onPathInputChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onPathSubmit(pathInput.trim());
        }}
      />
      <Button variant="ghost" size="icon-xs" onClick={onRefresh} title={t("sftp.refresh")}>
        <RefreshCw className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
