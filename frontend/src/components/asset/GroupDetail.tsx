import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Folder } from "lucide-react";
import { useAssetStore } from "@/stores/assetStore";
import { CommandPolicyCard } from "@/components/asset/CommandPolicyCard";
import { DetailGrid, DetailSection, InfoItem } from "@/components/asset/detail/InfoItem";
import { group_entity } from "../../../wailsjs/go/models";
import { UpdateGroup } from "../../../wailsjs/go/app/App";
import { toast } from "sonner";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";

interface GroupDetailProps {
  group: group_entity.Group;
}

export function GroupDetail({ group }: GroupDetailProps) {
  const { t } = useTranslation();
  const { assets, groups, fetchGroups } = useAssetStore();

  const [allowList, setAllowList] = useState<string[]>([]);
  const [denyList, setDenyList] = useState<string[]>([]);
  const [policyGroups, setPolicyGroups] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    try {
      const policy = JSON.parse(group.CmdPolicy || "{}");
      setAllowList(policy.allow_list || []);
      setDenyList(policy.deny_list || []);
      setPolicyGroups(policy.groups || []);
    } catch {
      setAllowList([]);
      setDenyList([]);
      setPolicyGroups([]);
    }
  }, [group.ID, group.CmdPolicy]);

  const GroupIcon = group.Icon ? getIconComponent(group.Icon) : Folder;
  const parentGroup = groups.find((g) => g.ID === group.ParentID);
  const assetCount = assets.filter((a) => a.GroupID === group.ID).length;

  const savePolicy = async (newAllow: string[], newDeny: string[], groups?: string[]) => {
    const grps = groups ?? policyGroups;
    const policyObj: Record<string, unknown> = {};
    if (newAllow.length > 0) policyObj.allow_list = newAllow;
    if (newDeny.length > 0) policyObj.deny_list = newDeny;
    if (grps.length > 0) policyObj.groups = grps;
    const cmdPolicy = Object.keys(policyObj).length > 0 ? JSON.stringify(policyObj) : "";
    const updated = new group_entity.Group({ ...group, CmdPolicy: cmdPolicy });
    setSaving(true);
    try {
      await UpdateGroup(updated);
      await fetchGroups();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  const handleGroupsChange = (newGroups: string[]) => {
    setPolicyGroups(newGroups);
    savePolicy(allowList, denyList, newGroups);
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-3 border-b">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
          <GroupIcon
            className="h-4 w-4 text-primary"
            style={group.Icon ? { color: getIconColor(group.Icon) } : undefined}
          />
        </div>
        <div>
          <h2 className="font-semibold leading-tight">{group.Name}</h2>
          <span className="text-xs text-muted-foreground">{t("asset.groupDetailTitle")}</span>
        </div>
      </div>

      <div className="flex-1 p-4 space-y-4 overflow-y-auto">
        {/* Basic info */}
        <DetailSection title={t("asset.groupDetailTitle")}>
          <DetailGrid>
            <InfoItem label={t("asset.name")} value={group.Name} />
            <InfoItem label={t("asset.parentGroup")} value={parentGroup?.Name || t("asset.parentGroupNone")} />
            <InfoItem label={t("asset.groupAssetCount")} value={String(assetCount)} />
          </DetailGrid>
          {group.Description && (
            <div className="mt-3 border-t pt-3">
              <span className="text-xs text-muted-foreground">{t("asset.description")}</span>
              <p className="mt-0.5 text-sm whitespace-pre-wrap">{group.Description}</p>
            </div>
          )}
        </DetailSection>

        {/* Command Policy */}
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
                savePolicy(next, denyList);
              },
              onRemove: (i) => {
                const next = allowList.filter((_, idx) => idx !== i);
                setAllowList(next);
                savePolicy(next, denyList);
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
                savePolicy(allowList, next);
              },
              onRemove: (i) => {
                const next = denyList.filter((_, idx) => idx !== i);
                setDenyList(next);
                savePolicy(allowList, next);
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
          hint={t("asset.cmdPolicyGroupHint")}
          groupID={group.ID}
          saving={saving}
          referencedGroups={policyGroups}
          onGroupsChange={handleGroupsChange}
          onReset={() => {
            setAllowList([]);
            setDenyList([]);
            setPolicyGroups([]);
            savePolicy([], [], []);
          }}
        />
      </div>
    </div>
  );
}
