/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../i18n", () => ({
  default: { t: (key: string, fallback: string) => fallback || key },
}));

import { useAIStore } from "@/stores/aiStore";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";
import { SendAIMessage, CreateConversation, QueueAIMessage } from "../../wailsjs/go/app/App";

describe("aiStore mentions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAIStore.setState({
      tabStates: { t1: {} },
      conversationMessages: { 1: [] },
      conversationStreaming: { 1: { sending: false, pendingQueue: [] } },
      conversations: [],
      configured: true,
      providerName: "",
      modelName: "",
    } as any);
    useAssetStore.setState({
      assets: [
        {
          ID: 42,
          Name: "prod-db",
          Type: "mysql",
          GroupID: 1,
          Config: JSON.stringify({ host: "10.0.0.5", port: 3306 }),
        } as any,
      ],
      groups: [{ ID: 1, Name: "数据库", ParentID: 0 } as any],
    } as any);
    useTabStore.setState({
      tabs: [
        {
          id: "t1",
          type: "ai",
          label: "对话",
          meta: { type: "ai", conversationId: 1, title: "对话" },
        } as any,
      ],
      activeTabId: "t1",
    } as any);
    vi.mocked(CreateConversation).mockResolvedValue({ ID: 1 } as any);
    vi.mocked(SendAIMessage).mockResolvedValue(undefined as any);
    vi.mocked(QueueAIMessage).mockResolvedValue(undefined as any);
  });

  it("sendToTab 把 mentions 写入新消息", async () => {
    await useAIStore.getState().sendToTab("t1", "ping @prod-db", [{ assetId: 42, name: "prod-db", start: 5, end: 13 }]);
    const msgs = useAIStore.getState().conversationMessages[1];
    const userMsg = msgs.find((m) => m.role === "user")!;
    expect(userMsg.content).toBe("ping @prod-db");
    expect(userMsg.mentions).toEqual([{ assetId: 42, name: "prod-db", start: 5, end: 13 }]);
  });

  it("SendAIMessage 调用的 AIContext 包含 MentionedAssets", async () => {
    await useAIStore
      .getState()
      .sendToTab("t1", "check @prod-db", [{ assetId: 42, name: "prod-db", start: 6, end: 14 }]);
    expect(SendAIMessage).toHaveBeenCalledTimes(1);
    const [, , ctx] = vi.mocked(SendAIMessage).mock.calls[0] as any[];
    expect(ctx.mentionedAssets).toHaveLength(1);
    expect(ctx.mentionedAssets[0]).toMatchObject({
      assetId: 42,
      name: "prod-db",
      type: "mysql",
      host: "10.0.0.5",
      groupPath: "数据库",
    });
  });

  it("资产已删除时跳过该 mention，不阻塞发送", async () => {
    await useAIStore.getState().sendToTab("t1", "ping @ghost", [{ assetId: 999, name: "ghost", start: 5, end: 11 }]);
    expect(SendAIMessage).toHaveBeenCalledTimes(1);
    const [, , ctx] = vi.mocked(SendAIMessage).mock.calls[0] as any[];
    expect(ctx.mentionedAssets).toHaveLength(0);
  });

  it("pendingQueue 条目带 mentions", () => {
    useAIStore.setState((s) => ({
      conversationStreaming: {
        ...s.conversationStreaming,
        1: { sending: true, pendingQueue: [] },
      },
    }));
    useAIStore.getState().sendToTab("t1", "queued @prod-db", [{ assetId: 42, name: "prod-db", start: 7, end: 15 }]);
    const q = useAIStore.getState().conversationStreaming[1].pendingQueue;
    expect(q).toHaveLength(1);
    expect(q[0]).toMatchObject({ text: "queued @prod-db" });
    expect(q[0].mentions).toEqual([{ assetId: 42, name: "prod-db", start: 7, end: 15 }]);
  });
});
