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

  it("keeps foreign-key toggling out of generated row statements", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.users",
        headers: ["id"],
        rows: [["1"]],
        mapping: { id: "id" },
        nullStrategy: "literal-null",
        mode: "copy",
        advancedOptions: { ignoreForeignKeyConstraint: true },
        driver: "mysql",
      })
    ).toEqual(["DELETE FROM `appdb`.`users`;", "INSERT INTO `appdb`.`users` (`id`) VALUES ('1');"]);
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

  it("defaults to auto record-delimiter detection so Windows CSVs do not leak \\r into cells", () => {
    expect(
      parseImportSourceText({
        text: "id,name\r\n1,Alice\r\n2,Bob\n",
        format: "csv",
        fieldDelimiter: ",",
      })
    ).toEqual({
      headers: ["id", "name"],
      rows: [
        ["1", "Alice"],
        ["2", "Bob"],
      ],
    });
  });

  it("respects an explicit record delimiter so opposite line endings stay inside cells", () => {
    expect(
      parseImportSourceText({
        text: "id,note\r1,line\nstill-one\r2,bob",
        format: "csv",
        fieldDelimiter: ",",
        recordDelimiter: "cr",
      })
    ).toEqual({
      headers: ["id", "note"],
      rows: [
        ["1", "line\nstill-one"],
        ["2", "bob"],
      ],
    });

    expect(
      parseImportSourceText({
        text: "id,note\r\n1,line\nstill-one\r\n",
        format: "csv",
        fieldDelimiter: ",",
        recordDelimiter: "crlf",
      })
    ).toEqual({
      headers: ["id", "note"],
      rows: [["1", "line\nstill-one"]],
    });
  });

  it("supports single-quote text qualifiers and disabling qualifiers entirely", () => {
    expect(
      parseImportSourceText({
        text: "id,name\n1,'Alice, A.'\n2,'say ''hi'''",
        format: "csv",
        fieldDelimiter: ",",
        textQualifier: "'",
      })
    ).toEqual({
      headers: ["id", "name"],
      rows: [
        ["1", "Alice, A."],
        ["2", "say 'hi'"],
      ],
    });

    expect(
      parseImportSourceText({
        text: 'id,name\n1,"Alice"',
        format: "csv",
        fieldDelimiter: ",",
        textQualifier: "none",
      })
    ).toEqual({
      headers: ["id", "name"],
      rows: [["1", '"Alice"']],
    });
  });

  it("normalizes decimal symbols for numeric target columns", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.metrics",
        headers: ["id", "score", "name"],
        rows: [["1", "1,5", "alice,bob"]],
        mapping: { id: "id", score: "score", name: "name" },
        nullStrategy: "literal-null",
        driver: "mysql",
        columnTypes: { id: "int", score: "decimal(10,2)", name: "varchar(64)" },
        conversionOptions: { decimalSymbol: "," },
      })
    ).toEqual(["INSERT INTO `appdb`.`metrics` (`id`, `score`, `name`) VALUES ('1', '1.5', 'alice,bob');"]);
  });

  it("converts source dates and datetimes to canonical SQL literals", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.events",
        headers: ["id", "occurred_on", "occurred_at"],
        rows: [["1", "24/8/23", "15:30:38 24/Aug/2023"]],
        mapping: { id: "id", occurred_on: "occurred_on", occurred_at: "occurred_at" },
        nullStrategy: "literal-null",
        driver: "mysql",
        columnTypes: { id: "int", occurred_on: "date", occurred_at: "datetime" },
        conversionOptions: {
          dateOrder: "dmy",
          dateDelimiter: "/",
          dateTimeOrder: "time-date",
          timeDelimiter: ":",
        },
      })
    ).toEqual([
      "INSERT INTO `appdb`.`events` (`id`, `occurred_on`, `occurred_at`) VALUES ('1', '2023-08-24', '2023-08-24 15:30:38');",
    ]);
  });

  it("supports a separate year delimiter when enabled", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.events",
        headers: ["d"],
        rows: [["24/8-2023"]],
        mapping: { d: "d" },
        nullStrategy: "literal-null",
        driver: "mysql",
        columnTypes: { d: "date" },
        conversionOptions: {
          dateOrder: "dmy",
          dateDelimiter: "/",
          yearDelimiter: "-",
        },
      })
    ).toEqual(["INSERT INTO `appdb`.`events` (`d`) VALUES ('2023-08-24');"]);
  });

  it("emits driver-aware binary literals from base64 and hex encodings", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.files",
        headers: ["id", "payload"],
        rows: [["1", "SGVsbG8="]],
        mapping: { id: "id", payload: "payload" },
        nullStrategy: "literal-null",
        driver: "mysql",
        columnTypes: { id: "int", payload: "blob" },
        conversionOptions: { binaryEncoding: "base64" },
      })
    ).toEqual(["INSERT INTO `appdb`.`files` (`id`, `payload`) VALUES ('1', X'48656c6c6f');"]);

    expect(
      buildImportInsertSql({
        tableName: "appdb.files",
        headers: ["id", "payload"],
        rows: [["1", "48656C6C6F"]],
        mapping: { id: "id", payload: "payload" },
        nullStrategy: "literal-null",
        driver: "postgresql",
        columnTypes: { id: "int", payload: "bytea" },
        conversionOptions: { binaryEncoding: "hex" },
      })
    ).toEqual([`INSERT INTO "appdb"."files" ("id", "payload") VALUES ('1', '\\x48656c6c6f'::bytea);`]);
  });

  it("uses converted literals in WHERE clauses for primary-key based modes", () => {
    expect(
      buildImportInsertSql({
        tableName: "appdb.events",
        headers: ["id", "name"],
        rows: [["1,5", "Alice"]],
        mapping: { id: "id", name: "name" },
        nullStrategy: "literal-null",
        primaryKeys: ["id"],
        mode: "update",
        driver: "mysql",
        columnTypes: { id: "decimal(10,2)", name: "varchar(64)" },
        conversionOptions: { decimalSymbol: "," },
      })
    ).toEqual(["UPDATE `appdb`.`events` SET `name` = 'Alice' WHERE `id` = '1.5';"]);
  });
});
