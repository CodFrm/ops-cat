import { create } from "zustand";
import {
  KafkaClusterOverview,
  KafkaGetConsumerGroup,
  KafkaGetTopic,
  KafkaListBrokers,
  KafkaListConsumerGroups,
  KafkaListTopics,
} from "../../wailsjs/go/app/App";
import { registerTabCloseHook, type QueryTabMeta } from "./tabStore";
import { useTabStore } from "./tabStore";

export type KafkaView = "overview" | "brokers" | "topics" | "consumerGroups";

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

export interface KafkaTabState {
  activeView: KafkaView;
  overview?: KafkaClusterOverviewInfo;
  brokers: KafkaBroker[];
  topics: KafkaTopicSummary[];
  topicsTotal: number;
  topicSearch: string;
  includeInternal: boolean;
  selectedTopic?: string;
  topicDetail?: KafkaTopicDetail;
  consumerGroups: KafkaConsumerGroup[];
  selectedGroup?: string;
  groupDetail?: KafkaConsumerGroupDetail;
  loadingOverview: boolean;
  loadingBrokers: boolean;
  loadingTopics: boolean;
  loadingTopicDetail: boolean;
  loadingGroups: boolean;
  loadingGroupDetail: boolean;
  error: string | null;
}

interface KafkaStoreState {
  states: Record<string, KafkaTabState>;
  ensureTab: (tabId: string) => void;
  setActiveView: (tabId: string, view: KafkaView) => void;
  setTopicSearch: (tabId: string, value: string) => void;
  setIncludeInternal: (tabId: string, value: boolean) => void;
  loadOverview: (tabId: string) => Promise<void>;
  loadBrokers: (tabId: string) => Promise<void>;
  loadTopics: (tabId: string) => Promise<void>;
  loadTopicDetail: (tabId: string, topic: string) => Promise<void>;
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
    loadingOverview: false,
    loadingBrokers: false,
    loadingTopics: false,
    loadingTopicDetail: false,
    loadingGroups: false,
    loadingGroupDetail: false,
    error: null,
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
        [tabId]: { ...s.states[tabId], selectedTopic: topic, topicDetail: undefined, loadingTopicDetail: true },
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
    } else {
      await get().loadConsumerGroups(tabId);
    }
  },
}));

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
