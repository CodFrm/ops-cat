import { useEffect, useState, type MutableRefObject, type RefObject } from "react";
import { OnFileDrop, OnFileDropOff } from "../../../../wailsjs/runtime/runtime";

interface UseNativeFileDropOptions {
  currentPathRef: MutableRefObject<string>;
  isOpen: boolean;
  panelRef: RefObject<HTMLDivElement | null>;
  sessionId: string;
  startUploadFile: (sessionId: string, localPath: string, remotePath: string) => Promise<string | null>;
}

export function useNativeFileDrop({
  currentPathRef,
  isOpen,
  panelRef,
  sessionId,
  startUploadFile,
}: UseNativeFileDropOptions) {
  const [isDragOver, setIsDragOver] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    const handler = (_x: number, _y: number, paths: string[]) => {
      setIsDragOver(false);
      for (const path of paths) {
        startUploadFile(sessionId, path, currentPathRef.current + "/");
      }
    };
    OnFileDrop(handler, true);
    return () => {
      OnFileDropOff();
    };
  }, [currentPathRef, isOpen, sessionId, startUploadFile]);

  useEffect(() => {
    const el = panelRef.current;
    if (!el || !isOpen) return;
    const observer = new MutationObserver(() => {
      setIsDragOver(el.classList.contains("wails-drop-target-active"));
    });
    observer.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, [isOpen, panelRef]);

  return isDragOver;
}
