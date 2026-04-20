import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { AIChatInput, type AIChatInputHandle } from "@/components/ai/AIChatInput";
import { useAssetStore } from "@/stores/assetStore";
import type { Editor } from "@tiptap/react";

function seed() {
  useAssetStore.setState({
    assets: [{ ID: 42, Name: "prod-db", Type: "mysql", GroupID: 0 }],
    groups: [],
  } as unknown as Parameters<typeof useAssetStore.setState>[0]);
}

describe("AIChatInput", () => {
  beforeEach(() => {
    seed();
  });

  it("纯文本提交回调收到 text + 空 mentions", async () => {
    const onSubmit = vi.fn();
    render(<AIChatInput onSubmit={onSubmit} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("hello");
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const [text, mentions] = onSubmit.mock.calls[0];
    expect(text).toBe("hello");
    expect(mentions).toEqual([]);
  });

  it("输入 @ 弹出 MentionList", async () => {
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("@prod");
    await waitFor(() => expect(screen.getByRole("listbox")).toBeInTheDocument());
    expect(screen.getByRole("option").textContent).toContain("prod-db");
  });

  it("选中 mention 后提交回调 mentions 包含 assetId", async () => {
    const onSubmit = vi.fn();
    const editorRef = { current: null as Editor | null };
    const handleRef = createRef<AIChatInputHandle>();
    render(<AIChatInput ref={handleRef} onSubmit={onSubmit} sendOnEnter={true} editorRef={editorRef} />);
    // 等待 editor 就绪
    await waitFor(() => expect(editorRef.current).not.toBeNull());
    const editor = editorRef.current!;
    // 直接通过 editor API 构造 "check @prod-db disk" 富文本内容
    editor
      .chain()
      .focus()
      .insertContent("check ")
      .insertContent({
        type: "mention",
        attrs: { id: "42", label: "prod-db" },
      })
      .insertContent(" disk")
      .run();
    // 通过 ref.submit 触发提交
    handleRef.current?.submit();
    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    const [text, mentions] = onSubmit.mock.calls[0];
    expect(text).toMatch(/@prod-db/);
    expect(mentions).toEqual([expect.objectContaining({ assetId: 42, name: "prod-db" })]);
    expect(mentions[0].end).toBeGreaterThan(mentions[0].start);
  });
});
