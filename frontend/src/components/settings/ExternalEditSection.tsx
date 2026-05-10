import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
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

export function ExternalEditSection() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState<ExternalEditSettings | null>(null);
  const [defaultEditorId, setDefaultEditorId] = useState("");
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [customEditors, setCustomEditors] = useState<ExternalEditEditorConfig[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getExternalEditSettings()
      .then((data) => {
        setSettings(data);
        setDefaultEditorId(data.defaultEditorId);
        setWorkspaceRoot(data.workspaceRoot);
        setCustomEditors(normalizeEditors(data.customEditors || []));
      })
      .catch((error) => toast.error(String(error)));
  }, []);

  const updateCustomEditor = (index: number, patch: Partial<ExternalEditEditorConfig>) => {
    setCustomEditors((current) =>
      current.map((editor, editorIndex) => (editorIndex === index ? { ...editor, ...patch } : editor))
    );
  };

  const removeCustomEditor = (index: number) => {
    setCustomEditors((current) => current.filter((_, editorIndex) => editorIndex !== index));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      // 设置页只负责整理用户输入并把完整快照交给后端；
      // 默认编辑器可用性、工作区落盘和自定义编辑器合法性都以后端返回为准。
      const next = await saveExternalEditSettings({
        defaultEditorId,
        workspaceRoot,
        customEditors: normalizeEditors(customEditors),
      });
      setSettings(next);
      setDefaultEditorId(next.defaultEditorId);
      setWorkspaceRoot(next.workspaceRoot);
      setCustomEditors(normalizeEditors(next.customEditors || []));
      toast.success(t("externalEdit.settings.saved"));
    } catch (error) {
      toast.error(String(error));
    } finally {
      setSaving(false);
    }
  };

  return (
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
              {(settings?.editors || []).map((editor) => (
                <SelectItem key={editor.id} value={editor.id} disabled={!editor.available}>
                  {editor.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-1.5">
          <Label>{t("externalEdit.settings.workspaceRoot")}</Label>
          <div className="flex gap-2">
            <Input value={workspaceRoot} onChange={(event) => setWorkspaceRoot(event.target.value)} />
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

        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <Label>{t("externalEdit.settings.customEditors")}</Label>
            <Button
              variant="outline"
              size="sm"
              className="gap-1"
              onClick={() =>
                setCustomEditors((current) => [
                  ...current,
                  { id: `custom-${current.length + 1}`, name: "", path: "", args: [] },
                ])
              }
            >
              <Plus className="h-3.5 w-3.5" />
              {t("action.add")}
            </Button>
          </div>
          {customEditors.map((editor, index) => (
            <div key={editor.id} className="rounded border p-3 space-y-2">
              <div className="flex justify-end">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => removeCustomEditor(index)}
                  aria-label={t("action.delete")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
              <div className="grid gap-1.5">
                <Label>{t("asset.name")}</Label>
                <Input
                  value={editor.name}
                  onChange={(event) => updateCustomEditor(index, { name: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>{t("externalEdit.settings.editorPath")}</Label>
                <div className="flex gap-2">
                  <Input
                    value={editor.path}
                    onChange={(event) => updateCustomEditor(index, { path: event.target.value })}
                  />
                  <Button
                    variant="outline"
                    onClick={async () => {
                      const selected = await selectExternalEditorExecutable();
                      if (selected) updateCustomEditor(index, { path: selected });
                    }}
                  >
                    {t("action.browse")}
                  </Button>
                </div>
              </div>
              <div className="grid gap-1.5">
                <Label>{t("externalEdit.settings.editorArgs")}</Label>
                <Input
                  value={(editor.args || []).join(" ")}
                  onChange={(event) =>
                    updateCustomEditor(index, {
                      // 这里使用的“空格切分”约束：
                      args: event.target.value
                        .split(" ")
                        .map((item) => item.trim())
                        .filter(Boolean),
                    })
                  }
                />
              </div>
            </div>
          ))}
        </div>

        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="gap-1">
            <Save className="h-4 w-4" />
            {saving ? t("action.saving") : t("action.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
