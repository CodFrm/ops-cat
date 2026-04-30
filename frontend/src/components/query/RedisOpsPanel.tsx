import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertCircle,
  BarChart3,
  Gauge,
  Info,
  Loader2,
  Monitor,
  RefreshCw,
  Search,
  Server,
  type LucideIcon,
} from "lucide-react";
import { Button, Input, Switch } from "@opskat/ui";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { useQueryStore } from "@/stores/queryStore";
import { ExecuteRedis } from "../../../wailsjs/go/app/App";

interface RedisInfoRow {
  section: string;
  key: string;
  value: string;
}

interface RedisKeyspaceRow {
  db: string;
  keys: number;
  expires: number;
  avgTtl: number;
}

interface RedisInfoDetails {
  values: Record<string, string>;
  rows: RedisInfoRow[];
  keyspace: RedisKeyspaceRow[];
}

interface RedisOpsPanelProps {
  tabId: string;
}

function unwrapRedisInfoResult(raw: string): string {
  try {
    const parsed = JSON.parse(raw) as { value?: unknown };
    return String(parsed.value ?? "");
  } catch {
    return raw;
  }
}

function parseKeyspaceValue(db: string, value: string): RedisKeyspaceRow {
  const item: RedisKeyspaceRow = { db, keys: 0, expires: 0, avgTtl: 0 };
  for (const part of value.split(",")) {
    const [key, raw] = part.split("=");
    const count = Number(raw || 0);
    if (!Number.isFinite(count)) continue;
    if (key === "keys") item.keys = count;
    if (key === "expires") item.expires = count;
    if (key === "avg_ttl") item.avgTtl = count;
  }
  return item;
}

function parseRedisInfoResult(raw: string): RedisInfoDetails {
  const text = unwrapRedisInfoResult(raw);
  const values: Record<string, string> = {};
  const rows: RedisInfoRow[] = [];
  const keyspace: RedisKeyspaceRow[] = [];
  let section = "";

  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith("#")) {
      section = trimmed.replace(/^#+\s*/, "");
      continue;
    }

    const index = trimmed.indexOf(":");
    if (index <= 0) continue;
    const key = trimmed.slice(0, index);
    const value = trimmed.slice(index + 1);
    values[key] = value;
    rows.push({ section, key, value });

    if (/^db\d+$/.test(key)) {
      keyspace.push(parseKeyspaceValue(key, value));
    }
  }

  return { values, rows, keyspace };
}

function formatNumber(value: number | string | undefined): string {
  const number = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(number)) return value ? String(value) : "-";
  return new Intl.NumberFormat().format(number);
}

function pickValue(values: Record<string, string>, key: string, fallback = "-"): string {
  const value = values[key];
  return value === undefined || value === "" ? fallback : value;
}

function InfoPanel({
  title,
  icon: Icon,
  rows,
}: {
  title: string;
  icon: LucideIcon;
  rows: Array<{ label: string; value: string }>;
}) {
  return (
    <section className="min-w-0 rounded-md border bg-background shadow-sm">
      <div className="flex h-11 items-center gap-2 border-b px-4 text-sm font-medium">
        <Icon className="size-4 text-muted-foreground" />
        <span className="truncate">{title}</span>
      </div>
      <div className="space-y-3 p-4">
        {rows.map((row) => (
          <div key={row.label} className="min-w-0 rounded-md border bg-muted/30 px-3 py-2 text-xs">
            <span className="text-muted-foreground">{row.label}:</span>
            <span className="ml-1 break-all font-mono text-emerald-600 dark:text-emerald-400">{row.value}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

export function RedisOpsPanel({ tabId }: RedisOpsPanelProps) {
  const { t } = useTranslation();
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const currentDb = useQueryStore((s) => s.redisStates[tabId]?.currentDb ?? 0);
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const [info, setInfo] = useState<RedisInfoDetails>({ values: {}, rows: [], keyspace: [] });
  const [search, setSearch] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!tabMeta) return;
    setLoading(true);
    setError(null);
    try {
      const infoResult = await ExecuteRedis(tabMeta.assetId, "INFO", currentDb);
      setInfo(parseRedisInfoResult(infoResult || ""));
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  }, [currentDb, tabMeta]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(refresh, 2_000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refresh]);

  const filteredRows = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return info.rows;
    return info.rows.filter((row) => {
      return (
        row.key.toLowerCase().includes(keyword) ||
        row.value.toLowerCase().includes(keyword) ||
        row.section.toLowerCase().includes(keyword)
      );
    });
  }, [info.rows, search]);

  const values = info.values;
  const serverRows = [
    { label: t("query.redisVersion"), value: pickValue(values, "redis_version") },
    { label: t("query.redisOs"), value: pickValue(values, "os") },
    { label: t("query.redisProcessId"), value: pickValue(values, "process_id") },
  ];
  const memoryRows = [
    { label: t("query.redisMemoryUsed"), value: pickValue(values, "used_memory_human") },
    { label: t("query.redisMemoryPeak"), value: pickValue(values, "used_memory_peak_human") },
    {
      label: t("query.redisLuaMemory"),
      value: pickValue(values, "used_memory_lua_human", pickValue(values, "used_memory_lua")),
    },
  ];
  const statusRows = [
    { label: t("query.redisConnectedClients"), value: pickValue(values, "connected_clients") },
    { label: t("query.redisTotalConnections"), value: formatNumber(pickValue(values, "total_connections_received")) },
    { label: t("query.redisTotalCommands"), value: formatNumber(pickValue(values, "total_commands_processed")) },
  ];

  return (
    <div className="flex h-full flex-col overflow-auto bg-background">
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
        {error && (
          <span className="flex min-w-0 items-center gap-1 truncate text-xs text-destructive">
            <AlertCircle className="size-3 shrink-0" />
            <span className="truncate">{error}</span>
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" onClick={refresh} disabled={loading}>
            {loading ? <Loader2 className="size-3 animate-spin" /> : <RefreshCw className="size-3" />}
            {t("query.refreshTree")}
          </Button>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>{t("query.redisAutoRefresh")}</span>
            <Switch checked={autoRefresh} onCheckedChange={setAutoRefresh} />
          </label>
        </div>
      </div>

      <div className="space-y-4 p-3">
        <div className="grid gap-3 xl:grid-cols-3">
          <InfoPanel title={t("query.redisServer")} icon={Server} rows={serverRows} />
          <InfoPanel title={t("query.redisMemoryPanel")} icon={Gauge} rows={memoryRows} />
          <InfoPanel title={t("query.redisRuntimeStatus")} icon={Monitor} rows={statusRows} />
        </div>

        <section className="rounded-md border bg-background shadow-sm">
          <div className="flex h-11 items-center gap-2 border-b px-4 text-sm font-medium">
            <BarChart3 className="size-4 text-muted-foreground" />
            <span>{t("query.redisKeyStats")}</span>
          </div>
          <div className="overflow-auto p-4">
            <table className="w-full min-w-[520px] border-separate border-spacing-0 text-xs">
              <thead>
                <tr className="text-left text-muted-foreground">
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisDb")}</th>
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisKeys")}</th>
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisExpires")}</th>
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisAvgTtl")}</th>
                </tr>
              </thead>
              <tbody>
                {info.keyspace.map((row) => (
                  <tr key={row.db}>
                    <td className="border-b px-2 py-2 font-mono text-muted-foreground">{row.db}</td>
                    <td className="border-b px-2 py-2 font-mono">{formatNumber(row.keys)}</td>
                    <td className="border-b px-2 py-2 font-mono">{formatNumber(row.expires)}</td>
                    <td className="border-b px-2 py-2 font-mono">{formatNumber(row.avgTtl)}</td>
                  </tr>
                ))}
                {!loading && info.keyspace.length === 0 && (
                  <tr>
                    <td className="px-2 py-6 text-center text-muted-foreground" colSpan={4}>
                      {t("query.redisOpsEmpty")}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="rounded-md border bg-background shadow-sm">
          <div className="flex h-11 items-center gap-2 border-b px-4 text-sm font-medium">
            <Info className="size-4 text-muted-foreground" />
            <span>{t("query.redisInfoFull")}</span>
            <div className="relative ml-auto w-64 max-w-[45%]">
              <Search className="absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="h-7 pl-7 text-xs"
                placeholder={t("query.redisInfoSearch")}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          </div>
          <div className="overflow-auto p-4">
            <table className="w-full min-w-[620px] border-separate border-spacing-0 text-xs">
              <thead>
                <tr className="text-left text-muted-foreground">
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisInfoKey")}</th>
                  <th className="border-b px-2 py-2 font-medium">{t("query.redisInfoValue")}</th>
                </tr>
              </thead>
              <tbody>
                {filteredRows.map((row) => (
                  <tr key={`${row.section}-${row.key}`} className="odd:bg-muted/20">
                    <td className="border-b px-2 py-2 font-mono text-muted-foreground">{row.key}</td>
                    <td className="border-b px-2 py-2 font-mono break-all">{row.value}</td>
                  </tr>
                ))}
                {!loading && filteredRows.length === 0 && (
                  <tr>
                    <td className="px-2 py-6 text-center text-muted-foreground" colSpan={2}>
                      {t("query.redisOpsEmpty")}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  );
}
