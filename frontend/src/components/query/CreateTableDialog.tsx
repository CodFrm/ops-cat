import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2, Plus, Trash2 } from "lucide-react";
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
  Switch,
} from "@opskat/ui";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { toast } from "sonner";

interface CreateTableDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  driver?: string;
  onSuccess: (tableName: string) => void;
}

interface ColumnDraft {
  id: number;
  name: string;
  type: string;
  nullable: boolean;
  defaultValue: string;
}

function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name}"`;
  return `\`${name}\``;
}

function sqlQuote(value: string): string {
  const escaped = value.replace(/'/g, "''");
  return `'${escaped}'`;
}

function formatDefaultValue(value: string): string {
  const v = value.trim();
  if (!v) return "";
  if (/^[-+]?\d+(\.\d+)?$/.test(v)) return v;
  if (/^(true|false|null)$/i.test(v)) return v.toUpperCase();
  if (/^(current_timestamp(?:\(\))?|now\(\))$/i.test(v)) return v;
  return sqlQuote(v);
}

function createEmptyColumn(id: number): ColumnDraft {
  return {
    id,
    name: "",
    type: "VARCHAR(255)",
    nullable: true,
    defaultValue: "",
  };
}

export function CreateTableDialog({
  open,
  onOpenChange,
  assetId,
  database,
  driver,
  onSuccess,
}: CreateTableDialogProps) {
  const { t } = useTranslation();
  const [tableName, setTableName] = useState("");
  const [columns, setColumns] = useState<ColumnDraft[]>([createEmptyColumn(1)]);
  const [nextId, setNextId] = useState(2);
  const [submitting, setSubmitting] = useState(false);

  const handleAddColumn = useCallback(() => {
    setColumns((prev) => [...prev, createEmptyColumn(nextId)]);
    setNextId((v) => v + 1);
  }, [nextId]);

  const handleRemoveColumn = useCallback((id: number) => {
    setColumns((prev) => (prev.length <= 1 ? prev : prev.filter((c) => c.id !== id)));
  }, []);

  const updateColumn = useCallback((id: number, patch: Partial<ColumnDraft>) => {
    setColumns((prev) => prev.map((col) => (col.id === id ? { ...col, ...patch } : col)));
  }, []);

  const resetForm = useCallback(() => {
    setTableName("");
    setColumns([createEmptyColumn(1)]);
    setNextId(2);
  }, []);

  const handleSubmit = useCallback(async () => {
    if (!assetId) return;

    const name = tableName.trim();
    if (!name) {
      toast.error(t("query.createTableNameRequired"));
      return;
    }
    if (columns.length === 0) {
      toast.error(t("query.createTableAtLeastOneColumn"));
      return;
    }

    for (const col of columns) {
      if (!col.name.trim()) {
        toast.error(t("query.createTableColumnNameRequired"));
        return;
      }
      if (!col.type.trim()) {
        toast.error(t("query.createTableColumnTypeRequired"));
        return;
      }
    }

    const tableRef =
      driver === "postgresql"
        ? quoteIdent(name, driver)
        : `${quoteIdent(database, driver)}.${quoteIdent(name, driver)}`;

    const defs = columns.map((col) => {
      const nullable = col.nullable ? "" : " NOT NULL";
      const def = col.defaultValue.trim() ? ` DEFAULT ${formatDefaultValue(col.defaultValue)}` : "";
      return `${quoteIdent(col.name.trim(), driver)} ${col.type.trim()}${nullable}${def}`;
    });

    const sql = `CREATE TABLE ${tableRef} (\n  ${defs.join(",\n  ")}\n)`;

    setSubmitting(true);
    try {
      await ExecuteSQL(assetId, sql, database);
      toast.success(t("query.createTableSuccess", { table: name }));
      onOpenChange(false);
      onSuccess(name);
      resetForm();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  }, [assetId, tableName, columns, driver, database, t, onOpenChange, onSuccess, resetForm]);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!submitting && !nextOpen) resetForm();
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-w-3xl" showCloseButton={!submitting}>
        <DialogHeader>
          <DialogTitle>{t("query.createTableDialogTitle")}</DialogTitle>
          <DialogDescription>{t("query.createTableDialogDesc", { database })}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label className="text-xs">{t("query.tableNameLabel")}</Label>
            <Input
              className="h-8 text-xs font-mono"
              value={tableName}
              onChange={(e) => setTableName(e.target.value)}
              placeholder={t("query.tableNamePlaceholder")}
              disabled={submitting}
            />
          </div>

          <div className="flex items-center justify-between">
            <Label className="text-xs">{t("query.columnsLabel")}</Label>
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={handleAddColumn}
              disabled={submitting}
            >
              <Plus className="h-3.5 w-3.5" />
              {t("query.addColumn")}
            </Button>
          </div>

          <ScrollArea className="max-h-[300px]">
            <div className="space-y-2 pr-2">
              {columns.map((col) => (
                <div key={col.id} className="rounded-md border border-border p-2">
                  <div className="grid grid-cols-12 gap-2 items-end">
                    <div className="col-span-4 space-y-1">
                      <Label className="text-[11px] text-muted-foreground">{t("query.columnNameLabel")}</Label>
                      <Input
                        className="h-8 text-xs font-mono"
                        value={col.name}
                        onChange={(e) => updateColumn(col.id, { name: e.target.value })}
                        placeholder={t("query.columnNamePlaceholder")}
                        disabled={submitting}
                      />
                    </div>
                    <div className="col-span-4 space-y-1">
                      <Label className="text-[11px] text-muted-foreground">{t("query.columnTypeLabel")}</Label>
                      <Input
                        className="h-8 text-xs font-mono"
                        value={col.type}
                        onChange={(e) => updateColumn(col.id, { type: e.target.value })}
                        placeholder={t("query.columnTypePlaceholder")}
                        disabled={submitting}
                      />
                    </div>
                    <div className="col-span-3 space-y-1">
                      <Label className="text-[11px] text-muted-foreground">{t("query.defaultValueLabel")}</Label>
                      <Input
                        className="h-8 text-xs font-mono"
                        value={col.defaultValue}
                        onChange={(e) => updateColumn(col.id, { defaultValue: e.target.value })}
                        placeholder={t("query.defaultValuePlaceholder")}
                        disabled={submitting}
                      />
                    </div>
                    <div className="col-span-1 flex justify-end">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="h-8 w-8"
                        onClick={() => handleRemoveColumn(col.id)}
                        disabled={submitting || columns.length === 1}
                        title={t("query.removeColumn")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                  <div className="mt-2 flex items-center gap-2">
                    <Switch
                      checked={col.nullable}
                      onCheckedChange={(checked) => updateColumn(col.id, { nullable: checked })}
                      disabled={submitting}
                    />
                    <Label className="text-[11px] text-muted-foreground">{t("query.columnNullableLabel")}</Label>
                  </div>
                </div>
              ))}
            </div>
          </ScrollArea>
        </div>

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
          <Button size="sm" className="h-8 text-xs" onClick={handleSubmit} disabled={submitting}>
            {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            {t("query.createTableSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
