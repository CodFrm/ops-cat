import { useState, useCallback, useRef, useEffect } from "react";

interface UseResizeHandleOptions {
  defaultWidth: number;
  minWidth: number;
  maxWidth: number;
  /** true for right-side panels where dragging left makes the panel wider */
  reverse?: boolean;
  /** localStorage key — if set, width is persisted across sessions */
  storageKey?: string;
}

function clamp(val: number, min: number, max: number) {
  return Math.max(min, Math.min(max, val));
}

export function useResizeHandle({
  defaultWidth,
  minWidth,
  maxWidth,
  reverse = false,
  storageKey,
}: UseResizeHandleOptions) {
  const [width, setWidth] = useState(() => {
    if (storageKey) {
      const saved = localStorage.getItem(storageKey);
      if (saved) return clamp(Number(saved), minWidth, maxWidth);
    }
    return defaultWidth;
  });
  const [isResizing, setIsResizing] = useState(false);
  const widthRef = useRef(width);
  useEffect(() => {
    widthRef.current = width;
  }, [width]);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      const startX = e.clientX;
      const startW = widthRef.current;

      const onMouseMove = (ev: MouseEvent) => {
        const delta = reverse ? startX - ev.clientX : ev.clientX - startX;
        setWidth(clamp(startW + delta, minWidth, maxWidth));
      };

      const onMouseUp = () => {
        setIsResizing(false);
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        if (storageKey) {
          localStorage.setItem(storageKey, String(widthRef.current));
        }
      };

      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    [minWidth, maxWidth, reverse, storageKey]
  );

  return { width, isResizing, handleMouseDown };
}
