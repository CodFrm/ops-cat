import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Folder } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAssetStore } from "@/stores/assetStore";
import { group_entity } from "../../../wailsjs/go/models";
import { UpdateGroup } from "../../../wailsjs/go/main/App";
import { toast } from "sonner";
import { getIconComponent } from "@/components/asset/IconPicker";

interface GroupDetailProps {
  group: group_entity.Group;
}

export function GroupDetail({ group }: GroupDetailProps) {
  const { t } = useTranslation();
  const { assets, groups, fetchGroups } = useAssetStore();

  const [allowList, setAllowList] = useState<string[]>([]);
  const [denyList, setDenyList] = useState<string[]>([]);
  const [allowInput, setAllowInput] = useState("");
  const [denyInput, setDenyInput] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    try {
      const policy = JSON.parse(group.CmdPolicy || "{}");
      setAllowList(policy.allow_list || []);
      setDenyList(policy.deny_list || []);
    } catch {
      setAllowList([]);
      setDenyList([]);
    }
    setAllowInput("");
    setDenyInput("");
  }, [group.ID, group.CmdPolicy]);

  const GroupIcon = group.Icon ? getIconComponent(group.Icon) : Folder;
  const parentGroup = groups.find((g) => g.ID === group.ParentID);
  const assetCount = assets.filter((a) => a.GroupID === group.ID).length;

  const handleSave = async () => {
    let cmdPolicy = "";
    if (allowList.length > 0 || denyList.length > 0) {
      cmdPolicy = JSON.stringify({
        allow_list: allowList.length > 0 ? allowList : undefined,
        deny_list: denyList.length > 0 ? denyList : undefined,
      });
    }

    const updated = new group_entity.Group({
      ...group,
      CmdPolicy: cmdPolicy,
    });

    setSaving(true);
    try {
      await UpdateGroup(updated);
      await fetchGroups();
      toast.success(t("settings.saved"));
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-3 border-b">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
          <GroupIcon className="h-4 w-4 text-primary" />
        </div>
        <div>
          <h2 className="font-semibold leading-tight">{group.Name}</h2>
          <span className="text-xs text-muted-foreground">
            {t("asset.groupDetailTitle")}
          </span>
        </div>
      </div>

      <div className="flex-1 p-4 space-y-4 overflow-y-auto">
        {/* Basic info */}
        <div className="rounded-xl border bg-card p-4">
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-xs text-muted-foreground">
                {t("asset.name")}
              </span>
              <p className="mt-0.5 font-medium">{group.Name}</p>
            </div>
            <div>
              <span className="text-xs text-muted-foreground">
                {t("asset.parentGroup")}
              </span>
              <p className="mt-0.5">
                {parentGroup?.Name || t("asset.parentGroupNone")}
              </p>
            </div>
            <div>
              <span className="text-xs text-muted-foreground">
                {t("asset.groupAssetCount")}
              </span>
              <p className="mt-0.5">{assetCount}</p>
            </div>
          </div>
        </div>

        {/* Command Policy */}
        <div className="rounded-xl border bg-card p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t("asset.cmdPolicy")}
          </h3>

          {/* Allow list */}
          <div className="grid gap-2 mb-3">
            <Label className="text-xs">
              {t("asset.cmdPolicyAllowList")}
            </Label>
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
                    onClick={() =>
                      setAllowList(allowList.filter((_, idx) => idx !== i))
                    }
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
                  setAllowList([...allowList, allowInput.trim()]);
                  setAllowInput("");
                }
              }}
              placeholder={t("asset.cmdPolicyPlaceholder")}
            />
          </div>

          {/* Deny list */}
          <div className="grid gap-2 mb-3">
            <Label className="text-xs">
              {t("asset.cmdPolicyDenyList")}
            </Label>
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
                    onClick={() =>
                      setDenyList(denyList.filter((_, idx) => idx !== i))
                    }
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
                  setDenyList([...denyList, denyInput.trim()]);
                  setDenyInput("");
                }
              }}
              placeholder={t("asset.cmdPolicyPlaceholder")}
            />
          </div>

          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground">
              {t("asset.cmdPolicyGroupHint")}
            </p>
            <Button size="sm" onClick={handleSave} disabled={saving}>
              {t("action.save")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
