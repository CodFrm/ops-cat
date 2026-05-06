import { describe, expect, it } from "vitest";
import {
  addFilterCondition,
  addFilterGroup,
  addSortCriterion,
  buildFilterWhereClause,
  buildSortOrderByClause,
  createFilterCondition,
  createSortCriterion,
  removeFilterItemsByColumn,
  toggleFilterNegator,
  toggleFilterJoin,
  toggleSortDirection,
  type TableFilterItem,
} from "@/lib/tableFilter";

describe("table filter helpers", () => {
  it("adds the next unused column in display order", () => {
    const items = [createFilterCondition("email", "email")];
    const next = addFilterCondition(items, ["id", "email", "name"], "mysql");

    expect(next).toHaveLength(2);
    expect(next[1]).toMatchObject({ kind: "condition", column: "id", operator: "=", enabled: true });
  });

  it("toggles joins between AND and OR", () => {
    const items = [createFilterCondition("email", "email", { join: "and" })];

    expect(toggleFilterJoin(items, "email")[0]).toMatchObject({ join: "or" });
    expect(toggleFilterJoin(toggleFilterJoin(items, "email"), "email")[0]).toMatchObject({ join: "and" });
  });

  it("builds SQL with OR joins and bracket groups", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("email", "email", { value: "11223", join: "or" }),
      addFilterGroup([], ["id", "name"], "mysql", "group-1")[0],
    ];
    const group = items[1];
    if (group.kind !== "group") throw new Error("expected group");
    group.items = [createFilterCondition("id", "id", { value: 2 })];

    expect(buildFilterWhereClause(items, "mysql")).toBe("`email` = '11223' OR (`id` = '2')");
  });

  it("builds SQL for LIKE and NOT LIKE criteria", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("name-like", "name", { operator: "like", value: "bob" }),
      createFilterCondition("email-not-like", "email", { operator: "not_like", value: "@test.com" }),
    ];

    expect(buildFilterWhereClause(items, "mysql")).toBe("`name` LIKE '%bob%' AND `email` NOT LIKE '%@test.com%'");
  });

  it("builds SQL for text, null, range, and list operators", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("begins", "name", { operator: "begins_with", value: "Al" }),
      createFilterCondition("not-ends", "email", { operator: "not_ends_with", value: "@old.com" }),
      createFilterCondition("null", "deleted_at", { operator: "is_null" }),
      createFilterCondition("range", "age", { operator: "between", value: [18, 30] }),
      createFilterCondition("list", "status", { operator: "in_list", value: ["active", "pending"] }),
    ];

    expect(buildFilterWhereClause(items, "mysql")).toBe(
      "`name` LIKE 'Al%' AND `email` NOT LIKE '%@old.com' AND `deleted_at` IS NULL AND `age` BETWEEN '18' AND '30' AND `status` IN ('active', 'pending')"
    );
  });

  it("removes filters for a column from nested groups", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("name", "name", { value: "A" }),
      {
        kind: "group",
        id: "group-1",
        join: "and",
        items: [
          createFilterCondition("name-nested", "name", { value: "B" }),
          createFilterCondition("email-nested", "email", { value: "a@example.com" }),
        ],
      },
    ];

    expect(removeFilterItemsByColumn(items, "name")).toEqual([
      {
        kind: "group",
        id: "group-1",
        join: "and",
        items: [createFilterCondition("email-nested", "email", { value: "a@example.com" })],
      },
    ]);
  });

  it("builds disabled, negated, empty, and comparison criteria inside bracket groups", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("name", "name", { value: "A", join: "and" }),
      {
        kind: "group",
        id: "names-or-amount",
        join: "and",
        enabled: true,
        negated: true,
        items: [
          createFilterCondition("name-b", "name", { value: "B", join: "or" }),
          createFilterCondition("amount", "amount", { operator: ">", value: 100, join: "and" }),
          createFilterCondition("ignored", "deleted_at", { enabled: false, value: null }),
        ],
      },
      createFilterCondition("empty", "memo", { operator: "is_empty" }),
    ];

    expect(buildFilterWhereClause(items, "mysql")).toBe(
      "`name` = 'A' AND NOT (`name` = 'B' OR `amount` > '100') AND (`memo` IS NULL OR `memo` = '')"
    );
  });

  it("toggles filter negators for conditions and groups", () => {
    const items: TableFilterItem[] = [
      createFilterCondition("name", "name", { value: "A" }),
      { kind: "group", id: "group-1", join: "and", items: [] },
    ];

    const negatedCondition = toggleFilterNegator(items, "name");
    expect(negatedCondition[0]).toMatchObject({ operator: "!=" });
    expect(buildFilterWhereClause(negatedCondition, "mysql")).toBe("`name` <> 'A'");

    const negatedGroup = toggleFilterNegator(items, "group-1")[1];
    expect(negatedGroup).toMatchObject({ kind: "group", negated: true });
  });

  it("adds sort criteria by unused field order and toggles direction", () => {
    const first = addSortCriterion([], ["id", "email", "name"], "mysql", "sort-id");
    const second = addSortCriterion(first, ["id", "email", "name"], "mysql", "sort-email");

    expect(second).toEqual([createSortCriterion("sort-id", "id"), createSortCriterion("sort-email", "email")]);
    expect(toggleSortDirection(second, "sort-id")[0]).toMatchObject({ dir: "desc" });
  });

  it("builds ORDER BY SQL from sort criteria", () => {
    expect(
      buildSortOrderByClause(
        [createSortCriterion("sort-id", "id", "asc"), createSortCriterion("sort-name", "name", "desc")],
        "mysql"
      )
    ).toBe("`id` ASC, `name` DESC");
  });
});
