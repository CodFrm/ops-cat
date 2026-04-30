import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
} from "@opskat/ui";
import {
  RedisHashSet,
  RedisListPush,
  RedisScanKeys,
  RedisSetAdd,
  RedisSetKeyTTL,
  RedisSetStringValue,
  RedisStreamAdd,
  RedisZSetAdd,
} from "../../../wailsjs/go/app/App";

type RedisCreateType = "string" | "hash" | "list" | "set" | "zset" | "stream";

interface PairDraft {
  field: string;
  value: string;
}

interface ValueDraft {
  value: string;
}

interface ZSetDraft {
  member: string;
  score: string;
}

interface RedisCreateKeyDialogProps {
  open: boolean;
  assetId: number;
  db: number;
  dbOptions?: number[];
  onOpenChange: (open: boolean) => void;
  onCreated: (key: string, db: number) => void | Promise<void>;
}

const CREATE_TYPES: RedisCreateType[] = ["string", "hash", "list", "set", "zset", "stream"];

function emptyPair(): PairDraft {
  return { field: "", value: "" };
}

function emptyValue(): ValueDraft {
  return { value: "" };
}

function emptyZSet(): ZSetDraft {
  return { member: "", score: "0" };
}

function parseTtlSeconds(raw: string): number | null {
  const value = Number(raw.trim());
  if (!Number.isInteger(value) || value < -1) return null;
  return value;
}

export function RedisCreateKeyDialog({
  open,
  assetId,
  db,
  dbOptions,
  onOpenChange,
  onCreated,
}: RedisCreateKeyDialogProps) {
  const { t } = useTranslation();
  const [type, setType] = useState<RedisCreateType>("string");
  const [targetDb, setTargetDb] = useState(String(db));
  const [keyName, setKeyName] = useState("");
  const [ttl, setTtl] = useState("-1");
  const [stringValue, setStringValue] = useState("");
  const [hashRows, setHashRows] = useState<PairDraft[]>([emptyPair()]);
  const [listRows, setListRows] = useState<ValueDraft[]>([emptyValue()]);
  const [setRows, setSetRows] = useState<ValueDraft[]>([emptyValue()]);
  const [zsetRows, setZsetRows] = useState<ZSetDraft[]>([emptyZSet()]);
  const [streamEntryId, setStreamEntryId] = useState("*");
  const [streamRows, setStreamRows] = useState<PairDraft[]>([emptyPair()]);
  const [submitting, setSubmitting] = useState(false);

  const reset = useCallback(
    (initialDb = db) => {
      setType("string");
      setTargetDb(String(initialDb));
      setKeyName("");
      setTtl("-1");
      setStringValue("");
      setHashRows([emptyPair()]);
      setListRows([emptyValue()]);
      setSetRows([emptyValue()]);
      setZsetRows([emptyZSet()]);
      setStreamEntryId("*");
      setStreamRows([emptyPair()]);
      setSubmitting(false);
    },
    [db]
  );

  useEffect(() => {
    if (open) {
      reset(db);
    }
  }, [db, open, reset]);

  const close = useCallback(() => {
    reset();
    onOpenChange(false);
  }, [onOpenChange, reset]);

  const submit = useCallback(async () => {
    const key = keyName.trim();
    if (!key) {
      toast.error(t("query.redisKeyNameRequired"));
      return;
    }

    const parsedDb = Number(targetDb);
    const ttlSeconds = parseTtlSeconds(ttl);
    if (!Number.isInteger(parsedDb) || parsedDb < 0) {
      toast.error(t("query.redisDbInvalid"));
      return;
    }
    if (ttlSeconds === null) {
      toast.error(t("query.redisTtlInvalid"));
      return;
    }

    const hashValues = hashRows.filter((row) => row.field.trim() !== "");
    const listValues = listRows.map((row) => row.value).filter((value) => value !== "");
    const setValues = Array.from(new Set(setRows.map((row) => row.value.trim()).filter(Boolean)));
    const zsetValues = zsetRows.filter((row) => row.member.trim() !== "");
    const streamValues = streamRows.filter((row) => row.field.trim() !== "");

    if (type === "hash" && hashValues.length === 0) {
      toast.error(t("query.redisFieldRequired"));
      return;
    }
    if ((type === "list" || type === "set") && (type === "list" ? listValues.length === 0 : setValues.length === 0)) {
      toast.error(t("query.redisElementRequired"));
      return;
    }
    if (type === "zset") {
      if (zsetValues.length === 0) {
        toast.error(t("query.redisMemberRequired"));
        return;
      }
      if (zsetValues.some((row) => Number.isNaN(Number(row.score)))) {
        toast.error(t("query.redisScoreInvalid"));
        return;
      }
    }
    if (type === "stream" && streamValues.length === 0) {
      toast.error(t("query.redisFieldRequired"));
      return;
    }

    setSubmitting(true);
    try {
      const existing = await RedisScanKeys({
        assetId,
        db: parsedDb,
        cursor: "0",
        match: key,
        type: "",
        count: 1,
        exact: true,
      });
      if ((existing.keys || []).includes(key)) {
        toast.error(t("query.redisKeyAlreadyExists", { key }));
        setSubmitting(false);
        return;
      }

      if (type === "string") {
        await RedisSetStringValue({ assetId, db: parsedDb, key, value: stringValue, format: "raw" });
      } else if (type === "hash") {
        for (const row of hashValues) {
          await RedisHashSet(assetId, parsedDb, key, row.field.trim(), row.value);
        }
      } else if (type === "list") {
        for (const value of listValues) {
          await RedisListPush(assetId, parsedDb, key, value);
        }
      } else if (type === "set") {
        for (const member of setValues) {
          await RedisSetAdd(assetId, parsedDb, key, member);
        }
      } else if (type === "zset") {
        for (const row of zsetValues) {
          await RedisZSetAdd(assetId, parsedDb, key, row.member.trim(), Number(row.score));
        }
      } else {
        await RedisStreamAdd(
          assetId,
          parsedDb,
          key,
          streamEntryId.trim() || "*",
          streamValues.map((row) => ({ field: row.field.trim(), value: row.value }))
        );
      }

      if (ttlSeconds > 0) {
        await RedisSetKeyTTL(assetId, parsedDb, key, ttlSeconds);
      }
      await onCreated(key, parsedDb);
      close();
    } catch (err) {
      toast.error(String(err));
      setSubmitting(false);
    }
  }, [
    assetId,
    close,
    hashRows,
    keyName,
    listRows,
    onCreated,
    setRows,
    streamEntryId,
    streamRows,
    stringValue,
    targetDb,
    t,
    ttl,
    type,
    zsetRows,
  ]);

  const removePairRow = (rows: PairDraft[], setter: (rows: PairDraft[]) => void, index: number) => {
    if (rows.length <= 1) return;
    setter(rows.filter((_, itemIndex) => itemIndex !== index));
  };
  const removeValueRow = (rows: ValueDraft[], setter: (rows: ValueDraft[]) => void, index: number) => {
    if (rows.length <= 1) return;
    setter(rows.filter((_, itemIndex) => itemIndex !== index));
  };
  const removeZSetRow = (index: number) => {
    if (zsetRows.length <= 1) return;
    setZsetRows((rows) => rows.filter((_, itemIndex) => itemIndex !== index));
  };

  const dbChoices =
    dbOptions && dbOptions.length > 0 ? dbOptions : Array.from({ length: Math.max(16, db + 1) }, (_, i) => i);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) {
          onOpenChange(true);
        } else if (!submitting) {
          close();
        }
      }}
    >
      <DialogContent className="max-w-2xl" showCloseButton={!submitting}>
        <DialogHeader>
          <DialogTitle>{t("query.createRedisKey")}</DialogTitle>
        </DialogHeader>

        <div className="max-h-[70vh] space-y-4 overflow-y-auto pr-1">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground">{t("query.redisKeyName")}</label>
              <Input
                data-testid="redis-create-key-input"
                className="h-8 font-mono text-xs"
                placeholder={t("query.redisKeyNamePlaceholder")}
                value={keyName}
                onChange={(e) => setKeyName(e.target.value)}
                disabled={submitting}
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground">{t("query.redisDbIndex")}</label>
              <Select value={targetDb} onValueChange={setTargetDb} disabled={submitting}>
                <SelectTrigger className="h-8 w-full text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {dbChoices.map((item) => (
                    <SelectItem key={item} value={String(item)}>
                      db{item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground">{t("query.redisKeyType")}</label>
              <Select value={type} onValueChange={(val) => setType(val as RedisCreateType)} disabled={submitting}>
                <SelectTrigger data-testid="redis-create-type-trigger" className="h-8 w-full text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CREATE_TYPES.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground">{t("query.ttl")}</label>
              <div className="flex gap-2">
                <Input
                  data-testid="redis-create-ttl-input"
                  className="h-8 font-mono text-xs"
                  placeholder={t("query.redisTtlPlaceholder")}
                  value={ttl}
                  onChange={(e) => setTtl(e.target.value)}
                  disabled={submitting}
                />
                <Button type="button" variant="outline" size="sm" className="h-8 text-xs" onClick={() => setTtl("-1")}>
                  {t("query.redisTtlForever")}
                </Button>
              </div>
            </div>
          </div>

          <div className="space-y-2">
            <div className="text-xs font-medium text-muted-foreground">{t("query.redisInitialValues")}</div>

            {type === "string" && (
              <Textarea
                data-testid="redis-create-string-value"
                className="min-h-28 font-mono text-xs"
                placeholder={t("query.newValue")}
                value={stringValue}
                onChange={(e) => setStringValue(e.target.value)}
                disabled={submitting}
              />
            )}

            {type === "hash" &&
              hashRows.map((row, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2">
                  <Input
                    data-testid={`redis-create-hash-field-${index}`}
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newField")}
                    value={row.field}
                    onChange={(e) =>
                      setHashRows((rows) =>
                        rows.map((item, itemIndex) => (itemIndex === index ? { ...item, field: e.target.value } : item))
                      )
                    }
                    disabled={submitting}
                  />
                  <Input
                    data-testid={`redis-create-hash-value-${index}`}
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newValue")}
                    value={row.value}
                    onChange={(e) =>
                      setHashRows((rows) =>
                        rows.map((item, itemIndex) => (itemIndex === index ? { ...item, value: e.target.value } : item))
                      )
                    }
                    disabled={submitting}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    title={t("query.redisRemoveInitialRow")}
                    onClick={() => removePairRow(hashRows, setHashRows, index)}
                    disabled={submitting || hashRows.length <= 1}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              ))}

            {type === "list" &&
              listRows.map((row, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
                  <Input
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newValue")}
                    value={row.value}
                    onChange={(e) =>
                      setListRows((rows) =>
                        rows.map((item, itemIndex) => (itemIndex === index ? { value: e.target.value } : item))
                      )
                    }
                    disabled={submitting}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    title={t("query.redisRemoveInitialRow")}
                    onClick={() => removeValueRow(listRows, setListRows, index)}
                    disabled={submitting || listRows.length <= 1}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              ))}

            {type === "set" &&
              setRows.map((row, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
                  <Input
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newMember")}
                    value={row.value}
                    onChange={(e) =>
                      setSetRows((rows) =>
                        rows.map((item, itemIndex) => (itemIndex === index ? { value: e.target.value } : item))
                      )
                    }
                    disabled={submitting}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    title={t("query.redisRemoveInitialRow")}
                    onClick={() => removeValueRow(setRows, setSetRows, index)}
                    disabled={submitting || setRows.length <= 1}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              ))}

            {type === "zset" &&
              zsetRows.map((row, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_96px_auto] gap-2">
                  <Input
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newMember")}
                    value={row.member}
                    onChange={(e) =>
                      setZsetRows((rows) =>
                        rows.map((item, itemIndex) =>
                          itemIndex === index ? { ...item, member: e.target.value } : item
                        )
                      )
                    }
                    disabled={submitting}
                  />
                  <Input
                    className="h-8 font-mono text-xs"
                    placeholder={t("query.newScore")}
                    value={row.score}
                    onChange={(e) =>
                      setZsetRows((rows) =>
                        rows.map((item, itemIndex) => (itemIndex === index ? { ...item, score: e.target.value } : item))
                      )
                    }
                    disabled={submitting}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    title={t("query.redisRemoveInitialRow")}
                    onClick={() => removeZSetRow(index)}
                    disabled={submitting || zsetRows.length <= 1}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              ))}

            {type === "stream" && (
              <>
                <Input
                  className="h-8 font-mono text-xs"
                  placeholder={t("query.streamEntryId")}
                  value={streamEntryId}
                  onChange={(e) => setStreamEntryId(e.target.value)}
                  disabled={submitting}
                />
                {streamRows.map((row, index) => (
                  <div key={index} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2">
                    <Input
                      className="h-8 font-mono text-xs"
                      placeholder={t("query.streamField")}
                      value={row.field}
                      onChange={(e) =>
                        setStreamRows((rows) =>
                          rows.map((item, itemIndex) =>
                            itemIndex === index ? { ...item, field: e.target.value } : item
                          )
                        )
                      }
                      disabled={submitting}
                    />
                    <Input
                      className="h-8 font-mono text-xs"
                      placeholder={t("query.streamValue")}
                      value={row.value}
                      onChange={(e) =>
                        setStreamRows((rows) =>
                          rows.map((item, itemIndex) =>
                            itemIndex === index ? { ...item, value: e.target.value } : item
                          )
                        )
                      }
                      disabled={submitting}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      title={t("query.redisRemoveInitialRow")}
                      onClick={() => removePairRow(streamRows, setStreamRows, index)}
                      disabled={submitting || streamRows.length <= 1}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                ))}
              </>
            )}

            {type !== "string" && (
              <Button
                data-testid="redis-create-add-row"
                type="button"
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 text-xs"
                onClick={() => {
                  if (type === "hash") setHashRows((rows) => [...rows, emptyPair()]);
                  if (type === "list") setListRows((rows) => [...rows, emptyValue()]);
                  if (type === "set") setSetRows((rows) => [...rows, emptyValue()]);
                  if (type === "zset") setZsetRows((rows) => [...rows, emptyZSet()]);
                  if (type === "stream") setStreamRows((rows) => [...rows, emptyPair()]);
                }}
                disabled={submitting}
              >
                <Plus className="size-3.5" />
                {t("query.redisAddInitialRow")}
              </Button>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={close} disabled={submitting}>
            {t("action.cancel")}
          </Button>
          <Button size="sm" onClick={submit} disabled={submitting}>
            {submitting ? <Loader2 className="mr-1 size-3 animate-spin" /> : null}
            {t("query.createRedisKeySubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
