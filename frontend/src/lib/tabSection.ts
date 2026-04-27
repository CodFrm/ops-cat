import type { Tab, InfoTabMeta, QueryTabMeta } from "@/stores/tabStore";
import type { asset_entity } from "../../wailsjs/go/models";
import { normalizeAssetSection, type HomeSection } from "@/lib/assetTypes";

export function tabBelongsToSection(tab: Tab, section: HomeSection, assets: asset_entity.Asset[]): boolean {
  if (section === "home") return true;
  if (tab.type === "terminal") return section === "ssh";
  if (tab.type === "query") {
    return normalizeAssetSection((tab.meta as QueryTabMeta).assetType) === section;
  }
  if (tab.type === "info") {
    const meta = tab.meta as InfoTabMeta;
    if (meta.targetType !== "asset") return false;
    const asset = assets.find((a) => a.ID === meta.targetId);
    if (!asset) return false;
    return normalizeAssetSection(asset.Type) === section;
  }
  return false;
}
