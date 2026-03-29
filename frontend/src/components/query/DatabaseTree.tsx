import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import {
  ChevronRight,
  ChevronDown,
  Database,
  Table2,
  Plus,
  RefreshCw,
  Loader2,
  AlertCircle,
  Search,
} from "lucide-react";
import {
  Button,
  ScrollArea,
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";

interface DatabaseTreeProps {
  tabId: string;
}

function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name}"`;
  return `\`${name}\``;
}

export function DatabaseTree({ tabId }: DatabaseTreeProps) {
  const { t } = useTranslation();
  const { dbStates, loadDatabases, toggleDbExpand, openTableTab, openSqlTab, refreshTables } = useQueryStore();

  const tab = useTabStore((s) => s.tabs.find((t) => t.id === tabId));
  const driver = (tab?.meta as QueryTabMeta | undefined)?.driver;

  const dbState = dbStates[tabId];

  useEffect(() => {
    loadDatabases(tabId);
  }, [tabId, loadDatabases]);

  if (!dbState) return null;

  const { databases, tables, expandedDbs, loadingDbs, error } = dbState;

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
            onClick={() => openSqlTab(tabId)}
            title={t("query.newSql")}
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
          ) : databases.length === 0 ? (
            <div className="text-xs text-muted-foreground text-center py-4">{t("query.databases")}</div>
          ) : (
            databases.map((db) => {
              const isExpanded = expandedDbs.has(db);
              const dbTables = tables[db];

              return (
                <div key={db}>
                  {/* Database node with context menu */}
                  <ContextMenu>
                    <ContextMenuTrigger className="block w-full">
                      <div
                        className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                        onClick={() => toggleDbExpand(tabId, db)}
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
                        <div className="px-2 py-1 text-xs text-muted-foreground italic">{t("query.noTables")}</div>
                      ) : (
                        dbTables.map((tbl) => (
                          <ContextMenu key={tbl}>
                            <ContextMenuTrigger className="block w-full">
                              <div
                                className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs cursor-pointer hover:bg-accent transition-colors duration-150"
                                onClick={() => openTableTab(tabId, db, tbl)}
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
                                  const tableName =
                                    driver === "postgresql"
                                      ? `"${tbl}"`
                                      : `${quoteIdent(db, driver)}.${quoteIdent(tbl, driver)}`;
                                  openSqlTab(tabId, db, `SELECT * FROM ${tableName} LIMIT 100`);
                                }}
                              >
                                <Search className="h-3.5 w-3.5" />
                                {t("query.newSql")}
                              </ContextMenuItem>
                            </ContextMenuContent>
                          </ContextMenu>
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
