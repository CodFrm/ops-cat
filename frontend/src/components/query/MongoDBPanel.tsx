import { useState, useEffect, useCallback, useMemo, useRef, useDeferredValue } from "react";
import { useTranslation } from "react-i18next";
import { X, Table2, Code2, Database, Loader2, Play, Filter, Download } from "lucide-react";
import type * as MonacoNS from "monaco-editor";
import { Button, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Input } from "@opskat/ui";
import { useResizeHandle } from "@opskat/ui";
import { toast } from "sonner";
import { useQueryStore, type MongoInnerTab } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { isMac, formatModKey } from "@/stores/shortcutStore";
import { MongoDBCollectionBrowser } from "./MongoDBCollectionBrowser";
import { MongoDBResultView } from "./MongoDBResultView";
import { ExecuteMongo } from "../../../wailsjs/go/app/App";
import { CodeEditor } from "@/components/CodeEditor";
import { parseMongosh, type ParsedMongosh } from "@/lib/mongosh-parser";

interface MongoDBPanelProps {
  tabId: string;
}

export function MongoDBPanel({ tabId }: MongoDBPanelProps) {
  const { t } = useTranslation();
  const mongoState = useQueryStore((s) => s.mongoStates[tabId]);
  const closeMongoInnerTab = useQueryStore((s) => s.closeMongoInnerTab);
  const setActiveMongoInnerTab = useQueryStore((s) => s.setActiveMongoInnerTab);

  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const meta = tab?.meta as QueryTabMeta | undefined;
  const assetId = meta?.assetId ?? 0;

  const sidebarRef = useRef<HTMLDivElement>(null);
  const { size: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultSize: 200,
    minSize: 140,
    maxSize: 400,
    targetRef: sidebarRef,
  });

  if (!mongoState) return null;

  const { innerTabs, activeInnerTabId } = mongoState;

  return (
    <div className="flex h-full w-full">
      {/* Left sidebar: Collection browser */}
      <div
        ref={sidebarRef}
        className="shrink-0 border-r border-border bg-sidebar h-full overflow-hidden"
        style={{ width: sidebarWidth }}
      >
        <MongoDBCollectionBrowser tabId={tabId} assetId={assetId} />
      </div>

      {/* Resize handle */}
      <div
        className="w-[3px] shrink-0 cursor-col-resize hover:bg-ring/40 active:bg-ring/60 transition-colors"
        onMouseDown={handleMouseDown}
      />

      {/* Right content area */}
      <div className="flex-1 min-w-0 flex flex-col h-full">
        {/* Inner tab bar */}
        {innerTabs.length > 0 && (
          <div className="flex items-center border-b border-border bg-muted/30 shrink-0 overflow-x-auto">
            {innerTabs.map((innerTab) => {
              const isActive = innerTab.id === activeInnerTabId;
              return (
                <div
                  key={innerTab.id}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs cursor-pointer border-r border-border whitespace-nowrap select-none transition-colors duration-150 ${
                    isActive
                      ? "bg-background text-foreground"
                      : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
                  }`}
                  onClick={() => setActiveMongoInnerTab(tabId, innerTab.id)}
                >
                  {innerTab.type === "collection" ? (
                    <Table2 className="h-3 w-3 shrink-0" />
                  ) : (
                    <Code2 className="h-3 w-3 shrink-0" />
                  )}
                  <span className="truncate max-w-[120px]">
                    {innerTab.type === "collection" ? `${innerTab.database}.${innerTab.collection}` : innerTab.title}
                  </span>
                  <button
                    className="ml-1 rounded-sm p-0.5 hover:bg-muted transition-colors"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeMongoInnerTab(tabId, innerTab.id);
                    }}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {/* Tab content */}
        <div className="flex-1 min-h-0 relative">
          {innerTabs.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full gap-3 text-muted-foreground">
              <Database className="h-10 w-10 opacity-30" />
              <p className="text-xs">{t("query.mongoDocuments")}</p>
            </div>
          )}
          {innerTabs.map((innerTab) => {
            const isActive = innerTab.id === activeInnerTabId;
            return (
              <div key={innerTab.id} className="absolute inset-0" style={{ display: isActive ? "block" : "none" }}>
                {innerTab.type === "collection" ? (
                  <MongoCollectionContent
                    tabId={tabId}
                    innerTabId={innerTab.id}
                    assetId={assetId}
                    database={innerTab.database}
                    collection={innerTab.collection}
                    pendingLoad={innerTab.pendingLoad === true}
                  />
                ) : (
                  <MongoQueryContent tabId={tabId} assetId={assetId} innerTab={innerTab} />
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

// --- Collection Tab Content ---

interface MongoCollectionContentProps {
  tabId: string;
  innerTabId: string;
  assetId: number;
  database: string;
  collection: string;
  pendingLoad: boolean;
}

const MONGO_REFRESH_SHORTCUT_LABEL = formatModKey("KeyR");

function MongoCollectionContent(props: MongoCollectionContentProps) {
  const { t } = useTranslation();
  const { markMongoCollectionTabLoaded } = useQueryStore();
  const isOuterActive = useTabStore((s) => s.activeTabId === props.tabId);
  const isInnerActive = useQueryStore((s) => s.mongoStates[props.tabId]?.activeInnerTabId === props.innerTabId);

  useEffect(() => {
    if (!props.pendingLoad || !isOuterActive || !isInnerActive) return;
    const handler = (e: KeyboardEvent) => {
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (mod && e.code === "KeyR" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        markMongoCollectionTabLoaded(props.tabId, props.innerTabId);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [props.pendingLoad, isOuterActive, isInnerActive, markMongoCollectionTabLoaded, props.tabId, props.innerTabId]);

  if (props.pendingLoad) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-muted-foreground">
        <p className="text-xs">
          {t("query.tableRestoredHint", {
            table: `${props.database}.${props.collection}`,
            shortcut: MONGO_REFRESH_SHORTCUT_LABEL,
          })}
        </p>
        <Button
          variant="outline"
          size="sm"
          className="h-8 text-xs gap-1"
          onClick={() => markMongoCollectionTabLoaded(props.tabId, props.innerTabId)}
        >
          <Download className="h-3.5 w-3.5" />
          {t("query.loadData")}
        </Button>
      </div>
    );
  }

  return <MongoCollectionContentBody {...props} />;
}

function MongoCollectionContentBody({ tabId, innerTabId, assetId, database, collection }: MongoCollectionContentProps) {
  const { t } = useTranslation();
  const [data, setData] = useState("");
  const [loading, setLoading] = useState(true);
  const [skip, setSkip] = useState(0);
  const [limit, setLimit] = useState(100);
  const isOuterActive = useTabStore((s) => s.activeTabId === tabId);
  const isInnerActive = useQueryStore((s) => s.mongoStates[tabId]?.activeInnerTabId === innerTabId);

  // In-progress edit state
  const [filterInput, setFilterInput] = useState("");
  const [sortInput, setSortInput] = useState("");
  // Committed state (what the server is currently using). Decoupling the two
  // means pagination / refresh keep using the last valid query even if the
  // user has half-typed something new in the inputs.
  const [appliedFilter, setAppliedFilter] = useState("");
  const [appliedSort, setAppliedSort] = useState("");

  const loadData = useCallback(
    async (newSkip: number, newLimit: number, filterJSON: string, sortJSON: string) => {
      setLoading(true);
      try {
        const query: Record<string, unknown> = { skip: newSkip, limit: newLimit };
        if (filterJSON.trim()) query.filter = JSON.parse(filterJSON);
        if (sortJSON.trim()) query.sort = JSON.parse(sortJSON);
        const result = await ExecuteMongo(assetId, "find", database, collection, JSON.stringify(query));
        setData(result);
        setSkip(newSkip);
      } catch (err) {
        setData(JSON.stringify({ error: String(err) }));
      } finally {
        setLoading(false);
      }
    },
    [assetId, database, collection]
  );

  useEffect(() => {
    loadData(0, limit, "", "");
    // Intentionally only depends on loadData — limit / filter / sort changes are
    // driven by their own handlers which call loadData directly.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadData]);

  const handleApply = () => {
    if (filterInput.trim()) {
      try {
        JSON.parse(filterInput);
      } catch {
        toast.error(t("query.mongoInvalidFilter"));
        return;
      }
    }
    if (sortInput.trim()) {
      try {
        JSON.parse(sortInput);
      } catch {
        toast.error(t("query.mongoInvalidSort"));
        return;
      }
    }
    setAppliedFilter(filterInput);
    setAppliedSort(sortInput);
    loadData(0, limit, filterInput, sortInput);
  };

  const handlePageSizeChange = (size: number) => {
    setLimit(size);
    loadData(0, size, appliedFilter, appliedSort);
  };

  const handleRefresh = useCallback(
    () => loadData(skip, limit, appliedFilter, appliedSort),
    [loadData, skip, limit, appliedFilter, appliedSort]
  );

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

  return (
    <div className="flex flex-col h-full">
      {/* Filter / sort bar */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/20 shrink-0">
        <div className="flex items-center gap-1 flex-1 min-w-0">
          <span className="text-[11px] font-mono text-muted-foreground">FILTER</span>
          <Input
            className="h-7 text-xs font-mono"
            value={filterInput}
            onChange={(e) => setFilterInput(e.target.value)}
            placeholder={t("query.mongoFilterPlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                handleApply();
              }
            }}
          />
        </div>
        <div className="flex items-center gap-1 flex-1 min-w-0">
          <span className="text-[11px] font-mono text-muted-foreground whitespace-nowrap">SORT</span>
          <Input
            className="h-7 text-xs font-mono"
            value={sortInput}
            onChange={(e) => setSortInput(e.target.value)}
            placeholder={t("query.mongoSortPlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                handleApply();
              }
            }}
          />
        </div>
        <Button variant="outline" size="sm" className="h-7 text-xs gap-1 shrink-0" onClick={handleApply}>
          <Filter className="h-3.5 w-3.5" />
          {t("query.applyFilter")}
        </Button>
      </div>

      {/* Result */}
      <div className="flex-1 min-h-0">
        <MongoDBResultView
          data={data}
          loading={loading}
          skip={skip}
          limit={limit}
          onPageChange={(s) => loadData(s, limit, appliedFilter, appliedSort)}
          onPageSizeChange={handlePageSizeChange}
          onRefresh={handleRefresh}
          refreshShortcutLabel={MONGO_REFRESH_SHORTCUT_LABEL}
        />
      </div>
    </div>
  );
}

// --- Query Tab Content ---

const MONGO_COLLECTION_METHODS = [
  "find",
  "findOne",
  "insertOne",
  "insertMany",
  "updateOne",
  "updateMany",
  "deleteOne",
  "deleteMany",
  "aggregate",
  "countDocuments",
];

interface MongoQueryContentProps {
  tabId: string;
  assetId: number;
  innerTab: Extract<MongoInnerTab, { type: "query" }>;
}

function MongoQueryContent({ tabId, assetId, innerTab }: MongoQueryContentProps) {
  const { t } = useTranslation();
  const mongoState = useQueryStore((s) => s.mongoStates[tabId]);
  const updateMongoInnerTab = useQueryStore((s) => s.updateMongoInnerTab);
  const availableDbs = mongoState?.databases ?? [];

  // 优先用 inner tab 自己记住的库，否则用当前选中的库
  const [database, setDatabase] = useState<string>(innerTab.database || mongoState?.activeDatabase || "");
  // 编辑器内容由 Monaco 自管（非受控）。queryRef 持最新值，query state 仅在 debounce 后同步用于驱动
  // parsePreview / canExecute / 占位提示等低频派生信息。按键本身不再触发 React 重渲。
  const editorRef = useRef<MonacoNS.editor.IStandaloneCodeEditor | null>(null);
  const queryRef = useRef(innerTab.queryText || "");
  const [query, setQuery] = useState(innerTab.queryText || "");
  const [data, setData] = useState("");
  const [loading, setLoading] = useState(false);
  // 执行过后缓存解析结果；分页 / 刷新据此重放
  const [lastParsed, setLastParsed] = useState<ParsedMongosh | null>(null);
  const [skip, setSkip] = useState(0);
  const [limit, setLimit] = useState(100);

  // Editor/result split — drag the bar between them to adjust editor height.
  const editorAreaRef = useRef<HTMLDivElement>(null);
  const { size: editorHeight, handleMouseDown: handleSplitterDown } = useResizeHandle({
    axis: "y",
    defaultSize: innerTab.editorHeight && innerTab.editorHeight > 0 ? innerTab.editorHeight : 200,
    minSize: 120,
    maxSize: 800,
    onResizeEnd: (h) => updateMongoInnerTab(tabId, innerTab.id, { editorHeight: h }),
    targetRef: editorAreaRef,
  });

  // database 低频，直接 sync；queryText 走 commit 时机
  useEffect(() => {
    updateMongoInnerTab(tabId, innerTab.id, { database });
  }, [database, tabId, innerTab.id, updateMongoInnerTab]);

  const commitQueryRef = useRef<() => void>(() => {
    updateMongoInnerTab(tabId, innerTab.id, { queryText: queryRef.current });
  });
  useEffect(() => {
    commitQueryRef.current = () => {
      updateMongoInnerTab(tabId, innerTab.id, { queryText: queryRef.current });
    };
  }, [tabId, innerTab.id, updateMongoInnerTab]);

  useEffect(() => {
    return () => {
      commitQueryRef.current();
    };
  }, []);

  // 预览解析结果（显示在编辑器下方小字提示），parse 错误不在此提示——只在执行时报错
  // useDeferredValue 让 parser 以低优先级跑，遇到紧急渲染（如切 Tab）可被打断
  const deferredQuery = useDeferredValue(query);
  const deferredDatabase = useDeferredValue(database);
  const parsePreview = useMemo(() => {
    if (!deferredQuery.trim()) return null;
    const r = parseMongosh(deferredQuery, deferredDatabase);
    if (!r.ok) return null;
    return r.value;
  }, [deferredQuery, deferredDatabase]);

  const execute = useCallback(
    async (execSkip = 0, execLimit = limit) => {
      const q = queryRef.current;
      if (!q.trim()) {
        toast.error(t("query.mongoshEmpty"));
        return;
      }
      const parsed = parseMongosh(q, database);
      if (!parsed.ok) {
        setData(JSON.stringify({ error: parsed.error.message }));
        toast.error(parsed.error.message);
        return;
      }
      if (!parsed.value.database) {
        toast.error(t("query.mongoshNoDatabase"));
        return;
      }
      setLoading(true);
      setLastParsed(parsed.value);
      try {
        const toSend: Record<string, unknown> = { ...parsed.value.query };
        // 分页覆盖：用户在语句里没显式写 limit/skip 时，UI 注入默认值
        if (parsed.value.operation === "find") {
          if (toSend.skip === undefined) toSend.skip = execSkip;
          if (toSend.limit === undefined) toSend.limit = execLimit;
        }
        const result = await ExecuteMongo(
          assetId,
          parsed.value.operation,
          parsed.value.database,
          parsed.value.collection,
          JSON.stringify(toSend)
        );
        setData(result);
        setSkip(typeof toSend.skip === "number" ? toSend.skip : execSkip);
        if (typeof toSend.limit === "number") setLimit(toSend.limit);
      } catch (err) {
        setData(JSON.stringify({ error: String(err) }));
      } finally {
        setLoading(false);
      }
    },
    [assetId, database, limit, t]
  );

  const handlePageChange = useCallback((newSkip: number) => execute(newSkip), [execute]);
  const handlePageSizeChange = useCallback(
    (size: number) => {
      setLimit(size);
      execute(0, size);
    },
    [execute]
  );

  // Bridge latest handleExecute into monaco command (registered once at mount).
  const executeRef = useRef(execute);
  useEffect(() => {
    executeRef.current = execute;
  }, [execute]);

  const handleEditorMount = useCallback((editor: MonacoNS.editor.IStandaloneCodeEditor, monaco: typeof MonacoNS) => {
    editorRef.current = editor;
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      executeRef.current(0);
    });

    // 订阅内容变化：更新 ref，debounce 300ms 同步 query state + 提交 store
    const model = editor.getModel();
    if (model) {
      let timer: ReturnType<typeof setTimeout> | null = null;
      model.onDidChangeContent(() => {
        queryRef.current = model.getValue();
        if (timer) clearTimeout(timer);
        timer = setTimeout(() => {
          timer = null;
          setQuery(queryRef.current);
          commitQueryRef.current();
        }, 300);
      });
    }

    editor.onDidBlurEditorWidget(() => {
      setQuery(queryRef.current);
      commitQueryRef.current();
    });
  }, []);

  // 当前库的集合列表——给 dynamicCompletions 用
  const currentCollections = useMemo(() => {
    if (!database || !mongoState) return [];
    return mongoState.collections[database] ?? [];
  }, [database, mongoState]);

  const dynamicCompletions = useCallback(
    (ctx: {
      monaco: typeof MonacoNS;
      range: MonacoNS.IRange;
      model: MonacoNS.editor.ITextModel;
      position: MonacoNS.Position;
    }) => {
      const line = ctx.model.getLineContent(ctx.position.lineNumber);
      const before = line.slice(0, ctx.position.column - 1);
      const items: MonacoNS.languages.CompletionItem[] = [];

      // db.<cursor> → 集合名
      if (/\bdb\.[\w$]*$/.test(before)) {
        for (const col of currentCollections) {
          items.push({
            label: col,
            kind: ctx.monaco.languages.CompletionItemKind.Class,
            insertText: col,
            range: ctx.range,
            sortText: "0_" + col,
            detail: t("query.mongoshCollection"),
          });
        }
      }

      // db.getSiblingDB("x").<cursor> — 暂不跨库查集合列表
      // db.<collection>.<cursor> → 方法名（<collection> 必须匹配一个已知集合或任意标识符）
      if (/\bdb\.[A-Za-z_$][\w$]*\.[\w$]*$/.test(before)) {
        for (const m of MONGO_COLLECTION_METHODS) {
          items.push({
            label: m,
            kind: ctx.monaco.languages.CompletionItemKind.Method,
            insertText: m,
            range: ctx.range,
            sortText: "0_" + m,
          });
        }
      }

      return items;
    },
    [currentCollections, t]
  );

  const shortcutLabel = formatModKey("Enter");
  const canExecute = !!database || /getSiblingDB\s*\(/.test(query);

  return (
    <div className="flex flex-col h-full">
      {/* Query editor area */}
      <div ref={editorAreaRef} className="shrink-0 flex flex-col p-3 gap-2" style={{ height: editorHeight }}>
        <div className="flex items-center gap-2 shrink-0">
          {/* Current database */}
          <Select
            value={database || undefined}
            onValueChange={(v) => setDatabase(v)}
            disabled={availableDbs.length === 0}
          >
            <SelectTrigger className="w-[180px] h-8 text-xs">
              <Database className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <SelectValue placeholder={t("query.mongoshPickDatabase")} />
            </SelectTrigger>
            <SelectContent>
              {availableDbs.map((db) => (
                <SelectItem key={db} value={db}>
                  {db}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Parsed preview */}
          <div className="flex-1 min-w-0 text-[11px] text-muted-foreground font-mono truncate">
            {parsePreview
              ? t("query.mongoshParsedAs", {
                  op: parsePreview.operation,
                  db: parsePreview.database || "?",
                  coll: parsePreview.collection,
                })
              : query.trim()
                ? t("query.mongoshWillParse")
                : t("query.mongoshHint")}
          </div>

          {/* Execute */}
          <Button
            size="sm"
            className="h-8 gap-1"
            onClick={() => execute(0)}
            disabled={loading || !canExecute}
            title={shortcutLabel}
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            {t("query.execute")}
          </Button>
        </div>

        <div className="flex-1 min-h-0 overflow-hidden border border-border rounded-md bg-background">
          <CodeEditor
            defaultValue={innerTab.queryText || ""}
            language="javascript"
            placeholder={t("query.mongoshPlaceholder")}
            onMount={handleEditorMount}
            dynamicCompletions={dynamicCompletions}
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
      <div className="flex-1 min-h-0">
        {data ? (
          <MongoDBResultView
            data={data}
            loading={loading}
            skip={skip}
            limit={limit}
            onPageChange={lastParsed?.operation === "find" ? handlePageChange : undefined}
            onPageSizeChange={lastParsed?.operation === "find" ? handlePageSizeChange : undefined}
            onRefresh={() => execute(skip)}
          />
        ) : (
          <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
            {t("query.noResult")}
          </div>
        )}
      </div>
    </div>
  );
}
