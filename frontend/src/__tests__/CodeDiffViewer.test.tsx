import { describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";

const diffEditorMock = vi.fn();

vi.mock("@/lib/monaco-setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@monaco-editor/react", () => ({
  DiffEditor: (props: Record<string, unknown>) => {
    diffEditorMock(props);
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
      };
    };

    expect(props.options?.diffAlgorithm).toBe("advanced");
    expect(props.options?.renderSideBySide).toBe(true);
    expect(props.options?.useInlineViewWhenSpaceIsLimited).toBe(false);
    expect(props.options?.readOnly).toBe(true);
    expect(props.options?.originalEditable).toBe(false);
  });
});
