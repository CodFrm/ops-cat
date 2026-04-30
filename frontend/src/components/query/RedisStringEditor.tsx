import { useState, useMemo } from "react";
import { Loader2, Pencil } from "lucide-react";
import { toast } from "sonner";
import { Button, Textarea, ScrollArea } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { RedisSetStringValue } from "../../../wailsjs/go/app/App";

interface JsonToken {
  value: string;
  className?: string;
}

const JSON_TOKEN_PATTERN = /"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\b(?:true|false|null)\b|[{}[\],:]/g;

function tokenizeJson(value: string): JsonToken[] {
  const tokens: JsonToken[] = [];
  let lastIndex = 0;

  for (const match of value.matchAll(JSON_TOKEN_PATTERN)) {
    const token = match[0];
    const index = match.index ?? 0;
    if (index > lastIndex) {
      tokens.push({ value: value.slice(lastIndex, index) });
    }

    if (token.startsWith('"')) {
      const rest = value.slice(index + token.length);
      tokens.push({
        value: token,
        className: /^\s*:/.test(rest) ? "text-sky-700 dark:text-sky-300" : "text-emerald-700 dark:text-emerald-300",
      });
    } else if (/^-?\d/.test(token)) {
      tokens.push({ value: token, className: "text-purple-700 dark:text-purple-300" });
    } else if (token === "true" || token === "false" || token === "null") {
      tokens.push({ value: token, className: "text-amber-700 dark:text-amber-300" });
    } else {
      tokens.push({ value: token, className: "text-muted-foreground" });
    }

    lastIndex = index + token.length;
  }

  if (lastIndex < value.length) {
    tokens.push({ value: value.slice(lastIndex) });
  }

  return tokens;
}

export function RedisStringEditor({ tabId, t }: { tabId: string; t: (key: string) => string }) {
  const { redisStates, selectKey } = useQueryStore();
  const state = redisStates[tabId];
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const [editing, setEditing] = useState(false);
  const [editVal, setEditVal] = useState("");
  const [saving, setSaving] = useState(false);
  const [viewMode, setViewMode] = useState<"raw" | "json" | "hex" | "base64">("raw");

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

  const hexValue = useMemo(
    () => Array.from(new TextEncoder().encode(originalVal), (b) => b.toString(16).padStart(2, "0")).join(""),
    [originalVal]
  );

  const base64Value = useMemo(() => {
    const bytes = new TextEncoder().encode(originalVal);
    let binary = "";
    for (const b of bytes) binary += String.fromCharCode(b);
    return btoa(binary);
  }, [originalVal]);

  const displayValue = useMemo(() => {
    if (viewMode === "json" && isJson) {
      try {
        return JSON.stringify(JSON.parse(originalVal), null, 2);
      } catch {
        return originalVal;
      }
    }
    if (viewMode === "hex") return hexValue;
    if (viewMode === "base64") return base64Value;
    return originalVal;
  }, [base64Value, hexValue, isJson, originalVal, viewMode]);

  const jsonTokens = useMemo(() => {
    if (!isJson || (viewMode !== "raw" && viewMode !== "json")) return null;
    return tokenizeJson(displayValue);
  }, [displayValue, isJson, viewMode]);

  if (!state?.keyInfo || !state.selectedKey || !tabMeta) return null;

  const db = state.currentDb;

  const startEdit = () => {
    setEditVal(displayValue);
    setEditing(true);
  };

  const save = async () => {
    setSaving(true);
    try {
      await RedisSetStringValue({
        assetId: tabMeta.assetId,
        db,
        key: state.selectedKey!,
        value: editVal,
        format: viewMode,
      });
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
        <div className="mb-2 flex justify-end">
          <div className="inline-flex rounded-md border text-xs">
            <button
              className={`px-2 py-0.5 rounded-l-md ${viewMode === "raw" ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setViewMode("raw")}
            >
              {t("query.rawText")}
            </button>
            <button
              className={`px-2 py-0.5 ${viewMode === "json" ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setViewMode("json")}
              disabled={!isJson}
            >
              {t("query.formatJson")}
            </button>
            <button
              className={`px-2 py-0.5 ${viewMode === "hex" ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setViewMode("hex")}
            >
              Hex
            </button>
            <button
              className={`px-2 py-0.5 rounded-r-md ${viewMode === "base64" ? "bg-accent text-accent-foreground" : ""}`}
              onClick={() => setViewMode("base64")}
            >
              Base64
            </button>
          </div>
        </div>
        <div className="group/str relative">
          <pre
            data-testid="redis-string-value"
            className="max-h-[calc(100vh-260px)] min-h-24 overflow-auto whitespace-pre rounded border bg-muted/50 p-3 font-mono text-xs leading-5"
          >
            {jsonTokens
              ? jsonTokens.map((token, index) =>
                  token.className ? (
                    <span key={`${index}-${token.value}`} className={token.className}>
                      {token.value}
                    </span>
                  ) : (
                    token.value
                  )
                )
              : displayValue}
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
