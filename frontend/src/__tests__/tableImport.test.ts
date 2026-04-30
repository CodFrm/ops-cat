import { describe, expect, it } from "vitest";
import { buildImportInsertSql, detectDelimiter, parseDelimitedText, parseImportSourceText } from "@/lib/tableImport";

describe("table import helpers", () => {
  it("parses quoted CSV cells with commas, quotes, and embedded newlines", () => {
    expect(parseDelimitedText('id,name,note\n1,"Alice, A.","line\nbreak"\n2,"say ""hi""",ok', ",")).toEqual({
      headers: ["id", "name", "note"],
      rows: [
        ["1", "Alice, A.", "line\nbreak"],
        ["2", 'say "hi"', "ok"],
      ],
    });
  });

  it("parses TSV without treating commas as separators", () => {
    expect(parseDelimitedText("id\tname\tnote\n1\tAlice, A.\t中文", "\t")).toEqual({
      headers: ["id", "name", "note"],
      rows: [["1", "Alice, A.", "中文"]],
    });
  });

  it("keeps empty cells as empty strings unless the null strategy marks them NULL", () => {
    const parsed = parseDelimitedText("id,name,note\n1,,NULL\n2,Bob,", ",");

    expect(
      buildImportInsertSql({
        tableName: "appdb.users",
        headers: parsed.headers,
        rows: parsed.rows,
        mapping: { id: "id", name: "name", note: "note" },
        nullStrategy: "literal-null",
        driver: "mysql",
      })
    ).toEqual([
      "INSERT INTO `appdb`.`users` (`id`, `name`, `note`) VALUES ('1', '', NULL);",
      "INSERT INTO `appdb`.`users` (`id`, `name`, `note`) VALUES ('2', 'Bob', '');",
    ]);
  });

  it("can map empty cells to SQL NULL", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.users",
        headers: ["id", "name"],
        rows: [["1", ""]],
        mapping: { id: "id", name: "name" },
        nullStrategy: "empty-is-null",
        driver: "mysql",
      })
    ).toEqual(["INSERT INTO `appdb`.`users` (`id`, `name`) VALUES ('1', NULL);"]);
  });

  it("builds SQL for primary-key based import modes", () => {
    const base = {
      tableName: "appdb.users",
      headers: ["id", "name", "email"],
      rows: [["1", "Alice", "alice@example.test"]],
      mapping: { id: "id", name: "name", email: "email" },
      nullStrategy: "literal-null" as const,
      primaryKeys: ["id"],
      driver: "mysql",
    };

    expect(buildImportInsertSql({ ...base, mode: "update" })).toEqual([
      "UPDATE `appdb`.`users` SET `name` = 'Alice', `email` = 'alice@example.test' WHERE `id` = '1';",
    ]);
    expect(buildImportInsertSql({ ...base, mode: "append-update" })).toEqual([
      "INSERT INTO `appdb`.`users` (`id`, `name`, `email`) VALUES ('1', 'Alice', 'alice@example.test') ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `email` = VALUES(`email`);",
    ]);
    expect(buildImportInsertSql({ ...base, mode: "append-skip" })).toEqual([
      "INSERT IGNORE INTO `appdb`.`users` (`id`, `name`, `email`) VALUES ('1', 'Alice', 'alice@example.test');",
    ]);
    expect(buildImportInsertSql({ ...base, mode: "delete" })).toEqual([
      "DELETE FROM `appdb`.`users` WHERE `id` = '1';",
    ]);
    expect(buildImportInsertSql({ ...base, mode: "copy" })).toEqual([
      "DELETE FROM `appdb`.`users`;",
      "INSERT INTO `appdb`.`users` (`id`, `name`, `email`) VALUES ('1', 'Alice', 'alice@example.test');",
    ]);
  });

  it("uses ON CONFLICT DO NOTHING for postgresql append-update with no value columns", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.users",
        headers: ["id"],
        rows: [["1"]],
        mapping: { id: "id" },
        nullStrategy: "literal-null",
        primaryKeys: ["id"],
        mode: "append-update",
        driver: "postgresql",
      })
    ).toEqual([`INSERT INTO "appdb"."users" ("id") VALUES ('1') ON CONFLICT ("id") DO NOTHING;`]);

    expect(
      buildImportInsertSql({
        tableName: "appdb.users",
        headers: ["id"],
        rows: [["1"]],
        mapping: { id: "id" },
        nullStrategy: "literal-null",
        primaryKeys: ["id"],
        mode: "append-update",
        driver: "mysql",
      })
    ).toEqual(["INSERT IGNORE INTO `appdb`.`users` (`id`) VALUES ('1');"]);
  });

  it("detects TSV when tabs outnumber commas in the header", () => {
    expect(detectDelimiter("id\tname\tnote\n1\tAlice\tok")).toBe("\t");
    expect(detectDelimiter("id,name,note\n1,Alice,ok")).toBe(",");
  });

  it("parses JSON arrays of objects into headers and rows", () => {
    expect(
      parseImportSourceText({
        text: JSON.stringify([
          { id: 1, name: "Alice", active: true },
          { id: 2, name: "Bob", note: { tier: "gold" } },
        ]),
        format: "json",
      })
    ).toEqual({
      headers: ["id", "name", "active", "note"],
      rows: [
        ["1", "Alice", "true", ""],
        ["2", "Bob", "", '{"tier":"gold"}'],
      ],
    });
  });

  it("parses XML repeated elements into headers and rows", () => {
    expect(
      parseImportSourceText({
        text: "<users><user><id>1</id><name>Alice</name></user><user><id>2</id><name>Bob</name></user></users>",
        format: "xml",
      })
    ).toEqual({
      headers: ["id", "name"],
      rows: [
        ["1", "Alice"],
        ["2", "Bob"],
      ],
    });
  });
});
