import { describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { buildTextDiffBlocks } from "../lib/textDiffBlocks";

const diffEditorMock = vi.fn();

vi.mock("@/lib/monaco-setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@monaco-editor/react", () => ({
  DiffEditor: (props: Record<string, unknown>) => {
    diffEditorMock(props);
    const onMount = props.onMount as
      | ((editor: { getOriginalEditor: () => unknown; getModifiedEditor: () => unknown }, monaco: unknown) => void)
      | undefined;
    onMount?.(
      {
        getOriginalEditor: () => ({
          createDecorationsCollection: vi.fn(() => ({ clear: vi.fn() })),
          revealLineInCenter: vi.fn(),
          setPosition: vi.fn(),
        }),
        getModifiedEditor: () => ({
          createDecorationsCollection: vi.fn(() => ({ clear: vi.fn() })),
          revealLineInCenter: vi.fn(),
          setPosition: vi.fn(),
        }),
      },
      {
        Range: vi.fn(function Range(this: unknown, startLine: number, startColumn: number, endLine: number, endColumn: number) {
          return { startLineNumber: startLine, startColumn, endLineNumber: endLine, endColumn };
        }),
        editor: { OverviewRulerLane: { Full: 7 } },
      }
    );
    return null;
  },
}));

describe("CodeDiffViewer", () => {
  it("forces side-by-side diff mode without inline fallback", async () => {
    const { CodeDiffViewer } = await import("../components/CodeDiffViewer");
    render(<CodeDiffViewer original="remote" modified="local" />);

    await waitFor(() => {
      expect(diffEditorMock).toHaveBeenCalled();
    });

    const calls = diffEditorMock.mock.calls as Array<[Record<string, unknown>]>;
    const props = calls[0][0] as {
      options?: {
        diffAlgorithm?: string;
        renderSideBySide?: boolean;
        useInlineViewWhenSpaceIsLimited?: boolean;
        readOnly?: boolean;
        originalEditable?: boolean;
        renderOverviewRuler?: boolean;
        glyphMargin?: boolean;
      };
      wrapperProps?: Record<string, string>;
    };

    expect(props.options?.diffAlgorithm).toBe("advanced");
    expect(props.options?.renderSideBySide).toBe(true);
    expect(props.options?.useInlineViewWhenSpaceIsLimited).toBe(false);
    expect(props.options?.readOnly).toBe(true);
    expect(props.options?.originalEditable).toBe(false);
    expect(props.options?.renderOverviewRuler).toBe(true);
    expect(props.options?.glyphMargin).toBe(true);
  });

  it("reports diff count and exposes wrapper test id for navigation", async () => {
    const onDiffStatsChange = vi.fn();
    const { CodeDiffViewer } = await import("../components/CodeDiffViewer");
    render(
      <CodeDiffViewer
        activeBlockIndex={0}
        modified={"same\nlocal\n"}
        navigationToken={1}
        original={"same\nremote\n"}
        testId="diff-editor"
        onDiffStatsChange={onDiffStatsChange}
      />
    );

    await waitFor(() => expect(diffEditorMock).toHaveBeenCalled());
    await waitFor(() => expect(onDiffStatsChange).toHaveBeenCalledWith(expect.objectContaining({ total: 1 })));

    const calls = diffEditorMock.mock.calls as Array<[Record<string, unknown>]>;
    const props = calls.at(-1)?.[0] as { wrapperProps?: Record<string, string> };
    expect(props.wrapperProps).toEqual({ "data-testid": "diff-editor" });
  });

  it("normalizes insert/delete/modify blocks for external edit navigation", () => {
    expect(buildTextDiffBlocks("a\nb\nc", "a\nB\nc")).toEqual([
      {
        id: "modify:1-1:1-1",
        kind: "modify",
        originalStartLine: 2,
        originalEndLine: 2,
        modifiedStartLine: 2,
        modifiedEndLine: 2,
      },
    ]);
    expect(buildTextDiffBlocks("a\nb", "a\nb\nc")[0]?.kind).toBe("insert");
    expect(buildTextDiffBlocks("a\nb\nc", "a\nb")[0]?.kind).toBe("delete");
  });
});
