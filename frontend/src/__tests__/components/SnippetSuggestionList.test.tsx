/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { createRef } from "react";
import {
  SnippetSuggestionList,
  type SnippetSuggestionItem,
  type SnippetSuggestionListRef,
} from "@/components/ai/SnippetSuggestionList";
import { useTabStore } from "@/stores/tabStore";

function makeItem(partial: Partial<SnippetSuggestionItem> = {}): SnippetSuggestionItem {
  return {
    id: 1,
    name: "Review SQL",
    preview: "Review this SQL for performance issues:",
    content: "Review this SQL for performance issues:\n",
    readOnly: false,
    ...partial,
  };
}

describe("SnippetSuggestionList", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null } as any);
  });

  it("renders items with name + preview; clicking invokes command", () => {
    const items = [
      makeItem({ id: 1, name: "Review SQL", preview: "Review this SQL" }),
      makeItem({ id: 2, name: "Write tests", preview: "Write unit tests" }),
    ];
    const command = vi.fn();
    render(<SnippetSuggestionList items={items} totalAvailable={items.length || 5} command={command} />);
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(2);
    expect(options[0].textContent).toContain("Review SQL");
    expect(options[0].textContent).toContain("Review this SQL");
    fireEvent.click(options[1]);
    expect(command).toHaveBeenCalledWith(items[1]);
  });

  it("renders a lock icon for read-only items", () => {
    const items = [makeItem({ id: 5, name: "ext prompt", readOnly: true })];
    const { container } = render(
      <SnippetSuggestionList items={items} totalAvailable={items.length || 5} command={vi.fn()} />
    );
    // Lucide renders an <svg>; the lock is marked aria-hidden alongside the name.
    const svgs = container.querySelectorAll("svg");
    expect(svgs.length).toBeGreaterThan(0);
  });

  it("ArrowDown + Enter selects the second item", () => {
    const items = [makeItem({ id: 1, name: "a" }), makeItem({ id: 2, name: "b" })];
    const command = vi.fn();
    const ref = createRef<SnippetSuggestionListRef>();
    render(<SnippetSuggestionList ref={ref} items={items} totalAvailable={items.length || 5} command={command} />);
    ref.current?.onKeyDown({ event: { key: "ArrowDown" } as any });
    const options = screen.getAllByRole("option");
    expect(options[1]).toHaveAttribute("aria-selected", "true");
    ref.current?.onKeyDown({ event: { key: "Enter" } as any });
    expect(command).toHaveBeenCalledWith(items[1]);
  });

  it("ArrowUp wraps around from first to last", () => {
    const items = [makeItem({ id: 1 }), makeItem({ id: 2 }), makeItem({ id: 3 })];
    const ref = createRef<SnippetSuggestionListRef>();
    render(<SnippetSuggestionList ref={ref} items={items} totalAvailable={items.length || 5} command={vi.fn()} />);
    ref.current?.onKeyDown({ event: { key: "ArrowUp" } as any });
    const options = screen.getAllByRole("option");
    expect(options[2]).toHaveAttribute("aria-selected", "true");
  });

  it("shows 'no matching' when items is empty but totalAvailable > 0", () => {
    render(<SnippetSuggestionList items={[]} totalAvailable={5} command={vi.fn()} />);
    expect(screen.getByText("snippet.slash.noMatch")).toBeInTheDocument();
    expect(screen.queryByText("snippet.slash.empty")).not.toBeInTheDocument();
  });

  it("shows 'no matching' state when filter zeroes out a non-empty total (regression: unreachable no-match)", () => {
    // Previously, because totalAvailable was stamped on each item, a filter
    // that wiped items to [] also wiped the total — flipping to the CTA.
    // Passing totalAvailable out-of-band fixes that.
    render(<SnippetSuggestionList items={[]} totalAvailable={7} command={vi.fn()} />);
    expect(screen.getByText("snippet.slash.noMatch")).toBeInTheDocument();
    expect(screen.queryByTestId("snippet-suggestion-open-manager")).toBeNull();
  });

  it("empty state (totalAvailable=0) shows CTA that opens snippets tab", () => {
    render(<SnippetSuggestionList items={[]} totalAvailable={0} command={vi.fn()} />);
    expect(screen.getByText("snippet.slash.empty")).toBeInTheDocument();
    const btn = screen.getByTestId("snippet-suggestion-open-manager");
    fireEvent.click(btn);
    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("snippets");
    expect(tabs[0].type).toBe("page");
    expect((tabs[0].meta as any).pageId).toBe("snippets");
  });

  it("Enter/Arrow keys are no-ops when items is empty", () => {
    const command = vi.fn();
    const ref = createRef<SnippetSuggestionListRef>();
    render(<SnippetSuggestionList ref={ref} items={[]} totalAvailable={5} command={command} />);
    const handled = ref.current?.onKeyDown({ event: { key: "Enter" } as any });
    expect(handled).toBe(false);
    expect(command).not.toHaveBeenCalled();
  });
});
