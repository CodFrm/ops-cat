export type TableFilterOperator =
  | "="
  | "!="
  | "<"
  | "<="
  | ">"
  | ">="
  | "contains"
  | "not_contains"
  | "begins_with"
  | "not_begins_with"
  | "ends_with"
  | "not_ends_with"
  | "is_null"
  | "is_not_null"
  | "is_empty"
  | "is_not_empty"
  | "between"
  | "not_between"
  | "in_list"
  | "not_in_list"
  | "like"
  | "not_like";

export const TABLE_FILTER_OPERATOR_OPTIONS: TableFilterOperator[] = [
  "=",
  "!=",
  "<",
  "<=",
  ">",
  ">=",
  "contains",
  "not_contains",
  "begins_with",
  "not_begins_with",
  "ends_with",
  "not_ends_with",
  "is_null",
  "is_not_null",
  "is_empty",
  "is_not_empty",
  "between",
  "not_between",
  "in_list",
  "not_in_list",
];

export const TABLE_FILTER_OPERATOR_LABEL_KEYS: Record<TableFilterOperator, string> = {
  "=": "query.filterOperatorIs",
  "!=": "query.filterOperatorIsNot",
  "<": "query.filterOperatorLessThan",
  "<=": "query.filterOperatorLessThanOrEqual",
  ">": "query.filterOperatorGreaterThan",
  ">=": "query.filterOperatorGreaterThanOrEqual",
  contains: "query.filterOperatorContains",
  not_contains: "query.filterOperatorDoesNotContain",
  begins_with: "query.filterOperatorBeginsWith",
  not_begins_with: "query.filterOperatorDoesNotBeginWith",
  ends_with: "query.filterOperatorEndsWith",
  not_ends_with: "query.filterOperatorDoesNotEndWith",
  is_null: "query.filterOperatorIsNull",
  is_not_null: "query.filterOperatorIsNotNull",
  is_empty: "query.filterOperatorIsEmpty",
  is_not_empty: "query.filterOperatorIsNotEmpty",
  between: "query.filterOperatorIsBetween",
  not_between: "query.filterOperatorIsNotBetween",
  in_list: "query.filterOperatorIsInList",
  not_in_list: "query.filterOperatorIsNotInList",
  like: "query.filterOperatorContains",
  not_like: "query.filterOperatorDoesNotContain",
};

export function filterOperatorNeedsValue(operator: TableFilterOperator): boolean {
  return !["is_null", "is_not_null", "is_empty", "is_not_empty"].includes(operator);
}

export function filterOperatorNeedsRange(operator: TableFilterOperator): boolean {
  return operator === "between" || operator === "not_between";
}
