import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AIChatContent } from "@/components/ai/AIChatContent";
import { useAIStore, type ChatMessage, type MentionRef } from "@/stores/aiStore";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";
import { SendAIMessage, StopAIGeneration, SaveConversationMessages } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

const mockInputSpies = vi.hoisted(() => ({
  clear: vi.fn(),
  loadDraft: vi.fn(),
}));

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
        <div>
          {mentions.map((mention) => (
            <span key={`${mention.assetId}-${mention.start}-${mention.end}`}>{`mention:${mention.name}`}</span>
          ))}
        </div>
        <button type="button" onClick={() => onSubmit(value, mentions)}>
          mock-submit
        </button>
      </div>
    );
  }),
}));

const editButtonName = /ai\.editMessage|编辑消息|Edit message/i;
const editingBannerName = /ai\.editingMessage|正在编辑消息|Editing message/i;
const pendingQueueName = /ai\.pendingMessages|等待发送|Pending/i;

function buildMessage(role: ChatMessage["role"], content: string, options?: Partial<ChatMessage>): ChatMessage {
  return {
    role,
    content,
    blocks: [],
    ...options,
  };
}

function setupConversationFixture({
  conversationId,
  messages,
  sending = false,
  pendingQueue = [],
  tabId,
}: {
  conversationId: number;
  messages: ChatMessage[];
  sending?: boolean;
  pendingQueue?: Array<{ text: string; mentions?: MentionRef[] }>;
  tabId?: string;
}) {
  useAIStore.setState({
    configured: true,
    tabStates: tabId ? { [tabId]: {} } : {},
    conversationMessages: { [conversationId]: messages },
    conversationStreaming: { [conversationId]: { sending, pendingQueue } },
    conversations: [],
    sidebarConversationId: null,
    sidebarUIState: { inputDraft: "", scrollTop: 0 },
  });

  useTabStore.setState(
    tabId
      ? {
          tabs: [
            {
              id: tabId,
              type: "ai",
              label: `conv-${conversationId}`,
              meta: { type: "ai", conversationId, title: `conv-${conversationId}` },
            },
          ],
          activeTabId: tabId,
        }
      : { tabs: [], activeTabId: null }
  );
}

describe("AIChat edit-and-resend 聚合回归", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInputSpies.clear.mockReset();
    mockInputSpies.loadDraft.mockReset();

    HTMLElement.prototype.scrollIntoView = vi.fn();

    useAssetStore.setState({
      assets: [{ ID: 42, Name: "prod-db", Type: "mysql", GroupID: 0, Config: '{"host":"db.internal"}' } as never],
      groups: [],
    } as never);

    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      configured: true,
      tabStates: {},
      conversations: [],
      conversationMessages: {},
      conversationStreaming: {},
      sidebarConversationId: null,
      sidebarUIState: { inputDraft: "", scrollTop: 0 },
    });

    vi.mocked(SendAIMessage).mockResolvedValue(undefined as never);
    vi.mocked(StopAIGeneration).mockResolvedValue(undefined as never);
    vi.mocked(SaveConversationMessages).mockResolvedValue(undefined as never);
    vi.mocked(EventsOn).mockImplementation((() => vi.fn()) as never);
  });

  it("tab 模式会预填 mention 草稿并沿真实 store 链路截断重发", async () => {
    const user = userEvent.setup();
    const conversationId = 201;
    const tabId = "ai-201";
    const mentions: MentionRef[] = [{ assetId: 42, name: "prod-db", start: 6, end: 14 }];

    setupConversationFixture({
      conversationId,
      tabId,
      messages: [
        buildMessage("user", "check @prod-db", { mentions }),
        buildMessage("assistant", "old answer"),
        buildMessage("user", "stale follow-up"),
        buildMessage("assistant", "stale answer"),
      ],
    });

    render(<AIChatContent tabId={tabId} />);

    await user.click(screen.getAllByRole("button", { name: editButtonName })[0]);

    expect(screen.getByText(editingBannerName)).toBeInTheDocument();
    expect(mockInputSpies.loadDraft).toHaveBeenCalledWith({ content: "check @prod-db", mentions });
    await waitFor(() => expect(screen.getByRole("textbox", { name: "mock-ai-input" })).toHaveValue("check @prod-db"));
    expect(screen.getByText("mention:prod-db")).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "mock-ai-input" }), " now");
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => expect(SendAIMessage).toHaveBeenCalledTimes(1));
    expect(vi.mocked(SendAIMessage).mock.calls[0]?.[0]).toBe(conversationId);

    const sentMessages = vi.mocked(SendAIMessage).mock.calls[0]?.[1] as Array<{ role: string; content: string }>;
    expect(sentMessages.map((message) => [message.role, message.content])).toEqual([["user", "check @prod-db now"]]);

    await waitFor(() => {
      expect(
        useAIStore.getState().conversationMessages[conversationId].map((message) => [message.role, message.content])
      ).toEqual([
        ["user", "check @prod-db now"],
        ["assistant", ""],
      ]);
    });
    expect(useAIStore.getState().conversationMessages[conversationId][0].mentions).toEqual(mentions);
    expect(screen.getByRole("button", { name: /prod-db/ })).toBeInTheDocument();
    expect(screen.queryByText("stale follow-up")).not.toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument());
  });

  it("conversationId 直连模式支持 sidebar 路径的 edit-and-resend", async () => {
    const user = userEvent.setup();
    const conversationId = 202;

    setupConversationFixture({
      conversationId,
      messages: [
        buildMessage("user", "first prompt"),
        buildMessage("assistant", "first answer"),
        buildMessage("user", "sidebar draft"),
        buildMessage("assistant", "sidebar answer"),
      ],
    });

    render(<AIChatContent conversationId={conversationId} />);

    await user.click(screen.getAllByRole("button", { name: editButtonName })[1]);

    expect(screen.getByText(editingBannerName)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("textbox", { name: "mock-ai-input" })).toHaveValue("sidebar draft"));

    await user.type(screen.getByRole("textbox", { name: "mock-ai-input" }), " refined");
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => {
      expect(
        useAIStore.getState().conversationMessages[conversationId].map((message) => [message.role, message.content])
      ).toEqual([
        ["user", "first prompt"],
        ["assistant", "first answer"],
        ["user", "sidebar draft refined"],
        ["assistant", ""],
      ]);
    });

    expect(vi.mocked(SendAIMessage).mock.calls[0]?.[0]).toBe(conversationId);
    expect(useTabStore.getState().tabs).toEqual([]);
    expect(screen.queryByText("sidebar answer")).not.toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText(editingBannerName)).not.toBeInTheDocument());
  });

  it("发送中编辑会清空 pendingQueue，并只保留新的后续分支", async () => {
    const user = userEvent.setup();
    const conversationId = 203;

    setupConversationFixture({
      conversationId,
      sending: true,
      pendingQueue: [{ text: "queued-1" }, { text: "queued-2" }],
      messages: [
        buildMessage("user", "root question"),
        buildMessage("assistant", "root answer"),
        buildMessage("user", "branch to edit"),
        buildMessage("assistant", "stale partial", { streaming: true }),
      ],
    });

    render(<AIChatContent conversationId={conversationId} />);

    expect(screen.getByText(pendingQueueName)).toBeInTheDocument();

    await user.click(screen.getAllByRole("button", { name: editButtonName })[1]);
    expect(screen.getByText(editingBannerName)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("textbox", { name: "mock-ai-input" })).toHaveValue("branch to edit"));

    await user.type(screen.getByRole("textbox", { name: "mock-ai-input" }), " revised");
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => expect(StopAIGeneration).toHaveBeenCalledWith(conversationId));
    await waitFor(() => {
      expect(useAIStore.getState().conversationStreaming[conversationId]).toEqual({
        sending: true,
        pendingQueue: [],
      });
    });
    await waitFor(() => {
      expect(
        useAIStore.getState().conversationMessages[conversationId].map((message) => [message.role, message.content])
      ).toEqual([
        ["user", "root question"],
        ["assistant", "root answer"],
        ["user", "branch to edit revised"],
        ["assistant", ""],
      ]);
    });

    expect(screen.queryByText(pendingQueueName)).not.toBeInTheDocument();
    expect(screen.queryByText("stale partial")).not.toBeInTheDocument();
    expect(vi.mocked(SendAIMessage).mock.calls[0]?.[0]).toBe(conversationId);
  });
});
