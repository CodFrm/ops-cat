import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  AlertCircle,
  Database,
  FileJson,
  GitBranch,
  ListTree,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Send,
  Server,
  Settings,
  ShieldCheck,
  Trash2,
  Users,
} from "lucide-react";
import {
  Button,
  Checkbox,
  ConfirmDialog,
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
  type KafkaACL,
  type KafkaACLMutationRequest,
  type KafkaConsumerGroup,
  type KafkaConsumerGroupDetail,
  type KafkaConnectorConfigRequest,
  type KafkaConnectorDetail,
  type KafkaConnectorSummary,
  type KafkaDeleteRecordsPartition,
  type KafkaMessageStartMode,
  type KafkaOffsetResetMode,
  type KafkaPayloadEncoding,
  type KafkaRecord,
  type KafkaRegisterSchemaRequest,
  type KafkaSchemaCompatibilityResponse,
  type KafkaSchemaReference,
  type KafkaTabState,
  type KafkaTopicConfigMutation,
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
  { id: "acls", icon: ShieldCheck, labelKey: "query.kafkaACLs" },
  { id: "schemas", icon: FileJson, labelKey: "query.kafkaSchemas" },
  { id: "connect", icon: Settings, labelKey: "query.kafkaConnect" },
];

const ACL_RESOURCE_TYPES = ["any", "topic", "group", "cluster", "transactional_id", "delegation_token"];
const ACL_MUTATION_RESOURCE_TYPES = ["topic", "group", "cluster", "transactional_id", "delegation_token"];
const ACL_FILTER_PATTERNS = ["any", "match", "literal", "prefixed"];
const ACL_MUTATION_PATTERNS = ["literal", "prefixed"];
const ACL_OPERATIONS = [
  "any",
  "all",
  "read",
  "write",
  "create",
  "delete",
  "alter",
  "describe",
  "describe_configs",
  "alter_configs",
  "idempotent_write",
  "cluster_action",
];
const ACL_MUTATION_OPERATIONS = ACL_OPERATIONS.filter((item) => item !== "any");
const ACL_PERMISSIONS = ["any", "allow", "deny"];
const ACL_MUTATION_PERMISSIONS = ["allow", "deny"];

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
  const loadACLs = useKafkaStore((s) => s.loadACLs);
  const loadSchemaSubjects = useKafkaStore((s) => s.loadSchemaSubjects);
  const loadConnectClusters = useKafkaStore((s) => s.loadConnectClusters);

  useEffect(() => {
    ensureTab(tabId);
    loadOverview(tabId);
    loadBrokers(tabId);
    loadTopics(tabId);
    loadConsumerGroups(tabId);
  }, [ensureTab, loadBrokers, loadConsumerGroups, loadOverview, loadTopics, tabId]);

  useEffect(() => {
    if (state?.activeView === "acls") {
      loadACLs(tabId);
    }
    if (state?.activeView === "schemas") {
      loadSchemaSubjects(tabId);
    }
    if (state?.activeView === "connect") {
      loadConnectClusters(tabId);
    }
  }, [loadACLs, loadConnectClusters, loadSchemaSubjects, state?.activeView, tabId]);

  const current = state || defaultPanelState();
  const busy =
    current.loadingOverview ||
    current.loadingBrokers ||
    current.loadingTopics ||
    current.loadingGroups ||
    current.loadingACLs ||
    current.loadingSchemaSubjects ||
    current.loadingConnectClusters ||
    current.loadingConnectors ||
    false;
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
          {current.activeView === "brokers" && <BrokersView tabId={tabId} state={current} />}
          {current.activeView === "topics" && <TopicsView tabId={tabId} state={current} />}
          {current.activeView === "consumerGroups" && <ConsumerGroupsView tabId={tabId} state={current} />}
          {current.activeView === "acls" && <ACLsView tabId={tabId} state={current} />}
          {current.activeView === "schemas" && <SchemaRegistryView tabId={tabId} state={current} />}
          {current.activeView === "connect" && <KafkaConnectView tabId={tabId} state={current} />}
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
    acls: [],
    aclsTotal: 0,
    aclFilters: {
      resourceType: "any",
      resourceName: "",
      patternType: "any",
      principal: "",
      host: "",
      operation: "any",
      permission: "any",
    },
    schemaSubjects: [],
    connectClusters: [],
    connectors: [],
    messageBrowser: {
      partition: "",
      startMode: "newest",
      offset: "",
      timestampMillis: "",
      limit: 50,
      maxBytes: 4096,
      decodeMode: "text",
      maxWaitMillis: 1000,
    },
    produceMessage: {
      partition: "",
      key: "",
      value: "",
      headers: "",
      keyEncoding: "text",
      valueEncoding: "text",
    },
    loadingOverview: false,
    loadingBrokers: false,
    loadingBrokerConfig: false,
    loadingClusterConfigs: false,
    loadingTopics: false,
    loadingTopicDetail: false,
    loadingMessages: false,
    producingMessage: false,
    topicAdminLoading: false,
    groupAdminLoading: false,
    loadingGroups: false,
    loadingGroupDetail: false,
    loadingACLs: false,
    aclAdminLoading: false,
    loadingSchemaSubjects: false,
    loadingSchemaDetail: false,
    schemaAdminLoading: false,
    loadingConnectClusters: false,
    loadingConnectors: false,
    loadingConnectorDetail: false,
    connectAdminLoading: false,
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

function BrokersView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [clusterConfigOpen, setClusterConfigOpen] = useState(false);
  const loadBrokerConfig = useKafkaStore((s) => s.loadBrokerConfig);
  const loadClusterConfigs = useKafkaStore((s) => s.loadClusterConfigs);

  const handleSelectBroker = (brokerId: number) => {
    loadBrokerConfig(tabId, brokerId);
  };

  const handleClusterConfig = () => {
    setClusterConfigOpen(true);
    loadClusterConfigs(tabId);
  };

  if (state.loadingBrokers && !state.brokers.length) return <LoadingBlock />;
  if (!state.brokers.length) return <EmptyState text={t("query.kafkaNoBrokers")} />;

  const showDetail = state.selectedBroker !== undefined;

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-4 py-2">
        <span className="text-xs text-muted-foreground">
          {t("query.kafkaBrokerTotal", { count: state.brokers.length })}
        </span>
        <Button variant="outline" size="sm" className="ml-auto h-8 gap-1.5" onClick={handleClusterConfig}>
          <Settings className="h-3.5 w-3.5" />
          {t("query.kafkaClusterConfig")}
        </Button>
      </div>
      <div className={`grid min-h-0 flex-1 ${showDetail ? "grid-cols-[minmax(320px,1fr)_minmax(300px,0.8fr)]" : "grid-cols-1"}`}>
        <div className="min-h-0 overflow-auto">
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
                    <tr
                      key={broker.nodeId}
                      className={`cursor-pointer border-t hover:bg-muted/30 ${state.selectedBroker === broker.nodeId ? "bg-muted/50" : ""}`}
                      onClick={() => handleSelectBroker(broker.nodeId)}
                    >
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
        </div>
        {showDetail && (
          <div className="min-h-0 overflow-auto border-l">
            <BrokerConfigPanel
              state={state}
              onClose={() =>
                useKafkaStore.setState((s) => ({
                  states: { ...s.states, [tabId]: { ...s.states[tabId], selectedBroker: undefined, brokerConfig: undefined } },
                }))
              }
            />
          </div>
        )}
      </div>

      <Dialog open={clusterConfigOpen} onOpenChange={setClusterConfigOpen}>
        <DialogContent className="max-h-[80vh] max-w-3xl overflow-hidden flex flex-col">
          <DialogHeader>
            <DialogTitle>{t("query.kafkaClusterConfig")}</DialogTitle>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-auto">
            {state.loadingClusterConfigs ? (
              <LoadingBlock />
            ) : state.clusterConfigs?.error ? (
              <div className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 shrink-0" />
                {state.clusterConfigs.error}
              </div>
            ) : (
              <ConfigTable configs={state.clusterConfigs?.configs || []} />
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setClusterConfigOpen(false)}>
              {t("action.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function BrokerConfigPanel({ state, onClose }: { state: KafkaTabState; onClose: () => void }) {
  const { t } = useTranslation();
  const broker = state.brokers.find((b) => b.nodeId === state.selectedBroker);

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b px-3 py-2">
        <span className="text-sm font-medium">
          {t("query.kafkaBrokerConfig")} — {broker ? `${broker.host}:${broker.port}` : `#${state.selectedBroker}`}
        </span>
        <Button variant="ghost" size="sm" className="h-6 w-6 p-0" onClick={onClose}>
          ✕
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {state.loadingBrokerConfig ? (
          <LoadingBlock />
        ) : state.brokerConfig?.error ? (
          <div className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 shrink-0" />
            {state.brokerConfig.error}
          </div>
        ) : (
          <ConfigTable configs={state.brokerConfig?.configs || []} />
        )}
      </div>
    </div>
  );
}

function ConfigTable({ configs }: { configs: { name: string; value?: string; isSensitive: boolean; source?: string }[] }) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const filtered = configs.filter((c) => !search || c.name.toLowerCase().includes(search.toLowerCase()));

  if (!configs.length) return <EmptyState text={t("query.kafkaNoConfigs")} />;

  return (
    <div className="flex flex-col gap-2">
      <div className="relative">
        <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="h-8 pl-7 text-sm"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("query.kafkaFilterConfigs")}
        />
      </div>
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 text-left font-medium">{t("asset.name")}</th>
              <th className="px-3 py-2 text-left font-medium">{t("query.kafkaConfigValue")}</th>
              <th className="px-3 py-2 text-left font-medium">{t("query.kafkaConfigSource")}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((cfg) => (
              <tr key={cfg.name} className="border-t">
                <td className="px-3 py-2 font-mono text-xs">{cfg.name}</td>
                <td className="max-w-[200px] truncate px-3 py-2 font-mono text-xs">
                  {cfg.isSensitive ? (
                    <span className="text-muted-foreground italic">{t("query.kafkaConfigSensitive")}</span>
                  ) : cfg.value !== undefined ? (
                    cfg.value
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </td>
                <td className="px-3 py-2 text-xs text-muted-foreground">{cfg.source || "—"}</td>
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
  const [createOpen, setCreateOpen] = useState(false);
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
        <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setCreateOpen(true)}>
          <Plus className="h-3.5 w-3.5" />
          {t("query.kafkaCreateTopic")}
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
          <TopicDetailPanel tabId={tabId} state={state} />
        </div>
      </div>
      <CreateTopicDialog tabId={tabId} open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function CreateTopicDialog({
  tabId,
  open,
  onOpenChange,
}: {
  tabId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const createTopic = useKafkaStore((s) => s.createTopic);
  const state = useKafkaStore((s) => s.states[tabId]);
  const [topic, setTopic] = useState("");
  const [partitions, setPartitions] = useState(1);
  const [replicationFactor, setReplicationFactor] = useState(1);
  const [configs, setConfigs] = useState("");

  const submit = async () => {
    const name = topic.trim();
    if (!name) return;
    await createTopic(tabId, {
      topic: name,
      partitions,
      replicationFactor,
      configs: parseOptionalJsonObject(configs),
    });
    setTopic("");
    setConfigs("");
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaCreateTopic")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            className="font-mono text-sm"
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            placeholder={t("query.kafkaTopic")}
          />
          <div className="grid grid-cols-2 gap-2">
            <NumberInput value={partitions} onChange={setPartitions} placeholder={t("query.kafkaPartitions")} />
            <NumberInput
              value={replicationFactor}
              onChange={setReplicationFactor}
              placeholder={t("query.kafkaReplicationFactor")}
            />
          </div>
          <Textarea
            className="min-h-24 font-mono text-xs"
            value={configs}
            onChange={(e) => setConfigs(e.target.value)}
            placeholder={t("query.kafkaConfigsPlaceholder")}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.topicAdminLoading || !topic.trim()} onClick={submit}>
            {state?.topicAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaCreateTopic")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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

function TopicDetailPanel({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [partitionsOpen, setPartitionsOpen] = useState(false);
  const deleteTopic = useKafkaStore((s) => s.deleteTopic);
  if (state.loadingTopicDetail) return <LoadingBlock />;
  if (!state.selectedTopic) return <EmptyState text={t("query.kafkaSelectTopic")} />;
  const detail = state.topicDetail;
  if (!detail) return <EmptyState text={t("query.kafkaNoTopicDetail")} />;
  return (
    <div className="space-y-4 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <Database className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-[220px] flex-1 truncate font-mono text-sm font-semibold">{detail.name}</div>
        {detail.internal && <StatusPill value="internal" />}
        <Button variant="outline" size="sm" className="h-7 gap-1.5" onClick={() => setConfigOpen(true)}>
          <Settings className="h-3.5 w-3.5" />
          {t("query.kafkaUpdateConfig")}
        </Button>
        <Button variant="outline" size="sm" className="h-7 gap-1.5" onClick={() => setPartitionsOpen(true)}>
          <GitBranch className="h-3.5 w-3.5" />
          {t("query.kafkaIncreasePartitions")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-destructive hover:text-destructive"
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
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
      <MessageBrowser tabId={tabId} state={state} />
      <ProduceMessagePanel tabId={tabId} state={state} />
      <AlterTopicConfigDialog
        tabId={tabId}
        topic={detail.name}
        open={configOpen}
        onOpenChange={setConfigOpen}
      />
      <IncreasePartitionsDialog
        tabId={tabId}
        topic={detail.name}
        currentCount={detail.partitionCount}
        open={partitionsOpen}
        onOpenChange={setPartitionsOpen}
      />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("query.kafkaDeleteTopic")}
        description={t("query.kafkaDeleteTopicConfirmDesc", { topic: detail.name })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={() => deleteTopic(tabId, detail.name)}
      />
    </div>
  );
}

function AlterTopicConfigDialog({
  tabId,
  topic,
  open,
  onOpenChange,
}: {
  tabId: string;
  topic: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [updatesText, setUpdatesText] = useState("");
  const alterTopicConfig = useKafkaStore((s) => s.alterTopicConfig);
  const state = useKafkaStore((s) => s.states[tabId]);

  const confirm = async () => {
    const updates = parseConfigUpdates(updatesText);
    await alterTopicConfig(tabId, topic, updates);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaUpdateConfig")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="rounded-md bg-muted/40 px-3 py-2 font-mono text-xs">{topic}</div>
          <Textarea
            className="min-h-40 font-mono text-xs"
            value={updatesText}
            onChange={(e) => setUpdatesText(e.target.value)}
            placeholder={t("query.kafkaConfigUpdatesPlaceholder")}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.topicAdminLoading || !updatesText.trim()} onClick={() => setConfirmOpen(true)}>
            {state?.topicAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("action.save")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaUpdateConfig")}
          description={t("query.kafkaUpdateConfigConfirmDesc", { topic })}
          cancelText={t("action.cancel")}
          confirmText={t("action.save")}
          onConfirm={confirm}
        />
      </DialogContent>
    </Dialog>
  );
}

function IncreasePartitionsDialog({
  tabId,
  topic,
  currentCount,
  open,
  onOpenChange,
}: {
  tabId: string;
  topic: string;
  currentCount: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [nextCount, setNextCount] = useState(currentCount + 1);
  const increasePartitions = useKafkaStore((s) => s.increasePartitions);
  const state = useKafkaStore((s) => s.states[tabId]);

  useEffect(() => {
    if (open) setNextCount(currentCount + 1);
  }, [currentCount, open]);

  const confirm = async () => {
    await increasePartitions(tabId, topic, nextCount);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaIncreasePartitions")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="rounded-md bg-muted/40 px-3 py-2 font-mono text-xs">{topic}</div>
          <Metric label={t("query.kafkaCurrentPartitions")} value={currentCount} />
          <NumberInput value={nextCount} onChange={setNextCount} placeholder={t("query.kafkaPartitions")} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button
            disabled={state?.topicAdminLoading || nextCount <= currentCount}
            onClick={() => setConfirmOpen(true)}
          >
            {state?.topicAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaIncreasePartitions")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaIncreasePartitions")}
          description={t("query.kafkaIncreasePartitionsConfirmDesc", { topic, count: nextCount })}
          cancelText={t("action.cancel")}
          confirmText={t("query.kafkaIncreasePartitions")}
          onConfirm={confirm}
        />
      </DialogContent>
    </Dialog>
  );
}

function MessageBrowser({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [deleteRecordsOpen, setDeleteRecordsOpen] = useState(false);
  const setMessageBrowser = useKafkaStore((s) => s.setMessageBrowser);
  const browseMessages = useKafkaStore((s) => s.browseMessages);
  const browser = state.messageBrowser;
  const records = browser.response?.records || [];

  return (
    <div className="overflow-hidden rounded-md border">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("query.kafkaMessages")}
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" className="h-7 gap-1.5" onClick={() => browseMessages(tabId)}>
            {state.loadingMessages ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <RefreshCw className="h-3.5 w-3.5" />
            )}
            {t("query.kafkaBrowseMessages")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 text-destructive hover:text-destructive"
            onClick={() => setDeleteRecordsOpen(true)}
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t("query.kafkaDeleteRecords")}
          </Button>
        </div>
      </div>
      <div className="grid gap-2 border-b bg-muted/20 p-3 text-xs md:grid-cols-6">
        <Input
          className="h-8 font-mono"
          value={browser.partition}
          onChange={(e) => setMessageBrowser(tabId, { partition: e.target.value })}
          placeholder={t("query.kafkaAllPartitions")}
        />
        <Select
          value={browser.startMode}
          onValueChange={(value) => setMessageBrowser(tabId, { startMode: value as KafkaMessageStartMode })}
        >
          <SelectTrigger className="h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="newest">{t("query.kafkaStartNewest")}</SelectItem>
            <SelectItem value="oldest">{t("query.kafkaStartOldest")}</SelectItem>
            <SelectItem value="offset">{t("query.kafkaStartOffset")}</SelectItem>
            <SelectItem value="timestamp">{t("query.kafkaStartTimestamp")}</SelectItem>
          </SelectContent>
        </Select>
        <Input
          className="h-8 font-mono"
          value={browser.startMode === "timestamp" ? browser.timestampMillis : browser.offset}
          onChange={(e) =>
            setMessageBrowser(
              tabId,
              browser.startMode === "timestamp" ? { timestampMillis: e.target.value } : { offset: e.target.value }
            )
          }
          disabled={browser.startMode === "newest" || browser.startMode === "oldest"}
          placeholder={browser.startMode === "timestamp" ? t("query.kafkaTimestampMillis") : t("query.kafkaOffset")}
        />
        <NumberInput
          value={browser.limit}
          onChange={(value) => setMessageBrowser(tabId, { limit: value })}
          placeholder={t("query.kafkaLimit")}
        />
        <NumberInput
          value={browser.maxBytes}
          onChange={(value) => setMessageBrowser(tabId, { maxBytes: value })}
          placeholder={t("query.kafkaMaxBytes")}
        />
        <Select
          value={browser.decodeMode}
          onValueChange={(value) => setMessageBrowser(tabId, { decodeMode: value as KafkaPayloadEncoding })}
        >
          <SelectTrigger className="h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="text">text</SelectItem>
            <SelectItem value="json">json</SelectItem>
            <SelectItem value="hex">hex</SelectItem>
            <SelectItem value="base64">base64</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {browser.response?.errors?.length ? (
        <div className="border-b bg-amber-500/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-200">
          {browser.response.errors.join("; ")}
        </div>
      ) : null}
      {state.loadingMessages && !records.length ? (
        <LoadingBlock />
      ) : records.length === 0 ? (
        <EmptyState text={t("query.kafkaNoMessages")} />
      ) : (
        <MessageTable records={records} />
      )}
      <DeleteRecordsDialog
        tabId={tabId}
        topic={state.selectedTopic || ""}
        open={deleteRecordsOpen}
        onOpenChange={setDeleteRecordsOpen}
      />
    </div>
  );
}

function DeleteRecordsDialog({
  tabId,
  topic,
  open,
  onOpenChange,
}: {
  tabId: string;
  topic: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [partition, setPartition] = useState("");
  const [offset, setOffset] = useState("");
  const deleteTopicRecords = useKafkaStore((s) => s.deleteTopicRecords);
  const state = useKafkaStore((s) => s.states[tabId]);

  const partitionValue = Number(partition);
  const offsetValue = Number(offset);
  const canSubmit = Number.isInteger(partitionValue) && partitionValue >= 0 && Number.isInteger(offsetValue) && offsetValue >= 0;

  const confirm = async () => {
    const partitions: KafkaDeleteRecordsPartition[] = [{ partition: partitionValue, offset: offsetValue }];
    await deleteTopicRecords(tabId, topic, partitions);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaDeleteRecords")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="rounded-md bg-muted/40 px-3 py-2 font-mono text-xs">{topic}</div>
          <Input
            className="h-8 font-mono text-xs"
            value={partition}
            onChange={(e) => setPartition(e.target.value)}
            placeholder={t("query.kafkaPartition")}
          />
          <Input
            className="h-8 font-mono text-xs"
            value={offset}
            onChange={(e) => setOffset(e.target.value)}
            placeholder={t("query.kafkaDeleteBeforeOffset")}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button
            disabled={state?.topicAdminLoading || !canSubmit}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={() => setConfirmOpen(true)}
          >
            {state?.topicAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaDeleteRecords")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaDeleteRecords")}
          description={t("query.kafkaDeleteRecordsConfirmDesc", {
            topic,
            partition: partitionValue,
            offset: offsetValue,
          })}
          cancelText={t("action.cancel")}
          confirmText={t("action.delete")}
          onConfirm={confirm}
        />
      </DialogContent>
    </Dialog>
  );
}

function NumberInput({
  value,
  onChange,
  placeholder,
}: {
  value: number;
  onChange: (value: number) => void;
  placeholder: string;
}) {
  return (
    <Input
      className="h-8 font-mono"
      type="number"
      value={value}
      min={1}
      onChange={(e) => onChange(Number(e.target.value) || 1)}
      placeholder={placeholder}
    />
  );
}

function MessageTable({ records }: { records: KafkaRecord[] }) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-xs">
      <thead className="bg-muted/40 text-muted-foreground">
        <tr>
          <th className="px-3 py-2 text-right font-medium">P</th>
          <th className="px-3 py-2 text-right font-medium">Offset</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaMessageKey")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaMessageValue")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaHeaders")}</th>
        </tr>
      </thead>
      <tbody>
        {records.map((record) => (
          <tr key={`${record.partition}:${record.offset}`} className="border-t align-top">
            <td className="px-3 py-2 text-right font-mono">{record.partition}</td>
            <td className="px-3 py-2 text-right font-mono">
              <div>{record.offset}</div>
              <div className="mt-1 text-[10px] text-muted-foreground">{record.timestamp}</div>
            </td>
            <td className="max-w-[180px] px-3 py-2">
              <PayloadPreview
                value={record.key}
                bytes={record.keyBytes}
                encoding={record.keyEncoding}
                truncated={record.keyTruncated}
              />
            </td>
            <td className="max-w-[260px] px-3 py-2">
              <PayloadPreview
                value={record.value}
                bytes={record.valueBytes}
                encoding={record.valueEncoding}
                truncated={record.valueTruncated}
              />
            </td>
            <td className="max-w-[180px] px-3 py-2">
              {record.headers?.length ? (
                <div className="space-y-1">
                  {record.headers.map((header, index) => (
                    <div key={`${header.key}:${index}`} className="min-w-0">
                      <span className="font-mono text-muted-foreground">{header.key}</span>
                      <PayloadPreview
                        value={header.value}
                        bytes={header.valueBytes}
                        encoding={header.valueEncoding}
                        truncated={header.valueTruncated}
                      />
                    </div>
                  ))}
                </div>
              ) : (
                <span className="text-muted-foreground">-</span>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function PayloadPreview({
  value,
  bytes,
  encoding,
  truncated,
}: {
  value?: string;
  bytes: number;
  encoding: string;
  truncated: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      <div className="mb-1 flex flex-wrap items-center gap-1 text-[10px] text-muted-foreground">
        <span>{encoding}</span>
        <span>{bytes}B</span>
        {truncated && <span className="rounded border px-1 text-[9px]">{t("query.kafkaTruncated")}</span>}
      </div>
      <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/40 p-2 font-mono text-[11px] leading-relaxed">
        {value || "-"}
      </pre>
    </div>
  );
}

function ProduceMessagePanel({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const setProduceMessage = useKafkaStore((s) => s.setProduceMessage);
  const produceKafkaMessage = useKafkaStore((s) => s.produceKafkaMessage);
  const form = state.produceMessage;

  return (
    <div className="overflow-hidden rounded-md border">
      <div className="border-b px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("query.kafkaProduceMessage")}
      </div>
      <div className="space-y-3 p-3">
        <div className="grid gap-2 md:grid-cols-[120px_1fr_120px_120px]">
          <Input
            className="h-8 font-mono text-xs"
            value={form.partition}
            onChange={(e) => setProduceMessage(tabId, { partition: e.target.value })}
            placeholder={t("query.kafkaAllPartitions")}
          />
          <Input
            className="h-8 font-mono text-xs"
            value={form.key}
            onChange={(e) => setProduceMessage(tabId, { key: e.target.value })}
            placeholder={t("query.kafkaMessageKey")}
          />
          <EncodingSelect value={form.keyEncoding} onChange={(value) => setProduceMessage(tabId, { keyEncoding: value })} />
          <EncodingSelect
            value={form.valueEncoding}
            onChange={(value) => setProduceMessage(tabId, { valueEncoding: value })}
          />
        </div>
        <Textarea
          className="min-h-24 font-mono text-xs"
          value={form.value}
          onChange={(e) => setProduceMessage(tabId, { value: e.target.value })}
          placeholder={t("query.kafkaMessageValue")}
        />
        <Textarea
          className="min-h-16 font-mono text-xs"
          value={form.headers}
          onChange={(e) => setProduceMessage(tabId, { headers: e.target.value })}
          placeholder={t("query.kafkaHeadersPlaceholder")}
        />
        <div className="flex justify-end">
          <Button className="h-8 gap-1.5" size="sm" disabled={state.producingMessage} onClick={() => setConfirmOpen(true)}>
            {state.producingMessage ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
            {t("query.kafkaSendMessage")}
          </Button>
        </div>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("query.kafkaProduceConfirmTitle")}
        description={t("query.kafkaProduceConfirmDesc", { topic: state.selectedTopic || "" })}
        cancelText={t("action.cancel")}
        confirmText={t("query.kafkaSendMessage")}
        onConfirm={() => produceKafkaMessage(tabId)}
      />
    </div>
  );
}

function EncodingSelect({
  value,
  onChange,
}: {
  value: KafkaPayloadEncoding;
  onChange: (value: KafkaPayloadEncoding) => void;
}) {
  return (
    <Select value={value} onValueChange={(next) => onChange(next as KafkaPayloadEncoding)}>
      <SelectTrigger className="h-8 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="text">text</SelectItem>
        <SelectItem value="json">json</SelectItem>
        <SelectItem value="hex">hex</SelectItem>
        <SelectItem value="base64">base64</SelectItem>
      </SelectContent>
    </Select>
  );
}

function ACLsView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<KafkaACL | null>(null);
  const setACLFilters = useKafkaStore((s) => s.setACLFilters);
  const loadACLs = useKafkaStore((s) => s.loadACLs);
  const deleteACL = useKafkaStore((s) => s.deleteACL);
  const filters = state.aclFilters || {
    resourceType: "any",
    resourceName: "",
    patternType: "any",
    principal: "",
    host: "",
    operation: "any",
    permission: "any",
  };

  return (
    <div className="flex h-full flex-col">
      <div className="grid shrink-0 gap-2 border-b px-4 py-2 xl:grid-cols-[150px_1fr_150px_1fr_150px_150px_150px_auto_auto]">
        <CompactSelect
          value={filters.resourceType}
          onChange={(value) => setACLFilters(tabId, { resourceType: value })}
          items={ACL_RESOURCE_TYPES}
        />
        <Input
          className="h-8 font-mono text-xs"
          value={filters.resourceName}
          onChange={(e) => setACLFilters(tabId, { resourceName: e.target.value })}
          placeholder={t("query.kafkaACLResourceName")}
        />
        <CompactSelect
          value={filters.patternType}
          onChange={(value) => setACLFilters(tabId, { patternType: value })}
          items={ACL_FILTER_PATTERNS}
        />
        <Input
          className="h-8 font-mono text-xs"
          value={filters.principal}
          onChange={(e) => setACLFilters(tabId, { principal: e.target.value })}
          placeholder={t("query.kafkaACLPrincipal")}
        />
        <Input
          className="h-8 font-mono text-xs"
          value={filters.host}
          onChange={(e) => setACLFilters(tabId, { host: e.target.value })}
          placeholder={t("query.kafkaACLHost")}
        />
        <CompactSelect
          value={filters.operation}
          onChange={(value) => setACLFilters(tabId, { operation: value })}
          items={ACL_OPERATIONS}
        />
        <CompactSelect
          value={filters.permission}
          onChange={(value) => setACLFilters(tabId, { permission: value })}
          items={ACL_PERMISSIONS}
        />
        <Button variant="outline" size="sm" className="h-8" onClick={() => loadACLs(tabId)}>
          {t("query.applyFilter")}
        </Button>
        <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setCreateOpen(true)}>
          <Plus className="h-3.5 w-3.5" />
          {t("query.kafkaCreateACL")}
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {state.loadingACLs && !state.acls.length ? (
          <LoadingBlock />
        ) : state.acls.length === 0 ? (
          <EmptyState text={t("query.kafkaNoACLs")} />
        ) : (
          <ACLTable acls={state.acls} onDelete={setDeleteTarget} />
        )}
      </div>
      <div className="shrink-0 border-t px-4 py-2 text-xs text-muted-foreground">
        {t("query.kafkaACLTotal", { count: state.aclsTotal })}
      </div>
      <CreateACLDialog tabId={tabId} open={createOpen} onOpenChange={setCreateOpen} />
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("query.kafkaDeleteACL")}
        description={t("query.kafkaDeleteACLConfirmDesc", {
          principal: deleteTarget?.principal || "",
          resource: deleteTarget ? `${deleteTarget.resourceType}:${deleteTarget.resourceName}` : "",
        })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={async () => {
          if (!deleteTarget) return;
          await deleteACL(tabId, deleteTarget);
          setDeleteTarget(null);
        }}
      />
    </div>
  );
}

function ACLTable({ acls, onDelete }: { acls: KafkaACL[]; onDelete: (acl: KafkaACL) => void }) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-sm">
      <thead className="sticky top-0 bg-muted/90 text-xs text-muted-foreground backdrop-blur">
        <tr>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaACLResource")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaACLPrincipal")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaACLHost")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaACLOperation")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaACLPermission")}</th>
          <th className="w-12 px-3 py-2 text-right font-medium"></th>
        </tr>
      </thead>
      <tbody>
        {acls.map((acl) => (
          <tr key={aclKey(acl)} className="border-t">
            <td className="max-w-[360px] px-3 py-2">
              <div className="truncate font-mono text-xs">{acl.resourceName || "-"}</div>
              <div className="mt-0.5 flex flex-wrap gap-1 text-[10px] uppercase text-muted-foreground">
                <span>{acl.resourceType}</span>
                <span>{acl.patternType}</span>
              </div>
            </td>
            <td className="max-w-[280px] truncate px-3 py-2 font-mono text-xs">{acl.principal}</td>
            <td className="px-3 py-2 font-mono text-xs">{acl.host}</td>
            <td className="px-3 py-2">
              <StatusPill value={acl.operation} />
            </td>
            <td className="px-3 py-2">
              <StatusPill value={acl.permission} />
            </td>
            <td className="px-3 py-2 text-right">
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 text-destructive hover:text-destructive"
                onClick={() => onDelete(acl)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function CreateACLDialog({
  tabId,
  open,
  onOpenChange,
}: {
  tabId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const createACL = useKafkaStore((s) => s.createACL);
  const state = useKafkaStore((s) => s.states[tabId]);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [form, setForm] = useState<KafkaACLMutationRequest>({
    resourceType: "topic",
    resourceName: "",
    patternType: "literal",
    principal: "",
    host: "*",
    operation: "read",
    permission: "allow",
  });

  const update = (patch: Partial<KafkaACLMutationRequest>) => setForm((current) => ({ ...current, ...patch }));
  const resourceNameRequired = form.resourceType !== "cluster";
  const canSubmit =
    form.resourceType &&
    form.principal.trim() &&
    form.operation &&
    form.permission &&
    (!resourceNameRequired || form.resourceName?.trim());

  const submit = async () => {
    await createACL(tabId, {
      ...form,
      resourceName: form.resourceName?.trim(),
      principal: form.principal.trim(),
      host: form.host?.trim() || "*",
    });
    setForm({
      resourceType: "topic",
      resourceName: "",
      patternType: "literal",
      principal: "",
      host: "*",
      operation: "read",
      permission: "allow",
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaCreateACL")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-2">
            <CompactSelect
              value={form.resourceType}
              onChange={(value) => update({ resourceType: value })}
              items={ACL_MUTATION_RESOURCE_TYPES}
            />
            <CompactSelect
              value={form.patternType || "literal"}
              onChange={(value) => update({ patternType: value })}
              items={ACL_MUTATION_PATTERNS}
            />
          </div>
          <Input
            className="h-8 font-mono text-xs"
            value={form.resourceName || ""}
            disabled={form.resourceType === "cluster"}
            onChange={(e) => update({ resourceName: e.target.value })}
            placeholder={form.resourceType === "cluster" ? "kafka-cluster" : t("query.kafkaACLResourceName")}
          />
          <div className="grid grid-cols-2 gap-2">
            <Input
              className="h-8 font-mono text-xs"
              value={form.principal}
              onChange={(e) => update({ principal: e.target.value })}
              placeholder={t("query.kafkaACLPrincipalPlaceholder")}
            />
            <Input
              className="h-8 font-mono text-xs"
              value={form.host || ""}
              onChange={(e) => update({ host: e.target.value })}
              placeholder="*"
            />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <CompactSelect
              value={form.operation}
              onChange={(value) => update({ operation: value })}
              items={ACL_MUTATION_OPERATIONS}
            />
            <CompactSelect
              value={form.permission}
              onChange={(value) => update({ permission: value })}
              items={ACL_MUTATION_PERMISSIONS}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.aclAdminLoading || !canSubmit} onClick={() => setConfirmOpen(true)}>
            {state?.aclAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaCreateACL")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaCreateACL")}
          description={t("query.kafkaCreateACLConfirmDesc", {
            principal: form.principal.trim(),
            resource: `${form.resourceType}:${form.resourceType === "cluster" ? "kafka-cluster" : form.resourceName}`,
          })}
          cancelText={t("action.cancel")}
          confirmText={t("query.kafkaCreateACL")}
          onConfirm={submit}
        />
      </DialogContent>
    </Dialog>
  );
}

function CompactSelect({ value, onChange, items }: { value: string; onChange: (value: string) => void; items: string[] }) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item} value={item}>
            {item}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function aclKey(acl: KafkaACL): string {
  return [
    acl.resourceType,
    acl.resourceName,
    acl.patternType,
    acl.principal,
    acl.host,
    acl.operation,
    acl.permission,
  ].join("|");
}

function SchemaRegistryView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [registerOpen, setRegisterOpen] = useState(false);
  const loadSchemaSubjects = useKafkaStore((s) => s.loadSchemaSubjects);
  const loadSchemaVersions = useKafkaStore((s) => s.loadSchemaVersions);

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-4 py-2">
        <Button variant="outline" size="sm" className="h-8" onClick={() => loadSchemaSubjects(tabId)}>
          {state.loadingSchemaSubjects ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
          {t("query.refreshTree")}
        </Button>
        <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setRegisterOpen(true)}>
          <Plus className="h-3.5 w-3.5" />
          {t("query.kafkaRegisterSchema")}
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          {t("query.kafkaSchemaSubjectTotal", { count: state.schemaSubjects.length })}
        </span>
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(320px,0.8fr)_minmax(460px,1.2fr)]">
        <div className="min-h-0 overflow-auto border-r">
          {state.loadingSchemaSubjects && !state.schemaSubjects.length ? (
            <LoadingBlock />
          ) : state.schemaSubjects.length === 0 ? (
            <EmptyState text={t("query.kafkaNoSchemaSubjects")} />
          ) : (
            <SchemaSubjectTable
              subjects={state.schemaSubjects}
              selected={state.selectedSchemaSubject}
              onSelect={(subject) => loadSchemaVersions(tabId, subject)}
            />
          )}
        </div>
        <div className="min-h-0 overflow-auto">
          <SchemaDetailPanel tabId={tabId} state={state} />
        </div>
      </div>
      <RegisterSchemaDialog tabId={tabId} open={registerOpen} onOpenChange={setRegisterOpen} />
    </div>
  );
}

function SchemaSubjectTable({
  subjects,
  selected,
  onSelect,
}: {
  subjects: string[];
  selected?: string;
  onSelect: (subject: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-sm">
      <thead className="sticky top-0 bg-muted/90 text-xs text-muted-foreground backdrop-blur">
        <tr>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaSchemaSubject")}</th>
        </tr>
      </thead>
      <tbody>
        {subjects.map((subject) => (
          <tr
            key={subject}
            className={`cursor-pointer border-t hover:bg-muted/40 ${selected === subject ? "bg-muted/60" : ""}`}
            onClick={() => onSelect(subject)}
          >
            <td className="max-w-[420px] truncate px-3 py-2 font-mono text-xs">{subject}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function SchemaDetailPanel({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [deleteVersionOpen, setDeleteVersionOpen] = useState(false);
  const [deleteSubjectOpen, setDeleteSubjectOpen] = useState(false);
  const loadSchema = useKafkaStore((s) => s.loadSchema);
  const deleteSchema = useKafkaStore((s) => s.deleteSchema);
  if (state.loadingSchemaDetail) return <LoadingBlock />;
  if (!state.selectedSchemaSubject) return <EmptyState text={t("query.kafkaSelectSchemaSubject")} />;
  const detail = state.schemaDetail;
  const versions = state.schemaVersions?.versions || [];
  if (!detail) return <EmptyState text={t("query.kafkaNoSchemaDetail")} />;

  const deleteVersion = async () => {
    await deleteSchema(tabId, { subject: detail.subject, version: String(detail.version) });
    setDeleteVersionOpen(false);
  };
  const deleteSubject = async () => {
    await deleteSchema(tabId, { subject: detail.subject });
    setDeleteSubjectOpen(false);
  };

  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center gap-2">
        <FileJson className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1 truncate font-mono text-sm font-semibold">{detail.subject}</div>
        <StatusPill value={detail.schemaType || "schema"} />
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-destructive hover:text-destructive"
          onClick={() => setDeleteVersionOpen(true)}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Metric label="ID" value={detail.id} />
        <Metric label={t("query.kafkaSchemaVersion")} value={detail.version} />
        <Metric label={t("query.kafkaSchemaVersions")} value={versions.length} />
      </div>
      <div className="flex flex-wrap gap-1">
        {versions.map((version) => (
          <Button
            key={version}
            variant={version === detail.version ? "default" : "outline"}
            size="sm"
            className="h-7 px-2 font-mono text-xs"
            onClick={() => loadSchema(tabId, detail.subject, String(version))}
          >
            v{version}
          </Button>
        ))}
        <Button variant="ghost" size="sm" className="h-7 text-destructive" onClick={() => setDeleteSubjectOpen(true)}>
          {t("query.kafkaDeleteSubject")}
        </Button>
      </div>
      {detail.references?.length ? <SchemaReferencesTable references={detail.references} /> : null}
      <pre className="max-h-[520px] overflow-auto whitespace-pre-wrap break-all rounded-md border bg-muted/30 p-3 font-mono text-xs leading-relaxed">
        {formatSchema(detail.schema)}
      </pre>
      <ConfirmDialog
        open={deleteVersionOpen}
        onOpenChange={setDeleteVersionOpen}
        title={t("query.kafkaDeleteSchemaVersion")}
        description={t("query.kafkaDeleteSchemaVersionConfirmDesc", {
          subject: detail.subject,
          version: detail.version,
        })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={deleteVersion}
      />
      <ConfirmDialog
        open={deleteSubjectOpen}
        onOpenChange={setDeleteSubjectOpen}
        title={t("query.kafkaDeleteSubject")}
        description={t("query.kafkaDeleteSubjectConfirmDesc", { subject: detail.subject })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={deleteSubject}
      />
    </div>
  );
}

function SchemaReferencesTable({ references }: { references: KafkaSchemaReference[] }) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-md border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaSchemaReference")}</th>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaSchemaSubject")}</th>
            <th className="px-3 py-2 text-right font-medium">{t("query.kafkaSchemaVersion")}</th>
          </tr>
        </thead>
        <tbody>
          {references.map((reference) => (
            <tr key={`${reference.name}:${reference.subject}:${reference.version}`} className="border-t">
              <td className="px-3 py-2 font-mono text-xs">{reference.name}</td>
              <td className="px-3 py-2 font-mono text-xs">{reference.subject}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">{reference.version}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RegisterSchemaDialog({
  tabId,
  open,
  onOpenChange,
}: {
  tabId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const registerSchema = useKafkaStore((s) => s.registerSchema);
  const checkSchemaCompatibility = useKafkaStore((s) => s.checkSchemaCompatibility);
  const state = useKafkaStore((s) => s.states[tabId]);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [schemaType, setSchemaType] = useState("AVRO");
  const [schema, setSchema] = useState("");
  const [references, setReferences] = useState("");
  const [compatibility, setCompatibility] = useState<KafkaSchemaCompatibilityResponse | null>(null);
  const canSubmit = subject.trim() && schema.trim();

  const request = (): KafkaRegisterSchemaRequest => ({
    subject: subject.trim(),
    schema,
    schemaType: schemaType.trim() || undefined,
    references: parseSchemaReferences(references),
  });

  const check = async () => {
    const result = await checkSchemaCompatibility(tabId, { ...request(), version: "latest" });
    setCompatibility(result || null);
  };

  const submit = async () => {
    await registerSchema(tabId, request());
    setSubject("");
    setSchema("");
    setReferences("");
    setCompatibility(null);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaRegisterSchema")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="grid grid-cols-[1fr_140px] gap-2">
            <Input
              className="h-8 font-mono text-xs"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder={t("query.kafkaSchemaSubject")}
            />
            <CompactSelect value={schemaType} onChange={setSchemaType} items={["AVRO", "JSON", "PROTOBUF"]} />
          </div>
          <Textarea
            className="min-h-64 font-mono text-xs"
            value={schema}
            onChange={(e) => {
              setSchema(e.target.value);
              setCompatibility(null);
            }}
            placeholder={t("query.kafkaSchemaPlaceholder")}
          />
          <Textarea
            className="min-h-16 font-mono text-xs"
            value={references}
            onChange={(e) => setReferences(e.target.value)}
            placeholder={t("query.kafkaSchemaReferencesPlaceholder")}
          />
          {compatibility && (
            <div
              className={`rounded-md border px-3 py-2 text-sm ${
                compatibility.compatible ? "border-emerald-500/30 bg-emerald-500/10" : "border-destructive/30 bg-destructive/10"
              }`}
            >
              {compatibility.compatible ? t("query.kafkaSchemaCompatible") : t("query.kafkaSchemaIncompatible")}
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button variant="outline" disabled={state?.schemaAdminLoading || !canSubmit} onClick={check}>
            {state?.schemaAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaCheckCompatibility")}
          </Button>
          <Button disabled={state?.schemaAdminLoading || !canSubmit} onClick={() => setConfirmOpen(true)}>
            {state?.schemaAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaRegisterSchema")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaRegisterSchema")}
          description={t("query.kafkaRegisterSchemaConfirmDesc", { subject: subject.trim() })}
          cancelText={t("action.cancel")}
          confirmText={t("query.kafkaRegisterSchema")}
          onConfirm={submit}
        />
      </DialogContent>
    </Dialog>
  );
}

function KafkaConnectView({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [createOpen, setCreateOpen] = useState(false);
  const loadConnectClusters = useKafkaStore((s) => s.loadConnectClusters);
  const loadConnectors = useKafkaStore((s) => s.loadConnectors);
  const loadConnectorDetail = useKafkaStore((s) => s.loadConnectorDetail);
  const selectedCluster = state.selectedConnectCluster || state.connectClusters[0]?.name || "";

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-4 py-2">
        <div className="w-60">
          <Select
            value={selectedCluster}
            onValueChange={(next) => {
              if (next) loadConnectors(tabId, next);
            }}
            disabled={!state.connectClusters.length}
          >
            <SelectTrigger className="h-8 text-xs">
              <SelectValue placeholder={t("query.kafkaConnectCluster")} />
            </SelectTrigger>
            <SelectContent>
              {state.connectClusters.map((cluster) => (
                <SelectItem key={cluster.name} value={cluster.name}>
                  {cluster.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button variant="outline" size="sm" className="h-8" onClick={() => loadConnectClusters(tabId)}>
          {state.loadingConnectClusters ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
          {t("query.refreshTree")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          disabled={!selectedCluster}
          onClick={() => setCreateOpen(true)}
        >
          <Plus className="h-3.5 w-3.5" />
          {t("query.kafkaCreateConnector")}
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          {t("query.kafkaConnectorTotal", { count: state.connectors.length })}
        </span>
      </div>
      {!state.connectClusters.length && !state.loadingConnectClusters ? (
        <EmptyState text={t("query.kafkaNoConnectClusters")} />
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(340px,0.8fr)_minmax(480px,1.2fr)]">
          <div className="min-h-0 overflow-auto border-r">
            {state.loadingConnectors && !state.connectors.length ? (
              <LoadingBlock />
            ) : state.connectors.length === 0 ? (
              <EmptyState text={t("query.kafkaNoConnectors")} />
            ) : (
              <ConnectorTable
                connectors={state.connectors}
                selected={state.selectedConnector}
                onSelect={(name) => loadConnectorDetail(tabId, name)}
              />
            )}
          </div>
          <div className="min-h-0 overflow-auto">
            <ConnectorDetailPanel tabId={tabId} state={state} />
          </div>
        </div>
      )}
      <CreateConnectorDialog tabId={tabId} open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function ConnectorTable({
  connectors,
  selected,
  onSelect,
}: {
  connectors: KafkaConnectorSummary[];
  selected?: string;
  onSelect: (name: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <table className="w-full text-sm">
      <thead className="sticky top-0 bg-muted/90 text-xs text-muted-foreground backdrop-blur">
        <tr>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaConnector")}</th>
          <th className="px-3 py-2 text-left font-medium">{t("query.kafkaState")}</th>
          <th className="px-3 py-2 text-right font-medium">{t("query.kafkaConnectorTasks")}</th>
        </tr>
      </thead>
      <tbody>
        {connectors.map((connector) => (
          <tr
            key={connector.name}
            className={`cursor-pointer border-t hover:bg-muted/40 ${selected === connector.name ? "bg-muted/60" : ""}`}
            onClick={() => onSelect(connector.name)}
          >
            <td className="max-w-[320px] truncate px-3 py-2 font-mono text-xs">{connector.name}</td>
            <td className="px-3 py-2">
              <div className="flex items-center gap-1.5">
                <StatusPill value={connector.status} />
                {connector.type && <StatusPill value={connector.type} />}
              </div>
            </td>
            <td className="px-3 py-2 text-right font-mono text-xs text-muted-foreground">
              {formatConnectorTaskSummary(connector)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ConnectorDetailPanel({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [updateOpen, setUpdateOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const [pauseOpen, setPauseOpen] = useState(false);
  const [resumeOpen, setResumeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const pauseConnector = useKafkaStore((s) => s.pauseConnector);
  const resumeConnector = useKafkaStore((s) => s.resumeConnector);
  const deleteConnector = useKafkaStore((s) => s.deleteConnector);

  if (state.loadingConnectorDetail) return <LoadingBlock />;
  if (!state.selectedConnector) return <EmptyState text={t("query.kafkaSelectConnector")} />;
  const detail = state.connectorDetail;
  if (!detail) return <EmptyState text={t("query.kafkaNoConnectorDetail")} />;

  const connectorState = detail.status?.connector?.state;
  const tasks = detail.status?.tasks || [];

  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center gap-2">
        <Settings className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1 truncate font-mono text-sm font-semibold">{detail.name}</div>
        <StatusPill value={detail.type || detail.status?.type} />
        <StatusPill value={connectorState} />
        <Button variant="outline" size="sm" className="h-7" onClick={() => setUpdateOpen(true)}>
          {t("query.kafkaUpdateConnectorConfig")}
        </Button>
        <Button variant="outline" size="sm" className="h-7" onClick={() => setPauseOpen(true)}>
          {t("query.kafkaPauseConnector")}
        </Button>
        <Button variant="outline" size="sm" className="h-7" onClick={() => setResumeOpen(true)}>
          {t("query.kafkaResumeConnector")}
        </Button>
        <Button variant="outline" size="sm" className="h-7" onClick={() => setRestartOpen(true)}>
          {t("query.kafkaRestartConnector")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-destructive hover:text-destructive"
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Metric label={t("query.kafkaConnectCluster")} value={state.selectedConnectCluster || "-"} />
        <Metric label={t("query.kafkaConnectorTasks")} value={tasks.length || detail.tasks?.length || 0} />
        <Metric label={t("query.kafkaConnectorState")} value={connectorState || "-"} />
      </div>
      <ConnectorTasksTable detail={detail} />
      <ConnectorConfigTable config={detail.config || {}} />
      <ConnectorConfigDialog
        tabId={tabId}
        detail={detail}
        mode="update"
        open={updateOpen}
        onOpenChange={setUpdateOpen}
      />
      <RestartConnectorDialog tabId={tabId} name={detail.name} open={restartOpen} onOpenChange={setRestartOpen} />
      <ConfirmDialog
        open={pauseOpen}
        onOpenChange={setPauseOpen}
        title={t("query.kafkaPauseConnector")}
        description={t("query.kafkaPauseConnectorConfirmDesc", { name: detail.name })}
        cancelText={t("action.cancel")}
        confirmText={t("query.kafkaPauseConnector")}
        onConfirm={async () => {
          await pauseConnector(tabId, detail.name);
          setPauseOpen(false);
        }}
      />
      <ConfirmDialog
        open={resumeOpen}
        onOpenChange={setResumeOpen}
        title={t("query.kafkaResumeConnector")}
        description={t("query.kafkaResumeConnectorConfirmDesc", { name: detail.name })}
        cancelText={t("action.cancel")}
        confirmText={t("query.kafkaResumeConnector")}
        onConfirm={async () => {
          await resumeConnector(tabId, detail.name);
          setResumeOpen(false);
        }}
      />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("query.kafkaDeleteConnector")}
        description={t("query.kafkaDeleteConnectorConfirmDesc", { name: detail.name })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={async () => {
          await deleteConnector(tabId, detail.name);
          setDeleteOpen(false);
        }}
      />
    </div>
  );
}

function ConnectorTasksTable({ detail }: { detail: KafkaConnectorDetail }) {
  const { t } = useTranslation();
  const rows = detail.status?.tasks || [];
  if (!rows.length) return <EmptyState text={t("query.kafkaNoConnectorTasks")} />;
  return (
    <div className="overflow-hidden rounded-md border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-right font-medium">ID</th>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaState")}</th>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaConnectorWorker")}</th>
            <th className="px-3 py-2 text-left font-medium">{t("query.kafkaConnectorTrace")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((task) => (
            <tr key={task.id} className="border-t">
              <td className="px-3 py-2 text-right font-mono text-xs">{task.id}</td>
              <td className="px-3 py-2">
                <StatusPill value={task.state} />
              </td>
              <td className="max-w-[220px] truncate px-3 py-2 font-mono text-xs">{task.workerId || "-"}</td>
              <td className="max-w-[360px] truncate px-3 py-2 font-mono text-xs text-muted-foreground">
                {task.trace || "-"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ConnectorConfigTable({ config }: { config: Record<string, string> }) {
  const { t } = useTranslation();
  const entries = Object.entries(config).sort(([a], [b]) => a.localeCompare(b));
  if (!entries.length) return <EmptyState text={t("query.kafkaNoConnectorConfig")} />;
  return (
    <div className="overflow-hidden rounded-md border">
      <div className="border-b px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("query.kafkaConnectorConfig")}
      </div>
      <table className="w-full text-sm">
        <tbody>
          {entries.map(([key, value]) => (
            <tr key={key} className="border-t first:border-t-0">
              <td className="w-64 max-w-[260px] truncate bg-muted/30 px-3 py-2 font-mono text-xs">{key}</td>
              <td className="break-all px-3 py-2 font-mono text-xs">{value}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CreateConnectorDialog({
  tabId,
  open,
  onOpenChange,
}: {
  tabId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return <ConnectorConfigDialog tabId={tabId} mode="create" open={open} onOpenChange={onOpenChange} />;
}

function ConnectorConfigDialog({
  tabId,
  detail,
  mode,
  open,
  onOpenChange,
}: {
  tabId: string;
  detail?: KafkaConnectorDetail;
  mode: "create" | "update";
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const createConnector = useKafkaStore((s) => s.createConnector);
  const updateConnectorConfig = useKafkaStore((s) => s.updateConnectorConfig);
  const state = useKafkaStore((s) => s.states[tabId]);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [name, setName] = useState(detail?.name || "");
  const [config, setConfig] = useState(formatConnectorConfig(detail?.config));
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName(detail?.name || "");
    setConfig(formatConnectorConfig(detail?.config));
    setFormError(null);
  }, [detail, open]);

  const canSubmit = name.trim() && config.trim();

  const submit = async () => {
    setFormError(null);
    let parsedConfig: Record<string, string>;
    try {
      parsedConfig = parseConnectorConfigObject(config);
    } catch (err) {
      setConfirmOpen(false);
      setFormError(errorMessage(err));
      return;
    }
    const req: KafkaConnectorConfigRequest = {
      name: name.trim(),
      config: parsedConfig,
    };
    try {
      if (mode === "create") {
        await createConnector(tabId, req);
      } else {
        await updateConnectorConfig(tabId, req);
      }
      onOpenChange(false);
    } catch (err) {
      setConfirmOpen(false);
      setFormError(errorMessage(err));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {mode === "create" ? t("query.kafkaCreateConnector") : t("query.kafkaUpdateConnectorConfig")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            className="h-8 font-mono text-xs"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={mode === "update"}
            placeholder={t("query.kafkaConnector")}
          />
          <Textarea
            className="min-h-80 font-mono text-xs"
            value={config}
            onChange={(e) => setConfig(e.target.value)}
            placeholder={t("query.kafkaConnectorConfigPlaceholder")}
          />
          {formError && (
            <div className="flex items-center gap-1.5 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 shrink-0" />
              <span className="break-all">{formError}</span>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.connectAdminLoading || !canSubmit} onClick={() => setConfirmOpen(true)}>
            {state?.connectAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {mode === "create" ? t("query.kafkaCreateConnector") : t("query.kafkaUpdateConnectorConfig")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={mode === "create" ? t("query.kafkaCreateConnector") : t("query.kafkaUpdateConnectorConfig")}
          description={
            mode === "create"
              ? t("query.kafkaCreateConnectorConfirmDesc", { name: name.trim() })
              : t("query.kafkaUpdateConnectorConfigConfirmDesc", { name: name.trim() })
          }
          cancelText={t("action.cancel")}
          confirmText={mode === "create" ? t("query.kafkaCreateConnector") : t("query.kafkaUpdateConnectorConfig")}
          onConfirm={submit}
        />
      </DialogContent>
    </Dialog>
  );
}

function RestartConnectorDialog({
  tabId,
  name,
  open,
  onOpenChange,
}: {
  tabId: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const restartConnector = useKafkaStore((s) => s.restartConnector);
  const state = useKafkaStore((s) => s.states[tabId]);
  const [includeTasks, setIncludeTasks] = useState(false);
  const [onlyFailed, setOnlyFailed] = useState(false);

  const submit = async () => {
    await restartConnector(tabId, name, includeTasks, onlyFailed);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaRestartConnector")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="rounded-md bg-muted/40 px-3 py-2 font-mono text-xs">{name}</div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={includeTasks}
              onCheckedChange={(checked) => {
                const next = checked === true;
                setIncludeTasks(next);
                if (!next) setOnlyFailed(false);
              }}
            />
            {t("query.kafkaRestartConnectorTasks")}
          </label>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={onlyFailed}
              disabled={!includeTasks}
              onCheckedChange={(checked) => setOnlyFailed(checked === true)}
            />
            {t("query.kafkaRestartConnectorOnlyFailed")}
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.connectAdminLoading} onClick={submit}>
            {state?.connectAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaRestartConnector")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function parseSchemaReferences(value: string): KafkaSchemaReference[] | undefined {
  const text = value.trim();
  if (!text) return undefined;
  const parsed = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    throw new Error("schema references must be a JSON array");
  }
  return parsed as KafkaSchemaReference[];
}

function formatSchema(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value || "-";
  }
}

function formatConnectorConfig(config?: Record<string, string>): string {
  if (config && Object.keys(config).length > 0) {
    return JSON.stringify(config, null, 2);
  }
  return JSON.stringify(
    {
      "connector.class": "",
      "tasks.max": "1",
    },
    null,
    2
  );
}

function parseConnectorConfigObject(value: string): Record<string, string> {
  const parsed = parseOptionalJsonObject(value);
  if (!parsed) {
    throw new Error("connector config must be a JSON object");
  }
  return Object.fromEntries(Object.entries(parsed).map(([key, item]) => [key, String(item)]));
}

function formatConnectorTaskSummary(connector: KafkaConnectorSummary): string {
  const total = connector.taskCount || 0;
  const failed = connector.failedTaskCount || 0;
  if (!total) return "-";
  if (!failed) return String(total);
  return `${total} / ${failed}`;
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function parseOptionalJsonObject(value: string): Record<string, string> | undefined {
  const text = value.trim();
  if (!text) return undefined;
  const parsed = JSON.parse(text);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("configs must be a JSON object");
  }
  return parsed as Record<string, string>;
}

function parseConfigUpdates(value: string): KafkaTopicConfigMutation[] {
  const text = value.trim();
  if (!text) return [];
  const parsed = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    throw new Error("config updates must be a JSON array");
  }
  return parsed as KafkaTopicConfigMutation[];
}

function parseIntegerArray(value: string): number[] | undefined {
  const text = value.trim();
  if (!text) return undefined;
  const parsed = JSON.parse(text);
  if (!Array.isArray(parsed) || parsed.some((item) => !Number.isInteger(item))) {
    throw new Error("partitions must be a JSON array of integers");
  }
  return parsed as number[];
}

function parseRequiredNumber(value: string): number {
  const n = Number(value.trim());
  if (!Number.isInteger(n)) {
    throw new Error("value must be an integer");
  }
  return n;
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
        <ConsumerGroupDetailPanel tabId={tabId} state={state} />
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

function ConsumerGroupDetailPanel({ tabId, state }: { tabId: string; state: KafkaTabState }) {
  const { t } = useTranslation();
  const [resetOpen, setResetOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const deleteConsumerGroup = useKafkaStore((s) => s.deleteConsumerGroup);
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
        <Button variant="outline" size="sm" className="h-7 gap-1.5" onClick={() => setResetOpen(true)}>
          <GitBranch className="h-3.5 w-3.5" />
          {t("query.kafkaResetOffset")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-destructive hover:text-destructive"
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
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
      <ResetConsumerGroupOffsetDialog
        tabId={tabId}
        group={detail.group}
        open={resetOpen}
        onOpenChange={setResetOpen}
      />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("query.kafkaDeleteConsumerGroup")}
        description={t("query.kafkaDeleteConsumerGroupConfirmDesc", { group: detail.group })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={() => deleteConsumerGroup(tabId, detail.group)}
      />
    </div>
  );
}

function ResetConsumerGroupOffsetDialog({
  tabId,
  group,
  open,
  onOpenChange,
}: {
  tabId: string;
  group: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [topic, setTopic] = useState("");
  const [partitions, setPartitions] = useState("");
  const [mode, setMode] = useState<KafkaOffsetResetMode>("latest");
  const [offset, setOffset] = useState("");
  const [timestampMillis, setTimestampMillis] = useState("");
  const resetConsumerGroupOffset = useKafkaStore((s) => s.resetConsumerGroupOffset);
  const state = useKafkaStore((s) => s.states[tabId]);

  const confirm = async () => {
    await resetConsumerGroupOffset(tabId, {
      group,
      topic: topic.trim(),
      partitions: parseIntegerArray(partitions),
      mode,
      offset: mode === "offset" ? parseRequiredNumber(offset) : undefined,
      timestampMillis: mode === "timestamp" ? parseRequiredNumber(timestampMillis) : undefined,
    });
    onOpenChange(false);
  };

  const modeValue = mode === "timestamp" ? timestampMillis : offset;
  const canSubmit = topic.trim() && (mode === "offset" ? offset.trim() : true) && (mode === "timestamp" ? timestampMillis.trim() : true);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("query.kafkaResetOffset")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="rounded-md bg-muted/40 px-3 py-2 font-mono text-xs">{group}</div>
          <Input
            className="h-8 font-mono text-xs"
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            placeholder={t("query.kafkaTopic")}
          />
          <Input
            className="h-8 font-mono text-xs"
            value={partitions}
            onChange={(e) => setPartitions(e.target.value)}
            placeholder={t("query.kafkaPartitionsPlaceholder")}
          />
          <div className="grid gap-2 md:grid-cols-2">
            <Select value={mode} onValueChange={(next) => setMode(next as KafkaOffsetResetMode)}>
              <SelectTrigger className="h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="earliest">{t("query.kafkaOffsetEarliest")}</SelectItem>
                <SelectItem value="latest">{t("query.kafkaOffsetLatest")}</SelectItem>
                <SelectItem value="offset">Offset</SelectItem>
                <SelectItem value="timestamp">{t("query.kafkaStartTimestamp")}</SelectItem>
              </SelectContent>
            </Select>
            <Input
              className="h-8 font-mono text-xs"
              value={modeValue}
              onChange={(e) => (mode === "timestamp" ? setTimestampMillis(e.target.value) : setOffset(e.target.value))}
              disabled={mode === "earliest" || mode === "latest"}
              placeholder={mode === "timestamp" ? t("query.kafkaTimestampMillis") : t("query.kafkaOffset")}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button disabled={state?.groupAdminLoading || !canSubmit} onClick={() => setConfirmOpen(true)}>
            {state?.groupAdminLoading && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("query.kafkaResetOffset")}
          </Button>
        </DialogFooter>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t("query.kafkaResetOffset")}
          description={t("query.kafkaResetOffsetConfirmDesc", { group, topic: topic.trim(), mode })}
          cancelText={t("action.cancel")}
          confirmText={t("query.kafkaResetOffset")}
          onConfirm={confirm}
        />
      </DialogContent>
    </Dialog>
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
