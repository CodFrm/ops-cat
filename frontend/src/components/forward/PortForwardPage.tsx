import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Play, Square, Pencil, Trash2, CircleCheck, CircleAlert, CircleDot, CircleMinus } from "lucide-react";
import {
  Button,
  Input,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  ConfirmDialog,
} from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import {
  ListForwardConfigs,
  CreateForwardConfig,
  UpdateForwardConfig,
  DeleteForwardConfig,
  StartForwardConfig,
  StopForwardConfig,
} from "../../../wailsjs/go/app/App";
import { app, forward_entity } from "../../../wailsjs/go/models";

// 编辑中的规则（无 id）
interface EditRule {
  type: string;
  localHost: string;
  localPort: string;
  remoteHost: string;
  remotePort: string;
}

const emptyRule = (): EditRule => ({
  type: "local",
  localHost: "127.0.0.1",
  localPort: "",
  remoteHost: "127.0.0.1",
  remotePort: "",
});

export function PortForwardPage() {
  const { t } = useTranslation();
  const [configs, setConfigs] = useState<app.ForwardConfigWithStatus[]>([]);
  const [loading, setLoading] = useState(false);

  // 编辑弹窗
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editId, setEditId] = useState<number | null>(null); // null = 新建
  const [editName, setEditName] = useState("");
  const [editAssetId, setEditAssetId] = useState(0);
  const [editRules, setEditRules] = useState<EditRule[]>([emptyRule()]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const list = await ListForwardConfigs();
      setConfigs(list || []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const openCreate = () => {
    setEditId(null);
    setEditName("");
    setEditAssetId(0);
    setEditRules([emptyRule()]);
    setDialogOpen(true);
  };

  const openEdit = (cfg: app.ForwardConfigWithStatus) => {
    setEditId(cfg.id);
    setEditName(cfg.name);
    setEditAssetId(cfg.assetId);
    setEditRules(
      cfg.rules.length > 0
        ? cfg.rules.map((r) => ({
            type: r.type,
            localHost: r.localHost,
            localPort: String(r.localPort),
            remoteHost: r.remoteHost,
            remotePort: String(r.remotePort),
          }))
        : [emptyRule()]
    );
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!editName || !editAssetId) return;
    const rules = editRules
      .filter((r) => r.localPort && r.remotePort)
      .map(
        (r) =>
          new forward_entity.ForwardRule({
            type: r.type,
            localHost: r.localHost,
            localPort: parseInt(r.localPort, 10),
            remoteHost: r.remoteHost,
            remotePort: parseInt(r.remotePort, 10),
          })
      );
    if (editId) {
      await UpdateForwardConfig(editId, editName, editAssetId, rules);
    } else {
      await CreateForwardConfig(editName, editAssetId, rules);
    }
    setDialogOpen(false);
    refresh();
  };

  const [deleteTarget, setDeleteTarget] = useState<{ id: number; name: string } | null>(null);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    await DeleteForwardConfig(deleteTarget.id);
    setDeleteTarget(null);
    refresh();
  };

  const handleStart = async (id: number) => {
    await StartForwardConfig(id);
    refresh();
  };

  const handleStop = async (id: number) => {
    await StopForwardConfig(id);
    refresh();
  };

  const handleAssetChange = async (cfg: app.ForwardConfigWithStatus, assetId: number) => {
    const wasRunning = cfg.status !== "stopped";
    const rules = cfg.rules.map(
      (r) =>
        new forward_entity.ForwardRule({
          type: r.type,
          localHost: r.localHost,
          localPort: r.localPort,
          remoteHost: r.remoteHost,
          remotePort: r.remotePort,
        })
    );
    await UpdateForwardConfig(cfg.id, cfg.name, assetId, rules);
    if (wasRunning) {
      await StartForwardConfig(cfg.id);
    }
    refresh();
  };

  const updateRule = (idx: number, field: keyof EditRule, value: string) => {
    setEditRules((prev) => prev.map((r, i) => (i === idx ? { ...r, [field]: value } : r)));
  };

  const statusIcon = (status: string) => {
    switch (status) {
      case "running":
        return <CircleCheck className="h-4 w-4 text-green-500" />;
      case "partial":
        return <CircleDot className="h-4 w-4 text-yellow-500" />;
      case "error":
        return <CircleAlert className="h-4 w-4 text-destructive" />;
      default:
        return <CircleMinus className="h-4 w-4 text-muted-foreground" />;
    }
  };

  const statusLabel = (status: string) => {
    switch (status) {
      case "running":
        return t("forward.running");
      case "partial":
        return t("forward.partial");
      case "error":
        return t("forward.error");
      default:
        return t("forward.stopped");
    }
  };

  return (
    <div className="absolute inset-0 bg-background flex flex-col">
      <div className="px-4 py-3 border-b flex items-center justify-between">
        <h2 className="font-semibold">{t("nav.forward")}</h2>
        <Button size="sm" className="gap-1" onClick={openCreate}>
          <Plus className="h-3.5 w-3.5" />
          {t("forward.create")}
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {configs.length === 0 && !loading && (
          <div className="text-center text-muted-foreground py-12">{t("forward.empty")}</div>
        )}

        <div className="grid gap-4 max-w-3xl mx-auto">
          {configs.map((cfg) => (
            <div key={cfg.id} className="border rounded-lg p-4">
              {/* 卡片头部 */}
              <div className="flex items-center gap-3 mb-3">
                <span className="font-medium text-sm flex-1">{cfg.name}</span>

                {/* 资产选择（直接在卡片上切换） */}
                <AssetSelect
                  value={cfg.assetId}
                  onValueChange={(v) => handleAssetChange(cfg, v)}
                  filterType="ssh"
                  className="h-7 w-44 text-xs"
                />

                {/* 状态 */}
                <div className="flex items-center gap-1 text-xs min-w-20">
                  {statusIcon(cfg.status)}
                  <span>{statusLabel(cfg.status)}</span>
                </div>

                {/* 操作按钮 */}
                {cfg.status === "stopped" ? (
                  <Button variant="ghost" size="icon-xs" onClick={() => handleStart(cfg.id)} title={t("forward.start")}>
                    <Play className="h-3.5 w-3.5" />
                  </Button>
                ) : (
                  <Button variant="ghost" size="icon-xs" onClick={() => handleStop(cfg.id)} title={t("forward.stop")}>
                    <Square className="h-3.5 w-3.5" />
                  </Button>
                )}
                <Button variant="ghost" size="icon-xs" onClick={() => openEdit(cfg)} title={t("forward.edit")}>
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => setDeleteTarget({ id: cfg.id, name: cfg.name })}
                  title={t("forward.delete")}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>

              {/* 规则列表 */}
              <div className="space-y-1">
                {cfg.rules.map((rule) => {
                  const prefix = rule.type === "remote" ? "R" : rule.type === "dynamic" ? "D" : "L";
                  const label =
                    rule.type === "dynamic"
                      ? `${prefix}  ${rule.localHost}:${rule.localPort} (SOCKS5)`
                      : `${prefix}  ${rule.localHost}:${rule.localPort} \u2192 ${rule.remoteHost}:${rule.remotePort}`;
                  return (
                    <div key={rule.id} className="flex items-center gap-2 text-xs font-mono text-muted-foreground">
                      {rule.status === "running" ? (
                        <CircleCheck className="h-3 w-3 text-green-500 shrink-0" />
                      ) : rule.status === "error" ? (
                        <span title={rule.error} className="cursor-help shrink-0">
                          <CircleAlert className="h-3 w-3 text-destructive" />
                        </span>
                      ) : (
                        <CircleMinus className="h-3 w-3 text-muted-foreground/40 shrink-0" />
                      )}
                      <span>{label}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 新建/编辑弹窗 */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editId ? t("forward.edit") : t("forward.create")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>{t("forward.name")}</Label>
              <Input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                placeholder={t("forward.namePlaceholder")}
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t("forward.asset")}</Label>
              <AssetSelect
                value={editAssetId}
                onValueChange={setEditAssetId}
                filterType="ssh"
                placeholder={t("forward.selectAsset")}
              />
            </div>
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label>{t("forward.rules")}</Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 gap-1 text-xs"
                  onClick={() => setEditRules([...editRules, emptyRule()])}
                >
                  <Plus className="h-3 w-3" />
                  {t("forward.addRule")}
                </Button>
              </div>
              {editRules.map((rule, i) => (
                <div key={i} className="flex items-center gap-1.5 text-xs">
                  <Select value={rule.type} onValueChange={(v) => updateRule(i, "type", v)}>
                    <SelectTrigger className="h-7 w-16 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="local">L</SelectItem>
                      <SelectItem value="remote">R</SelectItem>
                      <SelectItem value="dynamic">D</SelectItem>
                    </SelectContent>
                  </Select>
                  <Input
                    className="h-7 text-xs w-24"
                    value={rule.localHost}
                    onChange={(e) => updateRule(i, "localHost", e.target.value)}
                    placeholder="127.0.0.1"
                  />
                  <span>:</span>
                  <Input
                    className="h-7 text-xs w-16"
                    value={rule.localPort}
                    onChange={(e) => updateRule(i, "localPort", e.target.value)}
                    placeholder={t("forward.port")}
                  />
                  {rule.type !== "dynamic" && (
                    <>
                      <span className="text-muted-foreground">&rarr;</span>
                      <Input
                        className="h-7 text-xs w-24"
                        value={rule.remoteHost}
                        onChange={(e) => updateRule(i, "remoteHost", e.target.value)}
                        placeholder="127.0.0.1"
                      />
                      <span>:</span>
                      <Input
                        className="h-7 text-xs w-16"
                        value={rule.remotePort}
                        onChange={(e) => updateRule(i, "remotePort", e.target.value)}
                        placeholder={t("forward.port")}
                      />
                    </>
                  )}
                  {rule.type === "dynamic" && <span className="text-muted-foreground ml-1">SOCKS5</span>}
                  {editRules.length > 1 && (
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => setEditRules(editRules.filter((_, j) => j !== i))}
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              {t("forward.cancel")}
            </Button>
            <Button onClick={handleSave} disabled={!editName || !editAssetId}>
              {t("forward.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("forward.deleteConfirmTitle")}
        description={t("forward.deleteConfirmDesc", { name: deleteTarget?.name ?? "" })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={handleDelete}
      />
    </div>
  );
}
