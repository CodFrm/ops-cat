/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../i18n", () => ({
  default: { t: (key: string, fallback: string) => fallback || key },
}));

import { useAIStore } from "@/stores/aiStore";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";
import { SendAIMessage, CreateConversation, QueueAIMessage } from "../../wailsjs/go/app/App";

// mention 信息现在以内联 <mention> XML 形式写在 content 里，前端不再维护独立的
// mentions 数组，也不再把 MentionedAssets 塞进 AIContext。这些 case 校验：
//   - content 原样进入消息体（已经在前端 AIChatInput 里 build 好）
//   - 排队场景下 QueueAIMessage 透传队列 ID + content（标签解析交给后端 prompt builder）
//   - AIContext 只包含 openTabs，不再有 mentionedAssets

const mentionXml = '<mention asset-id="42" type="mysql" host="10.0.0.5" group="数据库">@prod-db</mention>';

describe("aiStore mentions (XML inline)", () => {
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

  it("sendToTab 把含 <mention> 标签的 content 原样写入新消息", async () => {
    const content = `ping ${mentionXml}`;
    await useAIStore.getState().sendToTab("t1", content);
    const msgs = useAIStore.getState().conversationMessages[1];
    const userMsg = msgs.find((m) => m.role === "user")!;
    expect(userMsg.content).toBe(content);
  });

  it("SendAIMessage 调用的 AIContext 不再包含 MentionedAssets", async () => {
    await useAIStore.getState().sendToTab("t1", `check ${mentionXml}`);
    expect(SendAIMessage).toHaveBeenCalledTimes(1);
    const [, , ctx] = vi.mocked(SendAIMessage).mock.calls[0] as any[];
    expect(ctx).not.toHaveProperty("mentionedAssets");
    expect(Array.isArray(ctx.openTabs)).toBe(true);
  });

  it("SendAIMessage 的 AIContext 把 query tab 映射成真实资产类型", async () => {
    useTabStore.setState((state) => ({
      tabs: [
        ...state.tabs,
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
          },
        } as any,
      ],
      activeTabId: state.activeTabId,
    }));

    await useAIStore.getState().sendToTab("t1", `check ${mentionXml}`);

    const [, , ctx] = vi.mocked(SendAIMessage).mock.calls[0] as any[];
    expect(ctx.openTabs).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "database",
          assetId: 42,
          assetName: "prod-db",
        }),
      ])
    );
  });

  it("生成中时 sendToTab 把 content 入队并调用 QueueAIMessage(convId, queueId, content)", () => {
    useAIStore.setState((s) => ({
      conversationStreaming: {
        ...s.conversationStreaming,
        1: { sending: true, pendingQueue: [] },
      },
    }));
    const content = `queued ${mentionXml}`;
    useAIStore.getState().sendToTab("t1", content);

    const q = useAIStore.getState().conversationStreaming[1].pendingQueue;
    expect(q).toHaveLength(1);
    expect(q[0]?.id).toMatch(/^queue-/);
    expect(q[0]?.text).toBe(content);

    expect(QueueAIMessage).toHaveBeenCalledTimes(1);
    const args = vi.mocked(QueueAIMessage).mock.calls[0];
    expect(args[0]).toBe(1);
    expect(args[1]).toBe(q[0]?.id);
    expect(args[2]).toBe(content);
    // QueueAIMessage 只接收 conversationId、queueId、content 三个参数。
    expect(args).toHaveLength(3);
  });
});
