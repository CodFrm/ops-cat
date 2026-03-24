import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { RespondPlanApprovalWithEdits } from "../../../wailsjs/go/main/App";
import { ShieldAlert, Server, FolderOpen, Globe } from "lucide-react";

interface PlanItem {
  type: string;
  asset_id: number;
  asset_name: string;
  group_id: number;
  group_name: string;
  command: string;
  detail: string;
}

interface PlanApprovalEvent {
  session_id: string;
  description: string;
  items: PlanItem[];
}

// 目标分组键：按 (asset_id, group_id) 唯一分组
function targetKey(item: PlanItem): string {
  if (item.asset_id > 0) return `asset:${item.asset_id}`;
  if (item.group_id > 0) return `group:${item.group_id}`;
  return "all";
}

interface EditGroup {
  key: string;
  assetID: number;
  assetName: string;
  groupID: number;
  groupName: string;
  commands: string;
}

// 按目标分组 items，合并同一目标的命令为多行文本
function groupByTarget(items: PlanItem[]): EditGroup[] {
  const map = new Map<string, EditGroup>();
  for (const item of items) {
    const k = targetKey(item);
    const existing = map.get(k);
    if (existing) {
      if (item.command) {
        existing.commands = existing.commands
          ? existing.commands + "\n" + item.command
          : item.command;
      }
    } else {
      map.set(k, {
        key: k,
        assetID: item.asset_id,
        assetName: item.asset_name,
        groupID: item.group_id,
        groupName: item.group_name,
        commands: item.command || "",
      });
    }
  }
  return Array.from(map.values());
}

function ScopeBadge({ group }: { group: EditGroup }) {
  const { t } = useTranslation();
  const badgeClass = "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-normal text-muted-foreground bg-muted";

  if (group.assetID > 0) {
    return (
      <span className={badgeClass}>
        <Server className="h-3 w-3" />
        {t("planApproval.scopeAsset")}: {group.assetName}
      </span>
    );
  }
  if (group.groupID > 0) {
    return (
      <span className={badgeClass}>
        <FolderOpen className="h-3 w-3" />
        {t("planApproval.scopeGroup")}: {group.groupName}
      </span>
    );
  }
  return (
    <span className={badgeClass}>
      <Globe className="h-3 w-3" />
      {t("planApproval.scopeAll")}
    </span>
  );
}

export function PlanApprovalDialog() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [event, setEvent] = useState<PlanApprovalEvent | null>(null);
  const [editGroups, setEditGroups] = useState<EditGroup[]>([]);

  const handleEvent = useCallback((data: PlanApprovalEvent) => {
    setEvent(data);
    setEditGroups(groupByTarget(data.items));
    setOpen(true);
  }, []);

  useEffect(() => {
    EventsOn("opsctl:plan-approval", handleEvent);
    return () => { EventsOff("opsctl:plan-approval"); };
  }, [handleEvent]);

  const respond = useCallback((approved: boolean) => {
    if (event) {
      const editedItems = editGroups.map((g) => ({
        asset_id: g.assetID,
        asset_name: g.assetName,
        group_id: g.groupID,
        group_name: g.groupName,
        command: g.commands,
      }));
      RespondPlanApprovalWithEdits(event.session_id, approved, editedItems);
    }
    setOpen(false);
    setEvent(null);
    setEditGroups([]);
  }, [event, editGroups]);

  const updateGroupCommands = useCallback((index: number, value: string) => {
    setEditGroups((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], commands: value };
      return next;
    });
  }, []);

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) respond(false); }}>
      <DialogContent className="sm:max-w-lg max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert className="h-5 w-5 text-amber-500" />
            {t("planApproval.title")}
          </DialogTitle>
          <DialogDescription>
            {t("planApproval.description")}
          </DialogDescription>
        </DialogHeader>
        {event && (
          <div className="space-y-3 overflow-y-auto flex-1 min-h-0">
            {event.description && (
              <div className="text-sm font-medium">{event.description}</div>
            )}
            <div className="space-y-3">
              {editGroups.map((group, index) => (
                <div key={group.key} className="rounded-md border p-3 space-y-2">
                  <ScopeBadge group={group} />
                  <Textarea
                    value={group.commands}
                    onChange={(e) => updateGroupCommands(index, e.target.value)}
                    placeholder={t("planApproval.commandsPlaceholder")}
                    className="font-mono text-xs min-h-[60px] resize-y"
                    rows={Math.max(2, group.commands.split("\n").length)}
                  />
                </div>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {t("planApproval.editHint")}
            </p>
          </div>
        )}
        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => respond(false)}>
            {t("planApproval.deny")}
          </Button>
          <Button onClick={() => respond(true)}>
            {t("planApproval.approveAll")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
