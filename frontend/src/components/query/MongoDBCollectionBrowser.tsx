import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, ChevronDown, Code2, Database, Table2, Plus, RefreshCw, Loader2, Search } from "lucide-react";
import { Button, Input, ScrollArea } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";

interface MongoDBCollectionBrowserProps {
  tabId: string;
  assetId: number;
}

export function MongoDBCollectionBrowser({ tabId, assetId }: MongoDBCollectionBrowserProps) {
  const { t } = useTranslation();
  const {
    mongoStates,
    loadMongoDatabases,
    loadMongoCollections,
    toggleMongoDbExpand,
    openCollectionTab,
    openMongoQueryTab,
  } = useQueryStore();

  const mongoState = mongoStates[tabId];
  const [loadingDbs, setLoadingDbs] = useState(false);
  const [loadingCollections, setLoadingCollections] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState("");
  const [showFilter, setShowFilter] = useState(false);
  const [selected, setSelected] = useState<{ db: string; collection: string } | null>(null);

  // Auto-load only when nothing cached; restored tabs already have databases.
  useEffect(() => {
    if (!mongoState) return;
    if (mongoState.databases.length > 0) return;
    setLoadingDbs(true);
    loadMongoDatabases(tabId).finally(() => setLoadingDbs(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId, assetId]);

  const filterLower = filter.trim().toLowerCase();

  const visibleDbs = useMemo(() => {
    if (!mongoState) return [];
    if (!filterLower) {
      return mongoState.databases.map((db) => ({
        db,
        dbMatch: false,
        collections: mongoState.collections[db],
      }));
    }
    const out: { db: string; dbMatch: boolean; collections: string[] | undefined }[] = [];
    for (const db of mongoState.databases) {
      const dbMatch = db.toLowerCase().includes(filterLower);
      const loaded = mongoState.collections[db];
      const matched = loaded?.filter((c) => c.toLowerCase().includes(filterLower));
      if (dbMatch) {
        out.push({ db, dbMatch: true, collections: loaded });
      } else if (matched && matched.length > 0) {
        out.push({ db, dbMatch: false, collections: matched });
      }
    }
    return out;
  }, [mongoState, filterLower]);

  if (!mongoState) return null;

  const { expandedDbs } = mongoState;

  const handleToggle = (db: string) => {
    const willExpand = !expandedDbs.includes(db);
    if (willExpand && !mongoState.collections[db]) {
      setLoadingCollections((prev) => new Set(prev).add(db));
      loadMongoCollections(tabId, db).finally(() => {
        setLoadingCollections((prev) => {
          const next = new Set(prev);
          next.delete(db);
          return next;
        });
      });
    }
    toggleMongoDbExpand(tabId, db);
  };

  const handleLoad = () => {
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
            onClick={() => {
              setShowFilter((v) => {
                if (v) setFilter("");
                return !v;
              });
            }}
            title={t("query.filterCollections")}
          >
            <Search className={`h-3.5 w-3.5 ${showFilter ? "text-foreground" : ""}`} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => openMongoQueryTab(tabId, selected?.db, selected?.collection)}
            title={t("query.newSql")}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleLoad} title={t("query.refreshTree")}>
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Filter input */}
      {showFilter && (
        <div className="border-b px-2 py-1.5 shrink-0">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
            <Input
              autoFocus
              className="h-7 pl-7 text-xs"
              placeholder={t("query.filterCollections")}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setFilter("");
                  setShowFilter(false);
                }
              }}
            />
          </div>
        </div>
      )}

      {/* Tree */}
      <ScrollArea className="flex-1 min-h-0">
        <div className="p-1 space-y-0.5">
          {loadingDbs ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : visibleDbs.length === 0 ? (
            <div className="text-xs text-muted-foreground text-center py-4">
              {filterLower ? t("query.noMatch") : t("query.databases")}
            </div>
          ) : (
            visibleDbs.map(({ db, dbMatch, collections: dbCollections }) => {
              const isExpanded = filterLower ? true : expandedDbs.includes(db);

              return (
                <div key={db}>
                  {/* Database node */}
                  <div
                    className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                    onClick={() => {
                      if (filterLower) return;
                      handleToggle(db);
                    }}
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
                          {filterLower && !dbMatch ? t("query.noMatch") : t("query.mongoCollections")}
                        </div>
                      ) : (
                        dbCollections.map((col) => {
                          const isSelected = selected?.db === db && selected?.collection === col;
                          return (
                            <div
                              key={col}
                              className={`group flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer transition-colors duration-150 ${
                                isSelected ? "bg-accent text-accent-foreground" : "hover:bg-accent"
                              }`}
                              onClick={() => setSelected({ db, collection: col })}
                              onDoubleClick={() => {
                                setSelected({ db, collection: col });
                                openCollectionTab(tabId, db, col);
                              }}
                            >
                              <Table2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                              <span className="flex-1 truncate">{col}</span>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-5 w-5 opacity-0 group-hover:opacity-100 transition-opacity"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setSelected({ db, collection: col });
                                  openMongoQueryTab(tabId, db, col);
                                }}
                                title={t("query.newSql")}
                              >
                                <Code2 className="h-3 w-3" />
                              </Button>
                            </div>
                          );
                        })
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
