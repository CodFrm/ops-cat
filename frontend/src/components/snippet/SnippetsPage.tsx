import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Pencil, Copy, Trash2, Lock, Search, Loader2, Play } from "lucide-react";
import { toast } from "sonner";
import {
  Button,
  ConfirmDialog,
  Input,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  cn,
} from "@opskat/ui";
import { useSnippetStore } from "@/stores/snippetStore";
import { snippet_entity } from "../../../wailsjs/go/models";
import { SnippetFormDialog } from "./SnippetFormDialog";
import { SnippetAssetDrawer } from "./SnippetAssetDrawer";

const RUNNABLE_ASSET_TYPES = new Set(["ssh", "database", "mongodb"]);

function isRunnable(cat: string, cats: { id: string; assetType: string }[]): boolean {
  const c = cats.find((x) => x.id === cat);
  return !!c && RUNNABLE_ASSET_TYPES.has(c.assetType);
}

// Stable per-category badge styling. Order must be deterministic so a given
// category always renders with the same hue across reloads.
const CATEGORY_BADGE_COLORS = [
  "bg-blue-500/15 text-blue-700 dark:text-blue-300 ring-1 ring-inset ring-blue-500/20",
  "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 ring-1 ring-inset ring-emerald-500/20",
  "bg-rose-500/15 text-rose-700 dark:text-rose-300 ring-1 ring-inset ring-rose-500/20",
  "bg-amber-500/15 text-amber-700 dark:text-amber-300 ring-1 ring-inset ring-amber-500/20",
  "bg-violet-500/15 text-violet-700 dark:text-violet-300 ring-1 ring-inset ring-violet-500/20",
];

function categoryBadgeClass(categoryId: string, orderedIds: string[]): string {
  const idx = orderedIds.indexOf(categoryId);
  if (idx < 0) return CATEGORY_BADGE_COLORS[0];
  return CATEGORY_BADGE_COLORS[idx % CATEGORY_BADGE_COLORS.length];
}

function formatUpdated(raw: unknown): string {
  if (!raw) return "-";
  try {
    const d = new Date(raw as string);
    if (isNaN(d.getTime())) return "-";
    return d.toLocaleString();
  } catch {
    return "-";
  }
}

function isReadOnly(s: snippet_entity.Snippet): boolean {
  return typeof s.Source === "string" && s.Source.startsWith("ext:");
}

export function SnippetsPage() {
  const { t } = useTranslation();

  const categories = useSnippetStore((s) => s.categories);
  const list = useSnippetStore((s) => s.list);
  const listLoading = useSnippetStore((s) => s.listLoading);
  const filter = useSnippetStore((s) => s.filter);
  const loadCategories = useSnippetStore((s) => s.loadCategories);
  const loadList = useSnippetStore((s) => s.loadList);
  const setFilter = useSnippetStore((s) => s.setFilter);
  const duplicateSnippet = useSnippetStore((s) => s.duplicate);
  const removeSnippet = useSnippetStore((s) => s.remove);

  const [searchInput, setSearchInput] = useState(filter.keyword);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<"create" | "edit">("create");
  const [editTarget, setEditTarget] = useState<snippet_entity.Snippet | undefined>(undefined);
  const [confirmTarget, setConfirmTarget] = useState<snippet_entity.Snippet | null>(null);
  const [categoryFilterOpen, setCategoryFilterOpen] = useState(false);
  const [runTarget, setRunTarget] = useState<snippet_entity.Snippet | null>(null);

  // Load categories + initial list on mount.
  useEffect(() => {
    void loadCategories().then(() => loadList());
  }, [loadCategories, loadList]);

  // Debounce search input → filter.keyword
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    searchTimerRef.current = setTimeout(() => {
      if (searchInput !== filter.keyword) setFilter({ keyword: searchInput });
    }, 300);
    return () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    };
    // filter.keyword intentionally excluded: we only want to push local → store,
    // not re-run the timer every time the store re-emits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput]);

  const orderedCategoryIds = useMemo(() => categories.map((c) => c.id), [categories]);
  const categoryLabelById = useMemo(() => {
    const m: Record<string, string> = {};
    for (const c of categories) m[c.id] = c.label;
    return m;
  }, [categories]);

  // Orphan categories: snippets referencing a category id that is no longer
  // registered (e.g. the providing extension was uninstalled). Exposed in the
  // filter dropdown + column badge; Create/Edit form still only lists registered.
  const orphanCategoryIds = useMemo(() => {
    const registered = new Set(orderedCategoryIds);
    const seen = new Set<string>();
    const out: string[] = [];
    for (const s of list) {
      if (!s.Category || registered.has(s.Category) || seen.has(s.Category)) continue;
      seen.add(s.Category);
      out.push(s.Category);
    }
    out.sort();
    return out;
  }, [list, orderedCategoryIds]);

  const isOrphanCategory = useCallback((id: string) => !!id && !categoryLabelById[id], [categoryLabelById]);

  const toggleCategoryFilter = useCallback(
    (id: string) => {
      const next = filter.categories.includes(id)
        ? filter.categories.filter((c) => c !== id)
        : [...filter.categories, id];
      setFilter({ categories: next });
    },
    [filter.categories, setFilter]
  );

  const clearCategoryFilter = useCallback(() => setFilter({ categories: [] }), [setFilter]);

  const onClickNew = () => {
    setDialogMode("create");
    setEditTarget(undefined);
    setDialogOpen(true);
  };

  const onClickEdit = (s: snippet_entity.Snippet) => {
    if (isReadOnly(s)) return;
    setDialogMode("edit");
    setEditTarget(s);
    setDialogOpen(true);
  };

  const onClickDuplicate = async (s: snippet_entity.Snippet) => {
    try {
      await duplicateSnippet(s.ID);
      toast.success(t("snippet.toast.duplicated"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(msg);
    }
  };

  const onConfirmDelete = async () => {
    if (!confirmTarget) return;
    try {
      await removeSnippet(confirmTarget.ID);
      toast.success(t("snippet.toast.deleted"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(msg);
    } finally {
      setConfirmTarget(null);
    }
  };

  const selectedCategoryLabel =
    filter.categories.length === 0
      ? t("snippet.allCategories")
      : filter.categories
          .map((id) => (categoryLabelById[id] ? categoryLabelById[id] : t("snippet.unknownCategory", { name: id })))
          .join(", ");

  const isEmpty = list.length === 0 && !listLoading;
  const hasActiveFilter = filter.categories.length > 0 || filter.keyword.trim() !== "";

  return (
    <div className="flex flex-col h-full">
      {/* Header: title + filters + actions */}
      <div className="px-4 py-3 border-b flex items-center justify-between gap-3 flex-wrap">
        <h2 className="font-semibold">{t("snippet.title")}</h2>
        <div className="flex items-center gap-2 flex-wrap">
          {/* Search */}
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder={t("snippet.searchPlaceholder")}
              className="h-8 w-64 pl-7 text-xs"
            />
          </div>

          {/* Category multi-select (popover with checkboxes) */}
          <Popover open={categoryFilterOpen} onOpenChange={setCategoryFilterOpen}>
            <PopoverTrigger asChild>
              <Button variant="outline" size="sm" className="h-8 text-xs gap-1.5 font-normal max-w-xs">
                <span className="truncate">{selectedCategoryLabel}</span>
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-52 p-2" align="end">
              <div className="flex flex-col gap-1">
                <button
                  type="button"
                  onClick={clearCategoryFilter}
                  className={cn(
                    "text-xs px-2 py-1.5 rounded hover:bg-accent text-left",
                    filter.categories.length === 0 && "bg-accent"
                  )}
                >
                  {t("snippet.allCategories")}
                </button>
                <div className="my-1 h-px bg-border" />
                {categories.map((c) => {
                  const checked = filter.categories.includes(c.id);
                  return (
                    <label
                      key={c.id}
                      className="flex items-center gap-2 px-2 py-1.5 text-xs rounded hover:bg-accent cursor-pointer"
                    >
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggleCategoryFilter(c.id)}
                        className="h-3.5 w-3.5"
                      />
                      <span
                        className={cn(
                          "inline-block px-1.5 py-0.5 rounded text-[10px] font-mono",
                          categoryBadgeClass(c.id, orderedCategoryIds)
                        )}
                      >
                        {c.label}
                      </span>
                    </label>
                  );
                })}
                {orphanCategoryIds.length > 0 && (
                  <>
                    <div className="my-1 h-px bg-border" />
                    {orphanCategoryIds.map((id) => {
                      const checked = filter.categories.includes(id);
                      return (
                        <label
                          key={`orphan-${id}`}
                          className="flex items-center gap-2 px-2 py-1.5 text-xs rounded hover:bg-accent cursor-pointer"
                          title={t("snippet.unknownCategoryTooltip")}
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleCategoryFilter(id)}
                            className="h-3.5 w-3.5"
                          />
                          <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-mono bg-muted text-muted-foreground">
                            {t("snippet.unknownCategory", { name: id })}
                          </span>
                          <span className="text-[10px] text-muted-foreground">{t("snippet.orphanedSuffix")}</span>
                        </label>
                      );
                    })}
                  </>
                )}
              </div>
            </PopoverContent>
          </Popover>

          <Button size="sm" className="h-8 text-xs gap-1.5" onClick={onClickNew}>
            <Plus className="h-3.5 w-3.5" />
            {t("snippet.newButton")}
          </Button>
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-background border-b z-10">
            <tr className="text-left text-muted-foreground">
              <th className="px-4 py-2 font-medium w-24">{t("snippet.columns.category")}</th>
              <th className="px-4 py-2 font-medium">{t("snippet.columns.name")}</th>
              <th className="px-4 py-2 font-medium w-40">{t("snippet.columns.updated")}</th>
              <th className="px-4 py-2 font-medium w-28">{t("snippet.columns.source")}</th>
              <th className="px-4 py-2 font-medium w-28 text-right">{t("snippet.columns.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {listLoading && list.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-12 text-center text-muted-foreground">
                  <Loader2 className="h-5 w-5 animate-spin mx-auto text-muted-foreground" />
                </td>
              </tr>
            )}
            {isEmpty && (
              <tr>
                <td colSpan={5} className="px-4 py-12 text-center text-muted-foreground">
                  {hasActiveFilter ? t("snippet.emptyStateFiltered") : t("snippet.emptyState")}
                </td>
              </tr>
            )}
            {list.map((s) => {
              const readOnly = isReadOnly(s);
              return (
                <tr key={s.ID} className="border-b hover:bg-muted/50 transition-colors">
                  <td className="px-4 py-2">
                    {isOrphanCategory(s.Category) ? (
                      <span
                        className="inline-block px-1.5 py-0.5 rounded text-[10px] font-mono bg-muted text-muted-foreground"
                        title={t("snippet.unknownCategoryTooltip")}
                      >
                        {t("snippet.unknownCategory", { name: s.Category })}
                      </span>
                    ) : (
                      <span
                        className={cn(
                          "inline-block px-1.5 py-0.5 rounded text-[10px] font-mono",
                          categoryBadgeClass(s.Category, orderedCategoryIds)
                        )}
                      >
                        {categoryLabelById[s.Category] ?? s.Category}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2">
                    <div className="flex items-center gap-1.5">
                      {readOnly && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Lock className="h-3 w-3 text-muted-foreground" data-testid="readonly-lock" />
                          </TooltipTrigger>
                          <TooltipContent>{t("snippet.readOnlyTooltip")}</TooltipContent>
                        </Tooltip>
                      )}
                      <span className="font-medium">{s.Name}</span>
                      {s.Description && (
                        <span className="text-xs text-muted-foreground truncate max-w-xs" title={s.Description}>
                          — {s.Description}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground whitespace-nowrap">
                    {formatUpdated(s.UpdatedAt)}
                  </td>
                  <td className="px-4 py-2 text-xs font-mono text-muted-foreground">{s.Source || "user"}</td>
                  <td className="px-4 py-2">
                    <div className="flex items-center justify-end gap-1">
                      {isRunnable(s.Category, categories) && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7"
                              aria-label={t("snippet.actions.run")}
                              onClick={() => setRunTarget(s)}
                            >
                              <Play className="h-3.5 w-3.5" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t("snippet.actions.run")}</TooltipContent>
                        </Tooltip>
                      )}
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            disabled={readOnly}
                            aria-label={t("snippet.actions.edit")}
                            onClick={() => onClickEdit(s)}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                          {readOnly ? t("snippet.readOnlyTooltip") : t("snippet.actions.edit")}
                        </TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            aria-label={t("snippet.actions.duplicate")}
                            onClick={() => onClickDuplicate(s)}
                          >
                            <Copy className="h-3.5 w-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{t("snippet.actions.duplicate")}</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-destructive hover:text-destructive"
                            disabled={readOnly}
                            aria-label={t("snippet.actions.delete")}
                            onClick={() => setConfirmTarget(s)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                          {readOnly ? t("snippet.readOnlyTooltip") : t("snippet.actions.delete")}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <SnippetFormDialog
        open={dialogOpen}
        mode={dialogMode}
        initial={editTarget}
        onOpenChange={(o) => {
          setDialogOpen(o);
          if (!o) setEditTarget(undefined);
        }}
      />

      <ConfirmDialog
        open={!!confirmTarget}
        onOpenChange={(o) => !o && setConfirmTarget(null)}
        title={t("snippet.confirmDelete.title")}
        description={t("snippet.confirmDelete.message", { name: confirmTarget?.Name ?? "" })}
        confirmText={t("snippet.actions.delete")}
        cancelText={t("snippet.actions.cancel")}
        variant="destructive"
        onConfirm={onConfirmDelete}
      />

      {runTarget && (
        <SnippetAssetDrawer
          snippet={runTarget}
          onClose={() => setRunTarget(null)}
        />
      )}
    </div>
  );
}
