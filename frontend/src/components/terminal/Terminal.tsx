import { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState, forwardRef } from "react";
import type { Terminal as XTerminal } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import type { SearchAddon } from "@xterm/addon-search";
import { WriteSSH, WriteSerial, ResizeSSH, ResizeSerialTerminal } from "../../../wailsjs/go/app/App";
import { useShortcutStore, matchShortcut, formatBinding, formatModKey } from "@/stores/shortcutStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useTerminalThemeStore, toXtermTheme } from "@/stores/terminalThemeStore";
import { builtinThemes, defaultLightTheme, defaultDarkTheme } from "@/data/terminalThemes";
import { withTerminalFontFallback } from "@/data/terminalFonts";
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
import { bytesToBase64 } from "@/lib/terminalEncode";
import { getOrCreateTerminal, getTerminalInstance } from "./terminalRegistry";

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
  const wrapperRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const activeRef = useRef(active);
  const [showSearch, setShowSearch] = useState(false);
  const [hasSelection, setHasSelection] = useState(false);
  const shortcuts = useShortcutStore((s) => s.shortcuts);
  const fontSize = useTerminalThemeStore((s) => s.fontSize);
  const fontFamily = useTerminalThemeStore((s) => s.fontFamily);
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
  const isSerial = sessionId.startsWith("serial-");

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
        const writeFn = isSerial ? WriteSerial : WriteSSH;
        writeFn(sessionId, bytesToBase64(new TextEncoder().encode(text))).catch(console.error);
      }
    });
  }, [isSerial, sessionId]);

  const handleSelectAll = useCallback(() => {
    termRef.current?.selectAll();
  }, []);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const inst = getOrCreateTerminal(sessionId, { fontSize, fontFamily, theme: xtermTheme, scrollback });
    termRef.current = inst.term;
    fitAddonRef.current = inst.fitAddon;
    searchAddonRef.current = inst.searchAddon;

    // Attach the persistent host into the React-managed wrapper. Xterm content
    // survives because both the host element and the XTerminal live in the
    // registry, not in this component — so split-pane re-renders that unmount
    // this component don't destroy scrollback.
    wrapper.appendChild(inst.container);

    requestAnimationFrame(() => {
      inst.fitAddon.fit();
    });

    inst.term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      const action = matchShortcut(e, useShortcutStore.getState().shortcuts);
      if (action === "panel.filter" && e.type === "keydown") {
        setShowSearch((v) => !v);
        return false;
      }
      if (e.key === "c" && (e.ctrlKey || e.metaKey) && e.type === "keydown") {
        const selection = inst.term.getSelection();
        if (selection) {
          navigator.clipboard.writeText(selection);
          toast.success(t("ssh.contextMenu.copied"), { duration: 1500 });
          return false;
        }
      }
      return !action;
    });

    const selDispose = inst.term.onSelectionChange(() => {
      setHasSelection(!!inst.term.getSelection());
    });
    setHasSelection(!!inst.term.getSelection());

    let resizeTimer = 0;
    const resizeObserver = new ResizeObserver(() => {
      if (!activeRef.current) return;
      clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        if (!activeRef.current) return;
        inst.fitAddon.fit();
        const dims = inst.fitAddon.proposeDimensions();
        if (dims) {
          const resizeFn = isSerial ? ResizeSerialTerminal : ResizeSSH;
          resizeFn(sessionId, dims.cols, dims.rows).catch(console.error);
        }
      }, 50);
    });
    resizeObserver.observe(wrapper);

    return () => {
      clearTimeout(resizeTimer);
      selDispose.dispose();
      resizeObserver.disconnect();
      // If the registry already disposed this session (e.g. closePane / reconnect /
      // tab close ran before this cleanup), the xterm instance is destroyed —
      // skip any term operations and just detach.
      const stillAlive = getTerminalInstance(sessionId) === inst;
      if (stillAlive) {
        // Drop key handler so its closures can be GC'd; xterm only stores one slot.
        inst.term.attachCustomKeyEventHandler(() => true);
      }
      if (inst.container.parentElement === wrapper) {
        wrapper.removeChild(inst.container);
      }
      termRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  useEffect(() => {
    if (!termRef.current) return;
    termRef.current.options.theme = xtermTheme;
    termRef.current.options.fontSize = fontSize;
    termRef.current.options.fontFamily = withTerminalFontFallback(fontFamily);
    termRef.current.options.scrollback = scrollback;
    fitAddonRef.current?.fit();
  }, [xtermTheme, fontSize, fontFamily, scrollback]);

  useEffect(() => {
    activeRef.current = active;
  }, [active]);

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
          <div ref={wrapperRef} className="h-full w-full p-1" />
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
          <ContextMenuItem onClick={() => splitPane(tabId, "horizontal")} disabled={!paneConnected || isSerial}>
            {t("ssh.session.splitH")}
            <ContextMenuShortcut>{formatBinding(shortcuts["split.horizontal"])}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onClick={() => splitPane(tabId, "vertical")} disabled={!paneConnected || isSerial}>
            {t("ssh.session.splitV")}
            <ContextMenuShortcut>{formatBinding(shortcuts["split.vertical"])}</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuSeparator />
          {!isSerial && (
            <ContextMenuItem onClick={() => toggleFileManager(tabId)}>{t("ssh.contextMenu.sftp")}</ContextMenuItem>
          )}
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
