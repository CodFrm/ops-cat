import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TableFilterBuilder } from "@/components/query/TableFilterBuilder";
import { createFilterCondition, type TableFilterItem, type TableSortItem } from "@/lib/tableFilter";

describe("TableFilterBuilder", () => {
  it("shows distinct suggested values and writes the selected value into the condition", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <TableFilterBuilder
        columns={["id", "email"]}
        rows={[
          { id: 1, email: "alice@example.com" },
          { id: 2, email: "11223" },
          { id: 3, email: "alice@example.com" },
        ]}
        filters={[createFilterCondition("f-email", "email")]}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    await user.click(screen.getByTitle("query.chooseFilterValue"));
    const panel = await screen.findByRole("dialog");

    expect(within(panel).getByText("alice@example.com")).toBeInTheDocument();
    expect(within(panel).getByText("11223")).toBeInTheDocument();
    expect(within(panel).getAllByText("alice@example.com")).toHaveLength(1);

    await user.click(within(panel).getByText("11223"));

    expect(onChange).toHaveBeenLastCalledWith([
      expect.objectContaining({ kind: "condition", column: "email", value: "11223" }),
    ]);
  });

  it("adds grouped criteria using the next globally unused column", async () => {
    const user = userEvent.setup();
    let filters: TableFilterItem[] = [createFilterCondition("f-id", "id")];
    const sorts: TableSortItem[] = [];
    const onChange = vi.fn((next: TableFilterItem[]) => {
      filters = next;
    });
    const view = render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={sorts}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    await user.click(screen.getByTitle("query.addFilterGroup"));
    view.rerender(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={sorts}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );
    await user.click(screen.getAllByTitle("query.addFilter")[0]);

    const latest = onChange.mock.calls.at(-1)?.[0] as TableFilterItem[];
    const group = latest[1];
    expect(group).toMatchObject({ kind: "group" });
    if (group.kind !== "group") throw new Error("expected group");
    expect(group.items[0]).toMatchObject({ kind: "condition", column: "email" });
  });

  it("opens a filter row context menu and deletes one condition", async () => {
    const user = userEvent.setup();
    const filters: TableFilterItem[] = [createFilterCondition("f-id", "id"), createFilterCondition("f-email", "email")];
    const onChange = vi.fn();

    render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    fireEvent.contextMenu(screen.getByTestId("filter-item-f-id"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.deleteFilterItem"));

    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({ id: "f-email" })]);
  });

  it("uses context menu actions to disable and negate a condition", async () => {
    const user = userEvent.setup();
    const filters: TableFilterItem[] = [createFilterCondition("f-name", "name", { value: "alice" })];
    const onChange = vi.fn();

    render(
      <TableFilterBuilder
        columns={["id", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    fireEvent.contextMenu(screen.getByTestId("filter-item-f-name"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.disableFilterItem"));
    expect(onChange).toHaveBeenLastCalledWith([expect.objectContaining({ id: "f-name", enabled: false })]);

    fireEvent.contextMenu(screen.getByTestId("filter-item-f-name"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.toggleFilterNegator"));
    expect(onChange).toHaveBeenLastCalledWith([expect.objectContaining({ id: "f-name", operator: "!=" })]);
  });

  it("offers separate delete choices for brackets", async () => {
    const user = userEvent.setup();
    const child = createFilterCondition("f-email", "email");
    const filters: TableFilterItem[] = [
      { kind: "group", id: "g-1", join: "and", enabled: true, items: [child] },
      createFilterCondition("f-id", "id"),
    ];
    const onChange = vi.fn();

    render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    fireEvent.contextMenu(screen.getByTestId("filter-item-g-1"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.deleteFilterGroupOnly"));
    expect(onChange).toHaveBeenLastCalledWith([
      expect.objectContaining({ id: "f-email" }),
      expect.objectContaining({ id: "f-id" }),
    ]);

    fireEvent.contextMenu(screen.getByTestId("filter-item-g-1"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.deleteFilterGroupWithChildren"));
    expect(onChange).toHaveBeenLastCalledWith([expect.objectContaining({ id: "f-id" })]);
  });

  it("keeps grouped filter body separate from the clickable bracket rows", () => {
    const filters: TableFilterItem[] = [{ kind: "group", id: "g-1", join: "and", enabled: true, items: [] }];

    render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={vi.fn()}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    const groupOpen = screen.getByTestId("filter-group-g-1-open");
    const groupBody = screen.getByTestId("filter-group-g-1-body");
    const groupClose = screen.getByTestId("filter-group-g-1-close");

    fireEvent.click(groupBody);

    expect(groupOpen).not.toHaveClass("bg-primary/10");
    expect(groupBody).not.toHaveClass("bg-primary/10");
    expect(groupClose).not.toHaveClass("bg-primary/10");

    fireEvent.contextMenu(groupBody, { clientX: 20, clientY: 30 });
    expect(screen.queryByText("query.deleteFilterGroupWithChildren")).not.toBeInTheDocument();

    fireEvent.click(groupOpen);

    expect(groupOpen).toHaveClass("bg-primary/10");
    expect(groupBody).not.toHaveClass("bg-primary/10");
    expect(groupClose).toHaveClass("bg-primary/10");
  });

  it("copies and pastes filter criteria from the context menu", async () => {
    const user = userEvent.setup();
    let filters: TableFilterItem[] = [
      createFilterCondition("f-name", "name", { value: "alice" }),
      createFilterCondition("f-email", "email"),
    ];
    const onChange = vi.fn((next: TableFilterItem[]) => {
      filters = next;
    });
    const view = render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );

    fireEvent.contextMenu(screen.getByTestId("filter-item-f-name"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.copyFilterItem"));

    view.rerender(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={filters}
        sorts={[]}
        driver="mysql"
        onChange={onChange}
        onSortsChange={vi.fn()}
        onApply={vi.fn()}
      />
    );
    fireEvent.contextMenu(screen.getByTestId("filter-item-f-email"), { clientX: 20, clientY: 30 });
    await user.click(screen.getByText("query.pasteFilterItem"));

    const latest = onChange.mock.calls.at(-1)?.[0] as TableFilterItem[];
    expect(latest).toHaveLength(3);
    expect(latest[2]).toMatchObject({ kind: "condition", column: "name", value: "alice" });
    expect(latest[2].id).not.toBe("f-name");
  });

  it("adds sort criteria and toggles sort direction from the field suffix control", async () => {
    const user = userEvent.setup();
    let sorts: TableSortItem[] = [];
    const onSortsChange = vi.fn((next: TableSortItem[]) => {
      sorts = next;
    });
    const view = render(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={[]}
        sorts={sorts}
        driver="mysql"
        onChange={vi.fn()}
        onSortsChange={onSortsChange}
        onApply={vi.fn()}
      />
    );

    await user.click(screen.getByTitle("query.addSort"));

    expect(onSortsChange).toHaveBeenLastCalledWith([expect.objectContaining({ column: "id", dir: "asc" })]);

    view.rerender(
      <TableFilterBuilder
        columns={["id", "email", "name"]}
        rows={[]}
        filters={[]}
        sorts={sorts}
        driver="mysql"
        onChange={vi.fn()}
        onSortsChange={onSortsChange}
        onApply={vi.fn()}
      />
    );
    await user.click(screen.getByTitle("query.toggleSortDirection:id"));

    expect(onSortsChange).toHaveBeenLastCalledWith([expect.objectContaining({ column: "id", dir: "desc" })]);
  });
});
