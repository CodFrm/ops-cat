import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Activity, AlertCircle, Database, ListTree, Loader2, RefreshCw, Search, Server, Users } from "lucide-react";
import { Button, Input } from "@opskat/ui";
import {
  type KafkaConsumerGroup,
  type KafkaConsumerGroupDetail,
  type KafkaTabState,
  type KafkaTopicSummary,
  type KafkaView,
  useKafkaStore,
} from "@/stores/kafkaStore";

interface KafkaPanelProps {
  tabId: string;
}

const VIEWS: { id: KafkaView; icon: typeof Activity; labelKey: string }[] = [
  { id: "overview", icon: Activity, labelKey: "query.kafkaOverview" },
  { id: "brokers", icon: Server, labelKey: "query.kafkaBrokers" },
  { id: "topics", icon: ListTree, labelKey: "query.kafkaTopics" },
  { id: "consumerGroups", icon: Users, labelKey: "query.kafkaConsumerGroups" },
];

function EmptyState({ text }: { text: string }) {
  return <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">{text}</div>;
}

function LoadingBlock() {
  return (
    <div className="flex h-32 items-center justify-center">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-md border bg-background px-3 py-2">
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-1 text-xl font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function StatusPill({ value }: { value?: string }) {
  if (!value) return <span className="text-muted-foreground">-</span>;
  return (
    <span className="inline-flex rounded border bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase text-muted-foreground">
      {value}
    </span>
  );
}

export function KafkaPanel({ tabId }: KafkaPanelProps) {
  const { t } = useTranslation();
  const state = useKafkaStore((s) => s.states[tabId]);
  const ensureTab = useKafkaStore((s) => s.ensureTab);
  const setActiveView = useKafkaStore((s) => s.setActiveView);
  const refreshActiveView = useKafkaStore((s) => s.refreshActiveView);
  const loadOverview = useKafkaStore((s) => s.loadOverview);
  const loadBrokers = useKafkaStore((s) => s.loadBrokers);
  const loadTopics = useKafkaStore((s) => s.loadTopics);
  const loadConsumerGroups = useKafkaStore((s) => s.loadConsumerGroups);

  useEffect(() => {
    ensureTab(tabId);
    loadOverview(tabId);
    loadBrokers(tabId);
    loadTopics(tabId);
    loadConsumerGroups(tabId);
  }, [ensureTab, loadBrokers, loadConsumerGroups, loadOverview, loadTopics, tabId]);

  const current = state || defaultPanelState();
  const busy =
    current.loadingOverview || current.loadingBrokers || current.loadingTopics || current.loadingGroups || false;
  const activeLabel = t(VIEWS.find((view) => view.id === current.activeView)?.labelKey || "query.kafkaOverview");

  return (
    <div className="flex h-full w-full overflow-hidden">
      <aside className="w-56 shrink-0 border-r bg-muted/20">
        <div className="border-b px-3 py-2">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Kafka</div>
        </div>
        <nav className="p-2">
          {VIEWS.map((view) => {
            const Icon = view.icon;
            const active = current.activeView === view.id;
            return (
              <button
                key={view.id}
                type="button"
                className={`flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-sm transition-colors ${
                  active ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:bg-background/70"
                }`}
                onClick={() => setActiveView(tabId, view.id)}
              >
                <Icon className="h-4 w-4 shrink-0" />
                <span className="truncate">{t(view.labelKey)}</span>
              </button>
            );
          })}
        </nav>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col">
        <div className="flex h-11 shrink-0 items-center justify-between border-b px-4">
          <div className="text-sm font-semibold">{activeLabel}</div>
          <div className="flex items-center gap-2">
            {current.error && (
              <span className="flex max-w-[480px] items-center gap-1 truncate text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                <span className="truncate">{current.error}</span>
              </span>
            )}
            <Button variant="outline" size="sm" className="h-7 gap-1.5" onClick={() => refreshActiveView(tabId)}>
              {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              {t("query.refreshTree")}
            </Button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {current.activeView === "overview" && <OverviewView state={current} />}
          {current.activeView === "brokers" && <BrokersView state={current} />}
          {current.activeView === "topics" && <TopicsView tabId={tabId} state={current} />}
          {current.activeView === "consumerGroups" && <ConsumerGroupsView tabId={tabId} state={current} />}
        </div>
      </main>
    </div>
  );
}

function defaultPanelState(): KafkaTabState {
  return {
    activeView: "overview",
    brokers: [],
    topics: [],
    topicsTotal: 0,
    topicSearch: "",
    includeInternal: false,
    consumerGroups: [],
    loadingOverview: false,
    loadingBrokers: false,
    loadingTopics: false,
    loadingTopicDetail: false,
    loadingGroups: false,
    loadingGroupDetail: false,
    error: null,
  };
}

function OverviewView({ state }: { state: KafkaTabState }) {
  const { t } = useTranslation();
  const overview = state.overview;
  if (state.loadingOverview && !overview) return <LoadingBlock />;
  if (!overview) return <EmptyState text={t("query.kafkaNoOverview")} />;

  const controller = state.brokers.find((broker) => broker.nodeId === overview.controllerId);

  return (
    <div className="space-y-4 p-4">
      <div className="grid gap-3 md:grid-cols-4">
        <Metric label={t("query.kafkaBrokerCount")} value={overview.brokerCount} />
        <Metric label={t("query.kafkaTopicCount")} value={overview.topicCount} />
        <Metric label={t("query.kafkaPartitionCount")} value={overview.partitionCount} />
        <Metric label={t("query.kafkaUnderReplicated")} value={overview.underReplicatedPartitionCount} />
      </div>
      <div className="rounded-md border">
        <div className="grid grid-cols-2 gap-x-6 gap-y-3 p-4 text-sm md:grid-cols-4">
          <Info label={t("query.kafkaClusterId")} value={overview.clusterId || "-"} mono />
          <Info label={t("query.kafkaController")} value={String(overview.controllerId)} mono />
          <Info
            label={t("query.kafkaControllerHost")}
            value={controller ? `${controller.host}:${controller.port}` : "-"}
            mono
          />
          <Info label={t("query.kafkaInternalTopics")} value={String(overview.internalTopicCount)} mono />
          <Info label={t("query.kafkaOfflinePartitions")} value={String(overview.offlinePartitionCount)} mono />
        </div>
      </div>
      <TopicHealthTable topics={state.topics.slice(0, 8)} />
    </div>
  );
}

function Info({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className={`mt-1 truncate ${mono ? "font-mono text-xs" : ""}`}>{value}</div>
    </div>
  );
}

function TopicHealthTable({ topics }: { topics: KafkaTopicSummary[] }) {
  const { t } = useTranslation();
  if (!topics.length) return null;
  return (
    <div className="rounded-md border">
      <div className="border-b px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("query.kafkaRecentTopics")}
      </div>
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaTopic")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaPartitions")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaReplicationFactor")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaUnderReplicated")}</th>
          </tr>
        </thead>
        <tbody>
          {topics.map((topic) => (
            <tr key={topic.name} className="border-t">
              <td className="max-w-[360px] truncate px-3 py-2 font-mono text-xs">{topic.name}</td>
              <td className="px-3 py-2 text-right tabular-nums">{topic.partitionCount}</td>
              <td className="px-3 py-2 text-right tabular-nums">{topic.replicationFactor}</td>
              <td className="px-3 py-2 text-right tabular-nums">{topic.underReplicatedPartitionCount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function BrokersView({ state }: { state: KafkaTabState }) {
  const { t } = useTranslation();
  if (state.loadingBrokers && !state.brokers.length) return <LoadingBlock />;
  if (!state.brokers.length) return <EmptyState text={t("query.kafkaNoBrokers")} />;
  return (
    <div className="p-4">
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 text-left font-medium">{t("query.kafkaBrokerId")}</th>
              <th className="px-3 py-2 text-left font-medium">{t("asset.host")}</th>
              <th className="px-3 py-2 text-right font-medium">{t("asset.port")}</th>
              <th className="px-3 py-2 text-left font-medium">Rack</th>
            </tr>
          </thead>
          <tbody>
            {state.brokers.map((broker) => (
              <tr key={broker.nodeId} className="border-t">
                <td className="px-3 py-2 font-mono text-xs">{broker.nodeId}</td>
                <td className="px-3 py-2 font-mono text-xs">{broker.host}</td>
                <td className="px-3 py-2 text-right font-mono text-xs">{broker.port}</td>
                <td className="px-3 py-2 text-muted-foreground">{broker.rack || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TopicsView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const setTopicSearch = useKafkaStore((s) => s.setTopicSearch);
  const setIncludeInternal = useKafkaStore((s) => s.setIncludeInternal);
  const loadTopics = useKafkaStore((s) => s.loadTopics);
  const loadTopicDetail = useKafkaStore((s) => s.loadTopicDetail);

  const applySearch = () => loadTopics(tabId);

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-4 py-2">
        <div className="relative w-80 max-w-[50vw]">
          <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-8 pl-7 text-sm"
            value={state.topicSearch}
            onChange={(e) => setTopicSearch(tabId, e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") applySearch();
            }}
            placeholder={t("query.kafkaFilterTopics")}
          />
        </div>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={state.includeInternal}
            onChange={(e) => {
              setIncludeInternal(tabId, e.target.checked);
              setTimeout(() => loadTopics(tabId), 0);
            }}
          />
          {t("query.kafkaIncludeInternal")}
        </label>
        <Button variant="outline" size="sm" className="h-8" onClick={applySearch}>
          {t("query.applyFilter")}
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          {t("query.kafkaTopicTotal", { count: state.topicsTotal })}
        </span>
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(420px,1fr)_minmax(360px,0.9fr)]">
        <div className="min-h-0 overflow-auto border-r">
          {state.loadingTopics && !state.topics.length ? (
            <LoadingBlock />
          ) : state.topics.length === 0 ? (
            <EmptyState text={t("query.kafkaNoTopics")} />
          ) : (
            <TopicTable
              topics={state.topics}
              selected={state.selectedTopic}
              onSelect={(topic) => loadTopicDetail(tabId, topic)}
            />
          )}
        </div>
        <div className="min-h-0 overflow-auto">
          <TopicDetailPanel state={state} />
        </div>
      </div>
    </div>
  );
}

function TopicTable({
  topics,
  selected,
  onSelect,
}: {
  topics: KafkaTopicSummary[];
  selected?: string;
  onSelect: (topic: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-sm">
      <thead className="sticky top-0 bg-muted/90 text-xs text-muted-foreground backdrop-blur">
        <tr>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaTopic")}</th>
          <th className="px-3 py-2 text-right font-medium">{t("query.kafkaPartitions")}</th>
          <th className="px-3 py-2 text-right font-medium">RF</th>
          <th className="px-3 py-2 text-center font-medium">{t("query.kafkaInternal")}</th>
        </tr>
      </thead>
      <tbody>
        {topics.map((topic) => (
          <tr
            key={topic.name}
            className={`cursor-pointer border-t hover:bg-muted/40 ${selected === topic.name ? "bg-muted/60" : ""}`}
            onClick={() => onSelect(topic.name)}
          >
            <td className="max-w-[420px] truncate px-3 py-2 font-mono text-xs">{topic.name}</td>
            <td className="px-3 py-2 text-right tabular-nums">{topic.partitionCount}</td>
            <td className="px-3 py-2 text-right tabular-nums">{topic.replicationFactor}</td>
            <td className="px-3 py-2 text-center">{topic.internal ? "yes" : "-"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function TopicDetailPanel({ state }: { state: KafkaTabState }) {
  const { t } = useTranslation();
  if (state.loadingTopicDetail) return <LoadingBlock />;
  if (!state.selectedTopic) return <EmptyState text={t("query.kafkaSelectTopic")} />;
  const detail = state.topicDetail;
  if (!detail) return <EmptyState text={t("query.kafkaNoTopicDetail")} />;
  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center gap-2">
        <Database className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1 truncate font-mono text-sm font-semibold">{detail.name}</div>
        {detail.internal && <StatusPill value="internal" />}
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Metric label={t("query.kafkaPartitions")} value={detail.partitionCount} />
        <Metric label={t("query.kafkaReplicationFactor")} value={detail.replicationFactor} />
        <Metric label={t("query.kafkaUnderReplicated")} value={detail.underReplicatedPartitionCount} />
      </div>
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 text-right font-medium">P</th>
              <th className="px-3 py-2 text-right font-medium">Leader</th>
              <th className="px-3 py-2 text-left font-medium">Replicas</th>
              <th className="px-3 py-2 text-left font-medium">ISR</th>
            </tr>
          </thead>
          <tbody>
            {(detail.partitions || []).map((partition) => (
              <tr key={partition.partition} className="border-t">
                <td className="px-3 py-2 text-right font-mono text-xs">{partition.partition}</td>
                <td className="px-3 py-2 text-right font-mono text-xs">{partition.leader}</td>
                <td className="px-3 py-2 font-mono text-xs">{partition.replicas?.join(", ") || "-"}</td>
                <td className="px-3 py-2 font-mono text-xs">{partition.isr?.join(", ") || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ConsumerGroupsView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const loadConsumerGroupDetail = useKafkaStore((s) => s.loadConsumerGroupDetail);
  return (
    <div className="grid h-full grid-cols-[minmax(420px,1fr)_minmax(360px,0.9fr)]">
      <div className="min-h-0 overflow-auto border-r">
        {state.loadingGroups && !state.consumerGroups.length ? (
          <LoadingBlock />
        ) : state.consumerGroups.length === 0 ? (
          <EmptyState text={t("query.kafkaNoConsumerGroups")} />
        ) : (
          <ConsumerGroupTable
            groups={state.consumerGroups}
            selected={state.selectedGroup}
            onSelect={(group) => loadConsumerGroupDetail(tabId, group)}
          />
        )}
      </div>
      <div className="min-h-0 overflow-auto">
        <ConsumerGroupDetailPanel state={state} />
      </div>
    </div>
  );
}

function ConsumerGroupTable({
  groups,
  selected,
  onSelect,
}: {
  groups: KafkaConsumerGroup[];
  selected?: string;
  onSelect: (group: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-sm">
      <thead className="sticky top-0 bg-muted/90 text-xs text-muted-foreground backdrop-blur">
        <tr>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaGroup")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaState")}</th>
          <th className="px-3 py-2 text-right font-medium">{t("query.kafkaCoordinator")}</th>
        </tr>
      </thead>
      <tbody>
        {groups.map((group) => (
          <tr
            key={group.group}
            className={`cursor-pointer border-t hover:bg-muted/40 ${selected === group.group ? "bg-muted/60" : ""}`}
            onClick={() => onSelect(group.group)}
          >
            <td className="max-w-[420px] truncate px-3 py-2 font-mono text-xs">{group.group}</td>
            <td className="px-3 py-2">
              <StatusPill value={group.state} />
            </td>
            <td className="px-3 py-2 text-right font-mono text-xs">{group.coordinator}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ConsumerGroupDetailPanel({ state }: { state: KafkaTabState }) {
  const { t } = useTranslation();
  if (state.loadingGroupDetail) return <LoadingBlock />;
  if (!state.selectedGroup) return <EmptyState text={t("query.kafkaSelectConsumerGroup")} />;
  const detail = state.groupDetail;
  if (!detail) return <EmptyState text={t("query.kafkaNoConsumerGroupDetail")} />;
  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center gap-2">
        <Users className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1 truncate font-mono text-sm font-semibold">{detail.group}</div>
        <StatusPill value={detail.state} />
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Metric label={t("query.kafkaMembers")} value={detail.members?.length || 0} />
        <Metric label={t("query.kafkaTotalLag")} value={detail.totalLag || 0} />
        <Metric label={t("query.kafkaCoordinator")} value={detail.coordinator?.nodeId ?? "-"} />
      </div>
      {detail.lagError && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-2 text-xs">{detail.lagError}</div>
      )}
      <LagTable detail={detail} />
    </div>
  );
}

function LagTable({ detail }: { detail: KafkaConsumerGroupDetail }) {
  const { t } = useTranslation();
  const rows = detail.lag || [];
  if (!rows.length) return <EmptyState text={t("query.kafkaNoLag")} />;
  return (
    <div className="overflow-hidden rounded-md border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaTopic")}</th>
            <th className="px-3 py-2 text-right font-medium">P</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaCommittedOffset")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaEndOffset")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaLag")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={`${row.topic}:${row.partition}`} className="border-t">
              <td className="max-w-[260px] truncate px-3 py-2 font-mono text-xs">{row.topic}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">{row.partition}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">{row.committedOffset}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">{row.endOffset}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">{row.lag}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
