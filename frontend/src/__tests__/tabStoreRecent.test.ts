import { describe, it, expect, beforeEach } from "vitest";
import { useTabStore } from "../stores/tabStore";
import { useRecentAssetStore } from "../stores/recentAssetStore";
import type { Tab } from "../stores/tabStore";

function makeTerminalTab(id: string, assetId: number): Tab {
  return {
    id,
    type: "terminal",
    label: `Server ${assetId}`,
    meta: {
      type: "terminal",
      assetId,
      assetName: `Server ${assetId}`,
      assetIcon: "",
      host: "10.0.0.1",
      port: 22,
      username: "root",
    },
  };
}

function makeAITab(id: string, conversationId: number | null = null): Tab {
  return {
    id,
    type: "ai",
    label: "AI Chat",
    meta: {
      type: "ai",
      conversationId,
      title: "AI Chat",
    },
  };
}

describe("tabStore → recentAssetStore integration", () => {
  beforeEach(() => {
    localStorage.clear();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useRecentAssetStore.setState({ recentIds: [] });
  });

  it("records assetId in recentAssetStore when a terminal tab is opened", () => {
    useTabStore.getState().openTab(makeTerminalTab("t1", 7));
    expect(useRecentAssetStore.getState().recentIds[0]).toBe(7);
  });

  it("does not modify recentIds when an AI tab (no assetId) is opened", () => {
    useTabStore.getState().openTab(makeAITab("ai-1"));
    expect(useRecentAssetStore.getState().recentIds).toHaveLength(0);
  });

  it("does not touch again when openTab is called with an already-open tab id", () => {
    const tab = makeTerminalTab("t1", 7);
    // First open — should record the asset
    useTabStore.getState().openTab(tab);
    expect(useRecentAssetStore.getState().recentIds).toEqual([7]);

    // Open a second tab so assetId 7 is no longer at head
    useTabStore.getState().openTab(makeTerminalTab("t2", 99));
    expect(useRecentAssetStore.getState().recentIds[0]).toBe(99);

    // Re-activate the existing tab (same id) — should NOT re-touch / re-promote
    useTabStore.getState().openTab(tab);
    // 99 should still be at head because touch was NOT called again
    expect(useRecentAssetStore.getState().recentIds[0]).toBe(99);
    expect(useRecentAssetStore.getState().recentIds).toHaveLength(2);
    expect(useRecentAssetStore.getState().recentIds.includes(7)).toBe(true);
  });

  it("deduplicates recentIds — opening different tabs for the same assetId yields only one entry", () => {
    // touch is idempotent: it moves the id to head but does not duplicate
    useTabStore.getState().openTab(makeTerminalTab("t1", 5));
    useTabStore.getState().openTab(makeTerminalTab("t1-copy", 5));
    const ids = useRecentAssetStore.getState().recentIds;
    expect(ids.filter((x) => x === 5)).toHaveLength(1);
  });

  it("records assetId for a query tab", () => {
    const queryTab: Tab = {
      id: "query-42",
      type: "query",
      label: "DB 42",
      meta: {
        type: "query",
        assetId: 42,
        assetName: "DB 42",
        assetIcon: "",
        assetType: "database",
      },
    };
    useTabStore.getState().openTab(queryTab);
    expect(useRecentAssetStore.getState().recentIds[0]).toBe(42);
  });

  it("records targetId for an info tab when targetType is 'asset'", () => {
    const infoTab: Tab = {
      id: "info-asset-99",
      type: "info",
      label: "Asset 99",
      meta: {
        type: "info",
        targetType: "asset",
        targetId: 99,
        name: "Asset 99",
      },
    };
    useTabStore.getState().openTab(infoTab);
    expect(useRecentAssetStore.getState().recentIds[0]).toBe(99);
  });

  it("does not record targetId for an info tab when targetType is 'group'", () => {
    const infoTab: Tab = {
      id: "info-group-5",
      type: "info",
      label: "Group 5",
      meta: {
        type: "info",
        targetType: "group",
        targetId: 5,
        name: "Group 5",
      },
    };
    useTabStore.getState().openTab(infoTab);
    expect(useRecentAssetStore.getState().recentIds).toHaveLength(0);
  });
});
