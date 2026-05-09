import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { ConnectSSHAsync, DisconnectSSH, GetSSHSyncState } from "../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import { useTabStore } from "../stores/tabStore";
import {
  __resetTerminalSyncListenersForTest,
  useTerminalStore,
  type TerminalDirectorySyncState,
} from "../stores/terminalStore";
import { useAssetStore } from "../stores/assetStore";
import { asset_entity } from "../../wailsjs/go/models";

function makeSSHAsset(id: number, name = `Server ${id}`): asset_entity.Asset {
  return {
    ID: id,
    Name: name,
    Type: "ssh",
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    Config: JSON.stringify({ host: "10.0.0.1", port: 22, username: "root" }),
    CmdPolicy: "",
    SortOrder: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  } as asset_entity.Asset;
}

function makeSyncState(partial: Partial<TerminalDirectorySyncState> = {}): TerminalDirectorySyncState {
  return {
    sessionId: "s1",
    cwd: "/srv/app",
    cwdKnown: true,
    shell: "/bin/bash",
    shellType: "bash",
    supported: true,
    promptReady: true,
    promptClean: true,
    busy: false,
    status: "ready",
    ...partial,
  };
}

afterEach(() => {
  __resetTerminalSyncListenersForTest();
});

describe("terminalStore.connect", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useTerminalStore.setState({
      tabData: {},
      sessionSync: {},
      connections: {},
      connectingAssetIds: new Set(),
    });
    vi.spyOn(useAssetStore.getState(), "getAssetPath").mockReturnValue("Test/Server");
    vi.mocked(ConnectSSHAsync).mockReset();
  });

  it("should create a new tab when no existing tab for asset", async () => {
    vi.mocked(ConnectSSHAsync).mockResolvedValue("conn-123");

    const asset = makeSSHAsset(1);
    await useTerminalStore.getState().connect(asset);

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("conn-123");
    expect(tabs[0].type).toBe("terminal");
    expect((tabs[0].meta as { assetId: number }).assetId).toBe(1);

    expect(ConnectSSHAsync).toHaveBeenCalledTimes(1);
  });

  it("should reuse existing tab when asset already has a connected terminal", async () => {
    // Pre-populate: an already-connected terminal tab for asset 1
    useTabStore.setState({
      tabs: [
        {
          id: "session-abc",
          type: "terminal",
          label: "Server 1",
          meta: {
            type: "terminal",
            assetId: 1,
            assetName: "Server 1",
            assetIcon: "",
            host: "10.0.0.1",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: null,
    });
    useTerminalStore.setState({
      tabData: {
        "session-abc": {
          splitTree: { type: "terminal", sessionId: "session-abc" },
          activePaneId: "session-abc",
          panes: { "session-abc": { sessionId: "session-abc", connected: true, connectedAt: Date.now() } },
          directoryFollowMode: "off",
        },
      },
    });

    const asset = makeSSHAsset(1);
    const result = await useTerminalStore.getState().connect(asset);

    // Should not call backend
    expect(ConnectSSHAsync).not.toHaveBeenCalled();
    // Should activate existing tab
    expect(useTabStore.getState().activeTabId).toBe("session-abc");
    expect(result).toBe("session-abc");
    // Should not create a new tab
    expect(useTabStore.getState().tabs).toHaveLength(1);
  });

  it("should reuse existing tab when asset is in connecting state", async () => {
    useTabStore.setState({
      tabs: [
        {
          id: "conn-pending",
          type: "terminal",
          label: "Server 2",
          meta: {
            type: "terminal",
            assetId: 2,
            assetName: "Server 2",
            assetIcon: "",
            host: "10.0.0.2",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: null,
    });
    useTerminalStore.setState({
      connections: {
        "conn-pending": {
          connectionId: "conn-pending",
          assetId: 2,
          assetName: "Server 2",
          transport: "ssh",
          password: "",
          logs: [],
          status: "connecting",
          currentStep: "connect",
        },
      },
    });

    const asset = makeSSHAsset(2);
    const result = await useTerminalStore.getState().connect(asset);

    expect(ConnectSSHAsync).not.toHaveBeenCalled();
    expect(useTabStore.getState().activeTabId).toBe("conn-pending");
    expect(result).toBe("conn-pending");
  });

  it("should allow different assets to open separate tabs", async () => {
    vi.mocked(ConnectSSHAsync).mockResolvedValueOnce("conn-1").mockResolvedValueOnce("conn-2");

    await useTerminalStore.getState().connect(makeSSHAsset(1));
    await useTerminalStore.getState().connect(makeSSHAsset(2));

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(2);
    expect((tabs[0].meta as { assetId: number }).assetId).toBe(1);
    expect((tabs[1].meta as { assetId: number }).assetId).toBe(2);
    expect(ConnectSSHAsync).toHaveBeenCalledTimes(2);
  });

  it("should create a new tab with forceNew even when existing tab exists", async () => {
    // Pre-populate: an already-connected terminal tab for asset 1
    useTabStore.setState({
      tabs: [
        {
          id: "session-abc",
          type: "terminal",
          label: "Server 1",
          meta: {
            type: "terminal",
            assetId: 1,
            assetName: "Server 1",
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
          panes: { "session-abc": { sessionId: "session-abc", connected: true, connectedAt: Date.now() } },
          directoryFollowMode: "off",
        },
      },
    });

    vi.mocked(ConnectSSHAsync).mockResolvedValue("conn-new");

    const asset = makeSSHAsset(1);
    const result = await useTerminalStore.getState().connect(asset, "", true);

    // Should call backend to create a new connection
    expect(ConnectSSHAsync).toHaveBeenCalledTimes(1);
    // Should create a second tab
    expect(useTabStore.getState().tabs).toHaveLength(2);
    expect(result).toBe("conn-new");
    // Both tabs should exist
    const tabIds = useTabStore.getState().tabs.map((t) => t.id);
    expect(tabIds).toContain("session-abc");
    expect(tabIds).toContain("conn-new");
  });

  it("should reuse existing tab when forceNew is false (default)", async () => {
    useTabStore.setState({
      tabs: [
        {
          id: "session-abc",
          type: "terminal",
          label: "Server 1",
          meta: {
            type: "terminal",
            assetId: 1,
            assetName: "Server 1",
            assetIcon: "",
            host: "10.0.0.1",
            port: 22,
            username: "root",
          },
        },
      ],
      activeTabId: null,
    });

    const asset = makeSSHAsset(1);
    const result = await useTerminalStore.getState().connect(asset, "", false);

    expect(ConnectSSHAsync).not.toHaveBeenCalled();
    expect(useTabStore.getState().tabs).toHaveLength(1);
    expect(result).toBe("session-abc");
  });

  it("should open multiple new tabs for same asset with forceNew", async () => {
    vi.mocked(ConnectSSHAsync).mockResolvedValueOnce("conn-1").mockResolvedValueOnce("conn-2");

    const asset = makeSSHAsset(1);
    await useTerminalStore.getState().connect(asset, "", true);
    await useTerminalStore.getState().connect(asset, "", true);

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(2);
    expect(tabs[0].id).toBe("conn-1");
    expect(tabs[1].id).toBe("conn-2");
    expect(ConnectSSHAsync).toHaveBeenCalledTimes(2);
  });

  it("should add assetId to connectingAssetIds during connection", async () => {
    let resolveConnect: (val: string) => void;
    vi.mocked(ConnectSSHAsync).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveConnect = resolve;
        })
    );

    const promise = useTerminalStore.getState().connect(makeSSHAsset(5));

    // While connecting, assetId should be in the set
    expect(useTerminalStore.getState().connectingAssetIds.has(5)).toBe(true);

    resolveConnect!("conn-5");
    await promise;
  });
});

describe("terminalStore directory sync", () => {
  beforeEach(() => {
    useTerminalStore.setState({
      tabData: {
        tab1: {
          splitTree: { type: "terminal", sessionId: "s1" },
          activePaneId: "s1",
          panes: { s1: { sessionId: "s1", connected: true, connectedAt: Date.now() } },
          directoryFollowMode: "off",
        },
      },
      sessionSync: {},
    });
  });

  it("stores sync state per session", () => {
    useTerminalStore.getState().setSessionSyncState("s1", {
      sessionId: "s1",
      cwd: "/srv/app",
      cwdKnown: true,
      shell: "/bin/bash",
      shellType: "bash",
      supported: true,
      promptReady: true,
      promptClean: true,
      busy: false,
      status: "ready",
    });

    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/srv/app");
  });

  it("toggles directory follow mode per tab", () => {
    useTerminalStore.getState().setDirectoryFollowMode("tab1", "always");
    expect(useTerminalStore.getState().tabData.tab1.directoryFollowMode).toBe("always");

    useTerminalStore.getState().setDirectoryFollowMode("tab1", "off");
    expect(useTerminalStore.getState().tabData.tab1.directoryFollowMode).toBe("off");
  });
});

describe("terminalStore sync listener lifecycle", () => {
  const eventHandlers = new Map<string, (...data: unknown[]) => void>();

  beforeEach(() => {
    __resetTerminalSyncListenersForTest();
    eventHandlers.clear();
    vi.clearAllMocks();
    vi.mocked(EventsOn).mockImplementation((eventName, handler) => {
      eventHandlers.set(eventName, handler);
      return vi.fn();
    });
    vi.mocked(GetSSHSyncState).mockResolvedValue(makeSyncState());
    vi.spyOn(useAssetStore.getState(), "getAssetPath").mockReturnValue("Test/Server");
    useTabStore.setState({ tabs: [], activeTabId: null });
    useTerminalStore.setState({
      tabData: {},
      sessionSync: {},
      connections: {},
      connectingAssetIds: new Set(),
    });
  });

  async function connectAndEmitSuccess(
    connectionId = "conn-1",
    sessionId = "s1",
    syncState = makeSyncState({ sessionId })
  ) {
    vi.mocked(ConnectSSHAsync).mockResolvedValueOnce(connectionId);
    vi.mocked(GetSSHSyncState).mockResolvedValueOnce(syncState);

    await useTerminalStore.getState().connect(makeSSHAsset(1));

    const connectHandler = eventHandlers.get(`ssh:connect:${connectionId}`);
    expect(connectHandler).toEqual(expect.any(Function));
    if (!connectHandler) {
      throw new Error(`missing connect handler for ${connectionId}`);
    }

    connectHandler({ type: "connected", sessionId });
    await Promise.resolve();

    return connectHandler;
  }

  it("registers a sync listener on successful connect and hydrates initial sync state", async () => {
    await connectAndEmitSuccess("conn-1", "s1", makeSyncState({ sessionId: "s1", cwd: "/var/www" }));

    expect(EventsOn).toHaveBeenCalledWith("ssh:sync:s1", expect.any(Function));
    expect(GetSSHSyncState).toHaveBeenCalledWith("s1");
    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/var/www");
  });

  it("does not register a duplicate sync listener for the same session", async () => {
    const connectHandler = await connectAndEmitSuccess("conn-1", "s1");

    vi.mocked(EventsOn).mockClear();
    vi.mocked(GetSSHSyncState).mockClear();

    connectHandler({ type: "connected", sessionId: "s1" });

    expect(EventsOn).not.toHaveBeenCalledWith("ssh:sync:s1", expect.any(Function));
    expect(GetSSHSyncState).not.toHaveBeenCalled();
  });

  it("unregisters the sync listener and clears sync state when closing a pane", async () => {
    await connectAndEmitSuccess("conn-1", "s1", makeSyncState({ sessionId: "s1", cwd: "/srv/app" }));
    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/srv/app");

    vi.mocked(EventsOff).mockClear();

    useTerminalStore.getState().closePane("s1", "s1");

    expect(EventsOff).toHaveBeenCalledWith("ssh:sync:s1");
    expect(useTerminalStore.getState().sessionSync.s1).toBeUndefined();
  });

  it("unregisters the sync listener and clears sync state when closing a tab", async () => {
    await connectAndEmitSuccess("conn-1", "s1", makeSyncState({ sessionId: "s1", cwd: "/srv/app" }));
    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/srv/app");

    vi.mocked(EventsOff).mockClear();

    useTabStore.getState().closeTab("s1");

    expect(EventsOff).toHaveBeenCalledWith("ssh:sync:s1");
    expect(useTerminalStore.getState().sessionSync.s1).toBeUndefined();
  });

  it("unregisters the sync listener and clears sync state when disconnecting a pane", async () => {
    await connectAndEmitSuccess("conn-1", "s1", makeSyncState({ sessionId: "s1", cwd: "/srv/app" }));
    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/srv/app");

    vi.mocked(EventsOff).mockClear();
    vi.mocked(DisconnectSSH).mockClear();

    useTerminalStore.getState().disconnect("s1");

    expect(DisconnectSSH).toHaveBeenCalledWith("s1");
    expect(EventsOff).toHaveBeenCalledWith("ssh:sync:s1");
    expect(useTerminalStore.getState().sessionSync.s1).toBeUndefined();
  });

  it("unregisters the sync listener and clears sync state when a pane closes remotely", async () => {
    await connectAndEmitSuccess("conn-1", "s1", makeSyncState({ sessionId: "s1", cwd: "/srv/app" }));
    expect(useTerminalStore.getState().sessionSync.s1?.cwd).toBe("/srv/app");

    vi.mocked(EventsOff).mockClear();

    useTerminalStore.getState().markClosed("s1");

    expect(EventsOff).toHaveBeenCalledWith("ssh:sync:s1");
    expect(useTerminalStore.getState().sessionSync.s1).toBeUndefined();
  });
});
