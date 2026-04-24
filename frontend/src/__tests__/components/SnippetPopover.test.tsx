/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@opskat/ui";
import { useSnippetStore } from "../../stores/snippetStore";
import { useTabStore } from "../../stores/tabStore";
import { SnippetPopover } from "../../components/snippet/SnippetPopover";
import { ListSnippets, ListSnippetCategories, RecordSnippetUse } from "../../../wailsjs/go/app/App";

function makeSnippet(partial: Partial<any>): any {
  return {
    ID: 0,
    Name: "snippet",
    Category: "shell",
    Content: "echo hello",
    Description: "",
    LastAssetIDs: "",
    Source: "user",
    SourceRef: "",
    UseCount: 0,
    Status: 1,
    CreatedAt: "2024-01-01T00:00:00Z",
    UpdatedAt: "2024-01-01T00:00:00Z",
    ...partial,
  };
}

function renderPopover(overrides: Partial<React.ComponentProps<typeof SnippetPopover>> = {}) {
  const onInsert = overrides.onInsert ?? vi.fn();
  const utils = render(
    <TooltipProvider>
      <SnippetPopover
        category="shell"
        onInsert={onInsert}
        trigger={<button type="button">open</button>}
        {...overrides}
      />
    </TooltipProvider>
  );
  return { ...utils, onInsert };
}

describe("SnippetPopover", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSnippetStore.setState({
      categories: [{ id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any],
      categoriesLoading: false,
      list: [],
      listLoading: false,
      filter: { categories: [], keyword: "" },
    });
    useTabStore.setState({ tabs: [], activeTabId: null });
    vi.mocked(ListSnippetCategories).mockResolvedValue([
      { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any,
    ]);
    vi.mocked(ListSnippets).mockResolvedValue([
      makeSnippet({ ID: 1, Name: "list files", Content: "ls -la" }),
      makeSnippet({ ID: 2, Name: "disk usage", Content: "df -h" }),
      makeSnippet({ ID: 3, Name: "memory info", Content: "free -m" }),
    ]);
  });

  it("fetches + renders snippets on open", async () => {
    renderPopover();
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(ListSnippets).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText("list files")).toBeInTheDocument());
    expect(screen.getByText("disk usage")).toBeInTheDocument();
    expect(screen.getByText("memory info")).toBeInTheDocument();
  });

  it("narrows list when typing in search", async () => {
    renderPopover();
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(screen.getByText("list files")).toBeInTheDocument());

    const search = screen.getByPlaceholderText("snippet.popover.searchPlaceholder");
    fireEvent.change(search, { target: { value: "disk" } });

    await waitFor(() => {
      expect(screen.queryByText("list files")).not.toBeInTheDocument();
      expect(screen.getByText("disk usage")).toBeInTheDocument();
      expect(screen.queryByText("memory info")).not.toBeInTheDocument();
    });
  });

  it("inserts without enter on row click and records usage", async () => {
    const onInsert = vi.fn();
    renderPopover({ onInsert });
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(screen.getByText("list files")).toBeInTheDocument());

    fireEvent.click(screen.getByText("list files"));

    expect(onInsert).toHaveBeenCalledWith("ls -la", { withEnter: false });
    await waitFor(() => expect(RecordSnippetUse).toHaveBeenCalledWith(1));
  });

  it("shows 'Insert + Enter' button when showSendWithEnter=true and passes withEnter=true", async () => {
    const onInsert = vi.fn();
    renderPopover({ onInsert, showSendWithEnter: true });
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(screen.getByText("list files")).toBeInTheDocument());

    const enterButtons = screen.getAllByTestId("snippet-popover-row-enter");
    expect(enterButtons.length).toBe(3);
    fireEvent.click(enterButtons[0]);

    expect(onInsert).toHaveBeenCalledWith("ls -la", { withEnter: true });
    await waitFor(() => expect(RecordSnippetUse).toHaveBeenCalledWith(1));
  });

  it("shows empty state with a link that opens the Snippets tab", async () => {
    vi.mocked(ListSnippets).mockResolvedValue([]);
    renderPopover();
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(screen.getByText("snippet.popover.empty")).toBeInTheDocument());

    const openBtn = screen.getByText("snippet.popover.openManager");
    fireEvent.click(openBtn);

    const tabs = useTabStore.getState().tabs;
    expect(tabs.length).toBe(1);
    expect(tabs[0].id).toBe("snippets");
    expect(tabs[0].type).toBe("page");
  });

  it("re-fetches when category prop changes", async () => {
    const { rerender } = render(
      <TooltipProvider>
        <SnippetPopover category="shell" onInsert={vi.fn()} trigger={<button>open</button>} />
      </TooltipProvider>
    );
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(ListSnippets).toHaveBeenCalledTimes(1));

    // Close popover, change category, reopen
    fireEvent.keyDown(document.body, { key: "Escape" });
    rerender(
      <TooltipProvider>
        <SnippetPopover category="sql" onInsert={vi.fn()} trigger={<button>open</button>} />
      </TooltipProvider>
    );
    fireEvent.click(screen.getByText("open"));

    await waitFor(() => expect(ListSnippets).toHaveBeenCalledTimes(2));
    const secondCall = vi.mocked(ListSnippets).mock.calls[1][0] as any;
    expect(secondCall.categories).toEqual(["sql"]);
  });

  it("fetches snippets without assetId or includeGlobal fields", async () => {
    renderPopover();
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(ListSnippets).toHaveBeenCalledTimes(1));
    const req = vi.mocked(ListSnippets).mock.calls[0][0] as any;
    expect(req.categories).toEqual(["shell"]);
    expect(req.assetId).toBeUndefined();
    expect(req.includeGlobal).toBeUndefined();
  });
});
