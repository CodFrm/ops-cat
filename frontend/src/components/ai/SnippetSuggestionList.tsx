import { forwardRef, useImperativeHandle, useState } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { FileCode, Lock } from "lucide-react";
import { useTabStore } from "@/stores/tabStore";

export interface SnippetSuggestionItem {
  id: number;
  name: string;
  preview: string; // first non-blank line of content, truncated to ~80 chars
  content: string;
  readOnly: boolean; // source starts with "ext:"
}

export interface SnippetSuggestionListProps {
  /** The filtered list of items — TipTap's `items` callback already applies the query filter. */
  items: SnippetSuggestionItem[];
  /**
   * Total unfiltered prompt-snippet count. Passed out-of-band (not derived from
   * `items`) so the renderer can tell "filter zeroed out a non-empty list"
   * (`items.length===0 && totalAvailable>0` → "no matching") apart from
   * "no prompts exist at all" (`totalAvailable===0` → CTA empty state).
   */
  totalAvailable: number;
  command: (item: SnippetSuggestionItem) => void;
}

export interface SnippetSuggestionListRef {
  onKeyDown: (props: { event: KeyboardEvent }) => boolean;
}

export const SnippetSuggestionList = forwardRef<SnippetSuggestionListRef, SnippetSuggestionListProps>(
  function SnippetSuggestionList({ items, totalAvailable, command }, ref) {
    const totalEmpty = totalAvailable === 0;
    const { t } = useTranslation();
    const [selection, setSelection] = useState({ itemCount: 0, index: 0 });

    // Treat a changed filtered item count as a new menu session.
    const selectedIndex = selection.itemCount === items.length ? selection.index : 0;

    useImperativeHandle(ref, () => ({
      onKeyDown: ({ event }) => {
        if (items.length === 0) return false;
        if (event.key === "ArrowUp") {
          flushSync(() =>
            setSelection((current) => {
              const currentIndex = current.itemCount === items.length ? current.index : 0;
              return { itemCount: items.length, index: (currentIndex + items.length - 1) % items.length };
            })
          );
          return true;
        }
        if (event.key === "ArrowDown") {
          flushSync(() =>
            setSelection((current) => {
              const currentIndex = current.itemCount === items.length ? current.index : 0;
              return { itemCount: items.length, index: (currentIndex + 1) % items.length };
            })
          );
          return true;
        }
        if (event.key === "Enter") {
          const item = items[selectedIndex];
          if (item) command(item);
          return true;
        }
        return false;
      },
    }));

    const openSnippetsTab = () => {
      useTabStore.getState().openTab({
        id: "snippets",
        type: "page",
        label: t("nav.snippets"),
        meta: { type: "page", pageId: "snippets" },
      });
    };

    if (totalEmpty) {
      return (
        <div
          role="listbox"
          data-testid="snippet-suggestion-empty"
          className="bg-popover text-popover-foreground rounded-md border shadow-md px-3 py-3 text-xs min-w-[240px]"
        >
          <div className="flex items-center gap-2 text-muted-foreground">
            <FileCode className="h-3.5 w-3.5" aria-hidden="true" />
            <span>{t("snippet.slash.empty")}</span>
          </div>
          <button
            type="button"
            onClick={openSnippetsTab}
            className="mt-1.5 text-xs text-primary hover:underline"
            data-testid="snippet-suggestion-open-manager"
          >
            {t("snippet.slash.openManager")}
          </button>
        </div>
      );
    }

    if (items.length === 0) {
      return (
        <div
          role="listbox"
          data-testid="snippet-suggestion-nomatch"
          className="bg-popover text-popover-foreground rounded-md border shadow-md px-3 py-2 text-xs text-muted-foreground"
        >
          {t("snippet.slash.noMatch")}
        </div>
      );
    }

    return (
      <div
        role="listbox"
        data-testid="snippet-suggestion-list"
        className="bg-popover text-popover-foreground rounded-md border shadow-md overflow-hidden min-w-[260px] max-w-[400px]"
      >
        {items.map((item, idx) => (
          <button
            type="button"
            role="option"
            aria-selected={idx === selectedIndex}
            key={item.id}
            onClick={() => command(item)}
            data-testid="snippet-suggestion-row"
            data-snippet-id={item.id}
            className={
              "flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-left " +
              (idx === selectedIndex ? "bg-accent" : "hover:bg-accent/60")
            }
          >
            {item.readOnly && <Lock className="h-3 w-3 text-muted-foreground shrink-0" aria-hidden="true" />}
            <span className="flex-1 min-w-0">
              <span className="block truncate text-foreground font-medium">{item.name}</span>
              {item.preview && (
                <span className="block truncate text-muted-foreground font-mono text-[10px]" title={item.preview}>
                  {item.preview}
                </span>
              )}
            </span>
          </button>
        ))}
      </div>
    );
  }
);
