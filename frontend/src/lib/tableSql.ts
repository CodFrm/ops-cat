export function sqlQuote(value: unknown): string {
  if (value == null) return "NULL";
  const s = String(value);
  const escaped = s.replace(/'/g, "''");
  return `'${escaped}'`;
}

export function quoteIdent(name: string, driver?: string): string {
  if (driver === "postgresql") return `"${name}"`;
  return `\`${name}\``;
}

export type CellValueFilterOperator = "=" | "!=" | "like" | "not_like" | "<" | ">";

export function buildFilterByCellValueClause(
  col: string,
  value: unknown,
  driver?: string,
  operator: CellValueFilterOperator = "="
): string {
  const quotedCol = quoteIdent(col, driver);
  if (value == null) {
    if (operator === "!=") return `${quotedCol} IS NOT NULL`;
    if (operator === "=") return `${quotedCol} IS NULL`;
    return "";
  }
  if (operator === "like") return `${quotedCol} LIKE ${sqlQuote(`%${String(value)}%`)}`;
  if (operator === "not_like") return `${quotedCol} NOT LIKE ${sqlQuote(`%${String(value)}%`)}`;
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

  const tableName =
    driver === "postgresql" ? `"${table}"` : `${quoteIdent(database, driver)}.${quoteIdent(table, driver)}`;
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
