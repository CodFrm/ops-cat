import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Button,
  Input,
  Textarea,
} from "@opskat/ui";
import { useWailsEvent } from "@/hooks/useWailsEvent";
import { RespondOpsctlApproval } from "../../../wailsjs/go/app/App";
import { ai } from "../../../wailsjs/go/models";
import { ShieldAlert, Terminal, Database, Server, FolderOpen, Globe, Usb } from "lucide-react";

interface ApprovalItemData {
  type: string;
  asset_id: number;
  asset_name: string;
  group_id?: number;
  group_name?: string;
  command: string;
  detail?: string;
}

interface SingleApprovalEvent {
  confirm_id: string;
  type: string;
  asset_id: number;
  asset_name: string;
  command?: string;
  detail?: string;
  session_id: string;
}

interface BatchApprovalEvent {
  confirm_id: string;
  items: ApprovalItemData[];
  session_id: string;
}

interface GrantApprovalEvent {
  items: ApprovalItemData[];
  session_id: string;
  description?: string;
}

interface QueueItem {
  id: string;
  kind: "single" | "batch" | "grant";
  items: ApprovalItemData[];
  description?: string;
  sessionID?: string;
  editable: boolean;
}

function TypeBadge({ type }: { type: string }) {
  const icons: Record<string, typeof Terminal> = {
    exec: Terminal,
    serial: Usb,
    sql: Database,
    redis: Server,
    mongo: Database,
    kafka: Database,
  };
  const Icon = icons[type] || Terminal;
  return (
    <span className="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground bg-muted">
      <Icon className="h-3 w-3" />
      {type.toUpperCase()}
    </span>
  );
}

function ScopeBadge({ item }: { item: ApprovalItemData }) {
  const { t } = useTranslation();
  const cls =
    "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground bg-muted";
  if (item.asset_id > 0) {
    return (
      <span className={cls}>
        <Server className="h-3 w-3" />
        {item.asset_name}
      </span>
    );
  }
  if (item.group_id && item.group_id > 0) {
    return (
      <span className={cls}>
        <FolderOpen className="h-3 w-3" />
        {item.group_name}
      </span>
    );
  }
  return (
    <span className={cls}>
      <Globe className="h-3 w-3" />
      {t("opsctlApproval.scopeAll")}
    </span>
  );
}

export function OpsctlApprovalDialog() {
  const { t } = useTranslation();
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [editState, setEditState] = useState<Record<string, Record<number, string>>>({});
  const [rememberMode, setRememberMode] = useState(false);

  const enqueue = useCallback((item: QueueItem) => {
    setQueue((prev) => (prev.some((q) => q.id === item.id) ? prev : [...prev, item]));
  }, []);

  useWailsEvent(
    "opsctl:approval",
    useCallback(
      (data: SingleApprovalEvent) => {
        enqueue({
          id: data.confirm_id,
          kind: "single",
          items: [
            {
              type: data.type,
              asset_id: data.asset_id,
              asset_name: data.asset_name,
              command: data.command || "",
              detail: data.detail,
            },
          ],
          sessionID: data.session_id,
          editable: false,
        });
      },
      [enqueue]
    )
  );

  useWailsEvent(
    "opsctl:batch-approval",
    useCallback(
      (data: BatchApprovalEvent) => {
        enqueue({
          id: data.confirm_id,
          kind: "batch",
          items: (data.items || []).map((i) => ({
            type: i.type,
            asset_id: i.asset_id,
            asset_name: i.asset_name,
            command: i.command,
          })),
          sessionID: data.session_id,
          editable: false,
        });
      },
      [enqueue]
    )
  );

  useWailsEvent(
    "opsctl:grant-approval",
    useCallback(
      (data: GrantApprovalEvent) => {
        const items: ApprovalItemData[] = (data.items || []).map((i) => ({
          type: i.type,
          asset_id: i.asset_id,
          asset_name: i.asset_name,
          group_id: i.group_id,
          group_name: i.group_name,
          command: i.command,
          detail: i.detail,
        }));
        enqueue({
          id: data.session_id,
          kind: "grant",
          items,
          description: data.description,
          sessionID: data.session_id,
          editable: true,
        });
        const edits: Record<number, string> = {};
        items.forEach((it, idx) => {
          edits[idx] = it.command;
        });
        setEditState((prev) => ({ ...prev, [data.session_id]: edits }));
      },
      [enqueue]
    )
  );

  const current = queue[0] || null;
  const open = !!current;

  const respond = useCallback(
    (decision: string) => {
      if (!current) return;

      const resp = new ai.ApprovalResponse();
      resp.decision = decision;

      const shouldSendEdits =
        (current.kind === "grant" && decision !== "deny") || (current.kind === "single" && decision === "allowAll");

      if (shouldSendEdits) {
        const edits = editState[current.id] || {};
        resp.edited_items = current.items.map((item, i) => {
          const edited = new ai.ApprovalItem();
          edited.type = item.type;
          edited.asset_id = item.asset_id;
          edited.asset_name = item.asset_name;
          edited.group_id = item.group_id || 0;
          edited.group_name = item.group_name || "";
          edited.command = edits[i] ?? item.command;
          edited.detail = item.detail || "";
          return edited;
        });
      }

      RespondOpsctlApproval(current.id, resp);
      setQueue((prev) => prev.slice(1));
      setRememberMode(false);
      setEditState((prev) => {
        const next = { ...prev };
        delete next[current.id];
        return next;
      });
    },
    [current, editState]
  );

  const handleOpenChange = useCallback(
    (v: boolean) => {
      if (!v) respond("deny");
    },
    [respond]
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-lg max-h-[80vh] flex flex-col"
        showCloseButton={false}
        onInteractOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        {current && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <ShieldAlert className="h-5 w-5 text-amber-500" />
                {current.kind === "grant"
                  ? t("opsctlApproval.grantTitle")
                  : current.kind === "batch"
                    ? t("opsctlApproval.batchTitle")
                    : t("opsctlApproval.title")}
                {queue.length > 1 && (
                  <span className="text-sm font-normal text-muted-foreground">(1/{queue.length})</span>
                )}
              </DialogTitle>
              <DialogDescription>
                {current.kind === "grant"
                  ? t("opsctlApproval.grantDescription")
                  : current.kind === "batch"
                    ? t("opsctlApproval.batchDescription", { count: current.items.length })
                    : t("opsctlApproval.description")}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-2 overflow-y-auto flex-1 min-h-0">
              {current.description && <div className="text-sm font-medium">{current.description}</div>}
              {current.items.map((item, i) => (
                <div key={i} className="rounded-md border p-2 space-y-1.5">
                  <div className="flex items-center gap-2">
                    {current.kind === "grant" ? (
                      <ScopeBadge item={item} />
                    ) : (
                      <>
                        <TypeBadge type={item.type} />
                        {item.asset_name && (
                          <span className="text-sm text-muted-foreground">
                            {item.asset_name}
                            {item.asset_id > 0 && ` (ID: ${item.asset_id})`}
                          </span>
                        )}
                      </>
                    )}
                  </div>
                  {current.editable ? (
                    <Textarea
                      value={editState[current.id]?.[i] ?? item.command}
                      onChange={(e) =>
                        setEditState((prev) => ({
                          ...prev,
                          [current.id]: { ...prev[current.id], [i]: e.target.value },
                        }))
                      }
                      className="font-mono text-xs min-h-[40px] resize-y"
                      rows={Math.max(2, (editState[current.id]?.[i] ?? item.command).split("\n").length)}
                    />
                  ) : (
                    <div className="rounded-md bg-muted p-2 max-h-[150px] overflow-auto">
                      <code className="text-xs font-mono whitespace-pre-wrap break-all">{item.command}</code>
                    </div>
                  )}
                  {item.detail && <div className="text-xs text-muted-foreground font-mono">{item.detail}</div>}
                </div>
              ))}
              {/* 记住模式：展开模式编辑器 */}
              {current.kind === "single" && current.sessionID && rememberMode && (
                <div className="space-y-1.5 pt-1">
                  <div className="text-xs text-muted-foreground">{t("opsctlApproval.patternLabel")}</div>
                  {current.items.map((_item, i) => (
                    <Input
                      key={i}
                      value={editState[current.id]?.[i] ?? _item.command}
                      onChange={(e) =>
                        setEditState((prev) => ({
                          ...prev,
                          [current.id]: { ...prev[current.id], [i]: e.target.value },
                        }))
                      }
                      className="font-mono text-xs"
                      placeholder={t("opsctlApproval.patternPlaceholder")}
                    />
                  ))}
                  <div className="text-[10px] text-muted-foreground/70">{t("opsctlApproval.patternHint")}</div>
                </div>
              )}
            </div>

            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => respond("deny")}>
                {t("opsctlApproval.deny")}
              </Button>
              {current.kind === "single" &&
                current.sessionID &&
                (rememberMode ? (
                  <Button variant="secondary" onClick={() => respond("allowAll")}>
                    {t("opsctlApproval.approve")}
                  </Button>
                ) : (
                  <Button
                    variant="secondary"
                    onClick={() => {
                      // 初始化 editState 用当前命令预填充
                      const edits: Record<number, string> = {};
                      current.items.forEach((it, idx) => {
                        edits[idx] = it.command;
                      });
                      setEditState((prev) => ({ ...prev, [current.id]: { ...prev[current.id], ...edits } }));
                      setRememberMode(true);
                    }}
                  >
                    {t("opsctlApproval.remember")}
                  </Button>
                ))}
              <Button onClick={() => respond("allow")}>
                {current.kind === "grant" ? t("opsctlApproval.approve") : t("opsctlApproval.allow")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
