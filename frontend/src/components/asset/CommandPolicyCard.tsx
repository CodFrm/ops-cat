import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Shield, Info, RotateCcw, Settings2 } from "lucide-react";
import { PolicyTagEditor, type PolicyVariant } from "@/components/asset/PolicyTagEditor";
import { PolicyTestPanel } from "@/components/asset/PolicyTestPanel";
import { PolicyGroupSelector } from "@/components/asset/PolicyGroupSelector";
import { PolicyGroupManager } from "@/components/asset/PolicyGroupManager";
import { Button, ConfirmDialog } from "@opskat/ui";

type PolicyType = string;

export interface PolicyList {
  key: string;
  label: string;
  items: string[];
  onAdd: (vals: string[]) => void;
  onRemove: (idx: number) => void;
  placeholder: string;
  variant: PolicyVariant;
}

interface CommandPolicyCardProps {
  title: string;
  policyType: PolicyType;
  lists: PolicyList[];
  buildPolicyJSON: () => string;
  hint?: string;
  saving?: boolean;
  assetID?: number;
  groupID?: number;
  onReset?: () => void;
  referencedGroups?: string[];
  onGroupsChange?: (ids: string[]) => void;
}

export function CommandPolicyCard({
  title,
  policyType,
  lists,
  buildPolicyJSON,
  hint,
  saving,
  assetID,
  groupID,
  onReset,
  referencedGroups,
  onGroupsChange,
}: CommandPolicyCardProps) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [managerOpen, setManagerOpen] = useState(false);
  const [selectorRefreshKey, setSelectorRefreshKey] = useState(0);

  const allowLists = lists.filter((l) => l.variant === "allow");
  const denyLists = lists.filter((l) => l.variant !== "allow");
  const isGroup = !!groupID;

  const managerTab =
    policyType === "ssh"
      ? ("command" as const)
      : policyType === "database"
        ? ("query" as const)
        : policyType === "redis"
          ? ("redis" as const)
          : (policyType as string);

  return (
    <div className="rounded-xl border bg-card p-4">
      {/* Header */}
      <div className="flex items-center gap-2 mb-3">
        <Shield className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{title}</h3>
        <div className="ml-auto flex items-center gap-2">
          {saving && <span className="text-[10px] text-muted-foreground">{t("settings.saved")}...</span>}
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-[10px] gap-1 text-muted-foreground hover:text-foreground"
            onClick={() => setManagerOpen(true)}
          >
            <Settings2 className="h-3 w-3" />
            {t("asset.policyGroup.manage")}
          </Button>
          {onReset && (
            <>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-[10px] gap-1 text-muted-foreground hover:text-foreground"
                onClick={() => setConfirmOpen(true)}
              >
                <RotateCcw className="h-3 w-3" />
                {isGroup ? t("asset.policyReset.clear") : t("asset.policyReset.default")}
              </Button>
              <ConfirmDialog
                open={confirmOpen}
                onOpenChange={setConfirmOpen}
                title={t("asset.policyReset.confirmTitle")}
                description={isGroup ? t("asset.policyReset.confirmClear") : t("asset.policyReset.confirmDefault")}
                cancelText={t("action.cancel")}
                confirmText={t("action.confirm")}
                onConfirm={() => {
                  setConfirmOpen(false);
                  onReset();
                }}
              />
            </>
          )}
        </div>
      </div>

      {/* Referenced policy groups */}
      {onGroupsChange && (
        <div className="mb-3">
          <PolicyGroupSelector
            policyType={policyType}
            selectedIds={referencedGroups || []}
            onChange={onGroupsChange}
            refreshKey={selectorRefreshKey}
          />
        </div>
      )}

      {/* Two-column layout */}
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-3">
          {allowLists.map((list) => (
            <PolicyTagEditor
              key={list.key}
              label={list.label}
              items={list.items}
              onAdd={list.onAdd}
              onRemove={list.onRemove}
              placeholder={list.placeholder}
              variant={list.variant}
            />
          ))}
        </div>
        <div className="space-y-3">
          {denyLists.map((list) => (
            <PolicyTagEditor
              key={list.key}
              label={list.label}
              items={list.items}
              onAdd={list.onAdd}
              onRemove={list.onRemove}
              placeholder={list.placeholder}
              variant={list.variant}
            />
          ))}
        </div>
      </div>

      {/* Hint */}
      {hint && (
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-muted/50 text-[11px] text-muted-foreground mt-3">
          <Info className="h-3 w-3 shrink-0" />
          {hint}
        </div>
      )}

      {/* Test panel */}
      <PolicyTestPanel policyType={policyType} buildPolicyJSON={buildPolicyJSON} assetID={assetID} groupID={groupID} />

      {/* Policy Group Manager Dialog */}
      <PolicyGroupManager
        open={managerOpen}
        onOpenChange={setManagerOpen}
        onGroupsChanged={() => setSelectorRefreshKey((k) => k + 1)}
        initialTab={managerTab}
      />
    </div>
  );
}
