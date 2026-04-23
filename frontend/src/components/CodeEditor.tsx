import { useCallback, useEffect, useRef, useState } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type * as MonacoNS from "monaco-editor";
import { useResolvedTheme } from "./theme-provider";
import {
  registerDynamicCompletions,
  unregisterDynamicCompletions,
  type DynamicCompletionGetter,
} from "@/lib/monaco-completions";

export type CodeEditorLanguage = "sql" | "javascript" | "json" | "plaintext";

export interface CodeEditorProps {
  /** 受控：父组件拥有文本 state，每次变化通过 onChange 回吐 */
  value?: string;
  /** 非受控：仅在挂载时注入初始值，后续由编辑器自管；通过 onMount/editor ref 读写 */
  defaultValue?: string;
  onChange?: (value: string) => void;
  language?: CodeEditorLanguage;
  readOnly?: boolean;
  /** 高度 css 值，默认 "100%"（占满父容器，父容器需有显式高度） */
  height?: string | number;
  /** 字号 px，默认 12（与项目现有 text-xs 一致） */
  fontSize?: number;
  /** value 为空时显示的提示文案。Monaco 无原生 placeholder，由外层 div 绘制 */
  placeholder?: string;
  /** 透传给 monaco 的 IEditorOptions，会与默认值浅合并 */
  options?: MonacoNS.editor.IStandaloneEditorConstructionOptions;
  /** 拿到 editor + monaco 实例，用来注册快捷键、读取选区等 */
  onMount?: OnMount;
  /**
   * 为本 editor 实例追加业务级补全（例如当前库下的表名、collection 名）。
   * 函数会在每次触发补全时被调用，因此可以闭包读最新业务状态；当依赖变化时
   * 重传新函数即可（CodeEditor 内部用 ref 桥接）。卸载时自动注销。
   */
  dynamicCompletions?: DynamicCompletionGetter;
  className?: string;
}

const DEFAULT_OPTIONS: MonacoNS.editor.IStandaloneEditorConstructionOptions = {
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  automaticLayout: true,
  wordWrap: "on",
  tabSize: 2,
  insertSpaces: true,
  renderLineHighlight: "line",
  scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
  fixedOverflowWidgets: true,
  contextmenu: false,
  smoothScrolling: true,
  // 补全 / 智能提示。回车默认换行，仅当意图明确时接受候选；触发只靠用户主动输入字符 / Ctrl+Space
  quickSuggestions: { other: true, comments: false, strings: false },
  suggestOnTriggerCharacters: true,
  acceptSuggestionOnEnter: "smart",
  tabCompletion: "on",
  wordBasedSuggestions: "currentDocument",
  parameterHints: { enabled: true },
  suggest: { showWords: true, showSnippets: false, showKeywords: true },
  // 与终端/表格视觉风格一致
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
};

export function CodeEditor({
  value,
  defaultValue,
  onChange,
  language = "sql",
  readOnly = false,
  height = "100%",
  fontSize = 12,
  placeholder,
  options,
  onMount,
  dynamicCompletions,
  className,
}: CodeEditorProps) {
  const isControlled = value !== undefined;
  const resolvedTheme = useResolvedTheme();
  const editorRef = useRef<MonacoNS.editor.IStandaloneCodeEditor | null>(null);
  const modelUriRef = useRef<string | null>(null);

  // 用 ref 桥接最新的 dynamicCompletions，避免 prop 变化时 re-mount editor
  const dynamicRef = useRef<DynamicCompletionGetter | undefined>(dynamicCompletions);
  useEffect(() => {
    dynamicRef.current = dynamicCompletions;
  }, [dynamicCompletions]);

  const handleMount = useCallback<OnMount>(
    (editor, monaco) => {
      editorRef.current = editor;
      const uri = editor.getModel()?.uri.toString() ?? null;
      modelUriRef.current = uri;
      if (uri) {
        registerDynamicCompletions(uri, (ctx) => dynamicRef.current?.(ctx) ?? []);
      }
      onMount?.(editor, monaco);
    },
    [onMount]
  );

  // 卸载时注销动态补全
  useEffect(() => {
    return () => {
      const uri = modelUriRef.current;
      if (uri) unregisterDynamicCompletions(uri);
    };
  }, []);

  // readOnly 切换需要主动同步给已挂载的 editor
  useEffect(() => {
    editorRef.current?.updateOptions({ readOnly });
  }, [readOnly]);

  // 非受控时，占位符是否显示不跟 React state 走 —— 由 mount 后的 editor 内容决定
  const [uncontrolledIsEmpty, setUncontrolledIsEmpty] = useState(() => !defaultValue);
  const showPlaceholder = !!placeholder && (isControlled ? !value : uncontrolledIsEmpty);

  const handleMountWithPlaceholder = useCallback<OnMount>(
    (editor, monaco) => {
      handleMount(editor, monaco);
      if (!isControlled && placeholder) {
        const model = editor.getModel();
        if (model) {
          // 仅在空/非空跨越时才 setState，避免每个按键一次 React 重渲
          let prevEmpty = model.getValue().length === 0;
          setUncontrolledIsEmpty(prevEmpty);
          model.onDidChangeContent(() => {
            const nowEmpty = model.getValue().length === 0;
            if (nowEmpty !== prevEmpty) {
              prevEmpty = nowEmpty;
              setUncontrolledIsEmpty(nowEmpty);
            }
          });
        }
      }
    },
    [handleMount, isControlled, placeholder]
  );

  return (
    <div className={`relative h-full w-full ${className ?? ""}`}>
      <Editor
        height={height}
        language={language}
        {...(isControlled ? { value } : { defaultValue: defaultValue ?? "" })}
        theme={resolvedTheme === "dark" ? "opskat-dark" : "opskat-light"}
        onChange={onChange ? (v) => onChange(v ?? "") : undefined}
        onMount={handleMountWithPlaceholder}
        options={{
          ...DEFAULT_OPTIONS,
          ...options,
          fontSize,
          readOnly,
        }}
      />
      {showPlaceholder && (
        <div className="pointer-events-none absolute left-[60px] top-[2px] text-xs text-muted-foreground/60 font-mono">
          {placeholder}
        </div>
      )}
    </div>
  );
}
