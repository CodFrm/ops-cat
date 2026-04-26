import { quoteIdent, sqlQuote } from "./tableSql";

export interface TableColumnRule {
  name: string;
  nullable: boolean;
  hasDefault: boolean;
  autoIncrement?: boolean;
}

export interface BuildInsertStatementInput {
  database: string;
  table: string;
  driver?: string;
  values: Record<string, unknown>;
}

function hasOwnValue(values: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(values, key);
}

function isMissingRequiredValue(value: unknown): boolean {
  return value == null || value === "";
}

export function validateInsertRow(rules: TableColumnRule[], values: Record<string, unknown>): string[] {
  return rules
    .filter((rule) => !rule.nullable && !rule.hasDefault && !rule.autoIncrement)
    .filter((rule) => !hasOwnValue(values, rule.name) || isMissingRequiredValue(values[rule.name]))
    .map((rule) => rule.name);
}

export function buildInsertStatement({ database, table, driver, values }: BuildInsertStatementInput): string {
  const tableName =
    driver === "postgresql"
      ? quoteIdent(table, driver)
      : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
  const columns = Object.keys(values);

  if (columns.length === 0) {
    return driver === "postgresql"
      ? `INSERT INTO ${tableName} DEFAULT VALUES;`
      : `INSERT INTO ${tableName} () VALUES ();`;
  }

  const columnSql = columns.map((column) => quoteIdent(column, driver)).join(", ");
  const valueSql = columns.map((column) => sqlQuote(values[column])).join(", ");
  return `INSERT INTO ${tableName} (${columnSql}) VALUES (${valueSql});`;
}
