import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Server, Pencil, Trash2, TerminalSquare, Loader2 } from "lucide-react";
import Markdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkBreaks from "remark-breaks";
import { Button, Separator, ConfirmDialog } from "@opskat/ui";
import { toast } from "sonner";
import { useAssetStore } from "@/stores/assetStore";
import { useExtensionStore } from "@/extension";
import { getAssetType, isBuiltinType } from "@/lib/assetTypes";
import { CommandPolicyCard } from "@/components/asset/CommandPolicyCard";
import { DetailGrid, DetailSection, InfoItem } from "@/components/asset/detail/InfoItem";
import { DISABLED_VALUE, ENABLED_VALUE, MASKED_SECRET, parseDetailConfig } from "@/components/asset/detail/utils";
import { asset_entity } from "../../../wailsjs/go/models";
import { GetDefaultPolicy } from "../../../wailsjs/go/app/App";

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
  const [savingPolicy, setSavingPolicy] = useState(false);

  const [policyFields, setPolicyFields] = useState<Record<string, string[]>>({});
  const [policyGroups, setPolicyGroups] = useState<string[]>([]);

  useEffect(() => {
    try {
      const parsed = JSON.parse(asset.CmdPolicy || "{}");
      setPolicyGroups(parsed.groups || []);
      const def = getAssetType(asset.Type);
      if (def?.policy) {
        const fields: Record<string, string[]> = {};
        for (const f of def.policy.fields) {
          fields[f.key] = parsed[f.key] || [];
        }
        setPolicyFields(fields);
      } else if (!isBuiltinType(asset.Type)) {
        // Extension types fallback
        setPolicyFields({
          allow_list: parsed.allow_list || [],
          deny_list: parsed.deny_list || [],
        });
      }
    } catch {
      setPolicyFields({});
      setPolicyGroups([]);
    }
  }, [asset.ID, asset.CmdPolicy, asset.Type]);

  const savePolicy = async (policyObj: Record<string, unknown>, groups?: string[]) => {
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

  const handleSavePolicyFields = async (updatedFields: Record<string, string[]>, groups?: string[]) => {
    await savePolicy(updatedFields, groups);
  };

  const handleGroupsChange = (newGroups: string[]) => {
    setPolicyGroups(newGroups);
    handleSavePolicyFields(policyFields, newGroups);
  };

  const handleResetPolicy = async () => {
    try {
      const defaultJSON = await GetDefaultPolicy(asset.Type);
      const parsed = JSON.parse(defaultJSON);
      const groups = parsed.groups || [];
      setPolicyGroups(groups);
      const def = getAssetType(asset.Type);
      const fields: Record<string, string[]> = {};
      if (def?.policy) {
        for (const f of def.policy.fields) {
          fields[f.key] = parsed[f.key] || [];
        }
      }
      setPolicyFields(fields);
      await savePolicy(fields, groups);
    } catch (e) {
      toast.error(String(e));
    }
  };

  // Extension asset info — subscribe to ready so we re-render when extensions load
  const extensionReady = useExtensionStore((s) => s.ready);
  const extInfo = extensionReady ? useExtensionStore.getState().getExtensionForAssetType(asset.Type) : undefined;
  const extAssetTypeDef = extInfo?.manifest.assetTypes?.find((at) => at.type === asset.Type);
  const hasConnectPage = !!extInfo?.manifest.frontend?.pages.find((p) => p.slot === "asset.connect");
  const isExtensionType = !isBuiltinType(asset.Type);

  // Show loading while extensions are initializing for extension asset types
  if (isExtensionType && !extensionReady) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const sshTunnelName = (id?: number) => {
    if (!id) return null;
    return assets.find((a) => a.ID === id)?.Name || `ID:${id}`;
  };

  const HeaderIcon = getAssetType(asset.Type)?.icon ?? Server;

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
          {(getAssetType(asset.Type)?.canConnect || hasConnectPage) && (
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
        {/* Builtin type Detail Info Card */}
        {(() => {
          const def = getAssetType(asset.Type);
          if (!def) return null;
          const Card = def.DetailInfoCard;
          return <Card asset={asset} sshTunnelName={sshTunnelName} />;
        })()}

        {/* Extension Config Info */}
        {extAssetTypeDef?.configSchema &&
          (() => {
            const schema = extAssetTypeDef.configSchema as {
              propertyOrder?: string[];
              properties?: Record<string, { title?: string; format?: string; type?: string }>;
            };
            const props = schema.properties ?? {};
            const order = schema.propertyOrder;
            const keys = order ? order.filter((k) => k in props) : Object.keys(props);
            const parsed = parseDetailConfig<Record<string, unknown>>(asset.Config) ?? {};
            return (
              <DetailSection title={extInfo?.manifest.i18n.displayName || asset.Type}>
                <DetailGrid>
                  {keys.map((key) => {
                    const prop = props[key];
                    if (!prop) return null;
                    const val = parsed[key];
                    if (val === undefined || val === null || val === "") return null;
                    return (
                      <InfoItem
                        key={key}
                        label={prop.title || key}
                        value={
                          prop.format === "password"
                            ? MASKED_SECRET
                            : prop.type === "boolean"
                              ? val
                                ? ENABLED_VALUE
                                : DISABLED_VALUE
                              : String(val)
                        }
                        mono={prop.type !== "boolean"}
                      />
                    );
                  })}
                </DetailGrid>
              </DetailSection>
            );
          })()}

        {/* Builtin type Policy Card */}
        {(() => {
          const def = getAssetType(asset.Type);
          if (!def?.policy) return null;
          const pol = def.policy;
          return (
            <CommandPolicyCard
              title={t(pol.titleKey)}
              policyType={pol.policyType}
              lists={pol.fields.map((f) => ({
                key: f.key,
                label: t(f.labelKey),
                items: policyFields[f.key] || [],
                onAdd: (vals: string[]) => {
                  const next = { ...policyFields, [f.key]: [...(policyFields[f.key] || []), ...vals] };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                onRemove: (i: number) => {
                  const next = {
                    ...policyFields,
                    [f.key]: (policyFields[f.key] || []).filter((_, idx) => idx !== i),
                  };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                placeholder: t(f.placeholderKey),
                variant: f.variant,
              }))}
              buildPolicyJSON={() =>
                JSON.stringify({
                  ...Object.fromEntries(pol.fields.map((f) => [f.key, policyFields[f.key] || []])),
                  ...(policyGroups.length > 0 ? { groups: policyGroups } : {}),
                })
              }
              hint={t(pol.hintKey)}
              saving={savingPolicy}
              assetID={asset.ID}
              onReset={handleResetPolicy}
              referencedGroups={policyGroups}
              onGroupsChange={handleGroupsChange}
            />
          );
        })()}

        {/* Extension Policy */}
        {extInfo?.manifest.policies && isExtensionType && (
          <CommandPolicyCard
            title={extInfo.manifest.i18n.displayName || asset.Type}
            policyType={extInfo.manifest.policies.type}
            lists={[
              {
                key: "allow_list",
                label: t("asset.cmdPolicyAllowList"),
                items: policyFields["allow_list"] || [],
                onAdd: (vals: string[]) => {
                  const next = { ...policyFields, allow_list: [...(policyFields["allow_list"] || []), ...vals] };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                onRemove: (i) => {
                  const next = {
                    ...policyFields,
                    allow_list: (policyFields["allow_list"] || []).filter((_, idx) => idx !== i),
                  };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                placeholder: extInfo.manifest.policies.actions.join(", "),
                variant: "allow",
              },
              {
                key: "deny_list",
                label: t("asset.cmdPolicyDenyList"),
                items: policyFields["deny_list"] || [],
                onAdd: (vals: string[]) => {
                  const next = { ...policyFields, deny_list: [...(policyFields["deny_list"] || []), ...vals] };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                onRemove: (i) => {
                  const next = {
                    ...policyFields,
                    deny_list: (policyFields["deny_list"] || []).filter((_, idx) => idx !== i),
                  };
                  setPolicyFields(next);
                  handleSavePolicyFields(next);
                },
                placeholder: extInfo.manifest.policies.actions.join(", "),
                variant: "deny",
              },
            ]}
            buildPolicyJSON={() =>
              JSON.stringify({
                ...Object.fromEntries(Object.entries(policyFields).filter(([, v]) => v.length > 0)),
                ...(policyGroups.length > 0 ? { groups: policyGroups } : {}),
              })
            }
            saving={savingPolicy}
            assetID={asset.ID}
            onReset={handleResetPolicy}
            referencedGroups={policyGroups}
            onGroupsChange={handleGroupsChange}
          />
        )}

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
