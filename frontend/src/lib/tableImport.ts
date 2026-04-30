import { quoteIdent, quoteQualifiedIdent, sqlQuote } from "./tableSql";

export type Delimiter = "," | "\t";
export type ImportNullStrategy = "empty-is-empty-string" | "empty-is-null" | "literal-null";
export type ImportDataFormat = "text" | "csv" | "json" | "xml";
export type ImportFieldDelimiter = "," | "\t" | ";" | "|" | " ";
export type ImportMode = "append" | "update" | "append-update" | "append-skip" | "delete" | "copy";

export interface ImportAdvancedOptions {
  extendedInsert?: boolean;
  maxStatementSizeKb?: number;
  emptyStringAsNull?: boolean;
  ignoreForeignKeyConstraint?: boolean;
}

export interface ParsedDelimitedTable {
  headers: string[];
  rows: string[][];
}

export interface ParseImportSourceTextArgs {
  text: string;
  format: ImportDataFormat;
  fieldDelimiter?: ImportFieldDelimiter;
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
}

export function detectDelimiter(text: string): Delimiter {
  const firstLine = text.split(/\r?\n/, 1)[0] ?? "";
  const tabs = (firstLine.match(/\t/g) ?? []).length;
  const commas = (firstLine.match(/,/g) ?? []).length;
  return tabs > commas ? "\t" : ",";
}

function parseDelimitedRows(text: string, delimiter: ImportFieldDelimiter = detectDelimiter(text)): string[][] {
  const rows: string[][] = [];
  let currentRow: string[] = [];
  let current = "";
  let inQuotes = false;

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const next = text[i + 1];

    if (ch === '"') {
      if (inQuotes && next === '"') {
        current += '"';
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

    if ((ch === "\n" || ch === "\r") && !inQuotes) {
      if (ch === "\r" && next === "\n") i++;
      currentRow.push(current);
      if (currentRow.some((cell) => cell !== "")) rows.push(currentRow);
      currentRow = [];
      current = "";
      continue;
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

  const rows = args.fixedWidth ? parseFixedWidthRows(args.text) : parseDelimitedRows(args.text, args.fieldDelimiter);
  return applyRowOptions(rows, args);
}

function importValue(cell: string, nullStrategy: ImportNullStrategy): unknown {
  if (nullStrategy === "empty-is-null" && cell === "") return null;
  if (nullStrategy === "literal-null" && cell.toUpperCase() === "NULL") return null;
  return cell;
}

function whereClauseForRow(
  row: string[],
  mapped: { source: string; index: number; target: string }[],
  nullStrategy: ImportNullStrategy,
  driver?: string
): string {
  return mapped
    .map((item) => {
      const value = importValue(row[item.index] ?? "", nullStrategy);
      const column = quoteIdent(item.target, driver);
      return value == null ? `${column} IS NULL` : `${column} = ${sqlQuote(value)}`;
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

  const rowValuesSql = rows.map(
    (row) => `(${mapped.map((item) => sqlQuote(importValue(row[item.index] ?? "", effectiveNullStrategy))).join(", ")})`
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
        .map(
          (item) =>
            `${quoteIdent(item.target, driver)} = ${sqlQuote(importValue(row[item.index] ?? "", effectiveNullStrategy))}`
        )
        .join(", ");
      return `UPDATE ${quotedTable} SET ${setSql} WHERE ${whereClauseForRow(row, keyMapped, effectiveNullStrategy, driver)};`;
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
      (row) => `DELETE FROM ${quotedTable} WHERE ${whereClauseForRow(row, keyMapped, effectiveNullStrategy, driver)};`
    );
  } else {
    statements = buildInsertStatements("INSERT INTO");
    if (mode === "copy") statements = [`DELETE FROM ${quotedTable};`, ...statements];
  }

  if (advancedOptions?.ignoreForeignKeyConstraint && driver !== "postgresql") {
    statements = ["SET FOREIGN_KEY_CHECKS = 0;", ...statements, "SET FOREIGN_KEY_CHECKS = 1;"];
  }

  return statements;
}
