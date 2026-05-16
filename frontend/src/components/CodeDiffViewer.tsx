import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { DiffEditor, type DiffOnMount } from "@monaco-editor/react";
import { useTranslation } from "react-i18next";
import type * as MonacoNS from "monaco-editor";
import { useResolvedTheme } from "./theme-provider";
import type { CodeEditorLanguage } from "./CodeEditor";
import { buildTextDiffBlocks, type TextDiffBlock } from "@/lib/textDiffBlocks";

export interface CodeDiffViewerProps {
  original: string;
  modified: string;
  originalTitle?: string;
  modifiedTitle?: string;
  badge?: string;
  language?: CodeEditorLanguage;
  height?: string | number;
  className?: string;
  activeBlockIndex?: number;
  navigationToken?: number;
  onDiffStatsChange?: (stats: { total: number; blocks: TextDiffBlock[] }) => void;
  testId?: string;
}

const DEFAULT_OPTIONS: MonacoNS.editor.IDiffEditorConstructionOptions = {
  automaticLayout: true,
  diffAlgorithm: "advanced",
  renderSideBySide: true,
  useInlineViewWhenSpaceIsLimited: false,
  readOnly: true,
  originalEditable: false,
  lineNumbers: "on",
  glyphMargin: true,
  renderIndicators: true,
  splitViewDefaultRatio: 0.5,
  enableSplitViewResizing: true,
  renderOverviewRuler: true,
  scrollBeyondLastLine: false,
  fixedOverflowWidgets: true,
  contextmenu: true,
  minimap: { enabled: false },
  diffWordWrap: "on",
  wordWrap: "on",
  overviewRulerLanes: 3,
  hideUnchangedRegions: { enabled: false },
  scrollbar: { alwaysConsumeMouseWheel: false, verticalScrollbarSize: 10, horizontalScrollbarSize: 10 },
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
};

function lineRangeDecorationClass(kind: TextDiffBlock["kind"], active: boolean, side: "original" | "modified") {
  if (active) return "external-edit-compare-line-current";
  if (kind === "insert") return side === "modified" ? "external-edit-compare-line-insert" : "";
  if (kind === "delete") return side === "original" ? "external-edit-compare-line-delete" : "";
  return "external-edit-compare-line-modify";
}

function lineMarkerDecorationClass(kind: TextDiffBlock["kind"], active: boolean, side: "original" | "modified") {
  if (active) return "external-edit-compare-gutter-current";
  if (kind === "insert") return side === "modified" ? "external-edit-compare-gutter-insert" : "";
  if (kind === "delete") return side === "original" ? "external-edit-compare-gutter-delete" : "";
  return "external-edit-compare-gutter-modify";
}

function makeEditorDecorations(
  monaco: typeof MonacoNS,
  blocks: TextDiffBlock[],
  activeBlockIndex: number,
  side: "original" | "modified"
): MonacoNS.editor.IModelDeltaDecoration[] {
  return blocks.flatMap((block, index) => {
    const startLine = side === "original" ? block.originalStartLine : block.modifiedStartLine;
    const endLine = side === "original" ? block.originalEndLine : block.modifiedEndLine;
    if (endLine < startLine || startLine < 1) return [];
    const isActive = index === activeBlockIndex;
    const className = lineRangeDecorationClass(block.kind, isActive, side);
    const glyphClassName = lineMarkerDecorationClass(block.kind, isActive, side);
    if (!className && !glyphClassName) return [];
    const color = block.kind === "insert" ? "#16a34a" : block.kind === "delete" ? "#dc2626" : "#d97706";
    return [
      {
        range: new monaco.Range(startLine, 1, endLine, 1),
        options: {
          isWholeLine: true,
          className,
          glyphMarginClassName: glyphClassName,
          overviewRuler: {
            color,
            position: monaco.editor.OverviewRulerLane.Full,
          },
        },
      },
    ];
  });
}

export function CodeDiffViewer({
  original,
  modified,
  originalTitle,
  modifiedTitle,
  badge,
  language = "plaintext",
  height = "100%",
  className,
  activeBlockIndex = 0,
  navigationToken = 0,
  onDiffStatsChange,
  testId,
}: CodeDiffViewerProps) {
  const { t } = useTranslation();
  const resolvedTheme = useResolvedTheme();
  const diffEditorRef = useRef<MonacoNS.editor.IStandaloneDiffEditor | null>(null);
  const monacoRef = useRef<typeof MonacoNS | null>(null);
  const originalDecorationsRef = useRef<MonacoNS.editor.IEditorDecorationsCollection | null>(null);
  const modifiedDecorationsRef = useRef<MonacoNS.editor.IEditorDecorationsCollection | null>(null);
  const [monacoReady, setMonacoReady] = useState(false);
  const [monacoLoadError, setMonacoLoadError] = useState<unknown>(null);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const diffBlocks = useMemo(() => buildTextDiffBlocks(original, modified), [original, modified]);

  useEffect(() => {
    onDiffStatsChange?.({ total: diffBlocks.length, blocks: diffBlocks });
  }, [diffBlocks, onDiffStatsChange]);

  const applyDecorations = useCallback(() => {
    const diffEditor = diffEditorRef.current;
    const monaco = monacoRef.current;
    if (!diffEditor || !monaco) return;
    const originalEditor = diffEditor.getOriginalEditor();
    const modifiedEditor = diffEditor.getModifiedEditor();
    const originalDecorations = makeEditorDecorations(monaco, diffBlocks, activeBlockIndex, "original");
    const modifiedDecorations = makeEditorDecorations(monaco, diffBlocks, activeBlockIndex, "modified");
    originalDecorationsRef.current?.clear();
    modifiedDecorationsRef.current?.clear();
    originalDecorationsRef.current = originalEditor.createDecorationsCollection(originalDecorations);
    modifiedDecorationsRef.current = modifiedEditor.createDecorationsCollection(modifiedDecorations);
  }, [activeBlockIndex, diffBlocks]);

  useEffect(() => {
    applyDecorations();
  }, [applyDecorations]);

  useEffect(() => {
    const diffEditor = diffEditorRef.current;
    if (!diffEditor || diffBlocks.length === 0) return;
    const target = diffBlocks[Math.min(Math.max(activeBlockIndex, 0), diffBlocks.length - 1)];
    const originalLine = Math.max(1, target.originalStartLine);
    const modifiedLine = Math.max(1, target.modifiedStartLine);
    diffEditor.getOriginalEditor().revealLineInCenter(originalLine);
    diffEditor.getModifiedEditor().revealLineInCenter(modifiedLine);
    diffEditor.getOriginalEditor().setPosition({ lineNumber: originalLine, column: 1 });
    diffEditor.getModifiedEditor().setPosition({ lineNumber: modifiedLine, column: 1 });
    applyDecorations();
  }, [activeBlockIndex, applyDecorations, diffBlocks, navigationToken]);

  useEffect(() => {
    let cancelled = false;
    setMonacoLoadError(null);
    import("@/lib/monaco-setup")
      .then(({ setupMonaco }) => {
        setupMonaco();
        if (!cancelled) setMonacoReady(true);
      })
      .catch((error) => {
        if (!cancelled) setMonacoLoadError(error);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAttempt]);

  const handleRetryLoad = useCallback(() => {
    setLoadAttempt((n) => n + 1);
  }, []);

  const handleMount = useCallback<DiffOnMount>(
    (editor, monaco) => {
      diffEditorRef.current = editor;
      monacoRef.current = monaco;
      window.setTimeout(() => {
        applyDecorations();
      }, 0);
    },
    [applyDecorations]
  );

  const showHeader = originalTitle || modifiedTitle || badge;
  const header = showHeader ? (
    <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3 border-b bg-muted/30 px-4 py-2 text-xs">
      <div className="min-w-0 truncate rounded bg-background px-2 py-1 font-medium text-muted-foreground">
        {originalTitle || ""}
      </div>
      {badge ? (
        <div className="rounded-full border border-border bg-background px-2 py-1 text-[10px] font-medium text-muted-foreground">
          {badge}
        </div>
      ) : (
        <div />
      )}
      <div className="min-w-0 truncate rounded bg-background px-2 py-1 text-right font-medium text-muted-foreground">
        {modifiedTitle || ""}
      </div>
    </div>
  ) : null;

  if (monacoLoadError) {
    const message = monacoLoadError instanceof Error ? monacoLoadError.message : String(monacoLoadError);
    return (
      <div
        className={`relative flex h-full w-full flex-col overflow-hidden rounded border bg-background shadow-inner ${className ?? ""}`}
      >
        {header}
        <div className="relative flex h-full w-full flex-col items-center justify-center gap-2 p-4 text-xs text-muted-foreground">
          <div className="text-destructive">{t("externalEdit.compare.loadFailed")}</div>
          <div className="font-mono text-[11px] opacity-70 max-w-full truncate">{message}</div>
          <button
            type="button"
            onClick={handleRetryLoad}
            className="px-2 py-1 text-xs rounded border border-border hover:bg-accent"
          >
            {t("action.retry")}
          </button>
        </div>
      </div>
    );
  }

  if (!monacoReady) {
    return (
      <div
        className={`relative flex h-full w-full flex-col overflow-hidden rounded border bg-background shadow-inner ${className ?? ""}`}
      >
        {header}
        <div className="relative h-full w-full" style={{ height }} />
      </div>
    );
  }

  return (
    <div
      className={`relative flex h-full w-full flex-col overflow-hidden rounded border bg-background shadow-inner ${className ?? ""}`}
    >
      {header}
      <DiffEditor
        height={typeof height === "number" ? `${height}px` : height}
        language={language}
        original={original}
        modified={modified}
        theme={resolvedTheme === "dark" ? "opskat-dark" : "opskat-light"}
        options={DEFAULT_OPTIONS}
        onMount={handleMount}
        wrapperProps={testId ? { "data-testid": testId } : undefined}
      />
    </div>
  );
}
