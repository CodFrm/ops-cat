import type { Editor } from "@tiptap/react";
import type { EditorView } from "@tiptap/pm/view";
import { buildEditorDocFromMessage } from "./content";
import type { AIChatInputDraft, InputHistoryNavigationOptions } from "./types";

export function shouldIgnoreEditorShortcut(view: EditorView, event: KeyboardEvent): boolean {
  return view.composing || event.isComposing || event.keyCode === 229;
}

export function shouldStartInputHistory(editor: Editor) {
  const { selection } = editor.state;
  return selection.empty && selection.from === 1;
}

export function shouldContinueInputHistory(editor: Editor) {
  return editor.state.selection.empty;
}

export function getInputHistoryNavigationState({
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

export function applyInputHistoryMessage(editor: Editor, nextMessage: string | AIChatInputDraft) {
  editor.commands.setContent(buildEditorDocFromMessage(nextMessage));
  editor.commands.focus("end");
}
