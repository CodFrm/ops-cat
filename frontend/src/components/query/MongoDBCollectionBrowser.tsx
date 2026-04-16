import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, ChevronDown, Database, Table2, Plus, RefreshCw, Loader2 } from "lucide-react";
import { Button, ScrollArea } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";

interface MongoDBCollectionBrowserProps {
  tabId: string;
  assetId: number;
}

export function MongoDBCollectionBrowser({ tabId, assetId }: MongoDBCollectionBrowserProps) {
  const { t } = useTranslation();
  const { mongoStates, loadMongoDatabases, loadMongoCollections, openCollectionTab, openMongoQueryTab } =
    useQueryStore();

  const mongoState = mongoStates[tabId];
  const [expandedDbs, setExpandedDbs] = useState<Set<string>>(new Set());
  const [loadingDbs, setLoadingDbs] = useState(false);
  const [loadingCollections, setLoadingCollections] = useState<Set<string>>(new Set());

  useEffect(() => {
    setLoadingDbs(true);
    loadMongoDatabases(tabId).finally(() => setLoadingDbs(false));
  }, [tabId, assetId, loadMongoDatabases]);

  if (!mongoState) return null;

  const { databases, collections } = mongoState;

  const toggleDbExpand = (db: string) => {
    const next = new Set(expandedDbs);
    if (next.has(db)) {
      next.delete(db);
    } else {
      next.add(db);
      // Load collections if not loaded
      if (!collections[db]) {
        setLoadingCollections((prev) => new Set(prev).add(db));
        loadMongoCollections(tabId, db).finally(() => {
          setLoadingCollections((prev) => {
            const next = new Set(prev);
            next.delete(db);
            return next;
          });
        });
      }
    }
    setExpandedDbs(next);
  };

  const handleRefresh = () => {
    setLoadingDbs(true);
    loadMongoDatabases(tabId).finally(() => setLoadingDbs(false));
  };

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-2 py-1.5 border-b border-border shrink-0">
        <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("query.collections")}
        </span>
        <div className="flex gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => openMongoQueryTab(tabId)}
            title={t("query.newSql")}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={handleRefresh}
            title={t("query.refreshTree")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Tree */}
      <ScrollArea className="flex-1 min-h-0">
        <div className="p-1 space-y-0.5">
          {loadingDbs ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : databases.length === 0 ? (
            <div className="text-xs text-muted-foreground text-center py-4">{t("query.databases")}</div>
          ) : (
            databases.map((db) => {
              const isExpanded = expandedDbs.has(db);
              const dbCollections = collections[db];

              return (
                <div key={db}>
                  {/* Database node */}
                  <div
                    className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                    onClick={() => toggleDbExpand(db)}
                  >
                    {isExpanded ? (
                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                    ) : (
                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                    )}
                    <Database className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span className="truncate">{db}</span>
                  </div>

                  {/* Collections */}
                  {isExpanded && (
                    <div className="ml-3">
                      {loadingCollections.has(db) || !dbCollections ? (
                        <div className="flex items-center gap-1.5 px-2 py-1">
                          <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                        </div>
                      ) : dbCollections.length === 0 ? (
                        <div className="px-2 py-1 text-xs text-muted-foreground italic">
                          {t("query.mongoCollections")}
                        </div>
                      ) : (
                        dbCollections.map((col) => (
                          <div
                            key={col}
                            className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                            onClick={() => openCollectionTab(tabId, db, col)}
                          >
                            <Table2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                            <span className="truncate">{col}</span>
                          </div>
                        ))
                      )}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
