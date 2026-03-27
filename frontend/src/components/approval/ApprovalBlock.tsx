import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Shield, Terminal, Database, Server, Globe, FolderOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { RespondAIApproval } from "../../../wailsjs/go/app/App";
import { ai } from "../../../wailsjs/go/models";
import type { ContentBlock } from "@/stores/aiStore";

interface ApprovalBlockProps {
  block: ContentBlock;
}

export function ApprovalBlock({ block }: ApprovalBlockProps) {
  const { t } = useTranslation();
  const isPending = block.status === "pending_confirm";
  const isDenied = block.status === "error";
  const items = block.approvalItems || [];
  const kind = block.approvalKind || "single";

  const [editedCommands, setEditedCommands] = useState<Record<number, string>>(() => {
    const map: Record<number, string> = {};
    items.forEach((item, i) => {
      map[i] = item.command;
    });
    return map;
  });

  // "记住"模式：展开模式编辑器
  const [rememberMode, setRememberMode] = useState(false);

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
    <div className="my-1.5 rounded-lg border border-amber-500/30 bg-amber-500/5 text-xs overflow-hidden">
      <div className="flex items-center gap-1.5 px-2.5 py-1.5">
        <Shield className="h-3.5 w-3.5 shrink-0 text-amber-500" />
        <span className="font-medium text-amber-600 dark:text-amber-400">
          {kind === "grant"
            ? t("ai.approvalGrantTitle")
            : kind === "batch"
              ? t("ai.approvalBatchTitle", { count: items.length })
              : t("ai.approvalSingleTitle")}
        </span>
        {block.agentRole && (
          <span className="text-[10px] text-muted-foreground bg-muted rounded px-1 py-0.5">
            {block.agentRole}
          </span>
        )}
        {!isPending && !isDenied && (
          <span className="ml-auto text-[10px] text-green-600">{t("ai.approvalApproved")}</span>
        )}
        {isDenied && (
          <span className="ml-auto text-[10px] text-red-500">{t("ai.approvalDenied")}</span>
        )}
      </div>

      <div className="border-t border-amber-500/20 px-2.5 py-1.5 space-y-1.5">
        {block.approvalDescription && (
          <div className="text-xs text-muted-foreground">{block.approvalDescription}</div>
        )}
        {items.map((item, i) => (
          <div key={i} className="rounded-md bg-muted/50 p-1.5 space-y-1">
            <div className="flex items-center gap-1.5">
              <TypeBadge type={item.type} />
              {item.asset_name && (
                <span className="text-muted-foreground">{item.asset_name}</span>
              )}
              {item.group_name && (
                <span className="inline-flex items-center gap-0.5 text-muted-foreground">
                  <FolderOpen className="h-3 w-3" /> {item.group_name}
                </span>
              )}
            </div>
            {kind === "grant" && isPending ? (
              <Textarea
                value={editedCommands[i] || ""}
                onChange={(e) =>
                  setEditedCommands((prev) => ({ ...prev, [i]: e.target.value }))
                }
                className="font-mono text-[11px] min-h-[32px] resize-y bg-background"
                rows={Math.max(1, (editedCommands[i] || "").split("\n").length)}
              />
            ) : (
              <code className="block font-mono text-[11px] text-muted-foreground whitespace-pre-wrap break-all">
                {item.command}
              </code>
            )}
          </div>
        ))}
        {/* 记住模式：展开模式编辑器 */}
        {kind === "single" && isPending && rememberMode && (
          <div className="space-y-1 pt-0.5">
            <div className="text-[10px] text-muted-foreground">{t("opsctlApproval.patternLabel")}</div>
            {items.map((_item, i) => (
              <Input
                key={i}
                value={editedCommands[i] || ""}
                onChange={(e) =>
                  setEditedCommands((prev) => ({ ...prev, [i]: e.target.value }))
                }
                className="font-mono text-[11px] h-7 bg-background"
                placeholder={t("opsctlApproval.patternPlaceholder")}
              />
            ))}
            <div className="text-[10px] text-muted-foreground/70">{t("opsctlApproval.patternHint")}</div>
          </div>
        )}
      </div>

      {isPending && (
        <div className="border-t border-amber-500/20 px-2.5 py-1.5 flex justify-end gap-1.5">
          <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={() => respond("deny")}>
            {t("ai.approvalDeny")}
          </Button>
          {kind === "single" && (
            rememberMode ? (
              <Button size="sm" variant="secondary" className="h-6 px-2 text-xs" onClick={() => respond("allowAll")}>
                {t("opsctlApproval.approve")}
              </Button>
            ) : (
              <Button size="sm" variant="secondary" className="h-6 px-2 text-xs" onClick={() => setRememberMode(true)}>
                {t("opsctlApproval.remember")}
              </Button>
            )
          )}
          <Button size="sm" className="h-6 px-2 text-xs" onClick={() => respond(kind === "grant" ? "allow" : "allow")}>
            {kind === "grant" ? t("ai.approvalApprove") : t("ai.approvalAllow")}
          </Button>
        </div>
      )}
    </div>
  );
}

function TypeBadge({ type }: { type: string }) {
  const icons: Record<string, typeof Terminal> = {
    exec: Terminal,
    sql: Database,
    redis: Server,
    grant: Globe,
  };
  const Icon = icons[type] || Terminal;
  return (
    <span className="inline-flex items-center gap-0.5 rounded border px-1 py-0.5 text-[10px] font-medium text-muted-foreground bg-background">
      <Icon className="h-3 w-3" />
      {type.toUpperCase()}
    </span>
  );
}
