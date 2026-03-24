import { useEffect, useMemo, useRef } from "react";
import { Terminal as XTerminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { WriteSSH, ResizeSSH } from "../../../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { useShortcutStore, matchShortcut } from "@/stores/shortcutStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useTerminalThemeStore, toXtermTheme } from "@/stores/terminalThemeStore";
import { builtinThemes } from "@/data/terminalThemes";

interface TerminalProps {
  sessionId: string;
  active: boolean;
}

export function Terminal({ sessionId, active }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const activeRef = useRef(active);
  const fontSize = useTerminalThemeStore((s) => s.fontSize);
  const selectedThemeId = useTerminalThemeStore((s) => s.selectedThemeId);
  const customThemes = useTerminalThemeStore((s) => s.customThemes);
  const xtermTheme = useMemo(() => {
    if (selectedThemeId === "default") return undefined;
    const theme =
      builtinThemes.find((t) => t.id === selectedThemeId) ||
      customThemes.find((t) => t.id === selectedThemeId);
    return theme ? toXtermTheme(theme) : undefined;
  }, [selectedThemeId, customThemes]);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerminal({
      cursorBlink: true,
      fontSize,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
      theme: xtermTheme,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(containerRef.current);

    // 初始 fit
    requestAnimationFrame(() => {
      fitAddon.fit();
    });

    // Let global shortcut handler handle shortcut keys instead of xterm
    term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      return !matchShortcut(e, useShortcutStore.getState().shortcuts);
    });

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // 用户输入 → 后端
    const onDataDispose = term.onData((data) => {
      const encoded = btoa(
        String.fromCharCode(...new TextEncoder().encode(data))
      );
      WriteSSH(sessionId, encoded).catch(console.error);
    });

    // 后端输出 → 终端
    const eventName = "ssh:data:" + sessionId;
    EventsOn(eventName, (dataB64: string) => {
      const binary = atob(dataB64);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
      }
      term.write(bytes);
    });

    // 会话关闭事件
    const closedEvent = "ssh:closed:" + sessionId;
    EventsOn(closedEvent, () => {
      term.write("\r\n\x1b[31m[Connection closed]\x1b[0m\r\n");
      useTerminalStore.getState().markClosed(sessionId);
    });

    // 窗口尺寸变化（debounce 避免过渡动画期间密集 refit）
    let resizeTimer = 0;
    const resizeObserver = new ResizeObserver(() => {
      if (!activeRef.current) return; // 非活动 tab 跳过 resize
      clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        if (!activeRef.current) return;
        fitAddon.fit();
        const dims = fitAddon.proposeDimensions();
        if (dims) {
          ResizeSSH(sessionId, dims.cols, dims.rows).catch(console.error);
        }
      }, 50);
    });
    resizeObserver.observe(containerRef.current);

    return () => {
      clearTimeout(resizeTimer);
      onDataDispose.dispose();
      EventsOff(eventName);
      EventsOff(closedEvent);
      resizeObserver.disconnect();
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
    };
  }, [sessionId]);

  // 主题或字体变更时实时刷新终端
  useEffect(() => {
    if (!termRef.current) return;
    termRef.current.options.theme = xtermTheme;
    termRef.current.options.fontSize = fontSize;
    fitAddonRef.current?.fit();
  }, [xtermTheme, fontSize]);

  // 同步 active 状态到 ref，供 ResizeObserver 闭包读取
  useEffect(() => {
    activeRef.current = active;
  }, [active]);

  // 当切换回来时 refit 并聚焦（页面切换期间容器尺寸可能变化）
  useEffect(() => {
    if (active) {
      requestAnimationFrame(() => {
        fitAddonRef.current?.fit();
        termRef.current?.focus();
      });
    }
  }, [active]);

  return (
    <div
      ref={containerRef}
      className="h-full w-full"
      style={{ padding: "4px" }}
    />
  );
}
