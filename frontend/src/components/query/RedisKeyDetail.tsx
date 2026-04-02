import { useState, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Key, Loader2, Send, ChevronRight, Trash2, X, RefreshCw, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button, Input, ConfirmDialog } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteRedis, ExecuteRedisArgs } from "../../../wailsjs/go/app/App";
import { RedisStringEditor } from "@/components/query/RedisStringEditor";
import { RedisCollectionTable } from "@/components/query/RedisCollectionTable";

interface RedisKeyDetailProps {
  tabId: string;
}

interface RedisResult {
  type: string;
  value: unknown;
}

function formatTtl(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return s > 0 ? `${m}m${s}s` : `${m}m`;
  }
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return m > 0 ? `${h}h${m}m` : `${h}h`;
  }
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  return h > 0 ? `${d}d${h}h` : `${d}d`;
}

const TYPE_COLORS: Record<string, string> = {
  string: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  hash: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  list: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400",
  set: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400",
  zset: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
};

function formatResult(parsed: RedisResult): string {
  if (parsed.type === "nil") return "(nil)";
  if (parsed.type === "string" || parsed.type === "integer") {
    return String(parsed.value);
  }
  if (parsed.type === "list" && Array.isArray(parsed.value)) {
    return (parsed.value as unknown[]).map((v, i) => `${i + 1}) ${JSON.stringify(v)}`).join("\n");
  }
  if (parsed.type === "hash" && typeof parsed.value === "object" && parsed.value !== null) {
    return Object.entries(parsed.value as Record<string, unknown>)
      .map(([k, v]) => `${k} => ${JSON.stringify(v)}`)
      .join("\n");
  }
  return JSON.stringify(parsed.value, null, 2);
}

export function RedisKeyDetail({ tabId }: RedisKeyDetailProps) {
  const { t } = useTranslation();
  const { redisStates, removeKey, loadDbKeyCounts, selectKey } = useQueryStore();
  const state = redisStates[tabId];
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;

  const [command, setCommand] = useState("");
  const [executing, setExecuting] = useState(false);
  const [cmdResult, setCmdResult] = useState<string | null>(null);
  const [cmdError, setCmdError] = useState<string | null>(null);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState(-1);
  const [deleting, setDeleting] = useState(false);
  const [showDeleteKeyConfirm, setShowDeleteKeyConfirm] = useState(false);
  const [editingTtl, setEditingTtl] = useState(false);
  const [ttlInput, setTtlInput] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const ttlInputRef = useRef<HTMLInputElement>(null);

  const executeCommand = useCallback(async () => {
    if (!command.trim() || !tabMeta || !state) return;

    setExecuting(true);
    setCmdResult(null);
    setCmdError(null);

    setHistory((prev) => {
      const next = [command, ...prev.filter((c) => c !== command)].slice(0, 20);
      return next;
    });
    setHistoryIdx(-1);

    try {
      const result = await ExecuteRedis(tabMeta.assetId, command.trim(), state.currentDb);
      const parsed: RedisResult = JSON.parse(result);
      setCmdResult(formatResult(parsed));
    } catch (err) {
      setCmdError(String(err));
    } finally {
      setExecuting(false);
    }
  }, [command, tabMeta, state]);

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

  const handleDeleteKey = useCallback(async () => {
    if (!tabMeta || !state?.selectedKey) return;
    setDeleting(true);
    try {
      await ExecuteRedisArgs(tabMeta.assetId, ["DEL", state.selectedKey], state.currentDb);
      removeKey(tabId, state.selectedKey);
      loadDbKeyCounts(tabId);
    } catch (err) {
      toast.error(String(err));
    }
    setDeleting(false);
  }, [tabMeta, state, tabId, removeKey, loadDbKeyCounts]);

  const handleRefreshKey = useCallback(() => {
    if (state?.selectedKey) {
      selectKey(tabId, state.selectedKey);
    }
  }, [state?.selectedKey, tabId, selectKey]);

  const handleCopyKeyName = useCallback(() => {
    if (state?.selectedKey) {
      navigator.clipboard.writeText(state.selectedKey);
      toast.success(t("query.copied"));
    }
  }, [state?.selectedKey, t]);

  const startTtlEdit = useCallback(() => {
    if (!state?.keyInfo) return;
    setTtlInput(state.keyInfo.ttl > 0 ? String(state.keyInfo.ttl) : "");
    setEditingTtl(true);
    setTimeout(() => ttlInputRef.current?.focus(), 0);
  }, [state?.keyInfo]);

  const saveTtl = useCallback(async () => {
    if (!tabMeta || !state?.selectedKey) return;
    const seconds = parseInt(ttlInput, 10);
    if (isNaN(seconds) || seconds <= 0) return;
    try {
      await ExecuteRedisArgs(tabMeta.assetId, ["EXPIRE", state.selectedKey, String(seconds)], state.currentDb);
      selectKey(tabId, state.selectedKey);
    } catch (err) {
      toast.error(String(err));
    }
    setEditingTtl(false);
  }, [tabMeta, state, ttlInput, tabId, selectKey]);

  const persistKey = useCallback(async () => {
    if (!tabMeta || !state?.selectedKey) return;
    try {
      await ExecuteRedisArgs(tabMeta.assetId, ["PERSIST", state.selectedKey], state.currentDb);
      selectKey(tabId, state.selectedKey);
    } catch (err) {
      toast.error(String(err));
    }
    setEditingTtl(false);
  }, [tabMeta, state, tabId, selectKey]);

  if (!state) return null;

  const hasKey = !!state.selectedKey;
  const keyInfo = state.keyInfo;
  const type = keyInfo?.type;
  const ttl = keyInfo?.ttl ?? -1;
  const isCollection = type === "hash" || type === "list" || type === "set" || type === "zset";

  const ttlDisplay = ttl === -1 ? t("query.ttlPersist") : formatTtl(ttl);

  return (
    <div className="flex h-full flex-col">
      {/* Main content area */}
      <div className="flex-1 min-h-0 flex flex-col">
        {!hasKey ? (
          <div className="flex flex-1 items-center justify-center text-muted-foreground">
            <div className="text-center">
              <Key className="mx-auto mb-2 size-8 opacity-40" />
              <p className="text-sm">{t("query.noKeySelected")}</p>
            </div>
          </div>
        ) : !keyInfo ? (
          <div className="flex flex-1 items-center justify-center">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <>
            {/* Header */}
            <div className="flex items-center gap-2 border-b px-3 py-2">
              <Key className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate font-mono text-sm font-medium">{state.selectedKey}</span>
              <Button variant="ghost" size="icon-xs" onClick={handleCopyKeyName} title={t("query.copyKeyName")}>
                <Copy className="size-3 text-muted-foreground" />
              </Button>
              <span
                className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${TYPE_COLORS[type!] || "bg-muted text-muted-foreground"}`}
              >
                {type}
              </span>
              {/* TTL - editable */}
              {editingTtl ? (
                <div className="flex items-center gap-1">
                  <Input
                    ref={ttlInputRef}
                    className="h-6 w-20 font-mono text-xs"
                    placeholder={t("query.ttlInput")}
                    value={ttlInput}
                    onChange={(e) => setTtlInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") saveTtl();
                      if (e.key === "Escape") setEditingTtl(false);
                    }}
                  />
                  <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={saveTtl}>
                    {t("query.setTtl")}
                  </Button>
                  {ttl > 0 && (
                    <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={persistKey}>
                      {t("query.persist")}
                    </Button>
                  )}
                  <Button variant="ghost" size="icon-xs" onClick={() => setEditingTtl(false)}>
                    <X className="size-3" />
                  </Button>
                </div>
              ) : (
                <span
                  className="text-xs text-muted-foreground cursor-pointer hover:text-foreground"
                  onClick={startTtlEdit}
                  title={ttl > 0 ? `${ttl}s` : undefined}
                >
                  {t("query.ttl")}: {ttlDisplay}
                </span>
              )}
              <div className="ml-auto flex items-center gap-0.5">
                <Button variant="ghost" size="icon-xs" onClick={handleRefreshKey} title={t("query.refreshKey")}>
                  <RefreshCw className="size-3.5 text-muted-foreground" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => setShowDeleteKeyConfirm(true)}
                  disabled={deleting}
                  title={t("query.deleteKey")}
                >
                  {deleting ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="size-3.5 text-muted-foreground hover:text-destructive" />
                  )}
                </Button>
              </div>
            </div>

            {/* Value display */}
            {isCollection ? (
              <div className="min-h-0 flex-1">
                <RedisCollectionTable info={keyInfo} tabId={tabId} t={t} />
              </div>
            ) : (
              <RedisStringEditor tabId={tabId} t={t} />
            )}
          </>
        )}
      </div>

      {/* Command input - always visible */}
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
          <Button variant="ghost" size="icon-xs" onClick={executeCommand} disabled={executing || !command.trim()}>
            {executing ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
          </Button>
        </div>

        {/* Command result - scrollable with clear */}
        {(cmdResult !== null || cmdError !== null) && (
          <div className="relative border-t max-h-[200px] overflow-auto px-3 py-2">
            <button
              className="absolute right-2 top-2"
              onClick={() => {
                setCmdResult(null);
                setCmdError(null);
              }}
              title={t("query.clearResult")}
            >
              <X className="size-3 text-muted-foreground hover:text-foreground" />
            </button>
            {cmdError ? (
              <pre className="whitespace-pre-wrap font-mono text-xs text-destructive pr-6">
                {t("query.error")}: {cmdError}
              </pre>
            ) : (
              <pre className="whitespace-pre-wrap font-mono text-xs text-foreground pr-6">{cmdResult}</pre>
            )}
          </div>
        )}
      </div>

      {/* Delete key confirm */}
      <ConfirmDialog
        open={showDeleteKeyConfirm}
        onOpenChange={setShowDeleteKeyConfirm}
        title={t("query.deleteKey")}
        description={t("query.deleteKeyConfirmDesc", { name: state.selectedKey ?? "" })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={handleDeleteKey}
      />
    </div>
  );
}
