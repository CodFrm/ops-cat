import { quoteIdent, sqlQuote } from "./tableSql";

export type Delimiter = "," | "\t";
export type ImportNullStrategy = "empty-is-empty-string" | "empty-is-null" | "literal-null";

export interface ParsedDelimitedTable {
  headers: string[];
  rows: string[][];
}

export interface BuildImportInsertSqlArgs {
  tableName: string;
  headers: string[];
  rows: string[][];
  mapping: Record<string, string>;
  nullStrategy: ImportNullStrategy;
  driver?: string;
}

export function detectDelimiter(text: string): Delimiter {
  const firstLine = text.split(/\r?\n/, 1)[0] ?? "";
  const tabs = (firstLine.match(/\t/g) ?? []).length;
  const commas = (firstLine.match(/,/g) ?? []).length;
  return tabs > commas ? "\t" : ",";
}

export function parseDelimitedText(text: string, delimiter: Delimiter = detectDelimiter(text)): ParsedDelimitedTable {
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

  const headers = rows[0] ?? [];
  return { headers, rows: rows.slice(1) };
}

function quoteTableName(tableName: string, driver?: string): string {
  return tableName
    .split(".")
    .filter(Boolean)
    .map((part) => quoteIdent(part, driver))
    .join(".");
}

function importValue(cell: string, nullStrategy: ImportNullStrategy): unknown {
  if (nullStrategy === "empty-is-null" && cell === "") return null;
  if (nullStrategy === "literal-null" && cell.toUpperCase() === "NULL") return null;
  return cell;
}

export function buildImportInsertSql({
  tableName,
  headers,
  rows,
  mapping,
  nullStrategy,
  driver,
}: BuildImportInsertSqlArgs): string[] {
  const mapped = headers
    .map((source, index) => ({ source, index, target: mapping[source] }))
    .filter((item): item is { source: string; index: number; target: string } => !!item.target);

  if (mapped.length === 0) return [];

  const quotedTable = quoteTableName(tableName, driver);
  const columnSql = mapped.map((item) => quoteIdent(item.target, driver)).join(", ");

  return rows.map((row) => {
    const values = mapped.map((item) => sqlQuote(importValue(row[item.index] ?? "", nullStrategy))).join(", ");
    return `INSERT INTO ${quotedTable} (${columnSql}) VALUES (${values});`;
  });
}
