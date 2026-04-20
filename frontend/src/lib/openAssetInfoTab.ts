import { useAssetStore } from "@/stores/assetStore";
import { useTabStore, type InfoTabMeta } from "@/stores/tabStore";
import { toast } from "sonner";

/** 打开资产详情 info tab；资产已删除时 toast 提示。 */
export function openAssetInfoTab(assetId: number): void {
  const asset = useAssetStore.getState().assets.find((a) => a.ID === assetId);
  if (!asset) {
    toast.error("资产已删除");
    return;
  }
  const tabStore = useTabStore.getState();
  const infoTabId = `info-asset-${assetId}`;
  const existing = tabStore.tabs.find((t) => t.id === infoTabId);
  const meta: InfoTabMeta = {
    type: "info",
    targetType: "asset",
    targetId: assetId,
    name: asset.Name,
    icon: asset.Icon || undefined,
  };
  if (existing) {
    tabStore.activateTab(infoTabId);
  } else {
    tabStore.openTab({
      id: infoTabId,
      type: "info",
      label: asset.Name,
      icon: asset.Icon || undefined,
      meta,
    });
  }
}
