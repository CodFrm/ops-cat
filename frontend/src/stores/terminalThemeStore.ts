import { create } from "zustand";
import { persist } from "zustand/middleware";
import { TerminalTheme, builtinThemes } from "@/data/terminalThemes";

export const SCROLLBACK_MIN = 100;
export const SCROLLBACK_MAX = 1000000;
export const SCROLLBACK_DEFAULT = 25000;

interface TerminalThemeState {
  selectedThemeId: string;
  customThemes: TerminalTheme[];
  fontSize: number;
  scrollback: number;

  setSelectedThemeId: (id: string) => void;
  setFontSize: (size: number) => void;
  setScrollback: (lines: number) => void;
  addCustomTheme: (theme: TerminalTheme) => void;
  updateCustomTheme: (theme: TerminalTheme) => void;
  removeCustomTheme: (id: string) => void;
  getActiveTheme: () => TerminalTheme;
}

export const useTerminalThemeStore = create<TerminalThemeState>()(
  persist(
    (set, get) => ({
      selectedThemeId: "default",
      customThemes: [],
      fontSize: 14,
      scrollback: SCROLLBACK_DEFAULT,

      setSelectedThemeId: (id) => set({ selectedThemeId: id }),

      setFontSize: (size) => set({ fontSize: Math.max(8, Math.min(32, size)) }),

      setScrollback: (lines) => {
        const n = Number.isFinite(lines) ? Math.floor(lines) : SCROLLBACK_DEFAULT;
        set({ scrollback: Math.max(SCROLLBACK_MIN, Math.min(SCROLLBACK_MAX, n)) });
      },

      addCustomTheme: (theme) =>
        set((state) => ({
          customThemes: [...state.customThemes, theme],
        })),

      updateCustomTheme: (theme) =>
        set((state) => ({
          customThemes: state.customThemes.map((t) => (t.id === theme.id ? theme : t)),
        })),

      removeCustomTheme: (id) =>
        set((state) => ({
          customThemes: state.customThemes.filter((t) => t.id !== id),
          // 如果删除的是当前选中的，回退到默认
          selectedThemeId: state.selectedThemeId === id ? "default" : state.selectedThemeId,
        })),

      getActiveTheme: () => {
        const { selectedThemeId, customThemes } = get();
        return (
          builtinThemes.find((t) => t.id === selectedThemeId) ||
          customThemes.find((t) => t.id === selectedThemeId) ||
          builtinThemes[0]
        );
      },
    }),
    {
      name: "terminal_theme",
    }
  )
);

/** 将 TerminalTheme 转换为 xterm.js ITheme 对象 */
export function toXtermTheme(theme: TerminalTheme) {
  return {
    background: theme.background,
    foreground: theme.foreground,
    cursor: theme.cursor,
    cursorAccent: theme.cursorAccent,
    selectionBackground: theme.selectionBackground,
    black: theme.black,
    red: theme.red,
    green: theme.green,
    yellow: theme.yellow,
    blue: theme.blue,
    magenta: theme.magenta,
    cyan: theme.cyan,
    white: theme.white,
    brightBlack: theme.brightBlack,
    brightRed: theme.brightRed,
    brightGreen: theme.brightGreen,
    brightYellow: theme.brightYellow,
    brightBlue: theme.brightBlue,
    brightMagenta: theme.brightMagenta,
    brightCyan: theme.brightCyan,
    brightWhite: theme.brightWhite,
  };
}
