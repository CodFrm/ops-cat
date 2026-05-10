import { useCallback, useEffect, useState } from "react";
import { DiffEditor } from "@monaco-editor/react";
import type * as MonacoNS from "monaco-editor";
import { useResolvedTheme } from "./theme-provider";
import type { CodeEditorLanguage } from "./CodeEditor";

export interface CodeDiffViewerProps {
  original: string;
  modified: string;
  language?: CodeEditorLanguage;
  height?: string | number;
  className?: string;
}

const DEFAULT_OPTIONS: MonacoNS.editor.IDiffEditorConstructionOptions = {
  automaticLayout: true,
  renderSideBySide: true,
  readOnly: true,
  originalEditable: false,
  renderOverviewRuler: false,
  scrollBeyondLastLine: false,
  fixedOverflowWidgets: true,
  contextmenu: true,
  minimap: { enabled: false },
  diffWordWrap: "on",
  wordWrap: "on",
  scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
};

export function CodeDiffViewer({
  original,
  modified,
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

  if (monacoLoadError) {
    const message = monacoLoadError instanceof Error ? monacoLoadError.message : String(monacoLoadError);
    return (
      <div
        className={`relative h-full w-full flex flex-col items-center justify-center gap-2 p-4 text-xs text-muted-foreground ${className ?? ""}`}
        style={{ height }}
      >
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
    );
  }

  if (!monacoReady) {
    return <div className={`relative h-full w-full ${className ?? ""}`} style={{ height }} />;
  }

  return (
    <div className={`relative h-full w-full ${className ?? ""}`}>
      <DiffEditor
        height={height}
        language={language}
        original={original}
        modified={modified}
        theme={resolvedTheme === "dark" ? "opskat-dark" : "opskat-light"}
        options={DEFAULT_OPTIONS}
      />
    </div>
  );
}
