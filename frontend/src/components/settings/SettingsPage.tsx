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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Separator } from "@/components/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useTheme } from "@/components/theme-provider";
import { useAIStore } from "@/stores/aiStore";
import { useAssetStore } from "@/stores/assetStore";
import {
  ExportToFile,
  SelectImportFile,
  ExecuteImportFile,
  StartGitHubDeviceFlow,
  WaitGitHubDeviceAuth,
  CancelGitHubAuth,
  GetGitHubUser,
  ExportToGist,
  ListBackupGists,
  ImportFromGist,
  PreviewTabbyConfig,
} from "../../../wailsjs/go/main/App";
import { backup_svc } from "../../../wailsjs/go/models";
import { import_svc } from "../../../wailsjs/go/models";
import { ImportDialog } from "@/components/settings/ImportDialog";
import {
  Bot, Palette, Check, HardDrive, Download, Upload, Import,
  Github, LogOut, Loader2, Copy, ExternalLink, Eye, EyeOff, Shuffle, Keyboard,
  Plus, Pencil, Trash2, MonitorDot,
} from "lucide-react";
import { ShortcutSettings } from "@/components/settings/ShortcutSettings";
import { TerminalThemeEditor } from "@/components/settings/TerminalThemeEditor";
import { useTerminalThemeStore } from "@/stores/terminalThemeStore";
import { builtinThemes, TerminalTheme } from "@/data/terminalThemes";
import { toast } from "sonner";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";
import { cn } from "@/lib/utils";

function generatePassword(length = 20): string {
  const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*";
  const values = crypto.getRandomValues(new Uint8Array(length));
  return Array.from(values, (v) => charset[v % charset.length]).join("");
}

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
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7"
          onClick={() => setVisible(!visible)}>
          {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
        </Button>
        {showGenerate && (
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7"
            onClick={() => { const p = generatePassword(); setVisible(true); onGenerate?.(p); }}>
            <Shuffle className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </div>
  );
}

export function SettingsPage() {
  const { t, i18n } = useTranslation();
  const { theme, setTheme } = useTheme();
  const { configure, configured, detectCLIs, localCLIs } = useAIStore();
  const { refresh } = useAssetStore();

  // AI Provider
  const [providerType, setProviderType] = useState(
    localStorage.getItem("ai_provider_type") || "openai"
  );
  const [apiBase, setApiBase] = useState(
    localStorage.getItem("ai_api_base") || "https://api.openai.com/v1"
  );
  const [apiKey, setApiKey] = useState(
    localStorage.getItem("ai_api_key") || ""
  );
  const [model, setModel] = useState(
    localStorage.getItem("ai_model") || "gpt-4o"
  );
  const [saved, setSaved] = useState(false);

  // 文件备份
  const [fileExporting, setFileExporting] = useState(false);
  const [fileImporting, setFileImporting] = useState(false);
  const [exportPasswordOpen, setExportPasswordOpen] = useState(false);
  const [exportPassword, setExportPassword] = useState("");
  const [importPasswordOpen, setImportPasswordOpen] = useState(false);
  const [importPassword, setImportPassword] = useState("");
  const [importFilePath, setImportFilePath] = useState("");

  // Tabby 导入
  const [tabbyPreview, setTabbyPreview] = useState<import_svc.PreviewResult | null>(null);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [tabbyLoading, setTabbyLoading] = useState(false);

  // GitHub
  const [ghToken, setGhToken] = useState(localStorage.getItem("github_token") || "");
  const [ghUser, setGhUser] = useState(localStorage.getItem("github_user") || "");
  const [deviceFlowOpen, setDeviceFlowOpen] = useState(false);
  const [deviceFlowInfo, setDeviceFlowInfo] = useState<backup_svc.DeviceFlowInfo | null>(null);
  const [ghLoggingIn, setGhLoggingIn] = useState(false);

  // Gist
  const [gists, setGists] = useState<backup_svc.GistInfo[]>([]);
  const [selectedGistId, setSelectedGistId] = useState("");
  const [gistPushing, setGistPushing] = useState(false);
  const [gistPulling, setGistPulling] = useState(false);
  const [gistPasswordOpen, setGistPasswordOpen] = useState(false);
  const [gistPassword, setGistPassword] = useState("");
  const [gistPullPasswordOpen, setGistPullPasswordOpen] = useState(false);
  const [gistPullPassword, setGistPullPassword] = useState("");

  // 终端主题
  const {
    selectedThemeId, setSelectedThemeId,
    fontSize, setFontSize,
    customThemes, addCustomTheme, updateCustomTheme, removeCustomTheme,
  } = useTerminalThemeStore();
  const [themeEditorOpen, setThemeEditorOpen] = useState(false);
  const [editingTheme, setEditingTheme] = useState<TerminalTheme | undefined>(undefined);

  useEffect(() => { detectCLIs(); }, [detectCLIs]);

  useEffect(() => {
    const savedType = localStorage.getItem("ai_provider_type");
    const savedBase = localStorage.getItem("ai_api_base");
    const savedKey = localStorage.getItem("ai_api_key");
    const savedModel = localStorage.getItem("ai_model");
    if (savedType && savedBase && savedKey && savedModel) {
      configure(savedType, savedBase, savedKey, savedModel);
    }
  }, [configure]);

  // 验证已保存的 GitHub token
  useEffect(() => {
    if (ghToken) {
      GetGitHubUser(ghToken).then(user => {
        setGhUser(user.login);
        localStorage.setItem("github_user", user.login);
      }).catch(() => {
        setGhToken("");
        setGhUser("");
        localStorage.removeItem("github_token");
        localStorage.removeItem("github_user");
      });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const loadGists = useCallback(async () => {
    if (!ghToken) return;
    try {
      const list = await ListBackupGists(ghToken);
      setGists(list || []);
    } catch {
      setGists([]);
    }
  }, [ghToken]);

  useEffect(() => { loadGists(); }, [loadGists]);

  // --- AI ---
  const handleSaveAI = async () => {
    localStorage.setItem("ai_provider_type", providerType);
    localStorage.setItem("ai_api_base", apiBase);
    localStorage.setItem("ai_api_key", apiKey);
    localStorage.setItem("ai_model", model);
    await configure(providerType, apiBase, apiKey, model);
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const handleLanguageChange = (lng: string) => {
    i18n.changeLanguage(lng);
    localStorage.setItem("language", lng);
  };

  // --- 文件备份 ---
  const handleFileExport = () => {
    setExportPassword("");
    setExportPasswordOpen(true);
  };

  const doFileExport = async () => {
    setExportPasswordOpen(false);
    setFileExporting(true);
    try {
      await ExportToFile(exportPassword);
      toast.success(t("backup.exportSuccess"));
    } catch (e: any) {
      toast.error(e?.message || String(e));
    } finally {
      setFileExporting(false);
    }
  };

  const handleFileImport = async () => {
    try {
      const info = await SelectImportFile();
      if (!info || !info.filePath) return;
      if (info.encrypted) {
        setImportFilePath(info.filePath);
        setImportPassword("");
        setImportPasswordOpen(true);
      } else {
        setFileImporting(true);
        await ExecuteImportFile(info.filePath, "");
        toast.success(t("backup.importSuccess"));
        await refresh();
        setFileImporting(false);
      }
    } catch (e: any) {
      toast.error(e?.message || String(e));
      setFileImporting(false);
    }
  };

  const doFileImportWithPassword = async () => {
    setImportPasswordOpen(false);
    setFileImporting(true);
    try {
      await ExecuteImportFile(importFilePath, importPassword);
      toast.success(t("backup.importSuccess"));
      await refresh();
    } catch (e: any) {
      toast.error(e?.message || String(e));
    } finally {
      setFileImporting(false);
    }
  };

  // --- Tabby ---
  const handlePreviewTabby = async () => {
    setTabbyLoading(true);
    try {
      const result = await PreviewTabbyConfig();
      if (result) {
        setTabbyPreview(result);
        setImportDialogOpen(true);
      }
    } catch (e: any) {
      toast.error(e?.message || String(e));
    } finally {
      setTabbyLoading(false);
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
      localStorage.setItem("github_token", token);

      const user = await GetGitHubUser(token);
      setGhUser(user.login);
      localStorage.setItem("github_user", user.login);
      toast.success(t("backup.gistLoggedIn", { user: user.login }));
    } catch (e: any) {
      if (!String(e).includes("取消")) {
        toast.error(e?.message || String(e));
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
    localStorage.removeItem("github_token");
    localStorage.removeItem("github_user");
  };

  const handleCancelDeviceFlow = () => {
    CancelGitHubAuth().catch(() => {});
    setDeviceFlowOpen(false);
  };

  // --- Gist ---
  const handleGistPush = () => {
    setGistPassword("");
    setGistPasswordOpen(true);
  };

  const doGistPush = async () => {
    if (!gistPassword) {
      toast.error(t("backup.passwordRequired"));
      return;
    }
    setGistPasswordOpen(false);
    setGistPushing(true);
    try {
      const gistId = selectedGistId === "__new__" ? "" : selectedGistId;
      const result = await ExportToGist(gistPassword, ghToken, gistId);
      toast.success(t("backup.gistPushSuccess"));
      if (result) {
        await loadGists();
        setSelectedGistId(result.id);
      }
    } catch (e: any) {
      toast.error(e?.message || String(e));
    } finally {
      setGistPushing(false);
    }
  };

  const handleGistPull = () => {
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
      await ImportFromGist(selectedGistId, gistPullPassword, ghToken);
      toast.success(t("backup.gistPullSuccess"));
      await refresh();
    } catch (e: any) {
      toast.error(e?.message || String(e));
    } finally {
      setGistPulling(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-3 border-b">
        <h2 className="font-semibold">{t("nav.settings")}</h2>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <Tabs defaultValue="ai" className="space-y-4 max-w-4xl mx-auto">
          <TabsList>
            <TabsTrigger value="ai" className="gap-1">
              <Bot className="h-3.5 w-3.5" />
              AI Provider
            </TabsTrigger>
            <TabsTrigger value="import" className="gap-1">
              <Import className="h-3.5 w-3.5" />
              {t("import.title")}
            </TabsTrigger>
            <TabsTrigger value="backup" className="gap-1">
              <HardDrive className="h-3.5 w-3.5" />
              {t("backup.title")}
            </TabsTrigger>
            <TabsTrigger value="shortcuts" className="gap-1">
              <Keyboard className="h-3.5 w-3.5" />
              {t("shortcut.title")}
            </TabsTrigger>
            <TabsTrigger value="terminal" className="gap-1">
              <MonitorDot className="h-3.5 w-3.5" />
              {t("terminal.title")}
            </TabsTrigger>
            <TabsTrigger value="appearance" className="gap-1">
              <Palette className="h-3.5 w-3.5" />
              {t("nav.appearance")}
            </TabsTrigger>
          </TabsList>

          {/* AI Provider */}
          <TabsContent value="ai" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">AI Provider</CardTitle>
                <CardDescription>
                  {configured ? "✓ " + t("settings.configured") : t("ai.notConfigured")}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-2">
                  <Label>{t("settings.providerType")}</Label>
                  <Select value={providerType} onValueChange={setProviderType}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="openai">OpenAI Compatible</SelectItem>
                      <SelectItem value="local_cli">Local CLI</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {providerType === "openai" && (
                  <>
                    <div className="grid gap-2">
                      <Label>API Base URL</Label>
                      <Input value={apiBase} onChange={(e) => setApiBase(e.target.value)} />
                    </div>
                    <div className="grid gap-2">
                      <Label>API Key</Label>
                      <Input type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
                    </div>
                    <div className="grid gap-2">
                      <Label>{t("settings.model")}</Label>
                      <Input value={model} onChange={(e) => setModel(e.target.value)} />
                    </div>
                  </>
                )}
                {providerType === "local_cli" && (
                  <>
                    <div className="grid gap-2">
                      <Label>{t("settings.cliPath")}</Label>
                      <Input value={apiBase} onChange={(e) => setApiBase(e.target.value)} placeholder="/usr/local/bin/claude" />
                    </div>
                    <div className="grid gap-2">
                      <Label>{t("settings.cliType")}</Label>
                      <Select value={model} onValueChange={setModel}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="claude">Claude Code</SelectItem>
                          <SelectItem value="codex">Codex</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {localCLIs.length > 0 && (
                      <div className="text-sm text-muted-foreground">
                        {t("settings.detectedCLIs")}: {localCLIs.map((c) => `${c.name} (${c.path})`).join(", ")}
                      </div>
                    )}
                  </>
                )}
                <Button onClick={handleSaveAI} className="gap-1">
                  {saved ? <Check className="h-4 w-4" /> : null}
                  {saved ? t("settings.saved") : t("action.save")}
                </Button>
              </CardContent>
            </Card>
          </TabsContent>

          {/* 导入 */}
          <TabsContent value="import" className="space-y-4">
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
          </TabsContent>

          {/* 备份 */}
          <TabsContent value="backup" className="space-y-4">
            {/* 文件备份 */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("backup.file")}</CardTitle>
                <CardDescription>{t("backup.fileDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="flex gap-2">
                <Button onClick={handleFileExport} disabled={fileExporting} variant="outline" className="gap-1">
                  <Download className="h-4 w-4" />
                  {fileExporting ? t("backup.exporting") : t("backup.export")}
                </Button>
                <Button onClick={handleFileImport} disabled={fileImporting} variant="outline" className="gap-1">
                  <Upload className="h-4 w-4" />
                  {fileImporting ? t("backup.importing") : t("backup.import")}
                </Button>
              </CardContent>
            </Card>

            {/* Gist 备份 */}
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
                      <span className="text-sm text-muted-foreground">
                        {t("backup.gistLoggedIn", { user: ghUser })}
                      </span>
                      <Button variant="ghost" size="sm" onClick={handleGitHubLogout} className="gap-1">
                        <LogOut className="h-3.5 w-3.5" />
                        {t("backup.gistLogout")}
                      </Button>
                    </div>
                    <div className="grid gap-2">
                      <Label>{t("backup.gistSelect")}</Label>
                      <Select value={selectedGistId} onValueChange={setSelectedGistId}>
                        <SelectTrigger><SelectValue placeholder={t("backup.gistSelect")} /></SelectTrigger>
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
          </TabsContent>

          {/* 快捷键 */}
          <TabsContent value="shortcuts" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("shortcut.title")}</CardTitle>
                <CardDescription>{t("shortcut.desc")}</CardDescription>
              </CardHeader>
              <CardContent>
                <ShortcutSettings />
              </CardContent>
            </Card>
          </TabsContent>

          {/* 终端 */}
          <TabsContent value="terminal" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("terminal.title")}</CardTitle>
                <CardDescription>{t("terminal.desc")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* 字体大小 */}
                <div className="grid gap-2">
                  <Label>{t("terminal.fontSize")}</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      min={8}
                      max={32}
                      value={fontSize}
                      onChange={(e) => setFontSize(Number(e.target.value))}
                      className="w-24"
                    />
                    <span className="text-sm text-muted-foreground">px</span>
                  </div>
                </div>

                <Separator />

                {/* 配色方案 */}
                <div className="space-y-2">
                  <Label>{t("terminal.builtinThemes")}</Label>
                  <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-2">
                    {/* 默认（无主题） */}
                    <button
                      onClick={() => setSelectedThemeId("default")}
                      className={cn(
                        "rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50",
                        selectedThemeId === "default" && "ring-2 ring-primary"
                      )}
                    >
                      <div className="rounded h-10 mb-1.5 flex items-center justify-center bg-black">
                        <span className="text-white text-xs font-mono">&gt;_</span>
                      </div>
                      <div className="text-xs truncate font-medium">{t("terminal.default")}</div>
                    </button>
                    {builtinThemes.map((bt) => (
                      <button
                        key={bt.id}
                        onClick={() => setSelectedThemeId(bt.id)}
                        className={cn(
                          "rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50",
                          selectedThemeId === bt.id && "ring-2 ring-primary"
                        )}
                      >
                        {/* 色块预览 */}
                        <div
                          className="rounded h-10 mb-1.5 flex items-end p-1 gap-0.5"
                          style={{ background: bt.background }}
                        >
                          {[bt.red, bt.green, bt.yellow, bt.blue, bt.magenta, bt.cyan].map(
                            (c, i) => (
                              <div
                                key={i}
                                className="w-2 h-3 rounded-sm"
                                style={{ background: c }}
                              />
                            )
                          )}
                        </div>
                        <div className="text-xs truncate font-medium">{bt.name}</div>
                      </button>
                    ))}
                  </div>
                </div>

                <Separator />

                {/* 自定义配色 */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>{t("terminal.customThemes")}</Label>
                    <Button
                      variant="outline"
                      size="sm"
                      className="gap-1"
                      onClick={() => {
                        setEditingTheme(undefined);
                        setThemeEditorOpen(true);
                      }}
                    >
                      <Plus className="h-3.5 w-3.5" />
                      {t("terminal.newTheme")}
                    </Button>
                  </div>
                  {customThemes.length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t("terminal.noCustomThemes")}</p>
                  ) : (
                    <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-2">
                      {customThemes.map((ct) => (
                        <div
                          key={ct.id}
                          className={cn(
                            "group relative rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50 cursor-pointer",
                            selectedThemeId === ct.id && "ring-2 ring-primary"
                          )}
                          onClick={() => setSelectedThemeId(ct.id)}
                        >
                          <div
                            className="rounded h-10 mb-1.5 flex items-end p-1 gap-0.5"
                            style={{ background: ct.background }}
                          >
                            {[ct.red, ct.green, ct.yellow, ct.blue, ct.magenta, ct.cyan].map(
                              (c, i) => (
                                <div
                                  key={i}
                                  className="w-2 h-3 rounded-sm"
                                  style={{ background: c }}
                                />
                              )
                            )}
                          </div>
                          <div className="text-xs truncate font-medium">{ct.name}</div>
                          {/* 编辑/删除 */}
                          <div className="absolute top-1 right-1 hidden group-hover:flex gap-0.5">
                            <button
                              className="rounded p-0.5 bg-background/80 hover:bg-muted"
                              onClick={(e) => {
                                e.stopPropagation();
                                setEditingTheme(ct);
                                setThemeEditorOpen(true);
                              }}
                            >
                              <Pencil className="h-3 w-3" />
                            </button>
                            <button
                              className="rounded p-0.5 bg-background/80 hover:bg-destructive/20"
                              onClick={(e) => {
                                e.stopPropagation();
                                removeCustomTheme(ct.id);
                              }}
                            >
                              <Trash2 className="h-3 w-3" />
                            </button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          {/* 外观和语言 */}
          <TabsContent value="appearance" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("theme.toggle")}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-2">
                  <Label>{t("theme.toggle")}</Label>
                  <Select value={theme} onValueChange={setTheme as (v: string) => void}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="light">{t("theme.light")}</SelectItem>
                      <SelectItem value="dark">{t("theme.dark")}</SelectItem>
                      <SelectItem value="system">{t("theme.system")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Separator />
                <div className="grid gap-2">
                  <Label>{t("language.label")}</Label>
                  <Select value={i18n.language} onValueChange={handleLanguageChange}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="zh-CN">{t("language.zh-CN")}</SelectItem>
                      <SelectItem value="en">{t("language.en")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>

      {/* Tabby 导入对话框 */}
      <ImportDialog open={importDialogOpen} onOpenChange={setImportDialogOpen} preview={tabbyPreview} />

      {/* 导出密码对话框 */}
      <Dialog open={exportPasswordOpen} onOpenChange={setExportPasswordOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("backup.export")}</DialogTitle>
            <DialogDescription>{t("backup.passwordOptional")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label>{t("backup.password")}</Label>
            <PasswordInput
              value={exportPassword}
              onChange={(e) => setExportPassword(e.target.value)}
              showGenerate
              onGenerate={(p) => setExportPassword(p)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExportPasswordOpen(false)}>{t("action.cancel")}</Button>
            <Button onClick={doFileExport}>{t("backup.export")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 导入密码对话框 */}
      <Dialog open={importPasswordOpen} onOpenChange={setImportPasswordOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("backup.import")}</DialogTitle>
            <DialogDescription>{t("backup.enterPassword")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label>{t("backup.password")}</Label>
            <PasswordInput
              value={importPassword}
              onChange={(e) => setImportPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && doFileImportWithPassword()}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setImportPasswordOpen(false)}>{t("action.cancel")}</Button>
            <Button onClick={doFileImportWithPassword} disabled={!importPassword}>{t("backup.import")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* GitHub Device Flow 对话框 */}
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
                  onClick={() => { navigator.clipboard.writeText(deviceFlowInfo.userCode); toast.success("Copied"); }}
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
            <Button variant="outline" onClick={handleCancelDeviceFlow}>{t("action.cancel")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Gist 推送密码对话框 */}
      <Dialog open={gistPasswordOpen} onOpenChange={setGistPasswordOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("backup.gistPush")}</DialogTitle>
            <DialogDescription>{t("backup.passwordRequired")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label>{t("backup.password")}</Label>
            <PasswordInput
              value={gistPassword}
              onChange={(e) => setGistPassword(e.target.value)}
              showGenerate
              onGenerate={(p) => setGistPassword(p)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setGistPasswordOpen(false)}>{t("action.cancel")}</Button>
            <Button onClick={doGistPush} disabled={!gistPassword}>{t("backup.gistPush")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Gist 拉取密码对话框 */}
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
            <Button variant="outline" onClick={() => setGistPullPasswordOpen(false)}>{t("action.cancel")}</Button>
            <Button onClick={doGistPull} disabled={!gistPullPassword}>{t("backup.gistPull")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 终端主题编辑器 */}
      <TerminalThemeEditor
        open={themeEditorOpen}
        onOpenChange={setThemeEditorOpen}
        theme={editingTheme}
        onSave={(theme) => {
          if (editingTheme) {
            updateCustomTheme(theme);
          } else {
            addCustomTheme(theme);
          }
          setSelectedThemeId(theme.id);
        }}
      />
    </div>
  );
}
