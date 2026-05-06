import { quoteIdent, quoteQualifiedIdent, sqlQuote } from "./tableSql";

export type Delimiter = "," | "\t";
export type ImportNullStrategy = "empty-is-empty-string" | "empty-is-null" | "literal-null";
export type ImportDataFormat = "text" | "csv" | "json" | "xml";
export type ImportFieldDelimiter = "," | "\t" | ";" | "|" | " ";
export type ImportMode = "append" | "update" | "append-update" | "append-skip" | "delete" | "copy";
export type ImportRecordDelimiter = "auto" | "lf" | "cr" | "crlf";
export type ImportTextQualifier = '"' | "'" | "none";
export type ImportDateOrder = "dmy" | "mdy" | "ymd";
export type ImportDateTimeOrder = "date-time" | "time-date";
export type ImportBinaryEncoding = "base64" | "hex";

export interface ImportAdvancedOptions {
  extendedInsert?: boolean;
  maxStatementSizeKb?: number;
  emptyStringAsNull?: boolean;
  ignoreForeignKeyConstraint?: boolean;
}

export interface ImportValueConversionOptions {
  dateOrder?: ImportDateOrder;
  dateTimeOrder?: ImportDateTimeOrder;
  dateDelimiter?: string;
  yearDelimiter?: string;
  timeDelimiter?: string;
  decimalSymbol?: string;
  binaryEncoding?: ImportBinaryEncoding;
}

export interface ParsedDelimitedTable {
  headers: string[];
  rows: string[][];
}

export interface ParseImportSourceTextArgs {
  text: string;
  format: ImportDataFormat;
  fieldDelimiter?: ImportFieldDelimiter;
  recordDelimiter?: ImportRecordDelimiter;
  textQualifier?: ImportTextQualifier;
  fixedWidth?: boolean;
  fieldNameRowEnabled?: boolean;
  fieldNameRow?: number;
  dataStartRow?: number;
  dataEndRow?: number;
}

export interface BuildImportInsertSqlArgs {
  tableName: string;
  headers: string[];
  rows: string[][];
  mapping: Record<string, string>;
  nullStrategy: ImportNullStrategy;
  mode?: ImportMode;
  primaryKeys?: string[];
  advancedOptions?: ImportAdvancedOptions;
  driver?: string;
  columnTypes?: Record<string, string>;
  conversionOptions?: ImportValueConversionOptions;
}

export function detectDelimiter(text: string): Delimiter {
  const firstLine = text.split(/\r?\n/, 1)[0] ?? "";
  const tabs = (firstLine.match(/\t/g) ?? []).length;
  const commas = (firstLine.match(/,/g) ?? []).length;
  return tabs > commas ? "\t" : ",";
}

interface NewlineMatch {
  match: boolean;
  len: number;
}

function newlineAt(text: string, i: number, mode: ImportRecordDelimiter): NewlineMatch {
  const ch = text[i];
  const next = text[i + 1];
  if (mode === "lf") return { match: ch === "\n", len: 1 };
  if (mode === "cr") return { match: ch === "\r" && next !== "\n", len: 1 };
  if (mode === "crlf") return { match: ch === "\r" && next === "\n", len: 2 };
  if (ch === "\r" && next === "\n") return { match: true, len: 2 };
  if (ch === "\n" || ch === "\r") return { match: true, len: 1 };
  return { match: false, len: 0 };
}

interface ParseDelimitedRowsOptions {
  recordDelimiter?: ImportRecordDelimiter;
  textQualifier?: ImportTextQualifier;
}

function parseDelimitedRows(
  text: string,
  delimiter: ImportFieldDelimiter = detectDelimiter(text),
  options: ParseDelimitedRowsOptions = {}
): string[][] {
  const recordDelimiter = options.recordDelimiter ?? "auto";
  const qualifier = options.textQualifier ?? '"';
  const useQualifier = qualifier !== "none";

  const rows: string[][] = [];
  let currentRow: string[] = [];
  let current = "";
  let inQuotes = false;

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const next = text[i + 1];

    if (useQualifier && ch === qualifier) {
      if (inQuotes && next === qualifier) {
        current += qualifier;
        i++;
      } else {
        inQuotes = !inQuotes;
      }
      continue;
    }

    if (ch === delimiter && !inQuotes) {
      currentRow.push(current);
      current = "";
      continue;
    }

    if (!inQuotes) {
      const nl = newlineAt(text, i, recordDelimiter);
      if (nl.match) {
        i += nl.len - 1;
        currentRow.push(current);
        if (currentRow.some((cell) => cell !== "")) rows.push(currentRow);
        currentRow = [];
        current = "";
        continue;
      }
    }

    current += ch;
  }

  currentRow.push(current);
  if (currentRow.some((cell) => cell !== "")) rows.push(currentRow);

  return rows;
}

export function parseDelimitedText(text: string, delimiter: Delimiter = detectDelimiter(text)): ParsedDelimitedTable {
  const rows = parseDelimitedRows(text, delimiter);
  const headers = rows[0] ?? [];
  return { headers, rows: rows.slice(1) };
}

function normalizeCell(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
  return JSON.stringify(value);
}

function tableFromRecords(records: Record<string, unknown>[]): ParsedDelimitedTable {
  const headers: string[] = [];
  for (const record of records) {
    for (const key of Object.keys(record)) {
      if (!headers.includes(key)) headers.push(key);
    }
  }

  return {
    headers,
    rows: records.map((record) => headers.map((header) => normalizeCell(record[header]))),
  };
}

function parseJsonText(text: string): ParsedDelimitedTable {
  const data = JSON.parse(text) as unknown;
  let rows: unknown[] = [];

  if (Array.isArray(data)) {
    rows = data;
  } else if (data && typeof data === "object") {
    const firstArray = Object.values(data).find(Array.isArray);
    rows = firstArray ?? [data];
  }

  const records = rows.map((row) =>
    row && typeof row === "object" && !Array.isArray(row) ? row : { value: row }
  ) as Record<string, unknown>[];
  return tableFromRecords(records);
}

function elementChildren(element: Element): Element[] {
  return Array.from(element.children);
}

function elementToRecord(element: Element): Record<string, unknown> {
  const record: Record<string, unknown> = {};
  for (const attr of Array.from(element.attributes)) {
    record[`@${attr.name}`] = attr.value;
  }

  for (const child of elementChildren(element)) {
    const value = child.children.length > 0 ? elementToRecord(child) : (child.textContent?.trim() ?? "");
    if (record[child.tagName]) {
      const existing = record[child.tagName];
      record[child.tagName] = Array.isArray(existing) ? [...existing, value] : [existing, value];
    } else {
      record[child.tagName] = value;
    }
  }

  if (Object.keys(record).length === 0) record.value = element.textContent?.trim() ?? "";
  return record;
}

function parseXmlText(text: string): ParsedDelimitedTable {
  const parser = new DOMParser();
  const doc = parser.parseFromString(text, "application/xml");
  const parseError = doc.querySelector("parsererror");
  if (parseError) throw new Error(parseError.textContent ?? "Invalid XML");

  const root = doc.documentElement;
  if (!root) return { headers: [], rows: [] };

  const children = elementChildren(root);
  const byName = new Map<string, Element[]>();
  for (const child of children) {
    byName.set(child.tagName, [...(byName.get(child.tagName) ?? []), child]);
  }
  const repeated = Array.from(byName.values())
    .filter((items) => items.length > 1)
    .sort((a, b) => b.length - a.length)[0];
  const rowElements =
    repeated ?? (children.length > 0 && children.every((child) => child.children.length > 0) ? children : [root]);

  return tableFromRecords(rowElements.map(elementToRecord));
}

function parseFixedWidthRows(text: string): string[][] {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => line.split(/\s{2,}|\t+/).filter(Boolean));
}

function applyRowOptions(rows: string[][], args: ParseImportSourceTextArgs): ParsedDelimitedTable {
  const hasHeader = args.fieldNameRowEnabled ?? true;
  const headerIndex = Math.max((args.fieldNameRow ?? 1) - 1, 0);
  const dataStartIndex = Math.max((args.dataStartRow ?? (hasHeader ? headerIndex + 2 : 1)) - 1, 0);
  const dataEndIndex = args.dataEndRow ? Math.max(args.dataEndRow, dataStartIndex + 1) : undefined;
  const maxColumns = Math.max(...rows.map((row) => row.length), 0);
  const headers = hasHeader
    ? (rows[headerIndex] ?? [])
    : Array.from({ length: maxColumns }, (_, index) => `Column ${index + 1}`);

  return {
    headers,
    rows: rows.slice(dataStartIndex, dataEndIndex),
  };
}

export function parseImportSourceText(args: ParseImportSourceTextArgs): ParsedDelimitedTable {
  if (args.format === "json") return parseJsonText(args.text);
  if (args.format === "xml") return parseXmlText(args.text);

  const rows = args.fixedWidth
    ? parseFixedWidthRows(args.text)
    : parseDelimitedRows(args.text, args.fieldDelimiter, {
        recordDelimiter: args.recordDelimiter,
        textQualifier: args.textQualifier,
      });
  return applyRowOptions(rows, args);
}

function importValue(cell: string, nullStrategy: ImportNullStrategy): unknown {
  if (nullStrategy === "empty-is-null" && cell === "") return null;
  if (nullStrategy === "literal-null" && cell.toUpperCase() === "NULL") return null;
  return cell;
}

const MONTH_NAMES: Record<string, number> = {
  jan: 1,
  january: 1,
  feb: 2,
  february: 2,
  mar: 3,
  march: 3,
  apr: 4,
  april: 4,
  may: 5,
  jun: 6,
  june: 6,
  jul: 7,
  july: 7,
  aug: 8,
  august: 8,
  sep: 9,
  sept: 9,
  september: 9,
  oct: 10,
  october: 10,
  nov: 11,
  november: 11,
  dec: 12,
  december: 12,
};

function isNumericType(type: string): boolean {
  return /^(tinyint|smallint|mediumint|int|bigint|integer|float|double|decimal|numeric|real)/i.test(type);
}

function isDateOnlyType(type: string): boolean {
  return /^date(?!time)/i.test(type);
}

function isDateTimeType(type: string): boolean {
  return /^(datetime|timestamp)/i.test(type);
}

function isBinaryType(type: string): boolean {
  return /^(blob|tinyblob|mediumblob|longblob|binary|varbinary|bytea)/i.test(type);
}

function pad(value: number, width: number): string {
  return String(value).padStart(width, "0");
}

function expandTwoDigitYear(text: string): number | null {
  const trimmed = text.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const n = Number(trimmed);
  if (trimmed.length <= 2) return n + (n < 70 ? 2000 : 1900);
  return n;
}

function parseMonth(text: string): number | null {
  const trimmed = text.trim().toLowerCase();
  if (trimmed in MONTH_NAMES) return MONTH_NAMES[trimmed];
  if (!/^\d+$/.test(trimmed)) return null;
  const n = Number(trimmed);
  if (n < 1 || n > 12) return null;
  return n;
}

function splitDateParts(
  text: string,
  order: ImportDateOrder,
  dateDelimiter: string,
  yearDelimiter?: string
): [string, string, string] | null {
  const trimmed = text.trim();
  if (!trimmed) return null;
  if (yearDelimiter && yearDelimiter !== dateDelimiter) {
    if (order === "ymd") {
      const idx = trimmed.indexOf(yearDelimiter);
      if (idx === -1) return null;
      const rest = trimmed.slice(idx + yearDelimiter.length).split(dateDelimiter);
      if (rest.length !== 2) return null;
      return [trimmed.slice(0, idx), rest[0], rest[1]];
    }
    const idx = trimmed.lastIndexOf(yearDelimiter);
    if (idx === -1) return null;
    const rest = trimmed.slice(0, idx).split(dateDelimiter);
    if (rest.length !== 2) return null;
    return [rest[0], rest[1], trimmed.slice(idx + yearDelimiter.length)];
  }
  const parts = trimmed.split(dateDelimiter);
  if (parts.length !== 3) return null;
  return [parts[0], parts[1], parts[2]];
}

function parseImportDate(text: string, options?: ImportValueConversionOptions): string | null {
  const order = options?.dateOrder ?? "ymd";
  const delim = options?.dateDelimiter && options.dateDelimiter.length > 0 ? options.dateDelimiter : "-";
  const yearDelim = options?.yearDelimiter && options.yearDelimiter.length > 0 ? options.yearDelimiter : undefined;

  const parts = splitDateParts(text, order, delim, yearDelim);
  if (!parts) return null;

  let yearText: string;
  let monthText: string;
  let dayText: string;
  if (order === "dmy") {
    [dayText, monthText, yearText] = parts;
  } else if (order === "mdy") {
    [monthText, dayText, yearText] = parts;
  } else {
    [yearText, monthText, dayText] = parts;
  }

  const y = expandTwoDigitYear(yearText);
  const m = parseMonth(monthText);
  const dTrim = dayText.trim();
  const d = /^\d+$/.test(dTrim) ? Number(dTrim) : NaN;
  if (y == null || m == null || !Number.isInteger(d) || d < 1 || d > 31) return null;
  return `${pad(y, 4)}-${pad(m, 2)}-${pad(d, 2)}`;
}

function parseImportTime(text: string, options?: ImportValueConversionOptions): string | null {
  const delim = options?.timeDelimiter && options.timeDelimiter.length > 0 ? options.timeDelimiter : ":";
  const trimmed = text.trim();
  if (!trimmed) return null;
  const parts = trimmed.split(delim);
  if (parts.length < 2 || parts.length > 3) return null;
  const h = /^\d+$/.test(parts[0]) ? Number(parts[0]) : NaN;
  const m = /^\d+$/.test(parts[1]) ? Number(parts[1]) : NaN;
  const sRaw = parts[2] ?? "0";
  const s = /^\d+$/.test(sRaw) ? Number(sRaw) : NaN;
  if (
    !Number.isInteger(h) ||
    !Number.isInteger(m) ||
    !Number.isInteger(s) ||
    h < 0 ||
    h > 23 ||
    m < 0 ||
    m > 59 ||
    s < 0 ||
    s > 59
  ) {
    return null;
  }
  return `${pad(h, 2)}:${pad(m, 2)}:${pad(s, 2)}`;
}

function parseImportDateTime(text: string, options?: ImportValueConversionOptions): string | null {
  const trimmed = text.trim();
  if (!trimmed) return null;
  const idx = trimmed.search(/\s/);
  if (idx === -1) {
    const date = parseImportDate(trimmed, options);
    return date ? `${date} 00:00:00` : null;
  }
  const left = trimmed.slice(0, idx);
  const right = trimmed.slice(idx + 1).trim();
  const order = options?.dateTimeOrder ?? "date-time";
  const datePart = order === "time-date" ? right : left;
  const timePart = order === "time-date" ? left : right;
  const date = parseImportDate(datePart, options);
  if (!date) return null;
  const time = parseImportTime(timePart, options);
  if (!time) return null;
  return `${date} ${time}`;
}

function bytesToHex(binary: string): string {
  let hex = "";
  for (let i = 0; i < binary.length; i++) {
    hex += binary.charCodeAt(i).toString(16).padStart(2, "0");
  }
  return hex;
}

function formatBinaryLiteral(text: string, encoding: ImportBinaryEncoding, driver?: string): string | null {
  const cleaned = text.replace(/\s+/g, "");
  let hex: string;
  if (encoding === "base64") {
    if (!cleaned) return driver === "postgresql" ? `'\\x'::bytea` : `X''`;
    if (!/^[A-Za-z0-9+/=]+$/.test(cleaned)) return null;
    try {
      hex = bytesToHex(atob(cleaned));
    } catch {
      return null;
    }
  } else {
    if (!/^[0-9a-fA-F]*$/.test(cleaned)) return null;
    hex = cleaned.toLowerCase();
    if (hex.length % 2 !== 0) hex = `0${hex}`;
  }
  if (driver === "postgresql") return `'\\x${hex}'::bytea`;
  return `X'${hex}'`;
}

function emitImportLiteral(
  cell: string,
  target: string,
  nullStrategy: ImportNullStrategy,
  columnTypes: Record<string, string> | undefined,
  conversion: ImportValueConversionOptions | undefined,
  driver: string | undefined
): string {
  const value = importValue(cell, nullStrategy);
  if (value == null) return "NULL";
  const text = String(value);
  const type = columnTypes?.[target];

  if (type) {
    if (isBinaryType(type) && conversion?.binaryEncoding) {
      const literal = formatBinaryLiteral(text, conversion.binaryEncoding, driver);
      if (literal) return literal;
    }
    if (isDateOnlyType(type)) {
      const iso = parseImportDate(text, conversion);
      if (iso) return sqlQuote(iso);
    } else if (isDateTimeType(type)) {
      const iso = parseImportDateTime(text, conversion);
      if (iso) return sqlQuote(iso);
    } else if (isNumericType(type) && conversion?.decimalSymbol && conversion.decimalSymbol !== ".") {
      const normalized = text.split(conversion.decimalSymbol).join(".");
      return sqlQuote(normalized);
    }
  }

  return sqlQuote(value);
}

type LiteralEmitter = (cell: string, target: string) => string;

function whereClauseForRow(
  row: string[],
  mapped: { source: string; index: number; target: string }[],
  emit: LiteralEmitter,
  driver?: string
): string {
  return mapped
    .map((item) => {
      const literal = emit(row[item.index] ?? "", item.target);
      const column = quoteIdent(item.target, driver);
      return literal === "NULL" ? `${column} IS NULL` : `${column} = ${literal}`;
    })
    .join(" AND ");
}

function chunkExtendedInsertStatements({
  insertPrefix,
  valuesSql,
  suffix,
  maxStatementSize,
}: {
  insertPrefix: string;
  valuesSql: string[];
  suffix: string;
  maxStatementSize: number;
}): string[] {
  const statements: string[] = [];
  let chunk: string[] = [];

  for (const valueSql of valuesSql) {
    const candidate = [...chunk, valueSql];
    const statement = `${insertPrefix} ${candidate.join(", ")}${suffix};`;
    if (chunk.length > 0 && statement.length > maxStatementSize) {
      statements.push(`${insertPrefix} ${chunk.join(", ")}${suffix};`);
      chunk = [valueSql];
    } else {
      chunk = candidate;
    }
  }

  if (chunk.length > 0) statements.push(`${insertPrefix} ${chunk.join(", ")}${suffix};`);
  return statements;
}

export function buildImportInsertSql({
  tableName,
  headers,
  rows,
  mapping,
  nullStrategy,
  mode = "append",
  primaryKeys = [],
  advancedOptions,
  driver,
  columnTypes,
  conversionOptions,
}: BuildImportInsertSqlArgs): string[] {
  const effectiveNullStrategy = advancedOptions?.emptyStringAsNull ? "empty-is-null" : nullStrategy;
  const mapped = headers
    .map((source, index) => ({ source, index, target: mapping[source] }))
    .filter((item): item is { source: string; index: number; target: string } => !!item.target);

  if (mapped.length === 0) return [];

  const quotedTable = quoteQualifiedIdent(tableName, driver);
  const columnSql = mapped.map((item) => quoteIdent(item.target, driver)).join(", ");
  const primaryKeySet = new Set(primaryKeys);
  const keyMapped = mapped.filter((item) => primaryKeySet.has(item.target));
  const valueMapped = mapped.filter((item) => !primaryKeySet.has(item.target));

  if (
    (mode === "update" || mode === "append-update" || mode === "append-skip" || mode === "delete") &&
    keyMapped.length === 0
  ) {
    return [];
  }

  const emit: LiteralEmitter = (cell, target) =>
    emitImportLiteral(cell, target, effectiveNullStrategy, columnTypes, conversionOptions, driver);

  const rowValuesSql = rows.map(
    (row) => `(${mapped.map((item) => emit(row[item.index] ?? "", item.target)).join(", ")})`
  );

  const buildInsertStatements = (insertKeyword: string, suffix = "") => {
    if (advancedOptions?.extendedInsert) {
      return chunkExtendedInsertStatements({
        insertPrefix: `${insertKeyword} ${quotedTable} (${columnSql}) VALUES`,
        valuesSql: rowValuesSql,
        suffix,
        maxStatementSize: Math.max(1, advancedOptions.maxStatementSizeKb ?? 1024) * 1024,
      });
    }

    return rowValuesSql.map((values) => `${insertKeyword} ${quotedTable} (${columnSql}) VALUES ${values}${suffix};`);
  };

  let statements: string[];
  if (mode === "update") {
    if (valueMapped.length === 0) return [];
    statements = rows.map((row) => {
      const setSql = valueMapped
        .map((item) => `${quoteIdent(item.target, driver)} = ${emit(row[item.index] ?? "", item.target)}`)
        .join(", ");
      return `UPDATE ${quotedTable} SET ${setSql} WHERE ${whereClauseForRow(row, keyMapped, emit, driver)};`;
    });
  } else if (mode === "append-update") {
    const insertPrefix = "INSERT INTO";
    if (valueMapped.length === 0) {
      if (driver === "postgresql") {
        const suffix = ` ON CONFLICT (${keyMapped.map((item) => quoteIdent(item.target, driver)).join(", ")}) DO NOTHING`;
        statements = buildInsertStatements(insertPrefix, suffix);
      } else {
        statements = buildInsertStatements("INSERT IGNORE INTO");
      }
    } else {
      const updateSql =
        driver === "postgresql"
          ? ` ON CONFLICT (${keyMapped.map((item) => quoteIdent(item.target, driver)).join(", ")}) DO UPDATE SET ${valueMapped
              .map((item) => `${quoteIdent(item.target, driver)} = excluded.${quoteIdent(item.target, driver)}`)
              .join(", ")}`
          : ` ON DUPLICATE KEY UPDATE ${valueMapped
              .map((item) => `${quoteIdent(item.target, driver)} = VALUES(${quoteIdent(item.target, driver)})`)
              .join(", ")}`;
      statements = buildInsertStatements(insertPrefix, updateSql);
    }
  } else if (mode === "append-skip") {
    if (driver === "postgresql") {
      const suffix = ` ON CONFLICT (${keyMapped.map((item) => quoteIdent(item.target, driver)).join(", ")}) DO NOTHING`;
      statements = buildInsertStatements("INSERT INTO", suffix);
    } else {
      statements = buildInsertStatements("INSERT IGNORE INTO");
    }
  } else if (mode === "delete") {
    statements = rows.map(
      (row) => `DELETE FROM ${quotedTable} WHERE ${whereClauseForRow(row, keyMapped, emit, driver)};`
    );
  } else {
    statements = buildInsertStatements("INSERT INTO");
    if (mode === "copy") statements = [`DELETE FROM ${quotedTable};`, ...statements];
  }

  return statements;
}
