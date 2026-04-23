import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  Button,
  Dialog,
  DialogContent,
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
  Textarea,
} from "@opskat/ui";
import { useSnippetStore } from "@/stores/snippetStore";
import { useAssetStore } from "@/stores/assetStore";
import { snippet_entity } from "../../../wailsjs/go/models";

export interface SnippetFormDialogProps {
  open: boolean;
  mode: "create" | "edit";
  initial?: snippet_entity.Snippet;
  /** Optional pre-selected category for create mode (e.g. opened from drawer). */
  defaultCategory?: string;
  onOpenChange: (open: boolean) => void;
}

function normalizeTags(raw: string): string {
  return raw
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter((s) => s.length > 0)
    .join(",");
}

export function SnippetFormDialog({ open, mode, initial, defaultCategory, onOpenChange }: SnippetFormDialogProps) {
  const { t } = useTranslation();
  const categories = useSnippetStore((s) => s.categories);
  const list = useSnippetStore((s) => s.list);
  const createSnippet = useSnippetStore((s) => s.create);
  const updateSnippet = useSnippetStore((s) => s.update);
  const assets = useAssetStore((s) => s.assets);

  const [category, setCategory] = useState<string>("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  const [assetId, setAssetId] = useState<string>(""); // "" = global
  const [content, setContent] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  // Reset form whenever the dialog opens (or the initial/mode changes).
  useEffect(() => {
    if (!open) return;
    if (mode === "edit" && initial) {
      setCategory(initial.Category);
      setName(initial.Name);
      setDescription(initial.Description ?? "");
      setTags(initial.Tags ?? "");
      setAssetId(initial.AssetID != null ? String(initial.AssetID) : "");
      setContent(initial.Content ?? "");
    } else {
      setCategory(defaultCategory ?? categories[0]?.id ?? "");
      setName("");
      setDescription("");
      setTags("");
      setAssetId("");
      setContent("");
    }
    setNameTouched(false);
    setSubmitting(false);
  }, [open, mode, initial, defaultCategory, categories]);

  const selectedCategory = useMemo(() => categories.find((c) => c.id === category), [categories, category]);
  const assetType = selectedCategory?.assetType ?? "";
  const showAssetField = assetType !== "";

  const filteredAssets = useMemo(() => {
    if (!showAssetField) return [];
    return assets.filter((a) => a.Type === assetType);
  }, [assets, assetType, showAssetField]);

  // Duplicate-name hint (same category, same name, different id).
  const duplicateHint = useMemo(() => {
    if (!nameTouched || !name.trim() || !category) return false;
    return list.some(
      (s) =>
        s.Category === category &&
        s.Name.trim().toLowerCase() === name.trim().toLowerCase() &&
        (mode === "create" || s.ID !== initial?.ID)
    );
  }, [nameTouched, name, category, list, mode, initial?.ID]);

  const canSubmit = !!category && !!name.trim() && !!content.trim() && !submitting;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const normalizedTags = normalizeTags(tags);
      const parsedAssetId = assetId ? Number(assetId) : undefined;
      if (mode === "create") {
        await createSnippet({
          name: name.trim(),
          category,
          content,
          description,
          tags: normalizedTags,
          assetId: showAssetField ? parsedAssetId : undefined,
        } as unknown as import("../../../wailsjs/go/models").snippet_svc.CreateReq);
        toast.success(t("snippet.toast.created"));
      } else if (initial) {
        await updateSnippet({
          id: initial.ID,
          name: name.trim(),
          content,
          description,
          tags: normalizedTags,
          assetId: showAssetField ? parsedAssetId : undefined,
        } as unknown as import("../../../wailsjs/go/models").snippet_svc.UpdateReq);
        toast.success(t("snippet.toast.updated"));
      }
      onOpenChange(false);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(msg);
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? t("snippet.form.createTitle") : t("snippet.form.editTitle")}</DialogTitle>
        </DialogHeader>

        <div className="grid gap-4">
          {/* Category */}
          <div className="grid gap-1.5">
            <Label htmlFor="snippet-category">{t("snippet.form.labelCategory")}</Label>
            <Select value={category} onValueChange={setCategory} disabled={mode === "edit"}>
              <SelectTrigger id="snippet-category" className="w-full">
                <SelectValue placeholder={t("snippet.form.labelCategory")} />
              </SelectTrigger>
              <SelectContent>
                {categories.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Name */}
          <div className="grid gap-1.5">
            <Label htmlFor="snippet-name">{t("snippet.form.labelName")}</Label>
            <Input
              id="snippet-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              onBlur={() => setNameTouched(true)}
              placeholder=""
            />
            {duplicateHint && <p className="text-amber-500 text-xs">{t("snippet.form.duplicateNameHint")}</p>}
          </div>

          {/* Description */}
          <div className="grid gap-1.5">
            <Label htmlFor="snippet-desc">{t("snippet.form.labelDescription")}</Label>
            <Input id="snippet-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>

          {/* Tags */}
          <div className="grid gap-1.5">
            <Label htmlFor="snippet-tags">{t("snippet.form.labelTags")}</Label>
            <Input id="snippet-tags" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="foo,bar,baz" />
          </div>

          {/* Asset binding */}
          {showAssetField && (
            <div className="grid gap-1.5">
              <Label htmlFor="snippet-asset">{t("snippet.form.labelAsset")}</Label>
              <Select value={assetId || "__global__"} onValueChange={(v) => setAssetId(v === "__global__" ? "" : v)}>
                <SelectTrigger id="snippet-asset" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__global__">{t("snippet.globalBinding")}</SelectItem>
                  {filteredAssets.map((a) => (
                    <SelectItem key={a.ID} value={String(a.ID)}>
                      {a.Name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Content */}
          <div className="grid gap-1.5">
            <Label htmlFor="snippet-content">{t("snippet.form.labelContent")}</Label>
            <Textarea
              id="snippet-content"
              rows={12}
              className="font-mono text-xs min-h-56"
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            {t("snippet.actions.cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit}>
            {mode === "create" ? t("snippet.actions.create") : t("snippet.actions.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
