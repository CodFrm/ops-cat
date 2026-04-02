import { useResizeHandle } from "@opskat/ui";
import { RedisKeyBrowser } from "./RedisKeyBrowser";
import { RedisKeyDetail } from "./RedisKeyDetail";

interface RedisPanelProps {
  tabId: string;
}

export function RedisPanel({ tabId }: RedisPanelProps) {
  const { width: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultWidth: 220,
    minWidth: 160,
    maxWidth: 400,
  });

  return (
    <div className="flex h-full">
      {/* Left: Key browser */}
      <div className="shrink-0 border-r" style={{ width: sidebarWidth }}>
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
