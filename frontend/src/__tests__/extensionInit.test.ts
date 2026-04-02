/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { useExtensionStore } from "../extension/store";
import { ListInstalledExtensions } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

// Mock extension dependencies
vi.mock("../extension/inject", () => ({ injectExtensionAPI: vi.fn() }));
vi.mock("../extension/api", () => ({ createExtensionAPI: vi.fn() }));
vi.mock("../extension/loader", () => ({ clearExtensionCache: vi.fn() }));

import { initExtensions, _refreshExtensions, _resetForTesting } from "../extension/init";

const manifest = {
  name: "oss",
  version: "1.0.0",
  icon: "cloud",
  i18n: { displayName: "OSS", description: "Object Storage" },
  frontend: {
    entry: "index.js",
    styles: "style.css",
    pages: [{ id: "browser", slot: "asset.connect", i18n: { name: "Browser" }, component: "BrowserPage" }],
  },
  assetTypes: [{ type: "oss", i18n: { name: "OSS" } }],
};

function resetStore() {
  useExtensionStore.setState({ ready: false, extensions: {} });
}

describe("extension store", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("starts with ready=false and empty extensions", () => {
    const state = useExtensionStore.getState();
    expect(state.ready).toBe(false);
    expect(state.extensions).toEqual({});
  });

  it("register adds an extension entry", () => {
    useExtensionStore.getState().register("oss", manifest as any);
    expect(useExtensionStore.getState().extensions["oss"]).toBeDefined();
  });

  it("unregister removes an extension entry", () => {
    useExtensionStore.getState().register("oss", manifest as any);
    useExtensionStore.getState().unregister("oss");
    expect(useExtensionStore.getState().extensions["oss"]).toBeUndefined();
  });

  it("getExtensionForAssetType finds correct extension", () => {
    useExtensionStore.getState().register("oss", manifest as any);
    const result = useExtensionStore.getState().getExtensionForAssetType("oss");
    expect(result).toBeDefined();
    expect(result!.name).toBe("oss");
  });

  it("isExtensionAssetType returns false for built-in type", () => {
    expect(useExtensionStore.getState().isExtensionAssetType("ssh")).toBe(false);
  });
});

describe("initExtensions (bootstrap + subscribe)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    _resetForTesting();
  });

  it("registers enabled extensions and sets ready=true", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue([{ name: "oss", enabled: true, manifest }] as any);

    await initExtensions();

    const state = useExtensionStore.getState();
    expect(state.ready).toBe(true);
    expect(state.extensions["oss"]).toBeDefined();
  });

  it("skips disabled extensions", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue([{ name: "oss", enabled: false, manifest }] as any);

    await initExtensions();

    const state = useExtensionStore.getState();
    // disabled-only list still has length > 0, so ready is set
    expect(state.ready).toBe(true);
    expect(state.extensions["oss"]).toBeUndefined();
  });

  it("defers ready when ListInstalledExtensions fails (waits for ext:ready)", async () => {
    vi.mocked(ListInstalledExtensions).mockRejectedValue(new Error("IPC not ready"));

    await initExtensions();

    // ready stays false — ext:ready event will set it later
    expect(useExtensionStore.getState().ready).toBe(false);
  });

  it("defers ready on null response (waits for ext:ready)", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue(null as any);

    await initExtensions();

    // empty list means backend init not done — ready deferred to ext:ready
    expect(useExtensionStore.getState().ready).toBe(false);
    expect(Object.keys(useExtensionStore.getState().extensions)).toHaveLength(0);
  });

  it("unregisters extensions that are no longer installed", async () => {
    useExtensionStore.getState().register("old-ext", manifest as any);

    vi.mocked(ListInstalledExtensions).mockResolvedValue([{ name: "oss", enabled: true, manifest }] as any);

    await initExtensions();

    const state = useExtensionStore.getState();
    expect(state.extensions["old-ext"]).toBeUndefined();
    expect(state.extensions["oss"]).toBeDefined();
  });

  it("registers ext:reload and ext:ready event listeners", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue([]);
    vi.mocked(EventsOn).mockReturnValue(() => {});

    await initExtensions();

    expect(EventsOn).toHaveBeenCalledWith("ext:reload", expect.any(Function));
    expect(EventsOn).toHaveBeenCalledWith("ext:ready", expect.any(Function));
  });

  it("is idempotent — second call is a no-op", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue([]);
    vi.mocked(EventsOn).mockReturnValue(() => {});

    await initExtensions();
    await initExtensions();

    expect(ListInstalledExtensions).toHaveBeenCalledTimes(1);
    // 2 subscriptions: ext:reload + ext:ready
    expect(EventsOn).toHaveBeenCalledTimes(2);
  });
});

describe("refreshExtensions (internal)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    _resetForTesting();
  });

  it("does not set ready — only bootstrap sets ready", async () => {
    vi.mocked(ListInstalledExtensions).mockResolvedValue([{ name: "oss", enabled: true, manifest }] as any);

    await _refreshExtensions();

    expect(useExtensionStore.getState().ready).toBe(false);
    expect(useExtensionStore.getState().extensions["oss"]).toBeDefined();
  });
});
