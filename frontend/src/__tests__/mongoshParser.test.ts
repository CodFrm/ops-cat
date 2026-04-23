import { describe, it, expect } from "vitest";
import { parseMongosh, MongoshParseError } from "@/lib/mongosh-parser";

function ok(src: string, currentDb = "test") {
  const r = parseMongosh(src, currentDb);
  if (!r.ok) throw new Error(`expected ok, got error: ${r.error.message}`);
  return r.value;
}

function err(src: string, currentDb = "test") {
  const r = parseMongosh(src, currentDb);
  if (r.ok) throw new Error(`expected error, got ok: ${JSON.stringify(r.value)}`);
  return r.error;
}

describe("parseMongosh - basic operations", () => {
  it("parses find with filter", () => {
    const r = ok("db.users.find({age: {$gt: 18}})");
    expect(r.database).toBe("test");
    expect(r.collection).toBe("users");
    expect(r.operation).toBe("find");
    expect(r.query).toEqual({ filter: { age: { $gt: 18 } } });
  });

  it("parses find with no args", () => {
    const r = ok("db.users.find()");
    expect(r.operation).toBe("find");
    expect(r.query).toEqual({ filter: {} });
  });

  it("parses find with projection as second arg", () => {
    const r = ok("db.users.find({}, {name: 1, age: 1})");
    expect(r.query).toEqual({ filter: {}, projection: { name: 1, age: 1 } });
  });

  it("parses find with chained sort/limit/skip", () => {
    const r = ok("db.users.find({}).sort({name: 1}).limit(10).skip(20)");
    expect(r.query).toEqual({ filter: {}, sort: { name: 1 }, limit: 10, skip: 20 });
  });

  it("parses find with .project() modifier", () => {
    const r = ok("db.users.find({}).project({name: 1})");
    expect(r.query).toEqual({ filter: {}, projection: { name: 1 } });
  });

  it("parses findOne", () => {
    const r = ok(`db.users.findOne({_id: ObjectId("507f1f77bcf86cd799439011")})`);
    expect(r.operation).toBe("findOne");
    expect(r.query).toEqual({
      filter: { _id: { $oid: "507f1f77bcf86cd799439011" } },
    });
  });

  it("parses insertOne", () => {
    const r = ok(`db.users.insertOne({name: "alice", age: 30})`);
    expect(r.operation).toBe("insertOne");
    expect(r.query).toEqual({ document: { name: "alice", age: 30 } });
  });

  it("parses insertMany", () => {
    const r = ok(`db.users.insertMany([{a: 1}, {b: 2}])`);
    expect(r.operation).toBe("insertMany");
    expect(r.query).toEqual({ documents: [{ a: 1 }, { b: 2 }] });
  });

  it("parses updateOne", () => {
    const r = ok(`db.users.updateOne({a: 1}, {$set: {b: 2}})`);
    expect(r.operation).toBe("updateOne");
    expect(r.query).toEqual({ filter: { a: 1 }, update: { $set: { b: 2 } } });
  });

  it("parses updateMany", () => {
    const r = ok(`db.users.updateMany({a: 1}, {$inc: {n: 1}})`);
    expect(r.operation).toBe("updateMany");
    expect(r.query).toEqual({ filter: { a: 1 }, update: { $inc: { n: 1 } } });
  });

  it("parses deleteOne / deleteMany", () => {
    expect(ok(`db.users.deleteOne({a: 1})`).query).toEqual({ filter: { a: 1 } });
    expect(ok(`db.users.deleteMany({})`).query).toEqual({ filter: {} });
  });

  it("parses aggregate pipeline", () => {
    const r = ok(`db.users.aggregate([{$match: {a: 1}}, {$group: {_id: "$a", n: {$sum: 1}}}])`);
    expect(r.operation).toBe("aggregate");
    expect(r.query).toEqual({
      pipeline: [{ $match: { a: 1 } }, { $group: { _id: "$a", n: { $sum: 1 } } }],
    });
  });

  it("maps count() to countDocuments", () => {
    const r = ok(`db.users.count({a: 1})`);
    expect(r.operation).toBe("countDocuments");
    expect(r.query).toEqual({ filter: { a: 1 } });
  });

  it("parses countDocuments", () => {
    const r = ok(`db.users.countDocuments({})`);
    expect(r.operation).toBe("countDocuments");
  });
});

describe("parseMongosh - EJSON literals", () => {
  it("ObjectId → $oid", () => {
    const r = ok(`db.c.find({_id: ObjectId("abc")})`);
    expect(r.query.filter).toEqual({ _id: { $oid: "abc" } });
  });

  it("ISODate → $date", () => {
    const r = ok(`db.c.find({t: {$gte: ISODate("2024-01-01T00:00:00Z")}})`);
    expect(r.query.filter).toEqual({ t: { $gte: { $date: "2024-01-01T00:00:00Z" } } });
  });

  it("new Date() → $date", () => {
    const r = ok(`db.c.find({t: new Date("2024-01-01")})`);
    expect(r.query.filter).toEqual({ t: { $date: "2024-01-01" } });
  });

  it("NumberLong → $numberLong", () => {
    const r = ok(`db.c.find({big: NumberLong("9223372036854775807")})`);
    expect(r.query.filter).toEqual({ big: { $numberLong: "9223372036854775807" } });
  });

  it("NumberDecimal → $numberDecimal", () => {
    const r = ok(`db.c.find({p: NumberDecimal("19.99")})`);
    expect(r.query.filter).toEqual({ p: { $numberDecimal: "19.99" } });
  });

  it("regex literal → $regex + $options", () => {
    const r = ok(`db.c.find({name: /alice/i})`);
    expect(r.query.filter).toEqual({ name: { $regex: "alice", $options: "i" } });
  });

  it("negative numbers via UnaryExpression", () => {
    const r = ok(`db.c.find({}).sort({age: -1})`);
    expect(r.query.sort).toEqual({ age: -1 });
  });

  it("string / number / boolean / null literals", () => {
    const r = ok(`db.c.find({s: "x", n: 1, b: true, z: null})`);
    expect(r.query.filter).toEqual({ s: "x", n: 1, b: true, z: null });
  });

  it("quoted keys", () => {
    const r = ok(`db.c.find({"a.b": 1})`);
    expect(r.query.filter).toEqual({ "a.b": 1 });
  });
});

describe("parseMongosh - database resolution", () => {
  it("uses currentDb when db.C is used", () => {
    expect(ok(`db.users.find({})`, "mydb").database).toBe("mydb");
  });

  it("getSiblingDB overrides database", () => {
    const r = ok(`db.getSiblingDB("admin").users.find({})`, "test");
    expect(r.database).toBe("admin");
    expect(r.collection).toBe("users");
  });

  it("accepts trailing semicolon", () => {
    const r = ok(`db.users.find({});`);
    expect(r.operation).toBe("find");
  });

  it("tolerates surrounding whitespace and newlines", () => {
    const r = ok(`\n  db.users\n    .find({})\n    .limit(5)\n`);
    expect(r.query).toEqual({ filter: {}, limit: 5 });
  });
});

describe("parseMongosh - errors", () => {
  it("empty input", () => {
    expect(err("")).toBeInstanceOf(MongoshParseError);
  });

  it("syntax error", () => {
    expect(err("db.users.find(")).toBeInstanceOf(MongoshParseError);
  });

  it("not starting with db", () => {
    expect(err("users.find({})")).toBeInstanceOf(MongoshParseError);
  });

  it("unknown operation", () => {
    expect(err("db.users.mapReduce({}, {})").message).toMatch(/不支持的操作|mapReduce/);
  });

  it("updateOne missing update arg", () => {
    expect(err("db.users.updateOne({a: 1})").message).toMatch(/update/);
  });

  it("aggregate needs array", () => {
    expect(err("db.users.aggregate({a: 1})").message).toMatch(/数组|pipeline/);
  });

  it("insertMany needs array", () => {
    expect(err("db.users.insertMany({a: 1})").message).toMatch(/数组|documents/);
  });

  it("unknown EJSON function", () => {
    expect(err(`db.users.find({_id: UnknownFn("x")})`).message).toMatch(/UnknownFn|不支持/);
  });

  it("unsupported modifier for find", () => {
    expect(err("db.users.find({}).foo(1)").message).toMatch(/foo|不支持/);
  });

  it("modifier on non-find op", () => {
    expect(err("db.users.deleteOne({}).sort({a: 1})").message).toMatch(/sort|不支持/);
  });

  it("empty database via getSiblingDB", () => {
    expect(err(`db.getSiblingDB("").users.find({})`).message).toMatch(/数据库|database|getSiblingDB/);
  });
});
