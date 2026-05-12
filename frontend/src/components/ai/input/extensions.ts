import type { MutableRefObject } from "react";
import { Extension } from "@tiptap/core";
import Mention from "@tiptap/extension-mention";
import { ReactRenderer } from "@tiptap/react";
import Suggestion, { type SuggestionKeyDownProps, type SuggestionProps } from "@tiptap/suggestion";
import tippy, { type Instance } from "tippy.js";
import { useSnippetStore } from "@/stores/snippetStore";
import { ListSnippets } from "../../../../wailsjs/go/app/App";
import { snippet_svc } from "../../../../wailsjs/go/models";
import { MentionList, type MentionItem, type MentionListRef } from "../MentionList";
import {
  SnippetSuggestionList,
  type SnippetSuggestionItem,
  type SnippetSuggestionListRef,
} from "../SnippetSuggestionList";

function firstLine(s: string): string {
  const index = s.indexOf("\n");
  return index === -1 ? s.trim() : s.slice(0, index).trim();
}

function createClientRect(left: number, top: number, right: number, bottom: number): DOMRect {
  const width = Math.max(0, right - left);
  const height = Math.max(0, bottom - top);
  return {
    x: left,
    y: top,
    width,
    height,
    left,
    top,
    right,
    bottom,
    toJSON: () => ({ x: left, y: top, width, height, left, top, right, bottom }),
  } as DOMRect;
}

function fallbackSuggestionClientRect(props: SuggestionProps<unknown>): DOMRect {
  const docSize = props.editor.state.doc.content.size;
  const from = Math.max(0, Math.min(props.range.from, docSize));
  const to = Math.max(from, Math.min(props.range.to, docSize));

  try {
    const start = props.editor.view.coordsAtPos(from);
    const end = props.editor.view.coordsAtPos(to);
    return createClientRect(
      Math.min(start.left, end.left),
      Math.min(start.top, end.top),
      Math.max(start.right, end.right),
      Math.max(start.bottom, end.bottom)
    );
  } catch {
    return createClientRect(0, 0, 0, 0);
  }
}

function suggestionReferenceClientRect<TItem>(props: SuggestionProps<TItem>) {
  return () => props.clientRect?.() ?? fallbackSuggestionClientRect(props as SuggestionProps<unknown>);
}

function createSuggestionPopup<TItem>(props: SuggestionProps<TItem>, content: Element): Instance[] {
  return tippy("body", {
    getReferenceClientRect: suggestionReferenceClientRect(props),
    appendTo: () => document.body,
    content,
    showOnCreate: true,
    interactive: true,
    trigger: "manual",
    placement: "bottom-start",
  });
}

export function createMentionExtension(activeRef: MutableRefObject<boolean>) {
  return Mention.configure({
    HTMLAttributes: {
      class: "ai-mention inline-flex items-center rounded bg-primary/10 text-primary px-1 py-0.5 text-xs font-medium",
    },
    renderLabel: ({ node }) => `@${node.attrs.label}`,
    suggestion: {
      items: () => [] as MentionItem[],
      render: () => {
        let component: ReactRenderer<MentionListRef> | null = null;
        let popup: Instance[] = [];
        const makeProps = (props: SuggestionProps<MentionItem>) => ({
          query: props.query,
          command: (item: MentionItem) => {
            props.command({ id: String(item.id), label: item.label } as unknown as MentionItem);
          },
        });
        return {
          onStart: (props: SuggestionProps<MentionItem>) => {
            activeRef.current = true;
            component = new ReactRenderer(MentionList, {
              props: makeProps(props),
              editor: props.editor,
            });
            popup = createSuggestionPopup(props, component.element);
          },
          onUpdate: (props: SuggestionProps<MentionItem>) => {
            component?.updateProps(makeProps(props));
            if (popup[0]) {
              popup[0].setProps({ getReferenceClientRect: suggestionReferenceClientRect(props) });
            } else if (component) {
              popup = createSuggestionPopup(props, component.element);
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
    },
  });
}

export function createSnippetSuggestionExtension(activeRef: MutableRefObject<boolean>) {
  return Extension.create({
    name: "snippetSuggestion",
    addProseMirrorPlugins() {
      let lastTotal = 0;
      return [
        Suggestion<SnippetSuggestionItem>({
          editor: this.editor,
          char: "/",
          command: ({ editor, range, props }) => {
            editor.chain().focus().deleteRange(range).insertContent(props.content).run();
            useSnippetStore.getState().recordUse(props.id);
          },
          items: async ({ query }) => {
            try {
              const req = new snippet_svc.ListReq({
                categories: ["prompt"],
                keyword: "",
                limit: 0,
                offset: 0,
                orderBy: "",
              });
              const all = await ListSnippets(req);
              const list: SnippetSuggestionItem[] = (all ?? []).map((snippet) => ({
                id: snippet.ID,
                name: snippet.Name,
                preview: firstLine(snippet.Content ?? "").slice(0, 80),
                content: snippet.Content ?? "",
                readOnly: (snippet.Source ?? "").startsWith("ext:"),
              }));
              lastTotal = list.length;
              const q = query.toLowerCase();
              const filtered = q
                ? list.filter((item) => item.name.toLowerCase().includes(q) || item.preview.toLowerCase().includes(q))
                : list;
              return filtered.slice(0, 20);
            } catch {
              return [];
            }
          },
          render: () => {
            let component: ReactRenderer<SnippetSuggestionListRef> | null = null;
            let popup: Instance[] = [];
            const buildProps = (props: SuggestionProps<SnippetSuggestionItem>) => ({
              items: props.items,
              totalAvailable: lastTotal,
              command: props.command,
            });
            return {
              onStart: (props) => {
                activeRef.current = true;
                component = new ReactRenderer(SnippetSuggestionList, {
                  props: buildProps(props),
                  editor: props.editor,
                });
                popup = createSuggestionPopup(props, component.element);
              },
              onUpdate: (props) => {
                component?.updateProps(buildProps(props));
                if (popup[0]) {
                  popup[0].setProps({ getReferenceClientRect: suggestionReferenceClientRect(props) });
                } else if (component) {
                  popup = createSuggestionPopup(props, component.element);
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
