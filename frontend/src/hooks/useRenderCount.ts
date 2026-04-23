/// <reference types="vite/client" />
import { useEffect, useRef } from "react";

/**
 * 开发环境下统计组件重渲次数并打到 console。想排查"键入/拖拽时哪些组件在过度重渲"时，
 * 在目标组件首行加：`useRenderCount("DatabasePanel");`。生产构建下完全静默。
 */
export function useRenderCount(label: string): void {
  const countRef = useRef(0);
  useEffect(() => {
    countRef.current += 1;
    if (import.meta.env.DEV) {
      console.log(`[render] ${label}: ${countRef.current}`);
    }
  });
}
