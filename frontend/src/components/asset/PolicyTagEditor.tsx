import { useState } from "react";
import { X, Plus, ShieldCheck, ShieldX, ShieldAlert } from "lucide-react";
import { cn, Input } from "@opskat/ui";

type PolicyVariant = "allow" | "deny" | "warn";

const VARIANT_CONFIG: Record<
  PolicyVariant,
  {
    tagBg: string;
    tagBorder: string;
    tagText: string;
    removeBtnHover: string;
    icon: typeof ShieldCheck;
    accentBorder: string;
  }
> = {
  allow: {
    tagBg: "bg-emerald-500/8",
    tagBorder: "border-emerald-500/20",
    tagText: "text-emerald-700 dark:text-emerald-400",
    removeBtnHover: "hover:bg-emerald-500/20 hover:text-emerald-800 dark:hover:text-emerald-300",
    icon: ShieldCheck,
    accentBorder: "border-l-emerald-500",
  },
  deny: {
    tagBg: "bg-red-500/8",
    tagBorder: "border-red-500/20",
    tagText: "text-red-700 dark:text-red-400",
    removeBtnHover: "hover:bg-red-500/20 hover:text-red-800 dark:hover:text-red-300",
    icon: ShieldX,
    accentBorder: "border-l-red-500",
  },
  warn: {
    tagBg: "bg-amber-500/8",
    tagBorder: "border-amber-500/20",
    tagText: "text-amber-700 dark:text-amber-400",
    removeBtnHover: "hover:bg-amber-500/20 hover:text-amber-800 dark:hover:text-amber-300",
    icon: ShieldAlert,
    accentBorder: "border-l-amber-500",
  },
};

interface PolicyTagEditorProps {
  label: string;
  items: string[];
  onAdd?: (vals: string[]) => void;
  onRemove?: (idx: number) => void;
  placeholder?: string;
  variant: PolicyVariant;
  emptyText?: string;
}

export function PolicyTagEditor({
  label,
  items,
  onAdd,
  onRemove,
  placeholder,
  variant,
  emptyText,
}: PolicyTagEditorProps) {
  const [input, setInput] = useState("");
  const [isFocused, setIsFocused] = useState(false);
  const cfg = VARIANT_CONFIG[variant];
  const Icon = cfg.icon;

  const readonly = !onAdd || !onRemove;

  const handleAdd = () => {
    if (!onAdd) return;
    const vals = input
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (vals.length === 0) return;
    onAdd(vals);
    setInput("");
  };

  return (
    <div className={cn("rounded-lg border-l-2 pl-3 py-2", cfg.accentBorder)}>
      <div className="flex items-center gap-1.5 mb-2">
        <Icon className={cn("h-3.5 w-3.5", cfg.tagText)} />
        <span className={cn("text-xs font-medium", cfg.tagText)}>{label}</span>
        {items.length > 0 && (
          <span className="text-[10px] text-muted-foreground ml-auto tabular-nums">{items.length}</span>
        )}
      </div>

      {items.length > 0 ? (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {items.map((item, i) => (
            <span
              key={i}
              className={cn(
                "inline-flex items-center gap-1 pl-2 pr-1 py-0.5 rounded-full text-xs font-mono border",
                cfg.tagBg,
                cfg.tagBorder,
                cfg.tagText
              )}
            >
              {item}
              {!readonly && (
                <button
                  type="button"
                  className={cn(
                    "inline-flex items-center justify-center h-4 w-4 rounded-full transition-colors",
                    cfg.removeBtnHover
                  )}
                  onClick={() => onRemove!(i)}
                >
                  <X className="h-2.5 w-2.5" />
                </button>
              )}
            </span>
          ))}
        </div>
      ) : emptyText ? (
        <p className="text-[11px] text-muted-foreground/60 mb-2 italic">{emptyText}</p>
      ) : null}

      {!readonly && (
        <div className="relative">
          <Input
            className={cn("h-7 text-xs font-mono pr-7 transition-colors", isFocused && "ring-1 ring-ring")}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                handleAdd();
              }
            }}
            onFocus={() => setIsFocused(true)}
            onBlur={() => setIsFocused(false)}
            placeholder={placeholder}
          />
          {input.trim() && (
            <button
              type="button"
              className="absolute right-1 top-1/2 -translate-y-1/2 h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-foreground transition-colors"
              onClick={handleAdd}
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export type { PolicyVariant };
