import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AssetTypeFilterButton } from "@/components/asset/AssetTypeFilterButton";
import { getAssetTypeOptions } from "@/lib/assetTypes/options";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, vars?: Record<string, unknown>) => (vars ? `${k}:${JSON.stringify(vars)}` : k) }),
}));

describe("AssetTypeFilterButton", () => {
  const builtinOpts = getAssetTypeOptions({});

  beforeEach(() => {
    cleanup();
  });
  afterEach(() => {
    cleanup();
  });

  it('renders without active dot when value is "all"', async () => {
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    const btn = screen.getByRole("button", { name: /asset.filterByType/i });
    expect(btn).toBeTruthy();
    expect(btn.querySelector('[data-active="true"]')).toBeNull();
  });

  it("renders an active marker when partial selection", () => {
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={() => {}} />);
    const btn = screen.getByRole("button", { name: /asset.filterByTypeActive/i });
    expect(btn.querySelector('[data-active="true"]')).not.toBeNull();
  });

  it("opens popover and lists built-in options", async () => {
    const user = userEvent.setup();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterAllTypes")).toBeTruthy();
    expect(screen.getByText("nav.ssh")).toBeTruthy();
    expect(screen.getByText("nav.database")).toBeTruthy();
    expect(screen.getByText("nav.redis")).toBeTruthy();
    expect(screen.getByText("nav.mongodb")).toBeTruthy();
  });

  it('toggling "All types" while not all switches selection to "all"', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("asset.filterAllTypes"));
    expect(onChange).toHaveBeenCalledWith("all");
  });

  it("toggling a single type from all narrows selection to other 3", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith(["database", "redis", "mongodb"]);
  });

  it('unchecking the last selected type falls back to "all"', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith("all");
  });

  it("renders Extensions section header when extension options are present", async () => {
    const user = userEvent.setup();
    const opts = getAssetTypeOptions({
      k8sExt: {
        manifest: {
          name: "k8sExt",
          version: "1",
          icon: "Server",
          i18n: { displayName: "Kubernetes", description: "" },
          assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
        },
      },
    } as never);
    render(<AssetTypeFilterButton value="all" options={opts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterExtensions")).toBeTruthy();
    expect(screen.getByText("Kubernetes")).toBeTruthy();
  });

  it("does not render Extensions section header when no extension options", async () => {
    const user = userEvent.setup();
    render(<AssetTypeFilterButton value="all" options={builtinOpts} onChange={() => {}} />);
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.queryByText("asset.filterExtensions")).toBeNull();
  });
});
