import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
  Separator,
} from "@opskat/ui";
import { FilePlus2, Link, Loader2, Play, Table2, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import {
  buildImportInsertSql,
  detectDelimiter,
  parseImportSourceText,
  type ImportDataFormat,
  type ImportFieldDelimiter,
  type ImportMode,
  type ImportNullStrategy,
  type ParsedDelimitedTable,
} from "@/lib/tableImport";

interface ImportTableDataDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  table: string;
  columns: string[];
  primaryKeys?: string[];
  driver?: string;
  columnTypes?: Record<string, string>;
  onSubmittingChange?: (submitting: boolean) => void;
  onSubmitStart?: () => number;
  isSubmitCancelled?: (requestId: number) => boolean;
  onSuccess: () => void;
}

type WizardStep = "type" | "source" | "delimiter" | "options" | "mapping" | "mode" | "summary";
type SourceItem = { id: string; name: string; kind: "file" | "url"; text: string };
type ImportProgress = {
  processed: number;
  added: number;
  updated: number;
  deleted: number;
  error: number;
  seconds: number;
};

const steps: WizardStep[] = ["type", "source", "delimiter", "options", "mapping", "mode", "summary"];
const importModes: ImportMode[] = ["append", "update", "append-update", "append-skip", "delete", "copy"];
const importFileRules: Record<ImportDataFormat, { extensions: string[]; mimes: string[] }> = {
  text: {
    extensions: [".txt", ".tsv"],
    mimes: ["text/plain", "text/tab-separated-values"],
  },
  csv: {
    extensions: [".csv"],
    mimes: ["text/csv"],
  },
  json: {
    extensions: [".json"],
    mimes: ["application/json"],
  },
  xml: {
    extensions: [".xml"],
    mimes: ["application/xml", "text/xml"],
  },
};

function formatAccept(format: ImportDataFormat): string {
  const rule = importFileRules[format];
  return [...rule.extensions, ...rule.mimes].join(",");
}

function isAcceptedFileForFormat(file: File, format: ImportDataFormat): boolean {
  const rule = importFileRules[format];
  const lowerName = file.name.toLowerCase();
  const matchesExtension = rule.extensions.some((extension) => lowerName.endsWith(extension));
  const matchesMime = file.type !== "" && rule.mimes.includes(file.type);
  return matchesExtension || matchesMime;
}

function mergeParsedTables(tables: ParsedDelimitedTable[]): ParsedDelimitedTable {
  const headers: string[] = [];
  for (const table of tables) {
    for (const header of table.headers) {
      if (header && !headers.includes(header)) headers.push(header);
    }
  }

  return {
    headers,
    rows: tables.flatMap((table) =>
      table.rows.map((row) => headers.map((header) => row[table.headers.indexOf(header)] ?? ""))
    ),
  };
}

function nextAutoMapping(headers: string[], columns: string[]): Record<string, string> {
  const lowerColumnMap = new Map(columns.map((column) => [column.toLowerCase(), column]));
  const mapping: Record<string, string> = {};
  for (const header of headers) {
    const exact = columns.includes(header) ? header : lowerColumnMap.get(header.toLowerCase());
    if (exact) mapping[header] = exact;
  }
  return mapping;
}

function importModeNeedsPrimaryKey(mode: ImportMode): boolean {
  return mode === "update" || mode === "append-update" || mode === "append-skip" || mode === "delete";
}

function importModeAffects(mode: ImportMode): keyof Pick<ImportProgress, "added" | "updated" | "deleted"> {
  if (mode === "update") return "updated";
  if (mode === "delete") return "deleted";
  return "added";
}

export function ImportTableDataDialog({
  open,
  onOpenChange,
  assetId,
  database,
  table,
  columns,
  primaryKeys: tablePrimaryKeys,
  driver,
  columnTypes = {},
  onSubmittingChange,
  onSubmitStart,
  isSubmitCancelled,
  onSuccess,
}: ImportTableDataDialogProps) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [stepIndex, setStepIndex] = useState(0);
  const [format, setFormat] = useState<ImportDataFormat>("text");
  const [sources, setSources] = useState<SourceItem[]>([]);
  const [urlDraft, setUrlDraft] = useState("");
  const [loadingUrl, setLoadingUrl] = useState(false);
  const [recordDelimiter, setRecordDelimiter] = useState("lf");
  const [importShape, setImportShape] = useState<"delimited" | "fixed">("delimited");
  const [fieldDelimiter, setFieldDelimiter] = useState<ImportFieldDelimiter>("\t");
  const [customDelimiter, setCustomDelimiter] = useState("");
  const [textQualifier, setTextQualifier] = useState('"');
  const [fieldNameRowEnabled, setFieldNameRowEnabled] = useState(true);
  const [fieldNameRow, setFieldNameRow] = useState(1);
  const [dataStartRow, setDataStartRow] = useState(2);
  const [dataEndRow, setDataEndRow] = useState("");
  const [dateOrder, setDateOrder] = useState("dmy");
  const [dateTimeOrder, setDateTimeOrder] = useState("date-time");
  const [dateDelimiter, setDateDelimiter] = useState("/");
  const [yearDelimiterEnabled, setYearDelimiterEnabled] = useState(false);
  const [yearDelimiter, setYearDelimiter] = useState("/");
  const [timeDelimiter, setTimeDelimiter] = useState(":");
  const [decimalSymbol, setDecimalSymbol] = useState(".");
  const [binaryEncoding, setBinaryEncoding] = useState("base64");
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [primaryKeys, setPrimaryKeys] = useState<Set<string>>(new Set(tablePrimaryKeys ?? []));
  const [nullStrategy, setNullStrategy] = useState<ImportNullStrategy>("literal-null");
  const [importMode, setImportMode] = useState<ImportMode>("append");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [extendedInsert, setExtendedInsert] = useState(true);
  const [maxStatementSizeKb, setMaxStatementSizeKb] = useState(1024);
  const [emptyStringAsNull, setEmptyStringAsNull] = useState(false);
  const [ignoreForeignKeyConstraint, setIgnoreForeignKeyConstraint] = useState(false);
  const [continueOnError, setContinueOnError] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [logLines, setLogLines] = useState<string[]>([]);
  const [progress, setProgress] = useState<ImportProgress>({
    processed: 0,
    added: 0,
    updated: 0,
    deleted: 0,
    error: 0,
    seconds: 0,
  });

  useEffect(() => {
    if (!open) return;
    setStepIndex(0);
    setSources([]);
    setUrlDraft("");
    setLogLines([]);
    setProgress({ processed: 0, added: 0, updated: 0, deleted: 0, error: 0, seconds: 0 });
    setPrimaryKeys(new Set(tablePrimaryKeys ?? []));
    setImportMode("append");
    setAdvancedOpen(false);
    setExtendedInsert(true);
    setMaxStatementSizeKb(1024);
    setEmptyStringAsNull(false);
    setIgnoreForeignKeyConstraint(false);
    setContinueOnError(true);
  }, [open, tablePrimaryKeys]);

  useEffect(() => {
    setSources([]);
    setStepIndex(0);
    setLogLines([]);
    setFieldDelimiter(format === "csv" ? "," : "\t");
  }, [format]);

  const parsed = useMemo(() => {
    if (sources.length === 0) return { headers: [], rows: [] };
    const delimiter = customDelimiter ? (customDelimiter[0] as ImportFieldDelimiter) : fieldDelimiter;
    return mergeParsedTables(
      sources.map((source) =>
        parseImportSourceText({
          text: source.text,
          format,
          fieldDelimiter: delimiter,
          fixedWidth: importShape === "fixed",
          fieldNameRowEnabled,
          fieldNameRow,
          dataStartRow,
          dataEndRow: dataEndRow ? Number(dataEndRow) : undefined,
        })
      )
    );
  }, [
    customDelimiter,
    dataEndRow,
    dataStartRow,
    fieldDelimiter,
    fieldNameRow,
    fieldNameRowEnabled,
    format,
    importShape,
    sources,
  ]);

  useEffect(() => {
    setMapping(nextAutoMapping(parsed.headers, columns));
  }, [columns, parsed.headers]);

  const tableName = driver === "postgresql" ? table : `${database}.${table}`;
  const primaryKeyColumns = useMemo(
    () => Array.from(primaryKeys).filter((column) => Object.values(mapping).includes(column)),
    [mapping, primaryKeys]
  );
  const statements = useMemo(
    () =>
      buildImportInsertSql({
        tableName,
        headers: parsed.headers,
        rows: parsed.rows,
        mapping,
        nullStrategy,
        mode: importMode,
        primaryKeys: primaryKeyColumns,
        advancedOptions: {
          extendedInsert,
          maxStatementSizeKb,
          emptyStringAsNull,
          ignoreForeignKeyConstraint,
        },
        driver,
      }),
    [
      driver,
      emptyStringAsNull,
      extendedInsert,
      ignoreForeignKeyConstraint,
      importMode,
      mapping,
      maxStatementSizeKb,
      nullStrategy,
      parsed.headers,
      parsed.rows,
      primaryKeyColumns,
      tableName,
    ]
  );
  const unmappedHeaders = parsed.headers.filter((header) => !mapping[header]);
  const hasMappedColumns = parsed.headers.length > unmappedHeaders.length;
  const modeMissingPrimaryKey = importModeNeedsPrimaryKey(importMode) && primaryKeyColumns.length === 0;
  const step = steps[stepIndex];
  const isDelimitedFormat = format === "text" || format === "csv";
  const canNext =
    step === "type" ||
    (step === "source" && sources.length > 0 && parsed.headers.length > 0) ||
    step === "delimiter" ||
    step === "options" ||
    (step === "mapping" && hasMappedColumns) ||
    (step === "mode" && !modeMissingPrimaryKey && statements.length > 0);
  const canStart = step === "summary" && statements.length > 0 && !submitting;

  const handleFiles = useCallback(
    async (files: FileList | null) => {
      if (!files?.length) return;
      const nextSources: SourceItem[] = [];
      let rejected = 0;
      for (const file of Array.from(files)) {
        if (!isAcceptedFileForFormat(file, format)) {
          rejected += 1;
          continue;
        }
        const text = await file.text();
        nextSources.push({ id: `${file.name}-${file.size}-${Date.now()}`, name: file.name, kind: "file", text });
        if (format === "text" || format === "csv") setFieldDelimiter(detectDelimiter(text));
      }
      if (rejected > 0) toast.error(t("query.importUnsupportedFileType", { count: rejected }));
      setSources((prev) => [...prev, ...nextSources]);
      if (fileInputRef.current) fileInputRef.current.value = "";
    },
    [format, t]
  );

  const handleAddUrl = useCallback(async () => {
    const url = urlDraft.trim();
    if (!url) return;
    setLoadingUrl(true);
    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      const text = await response.text();
      setSources((prev) => [...prev, { id: `${url}-${Date.now()}`, name: url, kind: "url", text }]);
      setUrlDraft("");
    } catch (e) {
      toast.error(String(e));
    } finally {
      setLoadingUrl(false);
    }
  }, [urlDraft]);

  const handleStart = useCallback(async () => {
    if (!assetId || statements.length === 0) return;
    const requestId = onSubmitStart?.() ?? 0;
    const startedAt = performance.now();
    setSubmitting(true);
    onSubmittingChange?.(true);
    const nextLogLines = [
      "[IMP] Import start",
      `[IMP] Import type - ${format.toUpperCase()} file`,
      `[IMP] Import mode - ${importMode}`,
      ...sources.map((source) => `[IMP] Import from - ${source.name}`),
      `[IMP] Import data [${table}]`,
    ];
    setLogLines(nextLogLines);
    setProgress({ processed: 0, added: 0, updated: 0, deleted: 0, error: 0, seconds: 0 });

    let added = 0;
    let updated = 0;
    let deleted = 0;
    let error = 0;
    try {
      for (let index = 0; index < statements.length; index++) {
        if (isSubmitCancelled?.(requestId)) return;
        try {
          const result = await ExecuteSQL(assetId, statements[index], database);
          if (isSubmitCancelled?.(requestId)) return;
          const parsedResult = JSON.parse(result || "{}") as { affected_rows?: number };
          const affected = Number(parsedResult.affected_rows ?? 0);
          const affectedBucket = importModeAffects(importMode);
          if (affectedBucket === "updated") updated += affected;
          else if (affectedBucket === "deleted") deleted += affected;
          else added += affected;
        } catch (e) {
          error += 1;
          const message = e instanceof Error ? e.message : String(e);
          nextLogLines.push(`[ERR] ${message}`);
          nextLogLines.push(`[ERR] ${statements[index]}`);
          setLogLines([...nextLogLines]);
          toast.error(message);
          if (!continueOnError) break;
        }
        setProgress({
          processed: index + 1,
          added,
          updated,
          deleted,
          error,
          seconds: (performance.now() - startedAt) / 1000,
        });
      }
      if (error === 0) {
        toast.success(t("query.importSuccess", { affected: added }));
        onOpenChange(false);
        onSuccess();
      } else {
        nextLogLines.push(
          `[IMP] Processed: ${statements.length}, Added: ${added}, Updated: ${updated}, Deleted: ${deleted}, Errors: ${error}`
        );
        nextLogLines.push("[IMP] Finished with error");
        setLogLines([...nextLogLines]);
      }
    } finally {
      setSubmitting(false);
      onSubmittingChange?.(false);
    }
  }, [
    assetId,
    database,
    format,
    importMode,
    continueOnError,
    isSubmitCancelled,
    onOpenChange,
    onSubmitStart,
    onSubmittingChange,
    onSuccess,
    sources,
    statements,
    table,
    t,
  ]);

  const goNext = () => {
    if (!canNext || stepIndex >= steps.length - 1) return;
    setStepIndex((value) => value + 1);
  };

  const renderStep = () => {
    if (step === "type") {
      return (
        <div className="space-y-5">
          <p className="text-sm font-medium">{t("query.importWizardTypeIntro")}</p>
          <div className="space-y-3">
            <Label className="text-sm">{t("query.importWizardTypeLabel")}</Label>
            {(["text", "csv", "json", "xml"] as ImportDataFormat[]).map((item) => (
              <label key={item} className="flex w-fit cursor-pointer items-center gap-2 text-sm">
                <input type="radio" name="import-type" checked={format === item} onChange={() => setFormat(item)} />
                {t(`query.importType${item[0].toUpperCase()}${item.slice(1)}`)}
              </label>
            ))}
          </div>
        </div>
      );
    }

    if (step === "source") {
      return (
        <div className="space-y-3">
          <p className="text-sm font-medium">{t("query.importSourceIntro")}</p>
          <div className="h-[280px] rounded-md border">
            <div className="grid grid-cols-[1fr_120px] border-b bg-muted/40 px-3 py-2 text-xs font-medium">
              <span>{t("query.importSource")}</span>
              <span>{t("query.importSourceKind")}</span>
            </div>
            <ScrollArea className="h-[240px]">
              {sources.map((source) => (
                <div key={source.id} className="grid grid-cols-[1fr_120px_32px] items-center gap-2 px-3 py-2 text-sm">
                  <span className="truncate" title={source.name}>
                    {source.name}
                  </span>
                  <span className="text-xs text-muted-foreground">{t(`query.importSource${source.kind}`)}</span>
                  <Button
                    type="button"
                    size="icon-xs"
                    variant="ghost"
                    onClick={() => setSources((prev) => prev.filter((item) => item.id !== source.id))}
                    title={t("action.delete")}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </ScrollArea>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept={formatAccept(format)}
              className="sr-only"
              onChange={(event) => handleFiles(event.target.files)}
            />
            <Button type="button" size="sm" variant="outline" onClick={() => fileInputRef.current?.click()}>
              <FilePlus2 className="h-3.5 w-3.5" />
              {t("query.importAddFile")}
            </Button>
            <div className="flex min-w-[280px] flex-1 items-center gap-2">
              <Input
                value={urlDraft}
                onChange={(event) => setUrlDraft(event.target.value)}
                placeholder={t("query.importUrlPlaceholder")}
                className="h-8 text-xs"
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={loadingUrl || !urlDraft.trim()}
                onClick={handleAddUrl}
              >
                {loadingUrl ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Link className="h-3.5 w-3.5" />}
                {t("query.importAddUrl")}
              </Button>
            </div>
          </div>
        </div>
      );
    }

    if (step === "delimiter") {
      return (
        <div className="space-y-5">
          <p className="text-sm font-medium">{t("query.importDelimiterIntro")}</p>
          <div className="flex items-center gap-3">
            <Label className="w-36 justify-end text-sm">{t("query.importRecordDelimiter")}</Label>
            <Select value={recordDelimiter} onValueChange={setRecordDelimiter}>
              <SelectTrigger className="h-8 w-32 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="lf">LF</SelectItem>
                <SelectItem value="crlf">CRLF</SelectItem>
                <SelectItem value="cr">CR</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <label className="flex w-fit items-center gap-2 text-sm">
            <input
              type="radio"
              checked={importShape === "delimited"}
              disabled={!isDelimitedFormat}
              onChange={() => setImportShape("delimited")}
            />
            {t("query.importDelimited")}
          </label>
          <label className="flex w-fit items-center gap-2 text-sm">
            <input type="radio" checked={importShape === "fixed"} onChange={() => setImportShape("fixed")} />
            {t("query.importFixedWidth")}
          </label>
          <div className="ml-7 space-y-3">
            <div className="flex items-center gap-3">
              <Label className="w-36 justify-end text-sm">{t("query.importFieldDelimiter")}</Label>
              <Select
                value={fieldDelimiter}
                disabled={importShape !== "delimited"}
                onValueChange={(value) => setFieldDelimiter(value as ImportFieldDelimiter)}
              >
                <SelectTrigger className="h-8 w-44 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="\t">{t("query.exportDelimiterTab")}</SelectItem>
                  <SelectItem value=",">{t("query.exportDelimiterComma")}</SelectItem>
                  <SelectItem value=";">{t("query.exportDelimiterSemicolon")}</SelectItem>
                  <SelectItem value="|">{t("query.exportDelimiterPipe")}</SelectItem>
                  <SelectItem value=" ">{t("query.exportDelimiterSpace")}</SelectItem>
                </SelectContent>
              </Select>
              <Input
                value={customDelimiter}
                onChange={(event) => setCustomDelimiter(event.target.value.slice(0, 1))}
                className="h-8 w-16 text-xs"
              />
            </div>
            <div className="flex items-center gap-3">
              <Label className="w-36 justify-end text-sm">{t("query.importTextQualifier")}</Label>
              <Select value={textQualifier} disabled={importShape !== "delimited"} onValueChange={setTextQualifier}>
                <SelectTrigger className="h-8 w-32 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={'"'}>"</SelectItem>
                  <SelectItem value="'">'</SelectItem>
                  <SelectItem value="none">{t("query.exportQualifierNone")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      );
    }

    if (step === "options") {
      return (
        <div className="space-y-5">
          <p className="text-sm font-medium">{t("query.importOptionsTitle")}</p>
          <div className="space-y-3">
            <label className="flex items-center gap-3 text-sm">
              <input
                type="checkbox"
                checked={fieldNameRowEnabled}
                onChange={(event) => setFieldNameRowEnabled(event.target.checked)}
              />
              {t("query.importFieldNameRow")}
              <span>{t("query.importRow")}</span>
              <Input
                type="number"
                value={fieldNameRow}
                disabled={!fieldNameRowEnabled}
                min={1}
                onChange={(event) => setFieldNameRow(Number(event.target.value))}
                className="h-8 w-20 text-xs"
              />
            </label>
            <div className="flex items-center gap-3 pl-7 text-sm">
              {t("query.importDataRow")}
              <span>{t("query.importRow")}</span>
              <Input
                type="number"
                value={dataStartRow}
                min={1}
                onChange={(event) => setDataStartRow(Number(event.target.value))}
                className="h-8 w-20 text-xs"
              />
              <span>~</span>
              <span>{t("query.importRow")}</span>
              <Input
                value={dataEndRow}
                onChange={(event) => setDataEndRow(event.target.value.replace(/\D/g, ""))}
                placeholder={t("query.importEndOfFile")}
                className="h-8 w-28 text-xs"
              />
            </div>
          </div>
          <Separator />
          <div className="grid grid-cols-[1fr_1fr] gap-6">
            <div className="space-y-3">
              <div className="text-sm font-semibold">{t("query.importDateTimeFormats")}</div>
              <div className="grid grid-cols-[140px_1fr] items-center gap-2 text-sm">
                <Label className="justify-end">{t("query.exportDateOrder")}</Label>
                <Select value={dateOrder} onValueChange={setDateOrder}>
                  <SelectTrigger className="h-8 w-40 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="dmy">DMY</SelectItem>
                    <SelectItem value="mdy">MDY</SelectItem>
                    <SelectItem value="ymd">YMD</SelectItem>
                  </SelectContent>
                </Select>
                <Label className="justify-end">{t("query.importDateTimeOrder")}</Label>
                <Select value={dateTimeOrder} onValueChange={setDateTimeOrder}>
                  <SelectTrigger className="h-8 w-48 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="date-time">{t("query.importDateTimeOrderDateTime")}</SelectItem>
                    <SelectItem value="time-date">{t("query.importDateTimeOrderTimeDate")}</SelectItem>
                  </SelectContent>
                </Select>
                <Label className="justify-end">{t("query.exportDateDelimiter")}</Label>
                <Input
                  value={dateDelimiter}
                  onChange={(event) => setDateDelimiter(event.target.value)}
                  className="h-8 w-28"
                />
                <label className="contents">
                  <span className="flex justify-end gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={yearDelimiterEnabled}
                      onChange={(event) => setYearDelimiterEnabled(event.target.checked)}
                    />
                    {t("query.importYearDelimiter")}
                  </span>
                  <Input
                    value={yearDelimiter}
                    disabled={!yearDelimiterEnabled}
                    onChange={(event) => setYearDelimiter(event.target.value)}
                    className="h-8 w-28"
                  />
                </label>
                <Label className="justify-end">{t("query.exportTimeDelimiter")}</Label>
                <Input
                  value={timeDelimiter}
                  onChange={(event) => setTimeDelimiter(event.target.value)}
                  className="h-8 w-28"
                />
              </div>
            </div>
            <div className="space-y-3 text-sm">
              <div className="font-semibold">{t("query.importDateTimeExample")}</div>
              <div className="space-y-1 text-muted-foreground">
                <div>24/8/23 15:30:38</div>
                <div>24/8/2023 15:30:38</div>
                <div>24/Aug/23 15:30:38</div>
                <div>24/August/23 15:30:38</div>
              </div>
            </div>
          </div>
          <div className="space-y-3">
            <div className="text-sm font-semibold">{t("query.importOtherFormats")}</div>
            <div className="grid grid-cols-[180px_180px] items-center gap-2 text-sm">
              <Label className="justify-end">{t("query.exportDecimalSymbol")}</Label>
              <Input value={decimalSymbol} onChange={(event) => setDecimalSymbol(event.target.value)} className="h-8" />
              <Label className="justify-end">{t("query.exportBinaryEncoding")}</Label>
              <Select value={binaryEncoding} onValueChange={setBinaryEncoding}>
                <SelectTrigger className="h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="base64">Base64</SelectItem>
                  <SelectItem value="hex">Hex</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      );
    }

    if (step === "mapping") {
      return (
        <div className="space-y-3">
          <p className="text-sm font-medium">{t("query.importMappingIntro")}</p>
          <div className="flex h-8 items-center gap-2 rounded-md border px-3 text-sm">
            <Table2 className="h-4 w-4 text-primary" />
            <span>
              {table} &lt;- {sources.map((source) => source.name.replace(/\.[^.]+$/, "")).join(", ")}
            </span>
          </div>
          <div className="h-[300px] rounded-md border">
            <div className="grid grid-cols-[1fr_200px_130px] border-b bg-muted/40 px-3 py-2 text-xs font-medium">
              <span>{t("query.importSourceField")}</span>
              <span>{t("query.importTargetField")}</span>
              <span>{t("query.importPrimaryKey")}</span>
            </div>
            <ScrollArea className="h-[260px]">
              {parsed.headers.map((header) => (
                <div key={header} className="grid grid-cols-[1fr_200px_130px] items-center gap-3 px-3 py-1.5 text-sm">
                  <span className="truncate font-mono" title={header}>
                    {header}
                  </span>
                  <Select
                    value={mapping[header] || "__skip__"}
                    onValueChange={(value) =>
                      setMapping((prev) => {
                        const next = { ...prev };
                        if (value === "__skip__") delete next[header];
                        else next[header] = value;
                        return next;
                      })
                    }
                  >
                    <SelectTrigger className="h-8 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__skip__">{t("query.importSkipColumn")}</SelectItem>
                      {columns.map((column) => (
                        <SelectItem key={column} value={column}>
                          {columnTypes[column] ? `${column} (${columnTypes[column]})` : column}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <input
                    type="checkbox"
                    checked={primaryKeys.has(mapping[header])}
                    disabled={!mapping[header]}
                    onChange={(event) =>
                      setPrimaryKeys((prev) => {
                        const next = new Set(prev);
                        if (event.target.checked) next.add(mapping[header]);
                        else next.delete(mapping[header]);
                        return next;
                      })
                    }
                  />
                </div>
              ))}
            </ScrollArea>
          </div>
          <div className="flex items-center justify-between gap-2">
            <Select value={nullStrategy} onValueChange={(value) => setNullStrategy(value as ImportNullStrategy)}>
              <SelectTrigger className="h-8 w-[240px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="literal-null">{t("query.importNullLiteral")}</SelectItem>
                <SelectItem value="empty-is-null">{t("query.importNullEmpty")}</SelectItem>
                <SelectItem value="empty-is-empty-string">{t("query.importNullEmptyString")}</SelectItem>
              </SelectContent>
            </Select>
            <span className="text-xs text-muted-foreground">
              {t("query.importPreviewRows", { count: parsed.rows.length })}
            </span>
          </div>
          {unmappedHeaders.length > 0 && (
            <div
              className={`rounded-md border px-3 py-2 text-xs ${
                hasMappedColumns
                  ? "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300"
                  : "border-destructive/40 bg-destructive/10 text-destructive"
              }`}
            >
              {hasMappedColumns
                ? t("query.importUnmappedColumns", {
                    count: unmappedHeaders.length,
                    columns: unmappedHeaders.join(", "),
                  })
                : t("query.importNoMappedColumns")}
            </div>
          )}
        </div>
      );
    }

    if (step === "mode") {
      return (
        <div className="space-y-5">
          <p className="text-sm font-medium">{t("query.importModeIntro")}</p>
          <div className="space-y-3">
            <Label className="text-sm">{t("query.importModeLabel")}</Label>
            {importModes.map((mode) => (
              <label key={mode} className="flex w-fit cursor-pointer items-start gap-2 text-sm leading-6">
                <input
                  type="radio"
                  name="import-mode"
                  checked={importMode === mode}
                  onChange={() => setImportMode(mode)}
                  className="mt-1"
                />
                <span>
                  {t(`query.importMode${mode.replace(/(^|-)([a-z])/g, (_, __, letter) => letter.toUpperCase())}`)}
                </span>
              </label>
            ))}
          </div>
          {modeMissingPrimaryKey && (
            <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {t("query.importModePrimaryKeyRequired")}
            </div>
          )}
          <div className="flex justify-end pt-20">
            <Button type="button" variant="outline" size="sm" onClick={() => setAdvancedOpen(true)}>
              {t("query.importAdvancedSettings")}
            </Button>
          </div>
        </div>
      );
    }

    return (
      <div className="space-y-4">
        <p className="break-words text-sm font-medium">{t("query.importSummaryIntro")}</p>
        <div className="grid w-56 grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm">
          <span className="text-right">{t("query.importTableCount")}</span>
          <span>
            {sources.length > 0 ? 1 : 0}/{sources.length > 0 ? 1 : 0}
          </span>
          <span className="text-right">{t("query.importProcessed")}</span>
          <span>{progress.processed}</span>
          <span className="text-right">{t("query.importAdded")}</span>
          <span>{progress.added}</span>
          <span className="text-right">{t("query.importUpdated")}</span>
          <span>{progress.updated}</span>
          <span className="text-right">{t("query.importDeleted")}</span>
          <span>{progress.deleted}</span>
          <span className="text-right">{t("query.importError")}</span>
          <span>{progress.error}</span>
          <span className="text-right">{t("query.importTime")}</span>
          <span>{progress.seconds.toFixed(1)}s</span>
        </div>
        <div className="h-[210px] max-w-full overflow-auto rounded-md border bg-muted/10 p-3 font-mono text-xs">
          {logLines.length > 0
            ? logLines.map((line, index) => (
                <div
                  key={index}
                  className={line.startsWith("[ERR]") ? "whitespace-pre-wrap text-destructive" : "whitespace-pre-wrap"}
                >
                  {line}
                </div>
              ))
            : statements.slice(0, 8).map((statement, index) => (
                <div key={index} className="truncate">
                  {statement}
                </div>
              ))}
        </div>
        <div className="h-2 rounded-full bg-muted">
          <div
            className={`h-full rounded-full transition-all ${progress.error > 0 ? "bg-destructive" : "bg-primary"}`}
            style={{ width: `${statements.length ? (progress.processed / statements.length) * 100 : 0}%` }}
          />
        </div>
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[calc(100vh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:!max-w-5xl"
        showCloseButton={!submitting}
      >
        <DialogHeader className="border-b px-6 py-4">
          <DialogTitle>{t("query.importDialogTitle")}</DialogTitle>
          <DialogDescription>{t("query.importDialogDesc", { table })}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto px-6 py-5">
          <div className="min-h-[420px] sm:min-h-[520px]">{renderStep()}</div>
        </div>

        <DialogFooter className="!flex-row flex-wrap items-center gap-2 border-t bg-muted/40 px-6 py-4 sm:!justify-end">
          <div>
            <Button type="button" variant="outline" size="sm" disabled>
              {t("query.importSaveProfile")}
            </Button>
          </div>
          <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={submitting || stepIndex === 0}
              onClick={() => setStepIndex(stepIndex - 1)}
            >
              {t("query.importWizardBack")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={submitting || step === "summary" || !canNext}
              onClick={goNext}
            >
              {t("query.importWizardNext")}
            </Button>
            <Button type="button" size="sm" disabled={!canStart} onClick={handleStart}>
              {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
              {t("query.importWizardStart")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
      <AlertDialog open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <AlertDialogContent className="max-w-xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("query.importAdvancedTitle")}</AlertDialogTitle>
          </AlertDialogHeader>
          <div className="space-y-3 py-2 text-sm">
            <label className="flex w-fit items-center gap-3">
              <input
                type="checkbox"
                checked={extendedInsert}
                onChange={(event) => setExtendedInsert(event.target.checked)}
              />
              {t("query.importAdvancedExtendedInsert")}
            </label>
            <label className="flex w-fit items-center gap-3 pl-7">
              <input
                type="checkbox"
                checked={extendedInsert}
                onChange={(event) => setExtendedInsert(event.target.checked)}
              />
              {t("query.importAdvancedMaxStatementSize")}
              <Input
                type="number"
                min={1}
                value={maxStatementSizeKb}
                disabled={!extendedInsert}
                onChange={(event) => setMaxStatementSizeKb(Number(event.target.value) || 1)}
                className="h-8 w-28 text-xs"
              />
              <span>KB</span>
            </label>
            <label className="flex w-fit items-center gap-3">
              <input
                type="checkbox"
                checked={emptyStringAsNull}
                onChange={(event) => setEmptyStringAsNull(event.target.checked)}
              />
              {t("query.importAdvancedEmptyStringAsNull")}
            </label>
            <label className="flex w-fit items-center gap-3">
              <input
                type="checkbox"
                checked={ignoreForeignKeyConstraint}
                onChange={(event) => setIgnoreForeignKeyConstraint(event.target.checked)}
              />
              {t("query.importAdvancedIgnoreForeignKey")}
            </label>
            <label className="flex w-fit items-center gap-3">
              <input
                type="checkbox"
                checked={continueOnError}
                onChange={(event) => setContinueOnError(event.target.checked)}
              />
              {t("query.importAdvancedContinueOnError")}
            </label>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("query.importAdvancedCancel")}</AlertDialogCancel>
            <AlertDialogAction>{t("query.importAdvancedOk")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  );
}
