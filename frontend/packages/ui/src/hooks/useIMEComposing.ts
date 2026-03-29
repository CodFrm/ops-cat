import { useRef, useCallback } from "react";

/**
 * 处理 IME 输入法组合状态，防止中文输入时按回车误触发提交。
 * 使用时间戳方式：compositionend 后 100ms 内的 Enter 视为 IME 确认，不触发提交。
 */
export function useIMEComposing() {
  const composingRef = useRef(false);
  const endTimestampRef = useRef(0);

  const onCompositionStart = useCallback(() => {
    composingRef.current = true;
  }, []);

  const onCompositionEnd = useCallback(() => {
    composingRef.current = false;
    endTimestampRef.current = Date.now();
  }, []);

  const isComposing = useCallback(() => {
    if (composingRef.current) return true;
    // compositionend 后 100ms 内视为仍在组合
    if (Date.now() - endTimestampRef.current < 100) return true;
    return false;
  }, []);

  return { composingRef, isComposing, onCompositionStart, onCompositionEnd };
}
