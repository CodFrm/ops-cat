/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@opskat/ui";
import { useSnippetStore } from "../../stores/snippetStore";
import { useAssetStore } from "../../stores/assetStore";
import { SnippetsPage } from "../../components/snippet/SnippetsPage";
import { ListSnippets, ListSnippetCategories, DuplicateSnippet } from "../../../wailsjs/go/app/App";

function renderPage() {
  return render(
    <TooltipProvider>
      <SnippetsPage />
    </TooltipProvider>
  );
}

describe("SnippetsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSnippetStore.setState({
      categories: [
        { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any,
        { id: "prompt", assetType: "", label: "Prompt", source: "builtin" } as any,
      ],
      categoriesLoading: false,
      list: [],
      listLoading: false,
      filter: { categories: [], keyword: "", tag: "" },
    });
    useAssetStore.setState({
      assets: [],
      groups: [],
      selectedAssetId: null,
      selectedGroupId: null,
      collapsedGroupIds: [],
      loading: false,
      initialized: true,
    });
    vi.mocked(ListSnippetCategories).mockResolvedValue([
      { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any,
      { id: "prompt", assetType: "", label: "Prompt", source: "builtin" } as any,
    ]);
    vi.mocked(ListSnippets).mockResolvedValue([]);
  });

  it("shows empty-state message when list is empty", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText("snippet.emptyState")).toBeInTheDocument());
  });

  it("renders table headers from i18n keys", async () => {
    renderPage();
    expect(screen.getByText("snippet.columns.category")).toBeInTheDocument();
    expect(screen.getByText("snippet.columns.name")).toBeInTheDocument();
    expect(screen.getByText("snippet.columns.updated")).toBeInTheDocument();
  });

  it("renders read-only lock icon for ext-sourced rows and disables Edit/Delete", async () => {
    const rows = [
      {
        ID: 1,
        Name: "user-row",
        Category: "shell",
        Content: "ls",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
        CreatedAt: "2024-01-01T00:00:00Z",
        UpdatedAt: "2024-01-01T00:00:00Z",
      },
      {
        ID: 2,
        Name: "ext-row",
        Category: "shell",
        Content: "ls",
        Description: "",
        Tags: "",
        Source: "ext:myext",
        SourceRef: "1",
        UseCount: 0,
        Status: 1,
        CreatedAt: "2024-01-01T00:00:00Z",
        UpdatedAt: "2024-01-01T00:00:00Z",
      },
    ];
    // Mount-triggered loadList() would otherwise clobber the seeded list with [].
    vi.mocked(ListSnippets).mockResolvedValue(rows as any);
    useSnippetStore.setState({ list: rows as any });
    renderPage();
    await waitFor(() => expect(screen.getByText("user-row")).toBeInTheDocument());

    // Lock icon only renders on ext-sourced row.
    const locks = screen.getAllByTestId("readonly-lock");
    expect(locks).toHaveLength(1);

    // Buttons: every row has edit + duplicate + delete. Ext row's edit & delete
    // are disabled; its duplicate remains enabled.
    const editButtons = screen.getAllByRole("button", { name: "snippet.actions.edit" });
    const deleteButtons = screen.getAllByRole("button", { name: "snippet.actions.delete" });
    const duplicateButtons = screen.getAllByRole("button", { name: "snippet.actions.duplicate" });
    expect(editButtons).toHaveLength(2);
    expect(deleteButtons).toHaveLength(2);
    expect(duplicateButtons).toHaveLength(2);
    // Row order matches list order.
    expect(editButtons[0]).not.toBeDisabled();
    expect(editButtons[1]).toBeDisabled();
    expect(deleteButtons[1]).toBeDisabled();
    expect(duplicateButtons[1]).not.toBeDisabled();
  });

  it("clicking New opens the form dialog in create mode", async () => {
    renderPage();
    const newBtn = screen.getByRole("button", { name: /snippet.newButton/ });
    fireEvent.click(newBtn);
    expect(await screen.findByText("snippet.form.createTitle")).toBeInTheDocument();
  });

  it("clicking Duplicate calls store.duplicate(id)", async () => {
    const rows = [
      {
        ID: 42,
        Name: "row",
        Category: "shell",
        Content: "ls",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
        CreatedAt: "2024-01-01T00:00:00Z",
        UpdatedAt: "2024-01-01T00:00:00Z",
      },
    ];
    vi.mocked(DuplicateSnippet).mockResolvedValue({ ID: 43 } as any);
    vi.mocked(ListSnippets).mockResolvedValue(rows as any);
    useSnippetStore.setState({ list: rows as any });

    renderPage();
    const dupBtn = await screen.findByRole("button", { name: "snippet.actions.duplicate" });
    fireEvent.click(dupBtn);
    await waitFor(() => expect(DuplicateSnippet).toHaveBeenCalledWith(42));
  });

  it("resolves asset name via assetStore", async () => {
    useAssetStore.setState({
      assets: [{ ID: 5, Name: "prod-db", Type: "database", GroupID: 0 } as any],
    });
    const seeded = [
      {
        ID: 1,
        Name: "q",
        Category: "shell",
        Content: "ls",
        Description: "",
        Tags: "",
        AssetID: 5,
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
        CreatedAt: "2024-01-01T00:00:00Z",
        UpdatedAt: "2024-01-01T00:00:00Z",
      },
    ];
    // Mount's loadList() would otherwise clobber the seeded list with [].
    vi.mocked(ListSnippets).mockResolvedValue(seeded as any);
    useSnippetStore.setState({ list: seeded as any });
    renderPage();
    expect(await screen.findByText("prod-db")).toBeInTheDocument();
  });
});
