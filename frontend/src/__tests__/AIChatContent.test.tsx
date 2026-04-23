import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "../i18n";
import { useAIStore, type MentionRef } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { AIChatContent } from "../components/ai/AIChatContent";

const mockInputSpies = vi.hoisted(() => ({
  loadDraft: vi.fn(),
  clear: vi.fn(),
}));

const defaultAIActions = {
  sendToTab: useAIStore.getState().sendToTab,
  editAndResendConversation: useAIStore.getState().editAndResendConversation,
  stopGeneration: useAIStore.getState().stopGeneration,
  regenerate: useAIStore.getState().regenerate,
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
    }: {
      onSubmit: (text: string, mentions: MentionRef[]) => void;
      onEmptyChange?: (empty: boolean) => void;
    },
    ref
  ) {
    const [value, setValue] = useState("");
    const [mentions, setMentions] = useState<MentionRef[]>([]);

    useEffect(() => {
      onEmptyChange?.(value.trim().length === 0 && mentions.length === 0);
    }, [mentions, onEmptyChange, value]);

    useImperativeHandle(
      ref,
      () => ({
        focus: () => {},
        clear: () => {
          mockInputSpies.clear();
          setValue("");
          setMentions([]);
        },
        isEmpty: () => value.trim().length === 0 && mentions.length === 0,
        submit: () => onSubmit(value, mentions),
        loadDraft: (draft: string | { content: string; mentions?: MentionRef[] }) => {
          mockInputSpies.loadDraft(draft);
          if (typeof draft === "string") {
            setValue(draft);
            setMentions([]);
            return;
          }
          setValue(draft.content);
          setMentions(draft.mentions ?? []);
        },
      }),
      [mentions, onSubmit, value]
    );

    return (
      <div>
        <input aria-label="mock-ai-input" value={value} onChange={(event) => setValue(event.target.value)} />
        <button type="button" onClick={() => onSubmit(value, mentions)}>
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
      removeFromQueue: defaultAIActions.removeFromQueue,
      clearQueue: defaultAIActions.clearQueue,
    });
  });

  it("从 conversationMessages 渲染 tab 会话消息", () => {
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
      tabStates: { [tabId]: {} },
    });

    render(<AIChatContent tabId={tabId} />);
    expect(screen.getByText("从 conversationMessages 读到")).toBeInTheDocument();
  });

  it("支持直接传 conversationId 渲染消息", () => {
    useAIStore.setState({
      conversationMessages: { 99: [{ role: "user", content: "直接用 convId", blocks: [] }] },
      conversationStreaming: { 99: { sending: false, pendingQueue: [] } },
    });

    render(<AIChatContent conversationId={99} />);
    expect(screen.getByText("直接用 convId")).toBeInTheDocument();
  });

  it("compact 模式会暴露 data-compact 属性", () => {
    useAIStore.setState({
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
    });

    const { container } = render(<AIChatContent conversationId={1} compact />);
    expect(container.querySelector("[data-compact='true']")).toBeTruthy();
  });

  it("edit mode 会加载 draft，并改走 conversation 级 edit-and-resend", async () => {
    const user = userEvent.setup();
    const sendToTab = vi.fn();
    const editAndResendConversation = vi.fn().mockResolvedValue(undefined);
    const mentions: MentionRef[] = [{ assetId: 42, name: "prod-db", start: 6, end: 14 }];
    const tabId = "ai-5";

    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 5, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: {} },
      conversationMessages: {
        5: [{ role: "user", content: "check @prod-db", mentions, blocks: [] }],
      },
      conversationStreaming: {
        5: { sending: false, pendingQueue: [] },
      },
      sendToTab,
      editAndResendConversation,
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    render(<AIChatContent tabId={tabId} />);

    await user.click(screen.getByRole("button", { name: editButtonName }));

    expect(mockInputSpies.loadDraft).toHaveBeenCalledWith({ content: "check @prod-db", mentions });
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => expect(editAndResendConversation).toHaveBeenCalledWith(5, 0, "check @prod-db", mentions));
    expect(sendToTab).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument());
  });

  it("取消编辑会清空预填草稿并退出 edit mode", async () => {
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

  it("会话切换时会显式复位 edit mode，避免状态泄漏", async () => {
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

    await waitFor(() => expect(mockInputSpies.clear).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument();
  });

  it("普通发送仍然走 onSendOverride", async () => {
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

    await waitFor(() => expect(onSendOverride).toHaveBeenCalledWith("sidebar send", undefined));
    expect(editAndResendConversation).not.toHaveBeenCalled();
  });
});
