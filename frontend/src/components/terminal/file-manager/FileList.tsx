import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { File, Folder, Loader2 } from "lucide-react";
import { Button, cn, Input, ScrollArea } from "@opskat/ui";
import { sftp_svc } from "../../../../wailsjs/go/models";
import { type EditingState } from "./types";
import { formatBytes, formatDate, formatMode, getEntryPath, sortEntries, validateEntryName } from "./utils";

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
  editingState: EditingState | null;
  editError: string | null;
  editSubmitting: boolean;
  onEditCommit: (name: string) => void;
  onEditCancel: () => void;
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
  editingState,
  editError,
  editSubmitting,
  onEditCommit,
  onEditCancel,
}: FileListProps) {
  const { t } = useTranslation();
  const sortedEntries = useMemo(() => sortEntries(entries), [entries]);

  return (
    <ScrollArea
      className="flex-1 min-h-0"
      onContextMenu={(e) => {
        if (e.defaultPrevented) return;
        e.preventDefault();
        onOpenContextMenu(e.clientX, e.clientY, null);
      }}
    >
      <div
        className="text-xs select-none min-h-full"
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
            {editingState && editingState.mode !== "rename" && (
              <EditRow
                kind={editingState.mode}
                initialValue=""
                error={editError}
                submitting={editSubmitting}
                onCommit={onEditCommit}
                onCancel={onEditCancel}
              />
            )}
            {entries.length === 0 && !editingState && (
              <div className="flex items-center justify-center py-8">
                <span className="text-muted-foreground">{t("sftp.empty")}</span>
              </div>
            )}
            {sortedEntries.map((entry) => {
              const fullPath = getEntryPath(currentPath, entry);
              const isSelected = selected === fullPath;
              const isRenaming = editingState?.mode === "rename" && editingState.targetName === entry.name;
              return (
                <div
                  key={entry.name}
                  className={cn(
                    "flex items-center gap-1.5 px-2 py-1 cursor-pointer transition-colors",
                    isSelected && !isRenaming ? "bg-primary/10 text-primary" : "hover:bg-muted/50"
                  )}
                  style={{ contentVisibility: "auto", containIntrinsicSize: "auto 28px" }}
                  onClick={() => !isRenaming && setSelected(fullPath)}
                  onDoubleClick={() => {
                    if (isRenaming) return;
                    if (entry.isDir) onNavigate(fullPath);
                  }}
                  onContextMenu={(e) => {
                    if (isRenaming) return;
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
                  {isRenaming ? (
                    <EditRow
                      kind="rename"
                      initialValue={entry.name}
                      isDirHint={entry.isDir}
                      error={editError}
                      submitting={editSubmitting}
                      onCommit={onEditCommit}
                      onCancel={onEditCancel}
                      inline
                    />
                  ) : (
                    <>
                      <span className="flex-1 truncate">{entry.name}</span>
                      <span className="text-muted-foreground shrink-0 text-[10px] font-mono tabular-nums">
                        {formatMode(entry.mode, entry.isDir)}
                      </span>
                      {!entry.isDir && (
                        <span className="text-muted-foreground shrink-0 text-[10px]">{formatBytes(entry.size)}</span>
                      )}
                      <span className="text-muted-foreground shrink-0 text-[10px]">{formatDate(entry.modTime)}</span>
                    </>
                  )}
                </div>
              );
            })}
          </>
        )}
      </div>
    </ScrollArea>
  );
}

interface EditRowProps {
  kind: "create-file" | "create-dir" | "rename";
  initialValue: string;
  isDirHint?: boolean;
  error: string | null;
  submitting: boolean;
  onCommit: (name: string) => void;
  onCancel: () => void;
  inline?: boolean;
}

function EditRow({ kind, initialValue, isDirHint, error, submitting, onCommit, onCancel, inline }: EditRowProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState(initialValue);
  const inputRef = useRef<HTMLInputElement>(null);
  const committedRef = useRef(false);

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.focus();
    if (kind === "rename" && initialValue) {
      // 选中文件 base（去扩展名），目录则全选
      const dot = initialValue.lastIndexOf(".");
      if (!isDirHint && dot > 0) {
        el.setSelectionRange(0, dot);
      } else {
        el.select();
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const submit = () => {
    if (committedRef.current || submitting) return;
    const localErr = validateEntryName(value);
    if (localErr) {
      // 留给上层 error 显示；这里只把空名当作 cancel
      if (localErr === "errNameEmpty" && value === initialValue) {
        onCancel();
        return;
      }
    }
    committedRef.current = true;
    onCommit(value);
  };

  const cancel = () => {
    if (committedRef.current) return;
    committedRef.current = true;
    onCancel();
  };

  // 提交期错误进来后允许再次尝试
  useEffect(() => {
    if (error) committedRef.current = false;
  }, [error]);

  const localValidationError = (() => {
    const code = validateEntryName(value);
    if (!code) return null;
    if (code === "errNameEmpty" && value === "") return null; // 空名只在提交时报
    return t("sftp." + code);
  })();

  const showError = error ?? localValidationError;

  const placeholder =
    kind === "create-file"
      ? t("sftp.newFile")
      : kind === "create-dir"
        ? t("sftp.newFolder")
        : t("sftp.namePlaceholder");

  if (inline) {
    return (
      <div className="flex-1 flex flex-col gap-0.5 min-w-0">
        <Input
          ref={inputRef}
          value={value}
          disabled={submitting}
          placeholder={placeholder}
          className="h-5 text-xs px-1 py-0 rounded-sm"
          onChange={(e) => setValue(e.target.value)}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              submit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
          onBlur={submit}
        />
        {showError && <span className="text-destructive text-[10px] truncate">{showError}</span>}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-0.5 px-2 py-1 bg-primary/5">
      <div className="flex items-center gap-1.5">
        {kind === "create-dir" ? (
          <Folder className="h-3.5 w-3.5 text-primary/70 shrink-0" />
        ) : (
          <File className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        )}
        <Input
          ref={inputRef}
          value={value}
          disabled={submitting}
          placeholder={placeholder}
          className="h-5 text-xs px-1 py-0 rounded-sm flex-1 min-w-0"
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              submit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
          onBlur={submit}
        />
        {submitting && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
      </div>
      {showError && <span className="text-destructive text-[10px] pl-5 truncate">{showError}</span>}
    </div>
  );
}
