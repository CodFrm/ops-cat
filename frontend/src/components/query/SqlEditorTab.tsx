import { useState, useCallback, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Play, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteSQL } from "../../../wailsjs/go/main/App";
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
  const databases = dbState?.databases || [];

  // Restore persisted state from store
  const innerTab = dbState?.innerTabs.find((t) => t.id === innerTabId);
  const persistedSql = innerTab?.type === "sql" ? innerTab.sql : undefined;
  const persistedDb = innerTab?.type === "sql" ? innerTab.selectedDb : undefined;

  const [sql, setSql] = useState(persistedSql || "");
  const [selectedDb, setSelectedDb] = useState(
    persistedDb || queryMeta?.defaultDatabase || ""
  );
  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  const [affectedRows, setAffectedRows] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const execute = useCallback(async () => {
    const trimmed = sql.trim();
    if (!trimmed || !assetId) return;

    setLoading(true);
    setError(null);
    setAffectedRows(null);
    setColumns([]);
    setRows([]);

    try {
      const result = await ExecuteSQL(assetId, trimmed, selectedDb);
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
  }, [sql, assetId, selectedDb]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        e.preventDefault();
        execute();
      }
    },
    [execute]
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
            {loading ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="h-3.5 w-3.5" />
            )}
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
        <QueryResultTable
          columns={columns}
          rows={rows}
          loading={loading}
          error={error ?? undefined}
        />
      </div>
    </div>
  );
}
