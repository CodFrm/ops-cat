import { useEffect, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Database, RefreshCw, Loader2, Search, Key } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useQueryStore } from "@/stores/queryStore";

interface RedisKeyBrowserProps {
  tabId: string;
}

const KEY_ROW_HEIGHT = 28;

export function RedisKeyBrowser({ tabId }: RedisKeyBrowserProps) {
  const { t } = useTranslation();
  const { redisStates, scanKeys, selectRedisDb, selectKey, setKeyFilter } =
    useQueryStore();
  const state = redisStates[tabId];
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: state?.keys.length ?? 0,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => KEY_ROW_HEIGHT,
    overscan: 20,
  });

  useEffect(() => {
    scanKeys(tabId, true);
  }, [tabId, scanKeys]);

  const handleDbChange = useCallback(
    (value: string) => {
      selectRedisDb(tabId, Number(value));
    },
    [tabId, selectRedisDb]
  );

  const handleFilterChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const pattern = e.target.value || "*";
      setKeyFilter(tabId, pattern);

      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
      debounceRef.current = setTimeout(() => {
        scanKeys(tabId, true);
      }, 300);
    },
    [tabId, setKeyFilter, scanKeys]
  );

  const handleRefresh = useCallback(() => {
    scanKeys(tabId, true);
  }, [tabId, scanKeys]);

  const handleLoadMore = useCallback(() => {
    scanKeys(tabId, false);
  }, [tabId, scanKeys]);

  const handleSelectKey = useCallback(
    (key: string) => {
      selectKey(tabId, key);
    },
    [tabId, selectKey]
  );

  if (!state) return null;

  const dbOptions = Array.from({ length: 16 }, (_, i) => i);

  return (
    <div className="flex h-full flex-col">
      {/* DB selector + refresh */}
      <div className="flex items-center gap-1 border-b px-2 py-1.5">
        <Database className="size-3.5 shrink-0 text-muted-foreground" />
        <Select
          value={String(state.currentDb)}
          onValueChange={handleDbChange}
        >
          <SelectTrigger size="sm" className="h-7 flex-1 text-xs">
            <SelectValue placeholder={t("query.selectDb")} />
          </SelectTrigger>
          <SelectContent>
            {dbOptions.map((db) => (
              <SelectItem key={db} value={String(db)}>
                db{db}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={handleRefresh}
          disabled={state.loadingKeys}
        >
          <RefreshCw
            className={`size-3.5 ${state.loadingKeys ? "animate-spin" : ""}`}
          />
        </Button>
      </div>

      {/* Filter input */}
      <div className="border-b px-2 py-1.5">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-7 pl-7 text-xs"
            placeholder={t("query.filterKeys")}
            value={state.keyFilter === "*" ? "" : state.keyFilter}
            onChange={handleFilterChange}
          />
        </div>
      </div>

      {/* Key count */}
      <div className="border-b px-2 py-1 text-xs text-muted-foreground">
        {t("query.keyCount", { count: state.keys.length })}
      </div>

      {/* Virtualized key list */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        <div
          style={{ height: virtualizer.getTotalSize(), position: "relative" }}
        >
          {virtualizer.getVirtualItems().map((virtualRow) => {
            const key = state.keys[virtualRow.index];
            return (
              <button
                key={key}
                className={`absolute left-0 flex w-full items-center gap-1.5 px-2 text-left text-xs font-mono hover:bg-accent ${
                  state.selectedKey === key
                    ? "bg-accent text-accent-foreground"
                    : ""
                }`}
                style={{
                  top: virtualRow.start,
                  height: virtualRow.size,
                }}
                onClick={() => handleSelectKey(key)}
              >
                <Key className="size-3 shrink-0 text-muted-foreground" />
                <span className="truncate">{key}</span>
              </button>
            );
          })}
        </div>

        {state.loadingKeys && (
          <div className="flex items-center justify-center py-3">
            <Loader2 className="size-4 animate-spin text-muted-foreground" />
          </div>
        )}
      </div>

      {/* Load more */}
      {state.hasMore && !state.loadingKeys && (
        <div className="border-t px-2 py-1.5">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-full text-xs"
            onClick={handleLoadMore}
          >
            {t("query.loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}
