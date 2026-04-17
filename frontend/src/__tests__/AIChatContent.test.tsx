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
});
