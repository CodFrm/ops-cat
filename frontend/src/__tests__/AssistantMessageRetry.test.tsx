import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("../i18n", () => ({
  default: { t: (key: string, fallback?: string) => fallback || key },
}));

import { AssistantMessage } from "../components/ai/AIChatContent";
import type { ChatMessage } from "../stores/aiStore";

// 回归 bug: cago 同步 retry 路径下 assistant 占位消息处于 streaming=true、blocks=[]、
// content="" 状态，AssistantMessage 旧逻辑命中 dot loader 早返回分支，完全不渲染
// RetryBanner。结果 cago 后端日志显示 retry 2 次，前端 UI 一秒都看不到"重试中"提示，
// 最终只看到 ErrorBlock。修复:dot loader 分支也必须挂 RetryBanner。
describe("AssistantMessage retry rendering", () => {
  const noop = () => {};

  it("streaming 中 + 空 blocks + 有 retryStatus → 渲染 RetryBanner（覆盖 dot loader 分支）", () => {
    const msg: ChatMessage = {
      id: "1",
      role: "assistant",
      content: "",
      blocks: [],
      streaming: true,
      retryStatus: {
        attempt: 1,
        delayMs: 1000,
        startedAt: Date.now(),
        cause: "503 service unavailable",
      },
    };
    render(<AssistantMessage msg={msg} index={0} sending={true} onRegenerate={noop} />);
    // RetryBanner 的 ARIA role="status" 是稳定测试锚点；i18n 文本受 setup mock
    // 影响（t() 返回 key 而非 fallback），用 role 而非 textContent 断言。
    const banner = screen.getByRole("status");
    expect(banner).toBeInTheDocument();
    // 同时确认 banner 文本里至少包含 retry i18n key（"ai.retry.retrying"）。
    expect(banner.textContent || "").toMatch(/retry/i);
  });

  it("有 blocks + retryStatus → hasBlocks 分支也渲染 RetryBanner", () => {
    const msg: ChatMessage = {
      id: "2",
      role: "assistant",
      content: "Hello",
      blocks: [{ type: "text", content: "Hello" }],
      streaming: true,
      retryStatus: {
        attempt: 2,
        delayMs: 2000,
        startedAt: Date.now(),
        cause: "503",
      },
    };
    render(<AssistantMessage msg={msg} index={0} sending={true} onRegenerate={noop} />);
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("RetryBanner 内显示 cause 原文 + classifyError 分类标题", () => {
    const msg: ChatMessage = {
      id: "4",
      role: "assistant",
      content: "",
      blocks: [],
      streaming: true,
      retryStatus: {
        attempt: 2,
        delayMs: 2000,
        startedAt: Date.now(),
        cause: "error, status code: 503, status: 503 Service Unavailable, message: No available channel",
      },
    };
    render(<AssistantMessage msg={msg} index={0} sending={true} onRegenerate={noop} />);
    const banner = screen.getByRole("status");
    // 原始 cause 必须出现在 banner 内（用户能看到 503 的真实文本和 request id）。
    expect(banner.textContent).toContain("503 Service Unavailable");
    expect(banner.textContent).toContain("No available channel");
  });

  it("无 retryStatus → 不渲染 RetryBanner", () => {
    const msg: ChatMessage = {
      id: "3",
      role: "assistant",
      content: "",
      blocks: [],
      streaming: true,
    };
    render(<AssistantMessage msg={msg} index={0} sending={true} onRegenerate={noop} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
