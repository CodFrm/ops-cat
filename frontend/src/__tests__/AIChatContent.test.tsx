import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { useAIStore } from "../stores/aiStore";
import { useTabStore } from "../stores/tabStore";
import { AIChatContent } from "../components/ai/AIChatContent";

describe("AIChatContent (Phase 1 refactor)", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useAIStore.setState({
      tabStates: {},
      conversations: [],
      configured: true,
      conversationMessages: {},
      conversationStreaming: {},
    });
  });

  it("renders messages read from conversationMessages (not tabStates)", () => {
    const tabId = "ai-5";
    useTabStore.setState({
      tabs: [{ id: tabId, type: "ai", label: "t", meta: { type: "ai", conversationId: 5, title: "t" } }],
      activeTabId: tabId,
    });
    // Only write to conversationMessages — component must read from there
    useAIStore.setState({
      conversationMessages: {
        5: [{ role: "user", content: "从 conversationMessages 读到", blocks: [] }],
      },
      tabStates: { [tabId]: {} },
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
});
