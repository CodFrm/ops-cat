import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Card, CardContent, CardDescription, CardHeader, CardTitle } from "@opskat/ui";
import {
  PreviewTabbyConfig,
  ImportTabbySelected,
  PreviewSSHConfig,
  ImportSSHConfigSelected,
} from "../../../wailsjs/go/app/App";
import { import_svc } from "../../../wailsjs/go/models";
import { ImportDialog, ImportCallOptions } from "@/components/settings/ImportDialog";
import { Import } from "lucide-react";
import { toast } from "sonner";

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));

export function ImportSection() {
  const { t } = useTranslation();

  const [importPreview, setImportPreview] = useState<import_svc.PreviewResult | null>(null);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [importDialogTitle, setImportDialogTitle] = useState("");
  const [importFn, setImportFn] = useState<
    ((indexes: number[], options: ImportCallOptions) => Promise<import_svc.ImportResult>) | null
  >(null);
  const [tabbyLoading, setTabbyLoading] = useState(false);
  const [sshConfigLoading, setSSHConfigLoading] = useState(false);

  const handlePreviewTabby = async () => {
    setTabbyLoading(true);
    try {
      const result = await PreviewTabbyConfig();
      if (result) {
        setImportPreview(result);
        setImportDialogTitle(t("import.tabby"));
        setImportFn(
          () => (indexes: number[], opts: ImportCallOptions) =>
            ImportTabbySelected(indexes, opts.passphrase, opts.overwrite)
        );
        setImportDialogOpen(true);
      }
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setTabbyLoading(false);
    }
  };

  const handlePreviewSSHConfig = async () => {
    setSSHConfigLoading(true);
    try {
      const result = await PreviewSSHConfig();
      if (result) {
        setImportPreview(result);
        setImportDialogTitle(t("import.sshConfig"));
        setImportFn(
          () => (indexes: number[], opts: ImportCallOptions) => ImportSSHConfigSelected(indexes, opts.overwrite)
        );
        setImportDialogOpen(true);
      }
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setSSHConfigLoading(false);
    }
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Tabby</CardTitle>
          <CardDescription>{t("import.tabbyDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button onClick={handlePreviewTabby} disabled={tabbyLoading} variant="outline" className="gap-1">
            <Import className="h-4 w-4" />
            {tabbyLoading ? t("import.importing") : t("import.tabby")}
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">SSH Config</CardTitle>
          <CardDescription>{t("import.sshConfigDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button onClick={handlePreviewSSHConfig} disabled={sshConfigLoading} variant="outline" className="gap-1">
            <Import className="h-4 w-4" />
            {sshConfigLoading ? t("import.importing") : t("import.sshConfig")}
          </Button>
        </CardContent>
      </Card>

      <ImportDialog
        open={importDialogOpen}
        onOpenChange={setImportDialogOpen}
        preview={importPreview}
        title={importDialogTitle}
        onImport={importFn!}
      />
    </>
  );
}
