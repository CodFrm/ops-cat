import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@opskat/ui";
import { Check, ChevronDown, Download, ExternalLink, FolderOpen, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { ExecuteSQL, OpenDirectory, SelectTableExportFile, WriteTableExportFile } from "../../../wailsjs/go/app/App";
import {
  buildTableExportContent,
  buildTableExportSelectSql,
  safeTableExportFilenamePart,
  type TableExportFormat,
  type TableExportOptions,
  type TableExportScope,
  type TableExportSortDir,
} from "@/lib/tableExport";

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
}

interface ExportTableDataDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  table: string;
  driver?: string;
  columns: string[];
  rows: Record<string, unknown>[];
  totalRows: number | null;
  page: number;
  pageSize: number;
  whereClause: string;
  orderByClause: string;
  sortColumn: string | null;
  sortDir: TableExportSortDir;
  initialFormat: TableExportFormat;
  onFormatChange: (format: TableExportFormat) => void;
}

const exportMeta: Record<TableExportFormat, { label: string; extension: string; filterName: string; pattern: string }> =
  {
    csv: { label: "CSV", extension: "csv", filterName: "CSV Files", pattern: "*.csv" },
    tsv: { label: "TSV", extension: "tsv", filterName: "Text Files", pattern: "*.tsv" },
    sql: { label: "SQL", extension: "sql", filterName: "SQL Files", pattern: "*.sql" },
  };

type TableExportEncoding = "utf-8" | "utf-8-bom" | "gb18030" | "gbk" | "big5" | "shift-jis" | "utf-16le";

const exportEncodings: Array<{ value: TableExportEncoding; label: string }> = [
  { value: "utf-8", label: "65001 - Unicode (UTF-8)" },
  { value: "utf-8-bom", label: "UTF-8 with BOM" },
  { value: "gb18030", label: "54936 - Chinese (GB18030)" },
  { value: "gbk", label: "936 - Chinese (GBK)" },
  { value: "big5", label: "950 - Chinese Traditional (Big5)" },
  { value: "shift-jis", label: "932 - Japanese (Shift JIS)" },
  { value: "utf-16le", label: "1200 - Unicode (UTF-16 LE)" },
];

const defaultExportOptions: TableExportOptions = {
  append: false,
  continueOnError: true,
  recordDelimiter: "lf",
  fieldDelimiter: "comma",
  textQualifier: "double",
  blankIfZero: false,
  zeroPaddingDate: true,
  dateOrder: "ymd",
  dateDelimiter: "-",
  timeDelimiter: ":",
  decimalSymbol: ".",
  binaryDataEncoding: "base64",
};

interface TableExportWriteOptions {
  encoding: TableExportEncoding;
  append: boolean;
}

async function writeTableExportFile(
  filePath: string,
  content: string,
  options: TableExportWriteOptions
): Promise<void> {
  const wailsWrite = (window as unknown as { go?: { app?: { App?: { WriteTableExportFile?: unknown } } } }).go?.app?.App
    ?.WriteTableExportFile;
  if (typeof wailsWrite === "function") {
    await wailsWrite(filePath, content, options);
    return;
  }
  await WriteTableExportFile(filePath, content);
}

function getContainingDirectory(filePath: string): string {
  const normalized = filePath.replace(/\\/g, "/");
  const slash = normalized.lastIndexOf("/");
  if (slash <= 0) return filePath;
  return filePath.slice(0, slash);
}

export function ExportTableDataDialog({
  open,
  onOpenChange,
  assetId,
  database,
  table,
  driver,
  columns,
  rows,
  totalRows,
  page,
  pageSize,
  whereClause,
  orderByClause,
  sortColumn,
  sortDir,
  initialFormat,
  onFormatChange,
}: ExportTableDataDialogProps) {
  const { t } = useTranslation();
  const [scope, setScope] = useState<TableExportScope>("all");
  const [format, setFormat] = useState<TableExportFormat>(initialFormat);
  const [selectedColumns, setSelectedColumns] = useState<string[]>(columns);
  const [includeHeaders, setIncludeHeaders] = useState(true);
  const [encoding, setEncoding] = useState<TableExportEncoding>("utf-8");
  const [exportOptions, setExportOptions] = useState<TableExportOptions>(defaultExportOptions);
  const [filePath, setFilePath] = useState("");
  const [exporting, setExporting] = useState(false);
  const [completed, setCompleted] = useState(false);
  const [logLines, setLogLines] = useState<string[]>([]);

  useEffect(() => {
    if (!open) return;
    setFormat(initialFormat);
    setSelectedColumns((prev) => {
      const retained = prev.filter((column) => columns.includes(column));
      return retained.length > 0 ? retained : columns;
    });
    setEncoding("utf-8");
    setExportOptions({
      ...defaultExportOptions,
      fieldDelimiter: initialFormat === "tsv" ? "tab" : "comma",
    });
    setFilePath("");
    setCompleted(false);
    setLogLines([]);
  }, [columns, initialFormat, open]);

  const meta = exportMeta[format];
  const defaultFilename = useMemo(() => {
    const baseName = `${safeTableExportFilenamePart(database)}_${safeTableExportFilenamePart(table)}`;
    return `${baseName}.${meta.extension}`;
  }, [database, meta.extension, table]);
  const estimatedRows = scope === "all" ? (totalRows ?? rows.length) : rows.length;
  const canStart = !!assetId && selectedColumns.length > 0 && !!filePath && !exporting;
  const tableName = driver === "postgresql" ? table : `${database}.${table}`;

  const appendLog = useCallback((line: string) => {
    setLogLines((prev) => [...prev, line]);
  }, []);

  const handleFormatChange = useCallback(
    (value: TableExportFormat) => {
      setFormat(value);
      onFormatChange(value);
      setFilePath("");
      setCompleted(false);
      setLogLines([]);
      setExportOptions((prev) => ({ ...prev, fieldDelimiter: value === "tsv" ? "tab" : "comma" }));
    },
    [onFormatChange]
  );

  const updateExportOption = useCallback(<K extends keyof TableExportOptions>(key: K, value: TableExportOptions[K]) => {
    setExportOptions((prev) => ({ ...prev, [key]: value }));
    setCompleted(false);
  }, []);

  const handleChooseFile = useCallback(async () => {
    try {
      const selected = await SelectTableExportFile(defaultFilename, meta.filterName, meta.pattern);
      if (selected) {
        setFilePath(selected);
        setCompleted(false);
        setLogLines([]);
      }
    } catch (e) {
      toast.error(String(e));
    }
  }, [defaultFilename, meta.filterName, meta.pattern]);

  const handleColumnToggle = useCallback(
    (column: string) => {
      setSelectedColumns((prev) => {
        if (prev.includes(column)) {
          if (prev.length === 1) return prev;
          return prev.filter((item) => item !== column);
        }
        return columns.filter((item) => item === column || prev.includes(item));
      });
      setCompleted(false);
    },
    [columns]
  );

  const handleStart = useCallback(async () => {
    if (!canStart) return;
    setExporting(true);
    setCompleted(false);
    setLogLines([]);
    const startedAt = performance.now();

    try {
      appendLog("[EXP] Export start");
      appendLog(`[EXP] Export Format - ${meta.label}`);
      appendLog(`[EXP] Encoding - ${encoding}`);
      appendLog(`[EXP] Export Scope - ${scope === "all" ? t("query.exportAllData") : t("query.exportPageData")}`);

      let exportRows = rows;
      if (scope === "all") {
        appendLog("[EXP] Getting all data ...");
        const sql = buildTableExportSelectSql({
          database,
          table,
          driver,
          scope,
          whereClause,
          orderByClause,
          sortColumn,
          sortDir,
          page,
          pageSize,
        });
        const result = await ExecuteSQL(assetId, sql, database);
        const parsed = JSON.parse(result || "{}") as SQLResult;
        exportRows = parsed.rows ?? [];
      } else {
        appendLog("[EXP] Getting current page data ...");
      }

      const content = buildTableExportContent({
        format,
        columns: selectedColumns,
        rows: exportRows,
        tableName,
        driver,
        includeHeaders,
        options: exportOptions,
      });
      appendLog(`[EXP] Export table [${table}]`);
      appendLog(`[EXP] Export to - ${filePath}`);
      await writeTableExportFile(filePath, content, { encoding, append: !!exportOptions.append });

      const elapsed = ((performance.now() - startedAt) / 1000).toFixed(3);
      appendLog(`[EXP] Processed ${exportRows.length} row(s) in ${elapsed}s`);
      appendLog("[EXP] Finished successfully");
      setCompleted(true);
      toast.success(t("query.exportSuccessDetailed", { count: exportRows.length }));
    } catch (e) {
      appendLog(`[EXP] Failed - ${String(e)}`);
      toast.error(String(e));
    } finally {
      setExporting(false);
    }
  }, [
    appendLog,
    assetId,
    canStart,
    database,
    driver,
    encoding,
    exportOptions,
    filePath,
    format,
    includeHeaders,
    meta.label,
    orderByClause,
    page,
    pageSize,
    rows,
    scope,
    selectedColumns,
    sortColumn,
    sortDir,
    table,
    tableName,
    t,
    whereClause,
  ]);

  const handleOpenExportFile = useCallback(async () => {
    if (!filePath) return;
    try {
      await OpenDirectory(filePath);
    } catch (e) {
      toast.error(String(e));
    }
  }, [filePath]);

  const handleOpenExportFolder = useCallback(async () => {
    if (!filePath) return;
    try {
      await OpenDirectory(getContainingDirectory(filePath));
    } catch (e) {
      toast.error(String(e));
    }
  }, [filePath]);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!exporting) onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-w-3xl max-h-[85vh] overflow-y-auto" showCloseButton={!exporting}>
        <DialogHeader>
          <DialogTitle>{t("query.exportDialogTitle")}</DialogTitle>
          <DialogDescription>{t("query.exportDialogDesc", { table })}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label className="text-xs">{t("query.exportDataScope")}</Label>
              <Select value={scope} onValueChange={(value) => setScope(value as TableExportScope)}>
                <SelectTrigger size="sm" className="h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all" className="text-xs">
                    {t("query.exportAllData")}
                  </SelectItem>
                  <SelectItem value="page" className="text-xs">
                    {t("query.exportPageData")}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label className="text-xs">{t("query.exportFormat")}</Label>
              <Select value={format} onValueChange={(value) => handleFormatChange(value as TableExportFormat)}>
                <SelectTrigger size="sm" className="h-8 text-xs">
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
            </div>
          </div>

          <div className="grid gap-2">
            <div className="flex items-center justify-between gap-2">
              <Label className="text-xs">{t("query.exportFile")}</Label>
              <Button
                variant="outline"
                size="sm"
                className="h-7 gap-1 text-xs"
                onClick={handleChooseFile}
                disabled={exporting}
              >
                <FolderOpen className="h-3.5 w-3.5" />
                {t("query.exportChooseFile")}
              </Button>
            </div>
            <div className="min-h-8 rounded-md border border-input bg-muted/30 px-3 py-2 font-mono text-xs text-muted-foreground">
              {filePath || t("query.exportNoFileSelected")}
            </div>
          </div>

          <div className="grid gap-3 rounded-md border border-border bg-muted/10 p-3">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportEncoding")}</Label>
                <Select value={encoding} onValueChange={(value) => setEncoding(value as TableExportEncoding)}>
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {exportEncodings.map((item) => (
                      <SelectItem key={item.value} value={item.value} className="text-xs">
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportRecordDelimiter")}</Label>
                <Select
                  value={exportOptions.recordDelimiter}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) =>
                    updateExportOption("recordDelimiter", value as TableExportOptions["recordDelimiter"])
                  }
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="lf" className="text-xs">
                      LF
                    </SelectItem>
                    <SelectItem value="crlf" className="text-xs">
                      CRLF
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportFieldDelimiter")}</Label>
                <Select
                  value={exportOptions.fieldDelimiter}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) =>
                    updateExportOption("fieldDelimiter", value as TableExportOptions["fieldDelimiter"])
                  }
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="comma" className="text-xs">
                      {t("query.exportDelimiterComma")}
                    </SelectItem>
                    <SelectItem value="tab" className="text-xs">
                      Tab
                    </SelectItem>
                    <SelectItem value="semicolon" className="text-xs">
                      ;
                    </SelectItem>
                    <SelectItem value="pipe" className="text-xs">
                      |
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportTextQualifier")}</Label>
                <Select
                  value={exportOptions.textQualifier}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) =>
                    updateExportOption("textQualifier", value as TableExportOptions["textQualifier"])
                  }
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="double" className="text-xs">
                      &quot;
                    </SelectItem>
                    <SelectItem value="single" className="text-xs">
                      &apos;
                    </SelectItem>
                    <SelectItem value="none" className="text-xs">
                      {t("query.exportQualifierNone")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <label className="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2 text-xs">
                <span>{t("query.exportAppend")}</span>
                <Switch
                  checked={!!exportOptions.append}
                  disabled={exporting}
                  onCheckedChange={(checked) => updateExportOption("append", checked)}
                  aria-label={t("query.exportAppend")}
                />
              </label>
              <label className="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2 text-xs">
                <span>{t("query.exportContinueOnError")}</span>
                <Switch
                  checked={!!exportOptions.continueOnError}
                  disabled={exporting}
                  onCheckedChange={(checked) => updateExportOption("continueOnError", checked)}
                  aria-label={t("query.exportContinueOnError")}
                />
              </label>
              <label className="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2 text-xs">
                <span>{t("query.exportBlankIfZero")}</span>
                <Switch
                  checked={!!exportOptions.blankIfZero}
                  disabled={format === "sql" || exporting}
                  onCheckedChange={(checked) => updateExportOption("blankIfZero", checked)}
                  aria-label={t("query.exportBlankIfZero")}
                />
              </label>
              <label className="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2 text-xs">
                <span>{t("query.exportZeroPaddingDate")}</span>
                <Switch
                  checked={!!exportOptions.zeroPaddingDate}
                  disabled={format === "sql" || exporting}
                  onCheckedChange={(checked) => updateExportOption("zeroPaddingDate", checked)}
                  aria-label={t("query.exportZeroPaddingDate")}
                />
              </label>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportDateOrder")}</Label>
                <Select
                  value={exportOptions.dateOrder}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) => updateExportOption("dateOrder", value as TableExportOptions["dateOrder"])}
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ymd" className="text-xs">
                      YMD
                    </SelectItem>
                    <SelectItem value="dmy" className="text-xs">
                      DMY
                    </SelectItem>
                    <SelectItem value="mdy" className="text-xs">
                      MDY
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportDateDelimiter")}</Label>
                <Input
                  className="h-8 text-xs"
                  value={exportOptions.dateDelimiter ?? ""}
                  disabled={format === "sql" || exporting}
                  onChange={(e) => updateExportOption("dateDelimiter", e.target.value.slice(0, 3))}
                />
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportTimeDelimiter")}</Label>
                <Input
                  className="h-8 text-xs"
                  value={exportOptions.timeDelimiter ?? ""}
                  disabled={format === "sql" || exporting}
                  onChange={(e) => updateExportOption("timeDelimiter", e.target.value.slice(0, 3))}
                />
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("query.exportDecimalSymbol")}</Label>
                <Select
                  value={exportOptions.decimalSymbol}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) =>
                    updateExportOption("decimalSymbol", value as TableExportOptions["decimalSymbol"])
                  }
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="." className="text-xs">
                      .
                    </SelectItem>
                    <SelectItem value="," className="text-xs">
                      ,
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5 sm:col-span-2">
                <Label className="text-xs">{t("query.exportBinaryEncoding")}</Label>
                <Select
                  value={exportOptions.binaryDataEncoding}
                  disabled={format === "sql" || exporting}
                  onValueChange={(value) =>
                    updateExportOption("binaryDataEncoding", value as TableExportOptions["binaryDataEncoding"])
                  }
                >
                  <SelectTrigger size="sm" className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="base64" className="text-xs">
                      Base64
                    </SelectItem>
                    <SelectItem value="hex" className="text-xs">
                      Hex
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>

          <div className="grid gap-2">
            <div className="flex items-center justify-between gap-2">
              <Label className="text-xs">{t("query.exportFields")}</Label>
              <div className="flex gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => setSelectedColumns(columns)}
                  disabled={exporting}
                >
                  {t("query.selectAll")}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => setSelectedColumns(columns.slice(0, 1))}
                  disabled={exporting || columns.length <= 1}
                >
                  {t("query.deselectAll")}
                </Button>
              </div>
            </div>
            <ScrollArea className="h-[160px] rounded-md border border-border">
              <div className="grid grid-cols-2 gap-1 p-2 sm:grid-cols-3">
                {columns.map((column) => (
                  <button
                    key={column}
                    type="button"
                    className="flex h-8 min-w-0 items-center gap-2 rounded-md px-2 text-left text-xs hover:bg-muted disabled:opacity-60"
                    disabled={exporting || (selectedColumns.length === 1 && selectedColumns.includes(column))}
                    onClick={() => handleColumnToggle(column)}
                    title={column}
                  >
                    <span
                      className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
                        selectedColumns.includes(column)
                          ? "border-primary bg-primary text-primary-foreground"
                          : "border-input bg-background"
                      }`}
                    >
                      {selectedColumns.includes(column) && <Check className="h-3 w-3" />}
                    </span>
                    <span className="truncate font-mono">{column}</span>
                  </button>
                ))}
              </div>
            </ScrollArea>
          </div>

          <div className="flex items-center justify-between rounded-md border border-border bg-muted/20 px-3 py-2">
            <div className="grid gap-0.5">
              <span className="text-xs font-medium">{t("query.exportIncludeHeaders")}</span>
              <span className="text-[11px] text-muted-foreground">
                {t("query.exportEstimatedRows", { count: estimatedRows })}
              </span>
            </div>
            <Switch
              checked={includeHeaders}
              disabled={format === "sql" || exporting}
              onCheckedChange={setIncludeHeaders}
              aria-label={t("query.exportIncludeHeaders")}
            />
          </div>

          {logLines.length > 0 && (
            <ScrollArea className="h-[150px] rounded-md border border-border bg-muted/20">
              <pre className="p-3 text-xs font-mono whitespace-pre-wrap">{logLines.join("\n")}</pre>
            </ScrollArea>
          )}
        </div>

        <DialogFooter>
          {completed && filePath && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="h-8 gap-1 text-xs" disabled={exporting}>
                  <ExternalLink className="h-3.5 w-3.5" />
                  {t("query.openExport")}
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem className="text-xs" onClick={handleOpenExportFolder}>
                  {t("query.openExportFolder")}
                </DropdownMenuItem>
                <DropdownMenuItem className="text-xs" onClick={handleOpenExportFile}>
                  {t("query.openExportFile")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          <Button
            variant="outline"
            size="sm"
            className="h-8 text-xs"
            onClick={() => onOpenChange(false)}
            disabled={exporting}
          >
            {completed ? t("action.close") : t("action.cancel")}
          </Button>
          <Button size="sm" className="h-8 gap-1 text-xs" disabled={!canStart} onClick={handleStart}>
            {exporting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Download className="h-3.5 w-3.5" />}
            {completed ? t("query.exportStartAgain") : t("query.exportStart")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
