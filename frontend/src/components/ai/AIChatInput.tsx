import { useEffect, useMemo, useRef, useImperativeHandle, forwardRef, type MutableRefObject } from "react";
import { EditorContent, useEditor, ReactRenderer, type Editor } from "@tiptap/react";
import { Extension } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import Placeholder from "@tiptap/extension-placeholder";
import Mention from "@tiptap/extension-mention";
import Suggestion, { type SuggestionProps, type SuggestionKeyDownProps } from "@tiptap/suggestion";
import tippy, { type Instance } from "tippy.js";
import { MentionList, type MentionListRef, type MentionItem } from "./MentionList";
import {
  SnippetSuggestionList,
  type SnippetSuggestionItem,
  type SnippetSuggestionListRef,
} from "./SnippetSuggestionList";
import type { MentionRef } from "@/stores/aiStore";
import { useSnippetStore } from "@/stores/snippetStore";
import { ListSnippets } from "../../../wailsjs/go/app/App";
import { snippet_svc } from "../../../wailsjs/go/models";

export interface AIChatInputDraft {
  content: string;
  mentions?: MentionRef[];
}

export interface AIChatInputHandle {
  focus: () => void;
  clear: () => void;
  isEmpty: () => boolean;
  submit: () => void;
  loadDraft: (draft: string | AIChatInputDraft) => void;
}

export interface AIChatInputProps {
  onSubmit: (text: string, mentions: MentionRef[]) => void;
  onEmptyChange?: (empty: boolean) => void;
  onDraftChange?: (draft: AIChatInputDraft) => void;
  sendOnEnter: boolean;
  userMessageHistory?: string[];
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

interface TipTapTextNode {
  type: "text";
  text: string;
}

interface TipTapMentionNode {
  type: "mention";
  attrs: {
    id: string;
    label: string;
  };
}

interface TipTapParagraphNode {
  type: "paragraph";
  content?: Array<TipTapTextNode | TipTapMentionNode>;
}

interface TipTapDocNode {
  type: "doc";
  content: TipTapParagraphNode[];
}

type InputHistoryDirection = "up" | "down";

interface InputHistoryNavigationOptions {
  direction: InputHistoryDirection;
  currentText: string;
  historyIndex: number;
  userMessageHistory: string[];
  canStartHistory: boolean;
  canContinueHistory: boolean;
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

function normalizeDraftMessage(draft: string | AIChatInputDraft): AIChatInputDraft {
  if (typeof draft === "string") {
    return { content: draft, mentions: [] };
  }
  return {
    content: draft.content ?? "",
    mentions: draft.mentions ?? [],
  };
}

function appendTextToParagraphs(
  paragraphs: TipTapParagraphNode[],
  text: string,
  currentParagraphContent: Array<TipTapTextNode | TipTapMentionNode>
) {
  const segments = text.split("\n");
  for (let index = 0; index < segments.length; index += 1) {
    const segment = segments[index];
    if (segment.length > 0) {
      currentParagraphContent.push({ type: "text", text: segment });
    }
    if (index < segments.length - 1) {
      paragraphs.push(
        currentParagraphContent.length > 0
          ? { type: "paragraph", content: currentParagraphContent }
          : { type: "paragraph" }
      );
      currentParagraphContent = [];
    }
  }
  return currentParagraphContent;
}

// 统一把持久化 user message（content + mentions）重建为 TipTap 文档，
// 供历史浏览与外部 draft 预填共用，避免两套回填逻辑出现偏差。
function buildEditorDocFromMessage(message: string | AIChatInputDraft): TipTapDocNode {
  const draft = normalizeDraftMessage(message);
  const content = draft.content ?? "";
  const mentions = [...(draft.mentions ?? [])].sort((a, b) => a.start - b.start);
  const paragraphs: TipTapParagraphNode[] = [];
  let currentParagraphContent: Array<TipTapTextNode | TipTapMentionNode> = [];
  let cursor = 0;

  for (const mention of mentions) {
    const start = Math.max(0, Math.min(mention.start, content.length));
    const end = Math.max(start, Math.min(mention.end, content.length));
    if (start < cursor || end <= start) {
      continue;
    }

    if (start > cursor) {
      currentParagraphContent = appendTextToParagraphs(
        paragraphs,
        content.slice(cursor, start),
        currentParagraphContent
      );
    }

    const mentionSlice = content.slice(start, end);
    if (mentionSlice.length === 0 || mentionSlice.includes("\n")) {
      currentParagraphContent = appendTextToParagraphs(paragraphs, mentionSlice, currentParagraphContent);
      cursor = end;
      continue;
    }

    const labelFromContent = mentionSlice.startsWith("@") ? mentionSlice.slice(1) : mentionSlice;
    const label = labelFromContent || mention.name;
    currentParagraphContent.push({
      type: "mention",
      attrs: {
        id: String(mention.assetId),
        label,
      },
    });
    cursor = end;
  }

  if (cursor < content.length) {
    currentParagraphContent = appendTextToParagraphs(paragraphs, content.slice(cursor), currentParagraphContent);
  }

  paragraphs.push(
    currentParagraphContent.length > 0 ? { type: "paragraph", content: currentParagraphContent } : { type: "paragraph" }
  );

  return {
    type: "doc",
    content: paragraphs,
  };
}

/** First non-empty line of snippet content, used as the popup preview line. */
function firstLine(s: string): string {
  const i = s.indexOf("\n");
  return i === -1 ? s.trim() : s.slice(0, i).trim();
}

/**
 * Build a ProseMirror plugin that triggers on `/` and inserts the picked snippet
 * as PLAIN TEXT (not a Mention node) — prompt snippets are just template text.
 *
 * The factory takes the component-local `activeRef` so the editor's Enter-handler
 * can yield to the popup (mirroring how the `@` mention suggestion works).
 */
function createSnippetSuggestionExtension(activeRef: MutableRefObject<boolean>) {
  return Extension.create({
    name: "snippetSuggestion",
    addProseMirrorPlugins() {
      // Shared closure: items() writes the unfiltered count here, render() reads
      // it when building props so the list can distinguish "filter zeroed out"
      // from "no prompt snippets at all". Avoids a separate fetch AND the earlier
      // item-stamping hack where an empty `items` array lost the total.
      let lastTotal = 0;
      return [
        Suggestion<SnippetSuggestionItem>({
          editor: this.editor,
          char: "/",
          // Rely on TipTap's default `allowedPrefixes: [" "]` — it already
          // enforces "doc-start or after whitespace" (that's why `http:/`
          // doesn't trigger — `:` is not in allowedPrefixes). A custom `allow`
          // here had an off-by-one and blocked legitimate "after space" cases
          // like `hello /`.
          command: ({ editor, range, props }) => {
            editor.chain().focus().deleteRange(range).insertContent(props.content).run();
            useSnippetStore.getState().recordUse(props.id);
          },
          items: async ({ query }) => {
            try {
              // NOTE: keyword is intentionally left blank and filtering is done client-side
              // so we can keep the unfiltered `lastTotal` — SnippetSuggestionList needs it to
              // tell "no prompts exist" (CTA) apart from "filter zeroed out" (no-match state).
              const req = new snippet_svc.ListReq({
                categories: ["prompt"],
                keyword: "",
                limit: 0,
                offset: 0,
                orderBy: "",
              });
              const all = await ListSnippets(req);
              const list: SnippetSuggestionItem[] = (all ?? []).map((s) => ({
                id: s.ID,
                name: s.Name,
                preview: firstLine(s.Content ?? "").slice(0, 80),
                content: s.Content ?? "",
                readOnly: (s.Source ?? "").startsWith("ext:"),
              }));
              lastTotal = list.length;
              const q = query.toLowerCase();
              const filtered = q
                ? list.filter((i) => i.name.toLowerCase().includes(q) || i.preview.toLowerCase().includes(q))
                : list;
              return filtered.slice(0, 20);
            } catch {
              return [];
            }
          },
          render: () => {
            let component: ReactRenderer<SnippetSuggestionListRef> | null = null;
            let popup: Instance[] = [];
            const buildProps = (p: SuggestionProps<SnippetSuggestionItem>) => ({
              items: p.items,
              totalAvailable: lastTotal,
              command: p.command,
            });
            return {
              onStart: (props) => {
                activeRef.current = true;
                component = new ReactRenderer(SnippetSuggestionList, {
                  props: buildProps(props),
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
              onUpdate: (props) => {
                component?.updateProps(buildProps(props));
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
                activeRef.current = false;
                popup[0]?.destroy();
                component?.destroy();
              },
            };
          },
        }),
      ];
    },
  });
}

// 仅在首行首字符接管向上历史，避免影响富文本输入的原生光标移动。
function shouldStartInputHistory(editor: Editor) {
  const { selection } = editor.state;
  return selection.empty && selection.from === 1;
}

// 进入历史浏览后，只要还是折叠光标就允许继续用上下键切换。
function shouldContinueInputHistory(editor: Editor) {
  return editor.state.selection.empty;
}

// 统一计算上下键历史切换的目标内容，避免把判断分散在按键处理中。
function getInputHistoryNavigationState({
  direction,
  currentText,
  historyIndex,
  userMessageHistory,
  canStartHistory,
  canContinueHistory,
}: InputHistoryNavigationOptions) {
  const currentHistoryMessage = historyIndex >= 0 ? userMessageHistory[historyIndex] : null;
  const isBrowsingHistory = currentHistoryMessage != null && currentText === currentHistoryMessage;
  const canNavigate = isBrowsingHistory ? canContinueHistory : canStartHistory;

  if (!canNavigate) return null;
  if (direction === "up" && userMessageHistory.length === 0) return null;
  if (direction === "down" && (!isBrowsingHistory || historyIndex < 0)) return null;

  const nextHistoryIndex =
    direction === "up" ? Math.min(historyIndex + 1, userMessageHistory.length - 1) : historyIndex - 1;
  const nextMessage = nextHistoryIndex >= 0 ? userMessageHistory[nextHistoryIndex] : "";

  return { nextHistoryIndex, nextMessage };
}

// 把历史消息写回编辑器，并把光标定位到末尾，保证连续切换时体验稳定。
function applyInputHistoryMessage(editor: Editor, nextMessage: string | AIChatInputDraft) {
  editor.commands.setContent(buildEditorDocFromMessage(nextMessage));
  editor.commands.focus("end");
}

export const AIChatInput = forwardRef<AIChatInputHandle, AIChatInputProps>(function AIChatInput(
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
    // 历史列表变化（切换会话、新消息到达等）时复位浏览游标，避免旧 index 落到错位条目。
    historyRef.current = userMessageHistory;
    historyIndexRef.current = -1;
  }, [userMessageHistory]);

  const triggerSubmitRef = useRef<() => void>(() => {});
  // @ 提及弹窗是否处于激活状态。ProseMirror 会先调用 editorProps.handleKeyDown
  // 再分发给插件，所以需要在此处主动让路，避免 Enter 直接触发发送。
  const mentionActiveRef = useRef(false);
  // `/` 片段弹窗的激活态；语义同 mentionActiveRef，两者互不冲突（不同插件）。
  const snippetSuggestionActiveRef = useRef(false);
  // Stable extension instance — `useMemo([])` ensures the plugin (and its tippy
  // lifecycle state) is created exactly once per mount, not rebuilt on each render.
  // The ref is only *stored* by the extension (mutated/read later in suggestion
  // callbacks), never read during render; the rule can't detect that.
  // eslint-disable-next-line react-hooks/refs
  const snippetSuggestionExtension = useMemo(() => createSnippetSuggestionExtension(snippetSuggestionActiveRef), []);

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
                mentionActiveRef.current = true;
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
                mentionActiveRef.current = false;
                popup[0]?.destroy();
                component?.destroy();
              },
            };
          },
        },
      }),
      snippetSuggestionExtension,
    ],
    editorProps: {
      attributes: {
        class: "ProseMirror min-h-[3rem] max-h-[25vh] overflow-y-auto px-3 pt-3 pb-1 text-sm outline-none resize-none",
        role: "textbox",
      },
      handleKeyDown: (_view, event) => {
        if (!editor) return false;

        const shouldSendOnEnter = sendOnEnterRef.current;
        const isEnter = event.key === "Enter";
        const mod = event.ctrlKey || event.metaKey;

        // 在允许的光标位置接管上下键，统一处理用户消息历史切换。
        if (
          (event.key === "ArrowUp" || event.key === "ArrowDown") &&
          !event.altKey &&
          !event.ctrlKey &&
          !event.metaKey &&
          !event.shiftKey
        ) {
          const { text: currentText } = extractTextAndMentions(editor.state.doc as unknown as ProseMirrorLikeNode);
          const nextHistoryState = getInputHistoryNavigationState({
            direction: event.key === "ArrowUp" ? "up" : "down",
            currentText,
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

        // 提及 / 片段弹窗激活时，把 Enter 让给 suggestion 插件用于选中候选项
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
      onEmptyChangeRef.current?.(ed.isEmpty);
      const { text, mentions } = extractTextAndMentions(ed.state.doc as unknown as ProseMirrorLikeNode);
      onDraftChangeRef.current?.({ content: text, mentions });
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
      historyIndexRef.current = -1;
      submitRef.current(text, mentions);
      // emitUpdate=true：默认 false 时 onUpdate 不会触发，外部 inputDraft 仍保留旧内容；
      // 侧边助手随后创建会话、conversationId 变化时会按草稿把刚发送的消息回填到输入框。
      editor.commands.clearContent(true);
    };
  }, [editor]);

  useEffect(() => {
    editor?.setEditable(!disabled);
  }, [editor, disabled]);

  useImperativeHandle(
    ref,
    () => ({
      focus: () => editor?.commands.focus(),
      clear: () => {
        historyIndexRef.current = -1;
        editor?.commands.clearContent();
      },
      isEmpty: () => editor?.isEmpty ?? true,
      submit: () => triggerSubmitRef.current(),
      loadDraft: (draft) => {
        if (!editor) return;
        historyIndexRef.current = -1;
        applyInputHistoryMessage(editor, draft);
      },
    }),
    [editor]
  );

  return <EditorContent editor={editor} />;
});
