import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Input,
  Label,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Separator,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@opskat/ui";
import {
  DetectOpsctl,
  GetOpsctlInstallDir,
  InstallOpsctl,
  DetectSkills,
  InstallSkills,
  GetSkillPreview,
  GetDataDir,
  GetAppVersion,
  OpenDirectory,
  GetPluginReferenceDir,
  ListAIProviders,
  CreateAIProvider,
  UpdateAIProvider,
  DeleteAIProvider,
  SetActiveAIProvider,
} from "../../../wailsjs/go/app/App";
import { app } from "../../../wailsjs/go/models";
import {
  Check,
  Loader2,
  ExternalLink,
  ChevronDown,
  ChevronUp,
  Info,
  FolderOpen,
  Pencil,
  RefreshCw,
  Trash2,
  Plus,
} from "lucide-react";
import { toast } from "sonner";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";
import { AIProviderForm, type AIProviderFormValues } from "@/components/ai/AIProviderForm";

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));

function IntegrationSection() {
  const { t } = useTranslation();
  const [opsctlInfo, setOpsctlInfo] = useState<{
    installed: boolean;
    path: string;
    version: string;
    embedded: boolean;
  }>({ installed: false, path: "", version: "", embedded: false });
  const [skillTargets, setSkillTargets] = useState<{ name: string; installed: boolean; path: string }[]>([]);
  const [installDir, setInstallDir] = useState("");
  const [installing, setInstalling] = useState(false);
  const [skillInstalling, setSkillInstalling] = useState(false);
  const [skillPreview, setSkillPreview] = useState("");
  const [showPreview, setShowPreview] = useState(false);
  const [dataDir, setDataDir] = useState("");
  const [appVersion, setAppVersion] = useState("");

  const detect = useCallback(async () => {
    try {
      const [info, skills, dir, dd, ver] = await Promise.all([
        DetectOpsctl(),
        DetectSkills(),
        GetOpsctlInstallDir(),
        GetDataDir(),
        GetAppVersion(),
      ]);
      setOpsctlInfo(info);
      setSkillTargets(skills || []);
      setInstallDir(dir);
      setDataDir(dd);
      setAppVersion(ver);
    } catch {
      // detection is optional
    }
  }, []);

  useEffect(() => {
    detect();
  }, [detect]);

  const handleInstallCLI = async () => {
    setInstalling(true);
    try {
      await InstallOpsctl(installDir);
      toast.success(t("integration.installSuccess"));
      await detect();
      toast.info(`${t("integration.pathHint")}: ${installDir}`);
    } catch (e: unknown) {
      toast.error(`${t("integration.installFailed")}: ${errMsg(e)}`);
    } finally {
      setInstalling(false);
    }
  };

  const handleInstallSkill = async () => {
    setSkillInstalling(true);
    try {
      await InstallSkills();
      toast.success(t("integration.skillInstallSuccess"));
      await detect();
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setSkillInstalling(false);
    }
  };

  const handlePreview = async () => {
    if (showPreview) {
      setShowPreview(false);
      return;
    }
    try {
      const content = await GetSkillPreview();
      setSkillPreview(content);
      setShowPreview(true);
    } catch {
      // preview is optional
    }
  };

  return (
    <>
      {/* opsctl CLI */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base">{t("integration.cli")}</CardTitle>
              <CardDescription>{t("integration.cliDesc")}</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              {opsctlInfo.installed ? (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-green-600 dark:text-green-400">
                  <Check className="h-3.5 w-3.5" />
                  {t("integration.installed")}
                </span>
              ) : (
                <span className="text-xs text-muted-foreground">{t("integration.notInstalled")}</span>
              )}
              <Button variant="ghost" size="icon" className="h-7 w-7" onClick={detect}>
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {opsctlInfo.installed && (
            <div className="space-y-3">
              <div className="grid gap-1.5 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t("integration.version")}</span>
                  <span className="font-mono text-xs">{opsctlInfo.version || "unknown"}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t("integration.path")}</span>
                  <span className="font-mono text-xs truncate max-w-[300px]">{opsctlInfo.path}</span>
                </div>
              </div>
              {appVersion && opsctlInfo.version && opsctlInfo.version !== appVersion && (
                <div className="flex items-center gap-2 rounded-md bg-yellow-50 dark:bg-yellow-950 border border-yellow-200 dark:border-yellow-800 px-3 py-2 text-xs text-yellow-700 dark:text-yellow-300">
                  <Info className="h-3.5 w-3.5 shrink-0" />
                  <span>{t("integration.versionMismatch", { appVersion, cliVersion: opsctlInfo.version })}</span>
                </div>
              )}
              {opsctlInfo.embedded && (
                <div className="space-y-2">
                  <div className="grid gap-1.5">
                    <Label className="text-sm">{t("integration.installDir")}</Label>
                    <Input
                      value={installDir}
                      onChange={(e) => setInstallDir(e.target.value)}
                      className="font-mono text-xs h-8"
                    />
                  </div>
                  <Button onClick={handleInstallCLI} disabled={installing} size="sm" variant="outline">
                    {installing ? (
                      <>
                        <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                        {t("integration.installing")}
                      </>
                    ) : (
                      t("integration.reinstall")
                    )}
                  </Button>
                </div>
              )}
            </div>
          )}

          {!opsctlInfo.installed && (
            <div className="space-y-3">
              {opsctlInfo.embedded ? (
                <div className="space-y-2">
                  <div className="grid gap-1.5">
                    <Label className="text-sm">{t("integration.installDir")}</Label>
                    <Input
                      value={installDir}
                      onChange={(e) => setInstallDir(e.target.value)}
                      className="font-mono text-xs h-8"
                    />
                  </div>
                  <Button onClick={handleInstallCLI} disabled={installing} size="sm">
                    {installing ? (
                      <>
                        <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                        {t("integration.installing")}
                      </>
                    ) : (
                      t("integration.install")
                    )}
                  </Button>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">{t("integration.noEmbedded")}</p>
              )}
              <Separator />
              <div className="space-y-1">
                <p className="text-sm font-medium">{t("integration.manualInstall")}</p>
                <p className="text-xs text-muted-foreground">{t("integration.manualInstallHint")}</p>
                <Button
                  variant="link"
                  size="sm"
                  className="h-auto p-0 text-xs"
                  onClick={() => BrowserOpenURL("https://github.com/opskat/opskat/release")}
                >
                  <ExternalLink className="h-3 w-3 mr-1" />
                  GitHub Releases
                </Button>
              </div>
            </div>
          )}

          <Separator />
          <div className="space-y-1">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">{t("integration.dataDir")}</span>
              <span className="font-mono text-xs truncate max-w-[300px]">{dataDir}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t("integration.dataDirDesc")}</p>
          </div>
        </CardContent>
      </Card>

      {/* AI Skill */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base">{t("integration.skill")}</CardTitle>
              <CardDescription>{t("integration.skillDesc")}</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              {skillTargets
                .filter((s) => s.installed)
                .map((s) => (
                  <span
                    key={s.name}
                    className="inline-flex items-center gap-1 text-xs font-medium text-green-600 dark:text-green-400"
                  >
                    <Check className="h-3.5 w-3.5" />
                    {s.name}
                  </span>
                ))}
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {skillTargets.some((s) => s.installed) && (
            <div className="space-y-1">
              {skillTargets
                .filter((s) => s.installed)
                .map((s) => (
                  <div key={s.name} className="flex items-center justify-between text-sm gap-2">
                    <span className="text-muted-foreground shrink-0">{s.name}</span>
                    <div className="flex items-center gap-1 min-w-0">
                      <span className="font-mono text-xs truncate">{s.path}</span>
                      <button
                        type="button"
                        className="shrink-0 p-0.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
                        onClick={() => OpenDirectory(s.path)}
                        title={t("integration.openDir")}
                      >
                        <FolderOpen className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
            </div>
          )}

          <div className="flex gap-2">
            <Button onClick={handleInstallSkill} disabled={skillInstalling} size="sm">
              {skillInstalling ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                  {t("integration.skillInstalling")}
                </>
              ) : skillTargets.every((s) => s.installed) ? (
                t("integration.skillUpdate")
              ) : (
                t("integration.skillInstall")
              )}
            </Button>
            <Button variant="outline" size="sm" onClick={handlePreview}>
              {showPreview ? <ChevronUp className="h-3.5 w-3.5 mr-1" /> : <ChevronDown className="h-3.5 w-3.5 mr-1" />}
              {t("integration.skillPreview")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={async () => {
                const dir = await GetPluginReferenceDir();
                OpenDirectory(dir);
              }}
            >
              <FolderOpen className="h-3.5 w-3.5 mr-1" />
              {t("integration.openDir")}
            </Button>
          </div>

          {showPreview && (
            <pre className="text-xs bg-muted p-3 rounded-md overflow-auto max-h-[300px] whitespace-pre-wrap">
              {skillPreview}
            </pre>
          )}
        </CardContent>
      </Card>
    </>
  );
}

export function AISettingsSection() {
  const { t } = useTranslation();
  const [providers, setProviders] = useState<app.AIProviderInfo[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<app.AIProviderInfo | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<app.AIProviderInfo | null>(null);
  const [saving, setSaving] = useState(false);

  const loadProviders = useCallback(async () => {
    try {
      const list = await ListAIProviders();
      setProviders(list || []);
    } catch (e) {
      toast.error(errMsg(e));
    }
  }, []);

  useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  const openAddDialog = () => {
    setEditingProvider(null);
    setDialogOpen(true);
  };

  const openEditDialog = (provider: app.AIProviderInfo) => {
    setEditingProvider(provider);
    setDialogOpen(true);
  };

  const handleSave = async (values: AIProviderFormValues) => {
    setSaving(true);
    try {
      if (editingProvider) {
        await UpdateAIProvider(
          editingProvider.id,
          values.name,
          values.type,
          values.apiBase,
          values.apiKey,
          values.model,
          values.maxOutputTokens,
          values.contextWindow
        );
      } else {
        const created = await CreateAIProvider(
          values.name,
          values.type,
          values.apiBase,
          values.apiKey,
          values.model,
          values.maxOutputTokens,
          values.contextWindow
        );
        if (providers.length === 0 && created.id) {
          await SetActiveAIProvider(created.id);
        }
      }
      toast.success(t("settings.saved"));
      setDialogOpen(false);
      await loadProviders();
    } catch (e) {
      toast.error(errMsg(e));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await DeleteAIProvider(deleteTarget.id);
      toast.success(t("settings.saved"));
      setDeleteTarget(null);
      await loadProviders();
    } catch (e) {
      toast.error(errMsg(e));
    }
  };

  const handleSetActive = async (id: number) => {
    try {
      await SetActiveAIProvider(id);
      await loadProviders();
    } catch (e) {
      toast.error(errMsg(e));
    }
  };

  return (
    <>
      {/* Provider list */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("settings.providers")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {providers.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("settings.noProviders")}</p>
          ) : (
            <div className="space-y-2">
              {providers.map((provider) => (
                <div
                  key={provider.id}
                  className={`flex items-center justify-between rounded-md border p-3 ${
                    provider.isActive ? "border-primary bg-primary/5" : ""
                  }`}
                >
                  <div className="flex items-center gap-2 min-w-0">
                    {provider.isActive && <Check className="h-4 w-4 text-primary shrink-0" />}
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium truncate">{provider.name}</span>
                        <span className="text-xs text-muted-foreground">
                          {provider.type === "anthropic" ? t("settings.anthropic") : t("settings.openai")}
                        </span>
                      </div>
                      <span className="text-xs text-muted-foreground">{provider.model}</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {!provider.isActive && (
                      <Button variant="ghost" size="sm" onClick={() => handleSetActive(provider.id)}>
                        {t("settings.setActive")}
                      </Button>
                    )}
                    <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEditDialog(provider)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => setDeleteTarget(provider)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
          <Button variant="outline" size="sm" onClick={openAddDialog}>
            <Plus className="h-3.5 w-3.5 mr-1.5" />
            {t("settings.addProvider")}
          </Button>
        </CardContent>
      </Card>

      {/* Add/Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editingProvider ? t("settings.editProvider") : t("settings.addProvider")}</DialogTitle>
            <DialogDescription>
              {editingProvider ? t("settings.editProvider") : t("settings.addProvider")}
            </DialogDescription>
          </DialogHeader>
          <AIProviderForm
            key={editingProvider?.id ?? "new"}
            initialValues={
              editingProvider
                ? {
                    name: editingProvider.name,
                    type: editingProvider.type,
                    apiBase: editingProvider.apiBase,
                    apiKey: editingProvider.apiKey || "",
                    maskedApiKey: editingProvider.maskedApiKey,
                    model: editingProvider.model,
                    maxOutputTokens: editingProvider.maxOutputTokens,
                    contextWindow: editingProvider.contextWindow,
                  }
                : undefined
            }
            isEditing={!!editingProvider}
            showTypeSelector={true}
            onSave={handleSave}
            saving={saving}
            submitLabel={t("action.save")}
          />
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("settings.deleteProvider")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.deleteProviderConfirm")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("action.cancel")}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleDelete}>
              {t("action.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* CLI Integration */}
      <IntegrationSection />
    </>
  );
}
