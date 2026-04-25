import { beforeEach, describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryResultTable, cellValueToText } from "@/components/query/QueryResultTable";

const { toastError, toastSuccess } = vi.hoisted(() => ({
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    error: toastError,
    success: toastSuccess,
  },
}));

const writeText = vi.fn();
const readText = vi.fn();

beforeEach(() => {
  writeText.mockReset();
  readText.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: {
      writeText,
      readText,
    },
  });
});

describe("cellValueToText", () => {
  it("null / undefined → empty string", () => {
    expect(cellValueToText(null)).toBe("");
    expect(cellValueToText(undefined)).toBe("");
  });

  it("primitive values use String()", () => {
    expect(cellValueToText("hello")).toBe("hello");
    expect(cellValueToText(42)).toBe("42");
    expect(cellValueToText(0)).toBe("0");
    expect(cellValueToText(false)).toBe("false");
    expect(cellValueToText(true)).toBe("true");
  });

  it("objects serialize as JSON (not [object Object])", () => {
    expect(cellValueToText({ $oid: "65ae19fba4255225f0f38a59" })).toBe('{"$oid":"65ae19fba4255225f0f38a59"}');
    expect(cellValueToText({ a: 1, b: "x" })).toBe('{"a":1,"b":"x"}');
  });

  it("arrays serialize as JSON", () => {
    expect(cellValueToText([1, 2, 3])).toBe("[1,2,3]");
    expect(cellValueToText([{ k: 1 }])).toBe('[{"k":1}]');
  });

  it("circular references fall back to String() without throwing", () => {
    const obj: Record<string, unknown> = { a: 1 };
    obj.self = obj;
    // Should not throw; falls back to String(v) = "[object Object]"
    expect(() => cellValueToText(obj)).not.toThrow();
    expect(cellValueToText(obj)).toBe("[object Object]");
  });
});

describe("QueryResultTable — object cell values", () => {
  const columns = ["_id", "name"];
  const rows = [
    { _id: { $oid: "65ae19fba4255225f0f38a59" }, name: "alice" },
    { _id: { $oid: "65ae19fba4255225f0f38a60" }, name: "bob" },
    { _id: { $oid: "65ae19fba4255225f0f38a59" }, name: "carol" },
  ];

  it("cell tooltip (td title) shows JSON, not [object Object]", () => {
    render(<QueryResultTable columns={columns} rows={rows} />);
    const cells = document.querySelectorAll("td[data-cell-key]");
    // Row 0, col _id
    const row0Id = Array.from(cells).find((c) => c.getAttribute("data-cell-key") === "0:_id")!;
    expect(row0Id.getAttribute("title")).toBe('{"$oid":"65ae19fba4255225f0f38a59"}');
    expect(row0Id.getAttribute("title")).not.toContain("[object Object]");
  });

  it("default cell rendering shows JSON, not [object Object]", () => {
    render(<QueryResultTable columns={columns} rows={rows} />);
    // With no custom renderCell, the default td should render the JSON string
    expect(screen.queryByText("[object Object]")).toBeNull();
    // The JSON form shows up (3 rows, 2 distinct values, so 3 total cells)
    expect(screen.getAllByText('{"$oid":"65ae19fba4255225f0f38a59"}')).toHaveLength(2);
    expect(screen.getAllByText('{"$oid":"65ae19fba4255225f0f38a60"}')).toHaveLength(1);
  });

  it("filter popover renders object values as JSON labels", async () => {
    const user = userEvent.setup();
    render(<QueryResultTable columns={columns} rows={rows} enableColumnFilter />);

    const filterButtons = screen.getAllByTitle("query.filterColumn");
    await user.click(filterButtons[0]);

    // Scope assertions to the popover panel — the raw JSON string appears in
    // td cells too, but we specifically care about the filter list here.
    const popover = await screen.findByRole("dialog");
    expect(within(popover).getByText('{"$oid":"65ae19fba4255225f0f38a59"}')).toBeInTheDocument();
    expect(within(popover).getByText('{"$oid":"65ae19fba4255225f0f38a60"}')).toBeInTheDocument();
    expect(within(popover).queryByText("[object Object]")).toBeNull();
  });

  it("filter popover dedupes equal objects by JSON key and shows counts", async () => {
    const user = userEvent.setup();
    render(<QueryResultTable columns={columns} rows={rows} enableColumnFilter />);
    const filterButtons = screen.getAllByTitle("query.filterColumn");
    await user.click(filterButtons[0]);

    const popover = await screen.findByRole("dialog");
    const a59Label = within(popover).getByText('{"$oid":"65ae19fba4255225f0f38a59"}');
    const row = a59Label.closest("label")!;
    const countSpan = row.querySelector("span.tabular-nums")!;
    expect(countSpan.textContent).toBe("2");
  });
});

describe("QueryResultTable — cell context actions", () => {
  const columns = ["id", "name"];
  const rows = [
    { id: 1, name: "alice" },
    { id: 2, name: "bob" },
  ];

  function openMenu(props: Partial<React.ComponentProps<typeof QueryResultTable>> = {}) {
    render(<QueryResultTable columns={columns} rows={rows} editable {...props} />);
    const cell = document.querySelector('[data-cell-key="1:name"]') as HTMLElement;
    fireEvent.contextMenu(cell, { clientX: 40, clientY: 50 });
  }

  it("shows the table cell context actions", () => {
    openMenu({ onSetCellValue: vi.fn(), onPasteCell: vi.fn(), onRefresh: vi.fn() });

    expect(screen.getByText("query.setEmptyString")).toBeInTheDocument();
    expect(screen.getByText("query.setNull")).toBeInTheDocument();
    expect(screen.getByText("query.copyValue")).toBeInTheDocument();
    expect(screen.getByText("query.copyFieldName")).toBeInTheDocument();
    expect(screen.getByText("query.pasteValue")).toBeInTheDocument();
    expect(screen.getByText("query.refreshTable")).toBeInTheDocument();
  });

  it("hides edit and refresh actions when the table has no matching capability", () => {
    openMenu({ editable: false });

    expect(screen.queryByText("query.setEmptyString")).not.toBeInTheDocument();
    expect(screen.queryByText("query.setNull")).not.toBeInTheDocument();
    expect(screen.queryByText("query.pasteValue")).not.toBeInTheDocument();
    expect(screen.queryByText("query.refreshTable")).not.toBeInTheDocument();
    expect(screen.getByText("query.copyValue")).toBeInTheDocument();
    expect(screen.getByText("query.copyFieldName")).toBeInTheDocument();
  });

  it("set NULL creates an edit for the right cell", async () => {
    const user = userEvent.setup();
    const onSetCellValue = vi.fn();
    openMenu({ onSetCellValue });

    await user.click(screen.getByText("query.setNull"));

    expect(onSetCellValue).toHaveBeenCalledWith({ rowIdx: 1, col: "name", value: null });
  });

  it("set empty string creates an edit for the right cell", async () => {
    const user = userEvent.setup();
    const onSetCellValue = vi.fn();
    openMenu({ onSetCellValue });

    await user.click(screen.getByText("query.setEmptyString"));

    expect(onSetCellValue).toHaveBeenCalledWith({ rowIdx: 1, col: "name", value: "" });
  });

  it("copy field name writes the current column name to clipboard", async () => {
    openMenu();

    fireEvent.click(screen.getByText("query.copyFieldName"));

    expect(writeText).toHaveBeenCalledWith("name");
  });

  it("paste reads clipboard text and creates an edit for the right cell", async () => {
    const onPasteCell = vi.fn();
    readText.mockResolvedValue("clipboard text");
    openMenu({ onPasteCell });

    fireEvent.click(screen.getByText("query.pasteValue"));

    await waitFor(() => expect(readText).toHaveBeenCalledOnce());
    expect(onPasteCell).toHaveBeenCalledWith({ rowIdx: 1, col: "name", value: "clipboard text" });
  });

  it("paste clipboard read failure does not create an edit and closes the menu", async () => {
    const onPasteCell = vi.fn();
    readText.mockRejectedValue(new Error("clipboard denied"));
    openMenu({ onPasteCell });

    fireEvent.click(screen.getByText("query.pasteValue"));

    await waitFor(() => expect(readText).toHaveBeenCalledOnce());
    expect(onPasteCell).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Error: clipboard denied");
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  });

  it("refresh invokes the refresh callback", async () => {
    const user = userEvent.setup();
    const onRefresh = vi.fn();
    openMenu({ onRefresh });

    await user.click(screen.getByText("query.refreshTable"));

    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("filter by cell value invokes the filter callback with the current cell context", async () => {
    const user = userEvent.setup();
    const onFilterByCellValue = vi.fn();
    openMenu({ onFilterByCellValue });

    await user.click(screen.getByText("query.filterByCellValue"));

    expect(onFilterByCellValue).toHaveBeenCalledWith({ rowIdx: 1, col: "name", value: "bob" });
  });

  it("sort context actions invoke the sort callback for the current column", async () => {
    const user = userEvent.setup();
    const onSortByColumn = vi.fn();
    openMenu({ onSortByColumn });

    await user.click(screen.getByText("query.sortAscending"));
    expect(onSortByColumn).toHaveBeenCalledWith("name", "asc");

    openMenu({ onSortByColumn });
    await user.click(screen.getByText("query.sortDescending"));
    expect(onSortByColumn).toHaveBeenCalledWith("name", "desc");
  });

  it("clear filter and sort invokes the clear callback", async () => {
    const user = userEvent.setup();
    const onClearFilterSort = vi.fn();
    openMenu({ onClearFilterSort });

    await user.click(screen.getByText("query.clearFilterSort"));

    expect(onClearFilterSort).toHaveBeenCalledOnce();
  });

  it("delete record invokes the delete callback with the current row", async () => {
    const user = userEvent.setup();
    const onDeleteRow = vi.fn();
    openMenu({ onDeleteRow });

    await user.click(screen.getByText("query.deleteRecord"));

    expect(onDeleteRow).toHaveBeenCalledWith(1);
  });

  it("generate UUID creates an edit for the current cell", async () => {
    const user = userEvent.setup();
    const onGenerateUuid = vi.fn();
    const randomUUID = vi.spyOn(crypto, "randomUUID").mockReturnValue("00000000-0000-4000-8000-000000000000");
    openMenu({ onGenerateUuid });

    await user.click(screen.getByText("query.generateUuid"));

    expect(onGenerateUuid).toHaveBeenCalledWith({
      rowIdx: 1,
      col: "name",
      value: "00000000-0000-4000-8000-000000000000",
    });
    randomUUID.mockRestore();
  });

  it("copy as actions pass the current cell context and requested format", async () => {
    const user = userEvent.setup();
    const onCopyAs = vi.fn();
    openMenu({ onCopyAs });

    await user.click(screen.getByText("query.copyAsInsert"));
    openMenu({ onCopyAs });
    await user.click(screen.getByText("query.copyAsUpdate"));
    openMenu({ onCopyAs });
    await user.click(screen.getByText("query.copyAsTsvData"));
    openMenu({ onCopyAs });
    await user.click(screen.getByText("query.copyAsTsvFields"));
    openMenu({ onCopyAs });
    await user.click(screen.getByText("query.copyAsTsvFieldsAndData"));

    expect(onCopyAs).toHaveBeenNthCalledWith(1, "insert", { rowIdx: 1, col: "name", value: "bob" });
    expect(onCopyAs).toHaveBeenNthCalledWith(2, "update", { rowIdx: 1, col: "name", value: "bob" });
    expect(onCopyAs).toHaveBeenNthCalledWith(3, "tsv-data", { rowIdx: 1, col: "name", value: "bob" });
    expect(onCopyAs).toHaveBeenNthCalledWith(4, "tsv-fields", { rowIdx: 1, col: "name", value: "bob" });
    expect(onCopyAs).toHaveBeenNthCalledWith(5, "tsv-fields-data", { rowIdx: 1, col: "name", value: "bob" });
  });

  it("renders only visible columns", () => {
    render(<QueryResultTable columns={columns} rows={rows} visibleColumns={["name"]} />);

    expect(screen.getByText("name")).toBeInTheDocument();
    expect(screen.queryByText("id")).not.toBeInTheDocument();
    expect(document.querySelector('[data-cell-key="0:id"]')).toBeNull();
    expect(document.querySelector('[data-cell-key="0:name"]')).toBeInTheDocument();
  });

  it("applies row density classes", () => {
    const { rerender } = render(<QueryResultTable columns={columns} rows={rows} rowDensity="compact" />);
    expect(document.querySelector('[data-cell-key="0:name"]')).toHaveClass("py-0.5");

    rerender(<QueryResultTable columns={columns} rows={rows} rowDensity="comfortable" />);
    expect(document.querySelector('[data-cell-key="0:name"]')).toHaveClass("py-2");
  });
});
