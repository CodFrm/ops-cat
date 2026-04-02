import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Play, Loader2, History } from "lucide-react";
import {
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Popover,
  PopoverTrigger,
  PopoverContent,
  ConfirmDialog,
} from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { QueryResultTable } from "./QueryResultTable";

interface SqlEditorTabProps {
  tabId: string;
  innerTabId: string;
}

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  affected_rows?: number;
}

export function SqlEditorTab({ tabId, innerTabId }: SqlEditorTabProps) {
  const { t } = useTranslation();
  const { dbStates, updateInnerTab } = useQueryStore();
  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const queryMeta = tab?.meta as QueryTabMeta | undefined;
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const dbState = dbStates[tabId];
  const assetId = queryMeta?.assetId ?? 0;
  const databases = useMemo(() => dbState?.databases || [], [dbState?.databases]);

  // Restore persisted state from store
  const innerTab = dbState?.innerTabs.find((t) => t.id === innerTabId);
  const persistedSql = innerTab?.type === "sql" ? innerTab.sql : undefined;
  const persistedDb = innerTab?.type === "sql" ? innerTab.selectedDb : undefined;

  const [sql, setSql] = useState(persistedSql || "");
  const [selectedDb, setSelectedDb] = useState(persistedDb || queryMeta?.defaultDatabase || "");
  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  const [affectedRows, setAffectedRows] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showDangerConfirm, setShowDangerConfirm] = useState(false);
  const [sqlHistory, setSqlHistory] = useState<string[]>([]);
  const [showHistory, setShowHistory] = useState(false);

  // Set default database when databases load
  useEffect(() => {
    if (!selectedDb && databases.length > 0) {
      setSelectedDb(queryMeta?.defaultDatabase || databases[0]);
    }
  }, [databases, selectedDb, queryMeta?.defaultDatabase]);

  // Persist sql and selectedDb to store
  useEffect(() => {
    updateInnerTab(tabId, innerTabId, { sql });
  }, [sql, tabId, innerTabId, updateInnerTab]);

  useEffect(() => {
    updateInnerTab(tabId, innerTabId, { selectedDb });
  }, [selectedDb, tabId, innerTabId, updateInnerTab]);

  const isDangerousSQL = useCallback((text: string) => {
    const upper = text.toUpperCase().replace(/\s+/g, " ").trim();
    return /^(DELETE|DROP|TRUNCATE|ALTER)\b/.test(upper);
  }, []);

  // Get the SQL text to execute: selected text if any, otherwise full text
  const getExecutableSQL = useCallback(() => {
    const textarea = textareaRef.current;
    if (textarea) {
      const { selectionStart, selectionEnd } = textarea;
      if (selectionStart !== selectionEnd) {
        return sql.substring(selectionStart, selectionEnd).trim();
      }
    }
    return sql.trim();
  }, [sql]);

  const doExecute = useCallback(async () => {
    const execSql = getExecutableSQL();
    if (!execSql || !assetId) return;

    // Record to history (dedup, max 30)
    setSqlHistory((prev) => [execSql, ...prev.filter((s) => s !== execSql)].slice(0, 30));

    setLoading(true);
    setError(null);
    setAffectedRows(null);
    setColumns([]);
    setRows([]);

    try {
      const result = await ExecuteSQL(assetId, execSql, selectedDb);
      const parsed: SQLResult = JSON.parse(result);

      if (parsed.affected_rows !== undefined) {
        setAffectedRows(parsed.affected_rows);
      } else {
        setColumns(parsed.columns || []);
        setRows(parsed.rows || []);
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [getExecutableSQL, assetId, selectedDb]);

  const execute = useCallback(() => {
    const execSql = getExecutableSQL();
    if (!execSql || !assetId) return;
    if (isDangerousSQL(execSql)) {
      setShowDangerConfirm(true);
    } else {
      doExecute();
    }
  }, [getExecutableSQL, assetId, isDangerousSQL, doExecute]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        e.preventDefault();
        execute();
        return;
      }
      // Tab key inserts two spaces instead of moving focus
      if (e.key === "Tab") {
        e.preventDefault();
        const textarea = e.currentTarget;
        const { selectionStart, selectionEnd } = textarea;
        const newValue = sql.substring(0, selectionStart) + "  " + sql.substring(selectionEnd);
        setSql(newValue);
        // Restore cursor position after state update
        requestAnimationFrame(() => {
          textarea.selectionStart = textarea.selectionEnd = selectionStart + 2;
        });
      }
    },
    [execute, sql]
  );

  return (
    <div className="flex flex-col h-full">
      {/* SQL editor area */}
      <div className="flex flex-col border-b border-border shrink-0">
        {/* Toolbar */}
        <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/30">
          <Button
            variant="default"
            size="sm"
            className="h-7 text-xs gap-1"
            onClick={execute}
            disabled={loading || !sql.trim()}
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            {loading ? t("query.executing") : t("query.execute")}
          </Button>
          <Select value={selectedDb} onValueChange={setSelectedDb}>
            <SelectTrigger size="sm" className="h-7 w-[160px] text-xs">
              <SelectValue placeholder={t("query.databases")} />
            </SelectTrigger>
            <SelectContent>
              {databases.map((db) => (
                <SelectItem key={db} value={db} className="text-xs">
                  {db}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {sqlHistory.length > 0 && (
            <Popover open={showHistory} onOpenChange={setShowHistory}>
              <PopoverTrigger asChild>
                <Button variant="ghost" size="sm" className="h-7 text-xs gap-1">
                  <History className="h-3.5 w-3.5" />
                  {t("query.history")}
                </Button>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-[400px] max-h-[300px] overflow-auto p-1">
                {sqlHistory.map((item, idx) => (
                  <button
                    key={idx}
                    className="w-full text-left px-2 py-1.5 text-xs font-mono rounded hover:bg-accent truncate block"
                    onClick={() => {
                      setSql(item);
                      setShowHistory(false);
                    }}
                    title={item}
                  >
                    {item.length > 80 ? item.substring(0, 80) + "..." : item}
                  </button>
                ))}
              </PopoverContent>
            </Popover>
          )}
        </div>
        {/* Textarea */}
        <textarea
          ref={textareaRef}
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t("query.sqlPlaceholder")}
          className="w-full min-h-[120px] max-h-[300px] resize-y bg-background px-3 py-2 text-xs font-mono outline-none placeholder:text-muted-foreground/60"
          spellCheck={false}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
        />
      </div>

      {/* Result area */}
      <div className="flex-1 min-h-0 flex flex-col">
        {affectedRows !== null && !error && (
          <div className="px-3 py-2 text-xs text-muted-foreground">
            {t("query.affectedRows")}: {affectedRows}
          </div>
        )}
        {columns.length > 0 && !loading && !error && (
          <div className="px-3 py-1.5 text-xs text-muted-foreground border-b border-border bg-muted/30">
            {t("query.rows", { count: rows.length })}
          </div>
        )}
        <QueryResultTable columns={columns} rows={rows} loading={loading} error={error ?? undefined} showRowNumber />
      </div>

      {/* Dangerous SQL confirmation */}
      <ConfirmDialog
        open={showDangerConfirm}
        onOpenChange={setShowDangerConfirm}
        title={t("query.dangerousSqlTitle")}
        description={t("query.dangerousSqlDesc")}
        cancelText={t("action.cancel")}
        confirmText={t("query.execute")}
        onConfirm={doExecute}
      />
    </div>
  );
}
