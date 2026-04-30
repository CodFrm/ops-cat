import { useEffect, useRef, useState, type WheelEvent } from "react";
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

function getKeyFromView(view: string) {
  return view.startsWith("key:") ? view.slice(4) : null;
}

export function RedisPanel({ tabId }: RedisPanelProps) {
  const { t } = useTranslation();
  const selectedKey = useQueryStore((s) => s.redisStates[tabId]?.selectedKey);
  const removedKey = useQueryStore((s) => s.redisStates[tabId]?.removedKey);
  const removedKeySeq = useQueryStore((s) => s.redisStates[tabId]?.removedKeySeq);
  const selectKey = useQueryStore((s) => s.selectKey);
  const clearSelectedKey = useQueryStore((s) => s.clearSelectedKey);
  const [activeView, setActiveView] = useState<string>(REDIS_OVERVIEW_VIEW);
  const [openKeys, setOpenKeys] = useState<string[]>([]);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const tabStripRef = useRef<HTMLDivElement>(null);
  const { size: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultSize: 220,
    minSize: 160,
    maxSize: 400,
    targetRef: sidebarRef,
  });

  useEffect(() => {
    if (selectedKey) {
      setOpenKeys((prev) => (prev.includes(selectedKey) ? prev : [...prev, selectedKey]));
      setActiveView(getKeyViewId(selectedKey));
    }
  }, [selectedKey]);

  const activeKey = getKeyFromView(activeView);

  useEffect(() => {
    if (!selectedKey && activeKey) {
      setActiveView(REDIS_OVERVIEW_VIEW);
    }
  }, [activeKey, selectedKey]);

  const activateKeyTab = (key: string) => {
    setActiveView(getKeyViewId(key));
    if (selectedKey !== key) {
      selectKey(tabId, key);
    }
  };

  const closeKeyTab = (key: string) => {
    setOpenKeys((prev) => {
      const next = prev.filter((item) => item !== key);
      if (activeKey === key) {
        const currentIndex = prev.indexOf(key);
        const fallback = next[Math.min(currentIndex, next.length - 1)] ?? null;
        if (fallback) {
          setActiveView(getKeyViewId(fallback));
          selectKey(tabId, fallback);
        } else {
          setActiveView(REDIS_OVERVIEW_VIEW);
          clearSelectedKey(tabId, key);
        }
      }
      return next;
    });
  };

  useEffect(() => {
    if (!removedKey || !removedKeySeq) return;
    setOpenKeys((prev) => {
      if (!prev.includes(removedKey)) return prev;
      const next = prev.filter((item) => item !== removedKey);
      if (activeKey === removedKey) {
        const currentIndex = prev.indexOf(removedKey);
        const fallback = next[Math.min(currentIndex, next.length - 1)] ?? null;
        if (fallback) {
          setActiveView(getKeyViewId(fallback));
          selectKey(tabId, fallback);
        } else {
          setActiveView(REDIS_OVERVIEW_VIEW);
        }
      }
      return next;
    });
  }, [activeKey, removedKey, removedKeySeq, selectKey, tabId]);

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
            onClick={() => setActiveView(REDIS_OVERVIEW_VIEW)}
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
