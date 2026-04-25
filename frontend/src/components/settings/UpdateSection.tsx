import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Input,
  Switch,
} from "@opskat/ui";
import {
  GetAppVersion,
  GetUpdateChannel,
  SetUpdateChannel,
  CheckForUpdate,
  DownloadAndInstallUpdate,
  GetDownloadMirror,
  SetDownloadMirror,
  GetAvailableMirrors,
  GetBugReportInfo,
  GetDebugMode,
  SetDebugMode,
  OpenLogsDir,
} from "../../../wailsjs/go/app/App";
import { Bug, Download, FolderOpen, Github, Loader2, ExternalLink, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { BrowserOpenURL, Quit } from "../../../wailsjs/runtime/runtime";
import { EventsOn } from "../../../wailsjs/runtime/runtime";

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
const REPOSITORY_URL = "https://github.com/opskat/opskat";

export function UpdateSection() {
  const { t } = useTranslation();
  const [currentVersion, setCurrentVersion] = useState("");
  const [channel, setChannel] = useState("stable");
  const [checking, setChecking] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<{
    hasUpdate: boolean;
    latestVersion: string;
    releaseNotes: string;
    releaseURL: string;
    publishedAt: string;
  } | null>(null);
  const [updating, setUpdating] = useState(false);
  const [progress, setProgress] = useState(0);
  const [updateDone, setUpdateDone] = useState(false);
  const [mirror, setMirror] = useState("");
  const [mirrors, setMirrors] = useState<{ id: string; name: string; url: string }[]>([]);
  const [customMirror, setCustomMirror] = useState("");
  const [showChecksumDialog, setShowChecksumDialog] = useState(false);
  const [debugMode, setDebugModeState] = useState(false);

  useEffect(() => {
    GetAppVersion()
      .then(setCurrentVersion)
      .catch(() => {});
    GetUpdateChannel()
      .then(setChannel)
      .catch(() => {});
    GetDebugMode()
      .then(setDebugModeState)
      .catch(() => {});
    GetDownloadMirror()
      .then((m) => {
        setMirror(m);
        GetAvailableMirrors().then((list) => {
          setMirrors(list);
          if (m && !list.some((item) => item.url === m)) {
            setCustomMirror(m);
            setMirror("custom");
          }
        });
      })
      .catch(() => {});
  }, []);

  const handleDebugModeChange = async (enabled: boolean) => {
    setDebugModeState(enabled);
    try {
      await SetDebugMode(enabled);
      toast.success(enabled ? t("appUpdate.debugModeOn") : t("appUpdate.debugModeOff"));
    } catch (e: unknown) {
      setDebugModeState(!enabled);
      toast.error(errMsg(e));
    }
  };

  const handleOpenLogsDir = async () => {
    try {
      await OpenLogsDir();
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  const handleReportBug = async () => {
    const params = new URLSearchParams({ template: "bug_report.yml", labels: "bug" });
    try {
      const info = await GetBugReportInfo();
      const version = info.version + (info.commit ? ` (${info.commit})` : "");
      if (version.trim()) params.set("version", version);
      if (info.osLabel) params.set("os", info.osLabel);
    } catch {
      // 取不到诊断信息时仍然打开模板，让用户手动填写
    }
    BrowserOpenURL(`https://github.com/opskat/opskat/issues/new?${params.toString()}`);
  };

  const handleOpenRepository = () => {
    BrowserOpenURL(REPOSITORY_URL);
  };

  const handleChannelChange = async (value: string) => {
    setChannel(value);
    setUpdateInfo(null);
    try {
      await SetUpdateChannel(value);
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  const handleMirrorChange = async (value: string) => {
    if (value === "custom") {
      setMirror("custom");
      return;
    }
    setMirror(value);
    try {
      await SetDownloadMirror(value);
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  const handleCustomMirrorSave = async () => {
    const trimmed = customMirror.trim();
    try {
      await SetDownloadMirror(trimmed);
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };

  useEffect(() => {
    const cancelProgress = EventsOn("update:progress", (data: { downloaded: number; total: number }) => {
      if (data.total > 0) {
        setProgress(Math.round((data.downloaded / data.total) * 100));
      }
    });
    const cancelOpsctlErr = EventsOn("update:opsctl-error", (errMsg: string) => {
      toast.error(t("appUpdate.opsctlUpdateFailed", { error: errMsg }));
    });
    const cancelSkillErr = EventsOn("update:skill-error", (errMsg: string) => {
      toast.error(t("appUpdate.skillUpdateFailed", { error: errMsg }));
    });
    return () => {
      cancelProgress();
      cancelOpsctlErr();
      cancelSkillErr();
    };
  }, [t]);

  const handleCheck = async () => {
    setChecking(true);
    setUpdateInfo(null);
    try {
      const info = await CheckForUpdate();
      setUpdateInfo(info);
      if (!info.hasUpdate) {
        toast.success(t("appUpdate.latestVersion"));
      }
    } catch (e: unknown) {
      toast.error(`${t("appUpdate.checkFailed")}: ${errMsg(e)}`);
    } finally {
      setChecking(false);
    }
  };

  const handleUpdate = async (skipChecksum = false) => {
    setUpdating(true);
    setProgress(0);
    try {
      await DownloadAndInstallUpdate(skipChecksum);
      setUpdateDone(true);
      toast.success(t("appUpdate.updateSuccess"));
    } catch (e: unknown) {
      const msg = errMsg(e);
      if (msg.startsWith("CHECKSUM_FETCH_FAILED:")) {
        setShowChecksumDialog(true);
      } else {
        toast.error(`${t("appUpdate.updateFailed")}: ${msg}`);
      }
    } finally {
      setUpdating(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("appUpdate.title")}</CardTitle>
        <CardDescription>{t("appUpdate.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex justify-between items-center text-sm">
          <span className="text-muted-foreground">{t("appUpdate.currentVersion")}</span>
          <span className="font-mono text-xs">{currentVersion || "dev"}</span>
        </div>

        <div className="flex justify-between items-center text-sm">
          <span className="text-muted-foreground">{t("appUpdate.updateChannel")}</span>
          <Select value={channel} onValueChange={handleChannelChange}>
            <SelectTrigger className="w-[160px] h-8 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="stable">{t("appUpdate.channelStable")}</SelectItem>
              <SelectItem value="beta">{t("appUpdate.channelBeta")}</SelectItem>
              <SelectItem value="nightly">{t("appUpdate.channelNightly")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {channel === "nightly" && <p className="text-xs text-muted-foreground">{t("appUpdate.nightlyWarning")}</p>}
        {channel === "beta" && <p className="text-xs text-muted-foreground">{t("appUpdate.betaWarning")}</p>}

        <div className="flex justify-between items-center text-sm">
          <span className="text-muted-foreground">{t("appUpdate.downloadMirror")}</span>
          <Select
            value={mirror === "custom" ? "custom" : mirror || "__default__"}
            onValueChange={(v) => handleMirrorChange(v === "__default__" ? "" : v)}
          >
            <SelectTrigger className="w-[160px] h-8 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {mirrors.map((m) => (
                <SelectItem key={m.id} value={m.url || "__default__"}>
                  {m.id === "github" ? t("appUpdate.mirrorGithub") : m.name}
                </SelectItem>
              ))}
              <SelectItem value="custom">{t("appUpdate.mirrorCustom")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {mirror === "custom" && (
          <div className="flex gap-2">
            <Input
              className="h-8 text-xs flex-1"
              placeholder={t("appUpdate.mirrorCustomPlaceholder")}
              value={customMirror}
              onChange={(e) => setCustomMirror(e.target.value)}
              onBlur={handleCustomMirrorSave}
            />
          </div>
        )}

        <div className="flex justify-between items-start text-sm pt-2 border-t">
          <div className="space-y-0.5">
            <div>{t("appUpdate.debugMode")}</div>
            <div className="text-xs text-muted-foreground">{t("appUpdate.debugModeDesc")}</div>
          </div>
          <Switch checked={debugMode} onCheckedChange={handleDebugModeChange} />
        </div>

        <div className="flex gap-2 flex-wrap">
          <Button onClick={handleCheck} disabled={checking || updating} size="sm" variant="outline">
            {checking ? (
              <>
                <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                {t("appUpdate.checking")}
              </>
            ) : (
              <>
                <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
                {t("appUpdate.checkUpdate")}
              </>
            )}
          </Button>
          <Button onClick={handleReportBug} size="sm" variant="outline" title={t("appUpdate.reportBugDesc")}>
            <Bug className="h-3.5 w-3.5 mr-1.5" />
            {t("appUpdate.reportBug")}
          </Button>
          <Button onClick={handleOpenRepository} size="sm" variant="outline" title={t("appUpdate.openRepositoryDesc")}>
            <Github className="h-3.5 w-3.5 mr-1.5" />
            {t("appUpdate.openRepository")}
          </Button>
          <Button onClick={handleOpenLogsDir} size="sm" variant="outline">
            <FolderOpen className="h-3.5 w-3.5 mr-1.5" />
            {t("appUpdate.openLogsDir")}
          </Button>
        </div>

        {updateInfo?.hasUpdate && (
          <div className="space-y-3 border rounded-md p-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">
                {t("appUpdate.newVersion")}: {updateInfo.latestVersion}
              </span>
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0 text-xs"
                onClick={() => BrowserOpenURL(updateInfo.releaseURL)}
              >
                <ExternalLink className="h-3 w-3 mr-1" />
                {t("appUpdate.viewRelease")}
              </Button>
            </div>

            {updateInfo.releaseNotes && (
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">{t("appUpdate.releaseNotes")}</p>
                <pre className="text-xs bg-muted p-2 rounded-md overflow-auto max-h-[200px] whitespace-pre-wrap">
                  {updateInfo.releaseNotes}
                </pre>
              </div>
            )}

            {updating && (
              <div className="space-y-1">
                <div className="w-full bg-muted rounded-full h-2">
                  <div className="bg-primary h-2 rounded-full transition-all" style={{ width: `${progress}%` }} />
                </div>
                <p className="text-xs text-muted-foreground text-center">
                  {t("appUpdate.downloadProgress", { percent: progress })}
                </p>
              </div>
            )}

            {!updateDone ? (
              <Button onClick={() => handleUpdate()} disabled={updating} size="sm">
                {updating ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                    {t("appUpdate.downloading")}
                  </>
                ) : (
                  <>
                    <Download className="h-3.5 w-3.5 mr-1.5" />
                    {t("appUpdate.download")}
                  </>
                )}
              </Button>
            ) : (
              <div className="flex gap-2">
                <Button onClick={() => Quit()} size="sm">
                  {t("appUpdate.restartNow")}
                </Button>
                <Button variant="outline" size="sm" onClick={() => setUpdateDone(false)}>
                  {t("appUpdate.restartLater")}
                </Button>
              </div>
            )}
          </div>
        )}
        <AlertDialog open={showChecksumDialog} onOpenChange={setShowChecksumDialog}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("appUpdate.checksumSkipTitle")}</AlertDialogTitle>
              <AlertDialogDescription>
                {t("appUpdate.checksumFetchFailed")}
                <br />
                <br />
                {t("appUpdate.checksumSkipConfirm")}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("appUpdate.checksumSkipCancel")}</AlertDialogCancel>
              <AlertDialogAction onClick={() => handleUpdate(true)}>
                {t("appUpdate.checksumSkipAction")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  );
}
