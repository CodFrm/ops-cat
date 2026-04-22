import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { X, Table2, Code2, Database, Loader2, Play } from "lucide-react";
import { Button, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Input, Textarea } from "@opskat/ui";
import { useResizeHandle } from "@opskat/ui";
import { useQueryStore, type MongoInnerTab } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { MongoDBCollectionBrowser } from "./MongoDBCollectionBrowser";
import { MongoDBResultView } from "./MongoDBResultView";
import { ExecuteMongo } from "../../../wailsjs/go/app/App";

interface MongoDBPanelProps {
  tabId: string;
}

export function MongoDBPanel({ tabId }: MongoDBPanelProps) {
  const { t } = useTranslation();
  const { mongoStates, closeMongoInnerTab, setActiveMongoInnerTab } = useQueryStore();
  const mongoState = mongoStates[tabId];

  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const meta = tab?.meta as QueryTabMeta | undefined;
  const assetId = meta?.assetId ?? 0;

  const { width: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultWidth: 200,
    minWidth: 140,
    maxWidth: 400,
  });

  if (!mongoState) return null;

  const { innerTabs, activeInnerTabId } = mongoState;

  return (
    <div className="flex h-full w-full">
      {/* Left sidebar: Collection browser */}
      <div
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
              <div
                key={innerTab.id}
                className="absolute inset-0 [contain:paint]"
                style={{
                  visibility: isActive ? "visible" : "hidden",
                  pointerEvents: isActive ? "auto" : "none",
                }}
              >
                {innerTab.type === "collection" ? (
                  <MongoCollectionContent
                    assetId={assetId}
                    database={innerTab.database}
                    collection={innerTab.collection}
                  />
                ) : (
                  <MongoQueryContent assetId={assetId} innerTab={innerTab} />
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
  assetId: number;
  database: string;
  collection: string;
}

function MongoCollectionContent({ assetId, database, collection }: MongoCollectionContentProps) {
  const [data, setData] = useState("");
  const [loading, setLoading] = useState(true);
  const [skip, setSkip] = useState(0);
  const limit = 20;

  const loadData = useCallback(
    async (newSkip: number) => {
      setLoading(true);
      try {
        const query = JSON.stringify({ skip: newSkip, limit });
        const result = await ExecuteMongo(assetId, "find", database, collection, query);
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
    loadData(0);
  }, [loadData]);

  return <MongoDBResultView data={data} loading={loading} skip={skip} limit={limit} onPageChange={loadData} />;
}

// --- Query Tab Content ---

const MONGO_OPERATIONS = [
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
] as const;

interface MongoQueryContentProps {
  assetId: number;
  innerTab: Extract<MongoInnerTab, { type: "query" }>;
}

function MongoQueryContent({ assetId, innerTab }: MongoQueryContentProps) {
  const { t } = useTranslation();
  const [operation, setOperation] = useState<string>("find");
  const [database, setDatabase] = useState(innerTab.database || "");
  const [collection, setCollection] = useState(innerTab.collection || "");
  const [query, setQuery] = useState("{}");
  const [data, setData] = useState("");
  const [loading, setLoading] = useState(false);
  const [skip, setSkip] = useState(0);
  const limit = 20;

  const operationLabels: Record<string, string> = {
    find: t("query.mongoFind"),
    findOne: t("query.mongoFindOne"),
    insertOne: t("query.mongoInsert"),
    insertMany: t("query.mongoInsertMany"),
    updateOne: t("query.mongoUpdate"),
    updateMany: t("query.mongoUpdateMany"),
    deleteOne: t("query.mongoDelete"),
    deleteMany: t("query.mongoDeleteMany"),
    aggregate: t("query.mongoAggregate"),
    countDocuments: t("query.mongoCount"),
  };

  const handleExecute = useCallback(
    async (execSkip = 0) => {
      if (!database || !collection) return;
      setLoading(true);
      try {
        let queryObj: Record<string, unknown>;
        try {
          queryObj = JSON.parse(query);
        } catch {
          queryObj = {};
        }
        // Inject skip/limit for find operations
        if (operation === "find") {
          queryObj.skip = execSkip;
          queryObj.limit = limit;
        }
        const result = await ExecuteMongo(assetId, operation, database, collection, JSON.stringify(queryObj));
        setData(result);
        setSkip(execSkip);
      } catch (err) {
        setData(JSON.stringify({ error: String(err) }));
      } finally {
        setLoading(false);
      }
    },
    [assetId, operation, database, collection, query]
  );

  const handlePageChange = useCallback(
    (newSkip: number) => {
      handleExecute(newSkip);
    },
    [handleExecute]
  );

  return (
    <div className="flex flex-col h-full">
      {/* Query editor area */}
      <div className="shrink-0 border-b border-border p-3 space-y-2">
        <div className="flex items-center gap-2">
          {/* Operation */}
          <Select value={operation} onValueChange={setOperation}>
            <SelectTrigger className="w-[140px] h-8 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {MONGO_OPERATIONS.map((op) => (
                <SelectItem key={op} value={op}>
                  {operationLabels[op] || op}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Database */}
          <Input
            className="w-[140px] h-8 text-xs"
            value={database}
            onChange={(e) => setDatabase(e.target.value)}
            placeholder={t("query.mongoDatabase")}
          />

          {/* Collection */}
          <Input
            className="flex-1 h-8 text-xs"
            value={collection}
            onChange={(e) => setCollection(e.target.value)}
            placeholder={t("query.mongoCollections")}
          />

          {/* Execute */}
          <Button
            size="sm"
            className="h-8 gap-1"
            onClick={() => handleExecute(0)}
            disabled={loading || !database || !collection}
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            {t("query.execute")}
          </Button>
        </div>

        <Textarea
          className="font-mono text-xs min-h-[60px] max-h-[200px] resize-y"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={operation === "aggregate" ? t("query.mongoPipeline") : t("query.mongoFilter")}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
              // IME 合成中：不触发执行，交给 IME 处理

              if (e.nativeEvent.isComposing || e.keyCode === 229) return;
              e.preventDefault();
              handleExecute(0);
            }
          }}
        />
      </div>

      {/* Result area */}
      <div className="flex-1 min-h-0">
        {data ? (
          <MongoDBResultView
            data={data}
            loading={loading}
            skip={skip}
            limit={limit}
            onPageChange={operation === "find" ? handlePageChange : undefined}
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
