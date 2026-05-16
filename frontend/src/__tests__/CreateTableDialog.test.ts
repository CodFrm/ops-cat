import { describe, expect, it } from "vitest";
import { buildCreateTableSql } from "../lib/tableSql";

describe("buildCreateTableSql", () => {
  it("builds mysql CREATE TABLE with db-qualified name and default value formatting", () => {
    const sql = buildCreateTableSql({
      driver: "mysql",
      database: "appdb",
      name: "users",
      columns: [
        { name: "id", type: "BIGINT", nullable: false, defaultValue: "" },
        { name: "age", type: "INT", nullable: true, defaultValue: "18" },
        { name: "name", type: "VARCHAR(100)", nullable: false, defaultValue: "anon" },
        { name: "active", type: "BOOLEAN", nullable: true, defaultValue: "true" },
        { name: "created_at", type: "DATETIME", nullable: false, defaultValue: "CURRENT_TIMESTAMP" },
      ],
    });

    expect(sql).toBe(
      "CREATE TABLE `appdb`.`users` (\n" +
        "  `id` BIGINT NOT NULL,\n" +
        "  `age` INT DEFAULT 18,\n" +
        "  `name` VARCHAR(100) NOT NULL DEFAULT 'anon',\n" +
        "  `active` BOOLEAN DEFAULT TRUE,\n" +
        "  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP\n" +
        ")"
    );
  });

  it("builds postgresql CREATE TABLE with unqualified name", () => {
    const sql = buildCreateTableSql({
      driver: "postgresql",
      database: "appdb",
      name: "users",
      columns: [
        { name: "id", type: "serial", nullable: false, defaultValue: "" },
        { name: "email", type: "text", nullable: true, defaultValue: "" },
      ],
    });

    expect(sql).toBe('CREATE TABLE "users" (\n' + '  "id" serial NOT NULL,\n' + '  "email" text\n' + ")");
  });

  it("escapes embedded delimiter characters in identifiers", () => {
    const mysql = buildCreateTableSql({
      driver: "mysql",
      database: "app",
      name: "we`ird",
      columns: [{ name: "col`1", type: "INT", nullable: true, defaultValue: "" }],
    });
    expect(mysql).toBe("CREATE TABLE `app`.`we``ird` (\n  `col``1` INT\n)");

    const pg = buildCreateTableSql({
      driver: "postgresql",
      database: "app",
      name: 'we"ird',
      columns: [{ name: 'col"1', type: "text", nullable: true, defaultValue: "" }],
    });
    expect(pg).toBe('CREATE TABLE "we""ird" (\n  "col""1" text\n)');
  });
});
