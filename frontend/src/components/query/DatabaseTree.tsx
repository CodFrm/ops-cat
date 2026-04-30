import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ChevronRight,
  ChevronDown,
  Database,
  Table2,
  SquarePen,
  RefreshCw,
  Loader2,
  AlertCircle,
  Search,
  Plus,
  Wrench,
} from "lucide-react";
import {
  Button,
  Input,
  ScrollArea,
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { CreateDatabaseDialog } from "./CreateDatabaseDialog";
import { CreateTableDialog } from "./CreateTableDialog";
import { AlterTableDialog } from "./AlterTableDialog";
import { quoteTableRef } from "@/lib/tableSql";

interface DatabaseTreeProps {
  tabId: string;
}

export function DatabaseTree({ tabId }: DatabaseTreeProps) {
  const { t } = useTranslation();
  const { dbStates, loadDatabases, toggleDbExpand, openTableTab, openSqlTab, refreshTables } = useQueryStore();
  const [showCreateDatabase, setShowCreateDatabase] = useState(false);
  const [showCreateTable, setShowCreateTable] = useState(false);
  const [createTableDatabase, setCreateTableDatabase] = useState("");
  const [showAlterTable, setShowAlterTable] = useState(false);
  const [alterDatabase, setAlterDatabase] = useState("");
  const [alterTableName, setAlterTableName] = useState("");

  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const driver = tabMeta?.driver;
  const defaultDatabase = tabMeta?.defaultDatabase ?? "";

  const dbState = dbStates[tabId];
  const [filter, setFilter] = useState("");
  const [showFilter, setShowFilter] = useState(false);
  const [selected, setSelected] = useState<{ db: string; table: string } | null>(null);

  // Auto-load only when there's nothing cached. Restored tabs come in with
  // databases/tables already populated from localStorage, so we skip the
  // refetch and rely on the user's refresh button.
  useEffect(() => {
    if (dbState && dbState.databases.length === 0 && !dbState.loadingDbs) {
      loadDatabases(tabId);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId]);

  const filterLower = filter.trim().toLowerCase();

  const visibleDbs = useMemo(() => {
    if (!dbState) return [];
    if (!filterLower) {
      return dbState.databases.map((db) => ({ db, dbMatch: false, tables: dbState.tables[db] }));
    }
    const out: { db: string; dbMatch: boolean; tables: string[] | undefined }[] = [];
    for (const db of dbState.databases) {
      const dbMatch = db.toLowerCase().includes(filterLower);
      const loaded = dbState.tables[db];
      const matchedTables = loaded?.filter((t) => t.toLowerCase().includes(filterLower));
      if (dbMatch) {
        out.push({ db, dbMatch: true, tables: loaded });
      } else if (matchedTables && matchedTables.length > 0) {
        out.push({ db, dbMatch: false, tables: matchedTables });
      }
    }
    return out;
  }, [dbState, filterLower]);

  if (!dbState) return null;

  const { expandedDbs, loadingDbs, error } = dbState;

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-2 py-1.5 border-b border-border shrink-0">
        <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("query.databases")}
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
            title={t("query.filterTables")}
          >
            <Search className={`h-3.5 w-3.5 ${showFilter ? "text-foreground" : ""}`} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => openSqlTab(tabId)}
            title={t("query.newSql")}
            aria-label={t("query.newSql")}
          >
            <SquarePen className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => setShowCreateDatabase(true)}
            title={t("query.createDatabase")}
            aria-label={t("query.createDatabase")}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => loadDatabases(tabId)}
            title={t("query.refreshTree")}
          >
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
              placeholder={t("query.filterTables")}
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

      {/* Error message */}
      {error && (
        <div className="flex items-start gap-2 border-b border-destructive/20 bg-destructive/10 px-2 py-2 text-xs text-destructive">
          <AlertCircle className="size-3.5 shrink-0 mt-0.5" />
          <span className="break-all">{error}</span>
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
            visibleDbs.map(({ db, dbMatch, tables: dbTables }) => {
              const isExpanded = filterLower ? true : expandedDbs.includes(db);

              return (
                <div key={db}>
                  {/* Database node with context menu */}
                  <ContextMenu>
                    <ContextMenuTrigger className="block w-full">
                      <div
                        className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                        onClick={() => {
                          if (filterLower) return;
                          toggleDbExpand(tabId, db);
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
                    </ContextMenuTrigger>
                    <ContextMenuContent>
                      <ContextMenuItem onClick={() => openSqlTab(tabId, db)}>
                        <Search className="h-3.5 w-3.5" />
                        {t("query.newSql")}
                      </ContextMenuItem>
                      <ContextMenuItem
                        onClick={() => {
                          setCreateTableDatabase(db);
                          setShowCreateTable(true);
                        }}
                      >
                        <Table2 className="h-3.5 w-3.5" />
                        {t("query.addTable")}
                      </ContextMenuItem>
                      <ContextMenuSeparator />
                      <ContextMenuItem onClick={() => refreshTables(tabId, db)}>
                        <RefreshCw className="h-3.5 w-3.5" />
                        {t("query.refreshTables")}
                      </ContextMenuItem>
                    </ContextMenuContent>
                  </ContextMenu>

                  {/* Tables */}
                  {isExpanded && (
                    <div className="ml-3">
                      {!dbTables ? (
                        <div className="flex items-center gap-1.5 px-2 py-1">
                          <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                        </div>
                      ) : dbTables.length === 0 ? (
                        <div className="px-2 py-1 text-xs text-muted-foreground italic">
                          {filterLower && !dbMatch ? t("query.noMatch") : t("query.noTables")}
                        </div>
                      ) : (
                        dbTables.map((tbl) => {
                          const isSelected = selected?.db === db && selected?.table === tbl;
                          return (
                            <ContextMenu key={tbl}>
                              <ContextMenuTrigger className="block w-full">
                                <div
                                  className={`flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer transition-colors duration-150 ${
                                    isSelected ? "bg-accent text-accent-foreground" : "hover:bg-accent"
                                  }`}
                                  onClick={() => setSelected({ db, table: tbl })}
                                  onDoubleClick={() => {
                                    setSelected({ db, table: tbl });
                                    openTableTab(tabId, db, tbl);
                                  }}
                                >
                                  <Table2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                                  <span className="truncate">{tbl}</span>
                                </div>
                              </ContextMenuTrigger>
                              <ContextMenuContent>
                                <ContextMenuItem onClick={() => openTableTab(tabId, db, tbl)}>
                                  <Table2 className="h-3.5 w-3.5" />
                                  {t("query.openTable")}
                                </ContextMenuItem>
                                <ContextMenuItem
                                  onClick={() => {
                                    setAlterDatabase(db);
                                    setAlterTableName(tbl);
                                    setShowAlterTable(true);
                                  }}
                                >
                                  <Wrench className="h-3.5 w-3.5" />
                                  {t("query.alterTable")}
                                </ContextMenuItem>
                                <ContextMenuItem
                                  onClick={() => {
                                    const tableName = quoteTableRef(db, tbl, driver);
                                    openSqlTab(tabId, db, `SELECT * FROM ${tableName} LIMIT 100`);
                                  }}
                                >
                                  <Search className="h-3.5 w-3.5" />
                                  {t("query.newSql")}
                                </ContextMenuItem>
                              </ContextMenuContent>
                            </ContextMenu>
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

      <CreateDatabaseDialog
        open={showCreateDatabase}
        onOpenChange={setShowCreateDatabase}
        assetId={tabMeta?.assetId ?? 0}
        defaultDatabase={defaultDatabase}
        driver={driver}
        onSuccess={() => loadDatabases(tabId)}
      />

      <CreateTableDialog
        open={showCreateTable}
        onOpenChange={(open) => {
          setShowCreateTable(open);
          if (!open) setCreateTableDatabase("");
        }}
        assetId={tabMeta?.assetId ?? 0}
        database={createTableDatabase || defaultDatabase}
        driver={driver}
        onSuccess={() => {
          const targetDb = createTableDatabase || defaultDatabase;
          if (targetDb) {
            refreshTables(tabId, targetDb);
          }
          setShowCreateTable(false);
          setCreateTableDatabase("");
        }}
      />

      <AlterTableDialog
        open={showAlterTable}
        onOpenChange={(open) => {
          setShowAlterTable(open);
          if (!open) {
            setAlterDatabase("");
            setAlterTableName("");
          }
        }}
        assetId={tabMeta?.assetId ?? 0}
        database={alterDatabase || defaultDatabase}
        table={alterTableName}
        driver={driver}
        onSuccess={(nextTableName) => {
          const targetDb = alterDatabase || defaultDatabase;
          if (targetDb) {
            refreshTables(tabId, targetDb);
          }
          if (nextTableName && targetDb) {
            openTableTab(tabId, targetDb, nextTableName);
          }
          setShowAlterTable(false);
          setAlterDatabase("");
          setAlterTableName("");
        }}
      />
    </div>
  );
}
