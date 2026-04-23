import { useRef } from "react";
import { useResizeHandle } from "@opskat/ui";
import { RedisKeyBrowser } from "./RedisKeyBrowser";
import { RedisKeyDetail } from "./RedisKeyDetail";

interface RedisPanelProps {
  tabId: string;
}

export function RedisPanel({ tabId }: RedisPanelProps) {
  const sidebarRef = useRef<HTMLDivElement>(null);
  const { size: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultSize: 220,
    minSize: 160,
    maxSize: 400,
    targetRef: sidebarRef,
  });

  return (
    <div className="flex h-full">
      {/* Left: Key browser */}
      <div ref={sidebarRef} className="shrink-0 border-r" style={{ width: sidebarWidth }}>
        <RedisKeyBrowser tabId={tabId} />
      </div>

      {/* Resize handle */}
      <div className="w-1 shrink-0 cursor-col-resize hover:bg-accent active:bg-accent" onMouseDown={handleMouseDown} />

      {/* Right: Key detail */}
      <div className="min-w-0 flex-1">
        <RedisKeyDetail tabId={tabId} />
      </div>
    </div>
  );
}
