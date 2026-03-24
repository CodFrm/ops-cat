import { useState, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Key, Loader2, Send, ChevronRight } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useQueryStore, RedisKeyInfo } from "@/stores/queryStore";
import { ExecuteRedis } from "../../../wailsjs/go/main/App";

interface RedisKeyDetailProps {
  tabId: string;
}

interface RedisResult {
  type: string;
  value: unknown;
}

const TYPE_COLORS: Record<string, string> = {
  string: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  hash: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  list: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400",
  set: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400",
  zset: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
};

const VALUE_ROW_HEIGHT = 30;

function formatResult(parsed: RedisResult): string {
  if (parsed.type === "nil") return "(nil)";
  if (parsed.type === "string" || parsed.type === "integer") {
    return String(parsed.value);
  }
  if (parsed.type === "list" && Array.isArray(parsed.value)) {
    return (parsed.value as unknown[])
      .map((v, i) => `${i + 1}) ${JSON.stringify(v)}`)
      .join("\n");
  }
  if (parsed.type === "hash" && typeof parsed.value === "object" && parsed.value !== null) {
    return Object.entries(parsed.value as Record<string, unknown>)
      .map(([k, v]) => `${k} => ${JSON.stringify(v)}`)
      .join("\n");
  }
  return JSON.stringify(parsed.value, null, 2);
}

function getItemCount(info: RedisKeyInfo): number {
  switch (info.type) {
    case "hash":
      return ((info.value as [string, string][]) || []).length;
    case "list":
    case "set":
      return ((info.value as string[]) || []).length;
    case "zset":
      return ((info.value as [string, string][]) || []).length;
    default:
      return 0;
  }
}

function CollectionTable({ info, tabId, t }: {
  info: RedisKeyInfo;
  tabId: string;
  t: (key: string, opts?: Record<string, unknown>) => string;
}) {
  const { loadMoreValues } = useQueryStore();
  const scrollRef = useRef<HTMLDivElement>(null);
  const itemCount = getItemCount(info);

  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => VALUE_ROW_HEIGHT,
    overscan: 20,
  });

  const totalLabel = info.total >= 0
    ? t("query.loadedOfTotal", { loaded: itemCount, total: info.total })
    : `${itemCount}`;

  return (
    <div className="flex h-full flex-col">
      {/* Table header */}
      <div className="flex items-center border-b text-xs">
        {info.type === "hash" && (
          <>
            <div className="w-1/3 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.field")}
            </div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.value")}
            </div>
          </>
        )}
        {info.type === "list" && (
          <>
            <div className="w-16 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.index")}
            </div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.value")}
            </div>
          </>
        )}
        {info.type === "set" && (
          <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">
            {t("query.member")}
          </div>
        )}
        {info.type === "zset" && (
          <>
            <div className="w-24 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.score")}
            </div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">
              {t("query.member")}
            </div>
          </>
        )}
        <div className="shrink-0 px-2 py-1.5 text-xs text-muted-foreground">
          {totalLabel}
        </div>
      </div>

      {/* Virtualized rows */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((virtualRow) => {
            const idx = virtualRow.index;
            return (
              <div
                key={virtualRow.key}
                className="absolute left-0 flex w-full items-center border-b text-xs font-mono last:border-0"
                style={{ top: virtualRow.start, height: virtualRow.size }}
              >
                {info.type === "hash" && (() => {
                  const entry = (info.value as [string, string][])[idx];
                  return (
                    <>
                      <div className="w-1/3 shrink-0 truncate px-2 text-foreground">{entry[0]}</div>
                      <div className="flex-1 truncate px-2 text-foreground">{entry[1]}</div>
                    </>
                  );
                })()}
                {info.type === "list" && (
                  <>
                    <div className="w-16 shrink-0 px-2 text-muted-foreground">{idx}</div>
                    <div className="flex-1 truncate px-2 text-foreground">
                      {(info.value as string[])[idx]}
                    </div>
                  </>
                )}
                {info.type === "set" && (
                  <div className="flex-1 truncate px-2 text-foreground">
                    {(info.value as string[])[idx]}
                  </div>
                )}
                {info.type === "zset" && (() => {
                  const pair = (info.value as [string, string][])[idx];
                  return (
                    <>
                      <div className="w-24 shrink-0 px-2 text-muted-foreground">{pair[1]}</div>
                      <div className="flex-1 truncate px-2 text-foreground">{pair[0]}</div>
                    </>
                  );
                })()}
              </div>
            );
          })}
        </div>
      </div>

      {/* Load more values */}
      {info.hasMoreValues && (
        <div className="border-t px-2 py-1.5">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-full text-xs"
            onClick={() => loadMoreValues(tabId)}
            disabled={info.loadingMore}
          >
            {info.loadingMore ? (
              <Loader2 className="mr-1 size-3 animate-spin" />
            ) : null}
            {t("query.loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}

export function RedisKeyDetail({ tabId }: RedisKeyDetailProps) {
  const { t } = useTranslation();
  const { redisStates, openTabs } = useQueryStore();
  const state = redisStates[tabId];
  const tab = openTabs.find((tb) => tb.id === tabId);

  const [command, setCommand] = useState("");
  const [executing, setExecuting] = useState(false);
  const [cmdResult, setCmdResult] = useState<string | null>(null);
  const [cmdError, setCmdError] = useState<string | null>(null);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);

  const executeCommand = useCallback(async () => {
    if (!command.trim() || !tab) return;

    setExecuting(true);
    setCmdResult(null);
    setCmdError(null);

    setHistory((prev) => {
      const next = [command, ...prev.filter((c) => c !== command)].slice(0, 20);
      return next;
    });
    setHistoryIdx(-1);

    try {
      const result = await ExecuteRedis(tab.assetId, command.trim());
      const parsed: RedisResult = JSON.parse(result);
      setCmdResult(formatResult(parsed));
    } catch (err) {
      setCmdError(String(err));
    } finally {
      setExecuting(false);
    }
  }, [command, tab]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter" && !executing) {
        e.preventDefault();
        executeCommand();
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        if (history.length === 0) return;
        const nextIdx = Math.min(historyIdx + 1, history.length - 1);
        setHistoryIdx(nextIdx);
        setCommand(history[nextIdx]);
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        if (historyIdx <= 0) {
          setHistoryIdx(-1);
          setCommand("");
          return;
        }
        const nextIdx = historyIdx - 1;
        setHistoryIdx(nextIdx);
        setCommand(history[nextIdx]);
      }
    },
    [executing, executeCommand, history, historyIdx]
  );

  if (!state) return null;

  // No key selected
  if (!state.selectedKey) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <div className="text-center">
          <Key className="mx-auto mb-2 size-8 opacity-40" />
          <p className="text-sm">{t("query.noKeySelected")}</p>
        </div>
      </div>
    );
  }

  // Key selected but info loading
  if (!state.keyInfo) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const { type, ttl } = state.keyInfo;
  const isCollection = type === "hash" || type === "list" || type === "set" || type === "zset";

  const ttlDisplay =
    ttl === -1
      ? t("query.ttlPersist")
      : t("query.ttlSeconds", { seconds: ttl });

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <Key className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate font-mono text-sm font-medium">
          {state.selectedKey}
        </span>
        <span
          className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${TYPE_COLORS[type] || "bg-muted text-muted-foreground"}`}
        >
          {type}
        </span>
        <span className="ml-auto text-xs text-muted-foreground">
          {t("query.ttl")}: {ttlDisplay}
        </span>
      </div>

      {/* Value display */}
      {isCollection ? (
        <div className="min-h-0 flex-1">
          <CollectionTable info={state.keyInfo} tabId={tabId} t={t} />
        </div>
      ) : (
        <ScrollArea className="flex-1">
          <div className="p-3">
            <pre className="whitespace-pre-wrap break-all rounded border bg-muted/50 p-3 font-mono text-xs">
              {String(state.keyInfo.value)}
            </pre>
          </div>
        </ScrollArea>
      )}

      {/* Command input */}
      <div className="border-t">
        <div className="flex items-center gap-1 px-2 py-1.5">
          <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
          <Input
            ref={inputRef}
            className="h-7 flex-1 font-mono text-xs"
            placeholder={t("query.redisPlaceholder")}
            value={command}
            onChange={(e) => {
              setCommand(e.target.value);
              setHistoryIdx(-1);
            }}
            onKeyDown={handleKeyDown}
            disabled={executing}
          />
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={executeCommand}
            disabled={executing || !command.trim()}
          >
            {executing ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <Send className="size-3.5" />
            )}
          </Button>
        </div>

        {/* Command result */}
        {(cmdResult !== null || cmdError !== null) && (
          <div className="border-t px-3 py-2">
            {cmdError ? (
              <pre className="whitespace-pre-wrap font-mono text-xs text-destructive">
                {t("query.error")}: {cmdError}
              </pre>
            ) : (
              <pre className="whitespace-pre-wrap font-mono text-xs text-foreground">
                {cmdResult}
              </pre>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
