import { useTranslation } from "react-i18next";
import { X, Table2, Code2, Database } from "lucide-react";
import { useQueryStore } from "@/stores/queryStore";
import { useResizeHandle } from "@opskat/ui";
import { DatabaseTree } from "./DatabaseTree";
import { TableDataTab } from "./TableDataTab";
import { SqlEditorTab } from "./SqlEditorTab";

interface DatabasePanelProps {
  tabId: string;
}

export function DatabasePanel({ tabId }: DatabasePanelProps) {
  const { t } = useTranslation();
  const { dbStates, closeInnerTab, setActiveInnerTab } = useQueryStore();
  const dbState = dbStates[tabId];

  const { width: sidebarWidth, handleMouseDown } = useResizeHandle({
    defaultWidth: 200,
    minWidth: 140,
    maxWidth: 400,
  });

  if (!dbState) return null;

  const { innerTabs, activeInnerTabId } = dbState;

  return (
    <div className="flex h-full w-full">
      {/* Left sidebar: Database tree */}
      <div
        className="shrink-0 border-r border-border bg-sidebar h-full overflow-hidden"
        style={{ width: sidebarWidth }}
      >
        <DatabaseTree tabId={tabId} />
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
            {innerTabs.map((tab) => {
              const isActive = tab.id === activeInnerTabId;
              return (
                <div
                  key={tab.id}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs cursor-pointer border-r border-border whitespace-nowrap select-none transition-colors duration-150 ${
                    isActive
                      ? "bg-background text-foreground"
                      : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
                  }`}
                  onClick={() => setActiveInnerTab(tabId, tab.id)}
                >
                  {tab.type === "table" ? (
                    <Table2 className="h-3 w-3 shrink-0" />
                  ) : (
                    <Code2 className="h-3 w-3 shrink-0" />
                  )}
                  <span className="truncate max-w-[120px]">
                    {tab.type === "table" ? `${tab.database}.${tab.table}` : tab.title}
                  </span>
                  <button
                    className="ml-1 rounded-sm p-0.5 hover:bg-muted transition-colors"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeInnerTab(tabId, tab.id);
                    }}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {/* Tab content — render all, hide inactive to preserve state */}
        <div className="flex-1 min-h-0 relative">
          {innerTabs.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full gap-3 text-muted-foreground">
              <Database className="h-10 w-10 opacity-30" />
              <p className="text-xs">{t("query.openTable")}</p>
            </div>
          )}
          {innerTabs.map((tab) => {
            const isActive = tab.id === activeInnerTabId;
            return (
              <div
                key={tab.id}
                className="absolute inset-0 [contain:paint]"
                style={{
                  visibility: isActive ? "visible" : "hidden",
                  pointerEvents: isActive ? "auto" : "none",
                }}
              >
                {tab.type === "table" ? (
                  <TableDataTab tabId={tabId} database={tab.database} table={tab.table} />
                ) : (
                  <SqlEditorTab tabId={tabId} innerTabId={tab.id} />
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
