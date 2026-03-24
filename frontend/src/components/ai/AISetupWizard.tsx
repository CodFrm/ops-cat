import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAIStore } from "@/stores/aiStore";
import {
  DetectOpsctl,
  DetectSkills,
  InstallSkills,
  InstallOpsctl,
  GetOpsctlInstallDir,
} from "../../../wailsjs/go/main/App";
import {
  Bot,
  Terminal,
  Check,
  Loader2,
  ArrowRight,
  Download,
  AlertTriangle,
} from "lucide-react";
import { toast } from "sonner";

export function AISetupWizard() {
  const { t } = useTranslation();
  const { configure, detectCLIs, localCLIs, fetchConversations } =
    useAIStore();

  // Provider selection
  const [providerType, setProviderType] = useState<string | null>(null);

  // Local CLI config
  const [cliPath, setCliPath] = useState("");
  const [cliType, setCliType] = useState("claude");

  // OpenAI config
  const [apiBase, setApiBase] = useState("https://api.openai.com/v1");
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState("gpt-4o");

  // Integration
  const [opsctlInfo, setOpsctlInfo] = useState<{
    installed: boolean;
    path: string;
    version: string;
    embedded: boolean;
  }>({ installed: false, path: "", version: "", embedded: false });
  const [skillsInstalled, setSkillsInstalled] = useState(false);
  const [installDir, setInstallDir] = useState("");
  const [opsctlInstalling, setOpsctlInstalling] = useState(false);
  const [completing, setCompleting] = useState(false);

  useEffect(() => {
    detectCLIs();
  }, [detectCLIs]);

  // Auto-fill CLI path from detected CLIs
  useEffect(() => {
    if (localCLIs.length > 0 && !cliPath) {
      const claude = localCLIs.find((c) => c.name === "claude");
      if (claude) {
        setCliPath(claude.path);
        setCliType("claude");
      } else {
        setCliPath(localCLIs[0].path);
        setCliType(localCLIs[0].name === "codex" ? "codex" : "claude");
      }
    }
  }, [localCLIs, cliPath]);

  const detectIntegration = useCallback(async () => {
    try {
      const [info, skills, dir] = await Promise.all([
        DetectOpsctl(),
        DetectSkills(),
        GetOpsctlInstallDir(),
      ]);
      setOpsctlInfo(info);
      setSkillsInstalled((skills || []).some((s: { installed: boolean }) => s.installed));
      setInstallDir(dir);
    } catch {}
  }, []);

  useEffect(() => {
    if (providerType === "local_cli") {
      detectIntegration();
    }
  }, [providerType, detectIntegration]);

  const handleInstallOpsctl = async () => {
    setOpsctlInstalling(true);
    try {
      await InstallOpsctl(installDir);
      toast.success(t("integration.installSuccess"));
      await detectIntegration();
    } catch (e: unknown) {
      const msg =
        e instanceof Error ? e.message : String(e);
      toast.error(`${t("integration.installFailed")}: ${msg}`);
    } finally {
      setOpsctlInstalling(false);
    }
  };

  const handleComplete = async () => {
    setCompleting(true);
    try {
      if (providerType === "local_cli") {
        // Auto-install skills
        if (!skillsInstalled) {
          try {
            await InstallSkills();
          } catch {
            // Non-blocking: user can install later from settings
          }
        }
        localStorage.setItem("ai_provider_type", "local_cli");
        localStorage.setItem("ai_api_base", cliPath);
        localStorage.setItem("ai_api_key", "");
        localStorage.setItem("ai_model", cliType);
        await configure("local_cli", cliPath, "", cliType);
      } else {
        localStorage.setItem("ai_provider_type", "openai");
        localStorage.setItem("ai_api_base", apiBase);
        localStorage.setItem("ai_api_key", apiKey);
        localStorage.setItem("ai_model", model);
        await configure("openai", apiBase, apiKey, model);
      }
      await fetchConversations();
    } catch (e: unknown) {
      toast.error(String(e));
    } finally {
      setCompleting(false);
    }
  };

  return (
    <div className="flex h-full overflow-y-auto">
      <div className="m-auto w-full max-w-lg p-6 space-y-6">
        {/* Header */}
        <div className="text-center space-y-2">
          <Bot className="h-10 w-10 text-primary mx-auto" />
          <h2 className="text-lg font-semibold">{t("setup.title")}</h2>
          <p className="text-sm text-muted-foreground">
            {t("setup.subtitle")}
          </p>
        </div>

        {/* Provider Selection */}
        <div className="grid grid-cols-2 gap-3">
          <button
            onClick={() => setProviderType("local_cli")}
            className={`rounded-lg border-2 p-4 text-left transition-all hover:border-primary/50 ${
              providerType === "local_cli"
                ? "border-primary bg-primary/5"
                : "border-border"
            }`}
          >
            <div className="flex items-center gap-2 mb-2">
              <Terminal className="h-5 w-5" />
              <span className="font-medium text-sm">
                {t("setup.localCLI")}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t("setup.localCLIDesc")}
            </p>
            <span className="inline-block mt-2 text-xs font-medium text-primary bg-primary/10 px-2 py-0.5 rounded">
              {t("setup.recommended")}
            </span>
          </button>

          <button
            onClick={() => setProviderType("openai")}
            className={`rounded-lg border-2 p-4 text-left transition-all hover:border-primary/50 ${
              providerType === "openai"
                ? "border-primary bg-primary/5"
                : "border-border"
            }`}
          >
            <div className="flex items-center gap-2 mb-2">
              <Bot className="h-5 w-5" />
              <span className="font-medium text-sm">OpenAI API</span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t("setup.openAIDesc")}
            </p>
            <span className="inline-block mt-2 text-xs font-medium text-amber-600 dark:text-amber-400 bg-amber-100 dark:bg-amber-900/30 px-2 py-0.5 rounded">
              {t("setup.notComplete")}
            </span>
          </button>
        </div>

        {/* Local CLI Configuration */}
        {providerType === "local_cli" && (
          <div className="space-y-4">
            {/* CLI Config */}
            <div className="rounded-lg border p-4 space-y-3">
              <h3 className="text-sm font-medium">{t("setup.cliConfig")}</h3>

              {localCLIs.length > 0 && (
                <div className="text-xs text-muted-foreground">
                  {t("settings.detectedCLIs")}:{" "}
                  {localCLIs
                    .map((c) => `${c.name} (${c.path})`)
                    .join(", ")}
                </div>
              )}

              <div className="grid gap-2">
                <Label className="text-xs">{t("settings.cliPath")}</Label>
                <Input
                  value={cliPath}
                  onChange={(e) => setCliPath(e.target.value)}
                  placeholder="/usr/local/bin/claude"
                  className="h-8 text-sm"
                />
              </div>

              <div className="grid gap-2">
                <Label className="text-xs">{t("settings.cliType")}</Label>
                <Select value={cliType} onValueChange={setCliType}>
                  <SelectTrigger className="h-8 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="claude">Claude Code</SelectItem>
                    <SelectItem value="codex">Codex</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Integration Status */}
            <div className="rounded-lg border p-4 space-y-3">
              <h3 className="text-sm font-medium">
                {t("setup.integration")}
              </h3>

              {/* opsctl CLI */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Terminal className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm">opsctl CLI</span>
                </div>
                {opsctlInfo.installed ? (
                  <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
                    <Check className="h-3.5 w-3.5" />
                    {t("integration.installed")}
                  </span>
                ) : opsctlInfo.embedded ? (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 text-xs"
                    onClick={handleInstallOpsctl}
                    disabled={opsctlInstalling}
                  >
                    {opsctlInstalling ? (
                      <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                    ) : (
                      <Download className="h-3 w-3 mr-1" />
                    )}
                    {t("integration.install")}
                  </Button>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t("setup.opsctlManualHint")}
                  </span>
                )}
              </div>

              {/* AI Skill */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Bot className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm">{t("integration.skill")}</span>
                </div>
                {skillsInstalled ? (
                  <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
                    <Check className="h-3.5 w-3.5" />
                    {t("integration.skillInstalled")}
                  </span>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t("setup.autoInstall")}
                  </span>
                )}
              </div>

              {!opsctlInfo.installed && !opsctlInfo.embedded && (
                <p className="text-xs text-amber-600 dark:text-amber-400 flex items-start gap-1">
                  <AlertTriangle className="h-3 w-3 mt-0.5 shrink-0" />
                  {t("setup.opsctlRequired")}
                </p>
              )}
            </div>

            <Button
              onClick={handleComplete}
              disabled={!cliPath.trim() || completing}
              className="w-full"
            >
              {completing ? (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              ) : (
                <ArrowRight className="h-4 w-4 mr-2" />
              )}
              {t("setup.complete")}
            </Button>
          </div>
        )}

        {/* OpenAI Configuration */}
        {providerType === "openai" && (
          <div className="space-y-4">
            <div className="rounded-lg border p-4 space-y-3">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium">
                  API {t("setup.config")}
                </h3>
                <span className="text-xs text-amber-600 dark:text-amber-400 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">
                  {t("setup.notComplete")}
                </span>
              </div>

              <div className="grid gap-2">
                <Label className="text-xs">API Base URL</Label>
                <Input
                  value={apiBase}
                  onChange={(e) => setApiBase(e.target.value)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="grid gap-2">
                <Label className="text-xs">API Key</Label>
                <Input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="grid gap-2">
                <Label className="text-xs">{t("settings.model")}</Label>
                <Input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  className="h-8 text-sm"
                />
              </div>
            </div>

            <Button
              onClick={handleComplete}
              disabled={!apiBase.trim() || !apiKey.trim() || completing}
              className="w-full"
            >
              {completing ? (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              ) : (
                <ArrowRight className="h-4 w-4 mr-2" />
              )}
              {t("setup.complete")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
