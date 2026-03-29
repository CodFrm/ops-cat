import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Shield, Server, Network, Settings2, Keyboard, Palette, Loader2, Lock } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Input,
  Label,
  Switch,
  Separator,
} from "@opskat/ui";
import { PreviewImportFile, ExecuteImportFile } from "../../../wailsjs/go/app/App";
import { backup_svc } from "../../../wailsjs/go/models";
import { useAssetStore } from "@/stores/assetStore";
import { useShortcutStore } from "@/stores/shortcutStore";
import { useTerminalThemeStore } from "@/stores/terminalThemeStore";

interface BackupImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  filePath: string;
  encrypted: boolean;
  initialSummary?: backup_svc.BackupSummary | null;
}

export function BackupImportDialog({
  open,
  onOpenChange,
  filePath,
  encrypted,
  initialSummary,
}: BackupImportDialogProps) {
  const { t } = useTranslation();
  const { refresh } = useAssetStore();

  const [password, setPassword] = useState("");
  const [decrypting, setDecrypting] = useState(false);
  const [summary, setSummary] = useState<backup_svc.BackupSummary | null>(initialSummary ?? null);
  const [importing, setImporting] = useState(false);

  // module toggles
  const [importAssets, setImportAssets] = useState(true);
  const [importCredentials, setImportCredentials] = useState(false);
  const [importForwards, setImportForwards] = useState(true);
  const [importPolicyGroups, setImportPolicyGroups] = useState(false);
  const [importShortcuts, setImportShortcuts] = useState(false);
  const [importThemes, setImportThemes] = useState(false);
  const [mode, setMode] = useState<"merge" | "replace">("merge");

  const needsDecrypt = encrypted && !summary;

  const handleDecrypt = async () => {
    setDecrypting(true);
    try {
      const result = await PreviewImportFile(filePath, password);
      setSummary(result);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setDecrypting(false);
    }
  };

  const handleImport = async () => {
    setImporting(true);
    try {
      const opts = new backup_svc.ImportOptions({
        import_assets: importAssets,
        import_credentials: importCredentials,
        import_forwards: importForwards,
        import_policy_groups: importPolicyGroups,
        import_shortcuts: importShortcuts,
        import_themes: importThemes,
        mode,
      });
      const result = await ExecuteImportFile(filePath, password, opts);

      // restore shortcuts
      if (result.shortcuts) {
        try {
          const parsed = JSON.parse(result.shortcuts);
          localStorage.setItem("keyboard_shortcuts", JSON.stringify(parsed));
          // reload store
          const store = useShortcutStore.getState();
          store.resetAll();
          for (const [action, binding] of Object.entries(parsed)) {
            store.updateShortcut(action as never, binding as never);
          }
        } catch {
          // ignore parse errors
        }
      }

      // restore custom themes
      if (result.custom_themes) {
        try {
          const themes = JSON.parse(result.custom_themes);
          const store = useTerminalThemeStore.getState();
          for (const theme of themes) {
            store.addCustomTheme(theme);
          }
        } catch {
          // ignore parse errors
        }
      }

      await refresh();

      const parts: string[] = [];
      if (result.assets_imported > 0) parts.push(`${result.assets_imported} assets`);
      if (result.groups_imported > 0) parts.push(`${result.groups_imported} groups`);
      if (result.credentials_imported > 0) parts.push(`${result.credentials_imported} credentials`);
      if (result.policy_groups_imported > 0) parts.push(`${result.policy_groups_imported} policy groups`);
      if (result.forwards_imported > 0) parts.push(`${result.forwards_imported} forwards`);
      toast.success(t("backup.importSuccess") + (parts.length > 0 ? `: ${parts.join(", ")}` : ""));

      onOpenChange(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setImporting(false);
    }
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      setPassword("");
      setSummary(initialSummary ?? null);
    }
    onOpenChange(v);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("backup.importTitle")}</DialogTitle>
        </DialogHeader>

        {/* Decrypt step */}
        {needsDecrypt && (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Lock className="h-4 w-4" />
              <span>{t("backup.encryptedFile")}</span>
            </div>
            <div className="flex items-center gap-2">
              <Input
                type="password"
                placeholder={t("backup.password")}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && password) handleDecrypt();
                }}
              />
              <Button onClick={handleDecrypt} disabled={decrypting || !password}>
                {decrypting && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                {t("backup.decryptPreview")}
              </Button>
            </div>
          </div>
        )}

        {/* Summary & module selection */}
        {summary && (
          <div className="space-y-4">
            <div className="text-sm text-muted-foreground">
              {t("backup.backupFrom")}: {summary.exported_at?.split("T")[0]}
              <span className="ml-2 text-xs">(v{summary.version})</span>
            </div>

            <Separator />

            <div className="space-y-3">
              {/* Assets */}
              {(summary.asset_count > 0 || summary.group_count > 0) && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Server className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">
                      {t("backup.importAssets")}
                      <span className="ml-1 text-xs text-muted-foreground">
                        ({summary.asset_count} assets, {summary.group_count} groups)
                      </span>
                    </Label>
                  </div>
                  <Switch checked={importAssets} onCheckedChange={setImportAssets} />
                </div>
              )}

              {/* Credentials */}
              {summary.includes_credentials && summary.credential_count > 0 && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Shield className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">
                      {t("backup.importCredentials")}
                      <span className="ml-1 text-xs text-muted-foreground">
                        ({summary.credential_count} credentials)
                      </span>
                    </Label>
                  </div>
                  <Switch checked={importCredentials} onCheckedChange={setImportCredentials} />
                </div>
              )}

              {/* Port Forwarding */}
              {summary.forward_count > 0 && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Network className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">
                      {t("backup.importForwards")}
                      <span className="ml-1 text-xs text-muted-foreground">({summary.forward_count} configs)</span>
                    </Label>
                  </div>
                  <Switch checked={importForwards} onCheckedChange={setImportForwards} />
                </div>
              )}

              {/* Policy Groups */}
              {summary.policy_group_count > 0 && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Settings2 className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">
                      {t("backup.importPolicyGroups")}
                      <span className="ml-1 text-xs text-muted-foreground">({summary.policy_group_count} groups)</span>
                    </Label>
                  </div>
                  <Switch checked={importPolicyGroups} onCheckedChange={setImportPolicyGroups} />
                </div>
              )}

              {/* Shortcuts */}
              {summary.has_shortcuts && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Keyboard className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">{t("backup.importShortcuts")}</Label>
                  </div>
                  <Switch checked={importShortcuts} onCheckedChange={setImportShortcuts} />
                </div>
              )}

              {/* Custom Themes */}
              {summary.has_custom_themes && (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Palette className="h-4 w-4 text-muted-foreground" />
                    <Label className="text-sm">{t("backup.importThemes")}</Label>
                  </div>
                  <Switch checked={importThemes} onCheckedChange={setImportThemes} />
                </div>
              )}
            </div>

            <Separator />

            {/* Import Mode */}
            <div className="space-y-2">
              <Label className="text-sm font-medium">{t("backup.importMode")}</Label>
              <div className="flex gap-4">
                <label className="flex items-center gap-1.5 text-sm cursor-pointer">
                  <input type="radio" name="importMode" checked={mode === "merge"} onChange={() => setMode("merge")} />
                  {t("backup.modeMerge")}
                </label>
                <label className="flex items-center gap-1.5 text-sm cursor-pointer">
                  <input
                    type="radio"
                    name="importMode"
                    checked={mode === "replace"}
                    onChange={() => setMode("replace")}
                  />
                  {t("backup.modeReplace")}
                </label>
              </div>
              {mode === "replace" && <p className="text-xs text-destructive">{t("backup.replaceWarning")}</p>}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          {summary && (
            <Button onClick={handleImport} disabled={importing}>
              {importing && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
              {importing ? t("backup.importing") : t("backup.import")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
