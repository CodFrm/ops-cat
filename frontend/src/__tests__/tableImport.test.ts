import { describe, expect, it } from "vitest";
import { buildImportInsertSql, detectDelimiter, parseDelimitedText } from "@/lib/tableImport";

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

  it("detects TSV when tabs outnumber commas in the header", () => {
    expect(detectDelimiter("id\tname\tnote\n1\tAlice\tok")).toBe("\t");
    expect(detectDelimiter("id,name,note\n1,Alice,ok")).toBe(",");
  });
});
