import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from "react";
import { Terminal as XTerminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { useTerminalThemeStore, toXtermTheme } from "@/stores/terminalThemeStore";
import { builtinThemes, defaultLightTheme, defaultDarkTheme } from "@/data/terminalThemes";
import { useResolvedTheme } from "@/components/theme-provider";

export interface K8sLogTerminalHandle {
  write: (data: string | Uint8Array) => void;
  clear: () => void;
}

export const K8sLogTerminal = forwardRef<K8sLogTerminalHandle>(function K8sLogTerminal(_, ref) {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);

  const fontSize = useTerminalThemeStore((s) => s.fontSize);
  const scrollback = useTerminalThemeStore((s) => s.scrollback);
  const selectedThemeId = useTerminalThemeStore((s) => s.selectedThemeId);
  const customThemes = useTerminalThemeStore((s) => s.customThemes);
  const resolvedTheme = useResolvedTheme();

  const xtermTheme = useMemo(() => {
    if (selectedThemeId === "default") {
      return resolvedTheme === "light" ? toXtermTheme(defaultLightTheme) : toXtermTheme(defaultDarkTheme);
    }
    const theme =
      builtinThemes.find((t) => t.id === selectedThemeId) || customThemes.find((t) => t.id === selectedThemeId);
    return theme ? toXtermTheme(theme) : undefined;
  }, [selectedThemeId, customThemes, resolvedTheme]);

  useImperativeHandle(ref, () => ({
    write: (data: string | Uint8Array) => {
      termRef.current?.write(data);
    },
    clear: () => {
      termRef.current?.clear();
    },
  }));

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const term = new XTerminal({
      cursorBlink: false,
      fontSize,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
      theme: xtermTheme,
      scrollback,
      disableStdin: true,
      convertEol: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(wrapper);

    fitAddon.fit();
    termRef.current = term;
    fitAddonRef.current = fitAddon;

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(wrapper);

    return () => {
      resizeObserver.disconnect();
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!termRef.current) return;
    termRef.current.options.theme = xtermTheme;
    termRef.current.options.fontSize = fontSize;
    termRef.current.options.scrollback = scrollback;
    fitAddonRef.current?.fit();
  }, [xtermTheme, fontSize, scrollback]);

  return <div ref={wrapperRef} className="flex-1 w-full rounded-lg overflow-hidden min-h-0" />;
});
