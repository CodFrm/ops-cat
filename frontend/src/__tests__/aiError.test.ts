import { describe, it, expect, vi } from "vitest";

// classifyError 内部读取 i18n.t 取友好标题；测试只关心 kind 分类，message 走 fallback。
vi.mock("../i18n", () => ({
  default: { t: (_key: string, fallback: string) => fallback },
}));

import { classifyError } from "../lib/aiError";

describe("classifyError", () => {
  it("命中 auth（401/403/api key/unauthorized）", () => {
    expect(classifyError("401 unauthorized").kind).toBe("auth");
    expect(classifyError("403 forbidden").kind).toBe("auth");
    expect(classifyError("invalid_api_key").kind).toBe("auth");
    expect(classifyError("authentication failed").kind).toBe("auth");
  });

  it("命中 rate_limit（429/too many/rate limit）", () => {
    expect(classifyError("429 too many requests").kind).toBe("rate_limit");
    expect(classifyError("rate_limit_exceeded").kind).toBe("rate_limit");
  });

  it("命中 network（timeout/EOF/connection reset/i/o timeout/tls handshake）", () => {
    expect(classifyError("dial tcp: i/o timeout").kind).toBe("network");
    expect(classifyError("read: connection reset by peer").kind).toBe("network");
    expect(classifyError("EOF").kind).toBe("network");
    expect(classifyError("tls handshake timeout").kind).toBe("network");
  });

  it("命中 server（5xx/service unavailable/bad gateway）", () => {
    expect(classifyError("503 service unavailable").kind).toBe("server");
    expect(classifyError("502 bad gateway").kind).toBe("server");
    expect(classifyError("500 internal server error").kind).toBe("server");
  });

  it("auth 优先于 server（401 不被 5xx 误捞）", () => {
    expect(classifyError("provider returned 401: api key unauthorized").kind).toBe("auth");
  });

  it("rate_limit 优先于 server（429 不被 5xx 误捞）", () => {
    expect(classifyError("429 rate limit exceeded").kind).toBe("rate_limit");
  });

  it("空字符串与未知错误归类为 unknown", () => {
    expect(classifyError("").kind).toBe("unknown");
    expect(classifyError(null).kind).toBe("unknown");
    expect(classifyError("some weird message").kind).toBe("unknown");
  });

  it("显式 kind 跳过正则匹配（用于 interrupted 路径）", () => {
    expect(classifyError("401 unauthorized", "interrupted").kind).toBe("interrupted");
    expect(classifyError("429 too many", "interrupted").kind).toBe("interrupted");
  });

  it("message 字段在所有 kind 上非空", () => {
    const kinds = ["rate_limit", "server", "network", "auth", "interrupted", "unknown"] as const;
    for (const k of kinds) {
      const { message } = classifyError("any", k);
      expect(message).toBeTruthy();
    }
  });
});
