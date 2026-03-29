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
} from "../../../wailsjs/go/app/App";
import { backup_svc } from "../../../wailsjs/go/models";
import { ExportDialog } from "@/components/settings/ExportDialog";
import { BackupImportDialog } from "@/components/settings/BackupImportDialog";
import { Download, Upload, Github, LogOut, Loader2, Copy, ExternalLink, Eye, EyeOff, Shuffle } from "lucide-react";
import { toast } from "sonner";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";

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
  const [exportDialogMode, setExportDialogMode] = useState<"file" | "gist">("file");
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
      toast.error(t("backup.passwordRequired"));
      return;
    }
    setGistPushing(true);
    try {
      const gistId = selectedGistId === "__new__" ? "" : selectedGistId;
      const result = await ExportToGist(password, ghToken, gistId, opts);
      toast.success(t("backup.gistPushSuccess"));
      if (result) {
        await loadGists();
        setSelectedGistId(result.id);
      }
    } catch (e: unknown) {
      toast.error(errMsg(e));
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
      await ImportFromGist(selectedGistId, gistPullPassword, ghToken, opts);
      toast.success(t("backup.gistPullSuccess"));
      await refresh();
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setGistPulling(false);
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

      {/* Export dialog */}
      <ExportDialog
        open={exportDialogOpen}
        onOpenChange={setExportDialogOpen}
        mode={exportDialogMode}
        onGistExport={handleGistExport}
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
    </>
  );
}
