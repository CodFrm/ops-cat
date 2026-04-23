// mongosh 风格语句解析器。
//
// 把 `db.users.find({age: {$gt: 18}}).sort({name: 1}).limit(10)` 这种 JS 表达式，
// 解析成后端 ExecuteMongo 需要的 (operation, database, collection, queryJSON) 四元组。
//
// 只用 acorn 做 AST，不跑 JS。ObjectId("...") / ISODate("...") / /regex/ 等 mongosh
// 字面量被转换成标准 EJSON（$oid / $date / $regex 等），后端的 bson.UnmarshalExtJSON
// 原生支持。
//
// 不支持：use 语句、变量、for/if 等语句、未知函数、calculated keys（[x]: 1）。

import { parseExpressionAt, type Node as AcornNode } from "acorn";

export class MongoshParseError extends Error {
  constructor(
    message: string,
    public readonly pos?: number
  ) {
    super(message);
    this.name = "MongoshParseError";
  }
}

export type MongoshOperation =
  | "find"
  | "findOne"
  | "insertOne"
  | "insertMany"
  | "updateOne"
  | "updateMany"
  | "deleteOne"
  | "deleteMany"
  | "aggregate"
  | "countDocuments";

export interface ParsedMongosh {
  database: string;
  collection: string;
  operation: MongoshOperation;
  query: Record<string, unknown>;
}

export type ParseResult = { ok: true; value: ParsedMongosh } | { ok: false; error: MongoshParseError };

export function parseMongosh(src: string, currentDb: string): ParseResult {
  try {
    return { ok: true, value: parseInner(src, currentDb) };
  } catch (e) {
    if (e instanceof MongoshParseError) return { ok: false, error: e };
    const msg = e instanceof Error ? e.message : String(e);
    return { ok: false, error: new MongoshParseError(msg) };
  }
}

// --- Internal ---

type JSONValue = string | number | boolean | null | JSONValue[] | { [k: string]: JSONValue };

function parseInner(src: string, currentDb: string): ParsedMongosh {
  const trimmed = src.trim().replace(/;\s*$/, "");
  if (!trimmed) throw new MongoshParseError("查询语句为空");

  let root: AcornNode;
  try {
    root = parseExpressionAt(trimmed, 0, { ecmaVersion: 2022 });
  } catch (e) {
    throw new MongoshParseError(`语法错误: ${e instanceof Error ? e.message : String(e)}`);
  }
  // parseExpressionAt may stop early; ensure it consumed the whole string (ignoring trailing whitespace).
  const rootWithEnd = root as AcornNode & { end: number };
  const tail = trimmed.slice(rootWithEnd.end).trim();
  if (tail) throw new MongoshParseError(`多余的内容: ${tail.slice(0, 30)}`);

  if (root.type !== "CallExpression") {
    throw new MongoshParseError("需要函数调用，例如 db.collection.find(...)");
  }

  // 剥 modifier 链：db.C.op(...).sort(...).limit(...)
  type ModifierCall = { name: string; args: JSONValue[] };
  const modifiers: ModifierCall[] = [];
  let anchor = root as AstCallExpression;

  while (
    anchor.callee.type === "MemberExpression" &&
    (anchor.callee as AstMemberExpression).object.type === "CallExpression"
  ) {
    const mem = anchor.callee as AstMemberExpression;
    if (mem.computed) throw new MongoshParseError("不支持计算属性访问");
    const name = propertyName(mem.property);
    modifiers.unshift({ name, args: anchor.arguments.map(convertValue) });
    anchor = mem.object as AstCallExpression;
  }

  if (anchor.callee.type !== "MemberExpression") {
    throw new MongoshParseError("需要 db.collection.operation(...) 形式");
  }
  const opMember = anchor.callee as AstMemberExpression;
  if (opMember.computed) throw new MongoshParseError("不支持计算属性访问");
  const operationName = propertyName(opMember.property);

  const { database, collection } = resolveCollection(opMember.object, currentDb);
  const args = anchor.arguments.map(convertValue);
  const { operation, query } = buildOpQuery(operationName, args, modifiers);

  return { database, collection, operation, query };
}

// --- AST 节点最小类型（避免依赖 @types/estree） ---

type AstCallExpression = AcornNode & {
  type: "CallExpression";
  callee: AcornNode;
  arguments: AcornNode[];
};
type AstMemberExpression = AcornNode & {
  type: "MemberExpression";
  object: AcornNode;
  property: AcornNode;
  computed: boolean;
};
type AstIdentifier = AcornNode & { type: "Identifier"; name: string };
type AstLiteral = AcornNode & {
  type: "Literal";
  value: string | number | boolean | null;
  raw?: string;
  regex?: { pattern: string; flags: string };
};
type AstProperty = AcornNode & {
  type: "Property";
  key: AcornNode;
  value: AcornNode;
  computed: boolean;
  shorthand: boolean;
};
type AstObjectExpression = AcornNode & { type: "ObjectExpression"; properties: AcornNode[] };
type AstArrayExpression = AcornNode & { type: "ArrayExpression"; elements: (AcornNode | null)[] };
type AstUnaryExpression = AcornNode & {
  type: "UnaryExpression";
  operator: string;
  argument: AcornNode;
  prefix: boolean;
};
type AstNewExpression = AcornNode & { type: "NewExpression"; callee: AcornNode; arguments: AcornNode[] };
type AstTemplateLiteral = AcornNode & {
  type: "TemplateLiteral";
  quasis: { value: { cooked: string; raw: string } }[];
  expressions: AcornNode[];
};

function propertyName(node: AcornNode): string {
  if (node.type === "Identifier") return (node as AstIdentifier).name;
  if (node.type === "Literal" && typeof (node as AstLiteral).value === "string") {
    return String((node as AstLiteral).value);
  }
  throw new MongoshParseError("不支持的属性名");
}

function resolveCollection(obj: AcornNode, currentDb: string): { database: string; collection: string } {
  if (obj.type !== "MemberExpression") {
    throw new MongoshParseError("需要以 db.&lt;collection&gt; 开头");
  }
  const mem = obj as AstMemberExpression;
  if (mem.computed) throw new MongoshParseError("不支持计算属性访问");
  const collection = propertyName(mem.property);
  const dbRef = mem.object;

  if (dbRef.type === "Identifier" && (dbRef as AstIdentifier).name === "db") {
    if (!currentDb) throw new MongoshParseError("请先在侧边栏选择数据库");
    return { database: currentDb, collection };
  }
  // db.getSiblingDB("name").coll
  if (dbRef.type === "CallExpression") {
    const call = dbRef as AstCallExpression;
    if (
      call.callee.type === "MemberExpression" &&
      (call.callee as AstMemberExpression).object.type === "Identifier" &&
      ((call.callee as AstMemberExpression).object as AstIdentifier).name === "db" &&
      propertyName((call.callee as AstMemberExpression).property) === "getSiblingDB"
    ) {
      const args = call.arguments.map(convertValue);
      if (typeof args[0] !== "string" || args[0].length === 0) {
        throw new MongoshParseError("getSiblingDB 参数必须是非空字符串");
      }
      return { database: args[0], collection };
    }
  }
  throw new MongoshParseError("需要以 db.&lt;collection&gt; 或 db.getSiblingDB(...).&lt;collection&gt; 开头");
}

function convertValue(node: AcornNode): JSONValue {
  switch (node.type) {
    case "Literal": {
      const lit = node as AstLiteral;
      if (lit.regex) {
        return { $regex: lit.regex.pattern, $options: lit.regex.flags };
      }
      if (lit.value === undefined) return null;
      return lit.value as JSONValue;
    }
    case "Identifier": {
      const id = node as AstIdentifier;
      if (id.name === "undefined") return null;
      if (id.name === "NaN" || id.name === "Infinity") {
        throw new MongoshParseError(`不支持的字面量: ${id.name}`);
      }
      throw new MongoshParseError(`不支持的标识符: ${id.name}`);
    }
    case "ObjectExpression": {
      const obj: Record<string, JSONValue> = {};
      for (const p of (node as AstObjectExpression).properties) {
        if (p.type !== "Property") throw new MongoshParseError("不支持的对象属性类型");
        const prop = p as AstProperty;
        if (prop.computed) throw new MongoshParseError("不支持计算键");
        if (prop.shorthand) throw new MongoshParseError("不支持简写属性，请写成 key: value");
        const key = propertyName(prop.key);
        obj[key] = convertValue(prop.value);
      }
      return obj;
    }
    case "ArrayExpression":
      return (node as AstArrayExpression).elements.map((e) => (e ? convertValue(e) : null));
    case "UnaryExpression": {
      const u = node as AstUnaryExpression;
      const v = convertValue(u.argument);
      if (u.operator === "-" && typeof v === "number") return -v;
      if (u.operator === "+" && typeof v === "number") return v;
      if (u.operator === "!" && typeof v === "boolean") return !v;
      throw new MongoshParseError(`不支持的一元运算符: ${u.operator}`);
    }
    case "CallExpression":
      return convertEjsonCall(node as AstCallExpression);
    case "NewExpression":
      return convertEjsonCall({
        type: "CallExpression",
        callee: (node as AstNewExpression).callee,
        arguments: (node as AstNewExpression).arguments,
      } as AstCallExpression);
    case "TemplateLiteral": {
      const tpl = node as AstTemplateLiteral;
      if (tpl.expressions.length > 0) {
        throw new MongoshParseError("不支持模板字符串插值");
      }
      return tpl.quasis.map((q) => q.value.cooked).join("");
    }
    default:
      throw new MongoshParseError(`不支持的表达式: ${node.type}`);
  }
}

function convertEjsonCall(call: AstCallExpression): JSONValue {
  if (call.callee.type !== "Identifier") {
    throw new MongoshParseError("不支持的调用表达式");
  }
  const name = (call.callee as AstIdentifier).name;
  const args = call.arguments.map(convertValue);

  switch (name) {
    case "ObjectId": {
      if (args.length === 0) throw new MongoshParseError("ObjectId 需要字符串参数");
      if (typeof args[0] !== "string") throw new MongoshParseError("ObjectId 参数必须是字符串");
      return { $oid: args[0] };
    }
    case "ISODate":
    case "Date": {
      if (args.length === 0) throw new MongoshParseError(`${name} 需要一个字符串参数`);
      if (typeof args[0] === "string") return { $date: args[0] };
      if (typeof args[0] === "number") return { $date: new Date(args[0]).toISOString() };
      throw new MongoshParseError(`${name} 参数必须是字符串或数字`);
    }
    case "NumberLong":
    case "NumberInt": {
      const v = args[0];
      const s = typeof v === "string" ? v : typeof v === "number" ? String(v) : null;
      if (s === null) throw new MongoshParseError(`${name} 参数必须是字符串或数字`);
      return name === "NumberLong" ? { $numberLong: s } : { $numberInt: s };
    }
    case "NumberDecimal": {
      const v = args[0];
      const s = typeof v === "string" ? v : typeof v === "number" ? String(v) : null;
      if (s === null) throw new MongoshParseError("NumberDecimal 参数必须是字符串");
      return { $numberDecimal: s };
    }
    case "Timestamp": {
      if (typeof args[0] !== "number" || typeof args[1] !== "number") {
        throw new MongoshParseError("Timestamp(t, i) 需要两个数字参数");
      }
      return { $timestamp: { t: args[0], i: args[1] } };
    }
    default:
      throw new MongoshParseError(`不支持的函数: ${name}`);
  }
}

// --- Operation → query shape ---

const FIND_MODIFIERS = new Set(["sort", "limit", "skip", "project"]);

function buildOpQuery(
  name: string,
  args: JSONValue[],
  modifiers: { name: string; args: JSONValue[] }[]
): { operation: MongoshOperation; query: Record<string, unknown> } {
  const q: Record<string, unknown> = {};
  let operation: MongoshOperation;

  switch (name) {
    case "find":
      operation = "find";
      q.filter = (args[0] as object) ?? {};
      if (args[1] !== undefined) q.projection = args[1];
      break;
    case "findOne":
      operation = "findOne";
      q.filter = (args[0] as object) ?? {};
      if (args[1] !== undefined) q.projection = args[1];
      break;
    case "insertOne":
      operation = "insertOne";
      if (args.length < 1) throw new MongoshParseError("insertOne 需要一个文档参数");
      q.document = args[0];
      break;
    case "insertMany":
      operation = "insertMany";
      if (!Array.isArray(args[0])) throw new MongoshParseError("insertMany 需要文档数组 documents");
      q.documents = args[0];
      break;
    case "updateOne":
    case "updateMany":
      operation = name;
      q.filter = (args[0] as object) ?? {};
      if (args[1] === undefined) throw new MongoshParseError(`${name} 需要 update 参数`);
      q.update = args[1];
      break;
    case "deleteOne":
    case "deleteMany":
      operation = name;
      q.filter = (args[0] as object) ?? {};
      break;
    case "aggregate":
      operation = "aggregate";
      if (!Array.isArray(args[0])) throw new MongoshParseError("aggregate 需要 pipeline 数组");
      q.pipeline = args[0];
      break;
    case "countDocuments":
    case "count":
      operation = "countDocuments";
      q.filter = (args[0] as object) ?? {};
      break;
    default:
      throw new MongoshParseError(`不支持的操作: ${name}`);
  }

  for (const m of modifiers) {
    if (operation !== "find") {
      throw new MongoshParseError(`${operation} 不支持 .${m.name}() 修饰符`);
    }
    if (!FIND_MODIFIERS.has(m.name)) {
      throw new MongoshParseError(`find 不支持 .${m.name}() 修饰符`);
    }
    switch (m.name) {
      case "sort":
        q.sort = m.args[0];
        break;
      case "limit":
        if (typeof m.args[0] !== "number") throw new MongoshParseError(".limit() 需要数字参数");
        q.limit = m.args[0];
        break;
      case "skip":
        if (typeof m.args[0] !== "number") throw new MongoshParseError(".skip() 需要数字参数");
        q.skip = m.args[0];
        break;
      case "project":
        q.projection = m.args[0];
        break;
    }
  }

  return { operation, query: q };
}
