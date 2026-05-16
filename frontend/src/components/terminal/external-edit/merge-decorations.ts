import type * as MonacoNS from "monaco-editor";
import type { TextDiffBlock } from "@/lib/textDiffBlocks";

export type MergePaneRole = "local" | "final" | "remote";

export type MergeEditorRefs = Record<
  MergePaneRole,
  { editor: MonacoNS.editor.IStandaloneCodeEditor | null; monaco: typeof MonacoNS | null }
>;

export type MergeDecorationRefs = Record<MergePaneRole, MonacoNS.editor.IEditorDecorationsCollection | null>;

export function blockLineRange(block: TextDiffBlock, pane: MergePaneRole) {
  const startLine = pane === "remote" ? block.originalStartLine : block.modifiedStartLine;
  const endLine = pane === "remote" ? block.originalEndLine : block.modifiedEndLine;
  return { startLine, endLine };
}

function mergePaneLineClass(block: TextDiffBlock, pane: MergePaneRole, active: boolean) {
  if (active) return "external-edit-merge-line-current";
  if (pane === "remote") {
    return block.kind === "insert" ? "" : "external-edit-merge-line-remote-change";
  }
  if (pane === "local") {
    return block.kind === "delete" ? "" : "external-edit-merge-line-local-change";
  }
  if (block.kind === "delete") return "external-edit-merge-line-final-local";
  if (block.kind === "insert") return "external-edit-merge-line-final-remote";
  return "external-edit-merge-line-final-combined";
}

function mergePaneGutterClass(block: TextDiffBlock, pane: MergePaneRole, active: boolean) {
  if (active) return "external-edit-merge-gutter-current";
  if (pane === "remote") {
    return block.kind === "insert" ? "" : "external-edit-merge-gutter-remote";
  }
  if (pane === "local") {
    return block.kind === "delete" ? "" : "external-edit-merge-gutter-local";
  }
  if (block.kind === "insert") return "external-edit-merge-gutter-remote";
  if (block.kind === "delete") return "external-edit-merge-gutter-local";
  return "external-edit-merge-gutter-combined";
}

export function buildMergePaneDecorations(
  monaco: typeof MonacoNS,
  blocks: TextDiffBlock[],
  activeIndex: number,
  pane: MergePaneRole
): MonacoNS.editor.IModelDeltaDecoration[] {
  return blocks.flatMap((block, index) => {
    const { startLine, endLine } = blockLineRange(block, pane);
    if (endLine < startLine || startLine < 1) return [];
    const active = index === activeIndex;
    const className = mergePaneLineClass(block, pane, active);
    const glyphMarginClassName = mergePaneGutterClass(block, pane, active);
    if (!className && !glyphMarginClassName) return [];
    return [
      {
        range: new monaco.Range(startLine, 1, endLine, 1),
        options: {
          isWholeLine: true,
          className,
          glyphMarginClassName,
          overviewRuler: {
            color: block.kind === "insert" ? "#16a34a" : block.kind === "delete" ? "#dc2626" : "#d97706",
            position: monaco.editor.OverviewRulerLane.Full,
          },
        },
      },
    ];
  });
}
