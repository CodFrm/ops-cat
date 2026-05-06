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
import { SqlPreviewDialog } from "./SqlPreviewDialog";
import { quoteIdent, quoteTableRef, sqlQuote } from "@/lib/tableSql";

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

function formatDefaultValue(value: string): string {
  const v = value.trim();
  if (!v) return "";
  if (/^[-+]?\d+(\.\d+)?$/.test(v)) return v;
  if (/^(true|false|null)$/i.test(v)) return v.toUpperCase();
  if (/^(current_timestamp(?:\(\))?|now\(\))$/i.test(v)) return v;
  return sqlQuote(v);
}

export interface BuildCreateTableSqlInput {
  driver?: string;
  database: string;
  name: string;
  columns: { name: string; type: string; nullable: boolean; defaultValue: string }[];
}

export function buildCreateTableSql({ driver, database, name, columns }: BuildCreateTableSqlInput): string {
  const tableRef = quoteTableRef(database, name, driver);

  const defs = columns.map((col) => {
    const nullable = col.nullable ? "" : " NOT NULL";
    const def = col.defaultValue.trim() ? ` DEFAULT ${formatDefaultValue(col.defaultValue)}` : "";
    return `${quoteIdent(col.name.trim(), driver)} ${col.type.trim()}${nullable}${def}`;
  });

  return `CREATE TABLE ${tableRef} (\n  ${defs.join(",\n  ")}\n)`;
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
  const [showSqlPreview, setShowSqlPreview] = useState(false);
  const [previewStatements, setPreviewStatements] = useState<string[]>([]);
  const [pendingName, setPendingName] = useState("");

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

  const handlePreview = useCallback(() => {
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

    const sql = buildCreateTableSql({
      driver,
      database,
      name,
      columns: columns.map((col) => ({
        name: col.name,
        type: col.type,
        nullable: col.nullable,
        defaultValue: col.defaultValue,
      })),
    });

    setPreviewStatements([sql]);
    setPendingName(name);
    setShowSqlPreview(true);
  }, [assetId, tableName, columns, driver, database, t]);

  const handleConfirmSubmit = useCallback(async () => {
    if (!assetId || previewStatements.length === 0) return;

    setSubmitting(true);
    try {
      for (const sql of previewStatements) {
        await ExecuteSQL(assetId, sql, database);
      }
      toast.success(t("query.createTableSuccess", { table: pendingName }));
      setShowSqlPreview(false);
      onOpenChange(false);
      onSuccess(pendingName);
      resetForm();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  }, [assetId, previewStatements, database, t, pendingName, onOpenChange, onSuccess, resetForm]);

  return (
    <>
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
            <Button size="sm" className="h-8 text-xs" onClick={handlePreview} disabled={submitting}>
              {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              {t("query.designTablePreviewChanges")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SqlPreviewDialog
        open={showSqlPreview}
        onOpenChange={(nextOpen) => {
          if (!submitting) setShowSqlPreview(nextOpen);
        }}
        statements={previewStatements}
        onConfirm={handleConfirmSubmit}
        submitting={submitting}
      />
    </>
  );
}
