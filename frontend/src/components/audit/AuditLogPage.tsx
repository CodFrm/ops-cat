import { useState, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, XCircle, ChevronLeft, ChevronRight, Info, RefreshCw, Unplug, Calendar } from "lucide-react";
import {
  Button,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Tabs,
  TabsList,
  TabsTrigger,
} from "@opskat/ui";
import { ListAuditLogs, ListAuditSessions, GetSSHPoolConnections } from "../../../wailsjs/go/app/App";
import { audit_entity, audit_repo, sshpool } from "../../../wailsjs/go/models";

const PAGE_SIZE = 20;

// 时间范围预设
const TIME_PRESETS = [
  { value: "1h", seconds: 3600 },
  { value: "3h", seconds: 10800 },
  { value: "6h", seconds: 21600 },
  { value: "1d", seconds: 86400 },
  { value: "7d", seconds: 604800 },
] as const;

// 决策来源标签样式
function decisionSourceBadge(source: string): { label: string; className: string } {
  switch (source) {
    case "policy_allow":
      return { label: "policy", className: "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300" };
    case "policy_deny":
      return { label: "policy", className: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300" };
    case "user_allow":
      return { label: "user", className: "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300" };
    case "user_deny":
      return { label: "user", className: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300" };
    case "grant_allow":
      return { label: "grant", className: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300" };
    case "grant_deny":
      return { label: "grant", className: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300" };
    default:
      return { label: source || "-", className: "bg-muted" };
  }
}

// 将 unix timestamp 按日期分组
function getSessionGroup(ts: number): "today" | "yesterday" | "thisWeek" | "earlier" {
  const now = new Date();
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000;
  const yesterdayStart = todayStart - 86400;
  const weekStart = todayStart - (now.getDay() || 7) * 86400 + 86400; // 本周一

  if (ts >= todayStart) return "today";
  if (ts >= yesterdayStart) return "yesterday";
  if (ts >= weekStart) return "thisWeek";
  return "earlier";
}

function fromDatetimeLocal(s: string): number {
  if (!s) return 0;
  return Math.floor(new Date(s).getTime() / 1000);
}

function formatDatetimeCompact(s: string): string {
  if (!s) return "...";
  const d = new Date(s);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function AuditLogPage() {
  const { t } = useTranslation();
  const [logs, setLogs] = useState<audit_entity.AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [sessionFilter, setSessionFilter] = useState("");
  const [timeRange, setTimeRange] = useState("1d");
  const [customStart, setCustomStart] = useState("");
  const [customEnd, setCustomEnd] = useState("");
  const [timePickerOpen, setTimePickerOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [detailLog, setDetailLog] = useState<audit_entity.AuditLog | null>(null);
  const [activeTab, setActiveTab] = useState("logs");
  const [poolEntries, setPoolEntries] = useState<sshpool.PoolEntryInfo[]>([]);
  const [sessions, setSessions] = useState<audit_repo.SessionInfo[]>([]);

  // 计算实际的 startTime
  const computeStartTime = useCallback(() => {
    if (timeRange === "custom") {
      return fromDatetimeLocal(customStart);
    }
    if (timeRange === "0") return 0; // 全部
    const preset = TIME_PRESETS.find((r) => r.value === timeRange);
    return preset ? Math.floor(Date.now() / 1000) - preset.seconds : 0;
  }, [timeRange, customStart]);

  const computeEndTime = useCallback(() => {
    if (timeRange === "custom") {
      return fromDatetimeLocal(customEnd);
    }
    return 0;
  }, [timeRange, customEnd]);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const startTime = computeStartTime();
      const endTime = computeEndTime();
      const result = await ListAuditLogs("", 0, startTime, endTime, page * PAGE_SIZE, PAGE_SIZE, sessionFilter);
      setLogs(result?.items || []);
      setTotal(result?.total || 0);
    } finally {
      setLoading(false);
    }
  }, [sessionFilter, computeStartTime, computeEndTime, page]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  // 加载会话列表
  const fetchSessions = useCallback(async () => {
    try {
      const startTime = computeStartTime();
      const result = await ListAuditSessions(startTime);
      setSessions(result || []);
    } catch {
      setSessions([]);
    }
  }, [computeStartTime]);

  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  const fetchPool = useCallback(async () => {
    try {
      const entries = await GetSSHPoolConnections();
      setPoolEntries(entries || []);
    } catch {
      setPoolEntries([]);
    }
  }, []);

  useEffect(() => {
    if (activeTab === "pool") {
      fetchPool();
    }
  }, [activeTab, fetchPool]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const formatTime = (ts: number) => {
    if (!ts) return "-";
    const d = new Date(ts * 1000);
    return d.toLocaleString();
  };

  const formatTimeShort = (ts: number) => {
    if (!ts) return "";
    const d = new Date(ts * 1000);
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  };

  const truncate = (s: string, max = 60) => {
    if (!s) return "-";
    return s.length > max ? s.slice(0, max) + "..." : s;
  };

  // 按时间分组会话
  const groupedSessions = useMemo(() => {
    const groups: Record<string, audit_repo.SessionInfo[]> = {};
    for (const s of sessions) {
      const group = getSessionGroup(s.last_time);
      if (!groups[group]) groups[group] = [];
      groups[group].push(s);
    }
    return groups;
  }, [sessions]);

  // 从当前日志中聚合 session 已允许的模式（grant_submit 记录）
  const sessionApprovedPatterns = useMemo(() => {
    if (!sessionFilter) return [];
    return logs
      .filter((l) => l.ToolName === "grant_submit" && l.Command)
      .map((l) => ({ asset: l.AssetName || "-", patterns: l.Command }));
  }, [logs, sessionFilter]);

  const groupOrder = ["today", "yesterday", "thisWeek", "earlier"] as const;
  const groupLabels: Record<string, string> = {
    today: t("audit.sessionGroupToday"),
    yesterday: t("audit.sessionGroupYesterday"),
    thisWeek: t("audit.sessionGroupThisWeek"),
    earlier: t("audit.sessionGroupEarlier"),
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header with tabs */}
      <div className="px-4 py-3 border-b flex items-center justify-between gap-4">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="h-8">
            <TabsTrigger value="logs" className="text-xs px-3">
              {t("audit.tabLogs")}
            </TabsTrigger>
            <TabsTrigger value="pool" className="text-xs px-3">
              <Unplug className="h-3.5 w-3.5 mr-1" />
              {t("audit.tabPool")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {activeTab === "logs" && (
          <div className="flex items-center gap-2">
            {/* 会话筛选 */}
            <Select
              value={sessionFilter || "all"}
              onValueChange={(v) => {
                setSessionFilter(v === "all" ? "" : v);
                setPage(0);
              }}
            >
              <SelectTrigger className="w-44 h-8 text-xs font-mono">
                <SelectValue placeholder={t("audit.sessionAll")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("audit.sessionAll")}</SelectItem>
                {groupOrder.map((group) => {
                  const items = groupedSessions[group];
                  if (!items || items.length === 0) return null;
                  return (
                    <SelectGroup key={group}>
                      <SelectLabel>{groupLabels[group]}</SelectLabel>
                      {items.map((s) => (
                        <SelectItem key={s.session_id} value={s.session_id}>
                          <span className="truncate">{s.session_id.slice(0, 8)}</span>
                          <span className="text-muted-foreground ml-1">
                            {formatTimeShort(s.first_time)} ({t("audit.sessionOps", { count: s.count })})
                          </span>
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  );
                })}
              </SelectContent>
            </Select>
            {/* 时间范围 */}
            <Popover open={timePickerOpen} onOpenChange={setTimePickerOpen}>
              <PopoverTrigger asChild>
                <Button variant="outline" size="sm" className="h-8 text-xs gap-1.5 font-normal">
                  <Calendar className="h-3.5 w-3.5 text-muted-foreground" />
                  {timeRange === "custom"
                    ? `${formatDatetimeCompact(customStart)} - ${formatDatetimeCompact(customEnd)}`
                    : timeRange === "0"
                      ? t("audit.timeAll")
                      : t(`audit.time${timeRange}`)}
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-auto p-3" align="end">
                {/* 快捷预设 */}
                <div className="flex flex-wrap gap-1.5 mb-3">
                  {[
                    { value: "0", label: t("audit.timeAll") },
                    ...TIME_PRESETS.map((p) => ({ value: p.value, label: t(`audit.time${p.value}`) })),
                  ].map((p) => (
                    <Button
                      key={p.value}
                      variant={timeRange === p.value ? "default" : "outline"}
                      size="sm"
                      className="h-7 text-xs px-2.5"
                      onClick={() => {
                        setTimeRange(p.value);
                        setPage(0);
                        setTimePickerOpen(false);
                      }}
                    >
                      {p.label}
                    </Button>
                  ))}
                </div>
                {/* 分隔线 */}
                <div className="border-t mb-3" />
                {/* 自定义时间 */}
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <label className="text-xs text-muted-foreground shrink-0 w-10">{t("audit.timeStart")}</label>
                    <input
                      type="datetime-local"
                      value={customStart}
                      onChange={(e) => {
                        setCustomStart(e.target.value);
                        setTimeRange("custom");
                        setPage(0);
                      }}
                      className="flex-1 text-xs border rounded-md px-2 py-1.5 bg-background outline-none focus-visible:ring-1 focus-visible:ring-ring/45"
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <label className="text-xs text-muted-foreground shrink-0 w-10">{t("audit.timeEnd")}</label>
                    <input
                      type="datetime-local"
                      value={customEnd}
                      onChange={(e) => {
                        setCustomEnd(e.target.value);
                        setTimeRange("custom");
                        setPage(0);
                      }}
                      className="flex-1 text-xs border rounded-md px-2 py-1.5 bg-background outline-none focus-visible:ring-1 focus-visible:ring-ring/45"
                    />
                  </div>
                </div>
              </PopoverContent>
            </Popover>
            <span className="text-xs text-muted-foreground">{t("audit.total", { total })}</span>
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={fetchLogs} disabled={loading}>
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            </Button>
          </div>
        )}
        {activeTab === "pool" && (
          <Button variant="ghost" size="sm" className="h-8" onClick={fetchPool}>
            <RefreshCw className="h-3.5 w-3.5 mr-1" />
            {t("audit.poolRefresh")}
          </Button>
        )}
      </div>

      {/* Logs tab content */}
      {activeTab === "logs" && (
        <>
          {/* Session 已允许模式汇总 */}
          {sessionApprovedPatterns.length > 0 && (
            <div className="px-4 py-2 border-b bg-blue-50 dark:bg-blue-950/30 text-xs">
              <span className="font-medium text-blue-700 dark:text-blue-300">{t("audit.sessionPatterns")}:</span>
              <div className="flex flex-wrap gap-1.5 mt-1">
                {sessionApprovedPatterns.map((p, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 bg-blue-100 dark:bg-blue-900/50 text-blue-800 dark:text-blue-200 px-2 py-0.5 rounded font-mono"
                  >
                    {p.asset !== "-" && <span className="text-blue-500">{p.asset}:</span>}
                    {p.patterns}
                  </span>
                ))}
              </div>
            </div>
          )}
          <div className="flex-1 overflow-auto">
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-background border-b">
                <tr className="text-left text-muted-foreground">
                  <th className="px-4 py-2 font-medium">{t("audit.time")}</th>
                  <th className="px-4 py-2 font-medium">{t("audit.toolName")}</th>
                  <th className="px-4 py-2 font-medium">{t("audit.assetName")}</th>
                  <th className="px-4 py-2 font-medium">{t("audit.command")}</th>
                  <th className="px-4 py-2 font-medium">{t("audit.decision")}</th>
                  <th className="px-4 py-2 font-medium w-16 text-center">{t("audit.result")}</th>
                  <th className="px-4 py-2 font-medium w-16"></th>
                </tr>
              </thead>
              <tbody>
                {logs.length === 0 && !loading && (
                  <tr>
                    <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">
                      {t("audit.empty")}
                    </td>
                  </tr>
                )}
                {logs.map((log) => {
                  const badge = decisionSourceBadge(log.DecisionSource);
                  return (
                    <tr key={log.ID} className="border-b hover:bg-muted/50 transition-colors">
                      <td className="px-4 py-2 text-xs text-muted-foreground whitespace-nowrap">
                        {formatTime(log.Createtime)}
                      </td>
                      <td className="px-4 py-2 font-mono text-xs">{log.ToolName}</td>
                      <td className="px-4 py-2">{log.AssetName || "-"}</td>
                      <td className="px-4 py-2 font-mono text-xs max-w-48 truncate" title={log.Command}>
                        {truncate(log.Command)}
                      </td>
                      <td className="px-4 py-2">
                        {log.DecisionSource ? (
                          <span
                            className={`inline-block px-1.5 py-0.5 text-xs rounded font-mono ${badge.className}`}
                            title={
                              log.MatchedPattern ? `${t("audit.matchedPattern")}: ${log.MatchedPattern}` : undefined
                            }
                          >
                            {log.Decision === "allow" ? "\u2713" : "\u2717"} {badge.label}
                          </span>
                        ) : (
                          "-"
                        )}
                      </td>
                      <td className="px-4 py-2 text-center">
                        {log.Success === 1 ? (
                          <CheckCircle2 className="h-4 w-4 text-green-500 mx-auto" />
                        ) : (
                          <XCircle className="h-4 w-4 text-destructive mx-auto" />
                        )}
                      </td>
                      <td className="px-4 py-2">
                        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setDetailLog(log)}>
                          <Info className="h-3.5 w-3.5" />
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="px-4 py-2 border-t flex items-center justify-center gap-2">
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                disabled={page === 0}
                onClick={() => setPage(page - 1)}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="text-sm text-muted-foreground">
                {page + 1} / {totalPages}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                disabled={page >= totalPages - 1}
                onClick={() => setPage(page + 1)}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          )}
        </>
      )}

      {/* Pool tab content */}
      {activeTab === "pool" && (
        <div className="flex-1 overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-background border-b">
              <tr className="text-left text-muted-foreground">
                <th className="px-4 py-2 font-medium">{t("audit.poolAsset")}</th>
                <th className="px-4 py-2 font-medium">{t("audit.poolRefCount")}</th>
                <th className="px-4 py-2 font-medium">{t("audit.poolLastUsed")}</th>
              </tr>
            </thead>
            <tbody>
              {poolEntries.length === 0 && (
                <tr>
                  <td colSpan={3} className="px-4 py-12 text-center text-muted-foreground">
                    {t("audit.poolEmpty")}
                  </td>
                </tr>
              )}
              {poolEntries.map((entry) => (
                <tr key={entry.asset_id} className="border-b hover:bg-muted/50 transition-colors">
                  <td className="px-4 py-2 font-mono">{entry.asset_id}</td>
                  <td className="px-4 py-2">
                    <span
                      className={`inline-block px-1.5 py-0.5 text-xs rounded font-mono ${
                        entry.ref_count > 0
                          ? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
                          : "bg-muted"
                      }`}
                    >
                      {entry.ref_count}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">{formatTime(entry.last_used)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Detail dialog */}
      <Dialog open={!!detailLog} onOpenChange={(open) => !open && setDetailLog(null)}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("audit.detail")}</DialogTitle>
          </DialogHeader>
          {detailLog && (
            <div className="space-y-3 text-sm">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <span className="text-muted-foreground">{t("audit.toolName")}:</span>{" "}
                  <span className="font-mono">{detailLog.ToolName}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">{t("audit.assetName")}:</span> {detailLog.AssetName || "-"}
                </div>
                <div>
                  <span className="text-muted-foreground">{t("audit.result")}:</span>{" "}
                  {detailLog.Success === 1 ? (
                    <span className="text-green-500">{t("audit.success")}</span>
                  ) : (
                    <span className="text-destructive">{t("audit.failed")}</span>
                  )}
                </div>
                <div>
                  <span className="text-muted-foreground">{t("audit.decision")}:</span>{" "}
                  <span className="font-mono">{detailLog.Decision || "-"}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">{t("audit.decisionSource")}:</span>{" "}
                  <span className="font-mono">{detailLog.DecisionSource || "-"}</span>
                </div>
                {detailLog.MatchedPattern && (
                  <div className="col-span-2">
                    <span className="text-muted-foreground">{t("audit.matchedPattern")}:</span>{" "}
                    <code className="font-mono bg-muted px-1 rounded">{detailLog.MatchedPattern}</code>
                  </div>
                )}
                {detailLog.SessionID && (
                  <div className="col-span-2">
                    <span className="text-muted-foreground">{t("audit.sessionID")}:</span>{" "}
                    <code className="font-mono text-xs bg-muted px-1 rounded">{detailLog.SessionID}</code>
                  </div>
                )}
                <div className="col-span-2">
                  <span className="text-muted-foreground">{t("audit.time")}:</span> {formatTime(detailLog.Createtime)}
                </div>
              </div>

              {detailLog.Command && (
                <div>
                  <div className="text-muted-foreground mb-1">{t("audit.command")}</div>
                  <pre className="bg-muted p-2 rounded text-xs font-mono whitespace-pre-wrap break-all">
                    {detailLog.Command}
                  </pre>
                </div>
              )}

              {detailLog.Request && (
                <div>
                  <div className="text-muted-foreground mb-1">{t("audit.request")}</div>
                  <pre className="bg-muted p-2 rounded text-xs font-mono whitespace-pre-wrap break-all max-h-40 overflow-y-auto">
                    {detailLog.Request}
                  </pre>
                </div>
              )}

              {detailLog.Result && (
                <div>
                  <div className="text-muted-foreground mb-1">{t("audit.response")}</div>
                  <pre className="bg-muted p-2 rounded text-xs font-mono whitespace-pre-wrap break-all max-h-40 overflow-y-auto">
                    {detailLog.Result}
                  </pre>
                </div>
              )}

              {detailLog.Error && (
                <div>
                  <div className="text-muted-foreground mb-1">{t("audit.error")}</div>
                  <pre className="bg-muted p-2 rounded text-xs font-mono whitespace-pre-wrap break-all text-destructive">
                    {detailLog.Error}
                  </pre>
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
