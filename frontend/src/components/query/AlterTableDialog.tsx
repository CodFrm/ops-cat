import { useCallback, useEffect, useMemo, useState } from "react";
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
  Switch,
} from "@opskat/ui";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { SqlPreviewDialog } from "./SqlPreviewDialog";
import { toast } from "sonner";
import { buildAlterStatements, quoteIdent, type AlterDraftColumn, type AlterLoadedColumn } from "@/lib/tableSql";

interface AlterTableDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  database: string;
  table: string;
  driver?: string;
  onSuccess: (tableName?: string) => void;
}

interface SQLResult {
  rows?: Record<string, unknown>[];
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

function createNewDraft(id: number): AlterDraftColumn {
  return {
    id,
    name: "",
    type: "VARCHAR(255)",
    nullable: true,
    defaultValue: "",
    comment: "",
    isNew: true,
  };
}

export function AlterTableDialog({
  open,
  onOpenChange,
  assetId,
  database,
  table,
  driver,
  onSuccess,
}: AlterTableDialogProps) {
  const { t } = useTranslation();

  const [columns, setColumns] = useState<AlterDraftColumn[]>([]);
  const [originalColumns, setOriginalColumns] = useState<AlterLoadedColumn[]>([]);
  const [tableNameDraft, setTableNameDraft] = useState(table);
  const [tableCommentDraft, setTableCommentDraft] = useState("");
  const [originalTableComment, setOriginalTableComment] = useState("");
  const [nextId, setNextId] = useState(1);
  const [loadingColumns, setLoadingColumns] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [showSqlPreview, setShowSqlPreview] = useState(false);
  const [previewStatements, setPreviewStatements] = useState<string[]>([]);
  const [pendingNextTableName, setPendingNextTableName] = useState<string | undefined>();

  const resetForm = useCallback(() => {
    setColumns([]);
    setOriginalColumns([]);
    setTableNameDraft(table);
    setTableCommentDraft("");
    setOriginalTableComment("");
    setNextId(1);
    setShowSqlPreview(false);
    setPreviewStatements([]);
    setPendingNextTableName(undefined);
  }, [table]);

  const loadColumns = useCallback(async () => {
    if (!assetId || !open) return;
    setLoadingColumns(true);

    try {
      let columnSql = "";
      let tableCommentSql = "";
      if (driver === "postgresql") {
        const escapedTable = escapeLiteral(table);
        columnSql =
          `SELECT c.column_name, c.data_type, c.udt_name, c.is_nullable, c.column_default, ` +
          `COALESCE(pg_catalog.col_description(cls.oid, c.ordinal_position::int), '') AS comment ` +
          `FROM information_schema.columns c ` +
          `JOIN pg_catalog.pg_class cls ON cls.relname = c.table_name ` +
          `JOIN pg_catalog.pg_namespace ns ON ns.oid = cls.relnamespace AND ns.nspname = c.table_schema ` +
          `WHERE c.table_schema = 'public' AND c.table_name = '${escapedTable}' ORDER BY c.ordinal_position`;
        tableCommentSql =
          `SELECT COALESCE(pg_catalog.obj_description(cls.oid, 'pg_class'), '') AS table_comment ` +
          `FROM pg_catalog.pg_class cls ` +
          `JOIN pg_catalog.pg_namespace ns ON ns.oid = cls.relnamespace ` +
          `WHERE ns.nspname = 'public' AND cls.relname = '${escapedTable}' LIMIT 1`;
      } else {
        const quotedTable = quoteIdent(table, driver);
        const escapedDb = escapeLiteral(database);
        const escapedTable = escapeLiteral(table);
        columnSql = `SHOW FULL COLUMNS FROM ${quotedTable}`;
        tableCommentSql =
          `SELECT COALESCE(table_comment, '') AS table_comment FROM information_schema.tables ` +
          `WHERE table_schema = '${escapedDb}' AND table_name = '${escapedTable}' LIMIT 1`;
      }

      const [columnResult, tableCommentResult] = await Promise.all([
        ExecuteSQL(assetId, columnSql, database),
        ExecuteSQL(assetId, tableCommentSql, database),
      ]);
      const parsed: SQLResult = JSON.parse(columnResult);
      const rows = parsed.rows || [];
      const commentParsed: SQLResult = JSON.parse(tableCommentResult);
      const tableCommentRows = commentParsed.rows || [];
      const loadedTableComment = String(getByKey(tableCommentRows[0] || {}, ["table_comment"]) ?? "");

      const loaded = rows
        .map((row) => {
          if (driver === "postgresql") {
            const name = String(getByKey(row, ["column_name"]) ?? "");
            const dataType = String(getByKey(row, ["data_type"]) ?? "");
            const udtName = String(getByKey(row, ["udt_name"]) ?? "");
            const nullable = String(getByKey(row, ["is_nullable"]) ?? "").toUpperCase() === "YES";
            const defaultValue = String(getByKey(row, ["column_default"]) ?? "");
            const comment = String(getByKey(row, ["comment"]) ?? "");
            return {
              name,
              type: dataType === "USER-DEFINED" && udtName ? udtName : dataType,
              nullable,
              defaultValue,
              comment,
            } as AlterLoadedColumn;
          }

          const name = String(getByKey(row, ["Field", "field"]) ?? "");
          const type = String(getByKey(row, ["Type", "type"]) ?? "");
          const nullable = String(getByKey(row, ["Null", "null"]) ?? "").toUpperCase() === "YES";
          const defaultValue = String(getByKey(row, ["Default", "default"]) ?? "");
          const comment = String(getByKey(row, ["Comment", "comment"]) ?? "");
          return { name, type, nullable, defaultValue, comment } as AlterLoadedColumn;
        })
        .filter((col) => col.name);

      setOriginalColumns(loaded);
      setColumns(
        loaded.map((col, idx) => ({
          id: idx + 1,
          originalName: col.name,
          name: col.name,
          type: col.type,
          nullable: col.nullable,
          defaultValue: col.defaultValue,
          comment: col.comment,
          isNew: false,
        }))
      );
      setOriginalTableComment(loadedTableComment);
      setTableCommentDraft(loadedTableComment);
      setNextId(loaded.length + 1);
      setTableNameDraft(table);
    } catch (e) {
      toast.error(String(e));
      setColumns([]);
      setOriginalColumns([]);
      setOriginalTableComment("");
      setTableCommentDraft("");
    } finally {
      setLoadingColumns(false);
    }
  }, [assetId, open, driver, table, database]);

  useEffect(() => {
    if (open) {
      loadColumns();
    }
  }, [open, loadColumns]);

  const hasPendingChanges = useMemo(() => {
    const built = buildAlterStatements({
      driver,
      database,
      table,
      tableNameDraft,
      tableCommentDraft,
      originalTableComment,
      originalColumns,
      draftColumns: columns,
    });
    return built.statements.length > 0;
  }, [driver, database, table, tableNameDraft, tableCommentDraft, originalTableComment, originalColumns, columns]);

  const updateColumn = useCallback((id: number, patch: Partial<AlterDraftColumn>) => {
    setColumns((prev) => prev.map((col) => (col.id === id ? { ...col, ...patch } : col)));
  }, []);

  const handleAddColumn = useCallback(() => {
    setColumns((prev) => [...prev, createNewDraft(nextId)]);
    setNextId((v) => v + 1);
  }, [nextId]);

  const handleRemoveColumn = useCallback((id: number) => {
    setColumns((prev) => prev.filter((col) => col.id !== id));
  }, []);

  const validateDraft = useCallback(() => {
    if (columns.length === 0) {
      toast.error(t("query.designTableAtLeastOneColumn"));
      return false;
    }

    const seen = new Set<string>();
    for (const col of columns) {
      const name = col.name.trim();
      if (!name) {
        toast.error(t("query.designTableColumnNameRequired"));
        return false;
      }
      if (!col.type.trim()) {
        toast.error(t("query.designTableColumnTypeRequired"));
        return false;
      }

      const lower = name.toLowerCase();
      if (seen.has(lower)) {
        toast.error(t("query.designTableDuplicateColumn", { name }));
        return false;
      }
      seen.add(lower);
    }

    return true;
  }, [columns, t]);

  const handlePreview = useCallback(() => {
    if (!assetId) return;
    if (!validateDraft()) return;

    const built = buildAlterStatements({
      driver,
      database,
      table,
      tableNameDraft,
      tableCommentDraft,
      originalTableComment,
      originalColumns,
      draftColumns: columns,
    });

    if (built.statements.length === 0) {
      toast.error(t("query.designTableNoChanges"));
      return;
    }

    setPreviewStatements(built.statements);
    setPendingNextTableName(built.nextTableName);
    setShowSqlPreview(true);
  }, [
    assetId,
    validateDraft,
    driver,
    database,
    table,
    tableNameDraft,
    tableCommentDraft,
    originalTableComment,
    originalColumns,
    columns,
    t,
  ]);

  const handleConfirmSubmit = useCallback(async () => {
    if (!assetId || previewStatements.length === 0) return;

    setSubmitting(true);
    try {
      for (const sql of previewStatements) {
        await ExecuteSQL(assetId, sql, database);
      }

      toast.success(t("query.alterTableSuccess"));
      setShowSqlPreview(false);
      onOpenChange(false);
      onSuccess(pendingNextTableName);
      resetForm();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  }, [assetId, previewStatements, database, t, onOpenChange, onSuccess, pendingNextTableName, resetForm]);

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(nextOpen) => {
          if (!submitting && !nextOpen) resetForm();
          onOpenChange(nextOpen);
        }}
      >
        <DialogContent className="w-[74vw] !max-w-none max-h-[92vh] overflow-hidden" showCloseButton={!submitting}>
          <DialogHeader>
            <DialogTitle>{t("query.designTableDialogTitle")}</DialogTitle>
            <DialogDescription>{t("query.designTableDialogDesc", { table })}</DialogDescription>
          </DialogHeader>

          <div className="space-y-3 flex-1 min-h-0 overflow-hidden">
            <div className="space-y-1.5">
              <Label className="text-xs">{t("query.alterRenameTableLabel")}</Label>
              <Input
                className="h-8 text-xs font-mono"
                value={tableNameDraft}
                onChange={(e) => setTableNameDraft(e.target.value)}
                placeholder={t("query.alterRenameTablePlaceholder")}
                disabled={submitting || loadingColumns}
              />
            </div>

            <div className="space-y-1.5">
              <Label className="text-xs">{t("query.tableCommentLabel")}</Label>
              <Input
                className="h-8 text-xs font-mono"
                value={tableCommentDraft}
                onChange={(e) => setTableCommentDraft(e.target.value)}
                placeholder={t("query.tableCommentPlaceholder")}
                disabled={submitting || loadingColumns}
              />
            </div>

            <div className="flex items-center justify-between">
              <Label className="text-xs">{t("query.designTableColumns")}</Label>
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-xs gap-1"
                onClick={handleAddColumn}
                disabled={submitting || loadingColumns}
              >
                <Plus className="h-3.5 w-3.5" />
                {t("query.designTableAddColumn")}
              </Button>
            </div>

            {loadingColumns ? (
              <div className="flex items-center justify-center py-8 text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
              </div>
            ) : (
              <div className="max-h-[52vh] min-h-0 overflow-y-auto pr-2">
                <div className="space-y-2">
                  <div className="grid grid-cols-20 gap-2 px-1">
                    <Label className="col-span-3 text-[11px] text-muted-foreground">{t("query.columnNameLabel")}</Label>
                    <Label className="col-span-4 text-[11px] text-muted-foreground">{t("query.columnTypeLabel")}</Label>
                    <Label className="col-span-4 text-[11px] text-muted-foreground">
                      {t("query.defaultValueLabel")}
                    </Label>
                    <Label className="col-span-6 text-[11px] text-muted-foreground">
                      {t("query.columnCommentLabel")}
                    </Label>
                    <Label className="col-span-2 text-[11px] text-muted-foreground">
                      {t("query.columnNullableLabel")}
                    </Label>
                    <Label className="col-span-1 text-[11px] text-muted-foreground">&nbsp;</Label>
                  </div>

                  {columns.map((col) => (
                    <div key={col.id} className="rounded-md border border-border p-2">
                      <div className="grid grid-cols-20 gap-2 items-center">
                        <div className="col-span-3">
                          <Input
                            className="h-8 text-xs font-mono"
                            value={col.name}
                            onChange={(e) => updateColumn(col.id, { name: e.target.value })}
                            placeholder={t("query.columnNamePlaceholder")}
                            disabled={submitting}
                          />
                        </div>
                        <div className="col-span-4">
                          <Input
                            className="h-8 text-xs font-mono"
                            value={col.type}
                            onChange={(e) => updateColumn(col.id, { type: e.target.value })}
                            placeholder={t("query.columnTypePlaceholder")}
                            disabled={submitting}
                          />
                        </div>
                        <div className="col-span-4">
                          <Input
                            className="h-8 text-xs font-mono"
                            value={col.defaultValue}
                            onChange={(e) => updateColumn(col.id, { defaultValue: e.target.value })}
                            placeholder={t("query.defaultValuePlaceholder")}
                            disabled={submitting}
                          />
                        </div>
                        <div className="col-span-6">
                          <Input
                            className="h-8 text-xs font-mono"
                            value={col.comment}
                            onChange={(e) => updateColumn(col.id, { comment: e.target.value })}
                            placeholder={t("query.columnCommentPlaceholder")}
                            disabled={submitting}
                          />
                        </div>
                        <div className="col-span-2 flex items-center justify-center gap-1">
                          <Switch
                            checked={col.nullable}
                            onCheckedChange={(checked) => updateColumn(col.id, { nullable: checked })}
                            disabled={submitting}
                          />
                          <span className="text-[11px] text-muted-foreground">
                            {col.nullable ? "NULL" : "NOT NULL"}
                          </span>
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
                    </div>
                  ))}
                </div>
              </div>
            )}
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
            <Button
              size="sm"
              className="h-8 text-xs"
              onClick={handlePreview}
              disabled={submitting || loadingColumns || !hasPendingChanges}
            >
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
