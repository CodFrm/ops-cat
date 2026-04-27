import { describe, it, expect } from "vitest";
import { getSessionIconLetter, getSessionIconColor } from "../sessionIconColor";

describe("getSessionIconLetter", () => {
  it("returns the first Chinese character", () => {
    expect(getSessionIconLetter("写迁移")).toBe("写");
  });

  it("uppercases the first ASCII letter", () => {
    expect(getSessionIconLetter("ssh debug")).toBe("S");
  });

  it("trims leading whitespace before extracting", () => {
    expect(getSessionIconLetter("  ssh")).toBe("S");
  });

  it("preserves a leading emoji as-is", () => {
    expect(getSessionIconLetter("🐛 调研")).toBe("🐛");
  });

  it("returns '?' for an empty string", () => {
    expect(getSessionIconLetter("")).toBe("?");
  });

  it("returns '?' for whitespace-only input", () => {
    expect(getSessionIconLetter("   ")).toBe("?");
  });

  it("strips leading ASCII punctuation, then uppercases", () => {
    expect(getSessionIconLetter("@user")).toBe("U");
    expect(getSessionIconLetter("--draft")).toBe("D");
  });
});

describe("getSessionIconColor", () => {
  it("returns the same color for the same title", () => {
    expect(getSessionIconColor("写迁移")).toEqual(getSessionIconColor("写迁移"));
  });

  it("returns an object with bg and fg strings", () => {
    const c = getSessionIconColor("hello");
    expect(typeof c.bg).toBe("string");
    expect(typeof c.fg).toBe("string");
    expect(c.bg.length).toBeGreaterThan(0);
    expect(c.fg.length).toBeGreaterThan(0);
  });

  it("distributes different titles across the palette", () => {
    const titles = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j"];
    const bgSet = new Set(titles.map((t) => getSessionIconColor(t).bg));
    expect(bgSet.size).toBeGreaterThanOrEqual(4);
  });

  it("returns a valid color for empty title (falls back to a default bucket)", () => {
    const c = getSessionIconColor("");
    expect(c.bg.length).toBeGreaterThan(0);
  });
});
