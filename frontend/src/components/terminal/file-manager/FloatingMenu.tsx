import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowUp,
  Download,
  FilePlus,
  FolderDown,
  FolderOpen,
  FolderPlus,
  KeyRound,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react";
import { cn } from "@opskat/ui";
import { type CtxMenuState } from "./types";

interface FloatingMenuProps {
  ctx: CtxMenuState;
  canGoUp: boolean;
  onAction: (action: string) => void;
  onClose: () => void;
}

export function FloatingMenu({ ctx, canGoUp, onAction, onClose }: FloatingMenuProps) {
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

  const item = (action: string, icon: ReactNode, label: string, variant?: "destructive") => (
    <div
      key={action}
      className={cn(
        "flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm cursor-default select-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        variant === "destructive"
          ? "text-destructive hover:bg-destructive/10 [&_svg]:text-destructive"
          : "hover:bg-accent hover:text-accent-foreground [&_svg:not([class*='text-'])]:text-muted-foreground"
      )}
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
            {item("downloadDir", <FolderDown />, t("sftp.downloadDir"))}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("rename", <Pencil />, t("sftp.rename"))}
            {item("chmod", <KeyRound />, t("sftp.permissions"))}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("delete", <Trash2 />, t("action.delete"), "destructive")}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("newFile", <FilePlus />, t("sftp.newFile"))}
            {item("newDir", <FolderPlus />, t("sftp.newFolder"))}
            {canGoUp && item("goUp", <ArrowUp />, t("sftp.parentDir"))}
            {item("refresh", <RefreshCw />, t("sftp.refresh"))}
          </>
        ) : (
          <>
            {item("download", <Download />, t("sftp.download"))}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("rename", <Pencil />, t("sftp.rename"))}
            {item("chmod", <KeyRound />, t("sftp.permissions"))}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("delete", <Trash2 />, t("action.delete"), "destructive")}
            <div className="-mx-1 my-1 h-px bg-border" />
            {item("newFile", <FilePlus />, t("sftp.newFile"))}
            {item("newDir", <FolderPlus />, t("sftp.newFolder"))}
            {canGoUp && item("goUp", <ArrowUp />, t("sftp.parentDir"))}
            {item("refresh", <RefreshCw />, t("sftp.refresh"))}
          </>
        )
      ) : (
        <>
          {item("newFile", <FilePlus />, t("sftp.newFile"))}
          {item("newDir", <FolderPlus />, t("sftp.newFolder"))}
          <div className="-mx-1 my-1 h-px bg-border" />
          {item("upload", <Upload />, t("sftp.upload"))}
          {item("uploadDir", <Upload />, t("sftp.uploadDir"))}
          <div className="-mx-1 my-1 h-px bg-border" />
          {canGoUp && item("goUp", <ArrowUp />, t("sftp.parentDir"))}
          {item("refresh", <RefreshCw />, t("sftp.refresh"))}
        </>
      )}
    </div>,
    document.body
  );
}
