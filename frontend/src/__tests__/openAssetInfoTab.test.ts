import { describe, it, expect, beforeEach, vi } from "vitest";
import { openAssetInfoTab } from "@/lib/openAssetInfoTab";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";
import { toast } from "sonner";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

describe("openAssetInfoTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAssetStore.setState({
      assets: [{ ID: 42, Name: "prod-db", Type: "mysql", Icon: "mysql" } as any],
      groups: [],
    } as any);
    useTabStore.setState({ tabs: [], activeTabId: null } as any);
  });

  it("资产存在时打开 info-asset-{id} tab", () => {
    openAssetInfoTab(42);
    const tabs = useTabStore.getState().tabs;
    expect(tabs.find((t) => t.id === "info-asset-42")).toBeTruthy();
  });

  it("同 id 二次调用不重复开 tab，仅激活", () => {
    openAssetInfoTab(42);
    openAssetInfoTab(42);
    const tabs = useTabStore.getState().tabs;
    expect(tabs.filter((t) => t.id === "info-asset-42")).toHaveLength(1);
    expect(useTabStore.getState().activeTabId).toBe("info-asset-42");
  });

  it("资产不存在时 toast 提示不开 tab", () => {
    openAssetInfoTab(999);
    expect(toast.error).toHaveBeenCalledWith("资产已删除");
    expect(useTabStore.getState().tabs).toHaveLength(0);
  });
});
