import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import {
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
} from "@opskat/ui";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { toast } from "sonner";

interface InsertRowDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  table: string;
  driver?: string;
  onSuccess: () => void;
}

interface SQLResult {
  rows?: Record<string, unknown>[];
}

interface TableColumn {
  name: string;
  type: string;
  nullable: boolean;
  hasDefault: boolean;
}

function sqlQuote(value: string): string {
  const escaped = value.replace(/'/g, "''");
  return `'${escaped}'`;
}

function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name}"`;
  return `\`${name}\``;
}

function escapeLiteral(value: string): string {
  return value.replace(/'/g, "''");
}

function getByKey(row: Record<string, unknown>, keys: string[]): unknown {
  for (const key of keys) {
    if (key in row) return row[key];
  }
  const lowerKeys = keys.map((k) => k.toLowerCase());
  for (const [k, v] of Object.entries(row)) {
    if (lowerKeys.includes(k.toLowerCase())) return v;
  }
  return undefined;
}

export function InsertRowDialog({
  open,
  onOpenChange,
  assetId,
  database,
  table,
  driver,
  onSuccess,
}: InsertRowDialogProps) {
  const { t } = useTranslation();
  const [columns, setColumns] = useState<TableColumn[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [loadingColumns, setLoadingColumns] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const loadColumns = useCallback(async () => {
    if (!assetId || !open) return;
    setLoadingColumns(true);

    try {
      let sql = "";
      if (driver === "postgresql") {
        const escapedTable = escapeLiteral(table);
        sql =
          `SELECT column_name, data_type, udt_name, is_nullable, column_default ` +
          `FROM information_schema.columns ` +
          `WHERE table_schema = 'public' AND table_name = '${escapedTable}' ORDER BY ordinal_position`;
      } else {
        const quotedTable = quoteIdent(table, driver);
        sql = `SHOW COLUMNS FROM ${quotedTable}`;
      }

      const result = await ExecuteSQL(assetId, sql, database);
      const parsed: SQLResult = JSON.parse(result);
      const rows = parsed.rows || [];

      const nextColumns: TableColumn[] = rows
        .map((row) => {
          if (driver === "postgresql") {
            const name = String(getByKey(row, ["column_name"]) ?? "");
            const dataType = String(getByKey(row, ["data_type"]) ?? "");
            const udtName = String(getByKey(row, ["udt_name"]) ?? "");
            const nullable = String(getByKey(row, ["is_nullable"]) ?? "").toUpperCase() === "YES";
            const hasDefault = getByKey(row, ["column_default"]) != null;
            return {
              name,
              type: dataType === "USER-DEFINED" && udtName ? udtName : dataType,
              nullable,
              hasDefault,
            };
          }

          const name = String(getByKey(row, ["Field", "field"]) ?? "");
          const type = String(getByKey(row, ["Type", "type"]) ?? "");
          const nullable = String(getByKey(row, ["Null", "null"]) ?? "").toUpperCase() === "YES";
          const defaultValue = getByKey(row, ["Default", "default"]);
          return {
            name,
            type,
            nullable,
            hasDefault: defaultValue != null,
          };
        })
        .filter((col) => col.name);

      setColumns(nextColumns);
      setValues({});
    } catch (e) {
      toast.error(String(e));
      setColumns([]);
    } finally {
      setLoadingColumns(false);
    }
  }, [assetId, open, driver, table, database]);

  useEffect(() => {
    if (open) {
      loadColumns();
    }
  }, [open, loadColumns]);

  const requiredColumns = useMemo(() => columns.filter((col) => !col.nullable && !col.hasDefault), [columns]);

  const handleSubmit = useCallback(async () => {
    if (!assetId) return;

    for (const col of requiredColumns) {
      if (!values[col.name] || values[col.name].trim() === "") {
        toast.error(t("query.insertRequiredField", { field: col.name }));
        return;
      }
    }

    const selected = columns
      .map((col) => ({ col, value: values[col.name] ?? "" }))
      .filter((item) => item.value.trim() !== "");

    const tableName =
      driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;

    const sql =
      selected.length === 0
        ? driver === "postgresql"
          ? `INSERT INTO ${tableName} DEFAULT VALUES`
          : `INSERT INTO ${tableName} () VALUES ()`
        : `INSERT INTO ${tableName} (${selected.map((item) => quoteIdent(item.col.name, driver)).join(", ")}) VALUES (${selected
            .map((item) => sqlQuote(item.value))
            .join(", ")})`;

    setSubmitting(true);
    try {
      await ExecuteSQL(assetId, sql, database);
      toast.success(t("query.insertSuccess"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  }, [assetId, requiredColumns, values, columns, driver, table, database, t, onOpenChange, onSuccess]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl" showCloseButton={!submitting}>
        <DialogHeader>
          <DialogTitle>{t("query.insertDialogTitle")}</DialogTitle>
          <DialogDescription>{t("query.insertDialogDesc", { table })}</DialogDescription>
        </DialogHeader>

        {loadingColumns ? (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
          </div>
        ) : (
          <ScrollArea className="max-h-[360px]">
            <div className="space-y-3 pr-2">
              {columns.map((col) => {
                const required = !col.nullable && !col.hasDefault;
                return (
                  <div key={col.name} className="space-y-1.5">
                    <Label className="text-xs">
                      {col.name}
                      {required ? <span className="text-destructive"> *</span> : null}
                      <span className="ml-1 text-muted-foreground">({col.type})</span>
                    </Label>
                    <Input
                      className="h-8 text-xs font-mono"
                      value={values[col.name] ?? ""}
                      onChange={(e) => setValues((prev) => ({ ...prev, [col.name]: e.target.value }))}
                      placeholder={required ? t("query.requiredInput") : t("query.optionalInput")}
                      disabled={submitting}
                    />
                  </div>
                );
              })}
              {columns.length === 0 && <p className="text-xs text-muted-foreground">{t("query.noColumnsFound")}</p>}
            </div>
          </ScrollArea>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="h-8 text-xs"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            {t("action.cancel")}
          </Button>
          <Button
            size="sm"
            className="h-8 text-xs"
            onClick={handleSubmit}
            disabled={submitting || loadingColumns || columns.length === 0}
          >
            {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            {t("query.insertSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
