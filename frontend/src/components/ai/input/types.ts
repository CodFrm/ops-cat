import type { Editor } from "@tiptap/react";

export interface AIChatInputDraft {
  content: string;
}

export interface AIChatInputHandle {
  focus: () => void;
  clear: () => void;
  isEmpty: () => boolean;
  submit: () => void;
  loadDraft: (draft: string | AIChatInputDraft) => void;
}

export interface ProseMirrorLikeNode {
  type: { name: string };
  text?: string;
  attrs: Record<string, unknown>;
  descendants: (fn: (node: ProseMirrorLikeNode) => boolean | void) => void;
}

export interface TipTapTextNode {
  type: "text";
  text: string;
}

export interface TipTapMentionNode {
  type: "mention";
  attrs: {
    id: string;
    label: string;
    kind?: "asset" | "database" | "table";
    database?: string;
    table?: string;
    driver?: string;
  };
}

export interface TipTapParagraphNode {
  type: "paragraph";
  content?: Array<TipTapTextNode | TipTapMentionNode>;
}

export interface TipTapDocNode {
  type: "doc";
  content: TipTapParagraphNode[];
}

export type InputHistoryDirection = "up" | "down";

export interface InputHistoryNavigationOptions {
  direction: InputHistoryDirection;
  currentText: string;
  historyIndex: number;
  userMessageHistory: string[];
  canStartHistory: boolean;
  canContinueHistory: boolean;
}

export type TipTapEditor = Editor;
