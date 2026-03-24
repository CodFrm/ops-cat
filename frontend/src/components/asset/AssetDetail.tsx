import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Server, Pencil, Trash2, TerminalSquare, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";
import { useAssetStore } from "@/stores/assetStore";
import { asset_entity } from "../../../wailsjs/go/models";

interface SSHConfig {
  host: string;
  port: number;
  username: string;
  auth_type: string;
  password?: string;
  key_id?: number;
  key_source?: string;
  private_keys?: string[];
  jump_host_id?: number;
  proxy?: {
    type: string;
    host: string;
    port: number;
    username?: string;
    password?: string;
  } | null;
}

interface AssetDetailProps {
  asset: asset_entity.Asset;
  isConnecting?: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onConnect: () => void;
}

export function AssetDetail({ asset, isConnecting, onEdit, onDelete, onConnect }: AssetDetailProps) {
  const { t } = useTranslation();
  const { assets, updateAsset } = useAssetStore();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  // Command policy inline editing
  const [allowList, setAllowList] = useState<string[]>([]);
  const [denyList, setDenyList] = useState<string[]>([]);
  const [allowInput, setAllowInput] = useState("");
  const [denyInput, setDenyInput] = useState("");
  const [savingPolicy, setSavingPolicy] = useState(false);

  useEffect(() => {
    try {
      const policy = JSON.parse(asset.CmdPolicy || "{}");
      setAllowList(policy.allow_list || []);
      setDenyList(policy.deny_list || []);
    } catch {
      setAllowList([]);
      setDenyList([]);
    }
    setAllowInput("");
    setDenyInput("");
  }, [asset.ID, asset.CmdPolicy]);

  const handleSavePolicy = async (newAllowList: string[], newDenyList: string[]) => {
    let cmdPolicy = "";
    if (newAllowList.length > 0 || newDenyList.length > 0) {
      cmdPolicy = JSON.stringify({
        allow_list: newAllowList.length > 0 ? newAllowList : undefined,
        deny_list: newDenyList.length > 0 ? newDenyList : undefined,
      });
    }
    const updated = new asset_entity.Asset({ ...asset, CmdPolicy: cmdPolicy });
    setSavingPolicy(true);
    try {
      await updateAsset(updated);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSavingPolicy(false);
    }
  };

  let sshConfig: SSHConfig | null = null;
  try {
    sshConfig = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }

  const jumpHostName = sshConfig?.jump_host_id
    ? assets.find((a) => a.ID === sshConfig!.jump_host_id)?.Name || `ID:${sshConfig.jump_host_id}`
    : null;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b">
        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
            <Server className="h-4 w-4 text-primary" />
          </div>
          <div>
            <h2 className="font-semibold leading-tight">{asset.Name}</h2>
            <span className="text-xs text-muted-foreground uppercase">
              {asset.Type}
            </span>
          </div>
        </div>
        <div className="flex gap-1.5">
          {asset.Type === "ssh" && (
            <Button size="sm" className="h-8 gap-1.5" onClick={onConnect} disabled={isConnecting}>
              {isConnecting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <TerminalSquare className="h-3.5 w-3.5" />
              )}
              {t("ssh.connect")}
            </Button>
          )}
          <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onEdit}>
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-destructive hover:text-destructive"
            onClick={() => setShowDeleteConfirm(true)}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      <AlertDialog open={showDeleteConfirm} onOpenChange={setShowDeleteConfirm}>
        <AlertDialogContent onOverlayClick={() => setShowDeleteConfirm(false)}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("asset.deleteAssetTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("asset.deleteAssetDesc", { name: asset.Name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("action.cancel")}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={onDelete}>
              {t("action.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <div className="flex-1 p-4 space-y-4 overflow-y-auto">
        {sshConfig && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              SSH Connection
            </h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <InfoItem label={t("asset.host")} value={sshConfig.host} mono />
              <InfoItem label={t("asset.port")} value={String(sshConfig.port)} mono />
              <InfoItem label={t("asset.username")} value={sshConfig.username} mono />
              <InfoItem
                label={t("asset.authType")}
                value={
                  sshConfig.auth_type === "password"
                    ? t("asset.authPassword") + (sshConfig.password ? " ●" : "")
                    : sshConfig.auth_type === "key"
                    ? t("asset.authKey") + (sshConfig.key_source === "managed" ? ` (${t("asset.keySourceManaged")})` : sshConfig.key_source === "file" ? ` (${t("asset.keySourceFile")})` : "")
                    : sshConfig.auth_type
                }
              />
            </div>
          </div>
        )}

        {/* Private Keys */}
        {sshConfig?.private_keys && sshConfig.private_keys.length > 0 && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.privateKeys")}
            </h3>
            <div className="space-y-1">
              {sshConfig.private_keys.map((key, i) => (
                <p key={i} className="text-sm font-mono text-muted-foreground">{key}</p>
              ))}
            </div>
          </div>
        )}

        {/* Jump Host */}
        {jumpHostName && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.jumpHost")}
            </h3>
            <p className="text-sm font-mono">{jumpHostName}</p>
          </div>
        )}

        {/* Proxy */}
        {sshConfig?.proxy && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.proxy")}
            </h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <InfoItem label={t("asset.proxyType")} value={sshConfig.proxy.type.toUpperCase()} />
              <InfoItem
                label={t("asset.proxyHost")}
                value={`${sshConfig.proxy.host}:${sshConfig.proxy.port}`}
                mono
              />
              {sshConfig.proxy.username && (
                <InfoItem label={t("asset.proxyUsername")} value={sshConfig.proxy.username} />
              )}
            </div>
          </div>
        )}

        {/* Command Policy — inline editing */}
        <div className="rounded-xl border bg-card p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t("asset.cmdPolicy")}
          </h3>

          {/* Allow list */}
          <div className="grid gap-2 mb-3">
            <Label className="text-xs">{t("asset.cmdPolicyAllowList")}</Label>
            <div className="flex flex-wrap gap-1.5 min-h-[24px]">
              {allowList.map((cmd, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-green-500/10 text-green-600 text-xs font-mono"
                >
                  {cmd}
                  <button
                    type="button"
                    className="hover:text-destructive"
                    onClick={() => {
                      const next = allowList.filter((_, idx) => idx !== i);
                      setAllowList(next);
                      handleSavePolicy(next, denyList);
                    }}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
            <Input
              className="h-7 text-xs font-mono"
              value={allowInput}
              onChange={(e) => setAllowInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && allowInput.trim()) {
                  e.preventDefault();
                  const next = [...allowList, allowInput.trim()];
                  setAllowList(next);
                  setAllowInput("");
                  handleSavePolicy(next, denyList);
                }
              }}
              placeholder={t("asset.cmdPolicyPlaceholder")}
            />
          </div>

          {/* Deny list */}
          <div className="grid gap-2 mb-3">
            <Label className="text-xs">{t("asset.cmdPolicyDenyList")}</Label>
            <div className="flex flex-wrap gap-1.5 min-h-[24px]">
              {denyList.map((cmd, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-red-500/10 text-red-600 text-xs font-mono"
                >
                  {cmd}
                  <button
                    type="button"
                    className="hover:text-destructive"
                    onClick={() => {
                      const next = denyList.filter((_, idx) => idx !== i);
                      setDenyList(next);
                      handleSavePolicy(allowList, next);
                    }}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
            <Input
              className="h-7 text-xs font-mono"
              value={denyInput}
              onChange={(e) => setDenyInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && denyInput.trim()) {
                  e.preventDefault();
                  const next = [...denyList, denyInput.trim()];
                  setDenyList(next);
                  setDenyInput("");
                  handleSavePolicy(allowList, next);
                }
              }}
              placeholder={t("asset.cmdPolicyPlaceholder")}
            />
          </div>

          <p className="text-xs text-muted-foreground">
            {savingPolicy ? t("settings.saved") + "..." : t("asset.cmdPolicyHint")}
          </p>
        </div>

        {asset.Description && (
          <>
            <Separator />
            <div className="text-sm">
              <span className="text-muted-foreground">
                {t("asset.description")}
              </span>
              <p className="mt-1">{asset.Description}</p>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function InfoItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <span className="text-xs text-muted-foreground">{label}</span>
      <p className={cn("mt-0.5 text-sm", mono && "font-mono")}>{value}</p>
    </div>
  );
}
