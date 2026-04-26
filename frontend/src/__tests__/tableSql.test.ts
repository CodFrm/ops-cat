import { describe, expect, it } from "vitest";
import { buildDeleteStatement, buildFilterByCellValueClause } from "@/lib/tableSql";

describe("table SQL helpers", () => {
  it("builds filter clauses for NULL and quoted values", () => {
    expect(buildFilterByCellValueClause("deleted_at", null)).toBe("`deleted_at` IS NULL");
    expect(buildFilterByCellValueClause("name", "O'Reilly")).toBe("`name` = 'O''Reilly'");
    expect(buildFilterByCellValueClause("age", 42)).toBe("`age` = '42'");
    expect(buildFilterByCellValueClause("name", "bob", "mysql", "!=")).toBe("`name` <> 'bob'");
    expect(buildFilterByCellValueClause("name", "bob", "mysql", "like")).toBe("`name` LIKE '%bob%'");
    expect(buildFilterByCellValueClause("name", "bob", "mysql", "not_like")).toBe("`name` NOT LIKE '%bob%'");
  });

  it("builds DELETE SQL using primary keys when available", () => {
    const result = buildDeleteStatement({
      database: "appdb",
      table: "users",
      columns: ["id", "name", "deleted_at"],
      row: { id: 7, name: "alice", deleted_at: null },
      primaryKeys: ["id"],
      driver: "mysql",
    });

    expect(result.sql).toBe("DELETE FROM `appdb`.`users` WHERE `id` = '7' LIMIT 1;");
    expect(result.usesPrimaryKey).toBe(true);
  });

  it("falls back to all columns when deleting without a primary key", () => {
    const result = buildDeleteStatement({
      database: "appdb",
      table: "users",
      columns: ["id", "name", "deleted_at"],
      row: { id: 7, name: "O'Reilly", deleted_at: null },
      primaryKeys: [],
      driver: "mysql",
    });

    expect(result.sql).toBe(
      "DELETE FROM `appdb`.`users` WHERE `id` = '7' AND `name` = 'O''Reilly' AND `deleted_at` IS NULL LIMIT 1;"
    );
    expect(result.usesPrimaryKey).toBe(false);
  });
});
