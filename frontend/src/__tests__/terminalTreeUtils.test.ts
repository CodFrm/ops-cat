import { describe, it, expect, beforeEach } from "vitest";
import { getSessionIds, getTerminalActiveAssetIds, type SplitNode } from "../stores/terminalStore";
import { useTerminalStore } from "../stores/terminalStore";
import { useTabStore } from "../stores/tabStore";

describe("getSessionIds", () => {
  it("returns sessionId for terminal leaf", () => {
    const node: SplitNode = { type: "terminal", sessionId: "s1" };
    expect(getSessionIds(node)).toEqual(["s1"]);
  });

  it("returns empty for pending node", () => {
    const node: SplitNode = { type: "pending", pendingId: "p1" };
    expect(getSessionIds(node)).toEqual([]);
  });

  it("returns empty for connecting node", () => {
    const node: SplitNode = { type: "connecting", connectionId: "c1" };
    expect(getSessionIds(node)).toEqual([]);
  });

  it("collects session IDs from split tree", () => {
    const node: SplitNode = {
      type: "split",
      direction: "horizontal",
      ratio: 0.5,
      first: { type: "terminal", sessionId: "s1" },
      second: { type: "terminal", sessionId: "s2" },
    };
    expect(getSessionIds(node)).toEqual(["s1", "s2"]);
  });

  it("handles nested splits with mixed node types", () => {
    const node: SplitNode = {
      type: "split",
      direction: "vertical",
      ratio: 0.5,
      first: {
        type: "split",
        direction: "horizontal",
        ratio: 0.5,
        first: { type: "terminal", sessionId: "s1" },
        second: { type: "pending", pendingId: "p1" },
      },
      second: { type: "terminal", sessionId: "s2" },
    };
    expect(getSessionIds(node)).toEqual(["s1", "s2"]);
  });
});

describe("getTerminalActiveAssetIds", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useTerminalStore.setState({ tabData: {}, sessionSync: {}, connections: {}, connectingAssetIds: new Set() });
  });

  it("returns empty set when no terminal tabs exist", () => {
    const result = getTerminalActiveAssetIds();
    expect(result.size).toBe(0);
  });

  it("returns asset ID when terminal has connected pane", () => {
    useTabStore.setState({
      tabs: [
        {
          id: "tab1",
          type: "terminal",
          label: "Server",
          meta: {
            type: "terminal",
            assetId: 42,
            assetName: "S1",
            assetIcon: "",
            host: "1.2.3.4",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: "tab1",
    });
    useTerminalStore.setState({
      tabData: {
        tab1: {
          splitTree: { type: "terminal", sessionId: "s1" },
          activePaneId: "s1",
          panes: { s1: { sessionId: "s1", transport: "ssh", connected: true, connectedAt: Date.now() } },
          directoryFollowMode: "off",
        },
      },
      connections: {},
      connectingAssetIds: new Set(),
    });

    const result = getTerminalActiveAssetIds();
    expect(result.has(42)).toBe(true);
  });

  it("excludes asset when no panes are connected", () => {
    useTabStore.setState({
      tabs: [
        {
          id: "tab1",
          type: "terminal",
          label: "Server",
          meta: {
            type: "terminal",
            assetId: 42,
            assetName: "S1",
            assetIcon: "",
            host: "1.2.3.4",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: "tab1",
    });
    useTerminalStore.setState({
      tabData: {
        tab1: {
          splitTree: { type: "pending", pendingId: "p1" },
          activePaneId: "p1",
          panes: {},
          directoryFollowMode: "off",
        },
      },
      connections: {},
      connectingAssetIds: new Set(),
    });

    const result = getTerminalActiveAssetIds();
    expect(result.size).toBe(0);
  });

  it("skips non-terminal tabs", () => {
    useTabStore.setState({
      tabs: [
        {
          id: "q1",
          type: "query",
          label: "DB",
          meta: {
            type: "query",
            assetId: 10,
            assetName: "DB1",
            assetIcon: "",
            assetType: "database" as const,
            driver: "mysql",
          },
        },
      ],
      activeTabId: "q1",
    });

    const result = getTerminalActiveAssetIds();
    expect(result.size).toBe(0);
  });
});
