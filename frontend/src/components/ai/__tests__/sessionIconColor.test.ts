import { describe, it, expect } from "vitest";
import { getSessionIconLetter } from "../sessionIconColor";

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
