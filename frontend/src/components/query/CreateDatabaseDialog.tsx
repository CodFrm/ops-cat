import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import { Button, Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, Input, Label } from "@opskat/ui";
import { ExecuteSQL } from "../../../wailsjs/go/app/App";
import { toast } from "sonner";

interface CreateDatabaseDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetId: number;
  defaultDatabase: string;
  driver?: string;
  onSuccess: () => void;
}

function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name.replace(/"/g, '""')}"`;
  return `\`${name.replace(/`/g, "``")}\``;
}

function sqlQuote(value: string): string {
  const escaped = value.replace(/'/g, "''");
  return `'${escaped}'`;
}

function isSafeOption(value: string): boolean {
  return /^[A-Za-z0-9_.-]+$/.test(value);
}

function buildCreateDatabaseSql(name: string, charset: string, collation: string, driver?: string): string {
  const databaseRef = quoteIdent(name, driver);

  if (driver === "postgresql") {
    const parts = [`CREATE DATABASE ${databaseRef}`];
    const options: string[] = [];

    if (charset.trim()) {
      options.push(`ENCODING = ${sqlQuote(charset.trim())}`);
    }
    if (collation.trim()) {
      options.push(`LC_COLLATE = ${sqlQuote(collation.trim())}`);
      options.push(`LC_CTYPE = ${sqlQuote(collation.trim())}`);
    }

    if (options.length > 0) {
      parts.push(`WITH ${options.join(" ")}`);
    }

    return parts.join(" ");
  }

  const parts = [`CREATE DATABASE ${databaseRef}`];
  if (charset.trim()) {
    parts.push(`CHARACTER SET ${charset.trim()}`);
  }
  if (collation.trim()) {
    parts.push(`COLLATE ${collation.trim()}`);
  }
  return parts.join(" ");
}

export function CreateDatabaseDialog({
  open,
  onOpenChange,
  assetId,
  defaultDatabase,
  driver,
  onSuccess,
}: CreateDatabaseDialogProps) {
  const { t } = useTranslation();
  const [databaseName, setDatabaseName] = useState("");
  const [charset, setCharset] = useState(driver === "postgresql" ? "UTF8" : "utf8mb4");
  const [collation, setCollation] = useState(
    driver === "postgresql" ? "en_US.UTF-8" : "utf8mb4_0900_ai_ci"
  );
  const [submitting, setSubmitting] = useState(false);

  const resetForm = useCallback(() => {
    setDatabaseName("");
    setCharset(driver === "postgresql" ? "UTF8" : "utf8mb4");
    setCollation(driver === "postgresql" ? "en_US.UTF-8" : "utf8mb4_0900_ai_ci");
  }, [driver]);

  const handleSubmit = useCallback(async () => {
    if (!assetId) return;

    const name = databaseName.trim();
    if (!name) {
      toast.error(t("query.createDatabaseNameRequired"));
      return;
    }
    if ((charset.trim() && !isSafeOption(charset.trim())) || (collation.trim() && !isSafeOption(collation.trim()))) {
      toast.error(t("query.createDatabaseOptionInvalid"));
      return;
    }

    const sql = buildCreateDatabaseSql(name, charset, collation, driver);

    setSubmitting(true);
    try {
      await ExecuteSQL(assetId, sql, defaultDatabase);
      toast.success(t("query.createDatabaseSuccess", { database: name }));
      onOpenChange(false);
      onSuccess();
      resetForm();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  }, [assetId, databaseName, charset, collation, driver, defaultDatabase, t, onOpenChange, onSuccess, resetForm]);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!submitting && !nextOpen) resetForm();
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-w-lg" showCloseButton={!submitting}>
        <DialogHeader>
          <DialogTitle>{t("query.createDatabaseDialogTitle")}</DialogTitle>
          <DialogDescription>{t("query.createDatabaseDialogDesc")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label className="text-xs">{t("query.databaseNameLabel")}</Label>
            <Input
              className="h-8 text-xs font-mono"
              value={databaseName}
              onChange={(e) => setDatabaseName(e.target.value)}
              placeholder={t("query.databaseNamePlaceholder")}
              disabled={submitting}
            />
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs">{t("query.charsetLabel")}</Label>
            <Input
              className="h-8 text-xs font-mono"
              value={charset}
              onChange={(e) => setCharset(e.target.value)}
              placeholder={t("query.charsetPlaceholder")}
              disabled={submitting}
            />
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs">{t("query.collationLabel")}</Label>
            <Input
              className="h-8 text-xs font-mono"
              value={collation}
              onChange={(e) => setCollation(e.target.value)}
              placeholder={t("query.collationPlaceholder")}
              disabled={submitting}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => onOpenChange(false)} disabled={submitting}>
            {t("action.cancel")}
          </Button>
          <Button size="sm" className="h-8 text-xs" onClick={handleSubmit} disabled={submitting}>
            {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            {t("query.createDatabaseSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
