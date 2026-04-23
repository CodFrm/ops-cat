import { memo, useState, useCallback, useRef, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Play, Loader2, History, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import type * as MonacoNS from "monaco-editor";
import {
  Button,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Popover,
  PopoverTrigger,
  PopoverContent,
  ConfirmDialog,
  useResizeHandle,
} from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteSQLPaged } from "../../../wailsjs/go/app/App";
import { QueryResultTable } from "./QueryResultTable";
import { CodeEditor } from "@/components/CodeEditor";
import type { DynamicCompletionGetter } from "@/lib/monaco-completions";

interface SqlEditorTabProps {
  tabId: string;
  innerTabId: string;
}

interface SQLPagedResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  total_count?: number;
  affected_rows?: number;
}

const PAGE_SIZES = [50, 100, 200, 500];
const DEFAULT_PAGE_SIZE = 100;

// props 为稳定字符串 —— memo 可阻止父组件的 innerTabs 结构变化传导进来
export const SqlEditorTab = memo(function SqlEditorTab({ tabId, innerTabId }: SqlEditorTabProps) {
  const { t } = useTranslation();
  // 细粒度订阅：actions 是稳定引用，dbState 整体取用（innerTab.sql 不再每键入都写回，所以 dbState 不会高频变化）
  const dbState = useQueryStore((s) => s.dbStates[tabId]);
  const updateInnerTab = useQueryStore((s) => s.updateInnerTab);
  const addSqlHistory = useQueryStore((s) => s.addSqlHistory);
  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const queryMeta = tab?.meta as QueryTabMeta | undefined;
  const editorRef = useRef<MonacoNS.editor.IStandaloneCodeEditor | null>(null);

  const assetId = queryMeta?.assetId ?? 0;
  const databases = useMemo(() => dbState?.databases || [], [dbState?.databases]);

  // 只在 mount 时读一次持久化内容 —— 之后编辑器内容由 Monaco 自管（非受控）
  const innerTabAtMount = useRef(dbState?.innerTabs.find((t) => t.id === innerTabId));
  const persistedSql = innerTabAtMount.current?.type === "sql" ? innerTabAtMount.current.sql : undefined;
  const persistedDb = innerTabAtMount.current?.type === "sql" ? innerTabAtMount.current.selectedDb : undefined;
  const persistedHeight = innerTabAtMount.current?.type === "sql" ? innerTabAtMount.current.editorHeight : undefined;

  // history 需要跟当前 innerTab 保持同步（执行 SQL 后会追加）
  const innerTab = dbState?.innerTabs.find((t) => t.id === innerTabId);
  const sqlHistory = innerTab?.type === "sql" ? innerTab.history || [] : [];

  // Editor/result split — drag the bar between them to adjust editor height.
  const editorAreaRef = useRef<HTMLDivElement>(null);
  const { size: editorHeight, handleMouseDown: handleSplitterDown } = useResizeHandle({
    axis: "y",
    defaultSize: persistedHeight && persistedHeight > 0 ? persistedHeight : 160,
    minSize: 80,
    maxSize: 800,
    onResizeEnd: (h) => updateInnerTab(tabId, innerTabId, { editorHeight: h }),
    targetRef: editorAreaRef,
  });

  // 当前 SQL 不作为 React state：由 editor model 持有，通过 ref/getValue 读取
  const sqlRef = useRef(persistedSql || "");
  // 仅 "空 ↔ 非空" 跨边界时 setState，用于驱动"执行"按钮禁用态
  const [isEmpty, setIsEmpty] = useState((persistedSql || "").length === 0);
  const [selectedDb, setSelectedDb] = useState(persistedDb || queryMeta?.defaultDatabase || "");
  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  const [totalRows, setTotalRows] = useState<number | null>(null);
  const [affectedRows, setAffectedRows] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showDangerConfirm, setShowDangerConfirm] = useState(false);
  const [showHistory, setShowHistory] = useState(false);

  // Pagination state
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [pageInput, setPageInput] = useState("1");
  // Store the last executed SQL for pagination
  const [lastExecSql, setLastExecSql] = useState("");

  const totalPages = totalRows != null ? Math.max(1, Math.ceil(totalRows / pageSize)) : null;

  // Set default database when databases load
  useEffect(() => {
    if (!selectedDb && databases.length > 0) {
      setSelectedDb(queryMeta?.defaultDatabase || databases[0]);
    }
  }, [databases, selectedDb, queryMeta?.defaultDatabase]);

  // Persist selectedDb to store (SQL 通过 commit 时机单独同步)
  useEffect(() => {
    updateInnerTab(tabId, innerTabId, { selectedDb });
  }, [selectedDb, tabId, innerTabId, updateInnerTab]);

  // 把最新的 sql 同步进 store。用 ref 桥接，handleEditorMount 里注册的回调永远读到最新版本。
  const commitSqlRef = useRef<() => void>(() => {
    updateInnerTab(tabId, innerTabId, { sql: sqlRef.current });
  });
  useEffect(() => {
    commitSqlRef.current = () => {
      updateInnerTab(tabId, innerTabId, { sql: sqlRef.current });
    };
  }, [tabId, innerTabId, updateInnerTab]);

  // 卸载时立即 flush（关闭 inner tab / 关闭整个 query tab 时触发）
  useEffect(() => {
    return () => {
      commitSqlRef.current();
    };
  }, []);

  // Sync page input
  useEffect(() => {
    setPageInput(String(page + 1));
  }, [page]);

  const isDangerousSQL = useCallback((text: string) => {
    const upper = text.toUpperCase().replace(/\s+/g, " ").trim();
    return /^(DELETE|DROP|TRUNCATE|ALTER)\b/.test(upper);
  }, []);

  // Get the SQL text to execute: selected text if any, otherwise full text
  const getExecutableSQL = useCallback(() => {
    const editor = editorRef.current;
    if (editor) {
      const sel = editor.getSelection();
      if (sel && !sel.isEmpty()) {
        const text = editor.getModel()?.getValueInRange(sel) ?? "";
        return text.trim();
      }
      return editor.getValue().trim();
    }
    return sqlRef.current.trim();
  }, []);

  const fetchPage = useCallback(
    async (execSql: string, pageNum: number) => {
      if (!execSql || !assetId) return;

      setLoading(true);
      setError(null);

      try {
        const result = await ExecuteSQLPaged(assetId, execSql, selectedDb, pageNum, pageSize);
        const parsed: SQLPagedResult = JSON.parse(result);

        if (parsed.affected_rows !== undefined) {
          setAffectedRows(parsed.affected_rows);
          setColumns([]);
          setRows([]);
          setTotalRows(null);
        } else {
          setAffectedRows(null);
          setColumns(parsed.columns || []);
          setRows(parsed.rows || []);
          setTotalRows(parsed.total_count ?? null);
        }
      } catch (e) {
        setError(String(e));
        setColumns([]);
        setRows([]);
        setTotalRows(null);
      } finally {
        setLoading(false);
      }
    },
    [assetId, selectedDb, pageSize]
  );

  const doExecute = useCallback(async () => {
    const execSql = getExecutableSQL();
    if (!execSql || !assetId) return;

    addSqlHistory(tabId, innerTabId, execSql);

    setLastExecSql(execSql);
    setPage(0);
    await fetchPage(execSql, 0);
  }, [getExecutableSQL, assetId, fetchPage, addSqlHistory, tabId, innerTabId]);

  // Re-fetch when page changes (but not on initial execute)
  useEffect(() => {
    if (lastExecSql && page > 0) {
      fetchPage(lastExecSql, page);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page]);

  // Re-fetch from page 0 when pageSize changes (only if we have a previous query)
  useEffect(() => {
    if (lastExecSql) {
      setPage(0);
      fetchPage(lastExecSql, 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageSize]);

  const execute = useCallback(() => {
    const execSql = getExecutableSQL();
    if (!execSql || !assetId) return;
    if (isDangerousSQL(execSql)) {
      setShowDangerConfirm(true);
    } else {
      doExecute();
    }
  }, [getExecutableSQL, assetId, isDangerousSQL, doExecute]);

  // Monaco 命令是在 onMount 时一次性注册的，闭包里拿到的 execute 会过期；
  // 用 ref 桥接到最新版。
  const executeRef = useRef(execute);
  useEffect(() => {
    executeRef.current = execute;
  }, [execute]);

  const handleEditorMount = useCallback((editor: MonacoNS.editor.IStandaloneCodeEditor, monaco: typeof MonacoNS) => {
    editorRef.current = editor;
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      executeRef.current();
    });

    // 订阅内容变化：更新 ref + 维护 isEmpty + debounce 300ms 回写 store
    const model = editor.getModel();
    if (model) {
      let prevEmpty = model.getValue().length === 0;
      let timer: ReturnType<typeof setTimeout> | null = null;
      model.onDidChangeContent(() => {
        const val = model.getValue();
        sqlRef.current = val;
        const nowEmpty = val.length === 0;
        if (nowEmpty !== prevEmpty) {
          prevEmpty = nowEmpty;
          setIsEmpty(nowEmpty);
        }
        if (timer) clearTimeout(timer);
        timer = setTimeout(() => {
          timer = null;
          commitSqlRef.current();
        }, 300);
      });
    }

    // 失焦立即提交，避免等 300ms debounce
    editor.onDidBlurEditorWidget(() => {
      commitSqlRef.current();
    });
  }, []);

  // 把当前选中库的表名注入到 monaco 补全（在 . 触发或主动唤起时一并出现）
  const tables = useMemo(() => dbState?.tables?.[selectedDb] ?? [], [dbState?.tables, selectedDb]);
  const tableCompletions = useCallback<DynamicCompletionGetter>(
    ({ monaco, range }) =>
      tables.map((tableName) => ({
        label: tableName,
        kind: monaco.languages.CompletionItemKind.Class,
        insertText: tableName,
        detail: selectedDb ? `table · ${selectedDb}` : "table",
        range,
        sortText: "0_" + tableName, // 表名排在关键字之前
      })),
    [tables, selectedDb]
  );

  const handlePageInputConfirm = useCallback(() => {
    const num = parseInt(pageInput, 10);
    if (isNaN(num) || num < 1) {
      setPageInput(String(page + 1));
      return;
    }
    const target = totalPages ? Math.min(num, totalPages) - 1 : num - 1;
    setPage(target);
  }, [pageInput, page, totalPages]);

  const hasNext = totalPages != null ? page < totalPages - 1 : rows.length === pageSize;
  const hasPrev = page > 0;
  const showPagination = columns.length > 0 && !loading && !error && lastExecSql;

  return (
    <div className="flex flex-col h-full">
      {/* SQL editor area */}
      <div ref={editorAreaRef} className="flex flex-col shrink-0" style={{ height: editorHeight }}>
        {/* Toolbar */}
        <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/30">
          <Button
            variant="default"
            size="sm"
            className="h-7 text-xs gap-1"
            onClick={execute}
            disabled={loading || isEmpty}
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
                      editorRef.current?.setValue(item);
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
        {/* Monaco editor; automaticLayout lets it reflow when the splitter changes height */}
        <div className="flex-1 min-h-0 w-full overflow-hidden bg-background">
          <CodeEditor
            defaultValue={persistedSql || ""}
            language="sql"
            placeholder={t("query.sqlPlaceholder")}
            onMount={handleEditorMount}
            dynamicCompletions={tableCompletions}
          />
        </div>
      </div>

      {/* Vertical splitter between editor and results */}
      <div
        role="separator"
        aria-orientation="horizontal"
        className="h-[4px] shrink-0 cursor-row-resize border-y border-border bg-muted/30 hover:bg-ring/40 active:bg-ring/60 transition-colors"
        onMouseDown={handleSplitterDown}
      />

      {/* Result area */}
      <div className="flex-1 min-h-0 flex flex-col">
        {affectedRows !== null && !error && (
          <div className="px-3 py-2 text-xs text-muted-foreground">
            {t("query.affectedRows")}: {affectedRows}
          </div>
        )}
        {showPagination && (
          <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/30 shrink-0">
            {totalRows != null && (
              <span className="text-xs text-muted-foreground">{t("query.totalRows", { count: totalRows })}</span>
            )}
            <div className="ml-auto flex items-center gap-1">
              {/* Page size selector */}
              <Select
                value={String(pageSize)}
                onValueChange={(v) => {
                  setPageSize(Number(v));
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
        )}
        <QueryResultTable
          columns={columns}
          rows={rows}
          loading={loading}
          error={error ?? undefined}
          showRowNumber
          rowNumberOffset={page * pageSize}
        />
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
});
