import { memo, useEffect, useState, useCallback, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { FileCode2, Copy, TriangleAlert, Download } from "lucide-react";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  ScrollArea,
} from "@opskat/ui";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { useQueryStore } from "@/stores/queryStore";
import { isMac, formatModKey } from "@/stores/shortcutStore";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import {
  QueryResultTable,
  CellEdit,
  SortDir,
  type CopyAsFormat,
  type FocusCellRequest,
  type RowDensity,
} from "./QueryResultTable";
import { SqlPreviewDialog } from "./SqlPreviewDialog";
import { ImportTableDataDialog } from "./ImportTableDataDialog";
import { ExportTableDataDialog } from "./ExportTableDataDialog";
import { TableFilterBuilder } from "./TableFilterBuilder";
import { TableDataStatusBar, TableEditorToolbar, type TableExportFormat } from "./TableEditorToolbar";
import { toast } from "sonner";
import { toInsertSql, toTsv, toTsvData, toTsvFields, toUpdateSql } from "@/lib/tableExport";
import { buildInsertStatement, validateInsertRow, type TableColumnRule } from "@/lib/tableEdit";
import {
  buildDeleteStatement,
  buildFilterByCellValueClause,
  quoteIdent,
  quoteTableRef,
  sqlQuote,
  type CellValueFilterOperator,
} from "@/lib/tableSql";
import {
  buildFilterWhereClause,
  buildSortOrderByClause,
  createFilterCondition,
  removeFilterItemsByColumn,
  type TableFilterItem,
  type TableSortItem,
} from "@/lib/tableFilter";
import { filterOperatorNeedsRange } from "@/lib/tableFilterOperators";
import { cellValueToText } from "@/lib/cellValue";

interface TableDataTabProps {
  tabId: string;
  innerTabId: string;
  database: string;
  table: string;
}

const DEFAULT_PAGE_SIZE = 1000;

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  affected_rows?: number;
}

const REFRESH_SHORTCUT_LABEL = formatModKey("KeyR");

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
  const [columnTypes, setColumnTypes] = useState<Record<string, string>>({});
  const [columnRules, setColumnRules] = useState<TableColumnRule[]>([]);
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  const [newRows, setNewRows] = useState<Record<string, unknown>[]>([]);
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
  const [filters, setFilters] = useState<TableFilterItem[]>([]);
  const [sorts, setSorts] = useState<TableSortItem[]>([]);
  const [showFilterSort, setShowFilterSort] = useState(false);
  const [whereClause, setWhereClause] = useState("");
  const [orderByClause, setOrderByClause] = useState("");
  const [sortColumn, setSortColumn] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>(null);
  const [applyVersion, setApplyVersion] = useState(0);
  const [primaryKeys, setPrimaryKeys] = useState<string[]>([]);
  const [pkLoaded, setPkLoaded] = useState(false);
  const [deletePreview, setDeletePreview] = useState<{
    statement: string;
    usesPrimaryKey: boolean;
  } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [selectedRowIdx, setSelectedRowIdx] = useState<number | null>(null);
  const [exportFormat, setExportFormat] = useState<TableExportFormat>("csv");
  const [showExportDialog, setShowExportDialog] = useState(false);
  const [showImportDialog, setShowImportDialog] = useState(false);
  const [importing, setImporting] = useState(false);
  const [visibleColumns, setVisibleColumns] = useState<string[]>([]);
  const [rowDensity, setRowDensity] = useState<RowDensity>("default");
  const [focusCellRequest, setFocusCellRequest] = useState<FocusCellRequest | null>(null);
  const requestSeq = useRef(0);
  const latestDataRequest = useRef(0);
  const latestCountRequest = useRef(0);
  const latestImportRequest = useRef(0);
  const cancelledRequests = useRef(new Set<number>());

  const driver = queryMeta?.driver;
  const assetId = queryMeta?.assetId ?? 0;

  const totalPages = totalRows != null ? Math.max(1, Math.ceil(totalRows / pageSize)) : null;
  const nextRequestId = useCallback(() => {
    requestSeq.current += 1;
    return requestSeq.current;
  }, []);
  const isCancelled = useCallback((requestId: number) => cancelledRequests.current.has(requestId), []);

  // Fetch primary key column names for the current table. Used to build a
  // concise UPDATE WHERE clause instead of matching every column.
  const fetchPrimaryKeys = useCallback(async () => {
    if (!assetId) return;
    try {
      let sql: string;
      if (driver === "postgresql") {
        const tableLiteral = sqlQuote(table);
        sql = `SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema WHERE tc.table_schema = 'public' AND tc.table_name = ${tableLiteral} AND tc.constraint_type = 'PRIMARY KEY' ORDER BY kcu.ordinal_position`;
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

  const fetchColumnTypes = useCallback(async () => {
    if (!assetId) return;
    try {
      let sql: string;
      if (driver === "postgresql") {
        const tableLiteral = sqlQuote(table);
        sql =
          `SELECT column_name, data_type, udt_name, is_nullable, column_default FROM information_schema.columns ` +
          `WHERE table_schema = 'public' AND table_name = ${tableLiteral} ORDER BY ordinal_position`;
      } else {
        sql = `SHOW COLUMNS FROM ${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
      }
      const result = await ExecuteSQL(assetId, sql, database);
      const parsed: SQLResult = JSON.parse(result);
      const next: Record<string, string> = {};
      const nextRules: TableColumnRule[] = [];
      for (const row of parsed.rows ?? []) {
        const name = String(row["column_name"] ?? row["Field"] ?? row["field"] ?? "");
        const type = String(row["data_type"] ?? row["Type"] ?? row["type"] ?? row["udt_name"] ?? "");
        if (name && type) next[name] = type;
        if (name) {
          const nullableRaw = String(row["is_nullable"] ?? row["Null"] ?? row["null"] ?? "").toUpperCase();
          const defaultValue = row["column_default"] ?? row["Default"] ?? row["default"];
          const extra = String(row["Extra"] ?? row["extra"] ?? "").toLowerCase();
          nextRules.push({
            name,
            nullable: nullableRaw === "YES",
            hasDefault: defaultValue != null,
            autoIncrement: extra.includes("auto_increment"),
          });
        }
      }
      setColumnTypes(next);
      setColumnRules(nextRules);
    } catch {
      setColumnTypes({});
      setColumnRules([]);
    }
  }, [assetId, database, table, driver]);

  useEffect(() => {
    setPrimaryKeys([]);
    setPkLoaded(false);
    fetchPrimaryKeys();
    setColumnTypes({});
    setColumnRules([]);
    fetchColumnTypes();
  }, [fetchPrimaryKeys, fetchColumnTypes]);

  // Fetch total count
  const fetchCount = useCallback(async () => {
    if (!assetId) return;
    const requestId = nextRequestId();
    latestCountRequest.current = requestId;
    const tableName = quoteTableRef(database, table, driver);
    const where = whereClause.trim();
    const wherePart = where ? ` WHERE ${where}` : "";
    try {
      const result = await ExecuteSQL(assetId, `SELECT COUNT(*) AS cnt FROM ${tableName}${wherePart}`, database);
      const parsed: SQLResult = JSON.parse(result);
      if (isCancelled(requestId) || latestCountRequest.current !== requestId) return;
      const row = parsed.rows?.[0];
      if (row) {
        const cnt = Number(Object.values(row)[0]);
        if (!isNaN(cnt)) setTotalRows(cnt);
      }
    } catch {
      if (isCancelled(requestId) || latestCountRequest.current !== requestId) return;
      setTotalRows(null);
    } finally {
      cancelledRequests.current.delete(requestId);
    }
  }, [assetId, database, table, driver, whereClause, nextRequestId, isCancelled]);

  const fetchData = useCallback(
    async (pageNum: number) => {
      if (!assetId) return;
      const requestId = nextRequestId();
      latestDataRequest.current = requestId;
      setLoading(true);
      setError(null);

      const offset = pageNum * pageSize;
      const tableName = quoteTableRef(database, table, driver);
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
        if (isCancelled(requestId) || latestDataRequest.current !== requestId) return;
        setColumns(parsed.columns || []);
        setRows(parsed.rows || []);
        setSelectedRowIdx(null);
      } catch (e) {
        if (isCancelled(requestId) || latestDataRequest.current !== requestId) return;
        setError(String(e));
        setColumns([]);
        setRows([]);
        setSelectedRowIdx(null);
      } finally {
        if (!isCancelled(requestId) && latestDataRequest.current === requestId) setLoading(false);
        cancelledRequests.current.delete(requestId);
      }
    },
    [
      assetId,
      database,
      table,
      driver,
      pageSize,
      whereClause,
      orderByClause,
      sortColumn,
      sortDir,
      nextRequestId,
      isCancelled,
    ]
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
    setNewRows([]);
  }, [page, pageSize]);

  useEffect(() => {
    setVisibleColumns((prev) => {
      const retained = prev.filter((col) => columns.includes(col));
      return retained.length > 0 ? retained : columns;
    });
  }, [columns]);

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
    setNewRows([]);
  }, []);

  const handleAddInlineRow = useCallback(() => {
    if (columns.length === 0) return;
    const rowIdx = rows.length + newRows.length;
    setNewRows((prev) => [...prev, {}]);
    setSelectedRowIdx(rowIdx);
    setFocusCellRequest({ rowIdx, col: columns[0], nonce: Date.now() });
  }, [columns, newRows.length, rows.length]);

  const removeNewRow = useCallback(
    (rowIdx: number) => {
      const newRowIdx = rowIdx - rows.length;
      if (newRowIdx < 0 || newRowIdx >= newRows.length) return;
      setNewRows((prev) => prev.filter((_, idx) => idx !== newRowIdx));
      setEdits((prev) => {
        const next = new Map<string, unknown>();
        for (const [key, value] of prev) {
          const sep = key.indexOf(":");
          const currentRowIdx = Number(key.substring(0, sep));
          const column = key.substring(sep + 1);
          if (currentRowIdx === rowIdx) continue;
          const shiftedRowIdx = currentRowIdx > rowIdx ? currentRowIdx - 1 : currentRowIdx;
          next.set(`${shiftedRowIdx}:${column}`, value);
        }
        return next;
      });
      setSelectedRowIdx(null);
    },
    [newRows.length, rows.length]
  );

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
      if (rowIdx >= rows.length) continue;
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

      const tableName = quoteTableRef(database, table, driver);
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

  const buildInsertStatements = useCallback((): { statements: string[]; missingFields: string[] } => {
    const statements: string[] = [];
    const missingFields = new Set<string>();

    newRows.forEach((_, newRowIdx) => {
      const rowIdx = rows.length + newRowIdx;
      const values: Record<string, unknown> = {};
      for (const column of columns) {
        const key = `${rowIdx}:${column}`;
        if (edits.has(key)) values[column] = edits.get(key);
      }

      for (const field of validateInsertRow(columnRules, values)) {
        missingFields.add(field);
      }
      statements.push(buildInsertStatement({ database, table, driver, values }));
    });

    return { statements: missingFields.size > 0 ? [] : statements, missingFields: Array.from(missingFields) };
  }, [columnRules, columns, database, driver, edits, newRows, rows.length, table]);

  const buildChangeStatements = useCallback((): { statements: string[]; missingFields: string[] } => {
    const insertResult = buildInsertStatements();
    if (insertResult.missingFields.length > 0) return insertResult;
    return { statements: [...insertResult.statements, ...buildUpdateStatements()], missingFields: [] };
  }, [buildInsertStatements, buildUpdateStatements]);

  const showInsertValidationError = useCallback(
    (fields: string[]) => {
      toast.error(t("query.insertRequiredFields", { fields: fields.join(", ") }));
    },
    [t]
  );

  const openSqlDialog = useCallback(
    (mode: "preview" | "confirm") => {
      const result = buildChangeStatements();
      if (result.missingFields.length > 0) {
        showInsertValidationError(result.missingFields);
        return;
      }
      if (result.statements.length === 0) return;
      setDialogMode(mode);
    },
    [buildChangeStatements, showInsertValidationError]
  );

  const previewStatements = useMemo(() => {
    if (dialogMode === null) return [];
    return buildChangeStatements().statements;
  }, [dialogMode, buildChangeStatements]);

  const pendingSqlSummary = useMemo(() => {
    if (edits.size === 0 && newRows.length === 0) return "";
    const result = buildChangeStatements();
    return result.missingFields.length === 0 ? (result.statements[0] ?? "") : "";
  }, [buildChangeStatements, edits.size, newRows.length]);

  const handleSubmit = useCallback(async () => {
    if ((edits.size === 0 && newRows.length === 0) || !assetId) return;

    const changeResult = buildChangeStatements();
    if (changeResult.missingFields.length > 0) {
      showInsertValidationError(changeResult.missingFields);
      return;
    }

    const statements = changeResult.statements;
    if (statements.length === 0) return;
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
      setNewRows([]);
      fetchData(page);
      fetchCount();
    }
    if (zeroAffected > 0) {
      toast.warning(t("query.updateMismatch", { count: zeroAffected }));
    }
    if (errorMsg) {
      toast.error(errorMsg.trim());
    }
  }, [
    edits.size,
    newRows.length,
    assetId,
    database,
    buildChangeStatements,
    showInsertValidationError,
    page,
    fetchData,
    fetchCount,
    t,
  ]);

  const handlePageInputConfirm = useCallback(() => {
    const num = parseInt(pageInput, 10);
    if (isNaN(num) || num < 1) {
      setPageInput(String(page + 1));
      return;
    }
    const target = totalPages ? Math.min(num, totalPages) - 1 : num - 1;
    setPage(target);
  }, [pageInput, page, totalPages]);

  const handlePageSizeChange = useCallback((nextPageSize: number) => {
    setPageSize(nextPageSize);
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
  }, []);

  const handleRefresh = useCallback(() => {
    fetchData(page);
    fetchCount();
  }, [fetchData, fetchCount, page]);

  const handleStopLoading = useCallback(() => {
    if (!loading && !importing) return;
    cancelledRequests.current.add(latestDataRequest.current);
    cancelledRequests.current.add(latestCountRequest.current);
    cancelledRequests.current.add(latestImportRequest.current);
    setLoading(false);
    setImporting(false);
    toast.info(t("query.stopLoadingToast"));
  }, [importing, loading, t]);

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
    setWhereClause(buildFilterWhereClause(filters, driver));
    const sortClause = buildSortOrderByClause(sorts, driver);
    setOrderByClause(sortClause);
    // Builder ORDER BY overrides the header-click sort state.
    if (sortClause) {
      setSortColumn(null);
      setSortDir(null);
    }
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
    setApplyVersion((v) => v + 1);
  }, [driver, filters, sorts]);

  const handleSortChange = useCallback((col: string | null, dir: SortDir) => {
    setSortColumn(col);
    setSortDir(dir);
    // Header click takes precedence — clear the builder ORDER BY state so the user
    // can see which sort is actually applied.
    setSorts([]);
    setOrderByClause("");
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
    setApplyVersion((v) => v + 1);
  }, []);

  const handleFilterByCellValue = useCallback(
    ({ col, value, operator = "=" }: { col: string; value: unknown; operator?: CellValueFilterOperator }) => {
      const filterValue = filterOperatorNeedsRange(operator) ? [value, value] : value;
      const clause = buildFilterByCellValueClause(col, filterValue, driver, operator);
      setFilters([createFilterCondition(`cell-${col}`, col, { value: filterValue, operator })]);
      setShowFilterSort(true);
      setWhereClause(clause);
      setPage(0);
      setEdits(new Map());
      setNewRows([]);
      setApplyVersion((v) => v + 1);
    },
    [driver]
  );

  const handleRemoveColumnFilter = useCallback(
    (column: string) => {
      setFilters((prev) => {
        const next = removeFilterItemsByColumn(prev, column);
        setWhereClause(buildFilterWhereClause(next, driver));
        return next;
      });
      setPage(0);
      setEdits(new Map());
      setNewRows([]);
      setApplyVersion((v) => v + 1);
    },
    [driver]
  );

  const handleRemoveAllFilters = useCallback(() => {
    setFilters([]);
    setWhereClause("");
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
    setApplyVersion((v) => v + 1);
  }, []);

  const handleSortByColumn = useCallback(
    (col: string, dir: Exclude<SortDir, null>) => {
      handleSortChange(col, dir);
    },
    [handleSortChange]
  );

  const handleClearFilterSort = useCallback(() => {
    setFilters([]);
    setWhereClause("");
    setSorts([]);
    setOrderByClause("");
    setSortColumn(null);
    setSortDir(null);
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
    setApplyVersion((v) => v + 1);
  }, []);

  const handleDeleteRow = useCallback(
    (rowIdx: number) => {
      if (rowIdx >= rows.length) {
        removeNewRow(rowIdx);
        return;
      }
      const row = rows[rowIdx];
      if (!row) return;
      const statement = buildDeleteStatement({
        database,
        table,
        columns,
        row,
        primaryKeys,
        driver,
      });
      setDeletePreview({ statement: statement.sql, usesPrimaryKey: statement.usesPrimaryKey });
    },
    [columns, database, driver, primaryKeys, removeNewRow, rows, table]
  );

  const handleConfirmDelete = useCallback(async () => {
    if (!assetId || !deletePreview) return;
    setDeleting(true);
    try {
      const result = await ExecuteSQL(assetId, deletePreview.statement, database);
      const parsed: SQLResult = JSON.parse(result);
      const affected = Number(parsed.affected_rows ?? 0);
      toast.success(t("query.deleteRecordSuccess", { affected }));
      setDeletePreview(null);
      setEdits(new Map());
      setNewRows([]);
      await fetchData(page);
      await fetchCount();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setDeleting(false);
    }
  }, [assetId, database, deletePreview, fetchCount, fetchData, page, t]);

  const handleDeleteSelectedRow = useCallback(() => {
    if (selectedRowIdx == null) return;
    handleDeleteRow(selectedRowIdx);
  }, [handleDeleteRow, selectedRowIdx]);

  const handleSelectedCellChange = useCallback((cell: { rowIdx: number } | null) => {
    setSelectedRowIdx(cell?.rowIdx ?? null);
  }, []);

  const handleExport = useCallback(() => {
    if (rows.length === 0 || columns.length === 0) return;
    setShowExportDialog(true);
  }, [columns.length, rows.length]);

  const handleCopyAs = useCallback(
    async (
      format: CopyAsFormat,
      ctx: { rowIdx: number; selectedColumns?: string[]; selectedRowIndices?: number[] }
    ) => {
      const activeColumns = ctx.selectedColumns?.length ? ctx.selectedColumns : columns;
      const activeRows = ctx.selectedRowIndices?.length
        ? ctx.selectedRowIndices.map((i) => rows[i]).filter(Boolean)
        : [rows[ctx.rowIdx]];
      if (activeRows.length === 0) return;
      const tableName = driver === "postgresql" ? table : `${database}.${table}`;
      const contentByFormat: Record<CopyAsFormat, string> = {
        insert: toInsertSql(tableName, activeColumns, activeRows, driver),
        update: toUpdateSql(tableName, activeColumns, activeRows[0], primaryKeys, driver),
        "tsv-data": toTsvData(activeColumns, activeRows),
        "tsv-fields": toTsvFields(activeColumns),
        "tsv-fields-data": toTsv(activeColumns, activeRows),
      };
      await navigator.clipboard.writeText(contentByFormat[format]);
      toast.success(t("query.copied"));
    },
    [columns, database, driver, primaryKeys, rows, table, t]
  );

  const handleImportSuccess = useCallback(async () => {
    setImporting(false);
    setPage(0);
    setEdits(new Map());
    setNewRows([]);
    setApplyVersion((v) => v + 1);
    await fetchData(0);
    await fetchCount();
  }, [fetchData, fetchCount]);

  const handleImportSubmitStart = useCallback(() => {
    const requestId = nextRequestId();
    latestImportRequest.current = requestId;
    return requestId;
  }, [nextRequestId]);

  const isImportSubmitCancelled = useCallback(
    (requestId: number) => isCancelled(requestId) || latestImportRequest.current !== requestId,
    [isCancelled]
  );

  const handleVisibleColumnToggle = useCallback(
    (column: string) => {
      setVisibleColumns((prev) => {
        const current = prev.length > 0 ? prev : columns;
        if (current.includes(column)) {
          if (current.length === 1) return current;
          return current.filter((col) => col !== column);
        }
        return columns.filter((col) => col === column || current.includes(col));
      });
    },
    [columns]
  );

  const handleHideColumn = useCallback(
    (column: string) => {
      setVisibleColumns((prev) => {
        const current = prev.length > 0 ? prev : columns;
        if (!current.includes(column) || current.length === 1) return current;
        return current.filter((col) => col !== column);
      });
    },
    [columns]
  );

  const handleAddColumnFilter = useCallback((column: string) => {
    setFilters((prev) => [...prev, createFilterCondition(`column-${column}-${Date.now()}`, column)]);
    setShowFilterSort(true);
  }, []);

  const handleViewDDL = useCallback(async () => {
    if (!assetId) return;
    setShowDDLDialog(true);
    setDdlLoading(true);

    try {
      let ddl = "";

      if (driver === "postgresql") {
        const tableLiteral = sqlQuote(table);
        const columnsSql = `SELECT column_name, data_type, udt_name, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ${tableLiteral} ORDER BY ordinal_position`;
        const primaryKeySql = `SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema WHERE tc.table_schema = 'public' AND tc.table_name = ${tableLiteral} AND tc.constraint_type = 'PRIMARY KEY' ORDER BY kcu.ordinal_position`;

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

            let line = `${quoteIdent(name, driver)} ${type}`;
            if (!nullable) line += " NOT NULL";
            if (columnDefault) line += ` DEFAULT ${columnDefault}`;
            return line;
          });

          if (primaryKeyColumns.length > 0) {
            defs.push(`PRIMARY KEY (${primaryKeyColumns.map((c) => quoteIdent(c, driver)).join(", ")})`);
          }

          ddl = `CREATE TABLE ${quoteIdent("public", driver)}.${quoteIdent(table, driver)} (\n  ${defs.join(",\n  ")}\n);`;
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
  const tableRows = useMemo(() => [...rows, ...newRows], [newRows, rows]);
  const pendingEditCount = edits.size + newRows.length;
  const hasEdits = pendingEditCount > 0;
  const hasSelectedRow = selectedRowIdx != null && tableRows[selectedRowIdx] != null;
  const effectiveVisibleColumns = visibleColumns.length > 0 ? visibleColumns : columns;
  const hasActiveFilterSort =
    filters.length > 0 || sorts.length > 0 || !!whereClause || !!orderByClause || !!sortColumn || !!sortDir;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 border-b border-border bg-muted/20 px-3 py-1.5 shrink-0">
        <Button variant="outline" size="sm" className="h-7 text-xs gap-1 shrink-0" onClick={handleViewDDL}>
          <FileCode2 className="h-3.5 w-3.5" />
          {t("query.viewDDL")}
        </Button>
        <TableEditorToolbar
          hasEdits={hasEdits}
          submitting={submitting || deleting}
          canExport={rows.length > 0}
          canImport={columns.length > 0}
          columns={columns}
          visibleColumns={effectiveVisibleColumns}
          rowDensity={rowDensity}
          exportFormat={exportFormat}
          onExportFormatChange={setExportFormat}
          onVisibleColumnToggle={handleVisibleColumnToggle}
          onRowDensityChange={setRowDensity}
          filterSortOpen={showFilterSort}
          filterSortActive={hasActiveFilterSort}
          onToggleFilterSort={() => setShowFilterSort((open) => !open)}
          onSubmit={() => openSqlDialog("confirm")}
          onDiscard={handleDiscard}
          onImport={() => setShowImportDialog(true)}
          onExport={handleExport}
          onPreviewSql={() => openSqlDialog("preview")}
        />
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

      {showFilterSort && (
        <TableFilterBuilder
          columns={columns}
          rows={rows}
          filters={filters}
          sorts={sorts}
          driver={driver}
          onChange={setFilters}
          onSortsChange={setSorts}
          onApply={handleApplyQuery}
        />
      )}

      {/* Table content */}
      <QueryResultTable
        columns={columns}
        rows={tableRows}
        loading={loading || importing}
        error={error ?? undefined}
        editable
        edits={edits}
        onCellEdit={handleCellEdit}
        onSetCellValue={handleCellEdit}
        onPasteCell={handleCellEdit}
        onGenerateUuid={handleCellEdit}
        onCopyAs={handleCopyAs}
        onFilterByCellValue={handleFilterByCellValue}
        onSortByColumn={handleSortByColumn}
        onClearFilterSort={handleClearFilterSort}
        onAddColumnFilter={handleAddColumnFilter}
        onRemoveColumnFilter={handleRemoveColumnFilter}
        onRemoveAllFilters={handleRemoveAllFilters}
        onDeleteRow={handleDeleteRow}
        onHideColumn={handleHideColumn}
        onVisibleColumnToggle={handleVisibleColumnToggle}
        onSelectedCellChange={handleSelectedCellChange}
        onRefresh={handleRefresh}
        showRowNumber
        rowNumberOffset={page * pageSize}
        sortColumn={sortColumn}
        sortDir={sortDir}
        onSortChange={handleSortChange}
        enableColumnFilter
        visibleColumns={effectiveVisibleColumns}
        columnTypes={columnTypes}
        rowDensity={rowDensity}
        focusCellRequest={focusCellRequest}
        renderCell={(value, ctx) => {
          if (ctx.rowIdx >= rows.length && value == null) {
            return <span className="text-muted-foreground/70 italic">(Default)</span>;
          }
          if (value == null) return <span className="text-muted-foreground italic">NULL</span>;
          return <span className="truncate block">{cellValueToText(value)}</span>;
        }}
      />

      {/* Footer bar */}
      <TableDataStatusBar
        pendingEditCount={pendingEditCount}
        sqlSummary={pendingSqlSummary}
        totalRows={totalRows}
        page={page}
        totalPages={totalPages}
        pageSize={pageSize}
        pageInput={pageInput}
        hasPrev={hasPrev}
        hasNext={hasNext}
        hasSelectedRow={hasSelectedRow}
        submitting={submitting || deleting}
        loading={loading || importing}
        refreshTitle={`${t("query.refreshTable")} (${REFRESH_SHORTCUT_LABEL})`}
        onRefresh={handleRefresh}
        onStopLoading={handleStopLoading}
        onPageInputChange={setPageInput}
        onPageInputConfirm={handlePageInputConfirm}
        onPageSizeChange={handlePageSizeChange}
        onFirstPage={() => setPage(0)}
        onPreviousPage={() => setPage((p) => p - 1)}
        onNextPage={() => setPage((p) => p + 1)}
        onLastPage={() => {
          if (totalPages != null) setPage(totalPages - 1);
        }}
        onAddRow={handleAddInlineRow}
        onDeleteRow={handleDeleteSelectedRow}
        onApplyChanges={() => openSqlDialog("confirm")}
        onDiscardChanges={handleDiscard}
      />

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

      <ImportTableDataDialog
        open={showImportDialog}
        onOpenChange={setShowImportDialog}
        assetId={assetId}
        database={database}
        table={table}
        columns={columns}
        columnTypes={columnTypes}
        primaryKeys={primaryKeys}
        driver={driver}
        onSubmittingChange={setImporting}
        onSubmitStart={handleImportSubmitStart}
        isSubmitCancelled={isImportSubmitCancelled}
        onSuccess={handleImportSuccess}
      />

      <ExportTableDataDialog
        open={showExportDialog}
        onOpenChange={setShowExportDialog}
        assetId={assetId}
        database={database}
        table={table}
        columns={columns}
        rows={rows}
        totalRows={totalRows}
        page={page}
        pageSize={pageSize}
        whereClause={whereClause}
        orderByClause={orderByClause}
        sortColumn={sortColumn}
        sortDir={sortDir}
        driver={driver}
        initialFormat={exportFormat}
        onFormatChange={setExportFormat}
      />

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

      <SqlPreviewDialog
        open={deletePreview !== null}
        onOpenChange={(open) => {
          if (!open && !deleting) setDeletePreview(null);
        }}
        statements={deletePreview ? [deletePreview.statement] : []}
        onConfirm={handleConfirmDelete}
        submitting={deleting}
        warning={
          deletePreview && !deletePreview.usesPrimaryKey ? (
            <div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
              {t("query.deleteRecordNoPrimaryKeyWarning")}
            </div>
          ) : undefined
        }
      />
    </div>
  );
}
