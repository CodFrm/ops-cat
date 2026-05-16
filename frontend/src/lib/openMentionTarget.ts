import { toast } from "sonner";
import i18n from "../i18n";
import type { MentionAttrs } from "./mentionXml";
import { useAssetStore } from "@/stores/assetStore";
import { useQueryStore } from "@/stores/queryStore";
import { openAssetInfoTab } from "./openAssetInfoTab";

function queryTabId(assetId: number) {
  return `query-${assetId}`;
}

function openDatabaseMention(attrs: MentionAttrs) {
  const asset = useAssetStore.getState().assets.find((item) => item.ID === attrs.assetId);
  if (!asset) {
    toast.error(i18n.t("common.mentionAssetDeleted"));
    return;
  }
  if (asset.Type !== "database") {
    openAssetInfoTab(attrs.assetId);
    return;
  }

  const store = useQueryStore.getState();
  const tabId = queryTabId(attrs.assetId);
  store.openQueryTab(asset);

  const database = attrs.database;
  if (!database) return;

  const nextStore = useQueryStore.getState();
  const dbState = nextStore.dbStates[tabId];
  if (dbState && !dbState.expandedDbs.includes(database)) {
    nextStore.toggleDbExpand(tabId, database);
  } else if (dbState && !dbState.tables[database] && !dbState.loadingTables[database]) {
    void nextStore.loadTables(tabId, database);
  }

  if (attrs.target === "table" && attrs.table) {
    useQueryStore.getState().openTableTab(tabId, database, attrs.table);
    return;
  }

  useQueryStore.getState().openSqlTab(tabId, database);
}

export function openMentionTarget(attrs: MentionAttrs): void {
  if (attrs.target === "database" || attrs.target === "table") {
    openDatabaseMention(attrs);
    return;
  }
  openAssetInfoTab(attrs.assetId);
}
