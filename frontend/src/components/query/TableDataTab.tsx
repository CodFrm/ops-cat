import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, Save, Undo2, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteSQL } from "../../../wailsjs/go/main/App";
import { QueryResultTable, CellEdit } from "./QueryResultTable";
import { toast } from "sonner";

interface TableDataTabProps {
  tabId: string;
  database: string;
  table: string;
}

const PAGE_SIZE = 100;

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  affected_rows?: number;
}

// Escape value for SQL — basic quoting
function sqlQuote(value: unknown): string {
  if (value == null) return "NULL";
  const s = String(value);
  // Escape single quotes
  const escaped = s.replace(/'/g, "''");
  return `'${escaped}'`;
}

function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name}"`;
  return `\`${name}\``;
}

export function TableDataTab({ tabId, database, table }: TableDataTabProps) {
  const { t } = useTranslation();
  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const queryMeta = tab?.meta as QueryTabMeta | undefined;

  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  const [totalRows, setTotalRows] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [edits, setEdits] = useState<Map<string, unknown>>(new Map());
  const [submitting, setSubmitting] = useState(false);

  const driver = queryMeta?.driver;
  const assetId = queryMeta?.assetId ?? 0;

  const fetchData = useCallback(
    async (pageNum: number) => {
      if (!assetId) return;
      setLoading(true);
      setError(null);

      const offset = pageNum * PAGE_SIZE;
      let sql: string;

      if (driver === "postgresql") {
        sql = `SELECT * FROM "${table}" LIMIT ${PAGE_SIZE} OFFSET ${offset}`;
      } else {
        sql = `SELECT * FROM ${quoteIdent(database, driver)}.${quoteIdent(table, driver)} LIMIT ${PAGE_SIZE} OFFSET ${offset}`;
      }

      try {
        const result = await ExecuteSQL(assetId, sql, database);
        const parsed: SQLResult = JSON.parse(result);
        setColumns(parsed.columns || []);
        setRows(parsed.rows || []);
        setTotalRows(parsed.count ?? (parsed.rows || []).length);
      } catch (e) {
        setError(String(e));
        setColumns([]);
        setRows([]);
      } finally {
        setLoading(false);
      }
    },
    [assetId, database, table, driver]
  );

  useEffect(() => {
    fetchData(page);
  }, [fetchData, page]);

  // Clear edits when page changes
  useEffect(() => {
    setEdits(new Map());
  }, [page]);

  const handleCellEdit = useCallback((edit: CellEdit) => {
    setEdits((prev) => {
      const next = new Map(prev);
      const key = `${edit.rowIdx}:${edit.col}`;
      next.set(key, edit.value);
      return next;
    });
  }, []);

  const handleDiscard = useCallback(() => {
    setEdits(new Map());
  }, []);

  const handleSubmit = useCallback(async () => {
    if (edits.size === 0 || !assetId) return;

    // Group edits by row
    const rowEdits = new Map<number, Map<string, unknown>>();
    for (const [key, value] of edits) {
      const [rowIdxStr, col] = [key.substring(0, key.indexOf(":")), key.substring(key.indexOf(":") + 1)];
      const rowIdx = Number(rowIdxStr);
      if (!rowEdits.has(rowIdx)) rowEdits.set(rowIdx, new Map());
      rowEdits.get(rowIdx)!.set(col, value);
    }

    setSubmitting(true);
    let successCount = 0;
    let errorMsg = "";

    for (const [rowIdx, colEdits] of rowEdits) {
      const row = rows[rowIdx];
      if (!row) continue;

      // Build SET clause
      const setClauses: string[] = [];
      for (const [col, value] of colEdits) {
        setClauses.push(`${quoteIdent(col, driver)} = ${sqlQuote(value)}`);
      }

      // Build WHERE clause using ALL original column values to identify the row
      const whereClauses: string[] = [];
      for (const col of columns) {
        const origVal = row[col];
        if (origVal == null) {
          whereClauses.push(`${quoteIdent(col, driver)} IS NULL`);
        } else {
          whereClauses.push(`${quoteIdent(col, driver)} = ${sqlQuote(origVal)}`);
        }
      }

      const tableName = driver === "postgresql"
        ? `"${table}"`
        : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;

      const updateSQL = `UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ${whereClauses.join(" AND ")} LIMIT 1`;

      try {
        await ExecuteSQL(assetId, updateSQL, database);
        successCount++;
      } catch (e) {
        errorMsg += String(e) + "\n";
      }
    }

    setSubmitting(false);

    if (errorMsg) {
      toast.error(errorMsg.trim());
    }
    if (successCount > 0) {
      toast.success(t("query.updateSuccess", { count: successCount }));
      setEdits(new Map());
      // Refresh data
      fetchData(page);
    }
  }, [edits, rows, columns, assetId, database, table, driver, page, fetchData, t]);

  const hasNext = rows.length === PAGE_SIZE;
  const hasPrev = page > 0;
  const hasEdits = edits.size > 0;

  return (
    <div className="flex flex-col h-full">
      {/* Header bar */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/30 shrink-0">
        <span className="text-xs font-mono font-semibold bg-muted px-1.5 py-0.5 rounded border border-border">
          {database}.{table}
        </span>
        {!loading && !error && (
          <span className="text-xs text-muted-foreground">
            {t("query.rows", { count: totalRows })}
          </span>
        )}
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            disabled={!hasPrev || loading}
            onClick={() => setPage((p) => p - 1)}
            title={t("query.prevPage")}
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </Button>
          <span className="text-xs text-muted-foreground min-w-[60px] text-center">
            {t("query.page", { page: page + 1 })}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            disabled={!hasNext || loading}
            onClick={() => setPage((p) => p + 1)}
            title={t("query.nextPage")}
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Table content */}
      <QueryResultTable
        columns={columns}
        rows={rows}
        loading={loading}
        error={error ?? undefined}
        editable
        edits={edits}
        onCellEdit={handleCellEdit}
      />

      {/* Edit action bar */}
      {hasEdits && (
        <div className="flex items-center gap-2 px-3 py-2 border-t border-border bg-muted/50 shrink-0">
          <span className="text-xs text-muted-foreground">
            {t("query.pendingEdits", { count: edits.size })}
          </span>
          <div className="ml-auto flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={handleDiscard}
              disabled={submitting}
            >
              <Undo2 className="h-3.5 w-3.5" />
              {t("query.discardEdits")}
            </Button>
            <Button
              variant="default"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={handleSubmit}
              disabled={submitting}
            >
              {submitting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              {t("query.submitEdits")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
