import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { File, Folder, Loader2 } from "lucide-react";
import { Button, cn, ScrollArea } from "@opskat/ui";
import { sftp_svc } from "../../../../wailsjs/go/models";
import { formatBytes, formatDate, getEntryPath, sortEntries } from "./utils";

interface FileListProps {
  currentPath: string;
  entries: sftp_svc.FileEntry[];
  error: string | null;
  loading: boolean;
  onGoUp: () => void;
  onNavigate: (path: string) => void;
  onOpenContextMenu: (x: number, y: number, entry: sftp_svc.FileEntry | null) => void;
  onRetry: () => void;
  selected: string | null;
  setSelected: (path: string | null) => void;
}

export function FileList({
  currentPath,
  entries,
  error,
  loading,
  onGoUp,
  onNavigate,
  onOpenContextMenu,
  onRetry,
  selected,
  setSelected,
}: FileListProps) {
  const { t } = useTranslation();
  const sortedEntries = useMemo(() => sortEntries(entries), [entries]);

  return (
    <ScrollArea className="flex-1 min-h-0">
      <div
        className="text-xs select-none"
        onContextMenu={(e) => {
          e.preventDefault();
          onOpenContextMenu(e.clientX, e.clientY, null);
        }}
      >
        {loading && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
        {error && !loading && (
          <div className="flex flex-col items-center justify-center py-8 gap-1 px-2">
            <span className="text-destructive text-center text-xs">{t("sftp.loadError")}</span>
            <span className="text-muted-foreground text-center break-all text-[10px]">{error}</span>
            <Button variant="outline" size="xs" onClick={onRetry} className="mt-1">
              {t("sftp.retry")}
            </Button>
          </div>
        )}
        {!loading && !error && entries.length === 0 && (
          <div className="flex items-center justify-center py-8">
            <span className="text-muted-foreground">{t("sftp.empty")}</span>
          </div>
        )}
        {!loading && !error && (
          <>
            {currentPath !== "/" && (
              <div
                className="flex items-center gap-1.5 px-2 py-1 cursor-pointer hover:bg-muted/50"
                onDoubleClick={onGoUp}
              >
                <Folder className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                <span className="flex-1 truncate">..</span>
              </div>
            )}
            {sortedEntries.map((entry) => {
              const fullPath = getEntryPath(currentPath, entry);
              const isSelected = selected === fullPath;
              return (
                <div
                  key={entry.name}
                  className={cn(
                    "flex items-center gap-1.5 px-2 py-1 cursor-pointer transition-colors",
                    isSelected ? "bg-primary/10 text-primary" : "hover:bg-muted/50"
                  )}
                  style={{ contentVisibility: "auto", containIntrinsicSize: "auto 28px" }}
                  onClick={() => setSelected(fullPath)}
                  onDoubleClick={() => {
                    if (entry.isDir) onNavigate(fullPath);
                  }}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setSelected(fullPath);
                    onOpenContextMenu(e.clientX, e.clientY, entry);
                  }}
                >
                  {entry.isDir ? (
                    <Folder className="h-3.5 w-3.5 text-primary/70 shrink-0" />
                  ) : (
                    <File className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                  )}
                  <span className="flex-1 truncate">{entry.name}</span>
                  {!entry.isDir && (
                    <span className="text-muted-foreground shrink-0 text-[10px]">{formatBytes(entry.size)}</span>
                  )}
                  <span className="text-muted-foreground shrink-0 text-[10px]">{formatDate(entry.modTime)}</span>
                </div>
              );
            })}
          </>
        )}
      </div>
    </ScrollArea>
  );
}
