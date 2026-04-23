import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryResultTable, cellValueToText } from "@/components/query/QueryResultTable";

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
