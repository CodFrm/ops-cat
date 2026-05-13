import { WriteSSH } from "../../../wailsjs/go/app/App";
import { bytesToBase64 } from "@/lib/terminalEncode";

type WriteSSHFn = (sessionId: string, dataB64: string) => Promise<void>;

interface TerminalInputWriterOptions {
  writeSSH?: WriteSSHFn;
  encodeBytes?: (bytes: Uint8Array) => string;
  flushDelayMs?: number;
}

const defaultFlushDelayMs = 8;
const encoder = new TextEncoder();

function hasControlInput(data: string): boolean {
  for (let i = 0; i < data.length; i++) {
    const code = data.charCodeAt(i);
    if (code <= 0x1f || code === 0x7f) return true;
  }
  return false;
}

export class TerminalInputWriter {
  private pending = "";
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private writing = false;
  private disposed = false;
  private readonly writeSSH: WriteSSHFn;
  private readonly encodeBytes: (bytes: Uint8Array) => string;
  private readonly flushDelayMs: number;

  constructor(
    private readonly sessionId: string,
    options: TerminalInputWriterOptions = {}
  ) {
    this.writeSSH = options.writeSSH ?? WriteSSH;
    this.encodeBytes = options.encodeBytes ?? bytesToBase64;
    this.flushDelayMs = options.flushDelayMs ?? defaultFlushDelayMs;
  }

  write(data: string): void {
    if (this.disposed || !data) return;

    this.pending += data;
    if (hasControlInput(data)) {
      this.clearTimer();
      this.flush();
      return;
    }

    if (this.flushTimer === null) {
      this.flushTimer = setTimeout(() => {
        this.flushTimer = null;
        this.flush();
      }, this.flushDelayMs);
    }
  }

  dispose(): void {
    this.disposed = true;
    this.pending = "";
    this.clearTimer();
  }

  private clearTimer(): void {
    if (this.flushTimer !== null) {
      clearTimeout(this.flushTimer);
      this.flushTimer = null;
    }
  }

  private flush(): void {
    if (this.disposed || this.writing || !this.pending) return;

    const data = this.pending;
    this.pending = "";
    this.writing = true;

    this.writeSSH(this.sessionId, this.encodeBytes(encoder.encode(data)))
      .catch(console.error)
      .finally(() => {
        this.writing = false;
        if (this.disposed || !this.pending) return;
        if (hasControlInput(this.pending)) {
          this.flush();
          return;
        }
        if (this.flushTimer === null) {
          this.flushTimer = setTimeout(() => {
            this.flushTimer = null;
            this.flush();
          }, this.flushDelayMs);
        }
      });
  }
}
