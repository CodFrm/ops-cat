import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Play, Search, X } from "lucide-react";
import { toast } from "sonner";
import { Button, Dialog, DialogContent, DialogHeader, DialogTitle, Input, cn } from "@opskat/ui";
import { useAssetStore } from "@/stores/assetStore";
import { useSnippetStore } from "@/stores/snippetStore";
import { snippet_entity } from "../../../wailsjs/go/models";
import { GetSnippetLastAssets, SetSnippetLastAssets, RecordSnippetUse } from "../../../wailsjs/go/app/App";
import { runSnippetOnAsset } from "./snippetRun";

interface SnippetAssetDrawerProps {
  snippet: snippet_entity.Snippet;
  onClose: () => void;
}

export function SnippetAssetDrawer({ snippet, onClose }: SnippetAssetDrawerProps) {
  const { t } = useTranslation();

  const categories = useSnippetStore((s) => s.categories);
  const allAssets = useAssetStore((s) => s.assets);

  // Find the assetType for this snippet's category
  const category = useMemo(() => categories.find((c) => c.id === snippet.Category), [categories, snippet.Category]);
  const assetType = category?.assetType ?? "";

  // Filter assets by type + active status
  const matchingAssets = useMemo(
    () => allAssets.filter((a) => a.Type === assetType && a.Status === 1),
    [allAssets, assetType]
  );

  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [submitting, setSubmitting] = useState(false);

  // Load last-used assets on mount
  useEffect(() => {
    GetSnippetLastAssets(snippet.ID)
      .then((ids) => {
        // Filter to only those still present in matchingAssets
        const valid = new Set<number>();
        const matchingIds = new Set(matchingAssets.map((a) => a.ID));
        for (const id of ids ?? []) {
          if (matchingIds.has(id)) valid.add(id);
        }
        setSelected(valid);
      })
      .catch(() => {});
  }, [snippet.ID, matchingAssets]);

  const filteredAssets = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return matchingAssets;
    return matchingAssets.filter((a) => a.Name.toLowerCase().includes(q));
  }, [matchingAssets, search]);

  const toggleAsset = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleRun = async () => {
    const ids = Array.from(selected);
    if (ids.length === 0) return;

    setSubmitting(true);
    try {
      // Persist selection + record use
      await SetSnippetLastAssets(snippet.ID, ids);
      await RecordSnippetUse(snippet.ID);

      // Run on each selected asset; catch per-asset errors but continue
      const assetsToRun = matchingAssets.filter((a) => selected.has(a.ID));
      for (const asset of assetsToRun) {
        try {
          await runSnippetOnAsset(asset, snippet.Content ?? "");
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          toast.error(`${asset.Name}: ${msg}`);
        }
      }
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="fixed right-0 top-0 h-full w-96 max-w-full rounded-none border-l sm:max-w-sm translate-x-0 translate-y-0 left-auto data-[state=open]:slide-in-from-right data-[state=closed]:slide-out-to-right"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-center justify-between pb-2 border-b">
          <DialogTitle className="text-sm font-medium">{t("snippet.runDrawer.title")}</DialogTitle>
          <Button variant="ghost" size="icon-xs" onClick={onClose} aria-label={t("action.close")}>
            <X className="h-3.5 w-3.5" />
          </Button>
        </DialogHeader>

        {/* Search */}
        <div className="relative mt-2">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("snippet.runDrawer.searchPlaceholder")}
            className="h-8 pl-7 text-xs"
          />
        </div>

        {/* Asset list */}
        <div className="flex-1 overflow-y-auto min-h-0 mt-2 space-y-1">
          {filteredAssets.length === 0 ? (
            <p className="text-xs text-muted-foreground text-center py-8">{t("snippet.runDrawer.noAssets")}</p>
          ) : (
            filteredAssets.map((asset) => {
              const checked = selected.has(asset.ID);
              return (
                <label
                  key={asset.ID}
                  className={cn(
                    "flex items-center gap-2 px-3 py-2 rounded-md text-sm cursor-pointer hover:bg-accent transition-colors",
                    checked && "bg-accent/60"
                  )}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleAsset(asset.ID)}
                    className="h-4 w-4 shrink-0"
                    aria-label={asset.Name}
                  />
                  <span className="truncate font-medium">{asset.Name}</span>
                </label>
              );
            })
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 pt-3 border-t mt-auto">
          <Button variant="outline" size="sm" onClick={onClose} disabled={submitting}>
            {t("action.cancel")}
          </Button>
          <Button
            size="sm"
            disabled={selected.size === 0 || submitting}
            onClick={handleRun}
            aria-label={t("snippet.actions.run")}
          >
            <Play className="h-3.5 w-3.5 mr-1" />
            {submitting ? "..." : t("snippet.actions.run")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
