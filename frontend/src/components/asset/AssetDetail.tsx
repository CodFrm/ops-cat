import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Server, Database, Pencil, Trash2, TerminalSquare, Loader2 } from "lucide-react";
import Markdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkBreaks from "remark-breaks";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { cn } from "@/lib/utils";
import { useAssetStore } from "@/stores/assetStore";
import { CommandPolicyCard } from "@/components/asset/CommandPolicyCard";
import { asset_entity } from "../../../wailsjs/go/models";
import { GetDefaultPolicy } from "../../../wailsjs/go/app/App";
import { useExtensionStore } from "@/stores/extensionStore";
import { ExtensionPanel } from "@/components/extension/ExtensionPanel";

interface SSHConfig {
  host: string;
  port: number;
  username: string;
  auth_type: string;
  password?: string;
  credential_id?: number;
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

interface DatabaseConfig {
  driver: string;
  host: string;
  port: number;
  username: string;
  password?: string;
  database?: string;
  ssl_mode?: string;
  tls?: boolean;
  params?: string;
  read_only?: boolean;
  ssh_asset_id?: number;
}

interface RedisConfig {
  host: string;
  port: number;
  username?: string;
  password?: string;
  database?: number;
  tls?: boolean;
  ssh_asset_id?: number;
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
  const isExtensionAssetType = useExtensionStore((s) => s.isExtensionAssetType);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);

  // SSH Command policy
  const [allowList, setAllowList] = useState<string[]>([]);
  const [denyList, setDenyList] = useState<string[]>([]);
  const [policyGroups, setPolicyGroups] = useState<number[]>([]);

  // Database Query policy
  const [queryAllowTypes, setQueryAllowTypes] = useState<string[]>([]);
  const [queryDenyTypes, setQueryDenyTypes] = useState<string[]>([]);
  const [queryDenyFlags, setQueryDenyFlags] = useState<string[]>([]);

  // Redis policy
  const [redisAllowList, setRedisAllowList] = useState<string[]>([]);
  const [redisDenyList, setRedisDenyList] = useState<string[]>([]);

  useEffect(() => {
    try {
      const policy = JSON.parse(asset.CmdPolicy || "{}");
      setPolicyGroups(policy.groups || []);
      if (asset.Type === "database") {
        setQueryAllowTypes(policy.allow_types || []);
        setQueryDenyTypes(policy.deny_types || []);
        setQueryDenyFlags(policy.deny_flags || []);
      } else if (asset.Type === "redis") {
        setRedisAllowList(policy.allow_list || []);
        setRedisDenyList(policy.deny_list || []);
      } else {
        setAllowList(policy.allow_list || []);
        setDenyList(policy.deny_list || []);
      }
    } catch {
      setAllowList([]);
      setDenyList([]);
      setPolicyGroups([]);
      setQueryAllowTypes([]);
      setQueryDenyTypes([]);
      setQueryDenyFlags([]);
      setRedisAllowList([]);
      setRedisDenyList([]);
    }
    // input states are managed internally by PolicyTagEditor
  }, [asset.ID, asset.CmdPolicy, asset.Type]);

  const savePolicy = async (policyObj: Record<string, unknown>, groups?: number[]) => {
    // Remove empty arrays (except groups which is managed separately)
    const cleaned: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(policyObj)) {
      if (Array.isArray(v) && v.length > 0) cleaned[k] = v;
    }
    const grps = groups ?? policyGroups;
    if (grps.length > 0) cleaned.groups = grps;
    const cmdPolicy = Object.keys(cleaned).length > 0 ? JSON.stringify(cleaned) : "";
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

  const handleSaveSSHPolicy = async (newAllow: string[], newDeny: string[], groups?: number[]) => {
    await savePolicy({ allow_list: newAllow, deny_list: newDeny }, groups);
  };

  const handleSaveQueryPolicy = async (
    newAllowTypes: string[],
    newDenyTypes: string[],
    newDenyFlags: string[],
    groups?: number[]
  ) => {
    await savePolicy({ allow_types: newAllowTypes, deny_types: newDenyTypes, deny_flags: newDenyFlags }, groups);
  };

  const handleSaveRedisPolicy = async (newAllow: string[], newDeny: string[], groups?: number[]) => {
    await savePolicy({ allow_list: newAllow, deny_list: newDeny }, groups);
  };

  const handleGroupsChange = (newGroups: number[]) => {
    setPolicyGroups(newGroups);
    if (asset.Type === "database") {
      handleSaveQueryPolicy(queryAllowTypes, queryDenyTypes, queryDenyFlags, newGroups);
    } else if (asset.Type === "redis") {
      handleSaveRedisPolicy(redisAllowList, redisDenyList, newGroups);
    } else {
      handleSaveSSHPolicy(allowList, denyList, newGroups);
    }
  };

  const handleResetPolicy = async () => {
    try {
      const defaultJSON = await GetDefaultPolicy(asset.Type);
      const policy = JSON.parse(defaultJSON);
      const groups = policy.groups || [];
      setPolicyGroups(groups);
      if (asset.Type === "database") {
        setQueryAllowTypes(policy.allow_types || []);
        setQueryDenyTypes(policy.deny_types || []);
        setQueryDenyFlags(policy.deny_flags || []);
        await savePolicy(
          { allow_types: policy.allow_types, deny_types: policy.deny_types, deny_flags: policy.deny_flags },
          groups
        );
      } else if (asset.Type === "redis") {
        setRedisAllowList(policy.allow_list || []);
        setRedisDenyList(policy.deny_list || []);
        await savePolicy({ allow_list: policy.allow_list, deny_list: policy.deny_list }, groups);
      } else {
        setAllowList(policy.allow_list || []);
        setDenyList(policy.deny_list || []);
        await savePolicy({ allow_list: policy.allow_list, deny_list: policy.deny_list }, groups);
      }
    } catch (e) {
      toast.error(String(e));
    }
  };

  // Parse config based on type
  let sshConfig: SSHConfig | null = null;
  let dbConfig: DatabaseConfig | null = null;
  let redisConfig: RedisConfig | null = null;
  try {
    const parsed = JSON.parse(asset.Config || "{}");
    if (asset.Type === "database") dbConfig = parsed;
    else if (asset.Type === "redis") redisConfig = parsed;
    else sshConfig = parsed;
  } catch {
    /* ignore */
  }

  const jumpHostName = sshConfig?.jump_host_id
    ? assets.find((a) => a.ID === sshConfig!.jump_host_id)?.Name || `ID:${sshConfig.jump_host_id}`
    : null;

  const sshTunnelName = (id?: number) => {
    if (!id) return null;
    return assets.find((a) => a.ID === id)?.Name || `ID:${id}`;
  };

  const HeaderIcon = asset.Type === "database" ? Database : Server;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b">
        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
            <HeaderIcon className="h-4 w-4 text-primary" />
          </div>
          <div>
            <h2 className="font-semibold leading-tight">{asset.Name}</h2>
            <span className="text-xs text-muted-foreground uppercase">{asset.Type}</span>
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
      <ConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        title={t("asset.deleteAssetTitle")}
        description={t("asset.deleteAssetDesc", { name: asset.Name })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={onDelete}
      />
      <div className="flex-1 p-4 space-y-4 overflow-y-auto">
        {/* SSH Connection Info */}
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
                      ? t("asset.authKey") +
                        (sshConfig.credential_id
                          ? ` (${t("asset.keySourceManaged")})`
                          : sshConfig.private_keys?.length
                            ? ` (${t("asset.keySourceFile")})`
                            : "")
                      : sshConfig.auth_type
                }
              />
            </div>
          </div>
        )}

        {/* SSH Private Keys */}
        {sshConfig?.private_keys && sshConfig.private_keys.length > 0 && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.privateKeys")}
            </h3>
            <div className="space-y-1">
              {sshConfig.private_keys.map((key, i) => (
                <p key={i} className="text-sm font-mono text-muted-foreground">
                  {key}
                </p>
              ))}
            </div>
          </div>
        )}

        {/* SSH Jump Host */}
        {jumpHostName && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.jumpHost")}
            </h3>
            <p className="text-sm font-mono">{jumpHostName}</p>
          </div>
        )}

        {/* SSH Proxy */}
        {sshConfig?.proxy && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.proxy")}
            </h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <InfoItem label={t("asset.proxyType")} value={sshConfig.proxy.type.toUpperCase()} />
              <InfoItem label={t("asset.proxyHost")} value={`${sshConfig.proxy.host}:${sshConfig.proxy.port}`} mono />
              {sshConfig.proxy.username && (
                <InfoItem label={t("asset.proxyUsername")} value={sshConfig.proxy.username} />
              )}
            </div>
          </div>
        )}

        {/* Database Connection Info */}
        {dbConfig && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t("asset.typeDatabase")}
            </h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <InfoItem label={t("asset.driver")} value={dbConfig.driver === "postgresql" ? "PostgreSQL" : "MySQL"} />
              <InfoItem label={t("asset.host")} value={`${dbConfig.host}:${dbConfig.port}`} mono />
              <InfoItem label={t("asset.username")} value={dbConfig.username} mono />
              {dbConfig.database && <InfoItem label={t("asset.database")} value={dbConfig.database} mono />}
              {dbConfig.password && <InfoItem label={t("asset.password")} value="●●●●●●" />}
              {dbConfig.ssl_mode && dbConfig.ssl_mode !== "disable" && (
                <InfoItem label={t("asset.sslMode")} value={dbConfig.ssl_mode} />
              )}
              {dbConfig.tls && <InfoItem label="TLS" value="✓" />}
              {dbConfig.read_only && <InfoItem label={t("asset.readOnly")} value="✓" />}
              {dbConfig.params && <InfoItem label={t("asset.params")} value={dbConfig.params} mono />}
            </div>
            {sshTunnelName(dbConfig.ssh_asset_id) && (
              <div className="mt-3 pt-3 border-t text-sm">
                <InfoItem label={t("asset.sshTunnel")} value={sshTunnelName(dbConfig.ssh_asset_id)!} mono />
              </div>
            )}
          </div>
        )}

        {/* Redis Connection Info */}
        {redisConfig && (
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">Redis</h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <InfoItem label={t("asset.host")} value={`${redisConfig.host}:${redisConfig.port}`} mono />
              {redisConfig.username && <InfoItem label={t("asset.username")} value={redisConfig.username} mono />}
              {redisConfig.password && <InfoItem label={t("asset.password")} value="●●●●●●" />}
              <InfoItem label={t("asset.redisDatabase")} value={String(redisConfig.database || 0)} mono />
              {redisConfig.tls && <InfoItem label={t("asset.tls")} value="✓" />}
            </div>
            {sshTunnelName(redisConfig.ssh_asset_id) && (
              <div className="mt-3 pt-3 border-t text-sm">
                <InfoItem label={t("asset.sshTunnel")} value={sshTunnelName(redisConfig.ssh_asset_id)!} mono />
              </div>
            )}
          </div>
        )}

        {/* SSH Command Policy */}
        {asset.Type === "ssh" && (
          <CommandPolicyCard
            title={t("asset.cmdPolicy")}
            policyType="ssh"
            lists={[
              {
                key: "allow_list",
                label: t("asset.cmdPolicyAllowList"),
                items: allowList,
                onAdd: (vals: string[]) => {
                  const next = [...allowList, ...vals];
                  setAllowList(next);
                  handleSaveSSHPolicy(next, denyList);
                },
                onRemove: (i) => {
                  const next = allowList.filter((_, idx) => idx !== i);
                  setAllowList(next);
                  handleSaveSSHPolicy(next, denyList);
                },
                placeholder: t("asset.cmdPolicyPlaceholder"),
                variant: "allow",
              },
              {
                key: "deny_list",
                label: t("asset.cmdPolicyDenyList"),
                items: denyList,
                onAdd: (vals: string[]) => {
                  const next = [...denyList, ...vals];
                  setDenyList(next);
                  handleSaveSSHPolicy(allowList, next);
                },
                onRemove: (i) => {
                  const next = denyList.filter((_, idx) => idx !== i);
                  setDenyList(next);
                  handleSaveSSHPolicy(allowList, next);
                },
                placeholder: t("asset.cmdPolicyPlaceholder"),
                variant: "deny",
              },
            ]}
            buildPolicyJSON={() =>
              JSON.stringify({
                allow_list: allowList,
                deny_list: denyList,
                ...(policyGroups.length > 0 ? { groups: policyGroups } : {}),
              })
            }
            hint={t("asset.cmdPolicyHint")}
            saving={savingPolicy}
            assetID={asset.ID}
            onReset={handleResetPolicy}
            referencedGroups={policyGroups}
            onGroupsChange={handleGroupsChange}
          />
        )}

        {/* Database Query Policy */}
        {asset.Type === "database" && (
          <CommandPolicyCard
            title={t("asset.queryPolicy")}
            policyType="database"
            lists={[
              {
                key: "allow_types",
                label: t("asset.queryPolicyAllowTypes"),
                items: queryAllowTypes,
                onAdd: (vals: string[]) => {
                  const next = [...queryAllowTypes, ...vals];
                  setQueryAllowTypes(next);
                  handleSaveQueryPolicy(next, queryDenyTypes, queryDenyFlags);
                },
                onRemove: (i) => {
                  const next = queryAllowTypes.filter((_, idx) => idx !== i);
                  setQueryAllowTypes(next);
                  handleSaveQueryPolicy(next, queryDenyTypes, queryDenyFlags);
                },
                placeholder: t("asset.queryPolicyPlaceholder"),
                variant: "allow",
              },
              {
                key: "deny_types",
                label: t("asset.queryPolicyDenyTypes"),
                items: queryDenyTypes,
                onAdd: (vals: string[]) => {
                  const next = [...queryDenyTypes, ...vals];
                  setQueryDenyTypes(next);
                  handleSaveQueryPolicy(queryAllowTypes, next, queryDenyFlags);
                },
                onRemove: (i) => {
                  const next = queryDenyTypes.filter((_, idx) => idx !== i);
                  setQueryDenyTypes(next);
                  handleSaveQueryPolicy(queryAllowTypes, next, queryDenyFlags);
                },
                placeholder: t("asset.queryPolicyPlaceholder"),
                variant: "deny",
              },
              {
                key: "deny_flags",
                label: t("asset.queryPolicyDenyFlags"),
                items: queryDenyFlags,
                onAdd: (vals: string[]) => {
                  const next = [...queryDenyFlags, ...vals];
                  setQueryDenyFlags(next);
                  handleSaveQueryPolicy(queryAllowTypes, queryDenyTypes, next);
                },
                onRemove: (i) => {
                  const next = queryDenyFlags.filter((_, idx) => idx !== i);
                  setQueryDenyFlags(next);
                  handleSaveQueryPolicy(queryAllowTypes, queryDenyTypes, next);
                },
                placeholder: t("asset.queryPolicyFlagPlaceholder"),
                variant: "warn",
              },
            ]}
            buildPolicyJSON={() =>
              JSON.stringify({
                allow_types: queryAllowTypes,
                deny_types: queryDenyTypes,
                deny_flags: queryDenyFlags,
                ...(policyGroups.length > 0 ? { groups: policyGroups } : {}),
              })
            }
            hint={t("asset.queryPolicyHint")}
            saving={savingPolicy}
            assetID={asset.ID}
            onReset={handleResetPolicy}
            referencedGroups={policyGroups}
            onGroupsChange={handleGroupsChange}
          />
        )}

        {/* Redis Policy */}
        {asset.Type === "redis" && (
          <CommandPolicyCard
            title={t("asset.redisPolicy")}
            policyType="redis"
            lists={[
              {
                key: "allow_list",
                label: t("asset.redisPolicyAllowList"),
                items: redisAllowList,
                onAdd: (vals: string[]) => {
                  const next = [...redisAllowList, ...vals];
                  setRedisAllowList(next);
                  handleSaveRedisPolicy(next, redisDenyList);
                },
                onRemove: (i) => {
                  const next = redisAllowList.filter((_, idx) => idx !== i);
                  setRedisAllowList(next);
                  handleSaveRedisPolicy(next, redisDenyList);
                },
                placeholder: t("asset.redisPolicyPlaceholder"),
                variant: "allow",
              },
              {
                key: "deny_list",
                label: t("asset.redisPolicyDenyList"),
                items: redisDenyList,
                onAdd: (vals: string[]) => {
                  const next = [...redisDenyList, ...vals];
                  setRedisDenyList(next);
                  handleSaveRedisPolicy(redisAllowList, next);
                },
                onRemove: (i) => {
                  const next = redisDenyList.filter((_, idx) => idx !== i);
                  setRedisDenyList(next);
                  handleSaveRedisPolicy(redisAllowList, next);
                },
                placeholder: t("asset.redisPolicyPlaceholder"),
                variant: "deny",
              },
            ]}
            buildPolicyJSON={() =>
              JSON.stringify({
                allow_list: redisAllowList,
                deny_list: redisDenyList,
                ...(policyGroups.length > 0 ? { groups: policyGroups } : {}),
              })
            }
            hint={t("asset.redisPolicyHint")}
            saving={savingPolicy}
            assetID={asset.ID}
            onReset={handleResetPolicy}
            referencedGroups={policyGroups}
            onGroupsChange={handleGroupsChange}
          />
        )}

        {/* Extension Panel */}
        {isExtensionAssetType(asset.Type) && <ExtensionPanel assetType={asset.Type} assetId={asset.ID} />}

        {asset.Description && (
          <>
            <Separator />
            <div className="text-sm">
              <span className="text-muted-foreground">{t("asset.description")}</span>
              <div className="mt-1 prose prose-sm dark:prose-invert prose-p:my-1 prose-pre:my-1 prose-pre:overflow-x-auto max-w-none">
                <Markdown remarkPlugins={[remarkBreaks]} rehypePlugins={[rehypeSanitize]}>
                  {asset.Description}
                </Markdown>
              </div>
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
