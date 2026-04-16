import { useState, useMemo, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Loader2, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@opskat/ui";

interface MongoDBResultViewProps {
  data: string;
  loading?: boolean;
  skip?: number;
  limit?: number;
  onPageChange?: (skip: number) => void;
}

export function MongoDBResultView({ data, loading, skip = 0, limit = 20, onPageChange }: MongoDBResultViewProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<"table" | "json">("table");
  const [expandedCells, setExpandedCells] = useState<Set<string>>(new Set());

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
    // Single document result
    if (typeof parsed === "object" && parsed !== null) return [parsed];
    return [];
  }, [parsed]);

  // Extract all unique top-level keys across documents for table columns
  const columns = useMemo(() => {
    const keySet = new Set<string>();
    for (const doc of documents) {
      for (const key of Object.keys(doc)) {
        keySet.add(key);
      }
    }
    // Put _id first if present
    const keys = Array.from(keySet);
    const idIdx = keys.indexOf("_id");
    if (idIdx > 0) {
      keys.splice(idIdx, 1);
      keys.unshift("_id");
    }
    return keys;
  }, [documents]);

  const toggleCell = useCallback((key: string) => {
    setExpandedCells((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const formatCellValue = (value: unknown, cellKey: string): React.ReactNode => {
    if (value === null || value === undefined) {
      return <span className="text-muted-foreground italic">null</span>;
    }
    if (typeof value === "object") {
      const json = JSON.stringify(value);
      const isExpanded = expandedCells.has(cellKey);
      if (isExpanded) {
        return (
          <pre
            className="text-xs whitespace-pre-wrap break-all cursor-pointer max-w-[500px]"
            onClick={() => toggleCell(cellKey)}
          >
            {JSON.stringify(value, null, 2)}
          </pre>
        );
      }
      const truncated = json.length > 80 ? json.slice(0, 80) + "..." : json;
      return (
        <span
          className="cursor-pointer hover:text-primary truncate block max-w-[300px]"
          onClick={() => toggleCell(cellKey)}
        >
          {truncated}
        </span>
      );
    }
    return <span>{String(value)}</span>;
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!data && !loading) {
    return (
      <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">{t("query.noResult")}</div>
    );
  }

  // Error result
  if (parsed && parsed.error) {
    return <div className="px-3 py-4 text-xs text-destructive whitespace-pre-wrap font-mono">{parsed.error}</div>;
  }

  const currentPage = Math.floor(skip / limit) + 1;

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar: view mode toggle + pagination */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-border bg-muted/30 shrink-0">
        <div className="flex items-center gap-2">
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
          <span className="text-xs text-muted-foreground">{t("query.mongoDocCount", { count: documents.length })}</span>
        </div>
        {onPageChange && (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              disabled={skip === 0}
              onClick={() => onPageChange(Math.max(0, skip - limit))}
            >
              <ChevronLeft className="h-3.5 w-3.5" />
            </Button>
            <span className="text-xs text-muted-foreground">{t("query.page", { page: currentPage })}</span>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              disabled={documents.length < limit}
              onClick={() => onPageChange(skip + limit)}
            >
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
          </div>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 overflow-auto">
        {viewMode === "table" ? (
          documents.length === 0 ? (
            <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
              {t("query.noResult")}
            </div>
          ) : (
            <table className="border-collapse text-xs font-mono w-full">
              <thead className="bg-muted sticky top-0 z-10">
                <tr>
                  {columns.map((col) => (
                    <th
                      key={col}
                      className="border border-border px-2 py-1.5 text-left font-semibold text-muted-foreground whitespace-nowrap"
                    >
                      {col}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {documents.map((doc, rowIdx) => (
                  <tr key={rowIdx} className={rowIdx % 2 === 0 ? "bg-background" : "bg-muted/40"}>
                    {columns.map((col) => (
                      <td
                        key={col}
                        className="border border-border px-2 py-1 whitespace-nowrap"
                        style={{ maxWidth: "400px" }}
                      >
                        {formatCellValue(doc[col], `${rowIdx}:${col}`)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          <pre className="p-3 text-xs font-mono whitespace-pre-wrap break-all">
            {JSON.stringify(documents, null, 2)}
          </pre>
        )}
      </div>
    </div>
  );
}
