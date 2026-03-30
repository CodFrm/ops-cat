import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { RefreshCw, Puzzle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle, Button } from "@opskat/ui";
import { ListInstalledExtensions, ReloadExtensions } from "../../../wailsjs/go/app/App";

interface ExtInfo {
  name: string;
  version: string;
  icon: string;
  displayName: string;
  description: string;
}

export function ExtensionSection() {
  const { t } = useTranslation();
  const [extensions, setExtensions] = useState<ExtInfo[]>([]);
  const [reloading, setReloading] = useState(false);

  const loadExtensions = async () => {
    try {
      const exts = await ListInstalledExtensions();
      setExtensions(exts || []);
    } catch {
      setExtensions([]);
    }
  };

  useEffect(() => {
    loadExtensions();
  }, []);

  const handleReload = async () => {
    setReloading(true);
    try {
      await ReloadExtensions();
      await loadExtensions();
      toast.success(t("extension.reloadSuccess"));
    } catch (e) {
      toast.error(`${t("extension.reloadError")}: ${String(e)}`);
    } finally {
      setReloading(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="text-base">{t("extension.installed")}</CardTitle>
          <CardDescription>
            {extensions.length > 0
              ? `${extensions.length} ${t("extension.title").toLowerCase()}`
              : t("extension.noExtensionsDesc")}
          </CardDescription>
        </div>
        <Button variant="outline" size="sm" onClick={handleReload} disabled={reloading} className="gap-1">
          <RefreshCw className={`h-3.5 w-3.5 ${reloading ? "animate-spin" : ""}`} />
          {t("extension.reload")}
        </Button>
      </CardHeader>
      <CardContent>
        {extensions.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <Puzzle className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">{t("extension.noExtensions")}</p>
          </div>
        ) : (
          <div className="space-y-3">
            {extensions.map((ext) => (
              <div key={ext.name} className="flex items-center justify-between p-3 border rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="h-8 w-8 rounded bg-muted flex items-center justify-center">
                    <Puzzle className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <div>
                    <p className="font-medium text-sm">{ext.displayName || ext.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {ext.description && <span>{ext.description} · </span>}
                      {t("extension.version")} {ext.version}
                    </p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
