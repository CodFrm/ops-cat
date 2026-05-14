import { useEffect, useState, type MutableRefObject, type RefObject } from "react";
import { OnFileDrop, OnFileDropOff } from "../../../../wailsjs/runtime/runtime";

interface UseNativeFileDropOptions {
  currentPathRef: MutableRefObject<string>;
  isActive: boolean;
  isOpen: boolean;
  panelRef: RefObject<HTMLDivElement | null>;
  tabId: string;
  sessionId: string;
  startUploadFile: (
    target: { tabId: string; sessionId: string },
    localPath: string,
    remotePath: string
  ) => Promise<string | null>;
}

export function useNativeFileDrop({
  currentPathRef,
  isActive,
  isOpen,
  panelRef,
  tabId,
  sessionId,
  startUploadFile,
}: UseNativeFileDropOptions) {
  const [isDragOver, setIsDragOver] = useState(false);

  useEffect(() => {
    if (!isOpen || !isActive) return;
    const handler = (_x: number, _y: number, paths: string[]) => {
      setIsDragOver(false);
      for (const path of paths) {
        startUploadFile({ tabId, sessionId }, path, currentPathRef.current + "/");
      }
    };
    OnFileDrop(handler, true);
    return () => {
      OnFileDropOff();
    };
  }, [currentPathRef, isActive, isOpen, sessionId, startUploadFile, tabId]);

  useEffect(() => {
    const el = panelRef.current;
    if (!el || !isOpen || !isActive) return;
    const observer = new MutationObserver(() => {
      setIsDragOver(el.classList.contains("wails-drop-target-active"));
    });
    observer.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, [isActive, isOpen, panelRef]);

  return isDragOver;
}
