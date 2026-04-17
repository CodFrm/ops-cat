import type { RefObject } from "react";

export interface TabDragContextValue {
  dragKeyRef: RefObject<string | null>;
  reorder: (fromId: string, toId: string) => void;
}

export function useTabDragAndDrop(tabKey: string, ctx: TabDragContextValue) {
  return {
    draggable: true as const,
    onDragStart: (e: React.DragEvent) => {
      ctx.dragKeyRef.current = tabKey;
      e.dataTransfer.effectAllowed = "move";
    },
    onDragOver: (e: React.DragEvent) => {
      if (ctx.dragKeyRef.current) {
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
      }
    },
    onDragEnter: (e: React.DragEvent) => {
      e.preventDefault();
      if (!ctx.dragKeyRef.current || ctx.dragKeyRef.current === tabKey) return;
      ctx.reorder(ctx.dragKeyRef.current, tabKey);
    },
    onDragEnd: () => {
      ctx.dragKeyRef.current = null;
    },
  };
}
