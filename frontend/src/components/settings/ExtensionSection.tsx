import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Plus, Puzzle, Trash2, Info } from "lucide-react";
import { useExtensionStore, extLocalized } from "@/stores/extensionStore";
import { SelectExtensionDir } from "../../../wailsjs/go/app/App";

export function ExtensionSection() {
  const { t, i18n } = useTranslation();
  const extensions = useExtensionStore((s) => s.extensions);
  const installExtension = useExtensionStore((s) => s.installExtension);
  const removeExtension = useExtensionStore((s) => s.removeExtension);
  const [installing, setInstalling] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);

  const handleInstall = async () => {
    setInstalling(true);
    try {
      const dir = await SelectExtensionDir();
      if (!dir) {
        setInstalling(false);
        return;
      }
      await installExtension(dir);
      toast.success(t("extension.installSuccess"));
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      toast.error(t("extension.installFailed"), { description: msg });
    } finally {
      setInstalling(false);
    }
  };

  const handleRemove = async (name: string) => {
    try {
      setRemoving(name);
      await removeExtension(name);
      toast.success(t("extension.removeSuccess"));
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      toast.error(t("extension.removeFailed"), { description: msg });
    } finally {
      setRemoving(null);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="text-base">{t("extension.installed")}</CardTitle>
          <CardDescription>{t("extension.installedDesc")}</CardDescription>
        </div>
        <Button size="sm" onClick={handleInstall} disabled={installing}>
          <Plus className="h-4 w-4 mr-1" />
          {t("extension.install")}
        </Button>
      </CardHeader>
      <CardContent className="space-y-0 p-0">
        {extensions.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Puzzle className="h-8 w-8 mb-2" />
            <p className="text-sm">{t("extension.noExtensions")}</p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {extensions.map((ext) => (
              <div key={ext.name} className="flex items-center gap-3 px-6 py-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted">
                  <Puzzle className="h-5 w-5 text-muted-foreground" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">
                      {extLocalized(ext.displayName || ext.name, ext.displayName_zh, i18n.language)}
                    </span>
                    <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">v{ext.version}</span>
                  </div>
                  <p className="text-xs text-muted-foreground truncate">
                    {extLocalized(ext.description, ext.description_zh, i18n.language)}
                  </p>
                </div>
                <div className="flex items-center gap-4 shrink-0">
                  <div className="flex items-center gap-1.5">
                    <div className="h-2 w-2 rounded-full bg-green-500" />
                    <span className="text-xs text-green-500">{t("extension.loaded")}</span>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleRemove(ext.name)}
                    disabled={removing === ext.name}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
        <div className="flex items-center gap-2 px-6 py-3 border-t">
          <Info className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          <p className="text-xs text-muted-foreground">{t("extension.restartHint")}</p>
        </div>
      </CardContent>
    </Card>
  );
}
