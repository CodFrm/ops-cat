import { useState, useMemo, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  Download,
  Shield,
  Network,
  Settings2,
  Keyboard,
  Palette,
  Loader2,
  ChevronDown,
  ChevronRight,
  Folder,
  Server,
  Database,
  Eye,
  EyeOff,
  Shuffle,
  AlertTriangle,
} from "lucide-react";
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
  ScrollArea,
} from "@opskat/ui";
import { backup_svc } from "../../../wailsjs/go/models";
import { ExportToFile } from "../../../wailsjs/go/app/App";
import { useAssetStore } from "@/stores/assetStore";
import { useShortcutStore, DEFAULT_SHORTCUTS } from "@/stores/shortcutStore";
import { useTerminalThemeStore } from "@/stores/terminalThemeStore";

interface ExportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "file" | "gist";
  onGistExport?: (password: string, opts: backup_svc.ExportOptions) => Promise<void>;
}

type AssetSelectionMode = "all" | "specific";

function generatePassword(length = 20): string {
  const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*";
  const arr = new Uint8Array(length);
  crypto.getRandomValues(arr);
  return Array.from(arr, (b) => chars[b % chars.length]).join("");
}

function AssetIcon({ type }: { type: string }) {
  switch (type) {
    case "database":
    case "redis":
      return <Database className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />;
    default:
      return <Server className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />;
  }
}

export function ExportDialog({ open, onOpenChange, mode, onGistExport }: ExportDialogProps) {
  const { t } = useTranslation();
  const { assets, groups } = useAssetStore();
  const { shortcuts } = useShortcutStore();
  const { customThemes } = useTerminalThemeStore();

  const [selectionMode, setSelectionMode] = useState<AssetSelectionMode>("all");
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [expandedGroups, setExpandedGroups] = useState<Set<number | string>>(new Set());

  const [includeCredentials, setIncludeCredentials] = useState(false);
  const [includeForwards, setIncludeForwards] = useState(true);
  const [includePolicyGroups, setIncludePolicyGroups] = useState(true);
  const [includeShortcuts, setIncludeShortcuts] = useState(false);
  const [includeThemes, setIncludeThemes] = useState(false);

  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [exporting, setExporting] = useState(false);

  // Reset state when dialog opens
  useMemo(() => {
    if (open) {
      setSelectionMode("all");
      setSelectedIds(new Set());
      setIncludeCredentials(false);
      setIncludeForwards(true);
      setIncludePolicyGroups(true);
      setIncludeShortcuts(false);
      setIncludeThemes(false);
      setPassword("");
      setShowPassword(false);
      // Expand all groups by default
      setExpandedGroups(new Set([...groups.map((g) => g.ID), "__ungrouped__"]));
    }
  }, [open, groups]);

  // Group assets by groupId
  const groupedAssets = useMemo(() => {
    const map = new Map<number | string, typeof assets>();
    for (const asset of assets) {
      const gid = asset.GroupID || "__ungrouped__";
      if (!map.has(gid)) map.set(gid, []);
      map.get(gid)!.push(asset);
    }
    return map;
  }, [assets]);

  const groupOrder = useMemo(() => {
    const order: (number | string)[] = groups.map((g) => g.ID);
    if (groupedAssets.has("__ungrouped__")) order.push("__ungrouped__");
    return order;
  }, [groups, groupedAssets]);

  const groupNameMap = useMemo(() => {
    const map = new Map<number | string, string>();
    for (const g of groups) map.set(g.ID, g.Name);
    return map;
  }, [groups]);

  const toggleAsset = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleGroup = useCallback(
    (groupId: number | string) => {
      const items = groupedAssets.get(groupId) || [];
      setSelectedIds((prev) => {
        const next = new Set(prev);
        const allSelected = items.every((a) => prev.has(a.ID));
        for (const item of items) {
          if (allSelected) next.delete(item.ID);
          else next.add(item.ID);
        }
        return next;
      });
    },
    [groupedAssets]
  );

  const toggleExpand = useCallback((groupId: number | string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupId)) next.delete(groupId);
      else next.add(groupId);
      return next;
    });
  }, []);

  const selectAll = () => setSelectedIds(new Set(assets.map((a) => a.ID)));
  const selectNone = () => setSelectedIds(new Set());

  const canExport = useMemo(() => {
    if (includeCredentials && !password) return false;
    if (selectionMode === "specific" && selectedIds.size === 0) return false;
    return true;
  }, [includeCredentials, password, selectionMode, selectedIds]);

  const buildOptions = useCallback((): backup_svc.ExportOptions => {
    const opts = new backup_svc.ExportOptions();
    opts.asset_ids = selectionMode === "all" ? [] : Array.from(selectedIds);
    opts.include_credentials = includeCredentials;
    opts.include_forwards = includeForwards;
    opts.include_policy_groups = includePolicyGroups;

    if (includeShortcuts) {
      // Export only custom (non-default) bindings
      const custom: Record<string, unknown> = {};
      for (const key of Object.keys(shortcuts) as (keyof typeof DEFAULT_SHORTCUTS)[]) {
        const val = shortcuts[key];
        const def = DEFAULT_SHORTCUTS[key];
        if (def && (val.code !== def.code || val.mod !== def.mod || val.shift !== def.shift || val.alt !== def.alt)) {
          custom[key] = val;
        }
      }
      if (Object.keys(custom).length > 0) {
        opts.shortcuts = JSON.stringify(custom);
      }
    }

    if (includeThemes && customThemes.length > 0) {
      opts.custom_themes = JSON.stringify(customThemes);
    }

    return opts;
  }, [
    selectionMode,
    selectedIds,
    includeCredentials,
    includeForwards,
    includePolicyGroups,
    includeShortcuts,
    includeThemes,
    shortcuts,
    customThemes,
  ]);

  const handleExport = async () => {
    if (!canExport) return;
    setExporting(true);
    try {
      const opts = buildOptions();
      if (mode === "gist" && onGistExport) {
        await onGistExport(includeCredentials ? password : "", opts);
      } else {
        await ExportToFile(includeCredentials ? password : "", opts);
      }
      toast.success(t("backup.exportSuccess"));
      onOpenChange(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setExporting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[85vh] !grid-rows-[auto_1fr_auto] overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Download className="h-5 w-5" />
            {t("backup.exportTitle")}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 overflow-y-auto min-h-0">
          {/* Asset selection */}
          <div className="space-y-2">
            <Label>{t("backup.assetSelection")}</Label>
            <div className="flex gap-2">
              <Button
                variant={selectionMode === "all" ? "default" : "outline"}
                size="sm"
                onClick={() => setSelectionMode("all")}
              >
                {t("backup.allAssets")}
              </Button>
              <Button
                variant={selectionMode === "specific" ? "default" : "outline"}
                size="sm"
                onClick={() => {
                  setSelectionMode("specific");
                  if (selectedIds.size === 0) selectAll();
                }}
              >
                {t("backup.selectedAssets", { count: selectionMode === "specific" ? selectedIds.size : assets.length })}
              </Button>
            </div>

            {selectionMode === "specific" && (
              <>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <span>{t("backup.selectedCount", { selected: selectedIds.size, total: assets.length })}</span>
                  <span className="ml-auto flex gap-2">
                    <button className="hover:text-foreground underline" onClick={selectAll}>
                      {t("backup.selectAll")}
                    </button>
                    <button className="hover:text-foreground underline" onClick={selectNone}>
                      {t("backup.selectNone")}
                    </button>
                  </span>
                </div>
                <ScrollArea
                  className="max-h-[30vh] border rounded-lg"
                  style={{
                    overflowY: "auto",
                  }}
                >
                  <div className="p-2 space-y-0.5">
                    {groupOrder.map((gid) => {
                      const items = groupedAssets.get(gid) || [];
                      if (items.length === 0) return null;
                      const groupName =
                        gid === "__ungrouped__" ? t("asset.ungrouped") : groupNameMap.get(gid) || String(gid);
                      const expanded = expandedGroups.has(gid);
                      const allSelected = items.every((a) => selectedIds.has(a.ID));
                      const someSelected = !allSelected && items.some((a) => selectedIds.has(a.ID));

                      return (
                        <div key={String(gid)}>
                          <div
                            className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium hover:bg-muted cursor-pointer"
                            onClick={() => toggleExpand(gid)}
                          >
                            <input
                              type="checkbox"
                              checked={allSelected}
                              ref={(el) => {
                                if (el) el.indeterminate = someSelected;
                              }}
                              onChange={(e) => {
                                e.stopPropagation();
                                toggleGroup(gid);
                              }}
                              onClick={(e) => e.stopPropagation()}
                              className="rounded"
                            />
                            {expanded ? (
                              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                            ) : (
                              <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                            )}
                            <Folder className="h-3.5 w-3.5 text-muted-foreground" />
                            <span>{groupName}</span>
                            <span className="ml-auto text-muted-foreground">{items.length}</span>
                          </div>
                          {expanded && (
                            <div>
                              {items.map((asset) => (
                                <label
                                  key={asset.ID}
                                  className="flex items-center gap-1.5 rounded-md pl-9 pr-2 py-1.5 text-sm cursor-pointer hover:bg-muted"
                                >
                                  <input
                                    type="checkbox"
                                    checked={selectedIds.has(asset.ID)}
                                    onChange={() => toggleAsset(asset.ID)}
                                    className="rounded"
                                  />
                                  <AssetIcon type={asset.Type} />
                                  <span className="truncate flex-1">{asset.Name}</span>
                                  <span className="text-xs text-muted-foreground font-mono shrink-0">{asset.Type}</span>
                                </label>
                              ))}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </ScrollArea>
              </>
            )}
          </div>

          {/* Module toggles */}
          <div className="space-y-3">
            <Label>{t("backup.exportContent")}</Label>

            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-sm">
                <Shield className="h-4 w-4 text-muted-foreground" />
                <span>{t("backup.includeCredentials")}</span>
              </div>
              <Switch checked={includeCredentials} onCheckedChange={setIncludeCredentials} />
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-sm">
                <Network className="h-4 w-4 text-muted-foreground" />
                <span>{t("backup.includeForwards")}</span>
              </div>
              <Switch checked={includeForwards} onCheckedChange={setIncludeForwards} />
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-sm">
                <Settings2 className="h-4 w-4 text-muted-foreground" />
                <span>{t("backup.includePolicyGroups")}</span>
              </div>
              <Switch checked={includePolicyGroups} onCheckedChange={setIncludePolicyGroups} />
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-sm">
                <Keyboard className="h-4 w-4 text-muted-foreground" />
                <span>{t("backup.includeShortcuts")}</span>
              </div>
              <Switch checked={includeShortcuts} onCheckedChange={setIncludeShortcuts} />
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-sm">
                <Palette className="h-4 w-4 text-muted-foreground" />
                <span>{t("backup.includeThemes")}</span>
              </div>
              <Switch checked={includeThemes} onCheckedChange={setIncludeThemes} />
            </div>
          </div>

          {/* Credential password */}
          {includeCredentials && (
            <div className="space-y-2">
              <div className="flex items-center gap-2 rounded-md bg-amber-500/10 border border-amber-500/20 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                <AlertTriangle className="h-4 w-4 shrink-0" />
                <span>{t("backup.credentialWarning")}</span>
              </div>
              <Label>{t("backup.password")}</Label>
              <div className="flex gap-1">
                <div className="relative flex-1">
                  <Input
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder={t("backup.passwordPlaceholder")}
                    className="pr-9"
                  />
                  <button
                    type="button"
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    onClick={() => setShowPassword(!showPassword)}
                    tabIndex={-1}
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => setPassword(generatePassword())}
                  title={t("backup.generatePassword")}
                >
                  <Shuffle className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleExport} disabled={exporting || !canExport}>
            {exporting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-1" />
                {t("backup.exporting")}
              </>
            ) : (
              <>
                <Download className="h-4 w-4 mr-1" />
                {t("backup.export")}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
