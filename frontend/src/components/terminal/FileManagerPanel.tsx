import {
  useState,
  useEffect,
  useCallback,
  useRef,
  useMemo,
  useLayoutEffect,
} from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import {
  Folder,
  File,
  Loader2,
  ArrowUp,
  Home,
  RefreshCw,
  Upload,
  Download,
  FolderDown,
  X,
  CheckCircle2,
  XCircle,
  FolderOpen,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { SFTPListDir, SFTPGetwd } from "../../../wailsjs/go/main/App";
import { OnFileDrop, OnFileDropOff } from "../../../wailsjs/runtime/runtime";
import { useSFTPStore } from "@/stores/sftpStore";
import { sftp_svc } from "../../../wailsjs/go/models";

// --- Helpers ---

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + " " + units[i];
}

function formatDate(timestamp: number): string {
  const d = new Date(timestamp * 1000);
  return d.toLocaleDateString(undefined, {
    month: "2-digit",
    day: "2-digit",
  });
}

// --- Context Menu ---

interface CtxMenuState {
  x: number;
  y: number;
  entry: sftp_svc.FileEntry | null;
}

function FloatingMenu({
  ctx,
  onAction,
  onClose,
}: {
  ctx: CtxMenuState;
  onAction: (action: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const [visible, setVisible] = useState(false);
  const [interactive, setInteractive] = useState(false);

  useLayoutEffect(() => {
    if (!ref.current) return;
    const rect = ref.current.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let left = ctx.x + 2;
    let top = ctx.y + 2;
    if (left + rect.width > vw) left = ctx.x - rect.width - 2;
    if (top + rect.height > vh) top = ctx.y - rect.height - 2;
    left = Math.max(4, Math.min(left, vw - rect.width - 4));
    top = Math.max(4, Math.min(top, vh - rect.height - 4));
    setPos({ top, left });
    setVisible(true);
  }, [ctx.x, ctx.y]);

  useEffect(() => {
    const timer = setTimeout(() => setInteractive(true), 150);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    const handlePointer = (e: PointerEvent) => {
      if (ref.current?.contains(e.target as Node)) return;
      onClose();
    };
    const timer = setTimeout(() => {
      document.addEventListener("pointerdown", handlePointer, true);
    }, 50);
    document.addEventListener("keydown", handleKey);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("pointerdown", handlePointer, true);
      document.removeEventListener("keydown", handleKey);
    };
  }, [onClose]);

  const item = (action: string, icon: React.ReactNode, label: string) => (
    <div
      className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm cursor-default hover:bg-accent hover:text-accent-foreground select-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground"
      onClick={() => onAction(action)}
    >
      {icon}
      {label}
    </div>
  );

  return createPortal(
    <div
      ref={ref}
      className={cn(
        "z-50 min-w-[10rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md",
        visible && "animate-in fade-in-0 zoom-in-95"
      )}
      style={{
        position: "fixed",
        top: pos.top,
        left: pos.left,
        visibility: visible ? "visible" : "hidden",
        pointerEvents: interactive ? "auto" : "none",
      }}
    >
      {ctx.entry ? (
        ctx.entry.isDir ? (
          <>
            {item("open", <FolderOpen />, t("sftp.openFolder"))}
            {item(
              "downloadDir",
              <FolderDown />,
              t("sftp.downloadDir")
            )}
          </>
        ) : (
          item("download", <Download />, t("sftp.download"))
        )
      ) : (
        <>
          {item("upload", <Upload />, t("sftp.upload"))}
          {item("uploadDir", <Upload />, t("sftp.uploadDir"))}
          <div className="-mx-1 my-1 h-px bg-border" />
          {item("refresh", <RefreshCw />, t("sftp.refresh"))}
        </>
      )}
    </div>,
    document.body
  );
}

// --- Transfer Row ---

function TransferRow({
  transfer,
}: {
  transfer: {
    transferId: string;
    direction: "upload" | "download";
    currentFile: string;
    bytesDone: number;
    bytesTotal: number;
    speed: number;
    status: "active" | "done" | "error" | "cancelled";
    error?: string;
  };
}) {
  const cancelTransfer = useSFTPStore((s) => s.cancelTransfer);
  const clearTransfer = useSFTPStore((s) => s.clearTransfer);

  const percent =
    transfer.bytesTotal > 0
      ? Math.round((transfer.bytesDone / transfer.bytesTotal) * 100)
      : 0;

  const fileName = transfer.currentFile
    ? transfer.currentFile.split("/").pop() || transfer.currentFile
    : "";

  return (
    <div className="flex items-center gap-1.5 text-[11px]">
      {/* Icon */}
      <div className="shrink-0">
        {transfer.status === "active" && (
          <Loader2 className="h-3 w-3 animate-spin text-primary" />
        )}
        {transfer.status === "done" && (
          <CheckCircle2 className="h-3 w-3 text-green-500" />
        )}
        {(transfer.status === "error" || transfer.status === "cancelled") && (
          <XCircle className="h-3 w-3 text-destructive" />
        )}
      </div>

      {/* Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1">
          {transfer.direction === "upload" ? (
            <Upload className="h-2.5 w-2.5 text-muted-foreground shrink-0" />
          ) : (
            <Download className="h-2.5 w-2.5 text-muted-foreground shrink-0" />
          )}
          <span className="truncate">{fileName}</span>
          {transfer.status === "active" && (
            <span className="shrink-0 text-muted-foreground ml-auto">
              {percent}%
            </span>
          )}
        </div>
        {transfer.status === "active" && (
          <div className="h-1 rounded-full bg-muted overflow-hidden mt-0.5">
            <div
              className="h-full rounded-full bg-primary transition-all duration-300"
              style={{ width: `${percent}%` }}
            />
          </div>
        )}
        {transfer.status === "error" && transfer.error && (
          <span
            className="text-destructive truncate block text-[10px]"
            title={transfer.error}
          >
            {transfer.error}
          </span>
        )}
      </div>

      {/* Action */}
      <Button
        variant="ghost"
        size="icon-xs"
        className="shrink-0 h-4 w-4"
        onClick={() =>
          transfer.status === "active"
            ? cancelTransfer(transfer.transferId)
            : clearTransfer(transfer.transferId)
        }
      >
        <X className="h-2.5 w-2.5" />
      </Button>
    </div>
  );
}

// --- Main Panel ---

interface FileManagerPanelProps {
  tabId: string;
  sessionId: string;
  width: number;
  onWidthChange: (width: number) => void;
}

export function FileManagerPanel({
  sessionId,
  width,
  onWidthChange,
}: FileManagerPanelProps) {
  const { t } = useTranslation();

  // File browsing state
  const [currentPath, setCurrentPath] = useState("/");
  const [pathInput, setPathInput] = useState("/");
  const [entries, setEntries] = useState<sftp_svc.FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  // Context menu
  const [ctxMenu, setCtxMenu] = useState<CtxMenuState | null>(null);

  // Resize
  const [isResizing, setIsResizing] = useState(false);

  // Refs
  const panelRef = useRef<HTMLDivElement>(null);
  const lastPathRef = useRef<Record<string, string>>({});
  const currentPathRef = useRef(currentPath);
  currentPathRef.current = currentPath;

  // Store
  const startUpload = useSFTPStore((s) => s.startUpload);
  const startUploadDir = useSFTPStore((s) => s.startUploadDir);
  const startUploadFile = useSFTPStore((s) => s.startUploadFile);
  const startDownload = useSFTPStore((s) => s.startDownload);
  const startDownloadDir = useSFTPStore((s) => s.startDownloadDir);
  const allTransfers = useSFTPStore((s) => s.transfers);
  const clearCompleted = useSFTPStore((s) => s.clearCompleted);

  const sessionTransfers = useMemo(
    () => Object.values(allTransfers).filter((t) => t.sessionId === sessionId),
    [allTransfers, sessionId]
  );

  // === File browsing ===
  const loadDir = useCallback(
    async (dirPath: string) => {
      setLoading(true);
      setError(null);
      setSelected(null);
      try {
        const result = await SFTPListDir(sessionId, dirPath);
        setEntries(result || []);
        setCurrentPath(dirPath);
        setPathInput(dirPath);
        lastPathRef.current[sessionId] = dirPath;
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    },
    [sessionId]
  );

  // Load initial directory on session change
  useEffect(() => {
    if (!sessionId) return;
    const lastPath = lastPathRef.current[sessionId];
    if (lastPath) {
      loadDir(lastPath);
    } else {
      SFTPGetwd(sessionId)
        .then((home) => loadDir(home || "/"))
        .catch(() => loadDir("/"));
    }
  }, [sessionId, loadDir]);

  // Auto-refresh after upload completes
  const doneUploadCount = sessionTransfers.filter(
    (t) => t.status === "done" && t.direction === "upload"
  ).length;
  const prevDoneCount = useRef(0);
  useEffect(() => {
    if (doneUploadCount > prevDoneCount.current) {
      loadDir(currentPath);
    }
    prevDoneCount.current = doneUploadCount;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doneUploadCount]);

  // === Navigation helpers ===
  const getFullPath = useCallback(
    (entry: sftp_svc.FileEntry) =>
      currentPath === "/"
        ? "/" + entry.name
        : currentPath + "/" + entry.name,
    [currentPath]
  );

  const goUp = () => {
    if (currentPath === "/") return;
    const parent = currentPath.replace(/\/[^/]+\/?$/, "") || "/";
    loadDir(parent);
  };

  const goHome = () => {
    SFTPGetwd(sessionId)
      .then((home) => loadDir(home || "/"))
      .catch(() => loadDir("/"));
  };

  // === Drag and drop (Wails native) ===
  useEffect(() => {
    const handler = (_x: number, _y: number, paths: string[]) => {
      for (const path of paths) {
        startUploadFile(sessionId, path, currentPathRef.current + "/");
      }
    };
    OnFileDrop(handler, true);
    return () => {
      OnFileDropOff();
    };
  }, [sessionId, startUploadFile]);

  // === Resize handle ===
  const handleResizeStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      const startX = e.clientX;
      const startWidth = width;

      const onMove = (e: MouseEvent) => {
        const delta = startX - e.clientX;
        onWidthChange(startWidth + delta);
      };
      const onUp = () => {
        setIsResizing(false);
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
      };
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
    },
    [width, onWidthChange]
  );

  // === Context menu actions ===
  const handleCtxAction = useCallback(
    (action: string) => {
      if (!ctxMenu) return;
      const entry = ctxMenu.entry;
      setCtxMenu(null);

      switch (action) {
        case "open":
          if (entry?.isDir) loadDir(getFullPath(entry));
          break;
        case "download":
          if (entry) startDownload(sessionId, getFullPath(entry));
          break;
        case "downloadDir":
          if (entry) startDownloadDir(sessionId, getFullPath(entry));
          break;
        case "upload":
          startUpload(sessionId, currentPath.endsWith("/") ? currentPath : currentPath + "/");
          break;
        case "uploadDir":
          startUploadDir(sessionId, currentPath.endsWith("/") ? currentPath : currentPath + "/");
          break;
        case "refresh":
          loadDir(currentPath);
          break;
      }
    },
    [
      ctxMenu,
      sessionId,
      currentPath,
      loadDir,
      getFullPath,
      startUpload,
      startUploadDir,
      startDownload,
      startDownloadDir,
    ]
  );

  // Sort entries: directories first, then alphabetical
  const sortedEntries = useMemo(
    () =>
      [...entries].sort((a, b) => {
        if (a.isDir && !b.isDir) return -1;
        if (!a.isDir && b.isDir) return 1;
        return a.name.localeCompare(b.name);
      }),
    [entries]
  );

  return (
    <div className="flex shrink-0">
      {/* Resize handle */}
      <div
        className={cn(
          "w-1 cursor-col-resize hover:bg-primary/20 transition-colors shrink-0",
          isResizing && "bg-primary/30"
        )}
        onMouseDown={handleResizeStart}
      />

      {/* Panel */}
      <div
        ref={panelRef}
        className="group/drop flex flex-col border-l bg-background relative overflow-hidden"
        style={
          {
            width: width,
            "--wails-drop-target": "drop",
          } as React.CSSProperties
        }
      >
        {/* Drop overlay */}
        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center opacity-0 group-[.wails-drop-target-active]/drop:opacity-100 bg-primary/5 border-2 border-dashed border-primary/30 rounded transition-opacity">
          <div className="flex flex-col items-center gap-1 text-primary/60">
            <Upload className="h-5 w-5" />
            <span className="text-xs">{t("sftp.dropToUpload")}</span>
          </div>
        </div>

        {/* Path bar */}
        <div className="flex items-center gap-0.5 px-1 py-1 border-b shrink-0">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={goUp}
            disabled={currentPath === "/"}
            title={t("sftp.parentDir")}
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={goHome}
            title={t("sftp.home")}
          >
            <Home className="h-3.5 w-3.5" />
          </Button>
          <Input
            className="h-6 text-xs flex-1 min-w-0"
            value={pathInput}
            onChange={(e) => setPathInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") loadDir(pathInput.trim());
            }}
          />
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => loadDir(currentPath)}
            title={t("sftp.refresh")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>

        {/* File list */}
        <ScrollArea className="flex-1 min-h-0">
          <div
            className="text-xs select-none"
            onContextMenu={(e) => {
              e.preventDefault();
              setCtxMenu({ x: e.clientX, y: e.clientY, entry: null });
            }}
          >
            {loading && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              </div>
            )}
            {error && !loading && (
              <div className="flex flex-col items-center justify-center py-8 gap-1 px-2">
                <span className="text-destructive text-center text-xs">
                  {t("sftp.loadError")}
                </span>
                <span className="text-muted-foreground text-center break-all text-[10px]">
                  {error}
                </span>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => loadDir(currentPath)}
                  className="mt-1"
                >
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
                    onDoubleClick={goUp}
                  >
                    <Folder className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                    <span className="flex-1 truncate">..</span>
                  </div>
                )}
                {sortedEntries.map((entry) => {
                  const fullPath = getFullPath(entry);
                  const isSelected = selected === fullPath;
                  return (
                    <div
                      key={entry.name}
                      className={cn(
                        "flex items-center gap-1.5 px-2 py-1 cursor-pointer transition-colors",
                        isSelected
                          ? "bg-primary/10 text-primary"
                          : "hover:bg-muted/50"
                      )}
                      onClick={() => setSelected(fullPath)}
                      onDoubleClick={() => {
                        if (entry.isDir) loadDir(fullPath);
                      }}
                      onContextMenu={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        setSelected(fullPath);
                        setCtxMenu({ x: e.clientX, y: e.clientY, entry });
                      }}
                    >
                      {entry.isDir ? (
                        <Folder className="h-3.5 w-3.5 text-primary/70 shrink-0" />
                      ) : (
                        <File className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                      )}
                      <span className="flex-1 truncate">{entry.name}</span>
                      {!entry.isDir && (
                        <span className="text-muted-foreground shrink-0 text-[10px]">
                          {formatBytes(entry.size)}
                        </span>
                      )}
                      <span className="text-muted-foreground shrink-0 text-[10px]">
                        {formatDate(entry.modTime)}
                      </span>
                    </div>
                  );
                })}
              </>
            )}
          </div>
        </ScrollArea>

        {/* Transfer section */}
        {sessionTransfers.length > 0 && (
          <div className="border-t shrink-0">
            <div className="flex items-center justify-between px-2 py-0.5">
              <span className="text-[11px] font-medium text-muted-foreground">
                {t("sftp.transfers")}
              </span>
              <Button
                variant="ghost"
                size="icon-xs"
                className="h-4 w-4"
                onClick={clearCompleted}
                title={t("sftp.clear")}
              >
                <X className="h-2.5 w-2.5" />
              </Button>
            </div>
            <ScrollArea className="max-h-28">
              <div className="px-2 pb-1 space-y-0.5">
                {sessionTransfers.map((transfer) => (
                  <TransferRow
                    key={transfer.transferId}
                    transfer={transfer}
                  />
                ))}
              </div>
            </ScrollArea>
          </div>
        )}
      </div>

      {/* Context menu */}
      {ctxMenu && (
        <FloatingMenu
          ctx={ctxMenu}
          onAction={handleCtxAction}
          onClose={() => setCtxMenu(null)}
        />
      )}
    </div>
  );
}
