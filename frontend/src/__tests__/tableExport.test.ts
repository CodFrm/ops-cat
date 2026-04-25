import { describe, expect, it } from "vitest";
import { toCsv, toInsertSql, toTsv, toTsvData, toTsvFields, toUpdateSql } from "@/lib/tableExport";

const columns = ["id", "name", "note", "missing"];
const rows = [
  { id: 1, name: "Alice", note: "hello, world", missing: null },
  { id: 2, name: "O'Reilly", note: "line\nbreak", missing: "" },
  { id: 3, name: "中文", note: 'say "hi"', missing: undefined },
];

describe("table export helpers", () => {
  it("exports CSV with headers, quotes, newlines, Chinese text, and empty NULL cells", () => {
    expect(toCsv(columns, rows)).toBe(
      ["id,name,note,missing", '1,Alice,"hello, world",', '2,O\'Reilly,"line\nbreak",', '3,中文,"say ""hi""",'].join(
        "\n"
      )
    );
  });

  it("exports TSV with tab/newline escaping and empty NULL cells", () => {
    expect(
      toTsv(
        ["id", "note"],
        [
          { id: 1, note: "tab\tvalue" },
          { id: 2, note: "line\nbreak" },
        ]
      )
    ).toBe(["id\tnote", '1\t"tab\tvalue"', '2\t"line\nbreak"'].join("\n"));
  });

  it("exports INSERT SQL with identifier quoting, value quoting, and SQL NULL", () => {
    expect(toInsertSql("appdb.users", ["id", "name", "missing"], rows, "mysql")).toBe(
      [
        "INSERT INTO `appdb`.`users` (`id`, `name`, `missing`) VALUES ('1', 'Alice', NULL);",
        "INSERT INTO `appdb`.`users` (`id`, `name`, `missing`) VALUES ('2', 'O''Reilly', '');",
        "INSERT INTO `appdb`.`users` (`id`, `name`, `missing`) VALUES ('3', '中文', NULL);",
      ].join("\n")
    );
  });

  it("exports Copy As TSV variants", () => {
    expect(toTsvData(["id", "name"], [rows[0]])).toBe("1\tAlice");
    expect(toTsvFields(["id", "name"])).toBe("id\tname");
    expect(toTsv(["id", "name"], [rows[0]])).toBe("id\tname\n1\tAlice");
  });

  it("exports UPDATE SQL using primary keys when available", () => {
    expect(toUpdateSql("appdb.users", ["id", "name"], rows[1], ["id"], "mysql")).toBe(
      "UPDATE `appdb`.`users` SET `id` = '2', `name` = 'O''Reilly' WHERE `id` = '2' LIMIT 1;"
    );
  });
});
