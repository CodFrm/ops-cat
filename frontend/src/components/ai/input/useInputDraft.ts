import { useCallback, useEffect, useRef, type MutableRefObject } from "react";
import type { Editor } from "@tiptap/react";
import { extractContentXml } from "./content";
import type { AIChatInputDraft, ProseMirrorLikeNode } from "./types";

const DRAFT_CHANGE_THROTTLE_MS = 120;

export function useInputDraftSync(onDraftChangeRef: MutableRefObject<((draft: AIChatInputDraft) => void) | undefined>) {
  const pendingEditorRef = useRef<Editor | null>(null);
  const draftPendingRef = useRef(false);
  const draftFlushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastDraftContentRef = useRef("");

  const cancelPendingDraft = useCallback(() => {
    if (draftFlushTimerRef.current != null) {
      clearTimeout(draftFlushTimerRef.current);
      draftFlushTimerRef.current = null;
    }
    draftPendingRef.current = false;
    pendingEditorRef.current = null;
  }, []);

  const scheduleDraftFlush = useCallback(
    (editor: Editor) => {
      if (!onDraftChangeRef.current) return;
      pendingEditorRef.current = editor;
      draftPendingRef.current = true;
      if (draftFlushTimerRef.current != null) return;

      draftFlushTimerRef.current = setTimeout(() => {
        draftFlushTimerRef.current = null;
        if (!draftPendingRef.current) return;
        draftPendingRef.current = false;

        const pendingEditor = pendingEditorRef.current;
        pendingEditorRef.current = null;
        if (!pendingEditor) return;

        const content = extractContentXml(pendingEditor.state.doc as unknown as ProseMirrorLikeNode);
        if (content === lastDraftContentRef.current) return;
        lastDraftContentRef.current = content;
        onDraftChangeRef.current?.({ content });
      }, DRAFT_CHANGE_THROTTLE_MS);
    },
    [onDraftChangeRef]
  );

  const clearDraftImmediately = useCallback(() => {
    cancelPendingDraft();
    lastDraftContentRef.current = "";
    onDraftChangeRef.current?.({ content: "" });
  }, [cancelPendingDraft, onDraftChangeRef]);

  useEffect(() => cancelPendingDraft, [cancelPendingDraft]);

  return {
    scheduleDraftFlush,
    clearDraftImmediately,
    cancelPendingDraft,
  };
}
