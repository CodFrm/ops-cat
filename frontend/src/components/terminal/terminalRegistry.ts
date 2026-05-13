import { Terminal as XTerminal, type ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import "@xterm/xterm/css/xterm.css";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { useTerminalStore } from "@/stores/terminalStore";
import { withTerminalFontFallback } from "@/data/terminalFonts";
import i18n from "@/i18n";
import { TerminalInputWriter } from "./terminalInputWriter";

export interface TerminalInstance {
  term: XTerminal;
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  container: HTMLDivElement;
  writeInput: (data: string) => void;
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
  const inputWriter = new TerminalInputWriter(sessionId);
  term.loadAddon(fitAddon);
  term.loadAddon(searchAddon);
  term.open(container);

  const onDataDispose = term.onData((data) => {
    inputWriter.write(data);
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
    writeInput: (data: string) => inputWriter.write(data),
    isClosed: false,
    dispose: () => {
      onDataDispose.dispose();
      onKeyDispose.dispose();
      inputWriter.dispose();
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

export function writeTerminalInput(sessionId: string, data: string): void {
  registry.get(sessionId)?.writeInput(data);
}
