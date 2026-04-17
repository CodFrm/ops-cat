import { useCallback, useRef } from "react";
import { useLayoutStore, isCollapsed, MIN_PANEL_WIDTH } from "@/stores/layoutStore";

interface LeftPanelProps {
  children: React.ReactNode;
}

export function LeftPanel({ children }: LeftPanelProps) {
  const width = useLayoutStore((s) => s.leftPanelWidth);
  const setPanelWidth = useLayoutStore((s) => s.setPanelWidth);
  const collapsed = isCollapsed({ leftPanelWidth: width });
  const effectiveWidth = collapsed ? MIN_PANEL_WIDTH : width;
  const startXRef = useRef(0);
  const startWRef = useRef(0);

  const onResizeStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startXRef.current = e.clientX;
      startWRef.current = width;

      const onMove = (ev: MouseEvent) => {
        const delta = ev.clientX - startXRef.current;
        setPanelWidth(startWRef.current + delta);
      };
      const onUp = () => {
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
      };
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
    },
    [width, setPanelWidth]
  );

  return (
    <div className="relative shrink-0 overflow-hidden border-r" style={{ width: effectiveWidth }}>
      {children}
      <div
        onMouseDown={onResizeStart}
        className="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30"
      />
    </div>
  );
}
