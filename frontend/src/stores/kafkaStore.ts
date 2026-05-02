import { create } from "zustand";
import {
  KafkaAlterTopicConfig,
  KafkaBrowseMessages,
  KafkaClusterOverview,
  KafkaCheckSchemaCompatibility,
  KafkaCreateACL,
  KafkaCreateConnector,
  KafkaCreateTopic,
  KafkaDeleteACL,
  KafkaDeleteConnector,
  KafkaDeleteRecords,
  KafkaDeleteSchema,
  KafkaDeleteTopic,
  KafkaDeleteConsumerGroup,
  KafkaGetBrokerConfig,
  KafkaGetSchema,
  KafkaGetSchemaSubjectVersions,
  KafkaGetConnector,
  KafkaGetConsumerGroup,
  KafkaGetTopic,
  KafkaIncreasePartitions,
  KafkaListACLs,
  KafkaListBrokers,
  KafkaListClusterConfigs,
  KafkaListConnectClusters,
  KafkaListConnectors,
  KafkaListConsumerGroups,
  KafkaListSchemaSubjects,
  KafkaListTopics,
  KafkaProduceMessage,
  KafkaRegisterSchema,
  KafkaPauseConnector,
  KafkaRestartConnector,
  KafkaResumeConnector,
  KafkaResetConsumerGroupOffset,
  KafkaUpdateConnectorConfig,
} from "../../wailsjs/go/app/App";
import { kafka_svc } from "../../wailsjs/go/models";
import { registerTabCloseHook, type QueryTabMeta } from "./tabStore";
import { useTabStore } from "./tabStore";

export type KafkaView = "overview" | "brokers" | "topics" | "consumerGroups" | "acls" | "schemas" | "connect";
export type KafkaMessageStartMode = "newest" | "oldest" | "offset" | "timestamp";
export type KafkaPayloadEncoding = "text" | "json" | "hex" | "base64";
export type KafkaOffsetResetMode = "earliest" | "latest" | "offset" | "timestamp";

export interface KafkaClusterOverviewInfo {
  assetId: number;
  clusterId: string;
  controllerId: number;
  brokerCount: number;
  topicCount: number;
  internalTopicCount: number;
  partitionCount: number;
  offlinePartitionCount: number;
  underReplicatedPartitionCount: number;
}

export interface KafkaBroker {
  nodeId: number;
  host: string;
  port: number;
  rack?: string;
}

export interface KafkaConfigEntry {
  name: string;
  value?: string;
  isSensitive: boolean;
  source?: string;
}

export interface KafkaBrokerConfig {
  brokerId: number;
  configs?: KafkaConfigEntry[];
  error?: string;
}

export interface KafkaClusterConfigs {
  configs?: KafkaConfigEntry[];
  error?: string;
}

export interface KafkaTopicSummary {
  name: string;
  id?: string;
  internal: boolean;
  partitionCount: number;
  replicationFactor: number;
  offlinePartitionCount: number;
  underReplicatedPartitionCount: number;
  error?: string;
}

export interface KafkaTopicPartition {
  partition: number;
  leader: number;
  leaderEpoch: number;
  replicas: number[];
  isr: number[];
  offlineReplicas: number[];
  error?: string;
}

export interface KafkaTopicDetail extends KafkaTopicSummary {
  partitions: KafkaTopicPartition[];
  authorizedOperations?: string[];
}

export interface KafkaConsumerGroup {
  group: string;
  coordinator: number;
  protocolType?: string;
  state?: string;
}

export interface KafkaConsumerGroupMember {
  memberId: string;
  instanceId?: string;
  clientId: string;
  clientHost: string;
  assignedPartitions?: { topic: string; partitions: number[] }[];
}

export interface KafkaConsumerGroupLag {
  topic: string;
  partition: number;
  committedOffset: number;
  endOffset: number;
  lag: number;
  memberId?: string;
  error?: string;
}

export interface KafkaConsumerGroupDetail {
  group: string;
  coordinator: KafkaBroker;
  state?: string;
  protocolType?: string;
  protocol?: string;
  members: KafkaConsumerGroupMember[];
  lag?: KafkaConsumerGroupLag[];
  totalLag: number;
  error?: string;
  lagError?: string;
}

export interface KafkaTopicListResponse {
  topics: KafkaTopicSummary[];
  total: number;
  page: number;
  pageSize: number;
}

export interface KafkaACL {
  resourceType: string;
  resourceName: string;
  patternType: string;
  principal: string;
  host: string;
  operation: string;
  permission: string;
  error?: string;
}

export interface KafkaACLListResponse {
  acls: KafkaACL[];
  total: number;
  page: number;
  pageSize: number;
}

export interface KafkaACLFilters {
  resourceType: string;
  resourceName: string;
  patternType: string;
  principal: string;
  host: string;
  operation: string;
  permission: string;
}

export interface KafkaACLMutationRequest {
  resourceType: string;
  resourceName?: string;
  patternType?: string;
  principal: string;
  host?: string;
  operation: string;
  permission: string;
}

export interface KafkaSchemaReference {
  name: string;
  subject: string;
  version: number;
}

export interface KafkaSchemaSubjectVersions {
  subject: string;
  versions: number[];
}

export interface KafkaSchemaVersionDetail {
  subject: string;
  id: number;
  version: number;
  schema: string;
  schemaType?: string;
  references?: KafkaSchemaReference[];
}

export interface KafkaRegisterSchemaRequest {
  subject: string;
  schema: string;
  schemaType?: string;
  references?: KafkaSchemaReference[];
}

export interface KafkaSchemaCompatibilityRequest extends KafkaRegisterSchemaRequest {
  version?: string;
}

export interface KafkaSchemaCompatibilityResponse {
  subject: string;
  version: string;
  compatible: boolean;
  messages?: string[];
}

export interface KafkaDeleteSchemaRequest {
  subject: string;
  version?: string;
  permanent?: boolean;
}

export interface KafkaConnectCluster {
  name: string;
  url: string;
}

export interface KafkaConnectorSummary {
  name: string;
  type?: string;
  status?: string;
  taskCount?: number;
  failedTaskCount?: number;
}

export interface KafkaConnectorTask {
  connector?: string;
  task: number;
}

export interface KafkaConnectorWorkerState {
  state?: string;
  workerId?: string;
  trace?: string;
}

export interface KafkaConnectorTaskState {
  id: number;
  state?: string;
  workerId?: string;
  trace?: string;
}

export interface KafkaConnectorStatus {
  name?: string;
  connector?: KafkaConnectorWorkerState;
  tasks?: KafkaConnectorTaskState[];
  type?: string;
}

export interface KafkaConnectorDetail {
  name: string;
  type?: string;
  config?: Record<string, string>;
  tasks?: KafkaConnectorTask[];
  status?: KafkaConnectorStatus;
}

export interface KafkaConnectorConfigRequest {
  cluster?: string;
  name: string;
  config: Record<string, string>;
}

export interface KafkaRecordHeader {
  key: string;
  value?: string;
  valueBytes: number;
  valueEncoding: string;
  valueTruncated: boolean;
}

export interface KafkaRecord {
  topic: string;
  partition: number;
  offset: number;
  timestamp: string;
  timestampMillis: number;
  key?: string;
  keyBytes: number;
  keyEncoding: string;
  keyTruncated: boolean;
  value?: string;
  valueBytes: number;
  valueEncoding: string;
  valueTruncated: boolean;
  headers?: KafkaRecordHeader[];
}

export interface KafkaBrowseMessagesResponse {
  topic: string;
  partitions: number[];
  startMode: KafkaMessageStartMode;
  limit: number;
  maxBytes: number;
  records: KafkaRecord[];
  nextOffset?: Record<string, number>;
  errors?: string[];
}

export interface KafkaMessageBrowserState {
  partition: string;
  startMode: KafkaMessageStartMode;
  offset: string;
  timestampMillis: string;
  limit: number;
  maxBytes: number;
  decodeMode: KafkaPayloadEncoding;
  maxWaitMillis: number;
  response?: KafkaBrowseMessagesResponse;
}

export interface KafkaProduceState {
  partition: string;
  key: string;
  value: string;
  headers: string;
  keyEncoding: KafkaPayloadEncoding;
  valueEncoding: KafkaPayloadEncoding;
}

export interface KafkaTopicConfigMutation {
  name: string;
  value?: string;
  op?: "set" | "delete" | "append" | "subtract";
}

export interface KafkaDeleteRecordsPartition {
  partition: number;
  offset: number;
}

export interface KafkaResetConsumerGroupOffsetRequest {
  group: string;
  topic: string;
  partitions?: number[];
  mode: KafkaOffsetResetMode;
  offset?: number;
  timestampMillis?: number;
}

export interface KafkaTabState {
  activeView: KafkaView;
  overview?: KafkaClusterOverviewInfo;
  brokers: KafkaBroker[];
  selectedBroker?: number;
  brokerConfig?: KafkaBrokerConfig;
  clusterConfigs?: KafkaClusterConfigs;
  topics: KafkaTopicSummary[];
  topicsTotal: number;
  topicSearch: string;
  includeInternal: boolean;
  selectedTopic?: string;
  topicDetail?: KafkaTopicDetail;
  consumerGroups: KafkaConsumerGroup[];
  selectedGroup?: string;
  groupDetail?: KafkaConsumerGroupDetail;
  acls: KafkaACL[];
  aclsTotal: number;
  aclFilters: KafkaACLFilters;
  schemaSubjects: string[];
  selectedSchemaSubject?: string;
  schemaVersions?: KafkaSchemaSubjectVersions;
  schemaDetail?: KafkaSchemaVersionDetail;
  connectClusters: KafkaConnectCluster[];
  selectedConnectCluster?: string;
  connectors: KafkaConnectorSummary[];
  selectedConnector?: string;
  connectorDetail?: KafkaConnectorDetail;
  messageBrowser: KafkaMessageBrowserState;
  produceMessage: KafkaProduceState;
  loadingOverview: boolean;
  loadingBrokers: boolean;
  loadingBrokerConfig: boolean;
  loadingClusterConfigs: boolean;
  loadingTopics: boolean;
  loadingTopicDetail: boolean;
  loadingMessages: boolean;
  producingMessage: boolean;
  topicAdminLoading: boolean;
  groupAdminLoading: boolean;
  loadingGroups: boolean;
  loadingGroupDetail: boolean;
  loadingACLs: boolean;
  aclAdminLoading: boolean;
  loadingSchemaSubjects: boolean;
  loadingSchemaDetail: boolean;
  schemaAdminLoading: boolean;
  loadingConnectClusters: boolean;
  loadingConnectors: boolean;
  loadingConnectorDetail: boolean;
  connectAdminLoading: boolean;
  error: string | null;
}

interface KafkaStoreState {
  states: Record<string, KafkaTabState>;
  ensureTab: (tabId: string) => void;
  setActiveView: (tabId: string, view: KafkaView) => void;
  setTopicSearch: (tabId: string, value: string) => void;
  setIncludeInternal: (tabId: string, value: boolean) => void;
  setACLFilters: (tabId: string, patch: Partial<KafkaACLFilters>) => void;
  setMessageBrowser: (tabId: string, patch: Partial<KafkaMessageBrowserState>) => void;
  setProduceMessage: (tabId: string, patch: Partial<KafkaProduceState>) => void;
  loadOverview: (tabId: string) => Promise<void>;
  loadBrokers: (tabId: string) => Promise<void>;
  loadBrokerConfig: (tabId: string, brokerId: number) => Promise<void>;
  loadClusterConfigs: (tabId: string) => Promise<void>;
  loadTopics: (tabId: string) => Promise<void>;
  loadTopicDetail: (tabId: string, topic: string) => Promise<void>;
  createTopic: (tabId: string, req: { topic: string; partitions: number; replicationFactor: number; configs?: Record<string, string> }) => Promise<void>;
  deleteTopic: (tabId: string, topic: string) => Promise<void>;
  alterTopicConfig: (tabId: string, topic: string, configs: KafkaTopicConfigMutation[]) => Promise<void>;
  increasePartitions: (tabId: string, topic: string, partitions: number) => Promise<void>;
  deleteTopicRecords: (tabId: string, topic: string, partitions: KafkaDeleteRecordsPartition[]) => Promise<void>;
  resetConsumerGroupOffset: (tabId: string, req: KafkaResetConsumerGroupOffsetRequest) => Promise<void>;
  deleteConsumerGroup: (tabId: string, group: string) => Promise<void>;
  loadACLs: (tabId: string) => Promise<void>;
  createACL: (tabId: string, req: KafkaACLMutationRequest) => Promise<void>;
  deleteACL: (tabId: string, acl: KafkaACL) => Promise<void>;
  loadSchemaSubjects: (tabId: string) => Promise<void>;
  loadSchemaVersions: (tabId: string, subject: string) => Promise<void>;
  loadSchema: (tabId: string, subject: string, version?: string) => Promise<void>;
  checkSchemaCompatibility: (
    tabId: string,
    req: KafkaSchemaCompatibilityRequest
  ) => Promise<KafkaSchemaCompatibilityResponse | undefined>;
  registerSchema: (tabId: string, req: KafkaRegisterSchemaRequest) => Promise<void>;
  deleteSchema: (tabId: string, req: KafkaDeleteSchemaRequest) => Promise<void>;
  loadConnectClusters: (tabId: string) => Promise<void>;
  loadConnectors: (tabId: string, cluster?: string) => Promise<void>;
  loadConnectorDetail: (tabId: string, name: string) => Promise<void>;
  createConnector: (tabId: string, req: KafkaConnectorConfigRequest) => Promise<void>;
  updateConnectorConfig: (tabId: string, req: KafkaConnectorConfigRequest) => Promise<void>;
  pauseConnector: (tabId: string, name: string) => Promise<void>;
  resumeConnector: (tabId: string, name: string) => Promise<void>;
  restartConnector: (tabId: string, name: string, includeTasks?: boolean, onlyFailed?: boolean) => Promise<void>;
  deleteConnector: (tabId: string, name: string) => Promise<void>;
  browseMessages: (tabId: string) => Promise<void>;
  produceKafkaMessage: (tabId: string) => Promise<void>;
  loadConsumerGroups: (tabId: string) => Promise<void>;
  loadConsumerGroupDetail: (tabId: string, group: string) => Promise<void>;
  refreshActiveView: (tabId: string) => Promise<void>;
}

function defaultKafkaState(): KafkaTabState {
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
    aclFilters: defaultACLFilters(),
    schemaSubjects: [],
    connectClusters: [],
    connectors: [],
    messageBrowser: defaultMessageBrowserState(),
    produceMessage: defaultProduceState(),
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

function defaultACLFilters(): KafkaACLFilters {
  return {
    resourceType: "any",
    resourceName: "",
    patternType: "any",
    principal: "",
    host: "",
    operation: "any",
    permission: "any",
  };
}

function defaultMessageBrowserState(): KafkaMessageBrowserState {
  return {
    partition: "",
    startMode: "newest",
    offset: "",
    timestampMillis: "",
    limit: 50,
    maxBytes: 4096,
    decodeMode: "text",
    maxWaitMillis: 1000,
  };
}

function defaultProduceState(): KafkaProduceState {
  return {
    partition: "",
    key: "",
    value: "",
    headers: "",
    keyEncoding: "text",
    valueEncoding: "text",
  };
}

function getKafkaAssetId(tabId: string): number | null {
  const tab = useTabStore.getState().tabs.find((item) => item.id === tabId);
  if (!tab || tab.type !== "query") return null;
  const meta = tab.meta as QueryTabMeta;
  if (meta.assetType !== "kafka") return null;
  return meta.assetId;
}

export const useKafkaStore = create<KafkaStoreState>((set, get) => ({
  states: {},

  ensureTab: (tabId) => {
    if (get().states[tabId]) return;
    set((s) => ({ states: { ...s.states, [tabId]: defaultKafkaState() } }));
  },

  setActiveView: (tabId, view) => {
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], activeView: view } } }));
  },

  setTopicSearch: (tabId, value) => {
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicSearch: value } } }));
  },

  setIncludeInternal: (tabId, value) => {
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], includeInternal: value } } }));
  },

  setACLFilters: (tabId, patch) => {
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          aclFilters: { ...defaultACLFilters(), ...s.states[tabId].aclFilters, ...patch },
        },
      },
    }));
  },

  setMessageBrowser: (tabId, patch) => {
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          messageBrowser: { ...s.states[tabId].messageBrowser, ...patch },
        },
      },
    }));
  },

  setProduceMessage: (tabId, patch) => {
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          produceMessage: { ...s.states[tabId].produceMessage, ...patch },
        },
      },
    }));
  },

  loadOverview: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingOverview: true } } }));
    try {
      const overview = (await KafkaClusterOverview(assetId)) as KafkaClusterOverviewInfo;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], overview, loadingOverview: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingOverview: false, error: String(err) } },
      }));
    }
  },

  loadBrokers: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingBrokers: true } } }));
    try {
      const brokers = ((await KafkaListBrokers(assetId)) || []) as KafkaBroker[];
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], brokers, loadingBrokers: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingBrokers: false, error: String(err) } },
      }));
    }
  },

  loadBrokerConfig: async (tabId, brokerId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({
      states: { ...s.states, [tabId]: { ...s.states[tabId], selectedBroker: brokerId, brokerConfig: undefined, loadingBrokerConfig: true } },
    }));
    try {
      const result = (await KafkaGetBrokerConfig(assetId, brokerId)) as KafkaBrokerConfig;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], brokerConfig: result, loadingBrokerConfig: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingBrokerConfig: false, error: String(err) } },
      }));
    }
  },

  loadClusterConfigs: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({
      states: { ...s.states, [tabId]: { ...s.states[tabId], clusterConfigs: undefined, loadingClusterConfigs: true } },
    }));
    try {
      const result = (await KafkaListClusterConfigs(assetId)) as KafkaClusterConfigs;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], clusterConfigs: result, loadingClusterConfigs: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingClusterConfigs: false, error: String(err) } },
      }));
    }
  },

  loadTopics: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const state = get().states[tabId] || defaultKafkaState();
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingTopics: true } } }));
    try {
      const response = (await KafkaListTopics({
        assetId,
        includeInternal: state.includeInternal,
        search: state.topicSearch,
        page: 1,
        pageSize: 200,
      })) as KafkaTopicListResponse;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            topics: response.topics || [],
            topicsTotal: response.total || 0,
            loadingTopics: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingTopics: false, error: String(err) } },
      }));
    }
  },

  loadTopicDetail: async (tabId, topic) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId || !topic) return;
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          selectedTopic: topic,
          topicDetail: undefined,
          messageBrowser: { ...s.states[tabId].messageBrowser, response: undefined },
          loadingTopicDetail: true,
        },
      },
    }));
    try {
      const topicDetail = (await KafkaGetTopic(assetId, topic)) as KafkaTopicDetail;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicDetail, loadingTopicDetail: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingTopicDetail: false, error: String(err) } },
      }));
    }
  },

  createTopic: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: true } } }));
    try {
      await KafkaCreateTopic({
        assetId,
        topic: req.topic,
        partitions: req.partitions,
        replicationFactor: req.replicationFactor,
        configs: req.configs,
      });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: null } } }));
      await Promise.all([get().loadTopics(tabId), get().loadOverview(tabId)]);
      await get().loadTopicDetail(tabId, req.topic);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  deleteTopic: async (tabId, topic) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: true } } }));
    try {
      await KafkaDeleteTopic(assetId, topic);
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            selectedTopic: undefined,
            topicDetail: undefined,
            messageBrowser: { ...s.states[tabId].messageBrowser, response: undefined },
            topicAdminLoading: false,
            error: null,
          },
        },
      }));
      await Promise.all([get().loadTopics(tabId), get().loadOverview(tabId)]);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  alterTopicConfig: async (tabId, topic, configs) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: true } } }));
    try {
      await KafkaAlterTopicConfig(new kafka_svc.AlterTopicConfigRequest({ assetId, topic, configs }));
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: null } } }));
      await get().loadTopicDetail(tabId, topic);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  increasePartitions: async (tabId, topic, partitions) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: true } } }));
    try {
      await KafkaIncreasePartitions({ assetId, topic, partitions });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: null } } }));
      await Promise.all([get().loadTopics(tabId), get().loadTopicDetail(tabId, topic), get().loadOverview(tabId)]);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  deleteTopicRecords: async (tabId, topic, partitions) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: true } } }));
    try {
      await KafkaDeleteRecords(new kafka_svc.DeleteRecordsRequest({ assetId, topic, partitions }));
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            messageBrowser: { ...s.states[tabId].messageBrowser, response: undefined },
            topicAdminLoading: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], topicAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  resetConsumerGroupOffset: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], groupAdminLoading: true } } }));
    try {
      await KafkaResetConsumerGroupOffset({ assetId, ...req });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], groupAdminLoading: false, error: null } } }));
      await get().loadConsumerGroupDetail(tabId, req.group);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], groupAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  deleteConsumerGroup: async (tabId, group) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], groupAdminLoading: true } } }));
    try {
      await KafkaDeleteConsumerGroup(assetId, group);
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            selectedGroup: undefined,
            groupDetail: undefined,
            groupAdminLoading: false,
            error: null,
          },
        },
      }));
      await get().loadConsumerGroups(tabId);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], groupAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  loadACLs: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const state = get().states[tabId] || defaultKafkaState();
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingACLs: true } } }));
    try {
      const filters = state.aclFilters || defaultACLFilters();
      const response = (await KafkaListACLs({
        assetId,
        resourceType: filters.resourceType === "any" ? "" : filters.resourceType,
        resourceName: filters.resourceName,
        patternType: filters.patternType === "any" ? "" : filters.patternType,
        principal: filters.principal,
        host: filters.host,
        operation: filters.operation === "any" ? "" : filters.operation,
        permission: filters.permission === "any" ? "" : filters.permission,
        page: 1,
        pageSize: 200,
      })) as KafkaACLListResponse;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            acls: response.acls || [],
            aclsTotal: response.total || 0,
            loadingACLs: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingACLs: false, error: String(err) } },
      }));
    }
  },

  createACL: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: true } } }));
    try {
      await KafkaCreateACL({ assetId, ...req });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: false, error: null } } }));
      await get().loadACLs(tabId);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  deleteACL: async (tabId, acl) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: true } } }));
    try {
      await KafkaDeleteACL({
        assetId,
        resourceType: acl.resourceType,
        resourceName: acl.resourceName,
        patternType: acl.patternType,
        principal: acl.principal,
        host: acl.host,
        operation: acl.operation,
        permission: acl.permission,
      });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: false, error: null } } }));
      await get().loadACLs(tabId);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], aclAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  loadSchemaSubjects: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingSchemaSubjects: true } } }));
    try {
      const schemaSubjects = ((await KafkaListSchemaSubjects(assetId)) || []) as string[];
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            schemaSubjects,
            loadingSchemaSubjects: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: { ...s.states[tabId], loadingSchemaSubjects: false, error: String(err) },
        },
      }));
    }
  },

  loadSchemaVersions: async (tabId, subject) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId || !subject) return;
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          selectedSchemaSubject: subject,
          schemaVersions: undefined,
          schemaDetail: undefined,
          loadingSchemaDetail: true,
        },
      },
    }));
    try {
      const schemaVersions = (await KafkaGetSchemaSubjectVersions(assetId, subject)) as KafkaSchemaSubjectVersions;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], schemaVersions, loadingSchemaDetail: false, error: null } },
      }));
      const latest = schemaVersions.versions?.[schemaVersions.versions.length - 1];
      if (latest !== undefined) {
        await get().loadSchema(tabId, subject, String(latest));
      }
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingSchemaDetail: false, error: String(err) } },
      }));
    }
  },

  loadSchema: async (tabId, subject, version) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId || !subject) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingSchemaDetail: true } } }));
    try {
      const schemaDetail = (await KafkaGetSchema(assetId, subject, version || "latest")) as KafkaSchemaVersionDetail;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: { ...s.states[tabId], selectedSchemaSubject: subject, schemaDetail, loadingSchemaDetail: false, error: null },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingSchemaDetail: false, error: String(err) } },
      }));
    }
  },

  checkSchemaCompatibility: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return undefined;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: true } } }));
    try {
      const result = (await KafkaCheckSchemaCompatibility(
        new kafka_svc.CheckSchemaCompatibilityRequest({ assetId, ...req })
      )) as KafkaSchemaCompatibilityResponse;
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: false, error: null } } }));
      return result;
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  registerSchema: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: true } } }));
    try {
      await KafkaRegisterSchema(new kafka_svc.RegisterSchemaRequest({ assetId, ...req }));
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: false, error: null } } }));
      await get().loadSchemaSubjects(tabId);
      await get().loadSchemaVersions(tabId, req.subject);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  deleteSchema: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: true } } }));
    try {
      await KafkaDeleteSchema({ assetId, ...req });
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            selectedSchemaSubject: req.version ? s.states[tabId].selectedSchemaSubject : undefined,
            schemaVersions: undefined,
            schemaDetail: undefined,
            schemaAdminLoading: false,
            error: null,
          },
        },
      }));
      await get().loadSchemaSubjects(tabId);
      if (req.version) {
        await get().loadSchemaVersions(tabId, req.subject);
      }
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], schemaAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  loadConnectClusters: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingConnectClusters: true } } }));
    try {
      const connectClusters = ((await KafkaListConnectClusters(assetId)) || []) as KafkaConnectCluster[];
      const selectedConnectCluster = get().states[tabId]?.selectedConnectCluster || connectClusters[0]?.name;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            connectClusters,
            selectedConnectCluster,
            loadingConnectClusters: false,
            error: null,
          },
        },
      }));
      if (selectedConnectCluster) {
        await get().loadConnectors(tabId, selectedConnectCluster);
      }
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingConnectClusters: false, error: String(err) } },
      }));
    }
  },

  loadConnectors: async (tabId, cluster) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const selectedConnectCluster = cluster || get().states[tabId]?.selectedConnectCluster || "";
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: {
          ...s.states[tabId],
          selectedConnectCluster,
          connectors: [],
          selectedConnector: undefined,
          connectorDetail: undefined,
          loadingConnectors: true,
        },
      },
    }));
    try {
      const connectors = ((await KafkaListConnectors({ assetId, cluster: selectedConnectCluster })) || []) as KafkaConnectorSummary[];
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], connectors, loadingConnectors: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingConnectors: false, error: String(err) } },
      }));
    }
  },

  loadConnectorDetail: async (tabId, name) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId || !name) return;
    get().ensureTab(tabId);
    const cluster = get().states[tabId]?.selectedConnectCluster || "";
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: { ...s.states[tabId], selectedConnector: name, connectorDetail: undefined, loadingConnectorDetail: true },
      },
    }));
    try {
      const connectorDetail = (await KafkaGetConnector(assetId, cluster, name)) as KafkaConnectorDetail;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: { ...s.states[tabId], connectorDetail, loadingConnectorDetail: false, error: null },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingConnectorDetail: false, error: String(err) } },
      }));
    }
  },

  createConnector: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const cluster = req.cluster || get().states[tabId]?.selectedConnectCluster || "";
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: true } } }));
    try {
      await KafkaCreateConnector({ assetId, ...req, cluster });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: null } } }));
      await get().loadConnectors(tabId, cluster);
      await get().loadConnectorDetail(tabId, req.name);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  updateConnectorConfig: async (tabId, req) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const cluster = req.cluster || get().states[tabId]?.selectedConnectCluster || "";
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: true } } }));
    try {
      await KafkaUpdateConnectorConfig({ assetId, ...req, cluster });
      set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: null } } }));
      await get().loadConnectorDetail(tabId, req.name);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  pauseConnector: async (tabId, name) => {
    await mutateConnectorState(tabId, name, (assetId, cluster) => KafkaPauseConnector(assetId, cluster, name), get, set);
  },

  resumeConnector: async (tabId, name) => {
    await mutateConnectorState(tabId, name, (assetId, cluster) => KafkaResumeConnector(assetId, cluster, name), get, set);
  },

  restartConnector: async (tabId, name, includeTasks, onlyFailed) => {
    await mutateConnectorState(
      tabId,
      name,
      (assetId, cluster) => KafkaRestartConnector({ assetId, cluster, name, includeTasks, onlyFailed }),
      get,
      set
    );
  },

  deleteConnector: async (tabId, name) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const cluster = get().states[tabId]?.selectedConnectCluster || "";
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: true } } }));
    try {
      await KafkaDeleteConnector(assetId, cluster, name);
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            selectedConnector: undefined,
            connectorDetail: undefined,
            connectAdminLoading: false,
            error: null,
          },
        },
      }));
      await get().loadConnectors(tabId, cluster);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: String(err) } },
      }));
      throw err;
    }
  },

  browseMessages: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const state = get().states[tabId] || defaultKafkaState();
    const topic = state.selectedTopic;
    if (!topic) return;
    const browser = state.messageBrowser;
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingMessages: true } } }));
    try {
      const req = new kafka_svc.BrowseMessagesRequest({
        assetId,
        topic,
        startMode: browser.startMode,
        limit: browser.limit,
        maxBytes: browser.maxBytes,
        decodeMode: browser.decodeMode,
        maxWaitMillis: browser.maxWaitMillis,
      });
      const partition = parseOptionalInteger(browser.partition, "partition");
      if (partition !== undefined) req.partition = partition;
      if (browser.startMode === "offset") req.offset = parseRequiredInteger(browser.offset, "offset");
      if (browser.startMode === "timestamp") {
        req.timestampMillis = parseRequiredInteger(browser.timestampMillis, "timestampMillis");
      }
      const response = (await KafkaBrowseMessages(req)) as KafkaBrowseMessagesResponse;
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            messageBrowser: { ...s.states[tabId].messageBrowser, response },
            loadingMessages: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingMessages: false, error: String(err) } },
      }));
    }
  },

  produceKafkaMessage: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    const state = get().states[tabId] || defaultKafkaState();
    const topic = state.selectedTopic;
    if (!topic) return;
    const form = state.produceMessage;
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], producingMessage: true } } }));
    try {
      const req = new kafka_svc.ProduceMessageRequest({
        assetId,
        topic,
        key: form.key,
        keyEncoding: form.keyEncoding,
        value: form.value,
        valueEncoding: form.valueEncoding,
      });
      const partition = parseOptionalInteger(form.partition, "partition");
      if (partition !== undefined) req.partition = partition;
      const headers = parseHeaders(form.headers);
      if (headers.length > 0) req.headers = headers;
      await KafkaProduceMessage(req);
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: {
            ...s.states[tabId],
            producingMessage: false,
            produceMessage: { ...s.states[tabId].produceMessage, value: "" },
            error: null,
          },
        },
      }));
      await get().browseMessages(tabId);
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], producingMessage: false, error: String(err) } },
      }));
    }
  },

  loadConsumerGroups: async (tabId) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId) return;
    get().ensureTab(tabId);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], loadingGroups: true } } }));
    try {
      const consumerGroups = ((await KafkaListConsumerGroups(assetId)) || []) as KafkaConsumerGroup[];
      set((s) => ({
        states: {
          ...s.states,
          [tabId]: { ...s.states[tabId], consumerGroups, loadingGroups: false, error: null },
        },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingGroups: false, error: String(err) } },
      }));
    }
  },

  loadConsumerGroupDetail: async (tabId, group) => {
    const assetId = getKafkaAssetId(tabId);
    if (!assetId || !group) return;
    get().ensureTab(tabId);
    set((s) => ({
      states: {
        ...s.states,
        [tabId]: { ...s.states[tabId], selectedGroup: group, groupDetail: undefined, loadingGroupDetail: true },
      },
    }));
    try {
      const groupDetail = (await KafkaGetConsumerGroup(assetId, group)) as KafkaConsumerGroupDetail;
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], groupDetail, loadingGroupDetail: false, error: null } },
      }));
    } catch (err) {
      set((s) => ({
        states: { ...s.states, [tabId]: { ...s.states[tabId], loadingGroupDetail: false, error: String(err) } },
      }));
    }
  },

  refreshActiveView: async (tabId) => {
    get().ensureTab(tabId);
    const view = get().states[tabId]?.activeView || "overview";
    if (view === "overview") {
      await Promise.all([get().loadOverview(tabId), get().loadBrokers(tabId), get().loadTopics(tabId)]);
    } else if (view === "brokers") {
      await get().loadBrokers(tabId);
    } else if (view === "topics") {
      await get().loadTopics(tabId);
    } else if (view === "acls") {
      await get().loadACLs(tabId);
    } else if (view === "schemas") {
      await get().loadSchemaSubjects(tabId);
    } else if (view === "connect") {
      await get().loadConnectClusters(tabId);
    } else {
      await get().loadConsumerGroups(tabId);
    }
  },
}));

async function mutateConnectorState(
  tabId: string,
  name: string,
  fn: (assetId: number, cluster: string) => Promise<unknown>,
  get: () => KafkaStoreState,
  set: (partial: KafkaStoreState | Partial<KafkaStoreState> | ((state: KafkaStoreState) => KafkaStoreState | Partial<KafkaStoreState>)) => void
) {
  const assetId = getKafkaAssetId(tabId);
  if (!assetId) return;
  get().ensureTab(tabId);
  const cluster = get().states[tabId]?.selectedConnectCluster || "";
  set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: true } } }));
  try {
    await fn(assetId, cluster);
    set((s) => ({ states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: null } } }));
    await get().loadConnectorDetail(tabId, name);
  } catch (err) {
    set((s) => ({
      states: { ...s.states, [tabId]: { ...s.states[tabId], connectAdminLoading: false, error: String(err) } },
    }));
    throw err;
  }
}

function parseOptionalInteger(value: string, field: string): number | undefined {
  const text = value.trim();
  if (!text) return undefined;
  return parseRequiredInteger(text, field);
}

function parseRequiredInteger(value: string, field: string): number {
  const n = Number(value.trim());
  if (!Number.isInteger(n)) {
    throw new Error(`${field} must be an integer`);
  }
  return n;
}

function parseHeaders(value: string): { key: string; value?: string; encoding?: KafkaPayloadEncoding }[] {
  const text = value.trim();
  if (!text) return [];
  const parsed = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    throw new Error("headers must be a JSON array");
  }
  return parsed;
}

registerTabCloseHook((tab) => {
  if (tab.type !== "query") return;
  const meta = tab.meta as QueryTabMeta;
  if (meta.assetType !== "kafka") return;
  useKafkaStore.setState((s) => {
    const states = { ...s.states };
    delete states[tab.id];
    return { states };
  });
});
