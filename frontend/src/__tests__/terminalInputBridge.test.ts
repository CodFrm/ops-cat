import { describe, it, expect, vi, beforeEach } from "vitest";
import type { Terminal as XTerminal } from "@xterm/xterm";
import { createTerminalInputBridge } from "@/components/terminal/terminalInputBridge";
import { DEFAULT_SHORTCUTS } from "@/stores/shortcutStore";

interface FakeKeyEvent {
  type: "keydown" | "keyup";
  code: string;
  key: string;
  keyCode?: number;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  altKey?: boolean;
  isComposing?: boolean;
}

function makeBridge() {
  let handler: ((e: KeyboardEvent) => boolean) | null = null;
  const term = {
    attachCustomKeyEventHandler: (fn: (e: KeyboardEvent) => boolean) => {
      handler = fn;
    },
  } as unknown as XTerminal;

  const onFilter = vi.fn();
  const onCopy = vi.fn(() => false);

  const bridge = createTerminalInputBridge({
    term,
    shortcuts: DEFAULT_SHORTCUTS,
    onFilter,
    onCopy,
  });

  const fire = (e: FakeKeyEvent): boolean => {
    if (!handler) throw new Error("handler not installed");
    const full = {
      shiftKey: false,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      isComposing: false,
      keyCode: 0,
      ...e,
    };
    return handler(full as unknown as KeyboardEvent);
  };

  const isMac = /Macintosh|Mac OS/.test(navigator.userAgent);
  const modKey: { metaKey: true } | { ctrlKey: true } = isMac ? { metaKey: true } : { ctrlKey: true };

  return { bridge, fire, onFilter, onCopy, modKey, getHandler: () => handler };
}

describe("terminalInputBridge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns true (lets xterm handle) when isComposing is true, even for a bound shortcut", () => {
    // panel.filter is Mod+F. While composing, the bridge must NOT call onFilter
    // and must let xterm process the key so the IME commit can land.
    const { fire, onFilter, modKey } = makeBridge();
    const result = fire({
      type: "keydown",
      code: "KeyF",
      key: "f",
      ...modKey,
      isComposing: true,
    });
    expect(result).toBe(true);
    expect(onFilter).not.toHaveBeenCalled();
  });

  it("returns true when keyCode === 229 (Firefox IME) regardless of isComposing flag", () => {
    const { fire, onFilter } = makeBridge();
    const result = fire({
      type: "keydown",
      code: "",
      key: "Process",
      keyCode: 229,
      isComposing: false,
    });
    expect(result).toBe(true);
    expect(onFilter).not.toHaveBeenCalled();
  });

  it("triggers onFilter and returns false on Mod+F outside composition", () => {
    const { fire, onFilter, modKey } = makeBridge();
    const result = fire({
      type: "keydown",
      code: "KeyF",
      key: "f",
      ...modKey,
      isComposing: false,
    });
    expect(result).toBe(false);
    expect(onFilter).toHaveBeenCalledTimes(1);
  });

  it("Cmd/Ctrl+C with selection: returns false (eats the event)", () => {
    const { fire, onCopy, modKey } = makeBridge();
    onCopy.mockImplementation(() => true);
    const result = fire({ type: "keydown", code: "KeyC", key: "c", ...modKey });
    expect(result).toBe(false);
    expect(onCopy).toHaveBeenCalledTimes(1);
  });

  it("Cmd/Ctrl+C without selection: returns true so xterm sends SIGINT", () => {
    const { fire, onCopy, modKey } = makeBridge();
    onCopy.mockImplementation(() => false);
    const result = fire({ type: "keydown", code: "KeyC", key: "c", ...modKey });
    expect(result).toBe(true);
    expect(onCopy).toHaveBeenCalledTimes(1);
  });

  it("setShortcuts hot-updates the binding used by the handler", () => {
    const { bridge, fire, onFilter } = makeBridge();
    // Remap panel.filter from KeyF to KeyG.
    bridge.setShortcuts({
      ...DEFAULT_SHORTCUTS,
      "panel.filter": { code: "KeyG", mod: true, shift: false, alt: false },
    });
    const isMac = /Macintosh|Mac OS/.test(navigator.userAgent);
    const mod = isMac ? { metaKey: true } : { ctrlKey: true };

    // Old binding no longer fires onFilter.
    expect(fire({ type: "keydown", code: "KeyF", key: "f", ...mod })).toBe(true);
    expect(onFilter).not.toHaveBeenCalled();

    // New binding does.
    expect(fire({ type: "keydown", code: "KeyG", key: "g", ...mod })).toBe(false);
    expect(onFilter).toHaveBeenCalledTimes(1);
  });

  it("dispose restores handler to a no-op pass-through; subsequent shortcut keys do nothing", () => {
    const { bridge, fire, onFilter, modKey } = makeBridge();
    bridge.dispose();
    const result = fire({
      type: "keydown",
      code: "KeyF",
      key: "f",
      ...modKey,
    });
    expect(result).toBe(true);
    expect(onFilter).not.toHaveBeenCalled();
  });
});
