import { quoteIdent, sqlQuote } from "./tableSql";

type TableRow = Record<string, unknown>;
export type TableExportFormat = "csv" | "tsv" | "sql";
export type TableExportScope = "page" | "all";
export type TableExportSortDir = "asc" | "desc" | null;

interface DelimitedExportOptions {
  includeHeaders?: boolean;
}

interface BuildTableExportSelectSqlInput {
  database: string;
  table: string;
  driver?: string;
  scope: TableExportScope;
  whereClause: string;
  orderByClause: string;
  sortColumn: string | null;
  sortDir: TableExportSortDir;
  page: number;
  pageSize: number;
}

interface BuildTableExportContentInput {
  format: TableExportFormat;
  columns: string[];
  rows: TableRow[];
  tableName: string;
  driver?: string;
  includeHeaders?: boolean;
}

function cellText(value: unknown): string {
  if (value == null) return "";
  return String(value);
}

function escapeDelimited(value: unknown, delimiter: string): string {
  const text = cellText(value);
  if (!text.includes(delimiter) && !text.includes('"') && !text.includes("\n") && !text.includes("\r")) {
    return text;
  }
  return `"${text.replace(/"/g, '""')}"`;
}

function toDelimited(
  columns: string[],
  rows: TableRow[],
  delimiter: string,
  options: DelimitedExportOptions = {}
): string {
  const includeHeaders = options.includeHeaders ?? true;
  const lines = includeHeaders ? [columns.map((col) => escapeDelimited(col, delimiter)).join(delimiter)] : [];
  for (const row of rows) {
    lines.push(columns.map((col) => escapeDelimited(row[col], delimiter)).join(delimiter));
  }
  return lines.join("\n");
}

function toDelimitedData(columns: string[], rows: TableRow[], delimiter: string): string {
  return rows.map((row) => columns.map((col) => escapeDelimited(row[col], delimiter)).join(delimiter)).join("\n");
}

function quoteTableName(tableName: string, driver?: string): string {
  return tableName
    .split(".")
    .filter(Boolean)
    .map((part) => quoteIdent(part, driver))
    .join(".");
}

export function toTsv(columns: string[], rows: TableRow[], options?: DelimitedExportOptions): string {
  return toDelimited(columns, rows, "\t", options);
}

export function toCsv(columns: string[], rows: TableRow[], options?: DelimitedExportOptions): string {
  return toDelimited(columns, rows, ",", options);
}

export function toTsvData(columns: string[], rows: TableRow[]): string {
  return toDelimitedData(columns, rows, "\t");
}

export function toTsvFields(columns: string[]): string {
  return columns.map((col) => escapeDelimited(col, "\t")).join("\t");
}

export function toInsertSql(tableName: string, columns: string[], rows: TableRow[], driver?: string): string {
  const quotedTable = quoteTableName(tableName, driver);
  const columnSql = columns.map((col) => quoteIdent(col, driver)).join(", ");
  return rows
    .map((row) => {
      const values = columns.map((col) => sqlQuote(row[col])).join(", ");
      return `INSERT INTO ${quotedTable} (${columnSql}) VALUES (${values});`;
    })
    .join("\n");
}

export function toUpdateSql(
  tableName: string,
  columns: string[],
  row: TableRow,
  primaryKeys: string[],
  driver?: string
): string {
  const quotedTable = quoteTableName(tableName, driver);
  const setSql = columns.map((col) => `${quoteIdent(col, driver)} = ${sqlQuote(row[col])}`).join(", ");
  const whereColumns = primaryKeys.length > 0 ? primaryKeys : columns;
  const whereSql = whereColumns
    .map((col) => {
      const value = row[col];
      if (value == null) return `${quoteIdent(col, driver)} IS NULL`;
      return `${quoteIdent(col, driver)} = ${sqlQuote(value)}`;
    })
    .join(" AND ");

  if (driver === "postgresql") return `UPDATE ${quotedTable} SET ${setSql} WHERE ${whereSql};`;
  return `UPDATE ${quotedTable} SET ${setSql} WHERE ${whereSql} LIMIT 1;`;
}

export function buildTableExportContent({
  format,
  columns,
  rows,
  tableName,
  driver,
  includeHeaders = true,
}: BuildTableExportContentInput): string {
  if (format === "csv") return toCsv(columns, rows, { includeHeaders });
  if (format === "tsv") return toTsv(columns, rows, { includeHeaders });
  return toInsertSql(tableName, columns, rows, driver);
}

export function safeTableExportFilenamePart(value: string): string {
  return value.replace(/[^a-z0-9._-]+/gi, "_").replace(/^_+|_+$/g, "") || "table";
}

export function buildTableExportSelectSql({
  database,
  table,
  driver,
  scope,
  whereClause,
  orderByClause,
  sortColumn,
  sortDir,
  page,
  pageSize,
}: BuildTableExportSelectSqlInput): string {
  const tableName =
    driver === "postgresql"
      ? quoteIdent(table, driver)
      : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
  const where = whereClause.trim();
  const orderBy =
    sortColumn && sortDir
      ? `${quoteIdent(sortColumn, driver)} ${sortDir === "asc" ? "ASC" : "DESC"}`
      : orderByClause.trim();
  const wherePart = where ? ` WHERE ${where}` : "";
  const orderByPart = orderBy ? ` ORDER BY ${orderBy}` : "";
  const pagePart = scope === "page" ? ` LIMIT ${pageSize} OFFSET ${page * pageSize}` : "";
  return `SELECT * FROM ${tableName}${wherePart}${orderByPart}${pagePart}`;
}
