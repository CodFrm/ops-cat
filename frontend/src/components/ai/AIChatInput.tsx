import { useEffect, useRef, useImperativeHandle, forwardRef, type MutableRefObject } from "react";
import { EditorContent, useEditor, ReactRenderer, type Editor } from "@tiptap/react";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import Placeholder from "@tiptap/extension-placeholder";
import Mention from "@tiptap/extension-mention";
import type { SuggestionProps, SuggestionKeyDownProps } from "@tiptap/suggestion";
import tippy, { type Instance } from "tippy.js";
import { MentionList, type MentionListRef, type MentionItem } from "./MentionList";
import type { MentionRef } from "@/stores/aiStore";

export interface AIChatInputHandle {
  focus: () => void;
  clear: () => void;
  isEmpty: () => boolean;
  submit: () => void;
}

export interface AIChatInputProps {
  onSubmit: (text: string, mentions: MentionRef[]) => void;
  onEmptyChange?: (empty: boolean) => void;
  sendOnEnter: boolean;
  placeholder?: string;
  disabled?: boolean;
  /** 仅用于测试：暴露 TipTap editor 以便测试代码直接操作富文本。 */
  editorRef?: MutableRefObject<Editor | null>;
}

interface ProseMirrorLikeNode {
  type: { name: string };
  text?: string;
  attrs: Record<string, unknown>;
  descendants: (fn: (node: ProseMirrorLikeNode) => boolean | void) => void;
}

/** 从 TipTap doc 提取纯文本 + mention 引用。 */
function extractTextAndMentions(doc: ProseMirrorLikeNode): {
  text: string;
  mentions: MentionRef[];
} {
  let text = "";
  const mentions: MentionRef[] = [];
  doc.descendants((node) => {
    if (node.type.name === "text") {
      text += node.text ?? "";
    } else if (node.type.name === "mention") {
      const id = Number(node.attrs.id);
      const label = String(node.attrs.label ?? "");
      const start = text.length;
      text += `@${label}`;
      mentions.push({ assetId: id, name: label, start, end: text.length });
    } else if (node.type.name === "paragraph" && text.length > 0) {
      text += "\n";
    }
    return true;
  });
  return { text: text.replace(/\n+$/g, ""), mentions };
}

export const AIChatInput = forwardRef<AIChatInputHandle, AIChatInputProps>(function AIChatInput(
  { onSubmit, onEmptyChange, sendOnEnter, placeholder, disabled, editorRef },
  ref
) {
  const submitRef = useRef(onSubmit);
  const sendOnEnterRef = useRef(sendOnEnter);
  const onEmptyChangeRef = useRef(onEmptyChange);

  useEffect(() => {
    submitRef.current = onSubmit;
  }, [onSubmit]);
  useEffect(() => {
    sendOnEnterRef.current = sendOnEnter;
  }, [sendOnEnter]);
  useEffect(() => {
    onEmptyChangeRef.current = onEmptyChange;
  }, [onEmptyChange]);

  const triggerSubmitRef = useRef<() => void>(() => {});

  const editor = useEditor({
    extensions: [
      Document,
      Paragraph,
      Text,
      Placeholder.configure({ placeholder: placeholder || "" }),
      Mention.configure({
        HTMLAttributes: {
          class:
            "ai-mention inline-flex items-center rounded bg-primary/10 text-primary px-1 py-0.5 text-xs font-medium",
        },
        renderLabel: ({ node }) => `@${node.attrs.label}`,
        suggestion: {
          items: () => [] as MentionItem[],
          render: () => {
            let component: ReactRenderer<MentionListRef> | null = null;
            let popup: Instance[] = [];
            const makeProps = (p: SuggestionProps<MentionItem>) => ({
              query: p.query,
              command: (item: MentionItem) => {
                // TipTap Mention 节点属性类型是 { id: string; label: string }，
                // 这里将资产 ID 转为字符串以对齐扩展类型约束。
                p.command({ id: String(item.id), label: item.label } as unknown as MentionItem);
              },
            });
            return {
              onStart: (props: SuggestionProps<MentionItem>) => {
                component = new ReactRenderer(MentionList, {
                  props: makeProps(props),
                  editor: props.editor,
                });
                if (!props.clientRect) return;
                popup = tippy("body", {
                  getReferenceClientRect: props.clientRect as () => DOMRect,
                  appendTo: () => document.body,
                  content: component.element,
                  showOnCreate: true,
                  interactive: true,
                  trigger: "manual",
                  placement: "bottom-start",
                });
              },
              onUpdate: (props: SuggestionProps<MentionItem>) => {
                component?.updateProps(makeProps(props));
                if (popup[0] && props.clientRect) {
                  popup[0].setProps({ getReferenceClientRect: props.clientRect as () => DOMRect });
                }
              },
              onKeyDown: (props: SuggestionKeyDownProps) => {
                if (props.event.key === "Escape") {
                  popup[0]?.hide();
                  return true;
                }
                return component?.ref?.onKeyDown(props) || false;
              },
              onExit: () => {
                popup[0]?.destroy();
                component?.destroy();
              },
            };
          },
        },
      }),
    ],
    editorProps: {
      attributes: {
        class: "ProseMirror min-h-[3rem] max-h-[25vh] overflow-y-auto px-3 pt-3 pb-1 text-sm outline-none resize-none",
        role: "textbox",
      },
      handleKeyDown: (_view, event) => {
        const shouldSendOnEnter = sendOnEnterRef.current;
        const isEnter = event.key === "Enter";
        const mod = event.ctrlKey || event.metaKey;
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
      onEmptyChangeRef.current?.(ed.isEmpty);
    },
    editable: !disabled,
  });

  // 在 effect 中更新可选的外部 editor 引用，避免在渲染期间写入 ref。
  useEffect(() => {
    if (editorRef) editorRef.current = editor ?? null;
    return () => {
      if (editorRef) editorRef.current = null;
    };
  }, [editor, editorRef]);

  // 把 triggerSubmit 放到 effect 中以便捕获最新 editor 引用。
  useEffect(() => {
    triggerSubmitRef.current = () => {
      if (!editor) return;
      if (editor.isEmpty) return;
      const { text, mentions } = extractTextAndMentions(editor.state.doc as unknown as ProseMirrorLikeNode);
      if (!text.trim() && mentions.length === 0) return;
      submitRef.current(text, mentions);
      editor.commands.clearContent();
    };
  }, [editor]);

  useEffect(() => {
    editor?.setEditable(!disabled);
  }, [editor, disabled]);

  useImperativeHandle(
    ref,
    () => ({
      focus: () => editor?.commands.focus(),
      clear: () => editor?.commands.clearContent(),
      isEmpty: () => editor?.isEmpty ?? true,
      submit: () => triggerSubmitRef.current(),
    }),
    [editor]
  );

  return <EditorContent editor={editor} />;
});
