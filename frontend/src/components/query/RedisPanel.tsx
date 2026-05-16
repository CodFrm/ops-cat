import { useRef, type WheelEvent } from "react";
import { useTranslation } from "react-i18next";
import { Activity, Key, X } from "lucide-react";
import { useResizeHandle } from "@opskat/ui";
import { RedisKeyBrowser } from "./RedisKeyBrowser";
import { RedisKeyDetail } from "./RedisKeyDetail";
import { RedisOpsPanel } from "./RedisOpsPanel";
import { useQueryStore } from "@/stores/queryStore";

interface RedisPanelProps {
  tabId: string;
}

const REDIS_OVERVIEW_VIEW = "overview";

function getKeyViewId(key: string) {
  return `key:${key}`;
}

export function RedisPanel({ tabId }: RedisPanelProps) {
  const { t } = useTranslation();
  const redisState = useQueryStore((s) => s.redisStates[tabId]);
  const selectedKey = redisState?.selectedKey ?? null;
  const openKeys = redisState?.openKeyTabs ?? (selectedKey ? [selectedKey] : []);
  const activeKey = redisState?.activeRedisKey === undefined ? selectedKey : redisState.activeRedisKey;
  const activeView = activeKey ? getKeyViewId(activeKey) : REDIS_OVERVIEW_VIEW;
  const activateRedisOverview = useQueryStore((s) => s.activateRedisOverview);
  const activateRedisKeyTab = useQueryStore((s) => s.activateRedisKeyTab);
  const closeRedisKeyTab = useQueryStore((s) => s.closeRedisKeyTab);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const tabStripRef = useRef<HTMLDivElement>(null);
  const { size: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultSize: 220,
    minSize: 160,
    maxSize: 400,
    targetRef: sidebarRef,
  });

  const activateKeyTab = (key: string) => {
    activateRedisKeyTab(tabId, key);
  };

  const closeKeyTab = (key: string) => {
    closeRedisKeyTab(tabId, key);
  };

  const handleTabStripWheel = (event: WheelEvent<HTMLDivElement>) => {
    const target = tabStripRef.current;
    if (!target) return;

    const scrollDelta = Math.abs(event.deltaY) > Math.abs(event.deltaX) ? event.deltaY : event.deltaX;
    if (scrollDelta === 0) return;

    event.preventDefault();
    target.scrollLeft += scrollDelta;
  };

  return (
    <div className="flex h-full w-full">
      {/* Left: Key browser */}
      <div ref={sidebarRef} className="shrink-0 border-r" style={{ width: sidebarWidth }}>
        <RedisKeyBrowser tabId={tabId} />
      </div>

      {/* Resize handle */}
      <div className="w-1 shrink-0 cursor-col-resize hover:bg-accent active:bg-accent" onMouseDown={handleMouseDown} />

      {/* Right: Redis pages */}
      <div className="flex min-w-0 flex-1 flex-col">
        <div
          ref={tabStripRef}
          role="tablist"
          data-testid="redis-key-tab-strip"
          className="flex h-9 shrink-0 items-stretch overflow-x-auto overflow-y-hidden border-b bg-muted/30"
          onWheel={handleTabStripWheel}
        >
          <button
            role="tab"
            aria-selected={activeView === REDIS_OVERVIEW_VIEW}
            title={t("query.redisOverview")}
            className={`flex h-9 shrink-0 items-center gap-1.5 border-r px-3 text-xs transition-colors ${
              activeView === REDIS_OVERVIEW_VIEW
                ? "bg-background text-foreground"
                : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
            }`}
            onClick={() => activateRedisOverview(tabId)}
          >
            <Activity className="size-3" />
            {t("query.redisOverview")}
          </button>
          {openKeys.map((key) => {
            const viewId = getKeyViewId(key);
            const selected = activeView === viewId;
            return (
              <div
                key={key}
                className={`flex h-9 w-56 max-w-[320px] shrink-0 items-stretch border-r text-xs transition-colors ${
                  selected
                    ? "bg-background text-foreground"
                    : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
                }`}
              >
                <button
                  role="tab"
                  aria-selected={selected}
                  title={key}
                  className="flex h-9 min-w-0 flex-1 items-center gap-1.5 px-3 text-left"
                  onClick={() => activateKeyTab(key)}
                >
                  <Key className="size-3 shrink-0" />
                  <span className="truncate font-mono">{key}</span>
                </button>
                <button
                  type="button"
                  aria-label={`${t("query.closeRedisKeyTab")} ${key}`}
                  title={t("action.close")}
                  className="my-1 mr-1 flex size-7 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={(event) => {
                    event.stopPropagation();
                    closeKeyTab(key);
                  }}
                >
                  <X className="size-3" />
                </button>
              </div>
            );
          })}
        </div>

        <div className="relative min-h-0 flex-1">
          <div className="absolute inset-0" style={{ display: activeView === REDIS_OVERVIEW_VIEW ? "block" : "none" }}>
            <RedisOpsPanel tabId={tabId} />
          </div>
          {selectedKey && activeKey && (
            <div className="absolute inset-0" style={{ display: selectedKey === activeKey ? "block" : "none" }}>
              <RedisKeyDetail tabId={tabId} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
