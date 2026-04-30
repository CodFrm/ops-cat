import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  cn,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@opskat/ui";
import { useAssetStore } from "@/stores/assetStore";
import {
  SelectImportFile,
  StartGitHubDeviceFlow,
  WaitGitHubDeviceAuth,
  CancelGitHubAuth,
  GetGitHubUser,
  ExportToGist,
  ListBackupGists,
  ImportFromGist,
  GetGitHubToken,
  GetStoredGitHubUser,
  SaveGitHubToken,
  ClearGitHubToken,
  GetWebDAVConfig,
  SaveWebDAVConfig,
  ClearWebDAVConfig,
  TestWebDAVConfig,
  ListWebDAVBackups,
  ExportToWebDAV,
  ImportFromWebDAV,
} from "../../../wailsjs/go/app/App";
import { backup_svc } from "../../../wailsjs/go/models";
import { ExportDialog } from "@/components/settings/ExportDialog";
import { BackupImportDialog } from "@/components/settings/BackupImportDialog";
import {
  Download,
  Upload,
  Github,
  LogOut,
  Loader2,
  Copy,
  ExternalLink,
  Eye,
  EyeOff,
  Shuffle,
  Cloud,
  Save,
  Trash2,
  RefreshCw,
} from "lucide-react";
import { toast } from "sonner";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";
import { useShortcutStore } from "@/stores/shortcutStore";
import { useTerminalThemeStore } from "@/stores/terminalThemeStore";

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));

function PasswordInput({
  showGenerate,
  onGenerate,
  className,
  ...props
}: React.ComponentProps<typeof Input> & {
  showGenerate?: boolean;
  onGenerate?: (password: string) => void;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        {...props}
        type={visible ? "text" : "password"}
        className={cn(showGenerate ? "pr-18" : "pr-9", className)}
      />
      <div className="absolute right-1 top-1/2 -translate-y-1/2 flex">
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => setVisible(!visible)}>
          {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
        </Button>
        {showGenerate && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={() => {
              const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*";
              const values = crypto.getRandomValues(new Uint8Array(20));
              const p = Array.from(values, (v) => charset[v % charset.length]).join("");
              setVisible(true);
              onGenerate?.(p);
            }}
          >
            <Shuffle className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </div>
  );
}

export function BackupSection() {
  const { t } = useTranslation();
  const { refresh } = useAssetStore();

  // File backup
  const [exportDialogOpen, setExportDialogOpen] = useState(false);
  const [exportDialogMode, setExportDialogMode] = useState<"file" | "gist" | "webdav">("file");
  const [backupImportOpen, setBackupImportOpen] = useState(false);
  const [backupImportFilePath, setBackupImportFilePath] = useState("");
  const [backupImportEncrypted, setBackupImportEncrypted] = useState(false);
  const [backupImportSummary, setBackupImportSummary] = useState<backup_svc.BackupSummary | null>(null);

  // GitHub
  const [ghToken, setGhToken] = useState("");
  const [ghUser, setGhUser] = useState("");
  const [deviceFlowOpen, setDeviceFlowOpen] = useState(false);
  const [deviceFlowInfo, setDeviceFlowInfo] = useState<backup_svc.DeviceFlowInfo | null>(null);
  const [ghLoggingIn, setGhLoggingIn] = useState(false);

  // Gist
  const [gists, setGists] = useState<backup_svc.GistInfo[]>([]);
  const [selectedGistId, setSelectedGistId] = useState("");
  const [gistPushing, setGistPushing] = useState(false);
  const [gistPulling, setGistPulling] = useState(false);
  const [gistPullPasswordOpen, setGistPullPasswordOpen] = useState(false);
  const [gistPullPassword, setGistPullPassword] = useState("");

  // WebDAV
  const [webdavConfigured, setWebDAVConfigured] = useState(false);
  const [webdavURL, setWebDAVURL] = useState("");
  const [webdavAuthType, setWebDAVAuthType] = useState<"none" | "basic" | "bearer">("basic");
  const [webdavUsername, setWebDAVUsername] = useState("");
  const [webdavPassword, setWebDAVPassword] = useState("");
  const [webdavToken, setWebDAVToken] = useState("");
  const [webdavBackups, setWebDAVBackups] = useState<backup_svc.WebDAVBackupInfo[]>([]);
  const [selectedWebDAVBackup, setSelectedWebDAVBackup] = useState("");
  const [webdavSaving, setWebDAVSaving] = useState(false);
  const [webdavTesting, setWebDAVTesting] = useState(false);
  const [webdavPushing, setWebDAVPushing] = useState(false);
  const [webdavPulling, setWebDAVPulling] = useState(false);
  const [webdavPullPasswordOpen, setWebDAVPullPasswordOpen] = useState(false);
  const [webdavPullPassword, setWebDAVPullPassword] = useState("");

  // Load GitHub token on mount
  useEffect(() => {
    (async () => {
      try {
        const token = await GetGitHubToken();
        const user = await GetStoredGitHubUser();
        if (token) {
          setGhToken(token);
          setGhUser(user || "");
          GetGitHubUser(token)
            .then((u) => {
              setGhUser(u.login);
              SaveGitHubToken(token, u.login).catch(() => {});
            })
            .catch(() => {
              setGhToken("");
              setGhUser("");
              ClearGitHubToken().catch(() => {});
            });
        }
      } catch {
        /* not configured */
      }
    })();
  }, []);

  const loadGists = useCallback(async () => {
    if (!ghToken) return;
    try {
      const list = await ListBackupGists(ghToken);
      setGists(list || []);
    } catch {
      setGists([]);
    }
  }, [ghToken]);

  useEffect(() => {
    loadGists();
  }, [loadGists]);

  useEffect(() => {
    (async () => {
      try {
        const cfg = await GetWebDAVConfig();
        if (!cfg) return;
        setWebDAVURL(cfg.url || "");
        const known: ReadonlyArray<"none" | "basic" | "bearer"> = ["none", "basic", "bearer"];
        const incoming = cfg.authType ?? "";
        setWebDAVAuthType(
          known.includes(incoming as "none" | "basic" | "bearer") ? (incoming as "none" | "basic" | "bearer") : "basic"
        );
        setWebDAVUsername(cfg.username || "");
        setWebDAVPassword(cfg.password || "");
        setWebDAVToken(cfg.token || "");
        setWebDAVConfigured(!!cfg.configured);
      } catch {
        /* not configured */
      }
    })();
  }, []);

  const loadWebDAVBackups = useCallback(async () => {
    if (!webdavConfigured) return;
    try {
      const list = await ListWebDAVBackups();
      const backups = list || [];
      setWebDAVBackups(backups);
      setSelectedWebDAVBackup((current) =>
        current && backups.some((backup) => backup.name === current) ? current : (backups[0]?.name ?? "")
      );
    } catch {
      setWebDAVBackups([]);
      setSelectedWebDAVBackup("");
    }
  }, [webdavConfigured]);

  useEffect(() => {
    loadWebDAVBackups();
  }, [loadWebDAVBackups]);

  const applyImportResult = useCallback(
    async (result: backup_svc.ImportResult) => {
      if (result.shortcuts) {
        try {
          const parsed = JSON.parse(result.shortcuts);
          localStorage.setItem("keyboard_shortcuts", JSON.stringify(parsed));
          const store = useShortcutStore.getState();
          store.resetAll();
          for (const [action, binding] of Object.entries(parsed)) {
            store.updateShortcut(action as never, binding as never);
          }
        } catch {
          // ignore parse errors
        }
      }

      if (result.custom_themes) {
        try {
          const themes = JSON.parse(result.custom_themes);
          const store = useTerminalThemeStore.getState();
          for (const theme of themes) {
            if (theme?.id) {
              store.removeCustomTheme(theme.id);
            }
            store.addCustomTheme(theme);
          }
        } catch {
          // ignore parse errors
        }
      }

      await refresh();
    },
    [refresh]
  );

  // --- File backup ---
  const handleFileExport = () => {
    setExportDialogMode("file");
    setExportDialogOpen(true);
  };

  const handleFileImport = async () => {
    try {
      const info = await SelectImportFile();
      if (!info || !info.filePath) return;
      setBackupImportFilePath(info.filePath);
      setBackupImportEncrypted(info.encrypted);
      setBackupImportSummary(info.summary ?? null);
      setBackupImportOpen(true);
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  // --- GitHub Auth ---
  const handleGitHubLogin = async () => {
    setGhLoggingIn(true);
    try {
      const info = await StartGitHubDeviceFlow();
      setDeviceFlowInfo(info);
      setDeviceFlowOpen(true);

      const token = await WaitGitHubDeviceAuth(info.deviceCode, info.interval);
      setDeviceFlowOpen(false);
      setGhToken(token);

      const user = await GetGitHubUser(token);
      setGhUser(user.login);
      await SaveGitHubToken(token, user.login);
      toast.success(t("backup.gistLoggedIn", { user: user.login }));
    } catch (e: unknown) {
      if (!String(e).includes("\u53D6\u6D88")) {
        toast.error(errMsg(e));
      }
    } finally {
      setDeviceFlowOpen(false);
      setGhLoggingIn(false);
    }
  };

  const handleGitHubLogout = () => {
    setGhToken("");
    setGhUser("");
    setGists([]);
    ClearGitHubToken().catch(() => {});
  };

  const handleCancelDeviceFlow = () => {
    CancelGitHubAuth().catch(() => {});
    setDeviceFlowOpen(false);
  };

  // --- Gist ---
  const handleGistPush = () => {
    setExportDialogMode("gist");
    setExportDialogOpen(true);
  };

  const handleGistExport = async (password: string, opts: backup_svc.ExportOptions) => {
    if (!password) {
      throw new Error(t("backup.passwordRequired"));
    }
    setGistPushing(true);
    try {
      const gistId = selectedGistId === "__new__" ? "" : selectedGistId;
      const result = await ExportToGist(password, ghToken, gistId, opts);
      if (result) {
        await loadGists();
        setSelectedGistId(result.id);
      }
    } finally {
      setGistPushing(false);
    }
  };

  const handleGistPull = async () => {
    if (!selectedGistId || selectedGistId === "__new__") {
      toast.error(t("backup.gistNoBackup"));
      return;
    }
    setGistPullPassword("");
    setGistPullPasswordOpen(true);
  };

  const doGistPull = async () => {
    if (!gistPullPassword) {
      toast.error(t("backup.passwordRequired"));
      return;
    }
    setGistPullPasswordOpen(false);
    setGistPulling(true);
    try {
      const opts = new backup_svc.ImportOptions({
        import_assets: true,
        import_credentials: true,
        import_forwards: true,
        import_policy_groups: true,
        import_shortcuts: true,
        import_themes: true,
        mode: "replace",
      });
      const result = await ImportFromGist(selectedGistId, gistPullPassword, ghToken, opts);
      await applyImportResult(result);
      toast.success(t("backup.gistPullSuccess"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setGistPulling(false);
    }
  };

  // --- WebDAV ---
  const buildWebDAVInput = () => ({
    url: webdavURL.trim(),
    authType: webdavAuthType,
    username: webdavUsername.trim(),
    password: webdavPassword,
    token: webdavToken.trim(),
  });

  const handleWebDAVSave = async () => {
    if (!webdavURL.trim()) {
      toast.error(t("backup.webdavURLRequired"));
      return;
    }
    setWebDAVSaving(true);
    try {
      await SaveWebDAVConfig(buildWebDAVInput());
      setWebDAVConfigured(true);
      toast.success(t("backup.webdavSaved"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setWebDAVSaving(false);
    }
  };

  const handleWebDAVTest = async () => {
    if (!webdavURL.trim()) {
      toast.error(t("backup.webdavURLRequired"));
      return;
    }
    setWebDAVTesting(true);
    try {
      await TestWebDAVConfig(buildWebDAVInput());
      toast.success(t("backup.webdavTestSuccess"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setWebDAVTesting(false);
    }
  };

  const handleWebDAVClear = async () => {
    try {
      await ClearWebDAVConfig();
      setWebDAVConfigured(false);
      setWebDAVURL("");
      setWebDAVAuthType("basic");
      setWebDAVUsername("");
      setWebDAVPassword("");
      setWebDAVToken("");
      setWebDAVBackups([]);
      setSelectedWebDAVBackup("");
      toast.success(t("backup.webdavCleared"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  const handleWebDAVPush = () => {
    if (!webdavConfigured) {
      toast.error(t("backup.webdavNotConfigured"));
      return;
    }
    setExportDialogMode("webdav");
    setExportDialogOpen(true);
  };

  const handleWebDAVExport = async (password: string, opts: backup_svc.ExportOptions) => {
    if (!password) {
      throw new Error(t("backup.passwordRequired"));
    }
    setWebDAVPushing(true);
    try {
      const result = await ExportToWebDAV(password, opts);
      await loadWebDAVBackups();
      if (result?.name) {
        setSelectedWebDAVBackup(result.name);
      }
    } finally {
      setWebDAVPushing(false);
    }
  };

  const handleWebDAVPull = async () => {
    if (!selectedWebDAVBackup) {
      toast.error(t("backup.webdavNoBackup"));
      return;
    }
    setWebDAVPullPassword("");
    setWebDAVPullPasswordOpen(true);
  };

  const doWebDAVPull = async () => {
    if (!webdavPullPassword) {
      toast.error(t("backup.passwordRequired"));
      return;
    }
    setWebDAVPullPasswordOpen(false);
    setWebDAVPulling(true);
    try {
      const opts = new backup_svc.ImportOptions({
        import_assets: true,
        import_credentials: true,
        import_forwards: true,
        import_policy_groups: true,
        import_shortcuts: true,
        import_themes: true,
        mode: "replace",
      });
      const result = await ImportFromWebDAV(selectedWebDAVBackup, webdavPullPassword, opts);
      await applyImportResult(result);
      toast.success(t("backup.webdavPullSuccess"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setWebDAVPulling(false);
    }
  };

  return (
    <>
      {/* File backup */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("backup.file")}</CardTitle>
          <CardDescription>{t("backup.fileDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex gap-2">
          <Button onClick={handleFileExport} variant="outline" className="gap-1">
            <Download className="h-4 w-4" />
            {t("backup.export")}
          </Button>
          <Button onClick={handleFileImport} variant="outline" className="gap-1">
            <Upload className="h-4 w-4" />
            {t("backup.import")}
          </Button>
        </CardContent>
      </Card>

      {/* Gist backup */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-1.5">
            <Github className="h-4 w-4" />
            {t("backup.gist")}
          </CardTitle>
          <CardDescription>{t("backup.gistDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!ghToken ? (
            <Button onClick={handleGitHubLogin} disabled={ghLoggingIn} variant="outline" className="gap-1">
              <Github className="h-4 w-4" />
              {ghLoggingIn ? t("backup.deviceFlowWaiting") : t("backup.gistLogin")}
            </Button>
          ) : (
            <>
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">{t("backup.gistLoggedIn", { user: ghUser })}</span>
                <Button variant="ghost" size="sm" onClick={handleGitHubLogout} className="gap-1">
                  <LogOut className="h-3.5 w-3.5" />
                  {t("backup.gistLogout")}
                </Button>
              </div>
              <div className="grid gap-2">
                <Label>{t("backup.gistSelect")}</Label>
                <Select value={selectedGistId} onValueChange={setSelectedGistId}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("backup.gistSelect")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__new__">{t("backup.gistCreateNew")}</SelectItem>
                    {gists.map((g) => (
                      <SelectItem key={g.id} value={g.id}>
                        {t("backup.gistUpdate", { desc: g.description })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex gap-2">
                <Button onClick={handleGistPush} disabled={gistPushing} variant="outline" className="gap-1">
                  <Upload className="h-4 w-4" />
                  {gistPushing ? t("backup.gistPushing") : t("backup.gistPush")}
                </Button>
                <Button
                  onClick={handleGistPull}
                  disabled={gistPulling || !selectedGistId || selectedGistId === "__new__"}
                  variant="outline"
                  className="gap-1"
                >
                  <Download className="h-4 w-4" />
                  {gistPulling ? t("backup.gistPulling") : t("backup.gistPull")}
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* WebDAV backup */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-1.5">
            <Cloud className="h-4 w-4" />
            {t("backup.webdav")}
          </CardTitle>
          <CardDescription>{t("backup.webdavDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label>{t("backup.webdavURL")}</Label>
              <Input
                value={webdavURL}
                onChange={(e) => setWebDAVURL(e.target.value)}
                placeholder={t("backup.webdavURLPlaceholder")}
              />
              <p className="text-xs text-muted-foreground">{t("backup.webdavDefaultDirectory", { dir: "opskat" })}</p>
            </div>
            <div className="grid gap-1.5">
              <Label>{t("backup.webdavAuthType")}</Label>
              <Select value={webdavAuthType} onValueChange={(v) => setWebDAVAuthType(v as "none" | "basic" | "bearer")}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">{t("backup.webdavAuthNone")}</SelectItem>
                  <SelectItem value="basic">{t("backup.webdavAuthBasic")}</SelectItem>
                  <SelectItem value="bearer">{t("backup.webdavAuthBearer")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {webdavAuthType === "basic" && (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label>{t("backup.webdavUsername")}</Label>
                  <Input
                    value={webdavUsername}
                    onChange={(e) => setWebDAVUsername(e.target.value)}
                    placeholder={t("backup.webdavUsernamePlaceholder")}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label>{t("backup.webdavPassword")}</Label>
                  <PasswordInput value={webdavPassword} onChange={(e) => setWebDAVPassword(e.target.value)} />
                </div>
              </div>
            )}
            {webdavAuthType === "bearer" && (
              <div className="grid gap-1.5">
                <Label>{t("backup.webdavToken")}</Label>
                <PasswordInput value={webdavToken} onChange={(e) => setWebDAVToken(e.target.value)} />
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Button onClick={handleWebDAVSave} disabled={webdavSaving} variant="outline" className="gap-1">
                {webdavSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                {t("backup.webdavSave")}
              </Button>
              <Button
                onClick={handleWebDAVTest}
                disabled={webdavTesting || !webdavURL.trim()}
                variant="outline"
                className="gap-1"
              >
                {webdavTesting ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                {t("backup.webdavTest")}
              </Button>
              {webdavConfigured && (
                <Button onClick={handleWebDAVClear} variant="ghost" className="gap-1 text-muted-foreground">
                  <Trash2 className="h-4 w-4" />
                  {t("backup.webdavClear")}
                </Button>
              )}
            </div>
          </div>

          {webdavConfigured && (
            <>
              <div className="grid gap-2">
                <Label>{t("backup.webdavSelect")}</Label>
                <div className="flex gap-2">
                  <Select value={selectedWebDAVBackup} onValueChange={setSelectedWebDAVBackup}>
                    <SelectTrigger className="flex-1">
                      <SelectValue placeholder={t("backup.webdavNoBackup")} />
                    </SelectTrigger>
                    <SelectContent>
                      {webdavBackups.map((backup) => (
                        <SelectItem key={backup.name} value={backup.name}>
                          {t("backup.webdavBackupItem", { name: backup.name, size: backup.size })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button onClick={loadWebDAVBackups} variant="outline" size="icon">
                    <RefreshCw className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <div className="flex gap-2">
                <Button onClick={handleWebDAVPush} disabled={webdavPushing} variant="outline" className="gap-1">
                  <Upload className="h-4 w-4" />
                  {webdavPushing ? t("backup.webdavPushing") : t("backup.webdavPush")}
                </Button>
                <Button
                  onClick={handleWebDAVPull}
                  disabled={webdavPulling || !selectedWebDAVBackup}
                  variant="outline"
                  className="gap-1"
                >
                  <Download className="h-4 w-4" />
                  {webdavPulling ? t("backup.webdavPulling") : t("backup.webdavPull")}
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* Export dialog */}
      <ExportDialog
        open={exportDialogOpen}
        onOpenChange={setExportDialogOpen}
        mode={exportDialogMode}
        onGistExport={handleGistExport}
        onWebDAVExport={handleWebDAVExport}
      />

      {/* Backup import dialog */}
      <BackupImportDialog
        open={backupImportOpen}
        onOpenChange={setBackupImportOpen}
        filePath={backupImportFilePath}
        encrypted={backupImportEncrypted}
        initialSummary={backupImportSummary}
      />

      {/* GitHub Device Flow dialog */}
      <Dialog open={deviceFlowOpen}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{t("backup.deviceFlow")}</DialogTitle>
            <DialogDescription>{t("backup.deviceFlowDesc")}</DialogDescription>
          </DialogHeader>
          {deviceFlowInfo && (
            <div className="space-y-4">
              <div className="flex items-center justify-center gap-2">
                <code className="rounded bg-muted px-3 py-2 text-2xl font-mono font-bold tracking-widest">
                  {deviceFlowInfo.userCode}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    navigator.clipboard.writeText(deviceFlowInfo.userCode);
                    toast.success("Copied");
                  }}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
              <Button className="w-full gap-1" onClick={() => BrowserOpenURL(deviceFlowInfo.verificationUri)}>
                <ExternalLink className="h-4 w-4" />
                {t("backup.deviceFlowOpen")}
              </Button>
              <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                {t("backup.deviceFlowWaiting")}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={handleCancelDeviceFlow}>
              {t("action.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Gist pull password dialog */}
      <Dialog open={gistPullPasswordOpen} onOpenChange={setGistPullPasswordOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("backup.gistPull")}</DialogTitle>
            <DialogDescription>{t("backup.enterPassword")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label>{t("backup.password")}</Label>
            <PasswordInput
              value={gistPullPassword}
              onChange={(e) => setGistPullPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && doGistPull()}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setGistPullPasswordOpen(false)}>
              {t("action.cancel")}
            </Button>
            <Button onClick={doGistPull} disabled={!gistPullPassword}>
              {t("backup.gistPull")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* WebDAV pull password dialog */}
      <Dialog open={webdavPullPasswordOpen} onOpenChange={setWebDAVPullPasswordOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("backup.webdavPull")}</DialogTitle>
            <DialogDescription>{t("backup.enterPassword")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label>{t("backup.password")}</Label>
            <PasswordInput
              value={webdavPullPassword}
              onChange={(e) => setWebDAVPullPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && doWebDAVPull()}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setWebDAVPullPasswordOpen(false)}>
              {t("action.cancel")}
            </Button>
            <Button onClick={doWebDAVPull} disabled={!webdavPullPassword}>
              {t("backup.webdavPull")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
