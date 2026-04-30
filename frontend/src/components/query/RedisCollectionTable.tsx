import { useState, useRef } from "react";
import { Loader2, Trash2, Pencil, Check, X, Plus } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { toast } from "sonner";
import { Button, Input, ConfirmDialog } from "@opskat/ui";
import { useQueryStore, RedisKeyInfo } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import {
  RedisHashDelete,
  RedisHashSet,
  RedisListDelete,
  RedisListPush,
  RedisListSet,
  RedisSetAdd,
  RedisSetRemove,
  RedisZSetAdd,
  RedisZSetRemove,
} from "../../../wailsjs/go/app/App";

const VALUE_ROW_HEIGHT = 30;

function getItemCount(info: RedisKeyInfo): number {
  switch (info.type) {
    case "hash":
      return ((info.value as [string, string][]) || []).length;
    case "list":
    case "set":
      return ((info.value as string[]) || []).length;
    case "zset":
      return ((info.value as [string, string][]) || []).length;
    default:
      return 0;
  }
}

// --- Inline edit row ---

function EditableCell({
  value,
  onSave,
  className,
}: {
  value: string;
  onSave: (val: string) => Promise<void>;
  className?: string;
}) {
  const [editing, setEditing] = useState(false);
  const [editVal, setEditVal] = useState(value);
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const startEdit = () => {
    setEditVal(value);
    setEditing(true);
    setTimeout(() => inputRef.current?.focus(), 0);
  };

  const save = async () => {
    if (editVal === value) {
      setEditing(false);
      return;
    }
    setSaving(true);
    try {
      await onSave(editVal);
      setEditing(false);
    } catch (err) {
      toast.error(String(err));
    }
    setSaving(false);
  };

  const cancel = () => {
    setEditing(false);
    setEditVal(value);
  };

  if (editing) {
    return (
      <div className={`flex items-center gap-0.5 ${className || ""}`}>
        <Input
          ref={inputRef}
          className="h-6 flex-1 font-mono text-xs"
          value={editVal}
          onChange={(e) => setEditVal(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") save();
            if (e.key === "Escape") cancel();
          }}
          disabled={saving}
        />
        <Button variant="ghost" size="icon-xs" onClick={save} disabled={saving}>
          <Check className="size-3 text-green-600" />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={cancel} disabled={saving}>
          <X className="size-3" />
        </Button>
      </div>
    );
  }

  return (
    <div
      className={`group/cell flex cursor-pointer items-center truncate ${className || ""}`}
      onDoubleClick={startEdit}
    >
      <span className="truncate">{value}</span>
      <button className="ml-auto hidden shrink-0 group-hover/cell:inline-flex" onClick={startEdit}>
        <Pencil className="size-3 text-muted-foreground hover:text-foreground" />
      </button>
    </div>
  );
}

// --- Add row form ---

function AddRowForm({
  type,
  onAdd,
  t,
}: {
  type: string;
  onAdd: (args: string[]) => Promise<void>;
  t: (key: string) => string;
}) {
  const [field, setField] = useState("");
  const [value, setValue] = useState("");
  const [score, setScore] = useState("0");
  const [adding, setAdding] = useState(false);
  const submit = async () => {
    setAdding(true);
    try {
      if (type === "hash") {
        await onAdd([field, value]);
        setField("");
        setValue("");
      } else if (type === "list") {
        await onAdd([value]);
        setValue("");
      } else if (type === "set") {
        await onAdd([value]);
        setValue("");
      } else if (type === "zset") {
        await onAdd([score, value]);
        setScore("0");
        setValue("");
      }
    } catch (err) {
      toast.error(String(err));
    }
    setAdding(false);
  };

  return (
    <div className="flex items-center gap-1 border-t px-2 py-1.5">
      {type === "hash" && (
        <>
          <Input
            className="h-6 w-1/3 shrink-0 font-mono text-xs"
            placeholder={t("query.newField")}
            value={field}
            onChange={(e) => setField(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
          <Input
            className="h-6 flex-1 font-mono text-xs"
            placeholder={t("query.newValue")}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
        </>
      )}
      {(type === "list" || type === "set") && (
        <Input
          className="h-6 flex-1 font-mono text-xs"
          placeholder={type === "set" ? t("query.newMember") : t("query.newValue")}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
        />
      )}
      {type === "zset" && (
        <>
          <Input
            className="h-6 w-20 shrink-0 font-mono text-xs"
            placeholder={t("query.newScore")}
            value={score}
            onChange={(e) => setScore(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
          <Input
            className="h-6 flex-1 font-mono text-xs"
            placeholder={t("query.newMember")}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
        </>
      )}
      <Button variant="ghost" size="icon-xs" onClick={submit} disabled={adding}>
        {adding ? <Loader2 className="size-3 animate-spin" /> : <Plus className="size-3" />}
      </Button>
    </div>
  );
}

// --- Collection Table ---

export function RedisCollectionTable({
  info,
  tabId,
  t,
}: {
  info: RedisKeyInfo;
  tabId: string;
  t: (key: string, opts?: Record<string, unknown>) => string;
}) {
  const { loadMoreValues, selectKey, redisStates } = useQueryStore();
  const state = redisStates[tabId];
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const scrollRef = useRef<HTMLDivElement>(null);
  const itemCount = getItemCount(info);
  const selectedKey = state?.selectedKey;
  const db = state?.currentDb ?? 0;
  const [deleteTarget, setDeleteTarget] = useState<{ label: string; action: () => void } | null>(null);

  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => VALUE_ROW_HEIGHT,
    overscan: 20,
  });

  const totalLabel =
    info.total >= 0 ? t("query.loadedOfTotal", { loaded: itemCount, total: info.total }) : `${itemCount}`;

  const handleEditHash = async (field: string, newVal: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisHashSet(tabMeta.assetId, db, selectedKey, field, newVal);
    selectKey(tabId, selectedKey);
  };

  const handleDeleteHash = async (field: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisHashDelete(tabMeta.assetId, db, selectedKey, field);
    selectKey(tabId, selectedKey);
  };

  const handleEditList = async (index: number, newVal: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisListSet(tabMeta.assetId, db, selectedKey, index, newVal);
    selectKey(tabId, selectedKey);
  };

  const handleDeleteList = async (index: number) => {
    if (!tabMeta || !selectedKey) return;
    await RedisListDelete(tabMeta.assetId, db, selectedKey, index);
    selectKey(tabId, selectedKey);
  };

  const handleDeleteSet = async (member: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisSetRemove(tabMeta.assetId, db, selectedKey, member);
    selectKey(tabId, selectedKey);
  };

  const handleEditZsetScore = async (member: string, newScore: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisZSetAdd(tabMeta.assetId, db, selectedKey, member, Number(newScore));
    selectKey(tabId, selectedKey);
  };

  const handleDeleteZset = async (member: string) => {
    if (!tabMeta || !selectedKey) return;
    await RedisZSetRemove(tabMeta.assetId, db, selectedKey, member);
    selectKey(tabId, selectedKey);
  };

  const handleAdd = async (args: string[]) => {
    if (!tabMeta || !selectedKey) return;
    if (info.type === "hash" && args.length === 2) {
      await RedisHashSet(tabMeta.assetId, db, selectedKey, args[0], args[1]);
    } else if (info.type === "list" && args.length === 1) {
      await RedisListPush(tabMeta.assetId, db, selectedKey, args[0]);
    } else if (info.type === "set" && args.length === 1) {
      await RedisSetAdd(tabMeta.assetId, db, selectedKey, args[0]);
    } else if (info.type === "zset" && args.length === 2) {
      await RedisZSetAdd(tabMeta.assetId, db, selectedKey, args[1], Number(args[0]));
    }
    selectKey(tabId, selectedKey);
  };

  return (
    <div className="flex h-full flex-col">
      {/* Table header */}
      <div className="flex items-center border-b text-xs">
        {info.type === "hash" && (
          <>
            <div className="w-1/3 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">{t("query.field")}</div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">{t("query.value")}</div>
          </>
        )}
        {info.type === "list" && (
          <>
            <div className="w-16 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">{t("query.index")}</div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">{t("query.value")}</div>
          </>
        )}
        {info.type === "set" && (
          <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">{t("query.member")}</div>
        )}
        {info.type === "zset" && (
          <>
            <div className="w-24 shrink-0 px-2 py-1.5 font-medium text-muted-foreground">{t("query.score")}</div>
            <div className="flex-1 px-2 py-1.5 font-medium text-muted-foreground">{t("query.member")}</div>
          </>
        )}
        <div className="shrink-0 px-2 py-1.5 text-xs text-muted-foreground">{totalLabel}</div>
      </div>

      {/* Virtualized rows */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((virtualRow) => {
            const idx = virtualRow.index;
            return (
              <div
                key={virtualRow.key}
                className="group/row absolute left-0 flex w-full items-center border-b text-xs font-mono last:border-0"
                style={{ top: virtualRow.start, height: virtualRow.size }}
              >
                {info.type === "hash" &&
                  (() => {
                    const entry = (info.value as [string, string][])[idx];
                    return (
                      <>
                        <div className="w-1/3 shrink-0 truncate px-2 text-foreground">{entry[0]}</div>
                        <EditableCell
                          value={entry[1]}
                          onSave={(v) => handleEditHash(entry[0], v)}
                          className="flex-1 px-2 text-foreground"
                        />
                      </>
                    );
                  })()}
                {info.type === "list" && (
                  <>
                    <div className="w-16 shrink-0 px-2 text-muted-foreground">{idx}</div>
                    <EditableCell
                      value={(info.value as string[])[idx]}
                      onSave={(v) => handleEditList(idx, v)}
                      className="flex-1 px-2 text-foreground"
                    />
                  </>
                )}
                {info.type === "set" && (
                  <div className="flex flex-1 items-center truncate px-2 text-foreground">
                    <span className="truncate">{(info.value as string[])[idx]}</span>
                  </div>
                )}
                {info.type === "zset" &&
                  (() => {
                    const pair = (info.value as [string, string][])[idx];
                    return (
                      <>
                        <EditableCell
                          value={pair[1]}
                          onSave={(v) => handleEditZsetScore(pair[0], v)}
                          className="w-24 shrink-0 px-2 text-muted-foreground"
                        />
                        <div className="flex-1 truncate px-2 text-foreground">{pair[0]}</div>
                      </>
                    );
                  })()}
                {/* Row delete button */}
                <button
                  className="mr-1 hidden shrink-0 group-hover/row:inline-flex"
                  onClick={() => {
                    let label = "";
                    let action = () => {};
                    if (info.type === "hash") {
                      const field = (info.value as [string, string][])[idx][0];
                      label = field;
                      action = () => handleDeleteHash(field);
                    } else if (info.type === "list") {
                      label = `index ${idx}`;
                      action = () => handleDeleteList(idx);
                    } else if (info.type === "set") {
                      const member = (info.value as string[])[idx];
                      label = member;
                      action = () => handleDeleteSet(member);
                    } else if (info.type === "zset") {
                      const member = (info.value as [string, string][])[idx][0];
                      label = member;
                      action = () => handleDeleteZset(member);
                    }
                    setDeleteTarget({ label, action });
                  }}
                >
                  <Trash2 className="size-3 text-muted-foreground hover:text-destructive" />
                </button>
              </div>
            );
          })}
        </div>
      </div>

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

      {/* Add row */}
      <AddRowForm type={info.type} onAdd={handleAdd} t={t} />

      {/* Delete element confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("query.deleteElementTitle")}
        description={t("query.deleteElementDesc", { name: deleteTarget?.label ?? "" })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={() => {
          deleteTarget?.action();
          setDeleteTarget(null);
        }}
      />
    </div>
  );
}
