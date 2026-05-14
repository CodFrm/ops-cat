import { Terminal as XTerminal, type ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { WriteSSH, WriteSerial } from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { bytesToBase64 } from "@/lib/terminalEncode";
import { useTerminalStore } from "@/stores/terminalStore";
import { useShortcutStore } from "@/stores/shortcutStore";
import { withTerminalFontFallback } from "@/data/terminalFonts";
import i18n from "@/i18n";
import { createTerminalInputBridge, type TerminalInputBridge } from "./terminalInputBridge";

export interface TerminalInstance {
  term: XTerminal;
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  container: HTMLDivElement;
  bridge: TerminalInputBridge;
}

interface InternalInstance extends TerminalInstance {
  isClosed: boolean;
  dispose: () => void;
}

const registry = new Map<string, InternalInstance>();

export function getOrCreateTerminal(
  sessionId: string,
  init: { fontSize: number; fontFamily: string; theme?: ITheme; scrollback: number; transport?: "ssh" | "serial" }
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

  // 优先用调用方传入的 transport；首次挂载若没拿到（罕见），退回 session id 前缀。
  const isSerial = init.transport ? init.transport === "serial" : sessionId.startsWith("serial-");
  const writeFn = isSerial ? WriteSerial : WriteSSH;
  const eventPrefix = isSerial ? "serial" : "ssh";

  // 单一 keyboard 处理入口：IME 守卫 + shortcut 拦截 + Cmd+C 选区复制。
  // 占位回调由 Terminal.tsx 在挂载时通过 setOnFilter/setOnCopy 注入。
  const bridge = createTerminalInputBridge({
    term,
    shortcuts: useShortcutStore.getState().shortcuts,
    onFilter: () => {},
    onCopy: () => false,
  });

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

  const writeData = (data: string) =>
    writeFn(sessionId, bytesToBase64(new TextEncoder().encode(data))).catch(console.error);

  const onDataDispose = term.onData(writeData);

  // 上游 bug 旁路（xterm v6.0.0，CoreBrowserTerminal._inputEvent）：
  // xterm 用全局 _keyDownSeen 给 IME composed insertText 做去重，假定一次只按一个键。
  // 百度五笔等输入法在「英文模式」下把每个按键都伪装成 keyCode=229，加上用户快速
  // 输入造成 key-rollover（前一键 keyup 之前下一键 input 已触发），xterm 误判
  // 「_keyDownSeen=true => 重复输入」把中间字符丢弃。这里精确匹配 xterm 的跳过条件，
  // 在它跳过时补一次 write。screenReaderMode 下 xterm 走另一条路径会自己发，
  // 所以同步加守卫避免双发。
  let detachRolloverPatch: () => void = () => {};
  const ta = term.textarea;
  if (ta) {
    const coreRef = (term as unknown as { _core?: { _keyDownSeen?: boolean } })._core;
    const rolloverHandler = (e: Event) => {
      const ie = e as InputEvent;
      if (
        ie.inputType === "insertText" &&
        ie.data &&
        !ie.isComposing &&
        ie.composed &&
        coreRef?._keyDownSeen === true &&
        !term.options.screenReaderMode
      ) {
        writeData(ie.data);
      }
    };
    ta.addEventListener("input", rolloverHandler, true);
    detachRolloverPatch = () => ta.removeEventListener("input", rolloverHandler, true);
  }

  const dataEvent = `${eventPrefix}:data:${sessionId}`;
  EventsOn(dataEvent, (dataB64: string) => {
    const binary = atob(dataB64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    term.write(bytes);
  });

  const closedEvent = `${eventPrefix}:closed:${sessionId}`;

  // 先声明再赋值,以便 instance.dispose 闭包可以引用 onKeyDispose
  // 而不依赖前向引用 const(可读性更好)。
  // eslint-disable-next-line prefer-const
  let onKeyDispose: { dispose: () => void };

  const instance: InternalInstance = {
    term,
    fitAddon,
    searchAddon,
    container,
    bridge,
    isClosed: false,
    dispose: () => {
      // bridge 持有 term.attachCustomKeyEventHandler 槽位的还原逻辑,
      // 必须在 term.dispose 之前调用,避免 dispose 后访问已释放对象。
      bridge.dispose();
      detachRolloverPatch();
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
