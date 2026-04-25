import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { Loader2, Upload } from "lucide-react";
import { toast } from "sonner";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { buildImportInsertSql, detectDelimiter, parseDelimitedText, type ImportNullStrategy } from "@/lib/tableImport";
import { SqlPreviewDialog } from "./SqlPreviewDialog";

interface ImportTableDataDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  table: string;
  columns: string[];
  driver?: string;
  onSubmittingChange?: (submitting: boolean) => void;
  onSubmitStart?: () => number;
  isSubmitCancelled?: (requestId: number) => boolean;
  onSuccess: () => void;
}

export function ImportTableDataDialog({
  open,
  onOpenChange,
  assetId,
  database,
  table,
  columns,
  driver,
  onSubmittingChange,
  onSubmitStart,
  isSubmitCancelled,
  onSuccess,
}: ImportTableDataDialogProps) {
  const { t } = useTranslation();
  const [headers, setHeaders] = useState<string[]>([]);
  const [rows, setRows] = useState<string[][]>([]);
  const [delimiter, setDelimiter] = useState<"," | "\t">(",");
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [nullStrategy, setNullStrategy] = useState<ImportNullStrategy>("literal-null");
  const [previewOpen, setPreviewOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const tableName = driver === "postgresql" ? table : `${database}.${table}`;
  const statements = useMemo(
    () => buildImportInsertSql({ tableName, headers, rows, mapping, nullStrategy, driver }),
    [driver, headers, mapping, nullStrategy, rows, tableName]
  );
  const previewRows = rows.slice(0, 20);

  const handleFile = useCallback(
    async (file: File | undefined) => {
      if (!file) return;
      try {
        const text = await file.text();
        const nextDelimiter = detectDelimiter(text);
        const parsed = parseDelimitedText(text, nextDelimiter);
        const nextMapping: Record<string, string> = {};
        for (const header of parsed.headers) {
          if (columns.includes(header)) nextMapping[header] = header;
        }
        setDelimiter(nextDelimiter);
        setHeaders(parsed.headers);
        setRows(parsed.rows);
        setMapping(nextMapping);
      } catch (e) {
        toast.error(String(e));
      }
    },
    [columns]
  );

  const handleConfirm = useCallback(async () => {
    if (!assetId || statements.length === 0) return;
    const requestId = onSubmitStart?.() ?? 0;
    setSubmitting(true);
    onSubmittingChange?.(true);
    let affected = 0;
    try {
      for (const sql of statements) {
        if (isSubmitCancelled?.(requestId)) return;
        const result = await ExecuteSQL(assetId, sql, database);
        if (isSubmitCancelled?.(requestId)) return;
        const parsed = JSON.parse(result || "{}") as { affected_rows?: number };
        affected += Number(parsed.affected_rows ?? 0);
      }
      toast.success(t("query.importSuccess", { affected }));
      setPreviewOpen(false);
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
      onSubmittingChange?.(false);
    }
  }, [assetId, database, isSubmitCancelled, onOpenChange, onSubmitStart, onSubmittingChange, onSuccess, statements, t]);

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-4xl" showCloseButton={!submitting}>
          <DialogHeader>
            <DialogTitle>{t("query.importDialogTitle")}</DialogTitle>
            <DialogDescription>{t("query.importDialogDesc", { table })}</DialogDescription>
          </DialogHeader>

          <div className="grid gap-3">
            <div className="flex items-center gap-2">
              <Label className="text-xs shrink-0">{t("query.importFile")}</Label>
              <input
                type="file"
                accept=".csv,.tsv,text/csv,text/tab-separated-values,text/plain"
                className="h-8 flex-1 rounded-md border border-input bg-background px-2 py-1 text-xs"
                disabled={submitting}
                onChange={(event) => handleFile(event.target.files?.[0])}
              />
              <span className="text-xs text-muted-foreground">{delimiter === "\t" ? "TSV" : "CSV"}</span>
            </div>

            {headers.length > 0 && (
              <div className="grid gap-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-medium">{t("query.importMapping")}</span>
                  <Select value={nullStrategy} onValueChange={(value) => setNullStrategy(value as ImportNullStrategy)}>
                    <SelectTrigger size="sm" className="h-7 w-[180px] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="literal-null" className="text-xs">
                        {t("query.importNullLiteral")}
                      </SelectItem>
                      <SelectItem value="empty-is-null" className="text-xs">
                        {t("query.importNullEmpty")}
                      </SelectItem>
                      <SelectItem value="empty-is-empty-string" className="text-xs">
                        {t("query.importNullEmptyString")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  {headers.map((header) => (
                    <div key={header} className="flex items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-xs font-mono" title={header}>
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
                        <SelectTrigger size="sm" className="h-7 w-[180px] text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__skip__" className="text-xs">
                            {t("query.importSkipColumn")}
                          </SelectItem>
                          {columns.map((column) => (
                            <SelectItem key={column} value={column} className="text-xs">
                              {column}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {previewRows.length > 0 && (
              <ScrollArea className="h-[260px] rounded-md border border-border">
                <table className="w-full border-collapse text-xs font-mono">
                  <thead className="sticky top-0 bg-muted">
                    <tr>
                      {headers.map((header) => (
                        <th key={header} className="border border-border px-2 py-1 text-left font-semibold">
                          {header}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {previewRows.map((row, rowIdx) => (
                      <tr key={rowIdx} className={rowIdx % 2 === 0 ? "bg-background" : "bg-muted/30"}>
                        {headers.map((header, colIdx) => (
                          <td key={header} className="max-w-[220px] truncate border border-border px-2 py-1">
                            {row[colIdx]}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </ScrollArea>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => onOpenChange(false)}>
              {t("action.cancel")}
            </Button>
            <Button
              size="sm"
              className="h-8 text-xs gap-1"
              disabled={submitting || statements.length === 0}
              onClick={() => setPreviewOpen(true)}
            >
              <Upload className="h-3.5 w-3.5" />
              {t("query.designTablePreviewChanges")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SqlPreviewDialog
        open={previewOpen}
        onOpenChange={(nextOpen) => {
          if (!submitting) setPreviewOpen(nextOpen);
        }}
        statements={statements}
        onConfirm={handleConfirm}
        submitting={submitting}
        warning={
          submitting ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t("query.executing")}
            </div>
          ) : undefined
        }
      />
    </>
  );
}
