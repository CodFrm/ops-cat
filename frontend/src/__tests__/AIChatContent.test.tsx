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
      onSubmit: (text: string, mentions: MentionRef[]) => void;
      onEmptyChange?: (empty: boolean) => void;
      onDraftChange?: (draft: { content: string; mentions?: MentionRef[] }) => void;
    },
    ref
  ) {
    const [value, setValue] = useState("");
    const [mentions, setMentions] = useState<MentionRef[]>([]);

    useEffect(() => {
      onEmptyChange?.(value.trim().length === 0 && mentions.length === 0);
      onDraftChange?.({ content: value, mentions });
    }, [mentions, onDraftChange, onEmptyChange, value]);

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
      tabStates: { [tabId]: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null } },
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
    const mentions: MentionRef[] = [{ assetId: 42, name: "prod-db", start: 6, end: 14 }];
    const tabId = "ai-5";

    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 5, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null } },
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

  it("exits edit mode synchronously on submit, even when content is unchanged and the resend promise has not resolved", async () => {
    const user = userEvent.setup();
    let resolveResend: (() => void) | undefined;
    const editAndResendConversation = vi.fn().mockReturnValue(
      new Promise<void>((resolve) => {
        resolveResend = () => resolve();
      })
    );
    const tabId = "ai-7";

    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 7, title: "t" } }],
      activeTabId: tabId,
    });
    useAIStore.setState({
      tabStates: { [tabId]: { inputDraft: { content: "", mentions: [] }, scrollTop: 0, editTarget: null } },
      conversationMessages: {
        7: [{ role: "user", content: "same text", blocks: [] }],
      },
      conversationStreaming: {
        7: { sending: false, pendingQueue: [] },
      },
      sendToTab: vi.fn(),
      editAndResendConversation,
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    render(<AIChatContent tabId={tabId} />);

    await user.click(screen.getByRole("button", { name: editButtonName }));
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();

    // 不修改输入框内容，直接以"同内容"提交。
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    expect(editAndResendConversation).toHaveBeenCalledWith(7, 0, "same text", undefined);
    // 不等 resend promise resolve，编辑模式就应当立即退出。
    expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument();

    resolveResend?.();
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

    await waitFor(() => expect(onSendOverride).toHaveBeenCalledWith("sidebar send", undefined));
    expect(editAndResendConversation).not.toHaveBeenCalled();
  });

  it("submitting from a fresh sidebar tab clears the saved draft so the new conversation does not re-load it", async () => {
    const user = userEvent.setup();
    const onSendOverride = vi.fn().mockResolvedValue(undefined);

    useAIStore.setState({
      sidebarTabs: [
        {
          id: "sidebar-7",
          conversationId: null,
          title: "新会话",
          createdAt: 0,
          uiState: {
            inputDraft: { content: "hello", mentions: [] },
            scrollTop: 0,
            editTarget: null,
          },
        },
      ],
      conversationMessages: { 99: [] },
      conversationStreaming: { 99: { sending: false, pendingQueue: [] } },
    } as Partial<ReturnType<typeof useAIStore.getState>>);

    const { rerender } = render(
      <AIChatContent sideTabId="sidebar-7" conversationId={null} onSendOverride={onSendOverride} compact />
    );

    // 初次挂载先把保存的 "hello" 草稿回填到编辑器。
    await waitFor(() => expect(mockInputSpies.loadDraft).toHaveBeenCalledWith({ content: "hello", mentions: [] }));
    mockInputSpies.loadDraft.mockClear();

    // 点击发送，把 "hello" 提交出去。
    await user.click(screen.getByRole("button", { name: "mock-submit" }));
    expect(onSendOverride).toHaveBeenCalledWith("hello", undefined);

    // 模拟 sendFromSidebarTab 完成新建会话后把 conversationId 绑回侧边 tab。
    useAIStore.setState((state) => ({
      sidebarTabs: state.sidebarTabs.map((tab) => (tab.id === "sidebar-7" ? { ...tab, conversationId: 99 } : tab)),
    }));
    rerender(<AIChatContent sideTabId="sidebar-7" conversationId={99} onSendOverride={onSendOverride} compact />);

    // conversationId 切换触发 loadDraft 时读到的应是空草稿，不是刚发出去的 "hello"。
    await waitFor(() => expect(mockInputSpies.loadDraft).toHaveBeenLastCalledWith({ content: "", mentions: [] }));
    expect(useAIStore.getState().sidebarTabs[0].uiState.inputDraft).toEqual({ content: "", mentions: [] });
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
});
