import { useEffect, useState, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, Save, Undo2, Loader2, RefreshCw, ChevronsLeft, ChevronsRight, Filter } from "lucide-react";
import { Button, Input, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@opskat/ui";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { QueryResultTable, CellEdit } from "./QueryResultTable";
import { SqlPreviewDialog } from "./SqlPreviewDialog";
import { toast } from "sonner";

interface TableDataTabProps {
  tabId: string;
  database: string;
  table: string;
}

const PAGE_SIZES = [50, 100, 200, 500];
const DEFAULT_PAGE_SIZE = 100;

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
  const [totalRows, setTotalRows] = useState<number | null>(null);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [pageInput, setPageInput] = useState("1");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [edits, setEdits] = useState<Map<string, unknown>>(new Map());
  const [submitting, setSubmitting] = useState(false);
  const [showSqlPreview, setShowSqlPreview] = useState(false);
  const [whereInput, setWhereInput] = useState("");
  const [orderByInput, setOrderByInput] = useState("");
  const [whereClause, setWhereClause] = useState("");
  const [orderByClause, setOrderByClause] = useState("");
  const [applyVersion, setApplyVersion] = useState(0);

  const driver = queryMeta?.driver;
  const assetId = queryMeta?.assetId ?? 0;

  const totalPages = totalRows != null ? Math.max(1, Math.ceil(totalRows / pageSize)) : null;

  // Fetch total count
  const fetchCount = useCallback(async () => {
    if (!assetId) return;
    const tableName =
      driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
    const where = whereClause.trim();
    const wherePart = where ? ` WHERE ${where}` : "";
    try {
      const result = await ExecuteSQL(assetId, `SELECT COUNT(*) AS cnt FROM ${tableName}${wherePart}`, database);
      const parsed: SQLResult = JSON.parse(result);
      const row = parsed.rows?.[0];
      if (row) {
        const cnt = Number(Object.values(row)[0]);
        if (!isNaN(cnt)) setTotalRows(cnt);
      }
    } catch {
      setTotalRows(null);
    }
  }, [assetId, database, table, driver, whereClause]);

  const fetchData = useCallback(
    async (pageNum: number) => {
      if (!assetId) return;
      setLoading(true);
      setError(null);

      const offset = pageNum * pageSize;
      const tableName =
        driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
      const where = whereClause.trim();
      const orderBy = orderByClause.trim();
      const wherePart = where ? ` WHERE ${where}` : "";
      const orderByPart = orderBy ? ` ORDER BY ${orderBy}` : "";
      const sql = `SELECT * FROM ${tableName}${wherePart}${orderByPart} LIMIT ${pageSize} OFFSET ${offset}`;

      try {
        const result = await ExecuteSQL(assetId, sql, database);
        const parsed: SQLResult = JSON.parse(result);
        setColumns(parsed.columns || []);
        setRows(parsed.rows || []);
      } catch (e) {
        setError(String(e));
        setColumns([]);
        setRows([]);
      } finally {
        setLoading(false);
      }
    },
    [assetId, database, table, driver, pageSize, whereClause, orderByClause]
  );

  useEffect(() => {
    fetchCount();
  }, [fetchCount, applyVersion]);

  useEffect(() => {
    fetchData(page);
  }, [fetchData, page, applyVersion]);

  // Sync page input
  useEffect(() => {
    setPageInput(String(page + 1));
  }, [page]);

  // Clear edits when page changes
  useEffect(() => {
    setEdits(new Map());
  }, [page, pageSize]);

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

  // Build SQL statements for preview
  const buildUpdateStatements = useCallback((): string[] => {
    if (edits.size === 0) return [];

    const rowEdits = new Map<number, Map<string, unknown>>();
    for (const [key, value] of edits) {
      const [rowIdxStr, col] = [key.substring(0, key.indexOf(":")), key.substring(key.indexOf(":") + 1)];
      const rowIdx = Number(rowIdxStr);
      if (!rowEdits.has(rowIdx)) rowEdits.set(rowIdx, new Map());
      rowEdits.get(rowIdx)!.set(col, value);
    }

    const statements: string[] = [];
    for (const [rowIdx, colEdits] of rowEdits) {
      const row = rows[rowIdx];
      if (!row) continue;

      const setClauses: string[] = [];
      for (const [col, value] of colEdits) {
        setClauses.push(`${quoteIdent(col, driver)} = ${sqlQuote(value)}`);
      }

      const whereClauses: string[] = [];
      for (const col of columns) {
        const origVal = row[col];
        if (origVal == null) {
          whereClauses.push(`${quoteIdent(col, driver)} IS NULL`);
        } else {
          whereClauses.push(`${quoteIdent(col, driver)} = ${sqlQuote(origVal)}`);
        }
      }

      const tableName =
        driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;

      if (driver === "postgresql") {
        statements.push(
          `UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ctid = (SELECT ctid FROM ${tableName} WHERE ${whereClauses.join(" AND ")} LIMIT 1);`
        );
      } else {
        statements.push(
          `UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ${whereClauses.join(" AND ")} LIMIT 1;`
        );
      }
    }
    return statements;
  }, [edits, rows, columns, driver, database, table]);

  const previewStatements = useMemo(() => {
    if (!showSqlPreview) return [];
    return buildUpdateStatements();
  }, [showSqlPreview, buildUpdateStatements]);

  const handleSubmit = useCallback(async () => {
    if (edits.size === 0 || !assetId) return;

    const statements = buildUpdateStatements();
    setSubmitting(true);
    let successCount = 0;
    let errorMsg = "";

    for (const sql of statements) {
      try {
        await ExecuteSQL(assetId, sql, database);
        successCount++;
      } catch (e) {
        errorMsg += String(e) + "\n";
      }
    }

    setSubmitting(false);
    setShowSqlPreview(false);

    if (errorMsg) {
      toast.error(errorMsg.trim());
    }
    if (successCount > 0) {
      toast.success(t("query.updateSuccess", { count: successCount }));
      setEdits(new Map());
      fetchData(page);
      fetchCount();
    }
  }, [edits, assetId, database, buildUpdateStatements, page, fetchData, fetchCount, t]);

  const handlePageInputConfirm = useCallback(() => {
    const num = parseInt(pageInput, 10);
    if (isNaN(num) || num < 1) {
      setPageInput(String(page + 1));
      return;
    }
    const target = totalPages ? Math.min(num, totalPages) - 1 : num - 1;
    setPage(target);
  }, [pageInput, page, totalPages]);

  const handleRefresh = useCallback(() => {
    fetchData(page);
    fetchCount();
  }, [fetchData, fetchCount, page]);

  const handleApplyQuery = useCallback(() => {
    setWhereClause(whereInput.trim());
    setOrderByClause(orderByInput.trim());
    setPage(0);
    setEdits(new Map());
    setApplyVersion((v) => v + 1);
  }, [whereInput, orderByInput]);

  const hasNext = totalPages != null ? page < totalPages - 1 : rows.length === pageSize;
  const hasPrev = page > 0;
  const hasEdits = edits.size > 0;

  return (
    <div className="flex flex-col h-full">
      {/* Filter bar */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/20 shrink-0">
        <div className="flex items-center gap-1 flex-1 min-w-0">
          <span className="text-[11px] font-mono text-muted-foreground">WHERE</span>
          <Input
            className="h-7 text-xs font-mono"
            value={whereInput}
            onChange={(e) => setWhereInput(e.target.value)}
            placeholder={t("query.wherePlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                handleApplyQuery();
              }
            }}
          />
        </div>
        <div className="flex items-center gap-1 flex-1 min-w-0">
          <span className="text-[11px] font-mono text-muted-foreground whitespace-nowrap">ORDER BY</span>
          <Input
            className="h-7 text-xs font-mono"
            value={orderByInput}
            onChange={(e) => setOrderByInput(e.target.value)}
            placeholder={t("query.orderByPlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                handleApplyQuery();
              }
            }}
          />
        </div>
        <Button variant="outline" size="sm" className="h-7 text-xs gap-1 shrink-0" onClick={handleApplyQuery}>
          <Filter className="h-3.5 w-3.5" />
          {t("query.applyFilter")}
        </Button>
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
        showRowNumber
        rowNumberOffset={page * pageSize}
      />

      {/* Edit action bar */}
      {hasEdits && (
        <div className="flex items-center gap-2 px-3 py-2 border-t border-border bg-muted/50 shrink-0">
          <span className="text-xs text-muted-foreground">{t("query.pendingEdits", { count: edits.size })}</span>
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
              onClick={() => setShowSqlPreview(true)}
              disabled={submitting}
            >
              {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
              {t("query.submitEdits")}
            </Button>
          </div>
        </div>
      )}

      {/* Footer bar */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-t border-border bg-muted/30 shrink-0">
        {totalRows != null && (
          <span className="text-xs text-muted-foreground">{t("query.totalRows", { count: totalRows })}</span>
        )}
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={handleRefresh}
          disabled={loading}
          title={t("query.refreshTable")}
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </Button>
        <div className="ml-auto flex items-center gap-1">
          {/* Page size selector */}
          <Select
            value={String(pageSize)}
            onValueChange={(v) => {
              setPageSize(Number(v));
              setPage(0);
            }}
          >
            <SelectTrigger size="sm" className="h-6 w-[80px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PAGE_SIZES.map((s) => (
                <SelectItem key={s} value={String(s)} className="text-xs">
                  {t("query.perPage", { count: s })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* First page */}
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            disabled={!hasPrev || loading}
            onClick={() => setPage(0)}
            title={t("query.firstPage")}
          >
            <ChevronsLeft className="h-3.5 w-3.5" />
          </Button>
          {/* Previous page */}
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
          {/* Page input */}
          <Input
            className="h-6 w-[48px] text-xs text-center px-1"
            value={pageInput}
            onChange={(e) => setPageInput(e.target.value)}
            onBlur={handlePageInputConfirm}
            onKeyDown={(e) => {
              if (e.key === "Enter") handlePageInputConfirm();
            }}
          />
          {totalPages != null && (
            <span className="text-xs text-muted-foreground whitespace-nowrap">/ {totalPages}</span>
          )}
          {/* Next page */}
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
          {/* Last page */}
          {totalPages != null && (
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              disabled={!hasNext || loading}
              onClick={() => setPage(totalPages - 1)}
              title={t("query.lastPage")}
            >
              <ChevronsRight className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
      </div>

      {/* SQL Preview confirmation dialog */}
      <SqlPreviewDialog
        open={showSqlPreview}
        onOpenChange={setShowSqlPreview}
        statements={previewStatements}
        onConfirm={handleSubmit}
        submitting={submitting}
      />
    </div>
  );
}
