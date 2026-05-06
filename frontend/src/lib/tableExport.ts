import { quoteIdent, quoteQualifiedIdent, quoteTableRef, sqlQuote } from "./tableSql";

type TableRow = Record<string, unknown>;
export type TableExportFormat = "csv" | "tsv" | "sql";
export type TableExportScope = "page" | "all";
export type TableExportSortDir = "asc" | "desc" | null;
export type TableExportRecordDelimiter = "lf" | "crlf";
export type TableExportFieldDelimiter = "comma" | "tab" | "semicolon" | "pipe";
export type TableExportTextQualifier = "double" | "single" | "none";
export type TableExportDateOrder = "ymd" | "dmy" | "mdy";
export type TableExportBinaryEncoding = "base64" | "hex";

export interface TableExportOptions {
  includeHeaders?: boolean;
  append?: boolean;
  continueOnError?: boolean;
  recordDelimiter?: TableExportRecordDelimiter;
  fieldDelimiter?: TableExportFieldDelimiter;
  textQualifier?: TableExportTextQualifier;
  blankIfZero?: boolean;
  zeroPaddingDate?: boolean;
  dateOrder?: TableExportDateOrder;
  dateDelimiter?: string;
  timeDelimiter?: string;
  decimalSymbol?: "." | ",";
  binaryDataEncoding?: TableExportBinaryEncoding;
  nullValue?: string;
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
  options?: TableExportOptions;
}

interface DateParts {
  year: number;
  month: number;
  day: number;
  hour?: number;
  minute?: number;
  second?: number;
}

function fieldDelimiterValue(kind: TableExportFieldDelimiter | undefined, fallback: string): string {
  if (kind === "tab") return "\t";
  if (kind === "semicolon") return ";";
  if (kind === "pipe") return "|";
  if (kind === "comma") return ",";
  return fallback;
}

function recordDelimiterValue(kind: TableExportRecordDelimiter | undefined): string {
  return kind === "crlf" ? "\r\n" : "\n";
}

function textQualifierValue(kind: TableExportTextQualifier | undefined): string {
  if (kind === "single") return "'";
  if (kind === "none") return "";
  return '"';
}

function encodeBinary(value: ArrayBufferView, encoding: TableExportBinaryEncoding | undefined): string {
  const bytes = new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  if (encoding === "hex") return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function parseDateParts(value: unknown): DateParts | null {
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return {
      year: value.getFullYear(),
      month: value.getMonth() + 1,
      day: value.getDate(),
      hour: value.getHours(),
      minute: value.getMinutes(),
      second: value.getSeconds(),
    };
  }
  if (typeof value !== "string") return null;
  const match = value.trim().match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[ T](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/);
  if (!match) return null;
  return {
    year: Number(match[1]),
    month: Number(match[2]),
    day: Number(match[3]),
    hour: match[4] == null ? undefined : Number(match[4]),
    minute: match[5] == null ? undefined : Number(match[5]),
    second: match[6] == null ? undefined : Number(match[6]),
  };
}

function formatDateParts(parts: DateParts, options: TableExportOptions): string {
  const pad = options.zeroPaddingDate ?? true;
  const dateDelimiter = options.dateDelimiter ?? "-";
  const timeDelimiter = options.timeDelimiter ?? ":";
  const value = (n: number) => (pad ? String(n).padStart(2, "0") : String(n));
  const dateParts = {
    y: String(parts.year),
    m: value(parts.month),
    d: value(parts.day),
  };
  const order = options.dateOrder ?? "ymd";
  const date =
    order === "dmy"
      ? [dateParts.d, dateParts.m, dateParts.y].join(dateDelimiter)
      : order === "mdy"
        ? [dateParts.m, dateParts.d, dateParts.y].join(dateDelimiter)
        : [dateParts.y, dateParts.m, dateParts.d].join(dateDelimiter);
  if (parts.hour == null || parts.minute == null) return date;
  const time = [value(parts.hour), value(parts.minute), value(parts.second ?? 0)].join(timeDelimiter);
  return `${date} ${time}`;
}

function cellText(value: unknown, options: TableExportOptions = {}): string {
  if (value == null) return options.nullValue ?? "";
  if (ArrayBuffer.isView(value)) return encodeBinary(value, options.binaryDataEncoding);
  if (typeof value === "number") {
    if (options.blankIfZero && Object.is(value, 0)) return "";
    const text = String(value);
    return options.decimalSymbol === "," ? text.replace(".", ",") : text;
  }
  const dateParts = parseDateParts(value);
  if (dateParts) return formatDateParts(dateParts, options);
  return String(value);
}

function escapeDelimited(value: unknown, delimiter: string, options: TableExportOptions = {}): string {
  const text = cellText(value, options);
  const qualifier = textQualifierValue(options.textQualifier);
  if (!qualifier) return text;
  if (!text.includes(delimiter) && !text.includes(qualifier) && !text.includes("\n") && !text.includes("\r")) {
    return text;
  }
  return `${qualifier}${text.split(qualifier).join(qualifier + qualifier)}${qualifier}`;
}

function toDelimited(columns: string[], rows: TableRow[], delimiter: string, options: TableExportOptions = {}): string {
  const includeHeaders = options.includeHeaders ?? true;
  const actualDelimiter = fieldDelimiterValue(options.fieldDelimiter, delimiter);
  const recordDelimiter = recordDelimiterValue(options.recordDelimiter);
  const lines = includeHeaders
    ? [columns.map((col) => escapeDelimited(col, actualDelimiter, options)).join(actualDelimiter)]
    : [];
  for (const row of rows) {
    lines.push(columns.map((col) => escapeDelimited(row[col], actualDelimiter, options)).join(actualDelimiter));
  }
  return lines.join(recordDelimiter);
}

function toDelimitedData(columns: string[], rows: TableRow[], delimiter: string): string {
  return rows.map((row) => columns.map((col) => escapeDelimited(row[col], delimiter)).join(delimiter)).join("\n");
}

export function toTsv(columns: string[], rows: TableRow[], options?: TableExportOptions): string {
  return toDelimited(columns, rows, "\t", options);
}

export function toCsv(columns: string[], rows: TableRow[], options?: TableExportOptions): string {
  return toDelimited(columns, rows, ",", options);
}

export function toTsvData(columns: string[], rows: TableRow[]): string {
  return toDelimitedData(columns, rows, "\t");
}

export function toTsvFields(columns: string[]): string {
  return columns.map((col) => escapeDelimited(col, "\t")).join("\t");
}

export function toInsertSql(tableName: string, columns: string[], rows: TableRow[], driver?: string): string {
  const quotedTable = quoteQualifiedIdent(tableName, driver);
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
  const quotedTable = quoteQualifiedIdent(tableName, driver);
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
  options = {},
}: BuildTableExportContentInput): string {
  const mergedOptions = { ...options, includeHeaders };
  if (format === "csv") return toCsv(columns, rows, mergedOptions);
  if (format === "tsv") return toTsv(columns, rows, mergedOptions);
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
  const tableName = quoteTableRef(database, table, driver);
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
