import { describe, it, expect, beforeAll } from "vitest";
import { pinyinMatch, __ensurePinyinReady } from "../lib/pinyin";

describe("pinyinMatch", () => {
  beforeAll(() => __ensurePinyinReady());

  it("returns true for empty query", () => {
    expect(pinyinMatch("anything", "")).toBe(true);
  });

  it("matches original text (case insensitive)", () => {
    expect(pinyinMatch("Hello World", "hello")).toBe(true);
    expect(pinyinMatch("Hello World", "world")).toBe(true);
    expect(pinyinMatch("Hello World", "xyz")).toBe(false);
  });

  it("matches Chinese characters directly", () => {
    expect(pinyinMatch("中转站", "中转")).toBe(true);
    expect(pinyinMatch("中转站", "站")).toBe(true);
  });

  it("matches by full pinyin", () => {
    expect(pinyinMatch("中转站", "zhongzhuanzhan")).toBe(true);
    expect(pinyinMatch("中转站", "zhongzhuan")).toBe(true);
  });

  it("matches by initials only", () => {
    expect(pinyinMatch("中转站", "zzz")).toBe(true);
    expect(pinyinMatch("中转站", "zz")).toBe(true);
  });

  it("matches by mixed pinyin (full + initials)", () => {
    expect(pinyinMatch("中转站", "zhongzz")).toBe(true);
    expect(pinyinMatch("中转站", "zhuanz")).toBe(true);
  });

  it("returns false for non-matching query", () => {
    expect(pinyinMatch("中转站", "abc")).toBe(false);
    expect(pinyinMatch("中转站", "xxx")).toBe(false);
  });

  it("handles mixed Chinese and English text", () => {
    expect(pinyinMatch("Web服务器", "web")).toBe(true);
    expect(pinyinMatch("Web服务器", "fwq")).toBe(true);
    expect(pinyinMatch("Web服务器", "fuwuqi")).toBe(true);
  });
});
