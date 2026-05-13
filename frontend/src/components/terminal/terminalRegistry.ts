import { Terminal as XTerminal, type ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { WriteSSH } from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { bytesToBase64 } from "@/lib/terminalEncode";
import { useTerminalStore } from "@/stores/terminalStore";
import { withTerminalFontFallback } from "@/data/terminalFonts";
import i18n from "@/i18n";

export interface TerminalInstance {
  term: XTerminal;
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  container: HTMLDivElement;
}

interface InternalInstance extends TerminalInstance {
  isClosed: boolean;
  dispose: () => void;
}

const registry = new Map<string, InternalInstance>();

export function getOrCreateTerminal(
  sessionId: string,
  init: { fontSize: number; fontFamily: string; theme?: ITheme; scrollback: number }
): TerminalInstance {
  const cached = registry.get(sessionId);
  if (cached) return cached;

  const container = document.createElement("div");
  container.style.height = "100%";
  container.style.width = "100%";

  const term = new XTerminal({
    cursorBlink: true,
    fontSize: init.fontSize,
    fontFamily: withTerminalFontFallback(init.fontFamily),
    theme: init.theme,
    scrollback: init.scrollback,
  });

  const fitAddon = new FitAddon();
  const searchAddon = new SearchAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(searchAddon);
  term.open(container);

  // GPU renderer: required so customGlyphs (powerline U+E0A0–U+E0D7, box drawing)
  // is drawn by xterm instead of the system font — fixes tofu boxes from terminal
  // prompts (oh-my-zsh powerlevel10k, starship, etc.). Falls back to DOM renderer
  // automatically on context loss or if WebGL initialization throws.
  try {
    const webglAddon = new WebglAddon();
    webglAddon.onContextLoss(() => {
      webglAddon.dispose();
    });
    term.loadAddon(webglAddon);
  } catch (err) {
    console.warn("WebGL renderer unavailable, falling back to DOM renderer", err);
  }

  const onDataDispose = term.onData((data) => {
    WriteSSH(sessionId, bytesToBase64(new TextEncoder().encode(data))).catch(console.error);
  });

  const dataEvent = "ssh:data:" + sessionId;
  EventsOn(dataEvent, (dataB64: string) => {
    const binary = atob(dataB64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    term.write(bytes);
  });

  const closedEvent = "ssh:closed:" + sessionId;

  // 先声明再赋值,以便 instance.dispose 闭包可以引用 onKeyDispose
  // 而不依赖前向引用 const(可读性更好)。
  // eslint-disable-next-line prefer-const
  let onKeyDispose: { dispose: () => void };

  const instance: InternalInstance = {
    term,
    fitAddon,
    searchAddon,
    container,
    isClosed: false,
    dispose: () => {
      onDataDispose.dispose();
      onKeyDispose.dispose();
      EventsOff(dataEvent);
      EventsOff(closedEvent);
      term.dispose();
      registry.delete(sessionId);
    },
  };

  onKeyDispose = term.onKey(({ key }) => {
    if (instance.isClosed && key === "\r") {
      instance.isClosed = false;
      useTerminalStore.getState().reconnectBySession(sessionId);
    }
  });

  EventsOn(closedEvent, () => {
    const hint = i18n.t("ssh.session.closedHint");
    term.write(`\r\n\x1b[31m${hint}\x1b[0m\r\n`);
    useTerminalStore.getState().markClosed(sessionId);
    instance.isClosed = true;
  });

  registry.set(sessionId, instance);
  return instance;
}

export function disposeTerminal(sessionId: string): void {
  const inst = registry.get(sessionId);
  if (inst) inst.dispose();
}

export function getTerminalInstance(sessionId: string): TerminalInstance | undefined {
  return registry.get(sessionId);
}
