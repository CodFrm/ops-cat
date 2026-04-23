import { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState, forwardRef } from "react";
import { Terminal as XTerminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import "@xterm/xterm/css/xterm.css";
import { WriteSSH, ResizeSSH } from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { useShortcutStore, matchShortcut, formatBinding, formatModKey } from "@/stores/shortcutStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useTerminalThemeStore, toXtermTheme } from "@/stores/terminalThemeStore";
import { builtinThemes, defaultLightTheme, defaultDarkTheme } from "@/data/terminalThemes";
import { useResolvedTheme } from "@/components/theme-provider";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuShortcut,
} from "@opskat/ui";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { useSFTPStore } from "@/stores/sftpStore";
import { useTabStore } from "@/stores/tabStore";

// Chunked binary -> base64 to avoid stack overflow on large pastes (>64KB).
// String.fromCharCode(...largeArray) uses function-argument spread which hits V8's
// ~65535 args limit and throws RangeError.
function bytesToBase64(bytes: Uint8Array): string {
  const CHUNK = 0x8000; // 32 KiB — safely under V8 args limit
  let binary = "";
  for (let i = 0; i < bytes.length; i += CHUNK) {
    const slice = bytes.subarray(i, i + CHUNK);
    binary += String.fromCharCode.apply(null, slice as unknown as number[]);
  }
  return btoa(binary);
}

export interface TerminalHandle {
  toggleSearch: () => void;
}

interface TerminalProps {
  sessionId: string;
  active: boolean;
  tabId: string;
}

export const Terminal = forwardRef<TerminalHandle, TerminalProps>(function Terminal({ sessionId, active, tabId }, ref) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const activeRef = useRef(active);
  const [showSearch, setShowSearch] = useState(false);
  const [hasSelection, setHasSelection] = useState(false);
  const shortcuts = useShortcutStore((s) => s.shortcuts);
  const fontSize = useTerminalThemeStore((s) => s.fontSize);
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
    toggleSearch: () => setShowSearch((v) => !v),
  }));

  const handleCopy = useCallback(() => {
    const selection = termRef.current?.getSelection();
    if (selection) {
      navigator.clipboard.writeText(selection);
      toast.success(t("ssh.contextMenu.copied"), { duration: 1500 });
    }
  }, [t]);

  const handlePaste = useCallback(() => {
    navigator.clipboard.readText().then((text) => {
      if (text && termRef.current) {
        WriteSSH(sessionId, bytesToBase64(new TextEncoder().encode(text))).catch(console.error);
      }
    });
  }, [sessionId]);

  const handleSelectAll = useCallback(() => {
    termRef.current?.selectAll();
  }, []);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerminal({
      cursorBlink: true,
      fontSize,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
      theme: xtermTheme,
    });

    const fitAddon = new FitAddon();
    const searchAddon = new SearchAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(searchAddon);
    term.open(containerRef.current);

    // 初始 fit
    requestAnimationFrame(() => {
      fitAddon.fit();
    });

    // Let global shortcut handler handle shortcut keys instead of xterm.
    // The panel.filter binding also opens in-terminal search when xterm is focused
    // (matched via current user binding — rebinding the shortcut moves both together).
    term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      const action = matchShortcut(e, useShortcutStore.getState().shortcuts);
      if (action === "panel.filter" && e.type === "keydown") {
        setShowSearch((v) => !v);
        return false;
      }
      // Cmd+C (Mac) or Ctrl+C (non-Mac): copy selection + show toast
      if (e.key === "c" && (e.ctrlKey || e.metaKey) && e.type === "keydown") {
        const selection = term.getSelection();
        if (selection) {
          navigator.clipboard.writeText(selection);
          toast.success(t("ssh.contextMenu.copied"), { duration: 1500 });
          return false;
        }
      }
      return !action;
    });

    termRef.current = term;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;

    // 跟踪选中状态
    const selDispose = term.onSelectionChange(() => {
      setHasSelection(!!term.getSelection());
    });

    // 用户输入 → 后端
    const onDataDispose = term.onData((data) => {
      WriteSSH(sessionId, bytesToBase64(new TextEncoder().encode(data))).catch(console.error);
    });

    // 后端输出 → 终端（Go 侧已做 10ms 合并，此处直接写入，依赖 xterm.js 内部写入缓冲）
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
      selDispose.dispose();
      onDataDispose.dispose();
      EventsOff(eventName);
      EventsOff(closedEvent);
      resizeObserver.disconnect();
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  const paneConnected = useTerminalStore((s) => s.tabData[tabId]?.panes[sessionId]?.connected ?? false);
  const splitPane = useTerminalStore((s) => s.splitPane);
  const reconnect = useTerminalStore((s) => s.reconnect);
  const closePane = useTerminalStore((s) => s.closePane);
  const toggleFileManager = useSFTPStore((s) => s.toggleFileManager);
  const closeTab = useTabStore((s) => s.closeTab);

  return (
    <div className="relative h-full w-full flex flex-col">
      <TerminalSearchBar
        visible={showSearch}
        onClose={() => {
          setShowSearch(false);
          termRef.current?.focus();
        }}
        searchAddon={searchAddonRef.current}
      />
      <ContextMenu
        onOpenChange={(open) => {
          if (!open) {
            requestAnimationFrame(() => termRef.current?.focus());
          }
        }}
      >
        <ContextMenuTrigger className="flex-1 min-h-0">
          <div ref={containerRef} className="h-full w-full" style={{ padding: "4px" }} />
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem onClick={handleCopy} disabled={!hasSelection}>
            {t("ssh.contextMenu.copy")}
            <ContextMenuShortcut>{formatModKey("KeyC")}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onClick={handlePaste}>
            {t("ssh.contextMenu.paste")}
            <ContextMenuShortcut>{formatModKey("KeyV")}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onClick={handleSelectAll}>
            {t("ssh.contextMenu.selectAll")}
            <ContextMenuShortcut>{formatModKey("KeyA")}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onClick={() => setShowSearch(true)}>
            {t("ssh.contextMenu.find")}
            <ContextMenuShortcut>{formatBinding(shortcuts["panel.filter"])}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onClick={() => splitPane(tabId, "horizontal")} disabled={!paneConnected}>
            {t("ssh.session.splitH")}
            <ContextMenuShortcut>{formatBinding(shortcuts["split.horizontal"])}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onClick={() => splitPane(tabId, "vertical")} disabled={!paneConnected}>
            {t("ssh.session.splitV")}
            <ContextMenuShortcut>{formatBinding(shortcuts["split.vertical"])}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onClick={() => toggleFileManager(tabId)}>{t("ssh.contextMenu.sftp")}</ContextMenuItem>
          <ContextMenuItem onClick={() => reconnect(tabId)}>{t("ssh.session.reconnect")}</ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onClick={() => closePane(tabId, sessionId)}>
            {t("ssh.contextMenu.closePane")}
          </ContextMenuItem>
          <ContextMenuItem onClick={() => closeTab(tabId)} variant="destructive">
            {t("ssh.contextMenu.closeTab")}
            <ContextMenuShortcut>{formatBinding(shortcuts["tab.close"])}</ContextMenuShortcut>
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  );
});
