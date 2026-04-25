import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Download,
  Eye,
  Loader2,
  Plus,
  RefreshCw,
  Save,
  Settings2,
  Square,
  Trash2,
  Undo2,
  Upload,
} from "lucide-react";
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

export type TableExportFormat = "csv" | "tsv" | "sql";

interface TableEditorToolbarProps {
  hasEdits: boolean;
  hasSelectedRow: boolean;
  loading: boolean;
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
  onAddRow: () => void;
  onDeleteRow: () => void;
  onSubmit: () => void;
  onDiscard: () => void;
  onRefresh: () => void;
  onStopLoading: () => void;
  onImport: () => void;
  onExport: () => void;
  onPreviewSql: () => void;
}

export function TableEditorToolbar({
  hasEdits,
  hasSelectedRow,
  loading,
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
  onAddRow,
  onDeleteRow,
  onSubmit,
  onDiscard,
  onRefresh,
  onStopLoading,
  onImport,
  onExport,
  onPreviewSql,
}: TableEditorToolbarProps) {
  const { t } = useTranslation();
  const editActionDisabled = !hasEdits || submitting;

  return (
    <div className="flex items-center gap-1.5 shrink-0">
      <Button variant="ghost" size="icon-xs" title={t("query.addRow")} onClick={onAddRow}>
        <Plus className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("query.deleteRecord")}
        onClick={onDeleteRow}
        disabled={!hasSelectedRow || submitting}
      >
        <Trash2 className="h-3.5 w-3.5" />
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
      <Button variant="ghost" size="icon-xs" title={t("query.refreshTable")} onClick={onRefresh} disabled={loading}>
        <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
      </Button>
      <Button variant="ghost" size="icon-xs" title={t("query.stopLoading")} onClick={onStopLoading} disabled={!loading}>
        <Square className="h-3.5 w-3.5" />
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
  pageInput: string;
  pageSize: number;
  pageSizes: number[];
  hasPrev: boolean;
  hasNext: boolean;
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
}

export function TableDataStatusBar({
  pendingEditCount,
  sqlSummary,
  totalRows,
  page,
  totalPages,
  pageInput,
  pageSize,
  pageSizes,
  hasPrev,
  hasNext,
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
}: TableDataStatusBarProps) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 border-t border-border bg-muted/30 shrink-0">
      <span className="text-xs text-muted-foreground whitespace-nowrap">
        {t("query.pendingEdits", { count: pendingEditCount })}
      </span>
      {sqlSummary && (
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground" title={sqlSummary}>
          {sqlSummary}
        </span>
      )}
      {!sqlSummary && totalRows != null && (
        <span className="text-xs text-muted-foreground">{t("query.totalRows", { count: totalRows })}</span>
      )}
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={onRefresh}
        disabled={loading}
        title={refreshTitle}
        className="ml-auto"
      >
        <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
      </Button>
      <Button variant="ghost" size="icon-xs" onClick={onStopLoading} disabled={!loading} title={t("query.stopLoading")}>
        <Square className="h-3.5 w-3.5" />
      </Button>
      <Select value={String(pageSize)} onValueChange={(value) => onPageSizeChange(Number(value))}>
        <SelectTrigger size="sm" className="h-6 w-[80px] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {pageSizes.map((size) => (
            <SelectItem key={size} value={String(size)} className="text-xs">
              {t("query.perPage", { count: size })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant="ghost"
        size="icon-xs"
        disabled={!hasPrev || loading}
        onClick={onFirstPage}
        title={t("query.firstPage")}
      >
        <ChevronsLeft className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
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
      {totalPages != null && <span className="text-xs text-muted-foreground whitespace-nowrap">/ {totalPages}</span>}
      <Button
        variant="ghost"
        size="icon-xs"
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
          disabled={!hasNext || loading}
          onClick={onLastPage}
          title={t("query.lastPage")}
        >
          <ChevronsRight className="h-3.5 w-3.5" />
        </Button>
      )}
      {totalRows != null && sqlSummary && (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {t("query.totalRows", { count: totalRows })}
        </span>
      )}
      {totalRows == null && !sqlSummary && (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {t("query.pageNumber", { page: page + 1 })}
        </span>
      )}
    </div>
  );
}
