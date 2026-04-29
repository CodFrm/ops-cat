import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Play, X } from "lucide-react";
import { toast } from "sonner";
import { Button, Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@opskat/ui";
import { AssetMultiSelect } from "@/components/asset/AssetMultiSelect";
import { filterAssetTreeAssets } from "@/lib/assetTree";
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

  const category = useMemo(() => categories.find((c) => c.id === snippet.Category), [categories, snippet.Category]);
  const assetType = category?.assetType ?? "";

  const matchingAssetIds = useMemo(
    () => new Set(filterAssetTreeAssets(allAssets, { filterType: assetType, activeOnly: true }).map((a) => a.ID)),
    [allAssets, assetType]
  );

  const [selected, setSelected] = useState<number[]>([]);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const ids = await GetSnippetLastAssets(snippet.ID).catch(() => null);
      if (cancelled) return;
      const valid = (ids ?? []).filter((id) => matchingAssetIds.has(id));
      setSelected(valid);
    })();
    return () => {
      cancelled = true;
    };
  }, [snippet.ID, matchingAssetIds]);

  const handleRun = async () => {
    if (selected.length === 0) return;

    setSubmitting(true);
    try {
      await SetSnippetLastAssets(snippet.ID, selected);
      await RecordSnippetUse(snippet.ID);

      const selectedSet = new Set(selected);
      const assetsToRun = allAssets.filter((a) => selectedSet.has(a.ID));
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
        className="fixed right-0 top-0 h-full w-96 max-w-full rounded-none border-l sm:max-w-sm translate-x-0 translate-y-0 left-auto data-[state=open]:slide-in-from-right data-[state=closed]:slide-out-to-right data-[state=open]:zoom-in-100 data-[state=closed]:zoom-out-100"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-center justify-between pb-2 border-b">
          <DialogTitle className="text-sm font-medium">{t("snippet.runDrawer.title")}</DialogTitle>
          <DialogDescription className="sr-only">{t("snippet.runDrawer.description")}</DialogDescription>
          <Button variant="ghost" size="icon-xs" onClick={onClose} aria-label={t("action.close")}>
            <X className="h-3.5 w-3.5" />
          </Button>
        </DialogHeader>

        <AssetMultiSelect
          values={selected}
          onValuesChange={setSelected}
          filterType={assetType}
          searchPlaceholder={t("snippet.runDrawer.searchPlaceholder")}
          emptyText={t("snippet.runDrawer.noAssets")}
          className="flex-1 mt-2"
        />

        <div className="flex items-center justify-end gap-2 pt-3 border-t mt-auto">
          <Button variant="outline" size="sm" onClick={onClose} disabled={submitting}>
            {t("action.cancel")}
          </Button>
          <Button
            size="sm"
            disabled={selected.length === 0 || submitting}
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
