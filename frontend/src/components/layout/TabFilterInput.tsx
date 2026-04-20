import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Search, X } from "lucide-react";
import { cn } from "@opskat/ui";

interface TabFilterInputProps {
  value: string;
  onChange: (v: string) => void;
  onClose: () => void;
  onEnter?: () => void;
  onArrow?: (dir: "up" | "down") => void;
  autoFocus?: boolean;
  className?: string;
}

export function TabFilterInput({
  value,
  onChange,
  onClose,
  onEnter,
  onArrow,
  autoFocus,
  className,
}: TabFilterInputProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (autoFocus) inputRef.current?.focus();
  }, [autoFocus]);

  return (
    <div className={cn("relative flex items-center gap-1 px-2 py-1.5 border-b bg-muted/30", className)}>
      <Search className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            onChange("");
            onClose();
          } else if (e.key === "Enter" && onEnter) {
            // IME 合成中：让 Enter 作为候选词确认，不触发动作

            if (e.nativeEvent.isComposing || e.keyCode === 229) return;
            e.preventDefault();
            onEnter();
          } else if (e.key === "ArrowUp" && onArrow) {
            e.preventDefault();
            onArrow("up");
          } else if (e.key === "ArrowDown" && onArrow) {
            e.preventDefault();
            onArrow("down");
          }
        }}
        placeholder={t("sideTabs.filterPlaceholder")}
        className="flex-1 bg-transparent outline-none text-sm placeholder:text-muted-foreground"
      />
      {value && (
        <button
          type="button"
          onClick={() => onChange("")}
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={t("sideTabs.clearFilter")}
        >
          <X className="h-3 w-3" />
        </button>
      )}
    </div>
  );
}
