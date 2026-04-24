/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@opskat/ui";
import { useSnippetStore } from "../../stores/snippetStore";
import { SnippetsPage } from "../../components/snippet/SnippetsPage";
import { ListSnippets, ListSnippetCategories, DuplicateSnippet } from "../../../wailsjs/go/app/App";

// Monaco editor is a heavy dependency; stub it out so the form dialog renders
// without a real Monaco instance in happy-dom.
vi.mock("@/components/CodeEditor", () => ({
  CodeEditor: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <textarea aria-label="snippet.form.labelContent" value={value ?? ""} onChange={(e) => onChange(e.target.value)} />
  ),
}));

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
      filter: { categories: [], keyword: "" },
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

  it("does not render asset or tags column headers", () => {
    renderPage();
    expect(screen.queryByText("snippet.columns.asset")).toBeNull();
    expect(screen.queryByText("snippet.columns.tags")).toBeNull();
  });

  it("renders read-only lock icon for ext-sourced rows and disables Edit/Delete", async () => {
    const rows = [
      {
        ID: 1,
        Name: "user-row",
        Category: "shell",
        Content: "ls",
        Description: "",
        LastAssetIDs: "",
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
        LastAssetIDs: "",
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
        LastAssetIDs: "",
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

  it("renders orphaned category badge and exposes it in the filter dropdown", async () => {
    // Registered categories only include built-ins; list contains a snippet whose
    // Category "kafka" is unknown (extension providing it was uninstalled).
    const rows = [
      {
        ID: 1,
        Name: "orphaned-one",
        Category: "kafka",
        Content: "kafka-topics --list",
        Description: "",
        LastAssetIDs: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
        CreatedAt: "2024-01-01T00:00:00Z",
        UpdatedAt: "2024-01-01T00:00:00Z",
      },
    ];
    vi.mocked(ListSnippets).mockResolvedValue(rows as any);
    useSnippetStore.setState({ list: rows as any });

    renderPage();
    await waitFor(() => expect(screen.getByText("orphaned-one")).toBeInTheDocument());

    // With the stubbed t() returning the key verbatim (no interpolation), the
    // orphan badge renders the literal i18n key. The cell shows that — prior
    // to this fix it would have shown the raw Category string "kafka".
    const badges = screen.getAllByText("snippet.unknownCategory");
    expect(badges.length).toBeGreaterThan(0);
    // The tooltip-providing title attribute carries the i18n key as well.
    const badgeEl = badges[0];
    expect(badgeEl.getAttribute("title")).toBe("snippet.unknownCategoryTooltip");

    // Open the category filter dropdown and assert the orphan row appears.
    const filterBtn = screen.getByRole("button", { name: /snippet.allCategories/ });
    fireEvent.click(filterBtn);
    // Registered (shell, prompt) + orphan (kafka) => 3 checkboxes.
    await waitFor(() => {
      const checkboxes = screen.getAllByRole("checkbox");
      expect(checkboxes.length).toBe(3);
    });
  });
});
