import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TerminalInputWriter } from "@/components/terminal/terminalInputWriter";

function decodeBytes(bytes: Uint8Array): string {
  return new TextDecoder().decode(bytes);
}

function deferred() {
  let resolve!: () => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flushPromises() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("TerminalInputWriter", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("batches printable input before writing to SSH", () => {
    const writeSSH = vi.fn().mockResolvedValue(undefined);
    const writer = new TerminalInputWriter("sess-1", {
      writeSSH,
      encodeBytes: decodeBytes,
      flushDelayMs: 8,
    });

    writer.write("c");
    writer.write("a");
    writer.write("t");

    vi.advanceTimersByTime(7);
    expect(writeSSH).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(writeSSH).toHaveBeenCalledTimes(1);
    expect(writeSSH).toHaveBeenCalledWith("sess-1", "cat");
  });

  it("serializes writes while an earlier WriteSSH call is still in flight", async () => {
    const first = deferred();
    const second = deferred();
    const writeSSH = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const writer = new TerminalInputWriter("sess-2", {
      writeSSH,
      encodeBytes: decodeBytes,
      flushDelayMs: 8,
    });

    writer.write("c");
    vi.advanceTimersByTime(8);
    expect(writeSSH).toHaveBeenCalledTimes(1);
    expect(writeSSH).toHaveBeenNthCalledWith(1, "sess-2", "c");

    writer.write("at");
    vi.advanceTimersByTime(8);
    expect(writeSSH).toHaveBeenCalledTimes(1);

    first.resolve();
    await flushPromises();
    vi.advanceTimersByTime(8);

    expect(writeSSH).toHaveBeenCalledTimes(2);
    expect(writeSSH).toHaveBeenNthCalledWith(2, "sess-2", "at");
    second.resolve();
  });

  it("flushes pending printable input immediately when control input arrives", () => {
    const writeSSH = vi.fn().mockResolvedValue(undefined);
    const writer = new TerminalInputWriter("sess-3", {
      writeSSH,
      encodeBytes: decodeBytes,
      flushDelayMs: 8,
    });

    writer.write("ls");
    writer.write("\r");

    expect(writeSSH).toHaveBeenCalledTimes(1);
    expect(writeSSH).toHaveBeenCalledWith("sess-3", "ls\r");
  });

  it("drops pending input after dispose", () => {
    const writeSSH = vi.fn().mockResolvedValue(undefined);
    const writer = new TerminalInputWriter("sess-4", {
      writeSSH,
      encodeBytes: decodeBytes,
      flushDelayMs: 8,
    });

    writer.write("cat");
    writer.dispose();
    writer.write(" clear");
    vi.advanceTimersByTime(8);

    expect(writeSSH).not.toHaveBeenCalled();
  });
});
