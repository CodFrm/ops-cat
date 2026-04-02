import { useState, useMemo } from "react";
import { Loader2, Pencil } from "lucide-react";
import { toast } from "sonner";
import { Button, Textarea, ScrollArea } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { ExecuteRedisArgs } from "../../../wailsjs/go/app/App";

export function RedisStringEditor({ tabId, t }: { tabId: string; t: (key: string) => string }) {
  const { redisStates, selectKey } = useQueryStore();
  const state = redisStates[tabId];
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const [editing, setEditing] = useState(false);
  const [editVal, setEditVal] = useState("");
  const [saving, setSaving] = useState(false);
  const [jsonFormatted, setJsonFormatted] = useState(true);

  const originalVal = String(state?.keyInfo?.value ?? "");

  const isJson = useMemo(() => {
    try {
      const trimmed = originalVal.trim();
      if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return false;
      JSON.parse(trimmed);
      return true;
    } catch {
      return false;
    }
  }, [originalVal]);

  const displayValue = useMemo(() => {
    if (isJson && jsonFormatted) {
      try {
        return JSON.stringify(JSON.parse(originalVal), null, 2);
      } catch {
        return originalVal;
      }
    }
    return originalVal;
  }, [isJson, jsonFormatted, originalVal]);

  if (!state?.keyInfo || !state.selectedKey || !tabMeta) return null;

  const db = state.currentDb;

  const startEdit = () => {
    setEditVal(originalVal);
    setEditing(true);
  };

  const save = async () => {
    setSaving(true);
    try {
      await ExecuteRedisArgs(tabMeta.assetId, ["SET", state.selectedKey!, editVal], db);
      selectKey(tabId, state.selectedKey!);
      setEditing(false);
    } catch (err) {
      toast.error(String(err));
    }
    setSaving(false);
  };

  const cancel = () => {
    setEditing(false);
  };

  if (editing) {
    return (
      <div className="flex flex-1 flex-col">
        <Textarea
          className="flex-1 resize-none font-mono text-xs"
          value={editVal}
          onChange={(e) => setEditVal(e.target.value)}
        />
        <div className="flex items-center justify-end gap-1 border-t px-2 py-1.5">
          <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={cancel} disabled={saving}>
            {t("query.cancelEdit")}
          </Button>
          <Button size="sm" className="h-7 text-xs" onClick={save} disabled={saving}>
            {saving && <Loader2 className="mr-1 size-3 animate-spin" />}
            {t("query.saveValue")}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <div className="p-3">
        {/* JSON format toggle */}
        {isJson && (
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
        )}
        <div className="group/str relative">
          <pre className="whitespace-pre-wrap break-all rounded border bg-muted/50 p-3 font-mono text-xs">
            {displayValue}
          </pre>
          <Button
            variant="ghost"
            size="icon-xs"
            className="absolute right-2 top-2 hidden group-hover/str:inline-flex"
            onClick={startEdit}
          >
            <Pencil className="size-3" />
          </Button>
        </div>
      </div>
    </ScrollArea>
  );
}
