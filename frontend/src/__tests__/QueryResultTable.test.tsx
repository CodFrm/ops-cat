import { beforeEach, describe, it, expect, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryResultTable } from "@/components/query/QueryResultTable";
import { cellValueToText } from "@/lib/cellValue";

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

  function openRowMenu(props: Partial<React.ComponentProps<typeof QueryResultTable>> = {}) {
    render(<QueryResultTable columns={columns} rows={rows} editable showRowNumber {...props} />);
    const rowHeader = document.querySelector('[data-row-header-key="1"]') as HTMLElement;
    fireEvent.contextMenu(rowHeader, { clientX: 20, clientY: 40 });
  }

  function openColumnMenu(props: Partial<React.ComponentProps<typeof QueryResultTable>> = {}) {
    render(
      <QueryResultTable
        columns={columns}
        rows={rows}
        editable
        columnTypes={{ id: "int", name: "varchar(128)" }}
        {...props}
      />
    );
    const header = document.querySelector('[data-column-header-key="name"]') as HTMLElement;
    fireEvent.contextMenu(header, { clientX: 40, clientY: 20 });
  }

  function openColumnMoreMenu(props: Partial<React.ComponentProps<typeof QueryResultTable>> = {}) {
    cleanup();
    render(
      <QueryResultTable
        columns={columns}
        rows={rows}
        editable
        columnTypes={{ id: "int", name: "varchar(128)" }}
        {...props}
      />
    );
    fireEvent.click(screen.getByTitle("query.columnActions:name"));
  }

  it("shows field types under column names", () => {
    render(<QueryResultTable columns={columns} rows={rows} columnTypes={{ id: "int", name: "varchar(128)" }} />);

    expect(screen.getByText("int")).toBeInTheDocument();
    expect(screen.getByText("varchar(128)")).toBeInTheDocument();
  });

  it("left-clicking a column header selects the full column", () => {
    render(<QueryResultTable columns={columns} rows={rows} columnTypes={{ id: "int", name: "varchar(128)" }} />);
    fireEvent.click(screen.getByText("name"));

    const selected = document.querySelectorAll('[data-column-selected="name"]');
    expect(selected).toHaveLength(rows.length + 1);
    expect(document.querySelector('[data-row-selected="true"]')).toBeNull();
  });

  it("shift-clicking a column header selects a continuous column range", () => {
    render(
      <QueryResultTable
        columns={["id", "name", "email"]}
        rows={[
          { id: 1, name: "alice", email: "a@example.com" },
          { id: 2, name: "bob", email: "b@example.com" },
        ]}
      />
    );

    fireEvent.click(screen.getByText("id"));
    fireEvent.click(screen.getByText("email"), { shiftKey: true });

    expect(document.querySelectorAll('[data-column-selected="id"]')).toHaveLength(3);
    expect(document.querySelectorAll('[data-column-selected="name"]')).toHaveLength(3);
    expect(document.querySelectorAll('[data-column-selected="email"]')).toHaveLength(3);
  });

  it("column more menu invokes sort, clear sort, and add filter actions", async () => {
    const user = userEvent.setup();
    const onSortByColumn = vi.fn();
    const onClearFilterSort = vi.fn();
    const onAddColumnFilter = vi.fn();
    openColumnMoreMenu({ onSortByColumn, onClearFilterSort, onAddColumnFilter });

    await user.click(screen.getByText("query.sortAsc"));
    expect(onSortByColumn).toHaveBeenCalledWith("name", "asc");

    openColumnMoreMenu({ onSortByColumn, onClearFilterSort, onAddColumnFilter });
    await user.click(screen.getByText("query.sortDesc"));
    expect(onSortByColumn).toHaveBeenCalledWith("name", "desc");

    openColumnMoreMenu({ onSortByColumn, onClearFilterSort, onAddColumnFilter });
    await user.click(screen.getByText("query.removeAllSorts"));
    expect(onClearFilterSort).toHaveBeenCalledOnce();

    openColumnMoreMenu({ onSortByColumn, onClearFilterSort, onAddColumnFilter });
    await user.click(screen.getByText("query.addFilter"));
    expect(onAddColumnFilter).toHaveBeenCalledWith("name");
  });

  it("header filter button adds a server-side filter when a handler is provided", async () => {
    const user = userEvent.setup();
    const onAddColumnFilter = vi.fn();

    render(<QueryResultTable columns={columns} rows={rows} enableColumnFilter onAddColumnFilter={onAddColumnFilter} />);

    await user.click(screen.getAllByTitle("query.filterColumn")[0]);

    expect(onAddColumnFilter).toHaveBeenCalledWith("id");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("right-clicking a column header shows column actions instead of cell actions", () => {
    openColumnMenu({ onCopyAs: vi.fn(), onHideColumn: vi.fn() });

    expect(screen.getByText("query.copyValue")).toBeInTheDocument();
    expect(screen.getByText("query.copyFieldName")).toBeInTheDocument();
    expect(screen.getByText("query.copyAs")).toBeInTheDocument();
    expect(screen.getByText("query.showHideColumns")).toBeInTheDocument();
    expect(screen.getByText("query.hideColumn")).toBeInTheDocument();
    expect(screen.getByText("query.freezeColumn")).toBeInTheDocument();
    expect(screen.getByText("query.setColumnWidth")).toBeInTheDocument();
    expect(screen.getByText("query.sizeColumnToFit")).toBeInTheDocument();
    expect(screen.getByText("query.sizeAllColumnsToFit")).toBeInTheDocument();
    expect(screen.getByText("query.showFieldType")).toBeInTheDocument();
    expect(screen.getByText("query.showComment")).toBeInTheDocument();
    expect(screen.queryByText("query.setNull")).not.toBeInTheDocument();
    expect(screen.queryByText("query.pasteValue")).not.toBeInTheDocument();
    expect(screen.queryByText("query.filterByCellValue")).not.toBeInTheDocument();
  });

  it("sets a column width from the column context menu", async () => {
    const prompt = vi.fn().mockReturnValue("220");
    Object.defineProperty(window, "prompt", { configurable: true, value: prompt });
    openColumnMenu({ onHideColumn: vi.fn() });

    fireEvent.click(screen.getByText("query.setColumnWidth"));

    const header = document.querySelector('[data-column-header-key="name"]') as HTMLElement;
    expect(header.style.width).toBe("220px");
  });

  it("uses stable default column widths when freezing columns", async () => {
    const user = userEvent.setup();
    render(<QueryResultTable columns={columns} rows={rows} editable showRowNumber />);
    fireEvent.contextMenu(document.querySelector('[data-column-header-key="id"]') as HTMLElement, {
      clientX: 40,
      clientY: 20,
    });

    await user.click(screen.getByText("query.freezeColumn"));

    const idHeader = document.querySelector('[data-column-header-key="id"]') as HTMLElement;
    const nameHeader = document.querySelector('[data-column-header-key="name"]') as HTMLElement;
    const idCell = document.querySelector('[data-cell-key="0:id"]') as HTMLElement;

    expect(idCell).toHaveClass("sticky");
    expect(idHeader.style.width).toBe("160px");
    expect(idHeader.style.minWidth).toBe("160px");
    expect(idHeader.style.maxWidth).toBe("160px");
    expect(idCell.style.width).toBe("160px");
    expect(idCell.style.minWidth).toBe("160px");
    expect(idCell.style.maxWidth).toBe("160px");
    expect(idCell.style.left).toBe("50px");
    expect(idHeader.style.left).toBe("50px");
    expect(nameHeader).not.toHaveClass("sticky");
    expect(nameHeader.style.left).toBe("");
  });

  it("keeps frozen record cells sticky when selected", async () => {
    const user = userEvent.setup();
    render(<QueryResultTable columns={columns} rows={rows} editable showRowNumber />);
    fireEvent.contextMenu(document.querySelector('[data-column-header-key="id"]') as HTMLElement, {
      clientX: 40,
      clientY: 20,
    });

    await user.click(screen.getByText("query.freezeColumn"));
    const idCell = document.querySelector('[data-cell-key="0:id"]') as HTMLElement;
    await user.click(idCell);

    expect(idCell).toHaveClass("sticky");
    expect(idCell).not.toHaveClass("relative");
    expect(idCell.style.left).toBe("50px");
  });

  it("can freeze another column after freezing the first without freezing columns between them", async () => {
    const user = userEvent.setup();
    render(<QueryResultTable columns={["a", "b", "c"]} rows={[{ a: 1, b: 2, c: 3 }]} editable showRowNumber />);

    fireEvent.contextMenu(document.querySelector('[data-column-header-key="a"]') as HTMLElement, {
      clientX: 40,
      clientY: 20,
    });
    await user.click(screen.getByText("query.freezeColumn"));
    fireEvent.contextMenu(document.querySelector('[data-column-header-key="c"]') as HTMLElement, {
      clientX: 120,
      clientY: 20,
    });
    await user.click(screen.getByText("query.freezeColumn"));

    const aCell = document.querySelector('[data-cell-key="0:a"]') as HTMLElement;
    const bCell = document.querySelector('[data-cell-key="0:b"]') as HTMLElement;
    const cCell = document.querySelector('[data-cell-key="0:c"]') as HTMLElement;

    expect(aCell).toHaveClass("sticky");
    expect(aCell.style.left).toBe("50px");
    expect(bCell).not.toHaveClass("sticky");
    expect(bCell.style.left).toBe("");
    expect(cCell).toHaveClass("sticky");
    expect(cCell.style.left).toBe("210px");
  });

  it("shows the table cell context actions", () => {
    openMenu({ onSetCellValue: vi.fn(), onPasteCell: vi.fn(), onRefresh: vi.fn() });

    expect(screen.getByText("query.setEmptyString")).toBeInTheDocument();
    expect(screen.getByText("query.setNull")).toBeInTheDocument();
    expect(screen.getByText("query.copyValue")).toBeInTheDocument();
    expect(screen.getByText("query.copyFieldName")).toBeInTheDocument();
    expect(screen.getByText("query.pasteValue")).toBeInTheDocument();
    expect(screen.getByText("query.refreshTable")).toBeInTheDocument();
  });

  it("keeps copy and copy as next to each other in the context menu", () => {
    openMenu({ onCopyAs: vi.fn(), onPasteCell: vi.fn() });

    const menu = screen.getByRole("menu");
    const directItems = Array.from(menu.children);
    const labels = directItems
      .map((child) => {
        // Submenu containers flatten all nested text — extract just the trigger label
        const btn = child.querySelector(":scope > button");
        if (btn) return btn.textContent?.trim();
        return child.textContent?.trim();
      })
      .filter(Boolean);

    const copyIndex = labels.indexOf("query.copyValue");
    expect(labels[copyIndex + 1]).toBe("query.copyAs");
  });

  it("does not clip the copy as submenu outside the context menu", () => {
    openMenu({ onCopyAs: vi.fn() });

    expect(screen.getByRole("menu")).not.toHaveClass("overflow-hidden");
  });

  it("does not clip the filter submenu outside the context menu", async () => {
    const user = userEvent.setup();
    openMenu({ onFilterByCellValue: vi.fn(), onClearFilterSort: vi.fn(), onAddColumnFilter: vi.fn() });

    await user.hover(screen.getByText("query.filter"));

    expect(screen.getByRole("menu")).not.toHaveClass("overflow-hidden");
    expect(screen.getByText("query.filterOperatorIsNot")).toBeVisible();
  });

  it("right-clicking a row number selects the full row and shows row actions only", () => {
    openRowMenu({ onDeleteRow: vi.fn(), onCopyAs: vi.fn(), onFilterByCellValue: vi.fn(), onSortByColumn: vi.fn() });

    const selectedCells = document.querySelectorAll('[data-row-selected="true"]');
    expect(selectedCells).toHaveLength(columns.length + 1);
    expect(screen.getByText("query.deleteRecord")).toBeInTheDocument();
    expect(screen.getByText("query.copyValue")).toBeInTheDocument();
    expect(screen.getByText("query.copyAs")).toBeInTheDocument();
    expect(screen.queryByText("query.copyFieldName")).not.toBeInTheDocument();
    expect(screen.queryByText("query.setNull")).not.toBeInTheDocument();
    expect(screen.queryByText("query.filterByCellValue")).not.toBeInTheDocument();
    expect(screen.queryByText("query.sortAscending")).not.toBeInTheDocument();
  });

  it("mouse-selects multiple rows with shift range and ctrl toggles", () => {
    const manyRows = [
      { id: 1, name: "alice" },
      { id: 2, name: "bob" },
      { id: 3, name: "carol" },
      { id: 4, name: "dave" },
    ];
    render(<QueryResultTable columns={columns} rows={manyRows} editable showRowNumber />);

    const row0 = document.querySelector('[data-row-header-key="0"]') as HTMLElement;
    const row2 = document.querySelector('[data-row-header-key="2"]') as HTMLElement;
    const row1 = document.querySelector('[data-row-header-key="1"]') as HTMLElement;

    fireEvent.mouseDown(row0);
    fireEvent.mouseDown(row2, { shiftKey: true });

    expect(document.querySelectorAll('[data-row-selected="true"]')).toHaveLength((columns.length + 1) * 3);

    fireEvent.mouseDown(row1, { ctrlKey: true });

    expect(document.querySelectorAll('[data-row-selected="true"]')).toHaveLength((columns.length + 1) * 2);
    expect(row0).toHaveAttribute("data-row-selected", "true");
    expect(row1).not.toHaveAttribute("data-row-selected");
    expect(row2).toHaveAttribute("data-row-selected", "true");
  });

  it("mouse-dragging row numbers selects a continuous row range", () => {
    const manyRows = [
      { id: 1, name: "alice" },
      { id: 2, name: "bob" },
      { id: 3, name: "carol" },
      { id: 4, name: "dave" },
    ];
    render(<QueryResultTable columns={columns} rows={manyRows} editable showRowNumber />);

    const row0 = document.querySelector('[data-row-header-key="0"]') as HTMLElement;
    const row3 = document.querySelector('[data-row-header-key="3"]') as HTMLElement;

    fireEvent.mouseDown(row0, { button: 0 });
    fireEvent.mouseEnter(row3, { buttons: 1 });
    fireEvent.mouseUp(row3);

    expect(document.querySelectorAll('[data-row-selected="true"]')).toHaveLength((columns.length + 1) * 4);
    expect(row0).toHaveAttribute("data-row-selected", "true");
    expect(row3).toHaveAttribute("data-row-selected", "true");
  });

  it("row context copy writes the selected row as tab separated values", async () => {
    openRowMenu();

    fireEvent.click(screen.getByText("query.copyValue"));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("2\tbob"));
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

  it("date-like cells show a date action and commit a datetime value", async () => {
    const user = userEvent.setup();
    const onSetCellValue = vi.fn();
    render(
      <QueryResultTable
        columns={["id", "created_at"]}
        rows={[{ id: 1, created_at: "2026-04-26 10:13:43" }]}
        editable
        columnTypes={{ created_at: "timestamp" }}
        onSetCellValue={onSetCellValue}
      />
    );
    const cell = document.querySelector('[data-cell-key="0:created_at"]') as HTMLElement;
    fireEvent.contextMenu(cell, { clientX: 40, clientY: 50 });

    await user.click(screen.getByText("query.setDateTime"));
    const dateInput = screen.getByLabelText("Date");
    const timeInput = screen.getByLabelText("Time");
    fireEvent.change(dateInput, { target: { value: "2026-04-27" } });
    fireEvent.change(timeInput, { target: { value: "08:09:10" } });
    await user.click(screen.getByText("action.ok"));

    expect(onSetCellValue).toHaveBeenCalledWith({
      rowIdx: 0,
      col: "created_at",
      value: "2026-04-27 08:09:10",
    });
  });

  it("opens the date editor from the selected date cell action", async () => {
    const user = userEvent.setup();
    const onSetCellValue = vi.fn();
    render(
      <QueryResultTable
        columns={["id", "created_at"]}
        rows={[{ id: 1, created_at: "2026-04-26 10:13:43" }]}
        editable
        columnTypes={{ created_at: "timestamp" }}
        onSetCellValue={onSetCellValue}
      />
    );

    await user.click(screen.getByText("2026-04-26 10:13:43"));
    await user.click(screen.getByTitle("query.openDateTimePicker"));
    const dateInput = screen.getByLabelText("Date");
    const timeInput = screen.getByLabelText("Time");
    fireEvent.change(dateInput, { target: { value: "2026-04-28" } });
    fireEvent.change(timeInput, { target: { value: "11:12:13" } });
    await user.click(screen.getByText("action.ok"));

    expect(onSetCellValue).toHaveBeenCalledWith({
      rowIdx: 0,
      col: "created_at",
      value: "2026-04-28 11:12:13",
    });
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

  it("filter submenu invokes the filter callback with the selected operator", async () => {
    const user = userEvent.setup();
    const onFilterByCellValue = vi.fn();
    openMenu({ onFilterByCellValue, onClearFilterSort: vi.fn(), onAddColumnFilter: vi.fn() });

    const filterTrigger = screen.getByText("query.filter");
    await user.hover(filterTrigger);
    await user.click(screen.getByText("query.filterOperatorIsNot"));

    expect(onFilterByCellValue).toHaveBeenCalledWith({ rowIdx: 1, col: "name", value: "bob", operator: "!=" });
  });

  it("filter submenu shows the full operator list without custom filter", async () => {
    const user = userEvent.setup();
    openMenu({ onFilterByCellValue: vi.fn(), onAddColumnFilter: vi.fn(), onClearFilterSort: vi.fn() });

    await user.hover(screen.getByText("query.filter"));

    [
      "query.filterOperatorIs",
      "query.filterOperatorIsNot",
      "query.filterOperatorLessThan",
      "query.filterOperatorLessThanOrEqual",
      "query.filterOperatorGreaterThan",
      "query.filterOperatorGreaterThanOrEqual",
      "query.filterOperatorContains",
      "query.filterOperatorDoesNotContain",
      "query.filterOperatorBeginsWith",
      "query.filterOperatorDoesNotBeginWith",
      "query.filterOperatorEndsWith",
      "query.filterOperatorDoesNotEndWith",
      "query.filterOperatorIsNull",
      "query.filterOperatorIsNotNull",
      "query.filterOperatorIsEmpty",
      "query.filterOperatorIsNotEmpty",
      "query.filterOperatorIsBetween",
      "query.filterOperatorIsNotBetween",
      "query.filterOperatorIsInList",
      "query.filterOperatorIsNotInList",
    ].forEach((labelKey) => {
      expect(screen.getByText(labelKey)).toBeInTheDocument();
    });
    expect(screen.queryByText("query.customFilter")).not.toBeInTheDocument();
  });

  it("filter submenu invokes remove filter callbacks for the current field", async () => {
    const user = userEvent.setup();
    const onAddColumnFilter = vi.fn();
    const onRemoveColumnFilter = vi.fn();
    openMenu({ onFilterByCellValue: vi.fn(), onAddColumnFilter, onRemoveColumnFilter, onClearFilterSort: vi.fn() });

    await user.hover(screen.getByText("query.filter"));
    expect(screen.queryByText("query.customFilter")).not.toBeInTheDocument();
    expect(onAddColumnFilter).not.toHaveBeenCalled();

    openMenu({ onFilterByCellValue: vi.fn(), onAddColumnFilter, onRemoveColumnFilter, onClearFilterSort: vi.fn() });
    await user.hover(screen.getByText("query.filter"));
    await user.click(screen.getByText("query.removeFilter"));
    expect(onRemoveColumnFilter).toHaveBeenCalledWith("name");
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
