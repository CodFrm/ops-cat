import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { AIChatInput, type AIChatInputHandle } from "@/components/ai/AIChatInput";
import { useAssetStore } from "@/stores/assetStore";
import type { Editor } from "@tiptap/react";
import { ListSnippets, RecordSnippetUse } from "../../wailsjs/go/app/App";

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

  it("提及弹窗激活时 Enter 选中候选项而不触发发送", async () => {
    const onSubmit = vi.fn();
    render(<AIChatInput onSubmit={onSubmit} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("@prod");
    await waitFor(() => expect(screen.getByRole("listbox")).toBeInTheDocument());
    await userEvent.keyboard("{Enter}");
    // Enter 应被 suggestion 消费用于插入 mention，不应触发 onSubmit
    expect(onSubmit).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    // 再次 Enter 应正常发送，mention 已插入
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const [text, mentions] = onSubmit.mock.calls[0];
    expect(text).toMatch(/@prod-db/);
    expect(mentions).toEqual([expect.objectContaining({ assetId: 42, name: "prod-db" })]);
  });

  it("ArrowUp 在首字符位置接管：取最近一条用户消息", async () => {
    const editorRef = { current: null as Editor | null };
    const history = ["最新", "次新", "更早"];
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} editorRef={editorRef} userMessageHistory={history} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("最新"));
  });

  it("重复 ArrowUp 逐步回溯更早的用户消息", async () => {
    const editorRef = { current: null as Editor | null };
    const history = ["最新", "次新", "更早"];
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} editorRef={editorRef} userMessageHistory={history} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("最新"));
    await userEvent.keyboard("{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("次新"));
    await userEvent.keyboard("{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("更早"));
    // 到达最老记录后再按 ArrowUp 应保持最老一条，不越界
    await userEvent.keyboard("{ArrowUp}");
    expect(editorRef.current!.getText()).toBe("更早");
  });

  it("ArrowDown 向前浏览，最终回到空输入", async () => {
    const editorRef = { current: null as Editor | null };
    const history = ["最新", "次新"];
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} editorRef={editorRef} userMessageHistory={history} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("{ArrowUp}{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("次新"));
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("最新"));
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe(""));
  });

  it("光标不在首字符时 ArrowUp 不接管历史", async () => {
    const editorRef = { current: null as Editor | null };
    const history = ["history message"];
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} editorRef={editorRef} userMessageHistory={history} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());
    // 通过 editor API 写入文本并把光标放到末尾，避免 userEvent + contenteditable 的段落偏差影响断言
    editorRef.current!.chain().focus().insertContent("typing").focus("end").run();
    const textBefore = editorRef.current!.getText();
    await userEvent.keyboard("{ArrowUp}");
    // ArrowUp 不应替换为历史记录；文本保持不变即可证明拦截被跳过
    expect(editorRef.current!.getText()).toBe(textBefore);
    expect(editorRef.current!.getText()).not.toBe("history message");
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

  it("输入 `/` 打开 snippet 弹窗并请求 prompt 分类的列表", async () => {
    vi.mocked(ListSnippets).mockResolvedValueOnce([
      {
        ID: 1,
        Name: "Review SQL",
        Category: "prompt",
        Content: "Review this SQL for performance issues:",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
      } as unknown as Awaited<ReturnType<typeof ListSnippets>>[number],
    ]);
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("/");
    await waitFor(() => expect(ListSnippets).toHaveBeenCalled());
    const req = vi.mocked(ListSnippets).mock.calls.at(-1)![0] as unknown as {
      categories: string[];
      includeGlobal: boolean;
    };
    expect(req.categories).toEqual(["prompt"]);
    expect(req.includeGlobal).toBe(true);
    // 列表以 portal 形式渲染在 document.body
    await waitFor(() => expect(document.querySelector("[data-testid=snippet-suggestion-list]")).toBeTruthy());
  });

  it("选中 `/` 片段后以纯文本插入内容并调用 recordUse", async () => {
    const content = "Review this SQL for performance issues:";
    vi.mocked(ListSnippets).mockResolvedValue([
      {
        ID: 77,
        Name: "Review SQL",
        Category: "prompt",
        Content: content,
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
      } as unknown as Awaited<ReturnType<typeof ListSnippets>>[number],
    ]);
    const editorRef = { current: null as Editor | null };
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} editorRef={editorRef} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());

    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("/");
    await waitFor(() => expect(document.querySelector("[data-testid=snippet-suggestion-list]")).toBeTruthy());
    // Enter 让 suggestion 插件处理，选中首项
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(editorRef.current!.getText()).toContain(content));
    // 没有 mention 节点应该被插入
    const doc = editorRef.current!.getJSON();
    const firstPara = doc.content?.[0];
    const hasMention = firstPara?.content?.some((n) => n.type === "mention") ?? false;
    expect(hasMention).toBe(false);
    await waitFor(() => expect(RecordSnippetUse).toHaveBeenCalledWith(77));
  });

  it("在 URL 中间的 `/` 不触发 snippet 弹窗（TipTap 默认 allowedPrefixes 阻止）", async () => {
    vi.mocked(ListSnippets).mockClear();
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("http:/");
    // TipTap 的 Suggestion 默认 allowedPrefixes=[' ']，要求 `/` 前是空白或行首，
    // 这里前一个字符是 `:`，因此 findSuggestionMatch 会直接拒绝。
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(document.querySelector("[data-testid=snippet-suggestion-list]")).toBeNull();
    expect(document.querySelector("[data-testid=snippet-suggestion-empty]")).toBeNull();
    expect(ListSnippets).not.toHaveBeenCalled();
  });

  it("`/` 在真实内容+空格之后仍能触发 snippet 弹窗", async () => {
    vi.mocked(ListSnippets).mockClear();
    vi.mocked(ListSnippets).mockResolvedValue([
      {
        ID: 1,
        Name: "Review SQL",
        Category: "prompt",
        Content: "Review this SQL for performance issues:",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
      } as unknown as Awaited<ReturnType<typeof ListSnippets>>[number],
    ]);
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    // 之前自定义 allow 存在 off-by-one，`hello /` 这种合法触发会被误拒。
    await userEvent.keyboard("hello /");
    await waitFor(() => expect(ListSnippets).toHaveBeenCalled());
    await waitFor(() => expect(document.querySelector("[data-testid=snippet-suggestion-list]")).toBeTruthy());
  });

  it("`/zzz` 过滤到 0 项但总数>0 时显示“无匹配”而非“暂无片段” CTA", async () => {
    // 回归用：此前 totalAvailable 被戳在每个 item 上，过滤到空后会读到 0，
    // 导致 UI 错误地翻到 totalEmpty 分支。
    vi.mocked(ListSnippets).mockClear();
    vi.mocked(ListSnippets).mockResolvedValue([
      {
        ID: 1,
        Name: "Review SQL",
        Category: "prompt",
        Content: "Review this SQL for performance issues:",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
      } as unknown as Awaited<ReturnType<typeof ListSnippets>>[number],
      {
        ID: 2,
        Name: "Write tests",
        Category: "prompt",
        Content: "Write unit tests",
        Description: "",
        Tags: "",
        Source: "user",
        SourceRef: "",
        UseCount: 0,
        Status: 1,
      } as unknown as Awaited<ReturnType<typeof ListSnippets>>[number],
    ]);
    render(<AIChatInput onSubmit={vi.fn()} sendOnEnter={true} />);
    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("/zzz");
    await waitFor(() => expect(document.querySelector("[data-testid=snippet-suggestion-nomatch]")).toBeTruthy());
    expect(document.querySelector("[data-testid=snippet-suggestion-empty]")).toBeNull();
  });

  it("preserves multi-paragraph mentions when submitting an externally loaded draft", async () => {
    const onSubmit = vi.fn();
    const editorRef = { current: null as Editor | null };
    const handleRef = createRef<AIChatInputHandle>();
    const content = "check @prod-db disk\nthen @prod-db again";
    const draftMentions = [
      { assetId: 42, name: "prod-db", start: 6, end: 14 },
      { assetId: 42, name: "prod-db", start: 25, end: 33 },
    ];

    render(<AIChatInput ref={handleRef} onSubmit={onSubmit} sendOnEnter={true} editorRef={editorRef} />);
    await waitFor(() => expect(editorRef.current).not.toBeNull());

    handleRef.current?.loadDraft({ content, mentions: draftMentions });
    await waitFor(() => expect(editorRef.current!.getText({ blockSeparator: "\n" })).toBe(content));

    handleRef.current?.submit();

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const [submittedText, submittedMentions] = onSubmit.mock.calls[0];
    expect(submittedText).toBe(content);
    expect(submittedMentions).toEqual(draftMentions);
  });

  it("resets the history cursor after loading an external draft so ArrowUp restarts from latest", async () => {
    const handleRef = createRef<AIChatInputHandle>();
    const editorRef = { current: null as Editor | null };
    const history = ["最新", "次新", "更早"];

    render(
      <AIChatInput
        ref={handleRef}
        onSubmit={vi.fn()}
        sendOnEnter={true}
        editorRef={editorRef}
        userMessageHistory={history}
      />
    );
    await waitFor(() => expect(editorRef.current).not.toBeNull());

    const editor = screen.getByRole("textbox");
    await userEvent.click(editor);
    await userEvent.keyboard("{ArrowUp}{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("次新"));

    handleRef.current?.loadDraft("外部草稿");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("外部草稿"));

    editorRef.current!.chain().focus("start").run();
    await userEvent.keyboard("{ArrowUp}");
    await waitFor(() => expect(editorRef.current!.getText()).toBe("最新"));
  });
});
