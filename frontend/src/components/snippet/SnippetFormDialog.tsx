import { useMemo, useState } from "react";
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
} from "@opskat/ui";
import { useSnippetStore } from "@/stores/snippetStore";
import { CodeEditor, type CodeEditorLanguage } from "@/components/CodeEditor";
import { snippet_entity, snippet_svc } from "../../../wailsjs/go/models";

export interface SnippetFormDialogProps {
  open: boolean;
  mode: "create" | "edit";
  initial?: snippet_entity.Snippet;
  /** Optional pre-selected category for create mode (e.g. opened from drawer). */
  defaultCategory?: string;
  onOpenChange: (open: boolean) => void;
}

export function SnippetFormDialog({ open, mode, initial, defaultCategory, onOpenChange }: SnippetFormDialogProps) {
  const categories = useSnippetStore((s) => s.categories);
  const formKey = `${mode}:${initial?.ID ?? "new"}:${defaultCategory ?? categories[0]?.id ?? ""}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open && (
        <SnippetFormDialogContent
          key={formKey}
          mode={mode}
          initial={initial}
          defaultCategory={defaultCategory}
          onOpenChange={onOpenChange}
          categories={categories}
        />
      )}
    </Dialog>
  );
}

function SnippetFormDialogContent({
  mode,
  initial,
  defaultCategory,
  onOpenChange,
  categories,
}: Omit<SnippetFormDialogProps, "open"> & { categories: snippet_svc.Category[] }) {
  const { t } = useTranslation();
  const list = useSnippetStore((s) => s.list);
  const createSnippet = useSnippetStore((s) => s.create);
  const updateSnippet = useSnippetStore((s) => s.update);

  const [category, setCategory] = useState<string>(
    mode === "edit" && initial ? initial.Category : (defaultCategory ?? categories[0]?.id ?? "")
  );
  const [name, setName] = useState(mode === "edit" && initial ? initial.Name : "");
  const [description, setDescription] = useState(mode === "edit" && initial ? (initial.Description ?? "") : "");
  const [content, setContent] = useState(mode === "edit" && initial ? (initial.Content ?? "") : "");
  const [nameTouched, setNameTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const contentLanguage: CodeEditorLanguage = useMemo(() => {
    switch (category) {
      case "shell":
        return "shell";
      case "sql":
        return "sql";
      case "redis":
        return "plaintext";
      case "mongo":
        return "javascript";
      case "prompt":
        return "markdown";
      default:
        return "plaintext";
    }
  }, [category]);

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
      if (mode === "create") {
        await createSnippet({
          name: name.trim(),
          category,
          content,
          description,
        } as unknown as import("../../../wailsjs/go/models").snippet_svc.CreateReq);
        toast.success(t("snippet.toast.created"));
      } else if (initial) {
        await updateSnippet({
          id: initial.ID,
          name: name.trim(),
          content,
          description,
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
    <DialogContent className="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{mode === "create" ? t("snippet.form.createTitle") : t("snippet.form.editTitle")}</DialogTitle>
      </DialogHeader>

      <div className="grid gap-4">
        {/* Category / Target */}
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
              {mode === "edit" && category && !categories.some((c) => c.id === category) && (
                <SelectItem key={`orphan-${category}`} value={category} disabled>
                  {t("snippet.unknownCategory", { name: category })}
                </SelectItem>
              )}
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
          />
          {duplicateHint && <p className="text-amber-500 text-xs">{t("snippet.form.duplicateNameHint")}</p>}
        </div>

        {/* Description */}
        <div className="grid gap-1.5">
          <Label htmlFor="snippet-desc">{t("snippet.form.labelDescription")}</Label>
          <Input id="snippet-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>

        {/* Content (Monaco) */}
        <div className="grid gap-1.5">
          <Label>{t("snippet.form.labelContent")}</Label>
          <div className="h-80 border rounded-md overflow-hidden">
            <CodeEditor value={content} onChange={setContent} language={contentLanguage} fontSize={12} />
          </div>
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
  );
}
