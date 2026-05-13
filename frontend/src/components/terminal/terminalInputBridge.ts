import type { Terminal as XTerminal } from "@xterm/xterm";
import { matchShortcut, type ShortcutAction, type ShortcutBinding } from "@/stores/shortcutStore";

export interface TerminalInputBridgeOptions {
  term: XTerminal;
  shortcuts: Record<ShortcutAction, ShortcutBinding>;
  onFilter: () => void;
  onCopy: () => boolean;
}

export interface TerminalInputBridge {
  setShortcuts(s: Record<ShortcutAction, ShortcutBinding>): void;
  setOnFilter(cb: () => void): void;
  setOnCopy(cb: () => boolean): void;
  dispose(): void;
}

export function createTerminalInputBridge(opts: TerminalInputBridgeOptions): TerminalInputBridge {
  const { term } = opts;
  let shortcuts = opts.shortcuts;
  let onFilter = opts.onFilter;
  let onCopy = opts.onCopy;
  let disposed = false;

  const handler = (e: KeyboardEvent): boolean => {
    if (disposed) return true;
    // W3C UI Events §5.4.3: 应用层在 IME composition / keyCode=229 时必须早返回，
    // 否则可能误吞掉应交给 IME / xterm 内部处理的按键，导致字符丢失。
    if (e.isComposing || e.keyCode === 229) return true;

    const action = matchShortcut(e, shortcuts);
    if (action === "panel.filter" && e.type === "keydown") {
      onFilter();
      return false;
    }
    if (e.key === "c" && (e.ctrlKey || e.metaKey) && e.type === "keydown") {
      if (onCopy()) return false;
    }
    return !action;
  };

  term.attachCustomKeyEventHandler(handler);

  return {
    setShortcuts(s) {
      shortcuts = s;
    },
    setOnFilter(cb) {
      onFilter = cb;
    },
    setOnCopy(cb) {
      onCopy = cb;
    },
    dispose() {
      disposed = true;
      term.attachCustomKeyEventHandler(() => true);
    },
  };
}
