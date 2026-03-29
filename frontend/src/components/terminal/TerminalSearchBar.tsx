import { useState, useRef, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { X, ChevronUp, ChevronDown, CaseSensitive, WholeWord, Regex } from "lucide-react";
import { cn, Input, Button } from "@opskat/ui";
import type { SearchAddon } from "@xterm/addon-search";

const SEARCH_DECORATIONS = {
  matchBackground: "#FFD33D44",
  matchBorder: "#FFD33D",
  matchOverviewRuler: "#FFD33D",
  activeMatchBackground: "#FF6A0088",
  activeMatchBorder: "#FF6A00",
  activeMatchColorOverviewRuler: "#FF6A00",
};

interface TerminalSearchBarProps {
  visible: boolean;
  onClose: () => void;
  searchAddon: SearchAddon | null;
}

export function TerminalSearchBar({ visible, onClose, searchAddon }: TerminalSearchBarProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [wholeWord, setWholeWord] = useState(false);
  const [regex, setRegex] = useState(false);

  useEffect(() => {
    if (visible) {
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      searchAddon?.clearDecorations();
    }
  }, [visible, searchAddon]);

  const doSearch = useCallback(
    (direction: "next" | "previous", term?: string) => {
      if (!searchAddon) return;
      const searchTerm = term ?? query;
      if (!searchTerm) {
        searchAddon.clearDecorations();
        return;
      }
      const opts = { caseSensitive, wholeWord, regex, decorations: SEARCH_DECORATIONS };
      if (direction === "next") {
        searchAddon.findNext(searchTerm, opts);
      } else {
        searchAddon.findPrevious(searchTerm, opts);
      }
    },
    [searchAddon, query, caseSensitive, wholeWord, regex]
  );

  const handleQueryChange = useCallback(
    (value: string) => {
      setQuery(value);
      if (!searchAddon) return;
      if (!value) {
        searchAddon.clearDecorations();
        return;
      }
      searchAddon.findNext(value, { caseSensitive, wholeWord, regex, decorations: SEARCH_DECORATIONS });
    },
    [searchAddon, caseSensitive, wholeWord, regex]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        doSearch(e.shiftKey ? "previous" : "next");
      } else if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    },
    [doSearch, onClose]
  );

  // 切换搜索选项时重新搜索
  useEffect(() => {
    if (visible && query) {
      doSearch("next");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [caseSensitive, wholeWord, regex]);

  if (!visible) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1.5 border-b bg-background/95 backdrop-blur-sm">
      <Input
        ref={inputRef}
        value={query}
        onChange={(e) => handleQueryChange(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={t("ssh.search.placeholder")}
        className="h-7 flex-1 text-sm"
      />
      <Button
        variant="ghost"
        size="icon"
        className={cn("h-7 w-7", caseSensitive && "bg-accent text-accent-foreground")}
        onClick={() => setCaseSensitive(!caseSensitive)}
        title={t("ssh.search.caseSensitive")}
      >
        <CaseSensitive className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className={cn("h-7 w-7", wholeWord && "bg-accent text-accent-foreground")}
        onClick={() => setWholeWord(!wholeWord)}
        title={t("ssh.search.wholeWord")}
      >
        <WholeWord className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className={cn("h-7 w-7", regex && "bg-accent text-accent-foreground")}
        onClick={() => setRegex(!regex)}
        title={t("ssh.search.regex")}
      >
        <Regex className="h-3.5 w-3.5" />
      </Button>
      <div className={cn("flex items-center border-l ml-0.5 pl-1")}>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={() => doSearch("previous")}
          title={t("ssh.search.previous")}
        >
          <ChevronUp className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={() => doSearch("next")}
          title={t("ssh.search.next")}
        >
          <ChevronDown className="h-3.5 w-3.5" />
        </Button>
      </div>
      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClose} title={t("ssh.search.close")}>
        <X className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
