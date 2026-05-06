import { useRef, useState, useMemo } from "react";
import { Loader2, Plus, Trash2, X } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { toast } from "sonner";
import { Button, Input, ScrollArea } from "@opskat/ui";
import { useQueryStore, RedisKeyInfo, RedisStreamEntry } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { RedisStreamAdd, RedisStreamDelete } from "../../../wailsjs/go/app/App";

const VALUE_ROW_HEIGHT = 30;

function formatFields(fields: Record<string, string>): string {
  const entries = Object.entries(fields);
  if (entries.length === 0) return "{}";
  const pairs = entries.map(([k, v]) => `${JSON.stringify(k)}: ${JSON.stringify(v)}`);
  return `{${pairs.join(", ")}}`;
}

export function RedisStreamViewer({
  info,
  tabId,
  t,
}: {
  info: RedisKeyInfo;
  tabId: string;
  t: (key: string, opts?: Record<string, unknown>) => string;
}) {
  const { loadMoreValues, redisStates, selectKey } = useQueryStore();
  const state = redisStates[tabId];
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const scrollRef = useRef<HTMLDivElement>(null);
  const entries = (info.value as RedisStreamEntry[]) || [];
  const [selectedEntryId, setSelectedEntryId] = useState<string | null>(null);
  const [entryId, setEntryId] = useState("*");
  const [field, setField] = useState("");
  const [value, setValue] = useState("");
  const [adding, setAdding] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => VALUE_ROW_HEIGHT,
    overscan: 20,
  });

  const totalLabel =
    info.total >= 0 ? t("query.loadedOfTotal", { loaded: entries.length, total: info.total }) : `${entries.length}`;

  const selectedEntry = entries.find((e) => e.id === selectedEntryId) || null;

  const handleRowClick = (entryId: string) => {
    setSelectedEntryId((prev) => (prev === entryId ? null : entryId));
  };

  const refreshSelectedKey = () => {
    if (state?.selectedKey) {
      selectKey(tabId, state.selectedKey);
    }
  };

  const handleAddEntry = async () => {
    if (!tabMeta || !state?.selectedKey || !field.trim()) return;
    setAdding(true);
    try {
      await RedisStreamAdd(tabMeta.assetId, state.currentDb, state.selectedKey, entryId.trim() || "*", [
        { field: field.trim(), value },
      ]);
      setEntryId("*");
      setField("");
      setValue("");
      refreshSelectedKey();
    } catch (err) {
      toast.error(String(err));
    } finally {
      setAdding(false);
    }
  };

  const handleDeleteEntry = async (id: string) => {
    if (!tabMeta || !state?.selectedKey) return;
    setDeletingId(id);
    try {
      await RedisStreamDelete(tabMeta.assetId, state.currentDb, state.selectedKey, [id]);
      setSelectedEntryId(null);
      refreshSelectedKey();
    } catch (err) {
      toast.error(String(err));
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Table header */}
      <div className="flex items-center border-b text-xs">
        <div className="w-48 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">{t("query.id")}</div>
        <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">{t("query.fields")}</div>
        <div className="shrink-0 px-2 py-1.5 text-xs text-muted-foreground">{totalLabel}</div>
      </div>

      {/* Virtualized rows */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((virtualRow) => {
            const entry = entries[virtualRow.index];
            const isSelected = selectedEntryId === entry.id;
            return (
              <div
                key={virtualRow.key}
                className={`absolute left-0 flex w-full cursor-pointer items-center border-b text-xs font-mono last:border-0 hover:bg-accent ${
                  isSelected ? "bg-accent" : ""
                }`}
                style={{ top: virtualRow.start, height: virtualRow.size }}
                onClick={() => handleRowClick(entry.id)}
              >
                <div className="w-48 shrink-0 truncate px-2 text-foreground">{entry.id}</div>
                <div className="flex-1 truncate px-2 text-muted-foreground">{formatFields(entry.fields)}</div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Entry detail panel */}
      {selectedEntry && (
        <div className="shrink-0 border-t">
          <div className="flex items-center justify-between border-b px-2 py-1">
            <span className="text-xs font-medium text-muted-foreground">{t("query.entryDetail")}</span>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => handleDeleteEntry(selectedEntry.id)}
                disabled={deletingId === selectedEntry.id}
                title={t("query.deleteEntry")}
              >
                {deletingId === selectedEntry.id ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <Trash2 className="size-3 text-muted-foreground hover:text-destructive" />
                )}
              </Button>
              <button className="inline-flex rounded p-0.5 hover:bg-accent" onClick={() => setSelectedEntryId(null)}>
                <X className="size-3 text-muted-foreground" />
              </button>
            </div>
          </div>
          <StreamEntryJsonViewer fields={selectedEntry.fields} t={t} />
        </div>
      )}

      {/* Load more values */}
      {info.hasMoreValues && (
        <div className="border-t px-2 py-1.5">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-full text-xs"
            onClick={() => loadMoreValues(tabId)}
            disabled={info.loadingMore}
          >
            {info.loadingMore ? <Loader2 className="mr-1 size-3 animate-spin" /> : null}
            {t("query.loadMore")}
          </Button>
        </div>
      )}

      <div className="flex items-center gap-1 border-t px-2 py-1.5">
        <Input
          className="h-6 w-28 shrink-0 font-mono text-xs"
          placeholder={t("query.streamEntryId")}
          value={entryId}
          onChange={(e) => setEntryId(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleAddEntry()}
        />
        <Input
          className="h-6 w-32 shrink-0 font-mono text-xs"
          placeholder={t("query.streamField")}
          value={field}
          onChange={(e) => setField(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleAddEntry()}
        />
        <Input
          className="h-6 min-w-0 flex-1 font-mono text-xs"
          placeholder={t("query.streamValue")}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleAddEntry()}
        />
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={handleAddEntry}
          disabled={adding || !field.trim()}
          title={t("query.addEntry")}
        >
          {adding ? <Loader2 className="size-3 animate-spin" /> : <Plus className="size-3" />}
        </Button>
      </div>
    </div>
  );
}

function StreamEntryJsonViewer({
  fields,
  t,
}: {
  fields: Record<string, string>;
  t: (key: string, opts?: Record<string, unknown>) => string;
}) {
  const [jsonFormatted, setJsonFormatted] = useState(true);

  const rawText = useMemo(() => JSON.stringify(fields), [fields]);
  const formattedText = useMemo(() => JSON.stringify(fields, null, 2), [fields]);
  const displayValue = jsonFormatted ? formattedText : rawText;

  return (
    <ScrollArea className="max-h-[200px]">
      <div className="p-3">
        <div className="mb-2 flex justify-end">
          <div className="inline-flex rounded-md border text-xs">
            <button
              className={`px-2 py-0.5 rounded-l-md ${jsonFormatted ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setJsonFormatted(true)}
            >
              {t("query.formatJson")}
            </button>
            <button
              className={`px-2 py-0.5 rounded-r-md ${!jsonFormatted ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setJsonFormatted(false)}
            >
              {t("query.rawText")}
            </button>
          </div>
        </div>
        <pre className="whitespace-pre-wrap break-all rounded border bg-muted/50 p-3 font-mono text-xs">
          {displayValue}
        </pre>
      </div>
    </ScrollArea>
  );
}
