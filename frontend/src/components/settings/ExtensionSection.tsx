import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { RefreshCw, Puzzle, Plus, MoreVertical, Info, Trash2 } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Switch,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogCancel,
  AlertDialogAction,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Separator,
} from "@opskat/ui";
import {
  ListInstalledExtensions,
  ReloadExtensions,
  InstallExtension,
  UninstallExtension,
  EnableExtension,
  DisableExtension,
} from "../../../wailsjs/go/app/App";

interface ExtInfo {
  name: string;
  version: string;
  icon: string;
  displayName: string;
  description: string;
  enabled: boolean;
  manifest?: {
    tools?: { name: string; i18n?: { description?: string } }[];
    policies?: {
      groups?: {
        id: string;
        i18n?: { name?: string };
        policy?: { allow_list?: string[]; deny_list?: string[] };
      }[];
    };
    frontend?: {
      pages?: { id: string; i18n?: { name?: string }; slot?: string }[];
    };
  };
}

export function ExtensionSection() {
  const { t } = useTranslation();
  const [extensions, setExtensions] = useState<ExtInfo[]>([]);
  const [reloading, setReloading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState<ExtInfo | null>(null);
  const [cleanData, setCleanData] = useState(false);
  const [detailTarget, setDetailTarget] = useState<ExtInfo | null>(null);

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

  const handleInstall = async () => {
    setInstalling(true);
    try {
      const result = await InstallExtension();
      if (result) {
        await loadExtensions();
        toast.success(t("extension.installSuccess"));
      }
    } catch (e) {
      toast.error(`${t("extension.installError")}: ${String(e)}`);
    } finally {
      setInstalling(false);
    }
  };

  const handleUninstall = async () => {
    if (!uninstallTarget) return;
    try {
      await UninstallExtension(uninstallTarget.name, cleanData);
      await loadExtensions();
      toast.success(t("extension.uninstallSuccess"));
    } catch (e) {
      toast.error(String(e));
    } finally {
      setUninstallTarget(null);
      setCleanData(false);
    }
  };

  const handleToggle = async (ext: ExtInfo) => {
    try {
      if (ext.enabled) {
        await DisableExtension(ext.name);
        toast.success(t("extension.disableSuccess"));
      } else {
        await EnableExtension(ext.name);
        toast.success(t("extension.enableSuccess"));
      }
      await loadExtensions();
    } catch (e) {
      toast.error(String(e));
    }
  };

  return (
    <>
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
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleInstall} disabled={installing} className="gap-1">
              <Plus className="h-3.5 w-3.5" />
              {t("extension.install")}
            </Button>
            <Button variant="outline" size="sm" onClick={handleReload} disabled={reloading} className="gap-1">
              <RefreshCw className={`h-3.5 w-3.5 ${reloading ? "animate-spin" : ""}`} />
              {t("extension.reload")}
            </Button>
          </div>
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
                <div
                  key={ext.name}
                  className={`flex items-center justify-between p-3 border rounded-lg ${
                    !ext.enabled ? "opacity-60" : ""
                  }`}
                >
                  <div className="flex items-center gap-3">
                    <div className="h-8 w-8 rounded bg-muted flex items-center justify-center">
                      <Puzzle className="h-4 w-4 text-muted-foreground" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <p className="font-medium text-sm">{ext.displayName || ext.name}</p>
                        {!ext.enabled && (
                          <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                            {t("extension.disabled")}
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {ext.description && <span>{ext.description} · </span>}
                        {t("extension.version")} {ext.version}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch checked={ext.enabled} onCheckedChange={() => handleToggle(ext)} />
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                          <MoreVertical className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setDetailTarget(ext)}>
                          <Info className="h-4 w-4 mr-2" />
                          {t("extension.detail")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => setUninstallTarget(ext)} className="text-destructive">
                          <Trash2 className="h-4 w-4 mr-2" />
                          {t("extension.uninstall")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog */}
      <AlertDialog
        open={!!uninstallTarget}
        onOpenChange={(open) => {
          if (!open) {
            setUninstallTarget(null);
            setCleanData(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("extension.uninstall")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("extension.uninstallConfirm", {
                name: uninstallTarget?.displayName || uninstallTarget?.name,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex items-center gap-3 py-2">
            <Switch id="clean-data" checked={cleanData} onCheckedChange={setCleanData} />
            <div>
              <label htmlFor="clean-data" className="text-sm font-medium cursor-pointer">
                {t("extension.cleanData")}
              </label>
              <p className="text-xs text-muted-foreground">{t("extension.cleanDataDesc")}</p>
            </div>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={handleUninstall}>{t("extension.uninstall")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Detail Dialog */}
      <Dialog open={!!detailTarget} onOpenChange={(open) => !open && setDetailTarget(null)}>
        <DialogContent className="max-w-lg max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{detailTarget?.displayName || detailTarget?.name}</DialogTitle>
          </DialogHeader>
          {detailTarget && <ExtensionDetail ext={detailTarget} />}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ExtensionDetail({ ext }: { ext: ExtInfo }) {
  const { t } = useTranslation();
  const tools = ext.manifest?.tools || [];
  const policyGroups = ext.manifest?.policies?.groups || [];
  const pages = ext.manifest?.frontend?.pages || [];

  return (
    <div className="space-y-4">
      {/* Basic Info */}
      <div className="space-y-1 text-sm">
        <p>
          <span className="text-muted-foreground">{t("extension.version")}:</span> {ext.version}
        </p>
        {ext.description && <p className="text-muted-foreground">{ext.description}</p>}
      </div>

      <Separator />

      {/* Tools */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.tools")}</h4>
        {tools.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noTools")}</p>
        ) : (
          <div className="space-y-1">
            {tools.map((tool) => (
              <div key={tool.name} className="flex justify-between text-xs p-2 rounded bg-muted/50">
                <span className="font-mono">{tool.name}</span>
                <span className="text-muted-foreground ml-2 text-right">{tool.i18n?.description || ""}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <Separator />

      {/* Policy Groups */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.policies")}</h4>
        {policyGroups.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noPolicies")}</p>
        ) : (
          <div className="space-y-1">
            {policyGroups.map((pg) => (
              <div key={pg.id} className="text-xs p-2 rounded bg-muted/50">
                <span className="font-medium">{pg.i18n?.name || pg.id}</span>
                {pg.policy && (
                  <div className="mt-1 text-muted-foreground">
                    {pg.policy.allow_list && <span>Allow: {pg.policy.allow_list.join(", ")}</span>}
                    {pg.policy.deny_list && <span className="ml-2">Deny: {pg.policy.deny_list.join(", ")}</span>}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <Separator />

      {/* Pages */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.pages")}</h4>
        {pages.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noPages")}</p>
        ) : (
          <div className="space-y-1">
            {pages.map((page) => (
              <div key={page.id} className="flex justify-between text-xs p-2 rounded bg-muted/50">
                <span>{page.i18n?.name || page.id}</span>
                <span className="text-muted-foreground font-mono">{page.slot || "-"}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
