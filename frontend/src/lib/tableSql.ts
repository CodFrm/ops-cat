import type { TableFilterOperator } from "./tableFilterOperators";

export function sqlQuote(value: unknown): string {
  if (value == null) return "NULL";
  const s = String(value);
  const escaped = s.replace(/'/g, "''");
  return `'${escaped}'`;
}

export function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name.replace(/"/g, '""')}"`;
  return `\`${name.replace(/`/g, "``")}\``;
}

export function quoteQualifiedIdent(name: string, driver?: string): string {
  return name
    .split(".")
    .filter(Boolean)
    .map((part) => quoteIdent(part, driver))
    .join(".");
}

export function quoteTableRef(database: string, table: string, driver?: string): string {
  if (driver === "postgresql") return quoteQualifiedIdent(table, driver);
  return `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
}

function formatDefaultValue(value: string): string {
  const v = value.trim();
  if (!v) return "";
  if (/^[-+]?\d+(\.\d+)?$/.test(v)) return v;
  if (/^(true|false|null)$/i.test(v)) return v.toUpperCase();
  if (/^(current_timestamp(?:\(\))?|now\(\))$/i.test(v)) return v;
  return sqlQuote(v);
}

export interface BuildCreateTableSqlInput {
  driver?: string;
  database: string;
  name: string;
  columns: { name: string; type: string; nullable: boolean; defaultValue: string }[];
}

export function buildCreateTableSql({ driver, database, name, columns }: BuildCreateTableSqlInput): string {
  const tableRef = quoteTableRef(database, name, driver);

  const defs = columns.map((col) => {
    const nullable = col.nullable ? "" : " NOT NULL";
    const def = col.defaultValue.trim() ? ` DEFAULT ${formatDefaultValue(col.defaultValue)}` : "";
    return `${quoteIdent(col.name.trim(), driver)} ${col.type.trim()}${nullable}${def}`;
  });

  return `CREATE TABLE ${tableRef} (\n  ${defs.join(",\n  ")}\n)`;
}

export interface AlterLoadedColumn {
  name: string;
  type: string;
  nullable: boolean;
  defaultValue: string;
  comment: string;
}

export interface AlterDraftColumn {
  id: number;
  originalName?: string;
  name: string;
  type: string;
  nullable: boolean;
  defaultValue: string;
  comment: string;
  isNew: boolean;
}

function normalizeDefault(value: string): string {
  return value.trim();
}

function normalizeComment(value: string): string {
  return value.trim();
}

function buildColumnDefinition(
  column: Pick<AlterDraftColumn, "name" | "type" | "nullable" | "defaultValue" | "comment">,
  driver?: string,
  forceComment = false
): string {
  const nullable = column.nullable ? "" : " NOT NULL";
  const defaultPart = normalizeDefault(column.defaultValue)
    ? ` DEFAULT ${formatDefaultValue(column.defaultValue)}`
    : "";
  const includeComment = driver !== "postgresql" && (forceComment || !!normalizeComment(column.comment));
  const commentPart = includeComment ? ` COMMENT ${sqlQuote(column.comment)}` : "";
  return `${quoteIdent(column.name.trim(), driver)} ${column.type.trim()}${nullable}${defaultPart}${commentPart}`;
}

export function buildAlterStatements(params: {
  driver?: string;
  database: string;
  table: string;
  tableNameDraft: string;
  tableCommentDraft: string;
  originalTableComment: string;
  originalColumns: AlterLoadedColumn[];
  draftColumns: AlterDraftColumn[];
}): { statements: string[]; nextTableName?: string } {
  const {
    driver,
    database,
    table,
    tableNameDraft,
    tableCommentDraft,
    originalTableComment,
    originalColumns,
    draftColumns,
  } = params;

  const statements: string[] = [];
  const nextTableName = tableNameDraft.trim();
  const hasRenameTable = !!nextTableName && nextTableName !== table;

  const originalTableRef = quoteTableRef(database, table, driver);
  const targetTable = hasRenameTable ? nextTableName : table;
  const targetTableRef = quoteTableRef(database, targetTable, driver);

  if (hasRenameTable) {
    if (driver === "postgresql") {
      statements.push(`ALTER TABLE ${originalTableRef} RENAME TO ${quoteIdent(nextTableName, driver)}`);
    } else {
      statements.push(`RENAME TABLE ${originalTableRef} TO ${quoteTableRef(database, nextTableName, driver)}`);
    }
  }

  const originalMap = new Map(originalColumns.map((col) => [col.name, col]));
  const keptOriginalNames = new Set<string>();

  const additions: string[] = [];
  const renames: string[] = [];
  const modifications: string[] = [];
  const drops: string[] = [];
  const comments: string[] = [];
  const tableCommentChanged = normalizeComment(originalTableComment) !== normalizeComment(tableCommentDraft);

  for (const col of draftColumns) {
    const name = col.name.trim();
    if (!name) continue;

    if (!col.originalName || col.isNew) {
      additions.push(buildColumnDefinition(col, driver));
      if (driver === "postgresql" && normalizeComment(col.comment)) {
        comments.push(`COMMENT ON COLUMN ${targetTableRef}.${quoteIdent(name, driver)} IS ${sqlQuote(col.comment)}`);
      }
      continue;
    }

    keptOriginalNames.add(col.originalName);
    const original = originalMap.get(col.originalName);
    if (!original) continue;

    const typeChanged = original.type.trim().toLowerCase() !== col.type.trim().toLowerCase();
    const nullableChanged = original.nullable !== col.nullable;
    const defaultChanged = normalizeDefault(original.defaultValue) !== normalizeDefault(col.defaultValue);
    const commentChanged = normalizeComment(original.comment) !== normalizeComment(col.comment);
    const renamed = col.originalName !== name;

    if (driver === "postgresql") {
      const currentName = renamed ? name : col.originalName;

      if (renamed) {
        renames.push(
          `ALTER TABLE ${targetTableRef} RENAME COLUMN ${quoteIdent(col.originalName, driver)} TO ${quoteIdent(name, driver)}`
        );
      }

      if (typeChanged) {
        modifications.push(
          `ALTER TABLE ${targetTableRef} ALTER COLUMN ${quoteIdent(currentName, driver)} TYPE ${col.type.trim()}`
        );
      }

      if (nullableChanged) {
        modifications.push(
          col.nullable
            ? `ALTER TABLE ${targetTableRef} ALTER COLUMN ${quoteIdent(currentName, driver)} DROP NOT NULL`
            : `ALTER TABLE ${targetTableRef} ALTER COLUMN ${quoteIdent(currentName, driver)} SET NOT NULL`
        );
      }

      if (defaultChanged) {
        modifications.push(
          normalizeDefault(col.defaultValue)
            ? `ALTER TABLE ${targetTableRef} ALTER COLUMN ${quoteIdent(currentName, driver)} SET DEFAULT ${formatDefaultValue(
                col.defaultValue
              )}`
            : `ALTER TABLE ${targetTableRef} ALTER COLUMN ${quoteIdent(currentName, driver)} DROP DEFAULT`
        );
      }
      if (commentChanged) {
        comments.push(
          `COMMENT ON COLUMN ${targetTableRef}.${quoteIdent(currentName, driver)} IS ${
            normalizeComment(col.comment) ? sqlQuote(col.comment) : "NULL"
          }`
        );
      }
    } else {
      if (!renamed && !typeChanged && !nullableChanged && !defaultChanged && !commentChanged) continue;

      if (renamed) {
        modifications.push(
          `CHANGE COLUMN ${quoteIdent(col.originalName, driver)} ${buildColumnDefinition(col, driver, commentChanged)}`
        );
      } else {
        modifications.push(`MODIFY COLUMN ${buildColumnDefinition(col, driver, commentChanged)}`);
      }
    }
  }

  for (const original of originalColumns) {
    if (!keptOriginalNames.has(original.name)) {
      drops.push(quoteIdent(original.name, driver));
    }
  }

  if (driver === "postgresql") {
    if (additions.length > 0) {
      for (const definition of additions) {
        statements.push(`ALTER TABLE ${targetTableRef} ADD COLUMN ${definition}`);
      }
    }

    statements.push(...renames);
    statements.push(...modifications);

    if (drops.length > 0) {
      for (const dropName of drops) {
        statements.push(`ALTER TABLE ${targetTableRef} DROP COLUMN ${dropName}`);
      }
    }

    if (tableCommentChanged) {
      statements.push(
        `COMMENT ON TABLE ${targetTableRef} IS ${normalizeComment(tableCommentDraft) ? sqlQuote(tableCommentDraft) : "NULL"}`
      );
    }
    statements.push(...comments);
  } else {
    const clauses: string[] = [];
    if (additions.length > 0) {
      for (const definition of additions) {
        clauses.push(`ADD COLUMN ${definition}`);
      }
    }
    clauses.push(...modifications);
    if (drops.length > 0) {
      for (const dropName of drops) {
        clauses.push(`DROP COLUMN ${dropName}`);
      }
    }

    if (tableCommentChanged) {
      clauses.push(`COMMENT = ${sqlQuote(tableCommentDraft)}`);
    }

    if (clauses.length > 0) {
      statements.push(`ALTER TABLE ${targetTableRef} ${clauses.join(", ")}`);
    }
  }

  return { statements, nextTableName: hasRenameTable ? nextTableName : undefined };
}

export type CellValueFilterOperator = TableFilterOperator;

function toRangeValues(value: unknown): [unknown, unknown] | null {
  if (Array.isArray(value) && value.length >= 2) return [value[0], value[1]];
  if (value !== undefined && value !== null) return [value, value];
  return null;
}

function toListValues(value: unknown): unknown[] {
  if (Array.isArray(value)) return value.filter((item) => item !== undefined);
  if (typeof value === "string") {
    return value
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean);
  }
  return value !== undefined ? [value] : [];
}

export function buildFilterByCellValueClause(
  col: string,
  value: unknown,
  driver?: string,
  operator: CellValueFilterOperator = "="
): string {
  const quotedCol = quoteIdent(col, driver);
  if (operator === "is_null") return `${quotedCol} IS NULL`;
  if (operator === "is_not_null") return `${quotedCol} IS NOT NULL`;
  if (operator === "is_empty") return `(${quotedCol} IS NULL OR ${quotedCol} = '')`;
  if (operator === "is_not_empty") return `(${quotedCol} IS NOT NULL AND ${quotedCol} <> '')`;

  if (value == null) {
    if (operator === "!=") return `${quotedCol} IS NOT NULL`;
    if (operator === "=") return `${quotedCol} IS NULL`;
    return "";
  }
  if (operator === "contains" || operator === "like") return `${quotedCol} LIKE ${sqlQuote(`%${String(value)}%`)}`;
  if (operator === "not_contains" || operator === "not_like") {
    return `${quotedCol} NOT LIKE ${sqlQuote(`%${String(value)}%`)}`;
  }
  if (operator === "begins_with") return `${quotedCol} LIKE ${sqlQuote(`${String(value)}%`)}`;
  if (operator === "not_begins_with") return `${quotedCol} NOT LIKE ${sqlQuote(`${String(value)}%`)}`;
  if (operator === "ends_with") return `${quotedCol} LIKE ${sqlQuote(`%${String(value)}`)}`;
  if (operator === "not_ends_with") return `${quotedCol} NOT LIKE ${sqlQuote(`%${String(value)}`)}`;
  if (operator === "between" || operator === "not_between") {
    const rangeValues = toRangeValues(value);
    if (!rangeValues) return "";
    return `${quotedCol} ${operator === "not_between" ? "NOT " : ""}BETWEEN ${sqlQuote(rangeValues[0])} AND ${sqlQuote(rangeValues[1])}`;
  }
  if (operator === "in_list" || operator === "not_in_list") {
    const listValues = toListValues(value);
    if (listValues.length === 0) return "";
    return `${quotedCol} ${operator === "not_in_list" ? "NOT " : ""}IN (${listValues.map(sqlQuote).join(", ")})`;
  }
  return `${quotedCol} ${operator === "!=" ? "<>" : operator} ${sqlQuote(value)}`;
}

export interface BuildDeleteStatementArgs {
  database: string;
  table: string;
  columns: string[];
  row: Record<string, unknown>;
  primaryKeys: string[];
  driver?: string;
}

export interface DeleteStatement {
  sql: string;
  usesPrimaryKey: boolean;
}

export function buildDeleteStatement({
  database,
  table,
  columns,
  row,
  primaryKeys,
  driver,
}: BuildDeleteStatementArgs): DeleteStatement {
  const usesPrimaryKey = primaryKeys.length > 0;
  const whereCols = usesPrimaryKey ? primaryKeys : columns;
  const whereClauses = whereCols.map((col) => {
    const value = row[col];
    if (value == null) return `${quoteIdent(col, driver)} IS NULL`;
    return `${quoteIdent(col, driver)} = ${sqlQuote(value)}`;
  });

  const tableName = quoteTableRef(database, table, driver);
  const whereSQL = whereClauses.join(" AND ");

  if (driver === "postgresql") {
    if (usesPrimaryKey) return { sql: `DELETE FROM ${tableName} WHERE ${whereSQL};`, usesPrimaryKey };
    return {
      sql: `DELETE FROM ${tableName} WHERE ctid = (SELECT ctid FROM ${tableName} WHERE ${whereSQL} LIMIT 1);`,
      usesPrimaryKey,
    };
  }

  return { sql: `DELETE FROM ${tableName} WHERE ${whereSQL} LIMIT 1;`, usesPrimaryKey };
}
