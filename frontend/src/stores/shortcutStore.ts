import { create } from "zustand";

export const isMac = /Macintosh|Mac OS/.test(navigator.userAgent);

export interface ShortcutBinding {
  code: string; // KeyboardEvent.code, e.g. "Digit1", "KeyW", "BracketLeft"
  mod: boolean; // Cmd (Mac) / Ctrl (Win)
  shift: boolean;
  alt: boolean;
}

export type ShortcutAction =
  | "tab.1"
  | "tab.2"
  | "tab.3"
  | "tab.4"
  | "tab.5"
  | "tab.6"
  | "tab.7"
  | "tab.8"
  | "tab.9"
  | "tab.close"
  | "tab.prev"
  | "tab.next"
  | "split.vertical"
  | "split.horizontal"
  | "panel.ai"
  | "panel.sidebar"
  | "panel.switch"
  | "panel.filter"
  | "page.home"
  | "page.settings"
  | "page.sshkeys";

export const SHORTCUT_ACTIONS: ShortcutAction[] = [
  "tab.1",
  "tab.2",
  "tab.3",
  "tab.4",
  "tab.5",
  "tab.6",
  "tab.7",
  "tab.8",
  "tab.9",
  "tab.close",
  "tab.prev",
  "tab.next",
  "split.vertical",
  "split.horizontal",
  "panel.ai",
  "panel.sidebar",
  "panel.switch",
  "panel.filter",
  "page.home",
  "page.settings",
  "page.sshkeys",
];

export const DEFAULT_SHORTCUTS: Record<ShortcutAction, ShortcutBinding> = {
  "tab.1": { code: "Digit1", mod: true, shift: false, alt: false },
  "tab.2": { code: "Digit2", mod: true, shift: false, alt: false },
  "tab.3": { code: "Digit3", mod: true, shift: false, alt: false },
  "tab.4": { code: "Digit4", mod: true, shift: false, alt: false },
  "tab.5": { code: "Digit5", mod: true, shift: false, alt: false },
  "tab.6": { code: "Digit6", mod: true, shift: false, alt: false },
  "tab.7": { code: "Digit7", mod: true, shift: false, alt: false },
  "tab.8": { code: "Digit8", mod: true, shift: false, alt: false },
  "tab.9": { code: "Digit9", mod: true, shift: false, alt: false },
  "tab.close": { code: "KeyW", mod: true, shift: false, alt: false },
  "tab.prev": { code: "BracketLeft", mod: true, shift: true, alt: false },
  "tab.next": { code: "BracketRight", mod: true, shift: true, alt: false },
  "split.vertical": { code: "KeyD", mod: true, shift: false, alt: false },
  "split.horizontal": { code: "KeyD", mod: true, shift: true, alt: false },
  "panel.ai": { code: "KeyB", mod: true, shift: false, alt: false },
  "panel.sidebar": { code: "KeyE", mod: true, shift: false, alt: false },
  "panel.switch": { code: "KeyE", mod: true, shift: true, alt: false },
  "panel.filter": { code: "KeyF", mod: true, shift: false, alt: false },
  "page.home": { code: "KeyH", mod: true, shift: true, alt: false },
  "page.settings": { code: "Comma", mod: true, shift: false, alt: false },
  "page.sshkeys": { code: "KeyK", mod: true, shift: true, alt: false },
};

const STORAGE_KEY = "keyboard_shortcuts";

function loadCustom(): Partial<Record<ShortcutAction, ShortcutBinding>> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function saveCustom(shortcuts: Record<ShortcutAction, ShortcutBinding>) {
  const custom: Partial<Record<ShortcutAction, ShortcutBinding>> = {};
  for (const [key, val] of Object.entries(shortcuts)) {
    const def = DEFAULT_SHORTCUTS[key as ShortcutAction];
    if (def && (val.code !== def.code || val.mod !== def.mod || val.shift !== def.shift || val.alt !== def.alt)) {
      custom[key as ShortcutAction] = val;
    }
  }
  if (Object.keys(custom).length > 0) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(custom));
  } else {
    localStorage.removeItem(STORAGE_KEY);
  }
}

interface ShortcutState {
  shortcuts: Record<ShortcutAction, ShortcutBinding>;
  isRecording: boolean;
  setIsRecording: (v: boolean) => void;
  updateShortcut: (action: ShortcutAction, binding: ShortcutBinding) => void;
  resetShortcut: (action: ShortcutAction) => void;
  resetAll: () => void;
}

export const useShortcutStore = create<ShortcutState>((set) => ({
  shortcuts: { ...DEFAULT_SHORTCUTS, ...loadCustom() },
  isRecording: false,

  setIsRecording: (v) => set({ isRecording: v }),

  updateShortcut: (action, binding) => {
    set((state) => {
      const shortcuts = { ...state.shortcuts, [action]: binding };
      saveCustom(shortcuts);
      return { shortcuts };
    });
  },

  resetShortcut: (action) => {
    set((state) => {
      const shortcuts = {
        ...state.shortcuts,
        [action]: DEFAULT_SHORTCUTS[action],
      };
      saveCustom(shortcuts);
      return { shortcuts };
    });
  },

  resetAll: () => {
    localStorage.removeItem(STORAGE_KEY);
    set({ shortcuts: { ...DEFAULT_SHORTCUTS } });
  },
}));

// Match a KeyboardEvent against shortcuts
export function matchShortcut(
  e: KeyboardEvent,
  shortcuts: Record<ShortcutAction, ShortcutBinding>
): ShortcutAction | null {
  const modPressed = isMac ? e.metaKey : e.ctrlKey;
  for (const [action, binding] of Object.entries(shortcuts)) {
    if (
      e.code === binding.code &&
      modPressed === binding.mod &&
      e.shiftKey === binding.shift &&
      e.altKey === binding.alt
    ) {
      return action as ShortcutAction;
    }
  }
  return null;
}

// Convert KeyboardEvent.code to display string
const CODE_DISPLAY: Record<string, string> = {
  Digit0: "0",
  Digit1: "1",
  Digit2: "2",
  Digit3: "3",
  Digit4: "4",
  Digit5: "5",
  Digit6: "6",
  Digit7: "7",
  Digit8: "8",
  Digit9: "9",
  BracketLeft: "[",
  BracketRight: "]",
  Minus: "-",
  Equal: "=",
  Backquote: "`",
  Comma: ",",
  Period: ".",
  Slash: "/",
  Backslash: "\\",
  Semicolon: ";",
  Quote: "'",
  Space: "Space",
  Enter: "Enter",
  Escape: "Esc",
  Backspace: "⌫",
  Tab: "Tab",
  Delete: "Del",
  ArrowUp: "↑",
  ArrowDown: "↓",
  ArrowLeft: "←",
  ArrowRight: "→",
};

function codeToDisplay(code: string): string {
  if (CODE_DISPLAY[code]) return CODE_DISPLAY[code];
  if (code.startsWith("Key")) return code.slice(3);
  return code;
}

export function formatBinding(binding: ShortcutBinding): string {
  const parts: string[] = [];
  if (binding.mod) parts.push(isMac ? "⌘" : "Ctrl");
  if (binding.shift) parts.push(isMac ? "⇧" : "Shift");
  if (binding.alt) parts.push(isMac ? "⌥" : "Alt");
  parts.push(codeToDisplay(binding.code));
  return isMac ? parts.join("") : parts.join("+");
}

// Platform-convention modifier shortcut (Cmd on Mac, Ctrl elsewhere) — for
// system-level bindings NOT in the shortcut registry (browser/xterm copy-paste,
// editor shortcuts, etc.). User-configurable shortcuts must use
// formatBinding(shortcuts[action]) so the UI follows rebindings.
export function formatModKey(code: string, options: { shift?: boolean; alt?: boolean } = {}): string {
  return formatBinding({ code, mod: true, shift: options.shift ?? false, alt: options.alt ?? false });
}
