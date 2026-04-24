import { describe, it, expect } from "vitest";
import {
  matchShortcut,
  formatBinding,
  formatModKey,
  DEFAULT_SHORTCUTS,
  type ShortcutBinding,
} from "../stores/shortcutStore";

// In happy-dom test env, isMac = false (non-Mac).

function makeKeyboardEvent(overrides: Partial<KeyboardEvent>): KeyboardEvent {
  return {
    code: "",
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    ...overrides,
  } as KeyboardEvent;
}

describe("matchShortcut", () => {
  it("matches Ctrl+1 to tab.1 (non-Mac)", () => {
    const event = makeKeyboardEvent({ code: "Digit1", ctrlKey: true });
    const result = matchShortcut(event, DEFAULT_SHORTCUTS);
    expect(result).toBe("tab.1");
  });

  it("matches Ctrl+W to tab.close", () => {
    const event = makeKeyboardEvent({ code: "KeyW", ctrlKey: true });
    const result = matchShortcut(event, DEFAULT_SHORTCUTS);
    expect(result).toBe("tab.close");
  });

  it("matches Ctrl+Shift+[ to tab.prev", () => {
    const event = makeKeyboardEvent({ code: "BracketLeft", ctrlKey: true, shiftKey: true });
    const result = matchShortcut(event, DEFAULT_SHORTCUTS);
    expect(result).toBe("tab.prev");
  });

  it("returns null when no shortcut matches", () => {
    const event = makeKeyboardEvent({ code: "KeyZ", ctrlKey: true });
    const result = matchShortcut(event, DEFAULT_SHORTCUTS);
    expect(result).toBeNull();
  });

  it("returns null when modifier keys don't match", () => {
    const event = makeKeyboardEvent({ code: "Digit1" }); // no Ctrl
    const result = matchShortcut(event, DEFAULT_SHORTCUTS);
    expect(result).toBeNull();
  });

  it("works with custom shortcuts", () => {
    const custom = {
      ...DEFAULT_SHORTCUTS,
      "tab.close": { code: "KeyQ", mod: true, shift: false, alt: false },
    };
    const event = makeKeyboardEvent({ code: "KeyQ", ctrlKey: true });
    expect(matchShortcut(event, custom)).toBe("tab.close");
  });
});

describe("formatBinding", () => {
  // In happy-dom, isMac = false, so we get Windows-style formatting

  it("formats Ctrl+key binding", () => {
    const binding: ShortcutBinding = { code: "KeyW", mod: true, shift: false, alt: false };
    expect(formatBinding(binding)).toBe("Ctrl+W");
  });

  it("formats Ctrl+Shift+key binding", () => {
    const binding: ShortcutBinding = { code: "BracketLeft", mod: true, shift: true, alt: false };
    expect(formatBinding(binding)).toBe("Ctrl+Shift+[");
  });

  it("formats Ctrl+Alt+key binding", () => {
    const binding: ShortcutBinding = { code: "KeyD", mod: true, shift: false, alt: true };
    expect(formatBinding(binding)).toBe("Ctrl+Alt+D");
  });

  it("formats digit keys correctly", () => {
    const binding: ShortcutBinding = { code: "Digit1", mod: true, shift: false, alt: false };
    expect(formatBinding(binding)).toBe("Ctrl+1");
  });

  it("formats special keys (Comma, Space, etc.)", () => {
    const binding: ShortcutBinding = { code: "Comma", mod: true, shift: false, alt: false };
    expect(formatBinding(binding)).toBe("Ctrl+,");
  });

  it("formats key without modifiers", () => {
    const binding: ShortcutBinding = { code: "F5", mod: false, shift: false, alt: false };
    expect(formatBinding(binding)).toBe("F5");
  });
});

describe("formatModKey", () => {
  // happy-dom env → isMac = false → Windows-style output

  it("formats letter keys", () => {
    expect(formatModKey("KeyC")).toBe("Ctrl+C");
    expect(formatModKey("KeyR")).toBe("Ctrl+R");
  });

  it("formats Enter", () => {
    expect(formatModKey("Enter")).toBe("Ctrl+Enter");
  });

  it("applies shift modifier", () => {
    expect(formatModKey("KeyF", { shift: true })).toBe("Ctrl+Shift+F");
  });

  it("applies alt modifier", () => {
    expect(formatModKey("KeyD", { alt: true })).toBe("Ctrl+Alt+D");
  });
});
