import { quoteIdent, sqlQuote } from "./tableSql";

export type TableFilterJoin = "and" | "or";
export type TableFilterOperator =
  | "="
  | "!="
  | "like"
  | "not_like"
  | ">"
  | ">="
  | "<"
  | "<="
  | "is_empty"
  | "is_not_empty";
export type TableSortDir = "asc" | "desc";

export interface TableFilterCondition {
  kind: "condition";
  id: string;
  column: string;
  operator: TableFilterOperator;
  value?: unknown;
  enabled: boolean;
  join: TableFilterJoin;
}

export interface TableFilterGroup {
  kind: "group";
  id: string;
  items: TableFilterItem[];
  join: TableFilterJoin;
  enabled?: boolean;
  negated?: boolean;
}

export type TableFilterItem = TableFilterCondition | TableFilterGroup;

export interface TableSortItem {
  id: string;
  column: string;
  dir: TableSortDir;
}

interface CreateFilterConditionOptions {
  value?: unknown;
  enabled?: boolean;
  join?: TableFilterJoin;
  operator?: TableFilterOperator;
}

let filterIdSeq = 0;

function nextFilterId(prefix = "filter"): string {
  filterIdSeq += 1;
  return `${prefix}-${filterIdSeq}`;
}

export function createFilterCondition(
  id: string,
  column: string,
  options: CreateFilterConditionOptions = {}
): TableFilterCondition {
  return {
    kind: "condition",
    id,
    column,
    operator: options.operator ?? "=",
    value: options.value,
    enabled: options.enabled ?? true,
    join: options.join ?? "and",
  };
}

function usedColumns(items: TableFilterItem[]): Set<string> {
  const out = new Set<string>();
  for (const item of items) {
    if (item.kind === "condition") out.add(item.column);
    else for (const col of usedColumns(item.items)) out.add(col);
  }
  return out;
}

export function pickNextFilterColumn(items: TableFilterItem[], columns: string[]): string | null {
  if (columns.length === 0) return null;
  const used = usedColumns(items);
  return columns.find((column) => !used.has(column)) ?? columns[0];
}

export function addFilterCondition(
  items: TableFilterItem[],
  columns: string[],
  _driver?: string,
  id = nextFilterId()
): TableFilterItem[] {
  const column = pickNextFilterColumn(items, columns);
  if (!column) return items;
  return [...items, createFilterCondition(id, column)];
}

export function addFilterGroup(
  items: TableFilterItem[],
  _columns: string[],
  _driver?: string,
  id = nextFilterId("group")
): TableFilterItem[] {
  return [...items, { kind: "group", id, items: [], join: "and", enabled: true }];
}

export function toggleFilterJoin(items: TableFilterItem[], id: string): TableFilterItem[] {
  return items.map((item) => {
    if (item.id === id) {
      return { ...item, join: item.join === "and" ? "or" : "and" };
    }
    if (item.kind === "group") return { ...item, items: toggleFilterJoin(item.items, id) };
    return item;
  });
}

function hasValue(value: unknown): boolean {
  return value !== undefined;
}

function conditionSql(condition: TableFilterCondition, driver?: string): string {
  if (!condition.enabled) return "";
  const column = quoteIdent(condition.column, driver);
  const operator = condition.operator ?? "=";

  if (operator === "is_empty") return `(${column} IS NULL OR ${column} = '')`;
  if (operator === "is_not_empty") return `(${column} IS NOT NULL AND ${column} <> '')`;

  if (!hasValue(condition.value)) return "";
  if (condition.value == null) {
    if (operator === "!=") return `${column} IS NOT NULL`;
    if (operator === "=") return `${column} IS NULL`;
    return "";
  }

  if (operator === "like") return `${column} LIKE ${sqlQuote(`%${String(condition.value)}%`)}`;
  if (operator === "not_like") return `${column} NOT LIKE ${sqlQuote(`%${String(condition.value)}%`)}`;

  const sqlOperator = operator === "!=" ? "<>" : operator;
  return `${column} ${sqlOperator} ${sqlQuote(condition.value)}`;
}

function buildItemsWhere(items: TableFilterItem[], driver?: string): string {
  const parts: { sql: string; join: TableFilterJoin }[] = [];
  for (const item of items) {
    const sql =
      item.kind === "condition"
        ? conditionSql(item, driver)
        : item.enabled === false
          ? ""
          : buildItemsWhere(item.items, driver);
    if (!sql) continue;
    const groupedSql = item.kind === "group" ? `${item.negated ? "NOT " : ""}(${sql})` : sql;
    parts.push({ sql: groupedSql, join: item.join });
  }

  return parts.reduce((acc, part, index) => {
    if (index === 0) return part.sql;
    return `${acc} ${parts[index - 1].join.toUpperCase()} ${part.sql}`;
  }, "");
}

export function buildFilterWhereClause(items: TableFilterItem[], driver?: string): string {
  return buildItemsWhere(items, driver);
}

export function updateFilterItem(
  items: TableFilterItem[],
  id: string,
  patch: Partial<TableFilterCondition>
): TableFilterItem[] {
  return items.map((item) => {
    if (item.kind === "condition" && item.id === id) return { ...item, ...patch };
    if (item.kind === "group") return { ...item, items: updateFilterItem(item.items, id, patch) };
    return item;
  });
}

export function updateFilterGroup(
  items: TableFilterItem[],
  id: string,
  patch: Partial<TableFilterGroup>
): TableFilterItem[] {
  return items.map((item) => {
    if (item.kind === "group" && item.id === id) return { ...item, ...patch };
    if (item.kind === "group") return { ...item, items: updateFilterGroup(item.items, id, patch) };
    return item;
  });
}

const NEGATED_OPERATOR: Record<TableFilterOperator, TableFilterOperator> = {
  "=": "!=",
  "!=": "=",
  like: "not_like",
  not_like: "like",
  ">": "<=",
  ">=": "<",
  "<": ">=",
  "<=": ">",
  is_empty: "is_not_empty",
  is_not_empty: "is_empty",
};

export function toggleFilterNegator(items: TableFilterItem[], id: string): TableFilterItem[] {
  return items.map((item) => {
    if (item.id === id) {
      if (item.kind === "condition") return { ...item, operator: NEGATED_OPERATOR[item.operator ?? "="] };
      return { ...item, negated: !item.negated };
    }
    if (item.kind === "group") return { ...item, items: toggleFilterNegator(item.items, id) };
    return item;
  });
}

export function toggleFilterItemEnabled(items: TableFilterItem[], id: string): TableFilterItem[] {
  return items.map((item) => {
    if (item.id === id) return { ...item, enabled: item.kind === "group" ? item.enabled === false : !item.enabled };
    if (item.kind === "group") return { ...item, items: toggleFilterItemEnabled(item.items, id) };
    return item;
  });
}

export function updateFilterGroupItems(
  items: TableFilterItem[],
  id: string,
  updater: (children: TableFilterItem[]) => TableFilterItem[]
): TableFilterItem[] {
  return items.map((item) => {
    if (item.kind === "group" && item.id === id) return { ...item, items: updater(item.items) };
    if (item.kind === "group") return { ...item, items: updateFilterGroupItems(item.items, id, updater) };
    return item;
  });
}

export function removeFilterItem(items: TableFilterItem[], id: string): TableFilterItem[] {
  return items
    .filter((item) => item.id !== id)
    .map((item) => (item.kind === "group" ? { ...item, items: removeFilterItem(item.items, id) } : item));
}

export function removeFilterItemsByColumn(items: TableFilterItem[], column: string): TableFilterItem[] {
  return items
    .filter((item) => item.kind !== "condition" || item.column !== column)
    .map((item) => (item.kind === "group" ? { ...item, items: removeFilterItemsByColumn(item.items, column) } : item));
}

export function unwrapFilterGroup(items: TableFilterItem[], id: string): TableFilterItem[] {
  const next: TableFilterItem[] = [];
  for (const item of items) {
    if (item.kind === "group" && item.id === id) {
      next.push(...item.items);
      continue;
    }
    next.push(item.kind === "group" ? { ...item, items: unwrapFilterGroup(item.items, id) } : item);
  }
  return next;
}

export function setAllFilterItemsEnabled(items: TableFilterItem[], enabled: boolean): TableFilterItem[] {
  return items.map((item) =>
    item.kind === "group"
      ? { ...item, enabled, items: setAllFilterItemsEnabled(item.items, enabled) }
      : { ...item, enabled }
  );
}

export function findFilterItem(items: TableFilterItem[], id: string): TableFilterItem | null {
  for (const item of items) {
    if (item.id === id) return item;
    if (item.kind === "group") {
      const child = findFilterItem(item.items, id);
      if (child) return child;
    }
  }
  return null;
}

export function cloneFilterItem(item: TableFilterItem): TableFilterItem {
  if (item.kind === "condition") return { ...item, id: nextFilterId() };
  return { ...item, id: nextFilterId("group"), items: item.items.map(cloneFilterItem) };
}

export function insertFilterItemAfter(
  items: TableFilterItem[],
  id: string,
  itemToInsert: TableFilterItem
): TableFilterItem[] {
  const index = items.findIndex((item) => item.id === id);
  if (index !== -1) {
    const next = [...items];
    next.splice(index + 1, 0, itemToInsert);
    return next;
  }

  return items.map((item) =>
    item.kind === "group" ? { ...item, items: insertFilterItemAfter(item.items, id, itemToInsert) } : item
  );
}

export function moveFilterItem(items: TableFilterItem[], id: string, direction: "up" | "down"): TableFilterItem[] {
  const index = items.findIndex((item) => item.id === id);
  if (index !== -1) {
    const target = direction === "up" ? index - 1 : index + 1;
    if (target < 0 || target >= items.length) return items;
    const next = [...items];
    const [item] = next.splice(index, 1);
    next.splice(target, 0, item);
    return next;
  }

  return items.map((item) =>
    item.kind === "group" ? { ...item, items: moveFilterItem(item.items, id, direction) } : item
  );
}

export function createSortCriterion(id: string, column: string, dir: TableSortDir = "asc"): TableSortItem {
  return { id, column, dir };
}

function usedSortColumns(items: TableSortItem[]): Set<string> {
  return new Set(items.map((item) => item.column));
}

function pickNextSortColumn(items: TableSortItem[], columns: string[]): string | null {
  if (columns.length === 0) return null;
  const used = usedSortColumns(items);
  return columns.find((column) => !used.has(column)) ?? columns[0];
}

export function addSortCriterion(
  items: TableSortItem[],
  columns: string[],
  _driver?: string,
  id = nextFilterId("sort")
): TableSortItem[] {
  const column = pickNextSortColumn(items, columns);
  if (!column) return items;
  return [...items, createSortCriterion(id, column)];
}

export function toggleSortDirection(items: TableSortItem[], id: string): TableSortItem[] {
  return items.map((item) => (item.id === id ? { ...item, dir: item.dir === "asc" ? "desc" : "asc" } : item));
}

export function updateSortCriterion(
  items: TableSortItem[],
  id: string,
  patch: Partial<TableSortItem>
): TableSortItem[] {
  return items.map((item) => (item.id === id ? { ...item, ...patch } : item));
}

export function buildSortOrderByClause(items: TableSortItem[], driver?: string): string {
  return items.map((item) => `${quoteIdent(item.column, driver)} ${item.dir.toUpperCase()}`).join(", ");
}
