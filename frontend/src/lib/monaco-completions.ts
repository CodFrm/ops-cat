// 集中注册 monaco 的补全 provider。
//
// - SQL：关键字 / 内置函数 / 常用 SELECT-FROM-WHERE 等 snippet（与方言无关，覆盖 MySQL / PostgreSQL 多数场景）
// - JavaScript（用于 MongoDB 查询）：mongo 查询/更新/聚合 operators（$eq/$gt/$in/$regex/$set/$group ...）
// - 调用方可以按 model.uri 注入"动态项"（例如当前选中库下的表名 / 集合名），
//   随 editor 卸载时再 unregister。
//
// 该模块只能被 import 一次（registerCompletions 内部做了幂等保护）。
//
import type * as MonacoNS from "monaco-editor";
import type * as MonacoRuntimeNS from "monaco-editor/esm/vs/editor/editor.api.js";

type MonacoRuntime = typeof MonacoRuntimeNS;

export type CompletionContext = {
  monaco: MonacoRuntime;
  range: MonacoNS.IRange;
  model: MonacoNS.editor.ITextModel;
  position: MonacoNS.Position;
};

export type DynamicCompletionGetter = (ctx: CompletionContext) => MonacoNS.languages.CompletionItem[];

const dynamicMap = new Map<string, DynamicCompletionGetter>();

export function registerDynamicCompletions(uri: string, getter: DynamicCompletionGetter): void {
  dynamicMap.set(uri, getter);
}

export function unregisterDynamicCompletions(uri: string): void {
  dynamicMap.delete(uri);
}

// ---------------- SQL ----------------

const SQL_KEYWORDS = [
  "SELECT",
  "FROM",
  "WHERE",
  "AND",
  "OR",
  "NOT",
  "NULL",
  "IS",
  "IN",
  "LIKE",
  "BETWEEN",
  "AS",
  "JOIN",
  "INNER JOIN",
  "LEFT JOIN",
  "RIGHT JOIN",
  "OUTER JOIN",
  "CROSS JOIN",
  "ON",
  "GROUP BY",
  "ORDER BY",
  "HAVING",
  "LIMIT",
  "OFFSET",
  "INSERT INTO",
  "VALUES",
  "UPDATE",
  "SET",
  "DELETE FROM",
  "CREATE TABLE",
  "CREATE DATABASE",
  "CREATE INDEX",
  "CREATE VIEW",
  "ALTER TABLE",
  "DROP TABLE",
  "TRUNCATE",
  "DISTINCT",
  "UNION",
  "UNION ALL",
  "EXISTS",
  "ALL",
  "ANY",
  "SOME",
  "IF",
  "CASE",
  "WHEN",
  "THEN",
  "ELSE",
  "END",
  "ASC",
  "DESC",
  "WITH",
  "RETURNING",
  "PRIMARY KEY",
  "FOREIGN KEY",
  "REFERENCES",
  "DEFAULT",
  "UNIQUE",
  "INDEX",
];

const SQL_FUNCTIONS = [
  "COUNT",
  "SUM",
  "AVG",
  "MIN",
  "MAX",
  "NOW",
  "CURRENT_DATE",
  "CURRENT_TIME",
  "CURRENT_TIMESTAMP",
  "CONCAT",
  "SUBSTRING",
  "LENGTH",
  "UPPER",
  "LOWER",
  "TRIM",
  "LTRIM",
  "RTRIM",
  "REPLACE",
  "COALESCE",
  "IFNULL",
  "NULLIF",
  "CAST",
  "CONVERT",
  "DATE",
  "YEAR",
  "MONTH",
  "DAY",
  "HOUR",
  "MINUTE",
  "SECOND",
  "DATE_FORMAT",
  "ROUND",
  "FLOOR",
  "CEILING",
  "ABS",
  "MOD",
  "POWER",
  "SQRT",
  "JSON_EXTRACT",
  "JSON_OBJECT",
  "JSON_ARRAY",
];

// ---------------- MongoDB operators ----------------

interface MongoOp {
  label: string;
  category: "Query" | "Update" | "Aggregation" | "Logical" | "Element";
  doc?: string;
}

const MONGO_OPERATORS: MongoOp[] = [
  // 查询比较
  { label: "$eq", category: "Query", doc: "字段等于" },
  { label: "$ne", category: "Query", doc: "字段不等于" },
  { label: "$gt", category: "Query", doc: "大于" },
  { label: "$gte", category: "Query", doc: "大于等于" },
  { label: "$lt", category: "Query", doc: "小于" },
  { label: "$lte", category: "Query", doc: "小于等于" },
  { label: "$in", category: "Query", doc: "字段值在数组内" },
  { label: "$nin", category: "Query", doc: "字段值不在数组内" },
  // 逻辑
  { label: "$and", category: "Logical", doc: "AND" },
  { label: "$or", category: "Logical", doc: "OR" },
  { label: "$not", category: "Logical", doc: "NOT" },
  { label: "$nor", category: "Logical", doc: "NOR" },
  // 元素
  { label: "$exists", category: "Element", doc: "字段存在" },
  { label: "$type", category: "Element", doc: "字段类型" },
  // 字符串/正则
  { label: "$regex", category: "Query", doc: "正则匹配" },
  { label: "$options", category: "Query", doc: "正则选项 (i / m / x / s)" },
  // 数组
  { label: "$all", category: "Query", doc: "数组包含所有" },
  { label: "$elemMatch", category: "Query", doc: "数组元素匹配" },
  { label: "$size", category: "Query", doc: "数组大小" },
  // 更新
  { label: "$set", category: "Update", doc: "设置字段" },
  { label: "$unset", category: "Update", doc: "删除字段" },
  { label: "$inc", category: "Update", doc: "递增" },
  { label: "$push", category: "Update", doc: "数组追加" },
  { label: "$pull", category: "Update", doc: "数组移除" },
  { label: "$addToSet", category: "Update", doc: "数组去重追加" },
  // 聚合 stage
  { label: "$match", category: "Aggregation", doc: "聚合 - 匹配" },
  { label: "$project", category: "Aggregation", doc: "聚合 - 投影" },
  { label: "$group", category: "Aggregation", doc: "聚合 - 分组" },
  { label: "$sort", category: "Aggregation", doc: "聚合 - 排序" },
  { label: "$limit", category: "Aggregation", doc: "聚合 - 限制" },
  { label: "$skip", category: "Aggregation", doc: "聚合 - 跳过" },
  { label: "$unwind", category: "Aggregation", doc: "聚合 - 展开数组" },
  { label: "$lookup", category: "Aggregation", doc: "聚合 - 关联" },
  // 聚合 accumulator
  { label: "$sum", category: "Aggregation", doc: "求和" },
  { label: "$avg", category: "Aggregation", doc: "平均" },
];

// ---------------- 注册 ----------------

let registered = false;

export function registerCompletions(monaco: MonacoRuntime): void {
  if (registered) return;
  registered = true;

  monaco.languages.registerCompletionItemProvider("sql", {
    triggerCharacters: ["."],
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position);
      const range: MonacoNS.IRange = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const suggestions: MonacoNS.languages.CompletionItem[] = [];

      for (const kw of SQL_KEYWORDS) {
        suggestions.push({
          label: kw,
          kind: monaco.languages.CompletionItemKind.Keyword,
          insertText: kw,
          range,
          sortText: "1_" + kw,
        });
      }
      for (const fn of SQL_FUNCTIONS) {
        suggestions.push({
          label: fn,
          kind: monaco.languages.CompletionItemKind.Function,
          insertText: fn,
          range,
          sortText: "2_" + fn,
        });
      }

      const dyn = dynamicMap.get(model.uri.toString());
      if (dyn) suggestions.push(...dyn({ monaco, range, model, position }));

      return { suggestions };
    },
  });

  // Mongo 查询通常用 javascript 模式（用户当前已选 js 高亮）。这里追加 $operators。
  // 当前只加载基础 javascript 语言包，不启用体积较大的 TypeScript worker。
  monaco.languages.registerCompletionItemProvider("javascript", {
    triggerCharacters: ["$", "."],
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position);
      const range: MonacoNS.IRange = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const suggestions: MonacoNS.languages.CompletionItem[] = MONGO_OPERATORS.map((op) => ({
        label: op.label,
        kind: monaco.languages.CompletionItemKind.Operator,
        insertText: op.label,
        documentation: op.doc ? `[${op.category}] ${op.doc}` : `[${op.category}]`,
        range,
        sortText: "0_" + op.label,
      }));

      const dyn = dynamicMap.get(model.uri.toString());
      if (dyn) suggestions.push(...dyn({ monaco, range, model, position }));

      return { suggestions };
    },
  });

  // 如果未来重新启用 TypeScript language service，mongo 查询体作为 JS 解析时，
  // 顶层 {...} 会被当作 block + label，容易误报。这里保留兼容逻辑。
  const tsLang = (
    monaco.languages as unknown as {
      typescript?: { javascriptDefaults?: { setDiagnosticsOptions?: (o: unknown) => void } };
    }
  ).typescript;
  tsLang?.javascriptDefaults?.setDiagnosticsOptions?.({
    noSemanticValidation: true,
    noSyntaxValidation: false,
  });
}
