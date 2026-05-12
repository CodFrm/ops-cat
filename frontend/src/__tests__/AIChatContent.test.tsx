import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "../i18n";
import { useAIStore } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { AIChatContent } from "../components/ai/AIChatContent";

// 流式跟随滚动测试需要在 deferred markdown 异步 commit 时被通知，
// 真实环境靠 ResizeObserver；happy-dom 没实现，这里桩一个能透出 callback 的版本。
const resizeObserverCallbacks: ResizeObserverCallback[] = [];
class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(cb: ResizeObserverCallback) {
    this.callback = cb;
    resizeObserverCallbacks.push(cb);
  }
  observe() {}
  unobserve() {}
  disconnect() {
    const idx = resizeObserverCallbacks.indexOf(this.callback);
    if (idx >= 0) resizeObserverCallbacks.splice(idx, 1);
  }
}
vi.stubGlobal("ResizeObserver", MockResizeObserver);

const mockInputSpies = vi.hoisted(() => ({
  loadDraft: vi.fn(),
  clear: vi.fn(),
}));

const defaultAIActions = {
  sendToTab: useAIStore.getState().sendToTab,
  editAndResendConversation: useAIStore.getState().editAndResendConversation,
  stopGeneration: useAIStore.getState().stopGeneration,
  regenerate: useAIStore.getState().regenerate,
  regenerateConversation: useAIStore.getState().regenerateConversation,
  removeFromQueue: useAIStore.getState().removeFromQueue,
  clearQueue: useAIStore.getState().clearQueue,
};

const editButtonName = /ai\.editMessage|编辑消息|Edit message/i;
const editingBannerName = /ai\.editingMessage|正在编辑消息|Editing message/i;
const cancelEditName = /ai\.cancelEdit|取消编辑|Cancel edit/i;

vi.mock("@/components/ai/AIChatInput", () => ({
  AIChatInput: forwardRef(function MockAIChatInput(
    {
      onSubmit,
      onEmptyChange,
      onDraftChange,
    }: {
      onSubmit: (content: string) => void;
      onEmptyChange?: (empty: boolean) => void;
      onDraftChange?: (draft: { content: string }) => void;
    },
    ref
  ) {
    const [value, setValue] = useState("");

    useEffect(() => {
      onEmptyChange?.(value.trim().length === 0);
      onDraftChange?.({ content: value });
    }, [onDraftChange, onEmptyChange, value]);

    useImperativeHandle(
      ref,
      () => ({
        focus: () => {},
        clear: () => {
          mockInputSpies.clear();
          setValue("");
        },
        isEmpty: () => value.trim().length === 0,
        submit: () => onSubmit(value),
        loadDraft: (draft: string | { content: string }) => {
          mockInputSpies.loadDraft(draft);
          if (typeof draft === "string") {
            setValue(draft);
            return;
          }
          setValue(draft.content);
        },
      }),
      [onSubmit, value]
    );

    return (
      <div>
        <input aria-label="mock-ai-input" value={value} onChange={(event) => setValue(event.target.value)} />
        <button type="button" onClick={() => onSubmit(value)}>
          mock-submit
        </button>
      </div>
    );
  }),
}));

describe("AIChatContent", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN");
    localStorage.setItem("language", "zh-CN");
    mockInputSpies.loadDraft.mockReset();
    mockInputSpies.clear.mockReset();
    resizeObserverCallbacks.length = 0;

    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      configured: true,
      conversationMessages: {},
      conversationStreaming: {},
      sendToTab: defaultAIActions.sendToTab,
      editAndResendConversation: defaultAIActions.editAndResendConversation,
      stopGeneration: defaultAIActions.stopGeneration,
      regenerate: defaultAIActions.regenerate,
      regenerateConversation: defaultAIActions.regenerateConversation,
      removeFromQueue: defaultAIActions.removeFromQueue,
      clearQueue: defaultAIActions.clearQueue,
    });
  });

  it("renders messages read from conversationMessages (not tabStates)", () => {
    const tabId = "ai-5";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 5, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      conversationMessages: {
        5: [{ role: "user", content: "从 conversationMessages 读到", blocks: [] }],
      },
      conversationStreaming: {
        5: { sending: false, pendingQueue: [] },
      },
      tabStates: { [tabId]: { inputDraft: { content: "" }, scrollTop: 0, editTarget: null } },
    });

    render(<AIChatContent tabId={tabId} />);
    expect(screen.getByText("从 conversationMessages 读到")).toBeInTheDocument();
  });

  it("accepts conversationId directly without tabId and renders messages", () => {
    useAIStore.setState({
      conversationMessages: { 99: [{ role: "user", content: "直接用 convId", blocks: [] }] },
      conversationStreaming: { 99: { sending: false, pendingQueue: [] } },
    });

    render(<AIChatContent conversationId={99} />);
    expect(screen.getByText("直接用 convId")).toBeInTheDocument();
  });

  it("compact mode adds data-compact attribute for CSS hooks", () => {
    useAIStore.setState({
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    const { container } = render(<AIChatContent conversationId={1} compact />);
    expect(container.querySelector("[data-compact='true']")).toBeTruthy();
  });

  it("edit mode loads the draft and routes submit through conversation-level edit-and-resend", async () => {
    const user = userEvent.setup();
    const sendToTab = vi.fn();
    const editAndResendConversation = vi.fn().mockResolvedValue(undefined);
    // content 已经携带内联 <mention> XML，编辑链路只传 content，不再有 mentions 数组。
    const content = 'check <mention asset-id="42" type="mysql">@prod-db</mention>';
    const tabId = "ai-5";

    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 5, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: { inputDraft: { content: "" }, scrollTop: 0, editTarget: null } },
      conversationMessages: {
        5: [{ role: "user", content, blocks: [] }],
      },
      conversationStreaming: {
        5: { sending: false, pendingQueue: [] },
      },
      sendToTab,
      editAndResendConversation,
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    render(<AIChatContent tabId={tabId} />);

    await user.click(screen.getByRole("button", { name: editButtonName }));

    expect(mockInputSpies.loadDraft).toHaveBeenCalledWith({ content });
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => expect(editAndResendConversation).toHaveBeenCalledWith(5, 0, content));
    expect(sendToTab).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument());
  });

  it("canceling edit clears the prefetched draft and exits edit mode", async () => {
    const user = userEvent.setup();

    useAIStore.setState({
      conversationMessages: {
        9: [{ role: "user", content: "需要编辑", blocks: [] }],
      },
      conversationStreaming: {
        9: { sending: false, pendingQueue: [] },
      },
    });

    render(<AIChatContent conversationId={9} />);

    await user.click(screen.getByRole("button", { name: editButtonName }));
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: cancelEditName }));

    expect(mockInputSpies.clear).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument();
  });

  it("switching conversations resets edit mode to avoid state leakage", async () => {
    const user = userEvent.setup();

    useAIStore.setState({
      conversationMessages: {
        11: [{ role: "user", content: "旧会话消息", blocks: [] }],
        12: [{ role: "user", content: "新会话消息", blocks: [] }],
      },
      conversationStreaming: {
        11: { sending: false, pendingQueue: [] },
        12: { sending: false, pendingQueue: [] },
      },
    });

    const { rerender } = render(<AIChatContent conversationId={11} />);

    await user.click(screen.getByRole("button", { name: editButtonName }));
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();

    rerender(<AIChatContent conversationId={12} />);

    await waitFor(() => expect(mockInputSpies.clear).toHaveBeenCalled());
    expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument();
  });

  it("regular sends still go through onSendOverride", async () => {
    const user = userEvent.setup();
    const onSendOverride = vi.fn().mockResolvedValue(undefined);
    const editAndResendConversation = vi.fn().mockResolvedValue(undefined);

    useAIStore.setState({
      conversationMessages: { 21: [] },
      conversationStreaming: { 21: { sending: false, pendingQueue: [] } },
      editAndResendConversation,
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    render(<AIChatContent conversationId={21} onSendOverride={onSendOverride} />);

    await user.type(screen.getByRole("textbox", { name: "mock-ai-input" }), "sidebar send");
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => expect(onSendOverride).toHaveBeenCalledWith("sidebar send"));
    expect(editAndResendConversation).not.toHaveBeenCalled();
  });

  it("conversationId regenerate routes through direct mode", async () => {
    const user = userEvent.setup();
    const regenerateConversation = vi.fn().mockResolvedValue(undefined);

    useAIStore.setState({
      conversationMessages: {
        31: [{ role: "assistant", content: "ready", blocks: [] }],
      },
      conversationStreaming: {
        31: { sending: false, pendingQueue: [] },
      },
      regenerateConversation,
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    render(<AIChatContent conversationId={31} compact />);

    await user.click(screen.getByRole("button", { name: /ai\.regenerate|重新生成|Regenerate/i }));
    await user.click(await screen.findByRole("button", { name: /common\.confirm|确定|Confirm/i }));

    await waitFor(() => expect(regenerateConversation).toHaveBeenCalledWith(31, 0));
  });

  it("assistant usage badge shows uncached input rather than input plus cache", () => {
    useAIStore.setState({
      conversationMessages: {
        41: [
          {
            role: "assistant",
            content: "usage ready",
            blocks: [],
            tokenUsage: {
              inputTokens: 80,
              outputTokens: 5,
              cacheCreationTokens: 10,
              cacheReadTokens: 20,
            },
          },
        ],
      },
      conversationStreaming: {
        41: { sending: false, pendingQueue: [] },
      },
    });

    render(<AIChatContent conversationId={41} />);

    expect(screen.getByText("80")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.queryByText("110")).not.toBeInTheDocument();
  });

  it("streaming auto-follows scroll when deferred content grows scrollHeight", () => {
    // 复现 useDeferredValue 异步 commit 的场景：
    // store 已经推过新内容，messages effect 跑过一次按当时 scrollHeight 滚到底；
    // 随后 markdown 异步 commit，把 scrollHeight 撑得更高——
    // 此时既不再触发 messages effect 也不触发 scroll 事件，
    // 唯有 ResizeObserver 能让视口继续跟到新底部。
    useAIStore.setState({
      conversationMessages: {
        77: [
          { role: "user", content: "hi", blocks: [] },
          {
            role: "assistant",
            content: "first",
            blocks: [{ type: "text", content: "first" }],
            streaming: true,
          },
        ],
      },
      conversationStreaming: {
        77: { sending: true, pendingQueue: [] },
      },
    });

    const { container } = render(<AIChatContent conversationId={77} />);
    const viewport = container.querySelector<HTMLDivElement>("[data-radix-scroll-area-viewport]");
    expect(viewport).toBeTruthy();
    expect(resizeObserverCallbacks.length).toBeGreaterThan(0);

    Object.defineProperty(viewport!, "scrollHeight", { configurable: true, get: () => 1200 });
    Object.defineProperty(viewport!, "clientHeight", { configurable: true, get: () => 300 });

    act(() => {
      for (const cb of resizeObserverCallbacks) {
        cb([] as unknown as ResizeObserverEntry[], {} as ResizeObserver);
      }
    });

    expect(viewport!.scrollTop).toBe(1200);
  });
});
