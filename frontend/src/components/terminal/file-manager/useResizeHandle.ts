import { useCallback, useRef, useState, type RefObject } from "react";
import { HANDLE_PX } from "./utils";

interface UseResizeHandleOptions {
  onWidthChange: (width: number) => void;
  panelRef: RefObject<HTMLDivElement | null>;
  width: number;
}

export function useResizeHandle({ onWidthChange, panelRef, width }: UseResizeHandleOptions) {
  const [isResizing, setIsResizing] = useState(false);
  const outerRef = useRef<HTMLDivElement>(null);

  const handleResizeStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      const startX = e.clientX;
      const startWidth = width;
      const prevCursor = document.body.style.cursor;
      document.body.style.cursor = "col-resize";

      const outer = outerRef.current;
      const prevOuterTransition = outer?.style.transition ?? "";
      if (outer) outer.style.transition = "none";

      let pending = startWidth;
      let rafId: number | null = null;
      const flushToDom = () => {
        rafId = null;
        if (panelRef.current) panelRef.current.style.width = `${pending}px`;
        if (outer) outer.style.width = `${pending + HANDLE_PX}px`;
      };

      const onMove = (e: MouseEvent) => {
        pending = startWidth + (startX - e.clientX);
        if (rafId == null) rafId = requestAnimationFrame(flushToDom);
      };
      const onUp = () => {
        if (rafId != null) {
          cancelAnimationFrame(rafId);
          rafId = null;
        }
        setIsResizing(false);
        document.body.style.cursor = prevCursor;
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
        if (panelRef.current) panelRef.current.style.width = "";
        if (outer) {
          outer.style.width = "";
          outer.style.transition = prevOuterTransition;
        }
        onWidthChange(pending);
      };
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
    },
    [onWidthChange, panelRef, width]
  );

  return { handleResizeStart, isResizing, outerRef };
}
