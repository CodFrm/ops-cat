import { memo, useEffect, useImperativeHandle, useMemo, useRef, forwardRef, type MutableRefObject } from "react";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import Document from "@tiptap/extension-document";
import HardBreak from "@tiptap/extension-hard-break";
import Paragraph from "@tiptap/extension-paragraph";
import Placeholder from "@tiptap/extension-placeholder";
import Text from "@tiptap/extension-text";
import { extractContentXml } from "./input/content";
import { createMentionExtension, createSnippetSuggestionExtension } from "./input/extensions";
import {
  applyInputHistoryMessage,
  getInputHistoryNavigationState,
  shouldContinueInputHistory,
  shouldIgnoreEditorShortcut,
  shouldStartInputHistory,
} from "./input/keyboard";
import { useInputDraftSync } from "./input/useInputDraft";
import type { AIChatInputDraft, AIChatInputHandle, ProseMirrorLikeNode } from "./input/types";

export type { AIChatInputDraft, AIChatInputHandle } from "./input/types";

export interface AIChatInputProps {
  onSubmit: (content: string) => void;
  onEmptyChange?: (empty: boolean) => void;
  onDraftChange?: (draft: AIChatInputDraft) => void;
  sendOnEnter: boolean;
  userMessageHistory?: string[];
  placeholder?: string;
  disabled?: boolean;
  /** 仅用于测试：暴露 TipTap editor 以便测试代码直接操作富文本。 */
  editorRef?: MutableRefObject<Editor | null>;
}

const AIChatInputComponent = forwardRef<AIChatInputHandle, AIChatInputProps>(function AIChatInput(
  { onSubmit, onEmptyChange, onDraftChange, sendOnEnter, userMessageHistory = [], placeholder, disabled, editorRef },
  ref
) {
  const submitRef = useRef(onSubmit);
  const sendOnEnterRef = useRef(sendOnEnter);
  const onEmptyChangeRef = useRef(onEmptyChange);
  const onDraftChangeRef = useRef(onDraftChange);
  const historyRef = useRef(userMessageHistory);
  const historyIndexRef = useRef(-1);
  const applyingHistoryRef = useRef(false);
  const lastIsEmptyRef = useRef<boolean | null>(null);
  const triggerSubmitRef = useRef<() => void>(() => {});
  const mentionActiveRef = useRef(false);
  const snippetSuggestionActiveRef = useRef(false);
  // The extension factory stores the ref for later suggestion callbacks; it does not read current during render.
  // eslint-disable-next-line react-hooks/refs
  const mentionExtension = useMemo(() => createMentionExtension(mentionActiveRef), []);
  // The extension factory stores the ref for later suggestion callbacks; it does not read current during render.
  // eslint-disable-next-line react-hooks/refs
  const snippetSuggestionExtension = useMemo(() => createSnippetSuggestionExtension(snippetSuggestionActiveRef), []);
  const { scheduleDraftFlush, clearDraftImmediately } = useInputDraftSync(onDraftChangeRef);

  useEffect(() => {
    submitRef.current = onSubmit;
  }, [onSubmit]);

  useEffect(() => {
    sendOnEnterRef.current = sendOnEnter;
  }, [sendOnEnter]);

  useEffect(() => {
    onEmptyChangeRef.current = onEmptyChange;
  }, [onEmptyChange]);

  useEffect(() => {
    onDraftChangeRef.current = onDraftChange;
  }, [onDraftChange]);

  useEffect(() => {
    historyRef.current = userMessageHistory;
    historyIndexRef.current = -1;
  }, [userMessageHistory]);

  const editor = useEditor({
    extensions: [
      Document,
      HardBreak,
      Paragraph,
      Text,
      Placeholder.configure({ placeholder: placeholder || "" }),
      mentionExtension,
      snippetSuggestionExtension,
    ],
    editorProps: {
      attributes: {
        class: "ProseMirror min-h-[3rem] max-h-[25vh] overflow-y-auto px-3 pt-3 pb-1 text-sm outline-none resize-none",
        role: "textbox",
      },
      handleKeyDown: (view, event) => {
        if (!editor) return false;
        if (shouldIgnoreEditorShortcut(view, event)) return false;

        const shouldSendOnEnter = sendOnEnterRef.current;
        const isEnter = event.key === "Enter";
        const mod = event.ctrlKey || event.metaKey;

        if (
          (event.key === "ArrowUp" || event.key === "ArrowDown") &&
          !event.altKey &&
          !event.ctrlKey &&
          !event.metaKey &&
          !event.shiftKey
        ) {
          const currentContent = extractContentXml(editor.state.doc as unknown as ProseMirrorLikeNode);
          const nextHistoryState = getInputHistoryNavigationState({
            direction: event.key === "ArrowUp" ? "up" : "down",
            currentText: currentContent,
            historyIndex: historyIndexRef.current,
            userMessageHistory: historyRef.current,
            canStartHistory: shouldStartInputHistory(editor),
            canContinueHistory: shouldContinueInputHistory(editor),
          });

          if (nextHistoryState) {
            event.preventDefault();
            historyIndexRef.current = nextHistoryState.nextHistoryIndex;
            applyingHistoryRef.current = true;
            applyInputHistoryMessage(editor, nextHistoryState.nextMessage);
            return true;
          }
        }

        const suggestionActive = mentionActiveRef.current || snippetSuggestionActiveRef.current;
        if (isEnter && suggestionActive) {
          return false;
        }
        if (isEnter && shouldSendOnEnter && !event.shiftKey && !mod) {
          event.preventDefault();
          triggerSubmitRef.current();
          return true;
        }
        if (isEnter && !shouldSendOnEnter && mod) {
          event.preventDefault();
          triggerSubmitRef.current();
          return true;
        }
        return false;
      },
    },
    onUpdate: ({ editor: ed }) => {
      if (applyingHistoryRef.current) {
        applyingHistoryRef.current = false;
      } else {
        historyIndexRef.current = -1;
      }

      const isEmpty = ed.isEmpty;
      if (lastIsEmptyRef.current !== isEmpty) {
        lastIsEmptyRef.current = isEmpty;
        onEmptyChangeRef.current?.(isEmpty);
      }

      scheduleDraftFlush(ed);
    },
    editable: !disabled,
  });

  useEffect(() => {
    if (editorRef) editorRef.current = editor ?? null;
    return () => {
      if (editorRef) editorRef.current = null;
    };
  }, [editor, editorRef]);

  useEffect(() => {
    triggerSubmitRef.current = () => {
      if (!editor || disabled || editor.view.composing || editor.isEmpty) return;
      const content = extractContentXml(editor.state.doc as unknown as ProseMirrorLikeNode);
      if (!content.trim()) return;

      historyIndexRef.current = -1;
      clearDraftImmediately();
      submitRef.current(content);
      editor.commands.clearContent(true);
    };
  }, [clearDraftImmediately, disabled, editor]);

  useEffect(() => {
    editor?.setEditable(!disabled);
  }, [editor, disabled]);

  useImperativeHandle(
    ref,
    () => ({
      focus: () => editor?.commands.focus(),
      clear: () => {
        historyIndexRef.current = -1;
        clearDraftImmediately();
        editor?.commands.clearContent(true);
      },
      isEmpty: () => editor?.isEmpty ?? true,
      submit: () => triggerSubmitRef.current(),
      loadDraft: (draft) => {
        if (!editor) return;
        historyIndexRef.current = -1;
        applyInputHistoryMessage(editor, draft);
      },
    }),
    [clearDraftImmediately, editor]
  );

  return <EditorContent editor={editor} />;
});

export const AIChatInput = memo(AIChatInputComponent) as typeof AIChatInputComponent;
