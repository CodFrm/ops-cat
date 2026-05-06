import {
  Check,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Download,
  Eye,
  ListFilter,
  Loader2,
  Minus,
  Plus,
  RefreshCw,
  Save,
  Settings2,
  Square,
  Undo2,
  Upload,
  X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import type { RowDensity } from "./QueryResultTable";
import type { TableExportFormat } from "@/lib/tableExport";

export type { TableExportFormat };

interface TableEditorToolbarProps {
  hasEdits: boolean;
  submitting: boolean;
  canExport: boolean;
  canImport?: boolean;
  columns?: string[];
  visibleColumns?: string[];
  rowDensity?: RowDensity;
  exportFormat?: TableExportFormat;
  onExportFormatChange?: (format: TableExportFormat) => void;
  onVisibleColumnToggle?: (column: string) => void;
  onRowDensityChange?: (density: RowDensity) => void;
  filterSortOpen: boolean;
  filterSortActive?: boolean;
  onToggleFilterSort: () => void;
  onSubmit: () => void;
  onDiscard: () => void;
  onImport: () => void;
  onExport: () => void;
  onPreviewSql: () => void;
}

export function TableEditorToolbar({
  hasEdits,
  submitting,
  canExport,
  canImport = false,
  columns = [],
  visibleColumns = columns,
  rowDensity = "default",
  exportFormat = "csv",
  onExportFormatChange,
  onVisibleColumnToggle,
  onRowDensityChange,
  filterSortOpen,
  filterSortActive = false,
  onToggleFilterSort,
  onSubmit,
  onDiscard,
  onImport,
  onExport,
  onPreviewSql,
}: TableEditorToolbarProps) {
  const { t } = useTranslation();
  const editActionDisabled = !hasEdits || submitting;

  return (
    <div className="flex items-center gap-1.5 shrink-0">
      <Button
        variant={filterSortOpen ? "secondary" : "ghost"}
        size="icon-xs"
        title={t("query.filterSort")}
        aria-pressed={filterSortOpen}
        className={filterSortActive && !filterSortOpen ? "text-primary" : undefined}
        onClick={onToggleFilterSort}
      >
        <ListFilter className="h-3.5 w-3.5" />
      </Button>
      <div className="mx-0.5 h-5 w-px bg-border" />
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("query.submitEdits")}
        onClick={onSubmit}
        disabled={editActionDisabled}
      >
        {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("query.discardEdits")}
        onClick={onDiscard}
        disabled={editActionDisabled}
      >
        <Undo2 className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("query.previewSql")}
        onClick={onPreviewSql}
        disabled={editActionDisabled}
      >
        <Eye className="h-3.5 w-3.5" />
      </Button>
      <div className="mx-0.5 h-5 w-px bg-border" />
      <Button variant="ghost" size="icon-xs" title={t("query.importData")} onClick={onImport} disabled={!canImport}>
        <Upload className="h-3.5 w-3.5" />
      </Button>
      <Select value={exportFormat} onValueChange={(value) => onExportFormatChange?.(value as TableExportFormat)}>
        <SelectTrigger size="sm" className="h-6 w-[74px] text-xs" title={t("query.exportFormat")} disabled={!canExport}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="csv" className="text-xs">
            CSV
          </SelectItem>
          <SelectItem value="tsv" className="text-xs">
            TSV
          </SelectItem>
          <SelectItem value="sql" className="text-xs">
            SQL
          </SelectItem>
        </SelectContent>
      </Select>
      <Button variant="ghost" size="icon-xs" title={t("query.exportData")} onClick={onExport} disabled={!canExport}>
        <Download className="h-3.5 w-3.5" />
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-xs" title={t("query.displaySettings")} disabled={columns.length === 0}>
            <Settings2 className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel className="text-xs">{t("query.visibleColumns")}</DropdownMenuLabel>
          {columns.map((column) => (
            <DropdownMenuCheckboxItem
              key={column}
              className="text-xs"
              checked={visibleColumns.includes(column)}
              onCheckedChange={() => onVisibleColumnToggle?.(column)}
              onSelect={(event) => event.preventDefault()}
              disabled={visibleColumns.length === 1 && visibleColumns.includes(column)}
            >
              <span className="truncate font-mono">{column}</span>
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-xs">{t("query.rowDensity")}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={rowDensity}
            onValueChange={(value) => onRowDensityChange?.(value as RowDensity)}
          >
            <DropdownMenuRadioItem value="compact" className="text-xs">
              {t("query.rowDensityCompact")}
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="default" className="text-xs">
              {t("query.rowDensityDefault")}
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="comfortable" className="text-xs">
              {t("query.rowDensityComfortable")}
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

interface TableDataStatusBarProps {
  pendingEditCount: number;
  sqlSummary: string;
  totalRows: number | null;
  page: number;
  totalPages: number | null;
  pageSize: number;
  pageInput: string;
  hasPrev: boolean;
  hasNext: boolean;
  hasSelectedRow: boolean;
  submitting: boolean;
  loading: boolean;
  refreshTitle: string;
  onRefresh: () => void;
  onStopLoading: () => void;
  onPageInputChange: (value: string) => void;
  onPageInputConfirm: () => void;
  onPageSizeChange: (value: number) => void;
  onFirstPage: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
  onLastPage: () => void;
  onAddRow: () => void;
  onDeleteRow: () => void;
  onApplyChanges: () => void;
  onDiscardChanges: () => void;
}

export function TableDataStatusBar({
  pendingEditCount,
  sqlSummary,
  totalRows,
  page,
  totalPages,
  pageSize,
  pageInput,
  hasPrev,
  hasNext,
  hasSelectedRow,
  submitting,
  loading,
  refreshTitle,
  onRefresh,
  onStopLoading,
  onPageInputChange,
  onPageInputConfirm,
  onPageSizeChange,
  onFirstPage,
  onPreviousPage,
  onNextPage,
  onLastPage,
  onAddRow,
  onDeleteRow,
  onApplyChanges,
  onDiscardChanges,
}: TableDataStatusBarProps) {
  const { t } = useTranslation();
  const hasPendingEdits = pendingEditCount > 0;
  const [pageSizeDraft, setPageSizeDraft] = useState(String(pageSize));

  useEffect(() => {
    setPageSizeDraft(String(pageSize));
  }, [pageSize]);

  const commitPageSize = () => {
    const next = Math.max(1, Math.min(100000, Math.floor(Number(pageSizeDraft))));
    if (!Number.isFinite(next)) {
      setPageSizeDraft(String(pageSize));
      return;
    }
    setPageSizeDraft(String(next));
    if (next !== pageSize) onPageSizeChange(next);
  };

  const actionTone = "text-foreground disabled:text-muted-foreground disabled:opacity-35";

  return (
    <div className="flex h-9 items-center gap-2 border-t border-border bg-muted/30 px-2 shrink-0">
      <div className="flex items-center gap-0.5 shrink-0">
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onAddRow}
          disabled={loading || submitting}
          title={t("query.addRow")}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onDeleteRow}
          disabled={!hasSelectedRow || loading || submitting}
          title={t("query.deleteRecord")}
        >
          <Minus className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onApplyChanges}
          disabled={!hasPendingEdits || loading || submitting}
          title={t("query.applyChanges")}
        >
          <Check className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onDiscardChanges}
          disabled={!hasPendingEdits || loading || submitting}
          title={t("query.discardChanges")}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onRefresh}
          disabled={loading}
          title={refreshTitle}
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          onClick={onStopLoading}
          disabled={!loading}
          title={t("query.stopLoading")}
        >
          <Square className="h-3.5 w-3.5" />
        </Button>
      </div>

      <div className="min-w-0 flex-1 text-center">
        {sqlSummary ? (
          <span className="block truncate font-mono text-[11px] text-foreground" title={sqlSummary}>
            {sqlSummary}
          </span>
        ) : totalRows != null ? (
          <span className="text-xs text-muted-foreground">
            {t("query.recordsInPage", { count: totalRows, page: page + 1 })}
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">{t("query.pageNumber", { page: page + 1 })}</span>
        )}
      </div>

      <div className="flex items-center gap-0.5 shrink-0">
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          disabled={!hasPrev || loading}
          onClick={onFirstPage}
          title={t("query.firstPage")}
        >
          <ChevronsLeft className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          disabled={!hasPrev || loading}
          onClick={onPreviousPage}
          title={t("query.prevPage")}
        >
          <ChevronLeft className="h-3.5 w-3.5" />
        </Button>
        <Input
          className="h-6 w-[48px] text-xs text-center px-1"
          value={pageInput}
          onChange={(e) => onPageInputChange(e.target.value)}
          onBlur={onPageInputConfirm}
          onKeyDown={(e) => {
            if (e.key === "Enter") onPageInputConfirm();
          }}
          aria-label={t("query.pageNumber")}
        />
        <Button
          variant="ghost"
          size="icon-xs"
          className={actionTone}
          disabled={!hasNext || loading}
          onClick={onNextPage}
          title={t("query.nextPage")}
        >
          <ChevronRight className="h-3.5 w-3.5" />
        </Button>
        {totalPages != null && (
          <Button
            variant="ghost"
            size="icon-xs"
            className={actionTone}
            disabled={!hasNext || loading}
            onClick={onLastPage}
            title={t("query.lastPage")}
          >
            <ChevronsRight className="h-3.5 w-3.5" />
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs" className={actionTone} title={t("query.tableFooterSettings")}>
              <Settings2 className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52 p-3">
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t("query.pageSize")}</span>
              <Input
                aria-label={t("query.pageSize")}
                className="h-7 text-xs"
                inputMode="numeric"
                value={pageSizeDraft}
                onChange={(event) => setPageSizeDraft(event.target.value)}
                onBlur={commitPageSize}
                onKeyDown={(event) => {
                  event.stopPropagation();
                  if (event.key === "Enter") commitPageSize();
                }}
              />
            </label>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
