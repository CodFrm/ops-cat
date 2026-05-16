import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@opskat/ui";
import { ConnectSSHAsync, ListAssets, ListGroups } from "../../wailsjs/go/app/App";
import { AssetTree } from "@/components/layout/AssetTree";
import { useAssetStore } from "../stores/assetStore";
import { useTabStore } from "../stores/tabStore";
import { useTerminalStore } from "../stores/terminalStore";
import { useQueryStore } from "../stores/queryStore";
import { asset_entity, group_entity } from "../../wailsjs/go/models";
import { getAssetType } from "../lib/assetTypes";
import React from "react";

// ---- Minimal AssetList component that replicates the key double-click logic from AssetTree ----
// AssetTree itself has heavy dependencies (icons, pinyin, scroll area, etc.).
// We extract the core interaction logic into a thin test-only component so we can
// verify the click → store → tab flow without mocking every UI dependency.

function AssetList({
  assets,
  onConnectAsset,
  onSelectAsset,
}: {
  assets: asset_entity.Asset[];
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onSelectAsset: (asset: asset_entity.Asset) => void;
}) {
  const clickTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);

  return (
    <div>
      {assets.map((asset) => {
        const isConnecting = connectingAssetIds.has(asset.ID);
        return (
          <div
            key={asset.ID}
            data-testid={`asset-${asset.ID}`}
            onClick={() => {
              if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
              clickTimerRef.current = setTimeout(() => {
                clickTimerRef.current = null;
                onSelectAsset(asset);
              }, 200);
            }}
            onDoubleClick={() => {
              if (clickTimerRef.current) {
                clearTimeout(clickTimerRef.current);
                clickTimerRef.current = null;
              }
              const def = getAssetType(asset.Type);
              if (def?.canConnect && (def.connectAction === "query" || !isConnecting)) {
                onConnectAsset(asset);
              } else {
                onSelectAsset(asset);
              }
            }}
          >
            {asset.Name}
          </div>
        );
      })}
    </div>
  );
}

// ---- Replicate App.tsx handleConnectAsset ----

function makeHandleConnectAsset() {
  const errors: string[] = [];
  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    if (asset.Type === "database" || asset.Type === "redis" || asset.Type === "mongodb") {
      useQueryStore.getState().openQueryTab(asset);
      return;
    }
    if (asset.Type !== "ssh") return;
    try {
      await useTerminalStore.getState().connect(asset);
    } catch (e) {
      errors.push(String(e));
    }
  };
  return { handleConnectAsset, errors };
}

function makeHandleConnectAssetInNewTab() {
  const errors: string[] = [];
  const handleConnectAssetInNewTab = async (asset: asset_entity.Asset) => {
    if (asset.Type !== "ssh") return;
    try {
      await useTerminalStore.getState().connect(asset, "", true);
    } catch (e) {
      errors.push(String(e));
    }
  };
  return { handleConnectAssetInNewTab, errors };
}

// ---- Helpers ----

function makeAsset(id: number, type: string, name?: string): asset_entity.Asset {
  return {
    ID: id,
    Name: name || `Asset ${id}`,
    Type: type,
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    Config: JSON.stringify(
      type === "ssh"
        ? { host: "10.0.0.1", port: 22, username: "root" }
        : type === "database"
          ? { driver: "mysql", database: "testdb" }
          : { host: "10.0.0.1", port: 6379 }
    ),
    CmdPolicy: "",
    SortOrder: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  } as asset_entity.Asset;
}

describe("AssetTree double-click → connection flow", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useTerminalStore.setState({ tabData: {}, connections: {}, connectingAssetIds: new Set() });
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} });
    useAssetStore.setState({ assets: [], groups: [] as group_entity.Group[] });
    vi.spyOn(useAssetStore.getState(), "getAssetPath").mockImplementation((a) => a.Name);
    vi.mocked(ConnectSSHAsync).mockReset();
  });

  it("double-click SSH asset opens terminal tab", async () => {
    vi.mocked(ConnectSSHAsync).mockResolvedValue("conn-1");
    const sshAsset = makeAsset(1, "ssh", "Web Server");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[sshAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-1"));
    // Wait for async connect
    await vi.waitFor(() => {
      expect(useTabStore.getState().tabs).toHaveLength(1);
    });

    const tab = useTabStore.getState().tabs[0];
    expect(tab.type).toBe("terminal");
    expect(tab.id).toBe("conn-1");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("double-click SSH asset with existing tab activates it without new connection", async () => {
    // Pre-populate an existing connected terminal tab
    useTabStore.setState({
      tabs: [
        {
          id: "session-abc",
          type: "terminal",
          label: "Web Server",
          meta: {
            type: "terminal",
            assetId: 1,
            assetName: "Web Server",
            assetIcon: "",
            host: "10.0.0.1",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: null,
    });

    const sshAsset = makeAsset(1, "ssh", "Web Server");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[sshAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-1"));
    await vi.waitFor(() => {
      expect(useTabStore.getState().activeTabId).toBe("session-abc");
    });

    // No new connection, no new tab
    expect(ConnectSSHAsync).not.toHaveBeenCalled();
    expect(useTabStore.getState().tabs).toHaveLength(1);
  });

  it("double-click database asset opens query tab", async () => {
    const dbAsset = makeAsset(2, "database", "MySQL Prod");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[dbAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-2"));

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("query-2");
    expect(tabs[0].type).toBe("query");
    expect(useQueryStore.getState().dbStates["query-2"]).toBeDefined();
  });

  it("double-click database asset with existing tab activates it", async () => {
    const dbAsset = makeAsset(2, "database", "MySQL Prod");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    // Open once
    useQueryStore.getState().openQueryTab(dbAsset);
    expect(useTabStore.getState().tabs).toHaveLength(1);

    // Switch to another tab
    useTabStore.setState({ activeTabId: null });

    const user = userEvent.setup();
    render(<AssetList assets={[dbAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-2"));

    // Should reuse, not create new
    expect(useTabStore.getState().tabs).toHaveLength(1);
    expect(useTabStore.getState().activeTabId).toBe("query-2");
  });

  it("double-click redis asset opens query tab", async () => {
    const redisAsset = makeAsset(3, "redis", "Cache");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[redisAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-3"));

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("query-3");
    expect(useQueryStore.getState().redisStates["query-3"]).toBeDefined();
  });

  it("double-clicking a mongodb asset opens query tab", async () => {
    const mongoAsset = new asset_entity.Asset({
      ID: 30,
      Name: "MongoDB",
      Type: "mongodb",
      Config: JSON.stringify({ host: "localhost", port: 27017 }),
      Status: 1,
    });
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(<AssetList assets={[mongoAsset]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);
    await user.dblClick(screen.getByTestId("asset-30"));

    const tabStore = useTabStore.getState();
    expect(tabStore.tabs.some((t) => t.id === "query-30")).toBe(true);
  });

  it("double-click unknown type asset triggers select, not connect", async () => {
    const otherAsset = makeAsset(4, "other", "Unknown");
    const onConnect = vi.fn();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[otherAsset]} onConnectAsset={onConnect} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-4"));
    // Double-click on unknown type should call onSelectAsset, not onConnectAsset
    expect(onConnect).not.toHaveBeenCalled();
    expect(onSelect).toHaveBeenCalledWith(otherAsset);
  });

  it("double-click SSH asset while connecting does not trigger connect", async () => {
    const sshAsset = makeAsset(5, "ssh", "Busy Server");
    useTerminalStore.setState({
      connectingAssetIds: new Set([5]),
    });

    const onConnect = vi.fn();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    render(<AssetList assets={[sshAsset]} onConnectAsset={onConnect} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-5"));
    // Should not call connect because isConnecting is true
    expect(onConnect).not.toHaveBeenCalled();
  });

  it("multiple different assets can each open their own tab", async () => {
    vi.mocked(ConnectSSHAsync).mockResolvedValueOnce("conn-1").mockResolvedValueOnce("conn-2");
    const ssh1 = makeAsset(1, "ssh", "Server 1");
    const ssh2 = makeAsset(2, "ssh", "Server 2");
    const db = makeAsset(3, "database", "MySQL");
    const { handleConnectAsset } = makeHandleConnectAsset();
    const onSelect = vi.fn();

    const user = userEvent.setup();
    const { rerender } = render(
      <AssetList assets={[ssh1, ssh2, db]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />
    );

    await user.dblClick(screen.getByTestId("asset-1"));
    await vi.waitFor(() => expect(useTabStore.getState().tabs).toHaveLength(1));

    // Need to rerender because connectingAssetIds changed
    rerender(<AssetList assets={[ssh1, ssh2, db]} onConnectAsset={handleConnectAsset} onSelectAsset={onSelect} />);

    await user.dblClick(screen.getByTestId("asset-2"));
    await vi.waitFor(() => expect(useTabStore.getState().tabs).toHaveLength(2));

    await user.dblClick(screen.getByTestId("asset-3"));
    expect(useTabStore.getState().tabs).toHaveLength(3);

    const types = useTabStore.getState().tabs.map((t) => t.type);
    expect(types).toEqual(["terminal", "terminal", "query"]);
  });

  it("connect in new tab creates new terminal even when existing tab exists", async () => {
    // Pre-populate an existing connected terminal tab
    useTabStore.setState({
      tabs: [
        {
          id: "session-abc",
          type: "terminal",
          label: "Web Server",
          meta: {
            type: "terminal",
            assetId: 1,
            assetName: "Web Server",
            assetIcon: "",
            host: "10.0.0.1",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: "session-abc",
    });
    useTerminalStore.setState({
      tabData: {
        "session-abc": {
          splitTree: { type: "terminal", sessionId: "session-abc" },
          activePaneId: "session-abc",
          panes: {
            "session-abc": { sessionId: "session-abc", transport: "ssh", connected: true, connectedAt: Date.now() },
          },
          directoryFollowMode: "off",
        },
      },
    });

    vi.mocked(ConnectSSHAsync).mockResolvedValue("conn-new");

    const sshAsset = makeAsset(1, "ssh", "Web Server");
    const { handleConnectAssetInNewTab } = makeHandleConnectAssetInNewTab();

    await handleConnectAssetInNewTab(sshAsset);

    await vi.waitFor(() => {
      expect(useTabStore.getState().tabs).toHaveLength(2);
    });

    expect(ConnectSSHAsync).toHaveBeenCalledTimes(1);
    const tabIds = useTabStore.getState().tabs.map((t) => t.id);
    expect(tabIds).toContain("session-abc");
    expect(tabIds).toContain("conn-new");
  });

  it("connect in new tab ignores non-SSH assets", async () => {
    const dbAsset = makeAsset(2, "database", "MySQL");
    const { handleConnectAssetInNewTab } = makeHandleConnectAssetInNewTab();

    await handleConnectAssetInNewTab(dbAsset);

    expect(ConnectSSHAsync).not.toHaveBeenCalled();
    expect(useTabStore.getState().tabs).toHaveLength(0);
  });
});

describe("AssetTree ungrouped virtual folder", () => {
  const renderAssetTree = () =>
    render(
      <TooltipProvider>
        <AssetTree
          collapsed={false}
          onAddAsset={() => {}}
          onAddGroup={() => {}}
          onEditGroup={() => {}}
          onGroupDetail={() => {}}
          onEditAsset={() => {}}
          onCopyAsset={() => {}}
          onConnectAsset={() => {}}
          onSelectAsset={() => {}}
        />
      </TooltipProvider>
    );

  beforeEach(() => {
    localStorage.clear();
    useAssetStore.setState({
      assets: [],
      groups: [],
      selectedAssetId: null,
      selectedGroupId: null,
      collapsedGroupIds: [0],
      loading: false,
      initialized: false,
    });
    useTerminalStore.setState({ tabData: {}, connections: {}, connectingAssetIds: new Set() });
    useTabStore.setState({ tabs: [], activeTabId: null });
    vi.mocked(ListAssets).mockResolvedValue([makeAsset(1, "ssh", "Ungrouped Server")]);
    vi.mocked(ListGroups).mockResolvedValue([]);
  });

  it("renders ungrouped assets after expanding the virtual folder", async () => {
    const user = userEvent.setup();
    renderAssetTree();

    expect(await screen.findByText("asset.ungrouped")).toBeTruthy();
    expect(screen.queryByText("Ungrouped Server")).toBeNull();

    await user.click(screen.getByText("asset.ungrouped"));

    expect(screen.getByText("Ungrouped Server")).toBeTruthy();
  });
});
