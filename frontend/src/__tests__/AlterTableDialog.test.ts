import { describe, expect, it } from "vitest";
import { buildAlterStatements } from "../lib/tableSql";

describe("buildAlterStatements", () => {
  it("builds mysql alter statements for rename/add/modify/drop with comments", () => {
    const result = buildAlterStatements({
      driver: "mysql",
      database: "appdb",
      table: "users",
      tableNameDraft: "users_v2",
      tableCommentDraft: "user table",
      originalTableComment: "",
      originalColumns: [
        { name: "id", type: "INT", nullable: false, defaultValue: "", comment: "" },
        { name: "name", type: "VARCHAR(100)", nullable: true, defaultValue: "", comment: "old name" },
      ],
      draftColumns: [
        {
          id: 1,
          originalName: "id",
          name: "id",
          type: "BIGINT",
          nullable: false,
          defaultValue: "",
          comment: "id column",
          isNew: false,
        },
        {
          id: 3,
          name: "email",
          type: "VARCHAR(255)",
          nullable: false,
          defaultValue: "",
          comment: "email column",
          isNew: true,
        },
      ],
    });

    expect(result.nextTableName).toBe("users_v2");
    expect(result.statements).toEqual([
      "RENAME TABLE `appdb`.`users` TO `appdb`.`users_v2`",
      "ALTER TABLE `appdb`.`users_v2` ADD COLUMN `email` VARCHAR(255) NOT NULL COMMENT 'email column', MODIFY COLUMN `id` BIGINT NOT NULL COMMENT 'id column', DROP COLUMN `name`, COMMENT = 'user table'",
    ]);
  });

  it("builds postgresql statements for rename/add/modify with table and column comments", () => {
    const result = buildAlterStatements({
      driver: "postgresql",
      database: "appdb",
      table: "users",
      tableNameDraft: "users_v2",
      tableCommentDraft: "new table comment",
      originalTableComment: "old table comment",
      originalColumns: [{ name: "name", type: "text", nullable: true, defaultValue: "", comment: "old comment" }],
      draftColumns: [
        {
          id: 1,
          originalName: "name",
          name: "full_name",
          type: "varchar(120)",
          nullable: false,
          defaultValue: "anonymous",
          comment: "full name comment",
          isNew: false,
        },
        {
          id: 2,
          name: "age",
          type: "integer",
          nullable: true,
          defaultValue: "",
          comment: "age in years",
          isNew: true,
        },
      ],
    });

    expect(result.nextTableName).toBe("users_v2");
    expect(result.statements).toEqual([
      'ALTER TABLE "users" RENAME TO "users_v2"',
      'ALTER TABLE "users_v2" ADD COLUMN "age" integer',
      'ALTER TABLE "users_v2" RENAME COLUMN "name" TO "full_name"',
      'ALTER TABLE "users_v2" ALTER COLUMN "full_name" TYPE varchar(120)',
      'ALTER TABLE "users_v2" ALTER COLUMN "full_name" SET NOT NULL',
      'ALTER TABLE "users_v2" ALTER COLUMN "full_name" SET DEFAULT \'anonymous\'',
      "COMMENT ON TABLE \"users_v2\" IS 'new table comment'",
      'COMMENT ON COLUMN "users_v2"."full_name" IS \'full name comment\'',
      'COMMENT ON COLUMN "users_v2"."age" IS \'age in years\'',
    ]);
  });
});
