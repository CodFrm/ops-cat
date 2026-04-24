import type { asset_entity } from "../../../wailsjs/go/models";
import { useTerminalStore, type TerminalTabData } from "@/stores/terminalStore";
import { useTabStore, type TerminalTabMeta } from "@/stores/tabStore";
import { useQueryStore } from "@/stores/queryStore";
import { WriteSSH } from "../../../wailsjs/go/app/App";
import { bytesToBase64 } from "@/lib/terminalEncode";

/**
 * Open the right tab for an asset and land the snippet content in its editor.
 * Never auto-executes.
 */
export async function runSnippetOnAsset(asset: asset_entity.Asset, content: string): Promise<void> {
  switch (asset.Type) {
    case "ssh": {
      const existing = findExistingConnectedPane(asset.ID);
      if (existing) {
        await WriteSSH(existing.paneId, bytesToBase64(new TextEncoder().encode(content)));
        return;
      }
      await useTerminalStore.getState().connect(asset, "", false, { initialInput: content });
      return;
    }
    case "database":
      useQueryStore.getState().openQueryTab(asset, { initialSQL: content });
      return;
    case "mongodb":
      useQueryStore.getState().openQueryTab(asset, { initialMongo: content });
      return;
    default:
      throw new Error(`snippetRun: unsupported asset type ${asset.Type}`);
  }
}

function findExistingConnectedPane(assetId: number): { paneId: string } | null {
  const { tabData } = useTerminalStore.getState();
  const tabs = useTabStore.getState().tabs;

  for (const tab of tabs) {
    if (tab.type !== "terminal") continue;
    const meta = tab.meta as TerminalTabMeta;
    if (meta.assetId !== assetId) continue;

    // After connection completes, tab.id === sessionId (the pane key in tabData)
    const data: TerminalTabData | undefined = tabData[tab.id];
    if (!data) continue;

    const paneId = data.activePaneId;
    if (paneId && data.panes[paneId]?.connected) {
      return { paneId };
    }
  }
  return null;
}
