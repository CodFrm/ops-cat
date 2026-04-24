import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Search, Lock, CornerDownLeft, Loader2, FileCode } from "lucide-react";
import { Button, Input, Popover, PopoverContent, PopoverTrigger, cn } from "@opskat/ui";
import { useSnippetStore } from "@/stores/snippetStore";
import { useTabStore } from "@/stores/tabStore";
import { ListSnippets } from "../../../wailsjs/go/app/App";
import type { snippet_entity, snippet_svc } from "../../../wailsjs/go/models";

type Snippet = snippet_entity.Snippet;

export interface SnippetPopoverProps {
  /** Category filter; fixes which snippets are listed (e.g. "shell", "sql", "mongo"). */
  category: string;
  /** Parent-supplied trigger element; wrapped via Radix `asChild`. */
  trigger: React.ReactNode;
  /** Invoked when the user picks a snippet. */
  onInsert: (content: string, opts: { withEnter: boolean }) => void;
  /**
   * Show the secondary "Insert + Enter" action. Only makes sense for the terminal,
   * where we can append \r to auto-execute. Default: false.
   */
  showSendWithEnter?: boolean;
  /** Test-only: expose the content root for assertions / typing into the search. */
  contentClassName?: string;
}

function isReadOnly(s: Snippet): boolean {
  return typeof s.Source === "string" && s.Source.startsWith("ext:");
}

function firstLine(content: string): string {
  const nl = content.indexOf("\n");
  return (nl === -1 ? content : content.slice(0, nl)).trim();
}

function matchesKeyword(s: Snippet, kw: string): boolean {
  if (!kw) return true;
  const lc = kw.toLowerCase();
  return (
    s.Name.toLowerCase().includes(lc) ||
    (s.Description || "").toLowerCase().includes(lc) ||
    (s.Content || "").toLowerCase().includes(lc)
  );
}

export function SnippetPopover({
  category,
  trigger,
  onInsert,
  showSendWithEnter = false,
  contentClassName,
}: SnippetPopoverProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<Snippet[]>([]);
  const [keyword, setKeyword] = useState("");

  // Dedupe + cancel-stale: newer fetches override older ones (rapid prop flips or reopen).
  const reqIdRef = useRef(0);
  // Cache per category for the popover's lifetime. Reset when props change.
  const cacheKey = category;
  const cachedKeyRef = useRef<string | null>(null);

  const loadCategories = useSnippetStore((s) => s.loadCategories);
  const recordUse = useSnippetStore((s) => s.recordUse);
  const categoriesLoaded = useSnippetStore((s) => s.categories.length > 0);

  const fetchList = useCallback(async () => {
    const myId = ++reqIdRef.current;
    setLoading(true);
    try {
      // Wails generated class uses camelCase — cast through unknown because the
      // generated constructor signature expects a source object with all fields.
      const req = {
        categories: [category],
        keyword: "",
        limit: 0,
        offset: 0,
        orderBy: "",
      } as unknown as snippet_svc.ListReq;
      const result = await ListSnippets(req);
      if (myId !== reqIdRef.current) return; // superseded
      setItems(result ?? []);
      cachedKeyRef.current = cacheKey;
    } catch {
      if (myId === reqIdRef.current) setItems([]);
    } finally {
      if (myId === reqIdRef.current) setLoading(false);
    }
  }, [category, cacheKey]);

  // Load-on-open (with cache) + re-fetch when props (category) change.
  useEffect(() => {
    if (!open) return;
    if (!categoriesLoaded) void loadCategories().catch(() => {});
    if (cachedKeyRef.current !== cacheKey) {
      void fetchList();
    }
  }, [open, cacheKey, categoriesLoaded, loadCategories, fetchList]);

  // Invalidate cache if the cache-key changes while open (rare: same popover instance
  // migrating between assets) — triggers re-fetch on next open or immediately if open.
  useEffect(() => {
    if (cachedKeyRef.current && cachedKeyRef.current !== cacheKey) {
      cachedKeyRef.current = null;
      if (open) void fetchList();
    }
  }, [cacheKey, open, fetchList]);

  const filtered = useMemo(() => items.filter((s) => matchesKeyword(s, keyword.trim())), [items, keyword]);

  const handlePick = useCallback(
    (s: Snippet, withEnter: boolean) => {
      onInsert(s.Content, { withEnter });
      recordUse(s.ID);
      setOpen(false);
      setKeyword("");
    },
    [onInsert, recordUse]
  );

  const openSnippetsTab = useCallback(() => {
    useTabStore.getState().openTab({
      id: "snippets",
      type: "page",
      label: t("nav.snippets"),
      meta: { type: "page", pageId: "snippets" },
    });
    setOpen(false);
  }, [t]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="end" className={cn("w-[360px] p-0", contentClassName)}>
        {/* Search */}
        <div className="p-2 border-b">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              autoFocus
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder={t("snippet.popover.searchPlaceholder")}
              className="h-8 pl-7 text-xs"
            />
          </div>
        </div>

        {/* List */}
        <div className="max-h-[320px] overflow-auto" data-testid="snippet-popover-list">
          {loading && items.length === 0 && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}
          {!loading && filtered.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-8 px-4 text-center">
              <FileCode className="h-6 w-6 text-muted-foreground/60" />
              <p className="text-xs text-muted-foreground">{t("snippet.popover.empty")}</p>
              <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={openSnippetsTab}>
                {t("snippet.popover.openManager")}
              </Button>
            </div>
          )}
          {filtered.map((s) => {
            const ro = isReadOnly(s);
            const preview = firstLine(s.Content || "");
            return (
              <div
                key={s.ID}
                role="button"
                tabIndex={0}
                onClick={() => handlePick(s, false)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    handlePick(s, false);
                  }
                }}
                className={cn(
                  "group flex items-center gap-2 px-3 py-2 text-xs cursor-pointer border-b last:border-b-0",
                  "hover:bg-accent focus:bg-accent focus:outline-none"
                )}
                data-testid="snippet-popover-row"
                data-snippet-id={s.ID}
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1.5">
                    {ro && <Lock className="h-3 w-3 text-muted-foreground shrink-0" aria-hidden="true" />}
                    <span className="font-medium truncate">{s.Name}</span>
                    {s.Description && (
                      <span className="text-muted-foreground truncate hidden sm:inline" title={s.Description}>
                        · {s.Description}
                      </span>
                    )}
                  </div>
                  {preview && (
                    <div className="mt-0.5 font-mono text-[10px] text-muted-foreground truncate" title={preview}>
                      {preview}
                    </div>
                  )}
                </div>
                {showSendWithEnter && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 opacity-0 group-hover:opacity-100 focus:opacity-100 shrink-0"
                    title={t("snippet.popover.sendWithEnter")}
                    aria-label={t("snippet.popover.sendWithEnter")}
                    onClick={(e) => {
                      e.stopPropagation();
                      handlePick(s, true);
                    }}
                    data-testid="snippet-popover-row-enter"
                  >
                    <CornerDownLeft className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}
