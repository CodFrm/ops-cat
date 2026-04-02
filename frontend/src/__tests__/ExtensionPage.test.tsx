/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { render, screen, act, cleanup } from "@testing-library/react";
import { useExtensionStore } from "../extension/store";
import { ExtensionPage } from "../extension/ExtensionPage";

vi.mock("../extension/loader", () => ({
  loadExtension: vi.fn(),
  clearExtensionCache: vi.fn(),
}));
vi.mock("../extension/i18n", () => ({
  loadExtensionLocales: vi.fn().mockResolvedValue(undefined),
}));

import { loadExtension } from "../extension/loader";

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

function DummyComponent({ assetId }: { assetId?: number }) {
  return <div data-testid="extension-content">Extension loaded (asset={assetId})</div>;
}

function resetStore() {
  useExtensionStore.setState({ ready: false, extensions: {} });
}

describe("ExtensionPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    resetStore();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("shows loading when store is not ready", () => {
    render(<ExtensionPage extensionName="oss" pageId="browser" />);
    expect(screen.getByText("Loading extension...")).toBeInTheDocument();
  });

  it("shows loading (not error) when ready but extension not yet registered", () => {
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="browser" />);
    expect(screen.getByText("Loading extension...")).toBeInTheDocument();
    expect(screen.queryByText(/not registered/)).not.toBeInTheDocument();
  });

  it("shows error after timeout when extension never registers", async () => {
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="browser" />);

    await act(async () => {
      vi.advanceTimersByTime(5000);
    });

    expect(screen.getByText(/not registered/)).toBeInTheDocument();
  });

  it("cancels timeout and loads when extension registers before timeout", async () => {
    useExtensionStore.getState().setReady(true);

    const { rerender } = render(<ExtensionPage extensionName="oss" pageId="browser" />);
    expect(screen.getByText("Loading extension...")).toBeInTheDocument();

    vi.mocked(loadExtension).mockResolvedValue({
      name: "oss",
      manifest: manifest as any,
      components: { BrowserPage: DummyComponent },
    });

    await act(async () => {
      vi.advanceTimersByTime(1000);
      useExtensionStore.getState().register("oss", manifest as any);
    });

    rerender(<ExtensionPage extensionName="oss" pageId="browser" />);

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.queryByText(/not registered/)).not.toBeInTheDocument();
    expect(screen.getByTestId("extension-content")).toBeInTheDocument();
  });

  it("loads extension immediately when already registered (happy path)", async () => {
    vi.mocked(loadExtension).mockResolvedValue({
      name: "oss",
      manifest: manifest as any,
      components: { BrowserPage: DummyComponent },
    });

    // Pre-register and set ready BEFORE render — simulates bootstrap completing before render
    useExtensionStore.getState().register("oss", manifest as any);
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="browser" assetId={42} />);

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    const content = screen.getByTestId("extension-content");
    expect(content).toBeInTheDocument();
    expect(content.textContent).toContain("asset=42");
  });

  it("uses cached loaded extension without re-loading", async () => {
    const loaded = {
      name: "oss",
      manifest: manifest as any,
      components: { BrowserPage: DummyComponent },
    };

    useExtensionStore.getState().register("oss", manifest as any);
    useExtensionStore.getState().setLoaded("oss", loaded);
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="browser" />);

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(loadExtension).not.toHaveBeenCalled();
    expect(screen.getByTestId("extension-content")).toBeInTheDocument();
  });

  it("shows error when page id not found in manifest", async () => {
    const loaded = {
      name: "oss",
      manifest: manifest as any,
      components: { BrowserPage: DummyComponent },
    };

    useExtensionStore.getState().register("oss", manifest as any);
    useExtensionStore.getState().setLoaded("oss", loaded);
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="nonexistent" />);

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.getByText(/not found in extension/)).toBeInTheDocument();
  });

  it("shows error when component not exported by extension", async () => {
    const loaded = {
      name: "oss",
      manifest: manifest as any,
      components: {},
    };

    useExtensionStore.getState().register("oss", manifest as any);
    useExtensionStore.getState().setLoaded("oss", loaded);
    useExtensionStore.getState().setReady(true);

    render(<ExtensionPage extensionName="oss" pageId="browser" />);

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.getByText(/not exported by extension/)).toBeInTheDocument();
  });
});
