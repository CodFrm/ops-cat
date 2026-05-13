import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { PencilLine, Plus, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  type ExternalEditEditor,
  type ExternalEditEditorConfig,
  type ExternalEditSettings,
  getExternalEditSettings,
  saveExternalEditSettings,
  selectExternalEditorExecutable,
  selectExternalEditWorkspaceRoot,
} from "@/lib/externalEditApi";

function normalizeEditors(editors: ExternalEditEditorConfig[]) {
  // 设置页允许新增空白行，真正持久化前再补齐稳定 id 和 args 数组，
  // 这样可以把“表单暂态”与“写入配置的最终结构”分开，避免保存时出现空值分支。
  return editors.map((editor, index) => ({
    ...editor,
    id: editor.id || `custom-${index + 1}`,
    args: editor.args || [],
  }));
}

function buildNextCustomEditorID(editors: ExternalEditEditorConfig[]) {
  const used = new Set(normalizeEditors(editors).map((editor) => editor.id));
  let index = editors.length + 1;
  while (used.has(`custom-${index}`)) {
    index += 1;
  }
  return `custom-${index}`;
}

function buildEditorOptions(
  savedEditors: ExternalEditEditor[],
  customEditors: ExternalEditEditorConfig[],
  defaultEditorId: string
): ExternalEditEditor[] {
  const customByID = new Map(normalizeEditors(customEditors).map((editor) => [editor.id, editor]));
  const savedByID = new Map(savedEditors.map((editor) => [editor.id, editor]));
  const builtIns = savedEditors.filter((editor) => editor.builtIn);
  const custom = Array.from(customByID.values()).map((editor) => {
    const saved = savedByID.get(editor.id);
    return {
      id: editor.id,
      name: editor.name || saved?.name || editor.id,
      path: editor.path,
      args: editor.args || [],
      builtIn: false,
      available: saved?.available ?? Boolean(editor.name.trim() && editor.path.trim()),
      default: editor.id === defaultEditorId,
    };
  });
  return [...builtIns, ...custom].map((editor) => ({
    ...editor,
    default: editor.id === defaultEditorId,
  }));
}

function pickFallbackDefaultEditorId(savedEditors: ExternalEditEditor[], customEditors: ExternalEditEditorConfig[]) {
  const options = buildEditorOptions(savedEditors, customEditors, "");
  return options.find((editor) => editor.available)?.id || options[0]?.id || "";
}

function formatEditorArgs(args?: string[]) {
  return (args || []).join(" ");
}

function parseEditorArgs(value: string) {
  return value
    .split(" ")
    .map((item) => item.trim())
    .filter(Boolean);
}

type EditorDialogState = {
  mode: "create" | "edit";
  index?: number;
  draft: ExternalEditEditorConfig;
} | null;

export function ExternalEditSection() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState<ExternalEditSettings | null>(null);
  const [defaultEditorId, setDefaultEditorId] = useState("");
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [cleanupRetentionDays, setCleanupRetentionDays] = useState("7");
  const [customEditors, setCustomEditors] = useState<ExternalEditEditorConfig[]>([]);
  const [editorDialog, setEditorDialog] = useState<EditorDialogState>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getExternalEditSettings()
      .then((data) => {
        setSettings(data);
        setDefaultEditorId(data.defaultEditorId);
        setWorkspaceRoot(data.workspaceRoot);
        setCleanupRetentionDays(String(data.cleanupRetentionDays || 7));
        setCustomEditors(normalizeEditors(data.customEditors || []));
      })
      .catch((error) => toast.error(String(error)));
  }, []);

  const editorOptions = useMemo(
    () => buildEditorOptions(settings?.editors || [], customEditors, defaultEditorId),
    [settings?.editors, customEditors, defaultEditorId]
  );

  useEffect(() => {
    if (!settings) return;
    if (editorOptions.some((editor) => editor.id === defaultEditorId)) return;
    const fallback = pickFallbackDefaultEditorId(settings.editors || [], customEditors);
    if (fallback !== defaultEditorId) {
      setDefaultEditorId(fallback);
    }
  }, [customEditors, defaultEditorId, editorOptions, settings]);

  const removeCustomEditor = (index: number) => {
    const removed = customEditors[index];
    const nextEditors = customEditors.filter((_, editorIndex) => editorIndex !== index);
    setCustomEditors(nextEditors);
    if (editorDialog?.mode === "edit" && editorDialog.index === index) {
      setEditorDialog(null);
    }
    if (removed?.id && removed.id === defaultEditorId) {
      setDefaultEditorId(pickFallbackDefaultEditorId(settings?.editors || [], nextEditors));
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      // 设置页只负责整理用户输入并把完整快照交给后端；
      // 默认编辑器可用性、工作区落盘和自定义编辑器合法性都以后端返回为准。
      const next = await saveExternalEditSettings({
        defaultEditorId,
        workspaceRoot,
        cleanupRetentionDays: Number.parseInt(cleanupRetentionDays, 10) || 7,
        customEditors: normalizeEditors(customEditors),
      });
      setSettings(next);
      setDefaultEditorId(next.defaultEditorId);
      setWorkspaceRoot(next.workspaceRoot);
      setCleanupRetentionDays(String(next.cleanupRetentionDays || 7));
      setCustomEditors(normalizeEditors(next.customEditors || []));
      toast.success(t("externalEdit.settings.saved"));
    } catch (error) {
      toast.error(String(error));
    } finally {
      setSaving(false);
    }
  };

  const openCreateEditor = () => {
    setEditorDialog({
      mode: "create",
      draft: {
        id: buildNextCustomEditorID(customEditors),
        name: "",
        path: "",
        args: [],
      },
    });
  };

  const openEditEditor = (index: number) => {
    const editor = customEditors[index];
    if (!editor) return;
    setEditorDialog({
      mode: "edit",
      index,
      draft: {
        ...editor,
        args: editor.args || [],
      },
    });
  };

  const updateEditorDraft = (patch: Partial<ExternalEditEditorConfig>) => {
    setEditorDialog((current) => (current ? { ...current, draft: { ...current.draft, ...patch } } : current));
  };

  const commitEditorDialog = () => {
    if (!editorDialog) return;
    const nextEditor = normalizeEditors([editorDialog.draft])[0];
    if (editorDialog.mode === "edit" && editorDialog.index !== undefined) {
      setCustomEditors((current) =>
        current.map((editor, editorIndex) => (editorIndex === editorDialog.index ? nextEditor : editor))
      );
    } else {
      setCustomEditors((current) => [...current, nextEditor]);
      if (!defaultEditorId) {
        setDefaultEditorId(nextEditor.id);
      }
    }
    setEditorDialog(null);
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-1.5">
            <PencilLine className="h-4 w-4" />
            {t("externalEdit.settings.title")}
          </CardTitle>
          <CardDescription>{t("externalEdit.settings.desc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-1.5">
            <Label>{t("externalEdit.settings.defaultEditor")}</Label>
            <Select value={defaultEditorId} onValueChange={setDefaultEditorId}>
              <SelectTrigger>
                <SelectValue placeholder={t("externalEdit.settings.defaultEditor")} />
              </SelectTrigger>
              <SelectContent>
                {editorOptions.map((editor) => (
                  <SelectItem key={editor.id} value={editor.id} disabled={!editor.available}>
                    {editor.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="external-edit-workspace-root">{t("externalEdit.settings.workspaceRoot")}</Label>
            <div className="flex gap-2">
              <Input
                id="external-edit-workspace-root"
                value={workspaceRoot}
                onChange={(event) => setWorkspaceRoot(event.target.value)}
              />
              <Button
                variant="outline"
                onClick={async () => {
                  const selected = await selectExternalEditWorkspaceRoot();
                  if (selected) setWorkspaceRoot(selected);
                }}
              >
                {t("action.browse")}
              </Button>
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="external-edit-cleanup-retention-days">
              {t("externalEdit.settings.cleanupRetentionDays")}
            </Label>
            <Input
              id="external-edit-cleanup-retention-days"
              type="number"
              min={1}
              max={365}
              value={cleanupRetentionDays}
              onChange={(event) => setCleanupRetentionDays(event.target.value)}
            />
            <div className="text-xs text-muted-foreground">{t("externalEdit.settings.cleanupRetentionDaysHint")}</div>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>{t("externalEdit.settings.customEditors")}</Label>
              <Button variant="outline" size="sm" className="gap-1" onClick={openCreateEditor}>
                <Plus className="h-3.5 w-3.5" />
                {t("externalEdit.settings.addEditor")}
              </Button>
            </div>
            {customEditors.length === 0 && (
              <div className="rounded border border-dashed px-3 py-4 text-sm text-muted-foreground">
                {t("externalEdit.settings.emptyCustomEditors")}
              </div>
            )}
            {customEditors.map((editor, index) => {
              const option = editorOptions.find((candidate) => candidate.id === editor.id);
              return (
                <div key={editor.id} className="rounded border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="truncate font-medium">{editor.name || editor.id}</div>
                        {defaultEditorId === editor.id && (
                          <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                            {t("externalEdit.settings.defaultBadge")}
                          </span>
                        )}
                      </div>
                      <div className="truncate text-sm text-muted-foreground">{editor.path}</div>
                      <div className="truncate text-xs text-muted-foreground">
                        {formatEditorArgs(editor.args) || t("externalEdit.settings.noArgs")}
                        {option && !option.available ? ` · ${t("externalEdit.settings.editorUnavailable")}` : ""}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => openEditEditor(index)}
                        aria-label={t("action.edit")}
                      >
                        {t("action.edit")}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => removeCustomEditor(index)}
                        aria-label={t("action.delete")}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>

          <div className="flex justify-end">
            <Button onClick={handleSave} disabled={saving} className="gap-1">
              <Save className="h-4 w-4" />
              {saving ? t("action.saving") : t("action.save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={!!editorDialog} onOpenChange={(open) => !open && setEditorDialog(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editorDialog?.mode === "edit"
                ? t("externalEdit.settings.editEditorTitle")
                : t("externalEdit.settings.addEditorTitle")}
            </DialogTitle>
            <DialogDescription>{t("externalEdit.settings.editorDialogDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid gap-1.5">
              <Label htmlFor="external-edit-editor-name">{t("asset.name")}</Label>
              <Input
                id="external-edit-editor-name"
                value={editorDialog?.draft.name || ""}
                onChange={(event) => updateEditorDraft({ name: event.target.value })}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="external-edit-editor-path">{t("externalEdit.settings.editorPath")}</Label>
              <div className="flex gap-2">
                <Input
                  id="external-edit-editor-path"
                  value={editorDialog?.draft.path || ""}
                  onChange={(event) => updateEditorDraft({ path: event.target.value })}
                />
                <Button
                  variant="outline"
                  onClick={async () => {
                    const selected = await selectExternalEditorExecutable();
                    if (selected) updateEditorDraft({ path: selected });
                  }}
                >
                  {t("action.browse")}
                </Button>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="external-edit-editor-args">{t("externalEdit.settings.editorArgs")}</Label>
              <Input
                id="external-edit-editor-args"
                value={formatEditorArgs(editorDialog?.draft.args)}
                onChange={(event) => updateEditorDraft({ args: parseEditorArgs(event.target.value) })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditorDialog(null)}>
              {t("action.cancel")}
            </Button>
            <Button
              onClick={commitEditorDialog}
              disabled={!editorDialog?.draft.name?.trim() || !editorDialog?.draft.path?.trim()}
            >
              {editorDialog?.mode === "edit" ? t("action.save") : t("action.add")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
