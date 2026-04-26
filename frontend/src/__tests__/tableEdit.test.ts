import { describe, expect, it } from "vitest";
import { buildInsertStatement, validateInsertRow, type TableColumnRule } from "@/lib/tableEdit";

const rules: TableColumnRule[] = [
  { name: "id", nullable: false, hasDefault: false, autoIncrement: true },
  { name: "name", nullable: false, hasDefault: false },
  { name: "email", nullable: true, hasDefault: false },
  { name: "note", nullable: false, hasDefault: true },
];

describe("table edit helpers", () => {
  it("requires non-null columns that do not have defaults before inserting", () => {
    expect(validateInsertRow(rules, { email: "a@example.com" })).toEqual(["name"]);
    expect(validateInsertRow(rules, { name: "Alice" })).toEqual([]);
  });

  it("builds an INSERT for edited fields and omits default-backed fields that were not edited", () => {
    expect(
      buildInsertStatement({
        database: "appdb",
        table: "users",
        driver: "mysql",
        values: { name: "Alice", email: "alice@example.com" },
      })
    ).toBe("INSERT INTO `appdb`.`users` (`name`, `email`) VALUES ('Alice', 'alice@example.com');");
  });
});
