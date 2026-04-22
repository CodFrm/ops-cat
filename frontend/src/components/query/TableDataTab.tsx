import { memo, useEffect, useState, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  ChevronLeft,
  ChevronRight,
  Save,
  Undo2,
  Loader2,
  RefreshCw,
  ChevronsLeft,
  ChevronsRight,
  Filter,
  FileCode2,
  Copy,
  Eye,
  TriangleAlert,
  Download,
} from "lucide-react";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  Input,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { useQueryStore } from "@/stores/queryStore";
import { isMac } from "@/stores/shortcutStore";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { QueryResultTable, CellEdit, SortDir } from "./QueryResultTable";
import { SqlPreviewDialog } from "./SqlPreviewDialog";
import { toast } from "sonner";

interface TableDataTabProps {
  tabId: string;
  innerTabId: string;
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

const REFRESH_SHORTCUT_LABEL = isMac ? "⌘R" : "Ctrl+R";

// 字符串 props 稳定 —— memo 避免 DatabasePanel 的 innerTabs 结构变化传导
export const TableDataTab = memo(function TableDataTab(props: TableDataTabProps) {
  const { t } = useTranslation();
  const { markTableTabLoaded } = useQueryStore();
  const innerTab = useQueryStore((s) => s.dbStates[props.tabId]?.innerTabs.find((it) => it.id === props.innerTabId));
  const pendingLoad = innerTab?.type === "table" && innerTab.pendingLoad === true;
  const isOuterActive = useTabStore((s) => s.activeTabId === props.tabId);
  const isInnerActive = useQueryStore((s) => s.dbStates[props.tabId]?.activeInnerTabId === props.innerTabId);

  useEffect(() => {
    if (!pendingLoad || !isOuterActive || !isInnerActive) return;
    const handler = (e: KeyboardEvent) => {
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (mod && e.code === "KeyR" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        markTableTabLoaded(props.tabId, props.innerTabId);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [pendingLoad, isOuterActive, isInnerActive, markTableTabLoaded, props.tabId, props.innerTabId]);

  if (pendingLoad) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-muted-foreground">
        <p className="text-xs">
          {t("query.tableRestoredHint", { table: props.table, shortcut: REFRESH_SHORTCUT_LABEL })}
        </p>
        <Button
          variant="outline"
          size="sm"
          className="h-8 text-xs gap-1"
          onClick={() => markTableTabLoaded(props.tabId, props.innerTabId)}
        >
          <Download className="h-3.5 w-3.5" />
          {t("query.loadData")}
        </Button>
      </div>
    );
  }

  return <TableDataTabContent {...props} />;
});

function TableDataTabContent({ tabId, innerTabId, database, table }: TableDataTabProps) {
  const { t } = useTranslation();
  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const queryMeta = tab?.meta as QueryTabMeta | undefined;
  const isOuterActive = useTabStore((s) => s.activeTabId === tabId);
  const isInnerActive = useQueryStore((s) => s.dbStates[tabId]?.activeInnerTabId === innerTabId);

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
  // `preview` = read-only view (opened by the "Preview SQL" button).
  // `confirm` = confirmation before submit (opened by the "Submit" button).
  const [dialogMode, setDialogMode] = useState<"preview" | "confirm" | null>(null);
  const [showDDLDialog, setShowDDLDialog] = useState(false);
  const [ddlLoading, setDdlLoading] = useState(false);
  const [ddlSQL, setDdlSQL] = useState("");
  const [whereInput, setWhereInput] = useState("");
  const [orderByInput, setOrderByInput] = useState("");
  const [whereClause, setWhereClause] = useState("");
  const [orderByClause, setOrderByClause] = useState("");
  const [sortColumn, setSortColumn] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>(null);
  const [applyVersion, setApplyVersion] = useState(0);
  const [primaryKeys, setPrimaryKeys] = useState<string[]>([]);
  const [pkLoaded, setPkLoaded] = useState(false);

  const driver = queryMeta?.driver;
  const assetId = queryMeta?.assetId ?? 0;

  const totalPages = totalRows != null ? Math.max(1, Math.ceil(totalRows / pageSize)) : null;

  // Fetch primary key column names for the current table. Used to build a
  // concise UPDATE WHERE clause instead of matching every column.
  const fetchPrimaryKeys = useCallback(async () => {
    if (!assetId) return;
    try {
      let sql: string;
      if (driver === "postgresql") {
        const escapedTable = table.replace(/'/g, "''");
        sql = `SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema WHERE tc.table_schema = 'public' AND tc.table_name = '${escapedTable}' AND tc.constraint_type = 'PRIMARY KEY' ORDER BY kcu.ordinal_position`;
      } else {
        sql = `SHOW KEYS FROM ${quoteIdent(database, driver)}.${quoteIdent(table, driver)} WHERE Key_name = 'PRIMARY'`;
      }
      const result = await ExecuteSQL(assetId, sql, database);
      const parsed: SQLResult = JSON.parse(result);
      const cols = (parsed.rows ?? []).map((r) => String(r["Column_name"] ?? r["column_name"] ?? "")).filter(Boolean);
      setPrimaryKeys(cols);
    } catch {
      setPrimaryKeys([]);
    } finally {
      setPkLoaded(true);
    }
  }, [assetId, database, table, driver]);

  useEffect(() => {
    setPrimaryKeys([]);
    setPkLoaded(false);
    fetchPrimaryKeys();
  }, [fetchPrimaryKeys]);

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
      // Header-click sort takes precedence over the manual ORDER BY input.
      const orderBy =
        sortColumn && sortDir
          ? `${quoteIdent(sortColumn, driver)} ${sortDir === "asc" ? "ASC" : "DESC"}`
          : orderByClause.trim();
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
    [assetId, database, table, driver, pageSize, whereClause, orderByClause, sortColumn, sortDir]
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

      // 优先用主键定位：WHERE 短、避免 TEXT/BLOB 模糊匹配，也能处理 PG 浮点等列等值不稳的情况。
      // 没主键时退回全列匹配；PG 还要用 ctid 包一层把"匹配到的多行"收敛为物理一行。
      const hasPK = primaryKeys.length > 0;
      const whereCols = hasPK ? primaryKeys : columns;
      const whereClauses: string[] = [];
      for (const col of whereCols) {
        const origVal = row[col];
        if (origVal == null) {
          whereClauses.push(`${quoteIdent(col, driver)} IS NULL`);
        } else {
          whereClauses.push(`${quoteIdent(col, driver)} = ${sqlQuote(origVal)}`);
        }
      }

      const tableName =
        driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
      const whereSQL = whereClauses.join(" AND ");

      if (driver === "postgresql") {
        if (hasPK) {
          statements.push(`UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ${whereSQL};`);
        } else {
          statements.push(
            `UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ctid = (SELECT ctid FROM ${tableName} WHERE ${whereSQL} LIMIT 1);`
          );
        }
      } else {
        statements.push(`UPDATE ${tableName} SET ${setClauses.join(", ")} WHERE ${whereSQL} LIMIT 1;`);
      }
    }
    return statements;
  }, [edits, rows, columns, driver, database, table, primaryKeys]);

  const previewStatements = useMemo(() => {
    if (dialogMode === null) return [];
    return buildUpdateStatements();
  }, [dialogMode, buildUpdateStatements]);

  const handleSubmit = useCallback(async () => {
    if (edits.size === 0 || !assetId) return;

    const statements = buildUpdateStatements();
    setSubmitting(true);
    let affectedTotal = 0;
    let zeroAffected = 0;
    let errorMsg = "";

    for (const sql of statements) {
      try {
        const result = await ExecuteSQL(assetId, sql, database);
        const parsed: SQLResult = JSON.parse(result);
        const affected = Number(parsed.affected_rows ?? 0);
        if (affected > 0) affectedTotal += affected;
        else zeroAffected++;
      } catch (e) {
        errorMsg += String(e) + "\n";
      }
    }

    setSubmitting(false);
    setDialogMode(null);

    if (affectedTotal > 0) {
      toast.success(t("query.updateSuccessAffected", { affected: affectedTotal }));
      setEdits(new Map());
      fetchData(page);
      fetchCount();
    }
    if (zeroAffected > 0) {
      toast.warning(t("query.updateMismatch", { count: zeroAffected }));
    }
    if (errorMsg) {
      toast.error(errorMsg.trim());
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

  useEffect(() => {
    if (!isOuterActive || !isInnerActive) return;
    const handler = (e: KeyboardEvent) => {
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (mod && e.code === "KeyR" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        handleRefresh();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [isOuterActive, isInnerActive, handleRefresh]);

  const handleApplyQuery = useCallback(() => {
    setWhereClause(whereInput.trim());
    setOrderByClause(orderByInput.trim());
    // Manual ORDER BY input overrides the header-click sort state.
    if (orderByInput.trim()) {
      setSortColumn(null);
      setSortDir(null);
    }
    setPage(0);
    setEdits(new Map());
    setApplyVersion((v) => v + 1);
  }, [whereInput, orderByInput]);

  const handleSortChange = useCallback((col: string | null, dir: SortDir) => {
    setSortColumn(col);
    setSortDir(dir);
    // Header click takes precedence — clear the manual ORDER BY input so the user
    // can see which sort is actually applied.
    setOrderByInput("");
    setOrderByClause("");
    setPage(0);
    setEdits(new Map());
    setApplyVersion((v) => v + 1);
  }, []);

  const handleViewDDL = useCallback(async () => {
    if (!assetId) return;
    setShowDDLDialog(true);
    setDdlLoading(true);

    try {
      let ddl = "";

      if (driver === "postgresql") {
        const escapedTable = table.replace(/'/g, "''");
        const columnsSql = `SELECT column_name, data_type, udt_name, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${escapedTable}' ORDER BY ordinal_position`;
        const primaryKeySql = `SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema WHERE tc.table_schema = 'public' AND tc.table_name = '${escapedTable}' AND tc.constraint_type = 'PRIMARY KEY' ORDER BY kcu.ordinal_position`;

        const columnsResult = await ExecuteSQL(assetId, columnsSql, database);
        const primaryKeyResult = await ExecuteSQL(assetId, primaryKeySql, database);
        const columnsParsed: SQLResult = JSON.parse(columnsResult);
        const primaryKeyParsed: SQLResult = JSON.parse(primaryKeyResult);

        const columns = columnsParsed.rows || [];
        const primaryKeyColumns = (primaryKeyParsed.rows || [])
          .map((r) => String(Object.values(r)[0] ?? ""))
          .filter(Boolean);

        if (columns.length > 0) {
          const defs = columns.map((col) => {
            const name = String(col.column_name ?? "");
            const dataType = String(col.data_type ?? "");
            const udtName = String(col.udt_name ?? "");
            const columnDefault = col.column_default == null ? "" : String(col.column_default);
            const type = dataType === "USER-DEFINED" && udtName ? udtName : dataType;
            const nullable = String(col.is_nullable ?? "").toUpperCase() === "YES";

            let line = `"${name}" ${type}`;
            if (!nullable) line += " NOT NULL";
            if (columnDefault) line += ` DEFAULT ${columnDefault}`;
            return line;
          });

          if (primaryKeyColumns.length > 0) {
            defs.push(`PRIMARY KEY (${primaryKeyColumns.map((c) => `"${c}"`).join(", ")})`);
          }

          ddl = `CREATE TABLE "public"."${table}" (\n  ${defs.join(",\n  ")}\n);`;
        }
      } else {
        const quotedTable = quoteIdent(table, driver);
        const result = await ExecuteSQL(assetId, `SHOW CREATE TABLE ${quotedTable}`, database);
        const parsed: SQLResult = JSON.parse(result);
        const row = parsed.rows?.[0];
        if (row) {
          const values = Object.values(row);
          const createSQL = values.find((v) => typeof v === "string" && /CREATE\s+(TABLE|VIEW)/i.test(String(v)));
          ddl = String(createSQL ?? values[1] ?? values[0] ?? "");
        }
      }

      setDdlSQL(ddl || t("query.ddlEmpty"));
    } catch (e) {
      setDdlSQL(String(e));
    } finally {
      setDdlLoading(false);
    }
  }, [assetId, driver, table, database, t]);

  const handleCopyDDL = useCallback(async () => {
    const text = ddlLoading ? "" : ddlSQL;
    if (!text) return;
    await navigator.clipboard.writeText(text);
    toast.success(t("query.copied"));
  }, [ddlLoading, ddlSQL, t]);

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
        <Button variant="outline" size="sm" className="h-7 text-xs gap-1 shrink-0" onClick={handleViewDDL}>
          <FileCode2 className="h-3.5 w-3.5" />
          {t("query.viewDDL")}
        </Button>
        {driver !== "postgresql" && pkLoaded && primaryKeys.length === 0 && columns.length > 0 && (
          <span
            className="flex items-center gap-1 text-[11px] text-amber-600 shrink-0"
            title={t("query.noPrimaryKeyTooltip")}
          >
            <TriangleAlert className="h-3.5 w-3.5" />
            {t("query.noPrimaryKey")}
          </span>
        )}
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
        sortColumn={sortColumn}
        sortDir={sortDir}
        onSortChange={handleSortChange}
        enableColumnFilter
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
              variant="outline"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={() => setDialogMode("preview")}
              disabled={submitting}
            >
              <Eye className="h-3.5 w-3.5" />
              {t("query.previewSql")}
            </Button>
            <Button
              variant="default"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={() => setDialogMode("confirm")}
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
          title={`${t("query.refreshTable")} (${REFRESH_SHORTCUT_LABEL})`}
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

      {/* DDL dialog */}
      <AlertDialog open={showDDLDialog} onOpenChange={setShowDDLDialog}>
        <AlertDialogContent className="max-w-3xl" onOverlayClick={() => setShowDDLDialog(false)}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("query.ddlDialogTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("query.ddlDialogDesc", { table })}</AlertDialogDescription>
          </AlertDialogHeader>
          <ScrollArea className="max-h-[420px]">
            <pre className="bg-muted rounded-md p-3 text-xs font-mono whitespace-pre-wrap break-all border border-border">
              {ddlLoading ? t("query.loadingDDL") : ddlSQL}
            </pre>
          </ScrollArea>
          <AlertDialogFooter>
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={handleCopyDDL}
              disabled={ddlLoading || !ddlSQL}
            >
              <Copy className="h-3.5 w-3.5" />
              {t("action.copy")}
            </Button>
            <AlertDialogCancel size="sm" className="h-7 text-xs px-3">
              {t("action.close")}
            </AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* SQL preview / submit confirmation */}
      <SqlPreviewDialog
        open={dialogMode !== null}
        onOpenChange={(open) => {
          if (!open && !submitting) setDialogMode(null);
        }}
        statements={previewStatements}
        onConfirm={dialogMode === "confirm" ? handleSubmit : undefined}
        submitting={submitting}
      />
    </div>
  );
}
