import * as React from "react";
import { useCallback } from "react";

import { cn } from "../lib/utils";
import { useIMEComposing } from "../hooks/useIMEComposing";

function Input({
  className,
  type,
  onKeyDown,
  onCompositionStart,
  onCompositionEnd,
  ...props
}: React.ComponentProps<"input">) {
  const { composingRef, onCompositionStart: imeStart, onCompositionEnd: imeEnd } = useIMEComposing();

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      // IME 合成中：吞掉 Enter，避免误触发下游动作（如发送、提交）

      if (e.key === "Enter" && (e.nativeEvent.isComposing || e.keyCode === 229 || composingRef.current)) {
        e.preventDefault();
        return;
      }
      onKeyDown?.(e);
    },
    [onKeyDown, composingRef]
  );

  const handleCompositionStart = useCallback(
    (e: React.CompositionEvent<HTMLInputElement>) => {
      imeStart();
      onCompositionStart?.(e);
    },
    [imeStart, onCompositionStart]
  );

  const handleCompositionEnd = useCallback(
    (e: React.CompositionEvent<HTMLInputElement>) => {
      imeEnd();
      onCompositionEnd?.(e);
    },
    [imeEnd, onCompositionEnd]
  );

  return (
    <input
      type={type}
      data-slot="input"
      autoComplete="off"
      autoCorrect="off"
      autoCapitalize="off"
      spellCheck={false}
      className={cn(
        "h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground/55 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
        "aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
        className
      )}
      onKeyDown={handleKeyDown}
      onCompositionStart={handleCompositionStart}
      onCompositionEnd={handleCompositionEnd}
      {...props}
    />
  );
}

export { Input };
