import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ShieldAlert, Terminal, Database, Server, Globe, FolderOpen, FileEdit, FilePlus } from "lucide-react";
import { Button, Input, Textarea } from "@opskat/ui";
import { RespondAIApproval } from "../../../wailsjs/go/app/App";
import { ai } from "../../../wailsjs/go/models";
import type { ContentBlock } from "@/stores/aiStore";

interface ApprovalBlockProps {
  block: ContentBlock;
}

// Local kind 由后端 hook_policy.go 的 kindLocal* 常量发出，对应 cago 内置工具
// （bash / write / edit）。这一组 kind 不绑资产，弹卡走"本机操作"分支。
const KIND_LOCAL_BASH = "local_bash";
const KIND_LOCAL_WRITE = "local_write";
const KIND_LOCAL_EDIT = "local_edit";

function isLocalKind(kind: string): boolean {
  return kind === KIND_LOCAL_BASH || kind === KIND_LOCAL_WRITE || kind === KIND_LOCAL_EDIT;
}

export function ApprovalBlock({ block }: ApprovalBlockProps) {
  const { t } = useTranslation();
  const isPending = block.status === "pending_confirm";
  const items = block.approvalItems || [];
  const kind = block.approvalKind || "single";
  const localKind = isLocalKind(kind);
  // bash 永远不开 allowAll；write / edit 走"本次会话起永久放行此工具"。
  const allowRemember = !localKind || kind === KIND_LOCAL_WRITE || kind === KIND_LOCAL_EDIT;

  const [editedCommands, setEditedCommands] = useState<Record<number, string>>(() => {
    const map: Record<number, string> = {};
    items.forEach((item, i) => {
      map[i] = item.command;
    });
    return map;
  });

  const [rememberMode, setRememberMode] = useState(false);

  // 确认/拒绝后不再显示
  if (!isPending) return null;

  const respond = (decision: string) => {
    if (!block.confirmId) return;

    const resp = new ai.ApprovalResponse();
    resp.decision = decision;

    if ((kind === "grant" || (kind === "single" && decision === "allowAll")) && decision !== "deny") {
      resp.edited_items = items.map((item, i) => {
        const edited = new ai.ApprovalItem();
        edited.type = item.type;
        edited.asset_id = item.asset_id;
        edited.asset_name = item.asset_name;
        edited.group_id = item.group_id || 0;
        edited.group_name = item.group_name || "";
        edited.command = editedCommands[i] || item.command;
        edited.detail = item.detail || "";
        return edited;
      });
    }

    RespondAIApproval(block.confirmId, resp);
  };

  return (
    <div className="my-2 rounded-[10px] border border-[#F59E0B40] bg-[#2D2410] p-4 space-y-3 text-xs overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <ShieldAlert className="h-4 w-4 shrink-0 text-amber-500" />
          <span className="font-semibold text-[13px] text-amber-500">
            {kind === "grant"
              ? t("ai.approvalGrantTitle")
              : kind === "batch"
                ? t("ai.approvalBatchTitle", { count: items.length })
                : localKind
                  ? t(`ai.approvalLocalTitle.${kind}`)
                  : t("ai.approvalSingleTitle")}
          </span>
          {block.agentRole && (
            <span className="text-[10px] text-muted-foreground bg-muted rounded px-1 py-0.5">{block.agentRole}</span>
          )}
        </div>
        <span className="inline-flex items-center rounded-full bg-[#F59E0B20] h-5 px-2 text-[10px] font-semibold text-amber-500">
          {t("ai.approvalPending", "等待确认")}
        </span>
      </div>

      {/* Items */}
      <div className="space-y-2">
        {items.map((item, i) =>
          kind === "batch" ? (
            <div key={i} className="rounded-lg bg-[#1E1A0E] p-2.5 space-y-1.5">
              <div className="flex items-center gap-1.5">
                <TypeBadge type={item.type} compact />
                {item.asset_name && <span className="text-[11px] text-[#D4A94E]">{item.asset_name}</span>}
              </div>
              <div className="rounded bg-[#16120B] px-2 py-[5px]">
                <code className="block font-mono text-[10px] text-muted-foreground whitespace-pre-wrap break-all">
                  {item.command}
                </code>
              </div>
            </div>
          ) : (
            <div key={i} className="rounded-lg bg-[#1E1A0E] p-3 space-y-2">
              <div className="flex items-center gap-2">
                {kind === "grant" ? (
                  <ScopeBadge item={item} />
                ) : (
                  <>
                    <TypeBadge type={item.type} />
                    {item.asset_name && <span className="text-xs text-[#D4A94E]">{item.asset_name}</span>}
                  </>
                )}
              </div>
              {kind === "grant" ? (
                <Textarea
                  value={editedCommands[i] || ""}
                  onChange={(e) => setEditedCommands((prev) => ({ ...prev, [i]: e.target.value }))}
                  className="font-mono text-[11px] min-h-[32px] resize-y bg-background border-border"
                  rows={Math.max(1, (editedCommands[i] || "").split("\n").length)}
                />
              ) : (
                <div className="rounded-md bg-[#16120B] px-2.5 py-2">
                  <code className="block font-mono text-[11px] text-muted-foreground whitespace-pre-wrap break-all">
                    {item.command}
                  </code>
                </div>
              )}
            </div>
          )
        )}
      </div>

      {/* Reason (grant only, before buttons) */}
      {kind === "grant" && block.approvalDescription && (
        <div className="flex gap-1.5">
          <span className="text-[11px] font-medium text-[#D4A94E] shrink-0">{t("ai.approvalReasonLabel")}</span>
          <span className="text-[11px] text-muted-foreground">{block.approvalDescription}</span>
        </div>
      )}

      {/* Remember mode pattern editor (asset-bound single only).
          Local 工具粒度是"整个工具"，没有 pattern 可编辑，所以这块直接不渲染。 */}
      {kind === "single" && rememberMode && !localKind && (
        <div className="space-y-1.5 pt-0.5">
          <div className="text-[10px] text-muted-foreground">{t("opsctlApproval.patternLabel")}</div>
          {items.map((_item, i) => (
            <Input
              key={i}
              value={editedCommands[i] || ""}
              onChange={(e) => setEditedCommands((prev) => ({ ...prev, [i]: e.target.value }))}
              className="font-mono text-[11px] h-8 bg-background border-border"
              placeholder={t("opsctlApproval.patternPlaceholder")}
            />
          ))}
          <div className="text-[10px] text-muted-foreground/70">{t("opsctlApproval.patternHint")}</div>
        </div>
      )}

      {/* Action buttons */}
      <div className="flex justify-end gap-2 pt-1">
        {kind === "batch" ? (
          <>
            <Button
              size="sm"
              variant="outline"
              className="h-8 rounded-md px-4 text-xs border-[#F59E0B40] text-[#D4A94E] hover:bg-[#F59E0B10] hover:text-[#D4A94E]"
              onClick={() => respond("deny")}
            >
              {t("ai.approvalDenyAll")}
            </Button>
            <Button
              size="sm"
              className="h-8 rounded-md px-4 text-xs bg-amber-500 hover:bg-amber-600 text-[#1A1400] font-semibold"
              onClick={() => respond("allow")}
            >
              {t("ai.approvalAllowAll")}
            </Button>
          </>
        ) : kind === "grant" ? (
          <>
            <Button
              size="sm"
              variant="outline"
              className="h-8 rounded-md px-4 text-xs border-[#F59E0B40] text-[#D4A94E] hover:bg-[#F59E0B10] hover:text-[#D4A94E]"
              onClick={() => respond("deny")}
            >
              {t("ai.approvalDeny")}
            </Button>
            <Button
              size="sm"
              className="h-8 rounded-md px-4 text-xs bg-amber-500 hover:bg-amber-600 text-[#1A1400] font-semibold"
              onClick={() => respond("allow")}
            >
              {t("ai.approvalApprove")}
            </Button>
          </>
        ) : (
          <>
            <Button
              size="sm"
              variant="outline"
              className="h-8 rounded-md px-4 text-xs border-[#F59E0B40] text-[#D4A94E] hover:bg-[#F59E0B10] hover:text-[#D4A94E]"
              onClick={() => respond("deny")}
            >
              {t("ai.approvalDeny")}
            </Button>
            {/* bash 不支持 remember—— allowAll 永远不弹此按钮，避免给用户错觉以为可以"放过 bash"。 */}
            {allowRemember && localKind && (
              <Button
                size="sm"
                className="h-8 rounded-md px-4 text-xs bg-[#3D3520] text-[#D4A94E] hover:bg-[#4D4530]"
                onClick={() => respond("allowAll")}
              >
                {t("ai.approvalAllowToolForSession")}
              </Button>
            )}
            {allowRemember && !localKind && rememberMode && (
              <Button
                size="sm"
                className="h-8 rounded-md px-4 text-xs bg-[#3D3520] text-[#D4A94E] hover:bg-[#4D4530]"
                onClick={() => respond("allowAll")}
              >
                {t("ai.approvalRememberAndAllow")}
              </Button>
            )}
            {allowRemember && !localKind && !rememberMode && (
              <Button
                size="sm"
                className="h-8 rounded-md px-4 text-xs bg-[#3D3520] text-[#D4A94E] hover:bg-[#4D4530]"
                onClick={() => {
                  setRememberMode(true);
                }}
              >
                {t("opsctlApproval.remember")}
              </Button>
            )}
            <Button
              size="sm"
              className="h-8 rounded-md px-4 text-xs bg-amber-500 hover:bg-amber-600 text-[#1A1400] font-semibold"
              onClick={() => respond("allow")}
            >
              {rememberMode && !localKind ? t("ai.approvalOnlyOnce") : t("ai.approvalAllow")}
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

function TypeBadge({ type, compact }: { type: string; compact?: boolean }) {
  const icons: Record<string, typeof Terminal> = {
    exec: Terminal,
    sql: Database,
    redis: Server,
    mongo: Database,
    kafka: Database,
    grant: Globe,
    [KIND_LOCAL_BASH]: Terminal,
    [KIND_LOCAL_WRITE]: FilePlus,
    [KIND_LOCAL_EDIT]: FileEdit,
  };
  // local_* 在徽标上去掉前缀更直观（"WRITE" 比 "LOCAL_WRITE" 短而且对齐工具名）。
  const labels: Record<string, string> = {
    [KIND_LOCAL_BASH]: "BASH",
    [KIND_LOCAL_WRITE]: "WRITE",
    [KIND_LOCAL_EDIT]: "EDIT",
  };
  const Icon = icons[type] || Terminal;
  const label = labels[type] || type.toUpperCase();
  if (compact) {
    return (
      <span className="inline-flex items-center gap-[3px] rounded-[3px] border border-[#F59E0B30] h-[18px] px-[5px] text-[8px] font-bold text-[#D4A94E] bg-background">
        <Icon className="h-[11px] w-[11px]" />
        {label}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded border border-[#F59E0B30] h-5 px-1.5 text-[9px] font-bold text-[#D4A94E] bg-background">
      <Icon className="h-3 w-3" />
      {label}
    </span>
  );
}

function ScopeBadge({
  item,
}: {
  item: { asset_id: number; asset_name: string; group_id?: number; group_name?: string };
}) {
  const { t } = useTranslation();
  const cls =
    "inline-flex items-center gap-[3px] rounded-[3px] border border-[#F59E0B30] h-[18px] px-[5px] text-[8px] font-semibold text-[#D4A94E] bg-background";
  if (item.asset_id > 0) {
    return (
      <span className={cls}>
        <Server className="h-[11px] w-[11px]" />
        {item.asset_name}
      </span>
    );
  }
  if (item.group_id && item.group_id > 0) {
    return (
      <span className={cls}>
        <FolderOpen className="h-[11px] w-[11px]" />
        {item.group_name}
      </span>
    );
  }
  return (
    <span className={cls}>
      <Globe className="h-[11px] w-[11px]" />
      {t("opsctlApproval.scopeAll")}
    </span>
  );
}
