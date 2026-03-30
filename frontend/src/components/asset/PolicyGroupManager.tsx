import { useState, useEffect, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Lock, Copy, Trash2, Plus, Save, ChevronDown, ChevronRight } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, Button, Input, Separator, ConfirmDialog } from "@opskat/ui";
import { PolicyTagEditor } from "@/components/asset/PolicyTagEditor";
import { toast } from "sonner";
import {
  ListPolicyGroups,
  CreatePolicyGroup,
  UpdatePolicyGroup,
  DeletePolicyGroup,
  CopyPolicyGroup,
} from "../../../wailsjs/go/app/App";
import { policy_group_entity } from "../../../wailsjs/go/models";

type TabType = "command" | "query" | "redis";

interface PolicyGroupManagerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onGroupsChanged?: () => void;
  initialTab?: TabType;
}

const tabs: { key: TabType; label: string }[] = [
  { key: "command", label: "SSH" },
  { key: "query", label: "Database" },
  { key: "redis", label: "Redis" },
];

interface EditState {
  id: string;
  name: string;
  description: string;
  policyType: string;
  policy: Record<string, string[]>;
  readonly?: boolean;
}

/** 获取内置权限组的 i18n 短 ID（去掉 builtin: 前缀） */
function builtinShortId(id: string): string {
  return id.replace("builtin:", "");
}

function parsePolicyJSON(json: string, policyType: string): Record<string, string[]> {
  try {
    const p = JSON.parse(json);
    if (policyType === "query") {
      return {
        allow_types: p.allow_types || [],
        deny_types: p.deny_types || [],
        deny_flags: p.deny_flags || [],
      };
    }
    return {
      allow_list: p.allow_list || [],
      deny_list: p.deny_list || [],
    };
  } catch {
    return policyType === "query"
      ? { allow_types: [], deny_types: [], deny_flags: [] }
      : { allow_list: [], deny_list: [] };
  }
}

function serializePolicy(policy: Record<string, string[]>): string {
  const cleaned: Record<string, string[]> = {};
  for (const [k, v] of Object.entries(policy)) {
    if (v.length > 0) cleaned[k] = v;
  }
  return JSON.stringify(cleaned);
}

export function PolicyGroupManager({ open, onOpenChange, onGroupsChanged, initialTab }: PolicyGroupManagerProps) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<TabType>(initialTab || "command");
  const [groups, setGroups] = useState<policy_group_entity.PolicyGroupItem[]>([]);
  const [editState, setEditState] = useState<EditState | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ id: string; name: string } | null>(null);
  const changedRef = useRef(false);

  const displayName = useCallback(
    (g: policy_group_entity.PolicyGroupItem) =>
      g.builtin ? t(`asset.policyGroup.builtin.${builtinShortId(g.id)}.name`) : g.name,
    [t]
  );

  const displayDesc = useCallback(
    (g: policy_group_entity.PolicyGroupItem) =>
      g.builtin ? t(`asset.policyGroup.builtin.${builtinShortId(g.id)}.desc`) : g.description,
    [t]
  );

  const fetchGroups = useCallback(async () => {
    try {
      const items = await ListPolicyGroups(activeTab);
      setGroups(items || []);
    } catch {
      setGroups([]);
    }
  }, [activeTab]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) fetchGroups();
  }, [open, fetchGroups]);

  const notifyChanged = () => {
    changedRef.current = true;
  };

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen && changedRef.current) {
      changedRef.current = false;
      onGroupsChanged?.();
    }
    onOpenChange(newOpen);
  };

  const handleCopy = async (id: string) => {
    try {
      await CopyPolicyGroup(id, "");
      toast.success(t("asset.policyGroup.copySuccess"));
      notifyChanged();
      await fetchGroups();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await DeletePolicyGroup(id);
      toast.success(t("asset.policyGroup.deleteSuccess"));
      if (editState?.id === id) setEditState(null);
      notifyChanged();
      await fetchGroups();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleCreate = () => {
    setEditState({
      id: "",
      name: "",
      description: "",
      policyType: activeTab,
      policy:
        activeTab === "query" ? { allow_types: [], deny_types: [], deny_flags: [] } : { allow_list: [], deny_list: [] },
    });
  };

  const handleEdit = (g: policy_group_entity.PolicyGroupItem) => {
    setEditState({
      id: g.id,
      name: g.name,
      description: g.description,
      policyType: g.policyType,
      policy: parsePolicyJSON(g.policy, g.policyType),
    });
  };

  const handleViewBuiltin = (g: policy_group_entity.PolicyGroupItem) => {
    if (editState?.id === g.id && editState?.readonly) {
      setEditState(null);
      return;
    }
    setEditState({
      id: g.id,
      name: displayName(g),
      description: displayDesc(g),
      policyType: g.policyType,
      policy: parsePolicyJSON(g.policy, g.policyType),
      readonly: true,
    });
  };

  const handleSave = async () => {
    if (!editState || !editState.name.trim()) {
      toast.error(t("asset.policyGroup.nameRequired"));
      return;
    }

    try {
      const policyJSON = serializePolicy(editState.policy);
      if (editState.id === "") {
        await CreatePolicyGroup(
          new policy_group_entity.PolicyGroup({
            name: editState.name,
            description: editState.description,
            policyType: editState.policyType,
            policy: policyJSON,
          })
        );
        toast.success(t("asset.policyGroup.saveSuccess"));
      } else {
        await UpdatePolicyGroup(
          new policy_group_entity.PolicyGroup({
            id: parseInt(editState.id),
            name: editState.name,
            description: editState.description,
            policyType: editState.policyType,
            policy: policyJSON,
          })
        );
        toast.success(t("asset.policyGroup.saveSuccess"));
      }
      setEditState(null);
      notifyChanged();
      await fetchGroups();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const updatePolicyField = (field: string, value: string[]) => {
    if (!editState) return;
    setEditState({
      ...editState,
      policy: { ...editState.policy, [field]: value },
    });
  };

  const builtinGroups = groups.filter((g) => g.builtin);
  const customGroups = groups.filter((g) => !g.builtin);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-4xl max-h-[80vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>{t("asset.policyGroup.manage")}</DialogTitle>
        </DialogHeader>

        {/* Tabs */}
        <div className="flex gap-1 border-b">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              className={`px-3 py-1.5 text-xs font-medium border-b-2 transition-colors ${
                activeTab === tab.key
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
              onClick={() => {
                setActiveTab(tab.key);
                setEditState(null);
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto space-y-4 py-2">
          {/* Built-in groups */}
          {builtinGroups.length > 0 && (
            <div>
              <div className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider mb-2">
                {t("asset.policyGroup.builtinSection")}
              </div>
              <div className="space-y-1">
                {builtinGroups.map((g) => (
                  <div
                    key={g.id}
                    className={`flex items-center gap-2 rounded-md border px-3 py-2 cursor-pointer hover:bg-accent/50 ${
                      editState?.id === g.id && editState?.readonly ? "border-primary bg-accent/50" : ""
                    }`}
                    onClick={() => handleViewBuiltin(g)}
                  >
                    {editState?.id === g.id && editState?.readonly ? (
                      <ChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
                    ) : (
                      <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />
                    )}
                    <Lock className="h-3 w-3 text-muted-foreground shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="text-xs font-medium truncate">{displayName(g)}</div>
                      {displayDesc(g) && (
                        <div className="text-[10px] text-muted-foreground truncate">{displayDesc(g)}</div>
                      )}
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 px-2 text-[10px] gap-1"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleCopy(g.id);
                      }}
                    >
                      <Copy className="h-3 w-3" />
                      {t("asset.policyGroup.copy")}
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Custom groups */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <div className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
                {t("asset.policyGroup.customSection")}
              </div>
              <Button variant="outline" size="sm" className="h-6 px-2 text-[10px] gap-1" onClick={handleCreate}>
                <Plus className="h-3 w-3" />
                {t("asset.policyGroup.create")}
              </Button>
            </div>
            {customGroups.length === 0 && !editState && (
              <div className="text-center text-xs text-muted-foreground py-4">{t("asset.policyGroup.empty")}</div>
            )}
            <div className="space-y-1">
              {customGroups.map((g) => (
                <div
                  key={g.id}
                  className={`flex items-center gap-2 rounded-md border px-3 py-2 cursor-pointer hover:bg-accent/50 ${
                    editState?.id === g.id ? "border-primary bg-accent/50" : ""
                  }`}
                  onClick={() => handleEdit(g)}
                >
                  <div className="flex-1 min-w-0">
                    <div className="text-xs font-medium truncate">{g.name}</div>
                    {g.description && <div className="text-[10px] text-muted-foreground truncate">{g.description}</div>}
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteConfirm({ id: g.id, name: g.name });
                    }}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              ))}
            </div>
          </div>

          {/* Edit/View panel */}
          {editState && (
            <>
              <Separator />
              <div className="space-y-3">
                {!editState.readonly && (
                  <div className="flex items-center gap-2">
                    <Input
                      className="h-7 text-xs flex-1"
                      placeholder={t("asset.policyGroup.namePlaceholder")}
                      value={editState.name}
                      onChange={(e) => setEditState({ ...editState, name: e.target.value })}
                    />
                    <Button size="sm" className="h-7 px-3 text-xs gap-1" onClick={handleSave}>
                      <Save className="h-3 w-3" />
                      {t("asset.policyGroup.save")}
                    </Button>
                  </div>
                )}

                {!editState.readonly && (
                  <Input
                    className="h-7 text-xs"
                    placeholder={t("asset.policyGroup.descPlaceholder")}
                    value={editState.description}
                    onChange={(e) => setEditState({ ...editState, description: e.target.value })}
                  />
                )}

                {editState.policyType === "query" ? (
                  <div className="grid grid-cols-2 gap-3">
                    <PolicyTagEditor
                      label={t("asset.queryPolicyAllowTypes")}
                      items={editState.policy.allow_types || []}
                      onAdd={
                        editState.readonly
                          ? undefined
                          : (vals: string[]) =>
                              updatePolicyField("allow_types", [...(editState.policy.allow_types || []), ...vals])
                      }
                      onRemove={
                        editState.readonly
                          ? undefined
                          : (i) =>
                              updatePolicyField(
                                "allow_types",
                                (editState.policy.allow_types || []).filter((_, idx) => idx !== i)
                              )
                      }
                      placeholder={t("asset.queryPolicyPlaceholder")}
                      variant="allow"
                    />
                    <div className="space-y-3">
                      <PolicyTagEditor
                        label={t("asset.queryPolicyDenyTypes")}
                        items={editState.policy.deny_types || []}
                        onAdd={
                          editState.readonly
                            ? undefined
                            : (vals: string[]) =>
                                updatePolicyField("deny_types", [...(editState.policy.deny_types || []), ...vals])
                        }
                        onRemove={
                          editState.readonly
                            ? undefined
                            : (i) =>
                                updatePolicyField(
                                  "deny_types",
                                  (editState.policy.deny_types || []).filter((_, idx) => idx !== i)
                                )
                        }
                        placeholder={t("asset.queryPolicyPlaceholder")}
                        variant="deny"
                      />
                      <PolicyTagEditor
                        label={t("asset.queryPolicyDenyFlags")}
                        items={editState.policy.deny_flags || []}
                        onAdd={
                          editState.readonly
                            ? undefined
                            : (vals: string[]) =>
                                updatePolicyField("deny_flags", [...(editState.policy.deny_flags || []), ...vals])
                        }
                        onRemove={
                          editState.readonly
                            ? undefined
                            : (i) =>
                                updatePolicyField(
                                  "deny_flags",
                                  (editState.policy.deny_flags || []).filter((_, idx) => idx !== i)
                                )
                        }
                        placeholder={t("asset.queryPolicyFlagPlaceholder")}
                        variant="warn"
                      />
                    </div>
                  </div>
                ) : (
                  <div className="grid grid-cols-2 gap-3">
                    <PolicyTagEditor
                      label={t("asset.cmdPolicyAllowList")}
                      items={editState.policy.allow_list || []}
                      onAdd={
                        editState.readonly
                          ? undefined
                          : (vals: string[]) =>
                              updatePolicyField("allow_list", [...(editState.policy.allow_list || []), ...vals])
                      }
                      onRemove={
                        editState.readonly
                          ? undefined
                          : (i) =>
                              updatePolicyField(
                                "allow_list",
                                (editState.policy.allow_list || []).filter((_, idx) => idx !== i)
                              )
                      }
                      placeholder={t("asset.cmdPolicyPlaceholder")}
                      variant="allow"
                    />
                    <PolicyTagEditor
                      label={t("asset.cmdPolicyDenyList")}
                      items={editState.policy.deny_list || []}
                      onAdd={
                        editState.readonly
                          ? undefined
                          : (vals: string[]) =>
                              updatePolicyField("deny_list", [...(editState.policy.deny_list || []), ...vals])
                      }
                      onRemove={
                        editState.readonly
                          ? undefined
                          : (i) =>
                              updatePolicyField(
                                "deny_list",
                                (editState.policy.deny_list || []).filter((_, idx) => idx !== i)
                              )
                      }
                      placeholder={t("asset.cmdPolicyPlaceholder")}
                      variant="deny"
                    />
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        {/* Delete confirmation dialog */}
        <ConfirmDialog
          open={!!deleteConfirm}
          onOpenChange={(open) => {
            if (!open) setDeleteConfirm(null);
          }}
          title={t("asset.policyGroup.deleteConfirmTitle")}
          description={t("asset.policyGroup.deleteConfirm", { name: deleteConfirm?.name })}
          cancelText={t("action.cancel")}
          confirmText={t("action.confirm")}
          onConfirm={() => {
            if (deleteConfirm) {
              handleDelete(deleteConfirm.id);
              setDeleteConfirm(null);
            }
          }}
        />
      </DialogContent>
    </Dialog>
  );
}
