import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SnippetAssetDrawer } from "../SnippetAssetDrawer";

vi.mock("../../../../wailsjs/go/app/App", () => ({
  GetSnippetLastAssets: vi.fn().mockResolvedValue([1]),
  SetSnippetLastAssets: vi.fn().mockResolvedValue(undefined),
  RecordSnippetUse: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../snippetRun", () => ({
  runSnippetOnAsset: vi.fn().mockResolvedValue(undefined),
}));

const assets = [
  { ID: 1, Name: "prod-mysql", Type: "database", Status: 1, Config: "{}" },
  { ID: 2, Name: "staging-mysql", Type: "database", Status: 1, Config: "{}" },
  { ID: 3, Name: "a-ssh", Type: "ssh", Status: 1, Config: "{}" },
];

vi.mock("@/stores/assetStore", () => ({
  useAssetStore: (selector: (s: unknown) => unknown) => selector({ assets }),
}));

vi.mock("@/stores/snippetStore", () => ({
  useSnippetStore: (selector: (s: unknown) => unknown) =>
    selector({
      categories: [
        { id: "sql", assetType: "database", label: "SQL", source: "builtin" },
        { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" },
      ],
    }),
}));

describe("SnippetAssetDrawer", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows only assets matching the snippet category's assetType", async () => {
    const snippet = { ID: 10, Name: "s", Category: "sql", Content: "SELECT 1" };
    render(<SnippetAssetDrawer snippet={snippet as never} onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("prod-mysql")).toBeInTheDocument());
    expect(screen.getByText("staging-mysql")).toBeInTheDocument();
    expect(screen.queryByText("a-ssh")).not.toBeInTheDocument();
  });

  it("pre-checks the assets returned by GetSnippetLastAssets", async () => {
    const snippet = { ID: 10, Name: "s", Category: "sql", Content: "SELECT 1" };
    render(<SnippetAssetDrawer snippet={snippet as never} onClose={() => {}} />);
    const checkbox = await screen.findByRole("checkbox", { name: /prod-mysql/i });
    await waitFor(() => expect(checkbox).toBeChecked());
  });

  it("disables Run when selection is empty", async () => {
    const snippet = { ID: 10, Name: "s", Category: "sql", Content: "SELECT 1" };
    render(<SnippetAssetDrawer snippet={snippet as never} onClose={() => {}} />);
    const user = userEvent.setup();
    const preChecked = await screen.findByRole("checkbox", { name: /prod-mysql/i });
    await waitFor(() => expect(preChecked).toBeChecked());
    await user.click(preChecked);
    const runBtn = screen.getByRole("button", { name: /run/i });
    expect(runBtn).toBeDisabled();
  });
});
