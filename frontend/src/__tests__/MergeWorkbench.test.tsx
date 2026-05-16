import { render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type * as MonacoNS from "monaco-editor";
import { ExternalEditMergeWorkbench } from "../components/terminal/external-edit/MergeWorkbench";
import { useExternalEditStore } from "../stores/externalEditStore";

const { codeEditorMountController } = vi.hoisted(() => ({
  codeEditorMountController: {
    mounts: [] as Array<() => void>,
    editors: new Map<
      string,
      {
        createDecorationsCollection: ReturnType<typeof vi.fn>;
        revealLineInCenter: ReturnType<typeof vi.fn>;
        setPosition: ReturnType<typeof vi.fn>;
      }
    >(),
  },
}));

const requestAnimationFrameMock = vi.fn((callback: FrameRequestCallback) => {
  callback(0);
  return 1;
});
const cancelAnimationFrameMock = vi.fn();
vi.stubGlobal("requestAnimationFrame", requestAnimationFrameMock);
vi.stubGlobal("cancelAnimationFrame", cancelAnimationFrameMock);

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/components/CodeEditor", () => ({
  CodeEditor: ({
    onMount,
    readOnly,
    testId,
    value,
  }: {
    onMount?: (editor: unknown, monaco: unknown) => void;
    readOnly?: boolean;
    testId?: string;
    value?: string;
  }) => {
    const editor = {
      createDecorationsCollection: vi.fn(() => ({ clear: vi.fn() })),
      revealLineInCenter: vi.fn(),
      setPosition: vi.fn(),
    };
    const monaco = {
      Range: vi.fn(function Range(
        this: unknown,
        startLine: number,
        startColumn: number,
        endLine: number,
        endColumn: number
      ) {
        return { startLineNumber: startLine, startColumn, endLineNumber: endLine, endColumn };
      }),
      editor: { OverviewRulerLane: { Full: 7 } },
    } as unknown as typeof MonacoNS;
    const mount = () => {
      codeEditorMountController.editors.set(testId || "unknown", editor);
      onMount?.(editor, monaco);
    };
    codeEditorMountController.mounts.push(mount);
    return readOnly ? (
      <pre data-testid={testId}>{value}</pre>
    ) : (
      <textarea data-testid={testId} value={value || ""} readOnly />
    );
  },
}));

describe("ExternalEditMergeWorkbench", () => {
  beforeEach(() => {
    codeEditorMountController.mounts = [];
    codeEditorMountController.editors.clear();
    requestAnimationFrameMock.mockClear();
    cancelAnimationFrameMock.mockClear();
    useExternalEditStore.setState({ applyMerge: vi.fn() });
  });

  it("re-runs decorations and reveal after Monaco mounts so the first conflict is visible on first open", async () => {
    render(
      <ExternalEditMergeWorkbench
        mergeResult={{
          documentKey: "101:/srv/app/demo.txt",
          primaryDraftSessionId: "conflict",
          fileName: "demo.txt",
          remotePath: "/srv/app/demo.txt",
          localContent: "line1\nlocal-change\nline3\n",
          remoteContent: "line1\nremote-change\nline3\n",
          finalContent: "line1\nlocal-change\nline3\n",
          remoteHash: "remote-hash",
        }}
        savingSessionId={null}
        onClose={vi.fn()}
        onError={vi.fn()}
      />
    );

    expect(codeEditorMountController.editors.size).toBe(0);

    const initialMounts = [...codeEditorMountController.mounts];
    for (const mount of initialMounts) {
      mount();
    }

    await waitFor(() => {
      expect(requestAnimationFrameMock).toHaveBeenCalled();
      const localEditor = codeEditorMountController.editors.get("external-edit-merge-local");
      const finalEditor = codeEditorMountController.editors.get("external-edit-merge-final");
      const remoteEditor = codeEditorMountController.editors.get("external-edit-merge-remote");

      expect(localEditor?.createDecorationsCollection).toHaveBeenCalled();
      expect(finalEditor?.createDecorationsCollection).toHaveBeenCalled();
      expect(remoteEditor?.createDecorationsCollection).toHaveBeenCalled();

      expect(localEditor?.revealLineInCenter).toHaveBeenCalledWith(2);
      expect(finalEditor?.revealLineInCenter).toHaveBeenCalledWith(2);
      expect(remoteEditor?.revealLineInCenter).toHaveBeenCalledWith(2);

      expect(localEditor?.setPosition).toHaveBeenCalledWith({ lineNumber: 2, column: 1 });
      expect(finalEditor?.setPosition).toHaveBeenCalledWith({ lineNumber: 2, column: 1 });
      expect(remoteEditor?.setPosition).toHaveBeenCalledWith({ lineNumber: 2, column: 1 });
    });
  });
});
