import { describe, it, expect, vi, beforeEach } from "vitest";

const hoisted = vi.hoisted(() => {
  const eventHandlers = new Map<string, (...args: unknown[]) => void>();
  const writeSpy = vi.fn();
  const disposeSpy = vi.fn();
  const reconnectBySessionMock = vi.fn();
  const terminalCtor = vi.fn();
  const bridgeDisposeSpy = vi.fn();
  const webglAddonCtor = vi.fn();
  const webglAddonDisposeSpy = vi.fn();
  const webglContextLossDisposeSpy = vi.fn();
  const setWebglEnabledSpy = vi.fn();
  const disposeOrder: string[] = [];
  const state: { capturedOnKey: ((e: { key: string }) => void) | null } = {
    capturedOnKey: null,
  };
  return {
    eventHandlers,
    writeSpy,
    disposeSpy,
    reconnectBySessionMock,
    terminalCtor,
    bridgeDisposeSpy,
    webglAddonCtor,
    webglAddonDisposeSpy,
    webglContextLossDisposeSpy,
    setWebglEnabledSpy,
    disposeOrder,
    state,
  };
});

vi.mock("../../wailsjs/runtime/runtime", () => ({
  EventsOn: (event: string, handler: (...args: unknown[]) => void) => {
    hoisted.eventHandlers.set(event, handler);
  },
  EventsOff: (event: string) => {
    hoisted.eventHandlers.delete(event);
  },
}));

vi.mock("../../wailsjs/go/app/App", () => ({
  WriteSSH: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@xterm/xterm", () => {
  class MockTerminal {
    loadAddon = vi.fn();
    open = vi.fn();
    write = hoisted.writeSpy;
    onData = vi.fn(() => ({ dispose: vi.fn() }));
    onKey = vi.fn((handler: (e: { key: string }) => void) => {
      hoisted.state.capturedOnKey = handler;
      return { dispose: vi.fn() };
    });
    attachCustomKeyEventHandler = vi.fn();
    dispose = vi.fn(() => {
      hoisted.disposeOrder.push("term");
      hoisted.disposeSpy();
    });
    constructor() {
      hoisted.terminalCtor();
    }
  }
  return { Terminal: MockTerminal };
});

vi.mock("@/components/terminal/terminalInputBridge", () => ({
  createTerminalInputBridge: vi.fn(() => ({
    setShortcuts: vi.fn(),
    setOnFilter: vi.fn(),
    setOnCopy: vi.fn(),
    dispose: vi.fn(() => {
      hoisted.disposeOrder.push("bridge");
      hoisted.bridgeDisposeSpy();
    }),
  })),
}));

vi.mock("@xterm/addon-fit", () => ({ FitAddon: class {} }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: class {} }));
vi.mock("@xterm/addon-webgl", () => {
  class MockWebglAddon {
    constructor() {
      hoisted.webglAddonCtor();
    }
    onContextLoss = vi.fn(() => ({
      dispose: vi.fn(() => {
        hoisted.webglContextLossDisposeSpy();
      }),
    }));
    dispose = vi.fn(() => {
      hoisted.disposeOrder.push("webgl");
      hoisted.webglAddonDisposeSpy();
    });
  }
  return { WebglAddon: MockWebglAddon };
});
vi.mock("@xterm/xterm/css/xterm.css", () => ({}));

vi.mock("@/stores/terminalStore", () => ({
  useTerminalStore: {
    getState: () => ({
      markClosed: vi.fn(),
      reconnectBySession: hoisted.reconnectBySessionMock,
    }),
  },
}));

vi.mock("@/stores/terminalThemeStore", () => ({
  useTerminalThemeStore: {
    getState: () => ({
      setWebglEnabled: hoisted.setWebglEnabledSpy,
    }),
  },
}));

vi.mock("@/data/terminalFonts", () => ({ withTerminalFontFallback: (s: string) => s }));
vi.mock("@/lib/terminalEncode", () => ({ bytesToBase64: () => "" }));

vi.mock("@/i18n", () => ({
  default: { t: (key: string) => `<<${key}>>` },
}));

import { getOrCreateTerminal, disposeTerminal } from "@/components/terminal/terminalRegistry";

describe("terminalRegistry", () => {
  beforeEach(() => {
    hoisted.eventHandlers.clear();
    hoisted.state.capturedOnKey = null;
    hoisted.writeSpy.mockClear();
    hoisted.disposeSpy.mockClear();
    hoisted.reconnectBySessionMock.mockClear();
    hoisted.terminalCtor.mockClear();
    hoisted.bridgeDisposeSpy.mockClear();
    hoisted.webglAddonCtor.mockClear();
    hoisted.webglAddonDisposeSpy.mockClear();
    hoisted.webglContextLossDisposeSpy.mockClear();
    hoisted.setWebglEnabledSpy.mockClear();
    hoisted.disposeOrder.length = 0;
  });

  it("writes the i18n closed hint and marks closed when ssh:closed fires", () => {
    getOrCreateTerminal("sess-1", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    const handler = hoisted.eventHandlers.get("ssh:closed:sess-1");
    expect(handler).toBeDefined();
    handler?.();
    const written = hoisted.writeSpy.mock.calls.map((c) => c[0]).join("");
    expect(written).toContain("<<ssh.session.closedHint>>");
    disposeTerminal("sess-1");
  });

  it("triggers reconnectBySession on Enter after close, and re-arms on the next close", () => {
    getOrCreateTerminal("sess-2", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    hoisted.eventHandlers.get("ssh:closed:sess-2")?.();
    expect(hoisted.state.capturedOnKey).toBeTruthy();

    hoisted.state.capturedOnKey?.({ key: "\r" });
    expect(hoisted.reconnectBySessionMock).toHaveBeenCalledWith("sess-2");
    expect(hoisted.reconnectBySessionMock).toHaveBeenCalledTimes(1);

    // 第二次 Enter 在同一次 closed 内不应再触发
    hoisted.state.capturedOnKey?.({ key: "\r" });
    expect(hoisted.reconnectBySessionMock).toHaveBeenCalledTimes(1);

    // 重新 closed 后,Enter 应当再次触发
    hoisted.eventHandlers.get("ssh:closed:sess-2")?.();
    hoisted.state.capturedOnKey?.({ key: "\r" });
    expect(hoisted.reconnectBySessionMock).toHaveBeenCalledTimes(2);

    disposeTerminal("sess-2");
  });

  it("ignores non-Enter keys after close", () => {
    getOrCreateTerminal("sess-3", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    hoisted.eventHandlers.get("ssh:closed:sess-3")?.();
    hoisted.state.capturedOnKey?.({ key: "a" });
    hoisted.state.capturedOnKey?.({ key: "\n" });
    expect(hoisted.reconnectBySessionMock).not.toHaveBeenCalled();
    disposeTerminal("sess-3");
  });

  it("does not trigger reconnect when not closed", () => {
    getOrCreateTerminal("sess-4", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    hoisted.state.capturedOnKey?.({ key: "\r" });
    expect(hoisted.reconnectBySessionMock).not.toHaveBeenCalled();
    disposeTerminal("sess-4");
  });

  it("disposes the input bridge before the xterm instance", () => {
    getOrCreateTerminal("sess-order", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    disposeTerminal("sess-order");
    expect(hoisted.bridgeDisposeSpy).toHaveBeenCalled();
    expect(hoisted.disposeSpy).toHaveBeenCalled();
    expect(hoisted.disposeOrder).toEqual(["bridge", "webgl", "term"]);
  });

  // 上游 term.dispose() 虽然会级联释放已加载 addon，但 onContextLoss 返回的
  // 订阅是独立 IDisposable，不显式 dispose 会留下事件监听器引用 → 资源泄露。
  it("disposes the WebGL addon and its onContextLoss subscription before xterm dispose", () => {
    getOrCreateTerminal("sess-webgl", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    expect(hoisted.webglAddonCtor).toHaveBeenCalledTimes(1);
    disposeTerminal("sess-webgl");
    expect(hoisted.webglAddonDisposeSpy).toHaveBeenCalledTimes(1);
    expect(hoisted.webglContextLossDisposeSpy).toHaveBeenCalledTimes(1);
    expect(hoisted.disposeOrder.indexOf("webgl")).toBeLessThan(hoisted.disposeOrder.indexOf("term"));
  });

  it("skips WebGL when webglEnabled is false", () => {
    getOrCreateTerminal("sess-no-webgl", {
      fontSize: 14,
      fontFamily: "mono",
      scrollback: 1000,
      webglEnabled: false,
    });
    expect(hoisted.webglAddonCtor).not.toHaveBeenCalled();
    disposeTerminal("sess-no-webgl");
    expect(hoisted.webglAddonDisposeSpy).not.toHaveBeenCalled();
  });

  it("re-creates a fresh terminal after dispose for the same sessionId", () => {
    const before = hoisted.terminalCtor.mock.calls.length;
    getOrCreateTerminal("sess-5", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    disposeTerminal("sess-5");
    expect(hoisted.disposeSpy).toHaveBeenCalled();
    getOrCreateTerminal("sess-5", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    expect(hoisted.terminalCtor.mock.calls.length).toBe(before + 2);
    disposeTerminal("sess-5");
  });
});
