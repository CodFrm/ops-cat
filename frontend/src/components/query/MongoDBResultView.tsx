import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, ChevronsLeft, RefreshCw, Loader2 } from "lucide-react";
import { Button, Input, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@opskat/ui";
import { QueryResultTable } from "./QueryResultTable";
import { CodeEditor } from "@/components/CodeEditor";

interface MongoDBResultViewProps {
  data: string;
  loading?: boolean;
  skip?: number;
  limit?: number;
  // When provided, enables pagination footer (prev/next/page input/first).
  onPageChange?: (skip: number) => void;
  // When provided, renders a page-size selector in the footer.
  onPageSizeChange?: (size: number) => void;
  // When provided, renders a refresh button that re-invokes the current query.
  onRefresh?: () => void;
  // Shortcut label appended to the refresh button's tooltip, e.g. "⌘R".
  refreshShortcutLabel?: string;
}

type ViewMode = "table" | "json";

const PAGE_SIZES = [20, 50, 100, 200];

export function MongoDBResultView({
  data,
  loading,
  skip = 0,
  limit = 100,
  onPageChange,
  onPageSizeChange,
  onRefresh,
  refreshShortcutLabel,
}: MongoDBResultViewProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<ViewMode>("table");

  const parsed = useMemo(() => {
    if (!data) return null;
    try {
      return JSON.parse(data);
    } catch {
      return null;
    }
  }, [data]);

  const documents: Record<string, unknown>[] = useMemo(() => {
    if (!parsed) return [];
    if (Array.isArray(parsed)) return parsed;
    if (parsed.documents && Array.isArray(parsed.documents)) return parsed.documents;
    if (parsed.result && Array.isArray(parsed.result)) return parsed.result;
    if (typeof parsed === "object" && parsed !== null && !parsed.error) return [parsed];
    return [];
  }, [parsed]);

  const columns = useMemo(() => {
    const keySet = new Set<string>();
    for (const doc of documents) {
      if (doc && typeof doc === "object") {
        for (const key of Object.keys(doc)) keySet.add(key);
      }
    }
    const keys = Array.from(keySet);
    const idIdx = keys.indexOf("_id");
    if (idIdx > 0) {
      keys.splice(idIdx, 1);
      keys.unshift("_id");
    }
    return keys;
  }, [documents]);

  const error: string | undefined = parsed && parsed.error ? String(parsed.error) : undefined;

  const currentPage = Math.floor(skip / limit) + 1;
  const [pageInput, setPageInput] = useState(String(currentPage));
  useEffect(() => {
    setPageInput(String(currentPage));
  }, [currentPage]);

  const hasPrev = skip > 0;
  // Total is unknown for MongoDB find — treat a short page as "no next".
  const hasNext = documents.length >= limit;

  const commitPageInput = () => {
    const n = Math.max(1, Math.floor(Number(pageInput) || 1));
    const nextSkip = (n - 1) * limit;
    if (nextSkip !== skip) onPageChange?.(nextSkip);
    else setPageInput(String(currentPage));
  };

  const showFooter = !!onPageChange || !!onRefresh || !!onPageSizeChange || documents.length > 0;

  return (
    <div className="flex flex-col h-full">
      {/* Top bar: view mode toggle */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-muted/20 shrink-0">
        <div className="flex border border-border rounded-md overflow-hidden">
          <button
            className={`px-2 py-0.5 text-xs transition-colors ${
              viewMode === "table" ? "bg-primary text-primary-foreground" : "hover:bg-muted"
            }`}
            onClick={() => setViewMode("table")}
          >
            {t("query.mongoTableView")}
          </button>
          <button
            className={`px-2 py-0.5 text-xs transition-colors ${
              viewMode === "json" ? "bg-primary text-primary-foreground" : "hover:bg-muted"
            }`}
            onClick={() => setViewMode("json")}
          >
            {t("query.mongoJsonView")}
          </button>
        </div>
      </div>

      {/* Table / JSON content */}
      <div className="flex-1 min-h-0 flex flex-col">
        {viewMode === "table" ? (
          <QueryResultTable
            columns={columns}
            rows={documents}
            loading={loading}
            error={error}
            showRowNumber
            rowNumberOffset={skip}
            enableColumnFilter
            renderCell={renderMongoCell}
          />
        ) : (
          <div className="flex-1 min-h-0 bg-background">
            <CodeEditor value={JSON.stringify(documents, null, 2)} language="json" readOnly />
          </div>
        )}
      </div>

      {/* Footer bar: doc count + refresh + pagination */}
      {showFooter && (
        <div className="flex items-center gap-2 px-3 py-1.5 border-t border-border bg-muted/30 shrink-0">
          <span className="text-xs text-muted-foreground">{t("query.mongoDocCount", { count: documents.length })}</span>
          {onRefresh && (
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              onClick={onRefresh}
              disabled={loading}
              title={
                refreshShortcutLabel ? `${t("query.refreshTable")} (${refreshShortcutLabel})` : t("query.refreshTable")
              }
            >
              {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            </Button>
          )}
          {onPageChange && (
            <div className="ml-auto flex items-center gap-1">
              {onPageSizeChange && (
                <Select value={String(limit)} onValueChange={(v) => onPageSizeChange(Number(v))}>
                  <SelectTrigger size="sm" className="h-6 w-[90px] text-xs">
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
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                disabled={!hasPrev || loading}
                onClick={() => onPageChange(0)}
                title={t("query.firstPage")}
              >
                <ChevronsLeft className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                disabled={!hasPrev || loading}
                onClick={() => onPageChange(Math.max(0, skip - limit))}
                title={t("query.prevPage")}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
              </Button>
              <Input
                className="h-6 w-[48px] text-xs text-center px-1"
                value={pageInput}
                onChange={(e) => setPageInput(e.target.value)}
                onBlur={commitPageInput}
                onKeyDown={(e) => {
                  if (e.key === "Enter") commitPageInput();
                }}
              />
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                disabled={!hasNext || loading}
                onClick={() => onPageChange(skip + limit)}
                title={t("query.nextPage")}
              >
                <ChevronRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// Primitives render as strings; objects/arrays get a truncated single-line JSON
// preview. Full value is available via hover title and right-click → copy
// (handled by QueryResultTable).
function renderMongoCell(value: unknown): React.ReactNode {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic">null</span>;
  }
  if (typeof value === "object") {
    const json = safeStringify(value);
    const truncated = json.length > 80 ? json.slice(0, 80) + "…" : json;
    return <span className="text-muted-foreground truncate block">{truncated}</span>;
  }
  return <span className="truncate block">{String(value)}</span>;
}

function safeStringify(v: unknown): string {
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
}
