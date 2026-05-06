import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@opskat/ui";
import { AssetTypeFilterButton } from "@/components/asset/AssetTypeFilterButton";
import { getAssetTypeOptions } from "@/lib/assetTypes/options";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, vars?: Record<string, unknown>) => (vars ? `${k}:${JSON.stringify(vars)}` : k),
  }),
}));

describe("AssetTypeFilterButton", () => {
  const builtinOpts = getAssetTypeOptions({});
  const builtinValues = builtinOpts.map((o) => o.value);

  beforeEach(() => {
    cleanup();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders without active dot when nothing is selected", async () => {
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={() => {}} />
      </TooltipProvider>
    );
    const btn = screen.getByRole("button", { name: /asset.filterByType/i });
    expect(btn).toBeTruthy();
    expect(btn.querySelector('[data-active="true"]')).toBeNull();
  });

  it("renders an active marker when partial selection", () => {
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={() => {}} />
      </TooltipProvider>
    );
    const btn = screen.getByRole("button", { name: /asset.filterByTypeActive/i });
    expect(btn.querySelector('[data-active="true"]')).not.toBeNull();
  });

  it("opens popover and lists built-in options", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={() => {}} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterAllTypes")).toBeTruthy();
    expect(screen.getByText("nav.ssh")).toBeTruthy();
    expect(screen.getByText("nav.database")).toBeTruthy();
    expect(screen.getByText("nav.redis")).toBeTruthy();
    expect(screen.getByText("nav.mongodb")).toBeTruthy();
    expect(screen.getByText("nav.kafka")).toBeTruthy();
    expect(screen.getByText("nav.k8s")).toBeTruthy();
  });

  it('clicking "All types" from empty selects every option', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={onChange} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    await user.click(screen.getByText("asset.filterAllTypes"));
    expect(onChange).toHaveBeenCalledWith(builtinValues);
  });

  it('clicking "All types" when everything is checked deselects all', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={builtinValues} options={builtinOpts} onChange={onChange} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("asset.filterAllTypes"));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("toggling a single type from empty selects only that type", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={onChange} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith(["ssh"]);
  });

  it("unchecking the only selected type leaves selection empty", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={["ssh"]} options={builtinOpts} onChange={onChange} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByTypeActive/i }));
    await user.click(screen.getByText("nav.ssh"));
    expect(onChange).toHaveBeenCalledWith([]);
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
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={opts} onChange={() => {}} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterExtensions")).toBeTruthy();
    expect(screen.getByText("Kubernetes")).toBeTruthy();
  });

  it("does not render Extensions section header when no extension options", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={() => {}} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.queryByText("asset.filterExtensions")).toBeNull();
  });

  it("does not render the hide-empty-folders toggle when its callback is omitted", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton value={[]} options={builtinOpts} onChange={() => {}} />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.queryByText("asset.filterHideEmptyGroups")).toBeNull();
  });

  it("renders the hide-empty-folders toggle and reflects current state", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton
          value={[]}
          options={builtinOpts}
          onChange={() => {}}
          hideEmptyGroups={true}
          onHideEmptyGroupsChange={() => {}}
        />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    expect(screen.getByText("asset.filterHideEmptyGroups")).toBeTruthy();
  });

  it("toggling hide-empty-folders flips the value via onHideEmptyGroupsChange", async () => {
    const user = userEvent.setup();
    const onHide = vi.fn();
    render(
      <TooltipProvider>
        <AssetTypeFilterButton
          value={[]}
          options={builtinOpts}
          onChange={() => {}}
          hideEmptyGroups={false}
          onHideEmptyGroupsChange={onHide}
        />
      </TooltipProvider>
    );
    await user.click(screen.getByRole("button", { name: /asset.filterByType/i }));
    await user.click(screen.getByText("asset.filterHideEmptyGroups"));
    expect(onHide).toHaveBeenCalledWith(true);
  });

  it("shows the active dot when only hide-empty-folders is on (no type selected)", () => {
    render(
      <TooltipProvider>
        <AssetTypeFilterButton
          value={[]}
          options={builtinOpts}
          onChange={() => {}}
          hideEmptyGroups={true}
          onHideEmptyGroupsChange={() => {}}
        />
      </TooltipProvider>
    );
    const btn = screen.getByRole("button");
    expect(btn.querySelector('[data-active="true"]')).not.toBeNull();
  });
});
