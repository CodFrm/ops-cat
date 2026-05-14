import { describe, it, expect, beforeEach } from "vitest";
import { useTerminalThemeStore, SCROLLBACK_DEFAULT } from "../stores/terminalThemeStore";
import { builtinThemes, type TerminalTheme } from "../data/terminalThemes";
import { DEFAULT_TERMINAL_FONT_FAMILY } from "../data/terminalFonts";

type TerminalThemeStoreState = ReturnType<typeof useTerminalThemeStore.getState>;

function makeCustomTheme(id: string, name: string): TerminalTheme {
  return {
    id,
    name,
    background: "#000",
    foreground: "#fff",
    cursor: "#fff",
    black: "#000",
    red: "#f00",
    green: "#0f0",
    yellow: "#ff0",
    blue: "#00f",
    magenta: "#f0f",
    cyan: "#0ff",
    white: "#fff",
    brightBlack: "#888",
    brightRed: "#f88",
    brightGreen: "#8f8",
    brightYellow: "#ff8",
    brightBlue: "#88f",
    brightMagenta: "#f8f",
    brightCyan: "#8ff",
    brightWhite: "#fff",
  };
}

describe("terminalThemeStore", () => {
  beforeEach(() => {
    localStorage.clear();
    useTerminalThemeStore.setState({
      selectedThemeId: "default",
      customThemes: [],
      fontSize: 14,
      fontPresetId: "default",
      customFontFamily: "",
      fontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
      scrollback: SCROLLBACK_DEFAULT,
    });
  });

  describe("setSelectedThemeId", () => {
    it("changes the selected theme", () => {
      useTerminalThemeStore.getState().setSelectedThemeId("dracula");
      expect(useTerminalThemeStore.getState().selectedThemeId).toBe("dracula");
    });
  });

  describe("setFontSize", () => {
    it("sets font size within bounds", () => {
      useTerminalThemeStore.getState().setFontSize(20);
      expect(useTerminalThemeStore.getState().fontSize).toBe(20);
    });

    it("clamps to minimum 8", () => {
      useTerminalThemeStore.getState().setFontSize(2);
      expect(useTerminalThemeStore.getState().fontSize).toBe(8);
    });

    it("clamps to maximum 32", () => {
      useTerminalThemeStore.getState().setFontSize(100);
      expect(useTerminalThemeStore.getState().fontSize).toBe(32);
    });
  });

  describe("font presets", () => {
    const defaultFontFamily = DEFAULT_TERMINAL_FONT_FAMILY;

    it("defaults to the existing terminal font stack", () => {
      expect(useTerminalThemeStore.getState().fontFamily).toBe(defaultFontFamily);
    });

    it("selects a known preset and applies its terminal font family", () => {
      const state: TerminalThemeStoreState = useTerminalThemeStore.getState();

      state.setFontPresetId("fira-code");

      expect(useTerminalThemeStore.getState().fontPresetId).toBe("fira-code");
      expect(useTerminalThemeStore.getState().fontFamily).toBe("'Fira Code'");
    });

    it("treats an unknown id as a system font family name", () => {
      const state: TerminalThemeStoreState = useTerminalThemeStore.getState();

      state.setFontPresetId("JetBrainsMono NFM");

      expect(useTerminalThemeStore.getState().fontPresetId).toBe("JetBrainsMono NFM");
      expect(useTerminalThemeStore.getState().fontFamily).toBe("'JetBrainsMono NFM'");
    });

    it("falls back to default when the id is whitespace-only", () => {
      const state: TerminalThemeStoreState = useTerminalThemeStore.getState();

      state.setFontPresetId("   ");

      expect(useTerminalThemeStore.getState().fontPresetId).toBe("default");
      expect(useTerminalThemeStore.getState().fontFamily).toBe(defaultFontFamily);
    });

    it("uses the edited custom font family and falls back when blank", () => {
      const state: TerminalThemeStoreState = useTerminalThemeStore.getState();

      state.setCustomFontFamily("  Iosevka Term, monospace  ");

      expect(useTerminalThemeStore.getState().fontPresetId).toBe("custom");
      expect(useTerminalThemeStore.getState().customFontFamily).toBe("Iosevka Term, monospace");
      expect(useTerminalThemeStore.getState().fontFamily).toBe("Iosevka Term, monospace");

      useTerminalThemeStore.getState().setCustomFontFamily("   ");

      expect(useTerminalThemeStore.getState().customFontFamily).toBe("");
      expect(useTerminalThemeStore.getState().fontFamily).toBe(defaultFontFamily);
    });
  });

  describe("setScrollback", () => {
    it("defaults to 25000", () => {
      expect(useTerminalThemeStore.getState().scrollback).toBe(25000);
      expect(SCROLLBACK_DEFAULT).toBe(25000);
    });

    it("sets scrollback within bounds", () => {
      useTerminalThemeStore.getState().setScrollback(5000);
      expect(useTerminalThemeStore.getState().scrollback).toBe(5000);
    });

    it("clamps to minimum 100", () => {
      useTerminalThemeStore.getState().setScrollback(10);
      expect(useTerminalThemeStore.getState().scrollback).toBe(100);
    });

    it("clamps to maximum 1000000", () => {
      useTerminalThemeStore.getState().setScrollback(99_999_999);
      expect(useTerminalThemeStore.getState().scrollback).toBe(1_000_000);
    });

    it("floors fractional values", () => {
      useTerminalThemeStore.getState().setScrollback(1234.9);
      expect(useTerminalThemeStore.getState().scrollback).toBe(1234);
    });

    it("falls back to default for non-finite input", () => {
      useTerminalThemeStore.getState().setScrollback(Number.NaN);
      expect(useTerminalThemeStore.getState().scrollback).toBe(SCROLLBACK_DEFAULT);
    });
  });

  describe("custom theme CRUD", () => {
    it("adds a custom theme", () => {
      const theme = makeCustomTheme("c1", "My Theme");
      useTerminalThemeStore.getState().addCustomTheme(theme);

      expect(useTerminalThemeStore.getState().customThemes).toHaveLength(1);
      expect(useTerminalThemeStore.getState().customThemes[0].id).toBe("c1");
    });

    it("updates a custom theme", () => {
      const theme = makeCustomTheme("c1", "My Theme");
      useTerminalThemeStore.getState().addCustomTheme(theme);

      const updated = { ...theme, name: "Renamed Theme" };
      useTerminalThemeStore.getState().updateCustomTheme(updated);

      expect(useTerminalThemeStore.getState().customThemes[0].name).toBe("Renamed Theme");
    });

    it("removes a custom theme", () => {
      const theme = makeCustomTheme("c1", "My Theme");
      useTerminalThemeStore.getState().addCustomTheme(theme);
      useTerminalThemeStore.getState().removeCustomTheme("c1");

      expect(useTerminalThemeStore.getState().customThemes).toHaveLength(0);
    });

    it("resets selectedThemeId to default when removing selected custom theme", () => {
      const theme = makeCustomTheme("c1", "My Theme");
      useTerminalThemeStore.getState().addCustomTheme(theme);
      useTerminalThemeStore.getState().setSelectedThemeId("c1");

      useTerminalThemeStore.getState().removeCustomTheme("c1");

      expect(useTerminalThemeStore.getState().selectedThemeId).toBe("default");
    });

    it("keeps selectedThemeId when removing a different custom theme", () => {
      const t1 = makeCustomTheme("c1", "Theme 1");
      const t2 = makeCustomTheme("c2", "Theme 2");
      useTerminalThemeStore.getState().addCustomTheme(t1);
      useTerminalThemeStore.getState().addCustomTheme(t2);
      useTerminalThemeStore.getState().setSelectedThemeId("c2");

      useTerminalThemeStore.getState().removeCustomTheme("c1");

      expect(useTerminalThemeStore.getState().selectedThemeId).toBe("c2");
    });
  });

  describe("getActiveTheme", () => {
    it("returns first builtin theme for default", () => {
      useTerminalThemeStore.getState().setSelectedThemeId("default");
      const active = useTerminalThemeStore.getState().getActiveTheme();
      expect(active).toEqual(builtinThemes[0]);
    });

    it("returns matching builtin theme", () => {
      useTerminalThemeStore.getState().setSelectedThemeId("dracula");
      const active = useTerminalThemeStore.getState().getActiveTheme();
      expect(active.id).toBe("dracula");
    });

    it("returns matching custom theme", () => {
      const theme = makeCustomTheme("c1", "My Theme");
      useTerminalThemeStore.getState().addCustomTheme(theme);
      useTerminalThemeStore.getState().setSelectedThemeId("c1");

      const active = useTerminalThemeStore.getState().getActiveTheme();
      expect(active.id).toBe("c1");
      expect(active.name).toBe("My Theme");
    });

    it("falls back to builtinThemes[0] for unknown ID", () => {
      useTerminalThemeStore.getState().setSelectedThemeId("nonexistent");
      const active = useTerminalThemeStore.getState().getActiveTheme();
      expect(active).toEqual(builtinThemes[0]);
    });
  });
});
