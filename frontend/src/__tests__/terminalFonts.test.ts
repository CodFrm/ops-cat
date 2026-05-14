import { afterEach, describe, expect, it, vi } from "vitest";
import {
  buildTerminalFontGroups,
  loadInstalledFonts,
  quoteFamilyName,
  RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES,
  resolveDefaultFontPrimary,
  resolveTerminalFontFamily,
  terminalFontPresets,
  withTerminalFontFallback,
} from "../data/terminalFonts";

describe("terminalFonts", () => {
  it("keeps font presets as static choices without system detection metadata", () => {
    expect(terminalFontPresets.some((preset) => "systemFontNames" in preset)).toBe(false);
  });

  it("keeps preset values as the primary font only", () => {
    expect(terminalFontPresets.find((preset) => preset.id === "fira-code")?.fontFamily).toBe("'Fira Code'");
  });

  const DEFAULT_STACK =
    "'JetBrainsMono NFM', 'JetBrainsMono Nerd Font Mono', 'MesloLGM NF', 'MesloLGM Nerd Font', " +
    "'FiraCode NFM', 'FiraCode Nerd Font Mono', 'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace";

  it("uses the default font stack when the custom value is blank", () => {
    expect(resolveTerminalFontFamily("  ")).toBe(DEFAULT_STACK);
  });

  it("keeps custom font family values unexpanded for storage", () => {
    expect(resolveTerminalFontFamily("Iosevka Term, monospace")).toBe("Iosevka Term, monospace");
  });

  it("adds shared fallbacks at terminal runtime without duplicating the primary font", () => {
    expect(withTerminalFontFallback("'Fira Code'")).toBe(
      "'Fira Code', 'JetBrainsMono NFM', 'JetBrainsMono Nerd Font Mono', 'MesloLGM NF', 'MesloLGM Nerd Font', " +
        "'FiraCode NFM', 'FiraCode Nerd Font Mono', 'JetBrains Mono', 'Cascadia Code', Menlo, monospace"
    );
  });

  it("strips trailing generic families before adding runtime fallbacks", () => {
    expect(withTerminalFontFallback("Iosevka Term, monospace")).toBe("Iosevka Term, " + DEFAULT_STACK);
  });

  it("uses the default runtime fallback when the runtime font value is blank", () => {
    expect(withTerminalFontFallback("  ")).toBe(DEFAULT_STACK);
  });

  it("does not duplicate the default runtime fallback stack", () => {
    expect(resolveTerminalFontFamily("Iosevka Term, monospace")).toBe("Iosevka Term, monospace");
    expect(withTerminalFontFallback(DEFAULT_STACK)).toBe(DEFAULT_STACK);
  });

  describe("quoteFamilyName", () => {
    it("leaves identifier-shaped names unquoted", () => {
      expect(quoteFamilyName("Menlo")).toBe("Menlo");
      expect(quoteFamilyName("monospace")).toBe("monospace");
    });

    it("quotes names with spaces or special characters", () => {
      expect(quoteFamilyName("JetBrainsMono NFM")).toBe("'JetBrainsMono NFM'");
      expect(quoteFamilyName("Source Code Pro")).toBe("'Source Code Pro'");
    });

    it("escapes embedded apostrophes", () => {
      expect(quoteFamilyName("Foo's Font")).toBe("'Foo\\'s Font'");
    });

    // A trailing backslash would otherwise escape the closing quote and break
    // the CSS string, so backslashes must be escaped before apostrophes are.
    it("escapes embedded backslashes before apostrophes", () => {
      expect(quoteFamilyName("Foo\\Bar")).toBe("'Foo\\\\Bar'");
      expect(quoteFamilyName("Trailing\\")).toBe("'Trailing\\\\'");
      expect(quoteFamilyName("Mix\\'s")).toBe("'Mix\\\\\\'s'");
    });

    it("strips control characters before quoting", () => {
      expect(quoteFamilyName("Foo\nBar")).toBe("FooBar");
      expect(quoteFamilyName("\tJetBrains Mono\r")).toBe("'JetBrains Mono'");
    });
  });

  describe("RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES", () => {
    it("lists Nerd Font Mono variants before plain monospace fonts", () => {
      const idx = (name: string) => RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES.indexOf(name);
      expect(idx("JetBrainsMono NFM")).toBeGreaterThanOrEqual(0);
      expect(idx("JetBrains Mono")).toBeGreaterThanOrEqual(0);
      expect(idx("JetBrainsMono NFM")).toBeLessThan(idx("JetBrains Mono"));
    });

    it("includes both v3.0 long and v3.1+ short Nerd Font names", () => {
      expect(RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES).toContain("JetBrainsMono NFM");
      expect(RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES).toContain("JetBrainsMono Nerd Font Mono");
    });
  });

  describe("loadInstalledFonts", () => {
    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it("returns null when queryLocalFonts is not available", async () => {
      vi.stubGlobal("window", {});
      expect(await loadInstalledFonts()).toBeNull();
    });

    it("returns null when the API rejects (e.g. permission denied)", async () => {
      vi.stubGlobal("window", {
        queryLocalFonts: () => Promise.reject(new Error("denied")),
      });
      expect(await loadInstalledFonts()).toBeNull();
    });

    it("returns deduplicated alphabetically-sorted family names from queryLocalFonts", async () => {
      vi.stubGlobal("window", {
        queryLocalFonts: () =>
          Promise.resolve([
            { family: "Zed Mono" },
            { family: "Menlo" },
            { family: "Zed Mono" }, // duplicate
            { family: "JetBrains Mono" },
          ]),
      });
      expect(await loadInstalledFonts()).toEqual(["JetBrains Mono", "Menlo", "Zed Mono"]);
    });

    it("normalizes installed font family names before returning them", async () => {
      vi.stubGlobal("window", {
        queryLocalFonts: () =>
          Promise.resolve([{ family: " Zed Mono\n" }, { family: "Bad\u0000Name" }, { family: "\t" }]),
      });
      expect(await loadInstalledFonts()).toEqual(["BadName", "Zed Mono"]);
    });

    it("falls back to the Wails backend when queryLocalFonts is unavailable", async () => {
      vi.stubGlobal("window", {
        go: {
          app: {
            App: {
              ListSystemFonts: () => Promise.resolve(["Zed Mono", "Menlo", "Zed Mono"]),
            },
          },
        },
      });
      expect(await loadInstalledFonts()).toEqual(["Menlo", "Zed Mono"]);
    });

    it("falls back to the Wails backend when queryLocalFonts rejects", async () => {
      vi.stubGlobal("window", {
        queryLocalFonts: () => Promise.reject(new Error("denied")),
        go: {
          app: {
            App: {
              ListSystemFonts: () => Promise.resolve(["Backend Mono"]),
            },
          },
        },
      });
      expect(await loadInstalledFonts()).toEqual(["Backend Mono"]);
    });
  });

  describe("buildTerminalFontGroups", () => {
    it("uses the curated static recommendations when installed fonts are unavailable", () => {
      const groups = buildTerminalFontGroups(null);
      expect(groups.otherFonts).toEqual([]);
      expect(groups.recommendedFonts.map((font) => font.name)).toContain("JetBrains Mono");
      expect(groups.recommendedFonts.find((font) => font.name === "Fira Code")?.value).toBe("fira-code");
    });

    it("keeps recommended fonts in curated order and other fonts alphabetically sorted", () => {
      const groups = buildTerminalFontGroups(["Zed Mono", "Menlo", "Fira Code", "JetBrains Mono"]);
      expect(groups.recommendedFonts.map((font) => font.name)).toEqual(["JetBrains Mono", "Fira Code", "Menlo"]);
      expect(groups.recommendedFonts.map((font) => font.value)).toEqual(["jetbrains-mono", "fira-code", "menlo"]);
      expect(groups.otherFonts.map((font) => font.name)).toEqual(["Zed Mono"]);
    });
  });

  describe("resolveDefaultFontPrimary", () => {
    it("returns null when installed list is null", () => {
      expect(resolveDefaultFontPrimary(null)).toBeNull();
    });

    it("returns null when installed list is empty", () => {
      expect(resolveDefaultFontPrimary([])).toBeNull();
    });

    it("returns the first fallback entry that is installed", () => {
      expect(resolveDefaultFontPrimary(["JetBrainsMono NFM", "Menlo"])).toBe("JetBrainsMono NFM");
      expect(resolveDefaultFontPrimary(["Menlo", "JetBrains Mono"])).toBe("JetBrains Mono");
    });

    it("returns null when no real family in the chain is installed (generic monospace doesn't count)", () => {
      expect(resolveDefaultFontPrimary(["SomeRandomFont"])).toBeNull();
    });
  });
});
