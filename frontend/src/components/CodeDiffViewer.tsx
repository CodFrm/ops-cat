import { useCallback, useEffect, useState } from "react";
import { DiffEditor } from "@monaco-editor/react";
import type * as MonacoNS from "monaco-editor";
import { useResolvedTheme } from "./theme-provider";
import type { CodeEditorLanguage } from "./CodeEditor";

export interface CodeDiffViewerProps {
  original: string;
  modified: string;
  originalTitle?: string;
  modifiedTitle?: string;
  badge?: string;
  language?: CodeEditorLanguage;
  height?: string | number;
  className?: string;
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
  renderOverviewRuler: false,
  scrollBeyondLastLine: false,
  fixedOverflowWidgets: true,
  contextmenu: true,
  minimap: { enabled: false },
  diffWordWrap: "on",
  wordWrap: "on",
  scrollbar: { alwaysConsumeMouseWheel: false, verticalScrollbarSize: 10, horizontalScrollbarSize: 10 },
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
};

export function CodeDiffViewer({
  original,
  modified,
  originalTitle,
  modifiedTitle,
  badge,
  language = "plaintext",
  height = "100%",
  className,
}: CodeDiffViewerProps) {
  const resolvedTheme = useResolvedTheme();
  const [monacoReady, setMonacoReady] = useState(false);
  const [monacoLoadError, setMonacoLoadError] = useState<unknown>(null);
  const [loadAttempt, setLoadAttempt] = useState(0);

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
          <div className="text-destructive">对比视图加载失败</div>
          <div className="font-mono text-[11px] opacity-70 max-w-full truncate">{message}</div>
          <button
            type="button"
            onClick={handleRetryLoad}
            className="px-2 py-1 text-xs rounded border border-border hover:bg-accent"
          >
            重试
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
      />
    </div>
  );
}
