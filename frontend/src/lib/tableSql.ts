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
