import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Plus, X, Shield, Lock, Puzzle } from "lucide-react";
import { Button, Popover, PopoverContent, PopoverTrigger } from "@opskat/ui";
import { ListPolicyGroups } from "../../../wailsjs/go/app/App";
import { loadExtensionLocales } from "@/extension/i18n";
import type { policy_group_entity } from "../../../wailsjs/go/models";

interface PolicyGroupSelectorProps {
  policyType: string;
  selectedIds: string[];
  onChange: (ids: string[]) => void;
  refreshKey?: number;
}

const policyTypeMap: Record<string, string> = {
  ssh: "command",
  k8s: "command",
  database: "query",
  redis: "redis",
  mongodb: "mongo",
  mongo: "mongo",
  kafka: "kafka",
};

/** 获取内置权限组的 i18n 短 ID（去掉 builtin: 前缀） */
function builtinShortId(id: string): string {
  return id.replace("builtin:", "");
}

export function PolicyGroupSelector({ policyType, selectedIds, onChange, refreshKey }: PolicyGroupSelectorProps) {
  const { t } = useTranslation();
  const [groups, setGroups] = useState<policy_group_entity.PolicyGroupItem[]>([]);
  const [open, setOpen] = useState(false);

  const fetchGroups = async () => {
    try {
      const items = await ListPolicyGroups(policyTypeMap[policyType] || policyType);
      // 先加载扩展 i18n，再 setGroups 触发渲染，避免首次显示 i18n key
      const extNames = new Set((items || []).filter((g) => g.extensionName).map((g) => g.extensionName!));
      for (const name of extNames) {
        await loadExtensionLocales(name);
      }
      setGroups(items || []);
    } catch {
      setGroups([]);
    }
  };

  useEffect(() => {
    fetchGroups();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [policyType, refreshKey]);

  const displayName = (g: policy_group_entity.PolicyGroupItem) => {
    if (g.extensionName) return t(g.name, { ns: `ext-${g.extensionName}` });
    return g.builtin ? t(`asset.policyGroup.builtin.${builtinShortId(g.id)}.name`) : g.name;
  };

  const displayDesc = (g: policy_group_entity.PolicyGroupItem) => {
    if (g.extensionName) return t(g.description, { ns: `ext-${g.extensionName}` });
    return g.builtin ? t(`asset.policyGroup.builtin.${builtinShortId(g.id)}.desc`) : g.description;
  };

  const selectedGroups = groups.filter((g) => selectedIds.includes(g.id));
  const availableGroups = groups.filter((g) => !selectedIds.includes(g.id));

  const handleAdd = (id: string) => {
    onChange([...selectedIds, id]);
  };

  const handleRemove = (id: string) => {
    onChange(selectedIds.filter((i) => i !== id));
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Shield className="h-3 w-3 text-indigo-500" />
        <span className="text-[11px] font-medium text-muted-foreground">{t("asset.policyGroup.referenced")}</span>
      </div>

      <div className="flex flex-wrap gap-1.5">
        {selectedGroups.map((g) => (
          <span
            key={g.id}
            className="inline-flex items-center gap-1 rounded-md border border-indigo-200 bg-indigo-50 px-2 py-0.5 text-[11px] text-indigo-700 dark:border-indigo-800 dark:bg-indigo-950 dark:text-indigo-300"
          >
            {g.extensionName ? <Puzzle className="h-2.5 w-2.5" /> : g.builtin && <Lock className="h-2.5 w-2.5" />}
            {displayName(g)}
            <button
              onClick={() => handleRemove(g.id)}
              className="ml-0.5 rounded-sm hover:bg-indigo-200 dark:hover:bg-indigo-800"
            >
              <X className="h-2.5 w-2.5" />
            </button>
          </span>
        ))}

        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="sm" className="h-5 w-5 p-0 text-muted-foreground hover:text-foreground">
              <Plus className="h-3 w-3" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-56 p-1" align="start">
            {availableGroups.length === 0 ? (
              <div className="px-2 py-3 text-center text-xs text-muted-foreground">{t("asset.policyGroup.noMore")}</div>
            ) : (
              <div className="max-h-48 overflow-y-auto">
                {availableGroups.map((g) => (
                  <button
                    key={g.id}
                    className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-xs hover:bg-accent"
                    onClick={() => {
                      handleAdd(g.id);
                      setOpen(false);
                    }}
                  >
                    {g.extensionName ? (
                      <Puzzle className="h-3 w-3 text-muted-foreground shrink-0" />
                    ) : (
                      g.builtin && <Lock className="h-3 w-3 text-muted-foreground shrink-0" />
                    )}
                    <div className="flex-1 text-left">
                      <div className="font-medium">{displayName(g)}</div>
                      {displayDesc(g) && <div className="text-[10px] text-muted-foreground">{displayDesc(g)}</div>}
                    </div>
                  </button>
                ))}
              </div>
            )}
          </PopoverContent>
        </Popover>
      </div>
    </div>
  );
}
