import { create } from "zustand";
import {
  ExecuteSQL,
  RedisGetKeyDetail,
  RedisListDatabases,
  RedisScanKeys,
  ListMongoDatabases,
  ListMongoCollections,
} from "../../wailsjs/go/app/App";
import { asset_entity } from "../../wailsjs/go/models";
import { useTabStore, registerTabCloseHook, registerTabRestoreHook, type QueryTabMeta } from "./tabStore";
import { useAssetStore } from "./assetStore";

// --- Types ---

export interface QueryTab {
  id: string; // "query:{assetId}"
  assetId: number;
  assetName: string;
  assetIcon: string;
  assetType: "database" | "redis" | "mongodb" | "kafka";
  driver?: string; // "mysql" | "postgresql"
  defaultDatabase?: string;
  redisDatabase?: number;
  redisScanPageSize?: number;
  redisKeySeparator?: string;
}

export type InnerTab =
  | { id: string; type: "table"; database: string; table: string; pendingLoad?: boolean }
  | {
      id: string;
      type: "sql";
      title: string;
      sql?: string;
      selectedDb?: string;
      editorHeight?: number;
      history?: string[];
    };

export interface DatabaseTabState {
  databases: string[];
  tables: Record<string, string[]>; // db -> table[]
  loadingTables: Record<string, boolean>; // db -> isLoading
  expandedDbs: string[];
  loadingDbs: boolean;
  innerTabs: InnerTab[];
  activeInnerTabId: string | null;
  error: string | null;
}

const REDIS_PAGE_SIZE = 100;

export interface RedisKeyInfo {
  type: string;
  ttl: number;
  size?: number;
  value: unknown;
  total: number; // LLEN/HLEN/SCARD/ZCARD, -1 for string
  valueCursor: string; // HSCAN/SSCAN cursor
  valueOffset: number; // LRANGE/ZRANGE next offset
  hasMoreValues: boolean;
  loadingMore: boolean;
}

export interface RedisTabState {
  currentDb: number;
  scanCursor: string;
  keys: string[];
  keyFilter: string;
  selectedKey: string | null;
  keyInfo: RedisKeyInfo | null;
  loadingKeys: boolean;
  hasMore: boolean;
  dbKeyCounts: Record<number, number>;
  scanRequestId?: number;
  keyDetailRequestId?: number;
  removedKey?: string;
  removedKeySeq?: number;
  error: string | null;
}

export type MongoInnerTab =
  | { id: string; type: "collection"; database: string; collection: string; pendingLoad?: boolean }
  | {
      id: string;
      type: "query";
      title: string;
      database?: string;
      collection?: string;
      operation?: string;
      queryText?: string;
      editorHeight?: number;
    };

export interface MongoDBTabState {
  databases: string[];
  collections: Record<string, string[]>;
  expandedDbs: string[];
  activeDatabase: string | null;
  innerTabs: MongoInnerTab[];
  activeInnerTabId: string | null;
  error: string | null;
}

interface QueryState {
  dbStates: Record<string, DatabaseTabState>;
  redisStates: Record<string, RedisTabState>;
  mongoStates: Record<string, MongoDBTabState>;

  openQueryTab: (asset: asset_entity.Asset, opts?: { initialSQL?: string; initialMongo?: string }) => void;

  // Database actions
  loadDatabases: (tabId: string) => Promise<void>;
  loadTables: (tabId: string, database: string) => Promise<void>;
  refreshTables: (tabId: string, database: string) => Promise<void>;
  toggleDbExpand: (tabId: string, database: string) => void;
  openTableTab: (tabId: string, database: string, table: string) => void;
  openSqlTab: (tabId: string, database?: string, sql?: string) => void;
  closeInnerTab: (tabId: string, innerTabId: string) => void;
  setActiveInnerTab: (tabId: string, innerTabId: string) => void;
  updateInnerTab: (tabId: string, innerTabId: string, patch: Record<string, unknown>) => void;
  markTableTabLoaded: (tabId: string, innerTabId: string) => void;
  addSqlHistory: (tabId: string, innerTabId: string, sql: string) => void;

  // Redis actions
  scanKeys: (tabId: string, reset?: boolean) => Promise<void>;
  selectRedisDb: (tabId: string, db: number) => Promise<void>;
  selectKey: (tabId: string, key: string) => Promise<void>;
  loadMoreValues: (tabId: string) => Promise<void>;
  setKeyFilter: (tabId: string, pattern: string) => void;
  loadDbKeyCounts: (tabId: string) => Promise<void>;
  clearSelectedKey: (tabId: string, key?: string) => void;
  removeKey: (tabId: string, key: string) => void;

  // MongoDB actions
  loadMongoDatabases: (tabId: string) => Promise<void>;
  loadMongoCollections: (tabId: string, database: string) => Promise<void>;
  toggleMongoDbExpand: (tabId: string, database: string) => void;
  openCollectionTab: (tabId: string, database: string, collection: string) => void;
  openMongoQueryTab: (tabId: string, database?: string, collection?: string) => void;
  closeMongoInnerTab: (tabId: string, innerTabId: string) => void;
  setActiveMongoInnerTab: (tabId: string, innerTabId: string) => void;
  updateMongoInnerTab: (tabId: string, innerTabId: string, patch: Record<string, unknown>) => void;
  markMongoCollectionTabLoaded: (tabId: string, innerTabId: string) => void;
}

// --- Helpers ---

function makeTabId(assetId: number) {
  return `query-${assetId}`;
}

function defaultDbState(): DatabaseTabState {
  return {
    databases: [],
    tables: {},
    loadingTables: {},
    expandedDbs: [],
    loadingDbs: false,
    innerTabs: [],
    activeInnerTabId: null,
    error: null,
  };
}

function defaultRedisState(options: { database?: number } = {}): RedisTabState {
  return {
    currentDb: Math.max(0, options.database || 0),
    scanCursor: "0",
    keys: [],
    keyFilter: "*",
    selectedKey: null,
    keyInfo: null,
    loadingKeys: false,
    hasMore: true,
    dbKeyCounts: {},
    scanRequestId: 0,
    keyDetailRequestId: 0,
    error: null,
  };
}

function defaultMongoState(): MongoDBTabState {
  return {
    databases: [],
    collections: {},
    expandedDbs: [],
    activeDatabase: null,
    innerTabs: [],
    activeInnerTabId: null,
    error: null,
  };
}

function toRedisMatchPattern(pattern: string): string {
  const trimmed = pattern.trim();
  if (!trimmed) return "*";
  if (trimmed.includes("*") || trimmed.includes("?") || trimmed.includes("[")) return trimmed;
  return `*${trimmed}*`;
}

export interface RedisStreamEntry {
  id: string;
  fields: Record<string, string>;
}

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  affected_rows?: number;
}

interface RedisKeyDetailResult {
  key: string;
  type: string;
  ttl: number;
  size: number;
  total: number;
  value: unknown;
  valueCursor: string;
  valueOffset: number;
  hasMoreValues: boolean;
}

function normalizeRedisDetailValue(type: string, value: unknown): unknown {
  if (type === "hash" && Array.isArray(value)) {
    return value.map((entry) => {
      if (Array.isArray(entry)) return [String(entry[0] ?? ""), String(entry[1] ?? "")] as [string, string];
      const obj = entry as { field?: unknown; value?: unknown };
      return [String(obj.field ?? ""), String(obj.value ?? "")] as [string, string];
    });
  }
  if (type === "zset" && Array.isArray(value)) {
    return value.map((entry) => {
      if (Array.isArray(entry)) return [String(entry[0] ?? ""), String(entry[1] ?? "0")] as [string, string];
      const obj = entry as { member?: unknown; score?: unknown };
      return [String(obj.member ?? ""), String(obj.score ?? "0")] as [string, string];
    });
  }
  if (type === "stream" && Array.isArray(value)) {
    return value.map((entry) => {
      const obj = entry as { id?: unknown; fields?: Record<string, string> };
      return { id: String(obj.id ?? ""), fields: obj.fields || {} } as RedisStreamEntry;
    });
  }
  return value;
}

function toRedisKeyInfo(detail: RedisKeyDetailResult): RedisKeyInfo {
  return {
    type: detail.type,
    ttl: detail.ttl,
    size: detail.size,
    value: normalizeRedisDetailValue(detail.type, detail.value),
    total: detail.total,
    valueCursor: detail.valueCursor,
    valueOffset: detail.valueOffset,
    hasMoreValues: detail.hasMoreValues,
    loadingMore: false,
  };
}

// --- Store ---

/** Returns the set of asset IDs that have an open query tab. */
export function getQueryActiveAssetIds(): Set<number> {
  const tabs = useTabStore.getState().tabs;
  const ids = new Set<number>();
  for (const tab of tabs) {
    if (tab.type !== "query") continue;
    ids.add((tab.meta as QueryTabMeta).assetId);
  }
  return ids;
}

// Helper: get query tab info from tabStore
function getQueryTabFromTabStore(tabId: string): QueryTab | undefined {
  const tab = useTabStore.getState().tabs.find((t) => t.id === tabId);
  if (!tab || tab.type !== "query") return undefined;
  const m = tab.meta as import("./tabStore").QueryTabMeta;
  return {
    id: tab.id,
    assetId: m.assetId,
    assetName: m.assetName,
    assetIcon: m.assetIcon,
    assetType: m.assetType,
    driver: m.driver,
    defaultDatabase: m.defaultDatabase,
    redisDatabase: m.redisDatabase,
    redisScanPageSize: m.redisScanPageSize,
    redisKeySeparator: m.redisKeySeparator,
  };
}

export const useQueryStore = create<QueryState>((set, get) => ({
  dbStates: {},
  redisStates: {},
  mongoStates: {},

  openQueryTab: (asset, opts) => {
    const tabId = makeTabId(asset.ID);
    const tabStore = useTabStore.getState();

    // If already open, activate and optionally inject initial content
    if (tabStore.tabs.some((t) => t.id === tabId)) {
      tabStore.activateTab(tabId);
      if (asset.Type === "database" && opts?.initialSQL) {
        get().openSqlTab(tabId, undefined, opts.initialSQL);
      } else if (asset.Type === "mongodb" && opts?.initialMongo) {
        get().openMongoQueryTab(tabId);
        const mongoState = get().mongoStates[tabId];
        const innerId = mongoState?.activeInnerTabId;
        if (innerId) {
          get().updateMongoInnerTab(tabId, innerId, { queryText: opts.initialMongo });
        }
      }
      return;
    }

    let driver: string | undefined;
    let defaultDatabase: string | undefined;
    let redisDatabase: number | undefined;
    let redisScanPageSize: number | undefined;
    let redisKeySeparator: string | undefined;
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      driver = cfg.driver;
      if (asset.Type === "redis") {
        redisDatabase = Math.max(0, Number(cfg.database) || 0);
        redisScanPageSize = Math.max(0, Number(cfg.scan_page_size) || 0) || undefined;
        redisKeySeparator = typeof cfg.key_separator === "string" ? cfg.key_separator : undefined;
      } else {
        defaultDatabase = cfg.database;
      }
    } catch {
      /* ignore */
    }

    const assetPath = useAssetStore.getState().getAssetPath(asset);
    tabStore.openTab({
      id: tabId,
      type: "query",
      label: assetPath,
      icon: asset.Icon || undefined,
      meta: {
        type: "query",
        assetId: asset.ID,
        assetName: asset.Name,
        assetIcon: asset.Icon || "",
        assetType: asset.Type as "database" | "redis" | "mongodb" | "kafka",
        driver,
        defaultDatabase,
        redisDatabase,
        redisScanPageSize,
        redisKeySeparator,
      },
    });

    if (asset.Type === "database") {
      set((s) => ({
        dbStates: { ...s.dbStates, [tabId]: defaultDbState() },
      }));
      if (opts?.initialSQL) {
        get().openSqlTab(tabId, undefined, opts.initialSQL);
      }
    } else if (asset.Type === "mongodb") {
      set((s) => ({
        mongoStates: { ...s.mongoStates, [tabId]: defaultMongoState() },
      }));
      if (opts?.initialMongo) {
        get().openMongoQueryTab(tabId);
        const mongoState = get().mongoStates[tabId];
        const innerId = mongoState?.activeInnerTabId;
        if (innerId) {
          get().updateMongoInnerTab(tabId, innerId, { queryText: opts.initialMongo });
        }
      }
    } else if (asset.Type === "redis") {
      set((s) => ({
        redisStates: { ...s.redisStates, [tabId]: defaultRedisState({ database: redisDatabase }) },
      }));
    }
  },

  // --- Database ---

  loadDatabases: async (tabId) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: { ...s.dbStates[tabId], loadingDbs: true },
      },
    }));

    try {
      const sql =
        tab.driver === "postgresql"
          ? "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"
          : "SHOW DATABASES";
      const result = await ExecuteSQL(tab.assetId, sql, "");
      const parsed: SQLResult = JSON.parse(result);
      const databases = (parsed.rows || [])
        .map((r) => {
          const vals = Object.values(r);
          return String(vals[0] || "");
        })
        .filter(Boolean);

      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: { ...s.dbStates[tabId], databases, loadingDbs: false, error: null },
        },
      }));

      // Also refresh tables for already-expanded databases that still exist,
      // otherwise the top-level refresh button leaves stale table lists.
      const expanded = get().dbStates[tabId]?.expandedDbs ?? [];
      await Promise.all(expanded.filter((db) => databases.includes(db)).map((db) => get().loadTables(tabId, db)));
    } catch (err) {
      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: { ...s.dbStates[tabId], loadingDbs: false, error: String(err) },
        },
      }));
    }
  },

  loadTables: async (tabId, database) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...s.dbStates[tabId],
          loadingTables: { ...s.dbStates[tabId].loadingTables, [database]: true },
        },
      },
    }));

    try {
      const sql =
        tab.driver === "postgresql"
          ? `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`
          : `SHOW TABLES FROM \`${database}\``;
      const result = await ExecuteSQL(tab.assetId, sql, database);
      const parsed: SQLResult = JSON.parse(result);
      const tables = (parsed.rows || [])
        .map((r) => {
          const vals = Object.values(r);
          return String(vals[0] || "");
        })
        .filter(Boolean);

      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: {
            ...s.dbStates[tabId],
            tables: { ...s.dbStates[tabId].tables, [database]: tables },
            loadingTables: { ...s.dbStates[tabId].loadingTables, [database]: false },
          },
        },
      }));
    } catch (err) {
      // Reset table list to [] (not undefined) so the UI exits the loading
      // state instead of showing an infinite spinner.
      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: {
            ...s.dbStates[tabId],
            tables: { ...s.dbStates[tabId].tables, [database]: [] },
            loadingTables: { ...s.dbStates[tabId].loadingTables, [database]: false },
            error: s.dbStates[tabId]?.error || String(err),
          },
        },
      }));
    }
  },

  refreshTables: async (tabId, database) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    await get().loadTables(tabId, database);
  },

  toggleDbExpand: (tabId, database) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const isExpanded = state.expandedDbs.includes(database);
    const expanded = isExpanded ? state.expandedDbs.filter((d) => d !== database) : [...state.expandedDbs, database];
    if (!isExpanded && !state.tables[database]) {
      // Load tables if not loaded
      get().loadTables(tabId, database);
    }
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: { ...s.dbStates[tabId], expandedDbs: expanded },
      },
    }));
  },

  openTableTab: (tabId, database, table) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const innerId = `table:${database}.${table}`;
    if (state.innerTabs.some((t) => t.id === innerId)) {
      set((s) => ({
        dbStates: { ...s.dbStates, [tabId]: { ...state, activeInnerTabId: innerId } },
      }));
      return;
    }
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: [...state.innerTabs, { id: innerId, type: "table", database, table }],
          activeInnerTabId: innerId,
        },
      },
    }));
  },

  openSqlTab: (tabId, database?, sql?) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const count = state.innerTabs.filter((t) => t.type === "sql").length + 1;
    const innerId = `sql:${Date.now()}`;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: [
            ...state.innerTabs,
            { id: innerId, type: "sql", title: `SQL ${count}`, sql, selectedDb: database },
          ],
          activeInnerTabId: innerId,
        },
      },
    }));
  },

  closeInnerTab: (tabId, innerTabId) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const idx = state.innerTabs.findIndex((t) => t.id === innerTabId);
    const newTabs = state.innerTabs.filter((t) => t.id !== innerTabId);
    let newActive = state.activeInnerTabId;
    if (newActive === innerTabId) {
      const neighbor = state.innerTabs[idx + 1] || state.innerTabs[idx - 1];
      newActive = neighbor?.id || null;
    }
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: { ...state, innerTabs: newTabs, activeInnerTabId: newActive },
      },
    }));
  },

  setActiveInnerTab: (tabId, innerTabId) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: { ...state, activeInnerTabId: innerTabId },
      },
    }));
  },

  updateInnerTab: (tabId, innerTabId, patch) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: state.innerTabs.map((t) => (t.id === innerTabId ? ({ ...t, ...patch } as InnerTab) : t)),
        },
      },
    }));
  },

  markTableTabLoaded: (tabId, innerTabId) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: state.innerTabs.map((t) =>
            t.id === innerTabId && t.type === "table" ? { ...t, pendingLoad: false } : t
          ),
        },
      },
    }));
  },

  addSqlHistory: (tabId, innerTabId, sql) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const trimmed = sql.trim();
    if (!trimmed) return;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: state.innerTabs.map((t) => {
            if (t.id !== innerTabId || t.type !== "sql") return t;
            const prev = t.history || [];
            const next = [trimmed, ...prev.filter((h) => h !== trimmed)].slice(0, 30);
            return { ...t, history: next };
          }),
        },
      },
    }));
  },

  // --- Redis ---

  scanKeys: async (tabId, reset) => {
    const tab = getQueryTabFromTabStore(tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state) return;
    if (!reset && state.loadingKeys) return;

    const cursor = reset ? "0" : state.scanCursor;
    if (!reset && cursor === "0" && state.keys.length > 0) return;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...state, loadingKeys: true, scanRequestId: (state.scanRequestId || 0) + 1 },
      },
    }));
    const requestId = (state.scanRequestId || 0) + 1;

    try {
      const result = await RedisScanKeys({
        assetId: tab.assetId,
        db: state.currentDb,
        cursor,
        match: toRedisMatchPattern(state.keyFilter || "*"),
        type: "",
        count: tab.redisScanPageSize || 200,
        exact: false,
      });

      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]:
            s.redisStates[tabId]?.scanRequestId === requestId
              ? {
                  ...s.redisStates[tabId],
                  scanCursor: result.cursor || "0",
                  keys: reset ? result.keys || [] : [...s.redisStates[tabId].keys, ...(result.keys || [])],
                  hasMore: !!result.hasMore,
                  loadingKeys: false,
                  error: null,
                }
              : s.redisStates[tabId],
        },
      }));
    } catch (err) {
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]:
            s.redisStates[tabId]?.scanRequestId === requestId
              ? { ...s.redisStates[tabId], loadingKeys: false, error: String(err) }
              : s.redisStates[tabId],
        },
      }));
    }
  },

  selectRedisDb: async (tabId, db) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    const prev = get().redisStates[tabId];
    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: {
          ...defaultRedisState(),
          currentDb: db,
          keyFilter: prev?.keyFilter || "*",
          dbKeyCounts: prev?.dbKeyCounts || {},
        },
      },
    }));

    get().scanKeys(tabId, true);
  },

  selectKey: async (tabId, key) => {
    const tab = getQueryTabFromTabStore(tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state) return;
    const db = state.currentDb;
    const requestId = (state.keyDetailRequestId || 0) + 1;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...s.redisStates[tabId], selectedKey: key, keyInfo: null, keyDetailRequestId: requestId },
      },
    }));

    try {
      const detail = await RedisGetKeyDetail({
        assetId: tab.assetId,
        db,
        key,
        cursor: "",
        offset: 0,
        count: REDIS_PAGE_SIZE,
      });

      set((s) => {
        const current = s.redisStates[tabId];
        if (
          !current ||
          current.keyDetailRequestId !== requestId ||
          current.selectedKey !== key ||
          current.currentDb !== db
        ) {
          return { redisStates: s.redisStates };
        }
        return {
          redisStates: {
            ...s.redisStates,
            [tabId]: {
              ...current,
              keyInfo: toRedisKeyInfo(detail as RedisKeyDetailResult),
            },
          },
        };
      });
    } catch {
      /* ignore */
    }
  },

  loadMoreValues: async (tabId) => {
    const tab = getQueryTabFromTabStore(tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state?.keyInfo || !state.selectedKey || !state.keyInfo.hasMoreValues || state.keyInfo.loadingMore)
      return;

    const key = state.selectedKey;
    const info = state.keyInfo;
    const db = state.currentDb;
    const requestId = (state.keyDetailRequestId || 0) + 1;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: {
          ...s.redisStates[tabId],
          keyDetailRequestId: requestId,
          keyInfo: { ...info, loadingMore: true },
        },
      },
    }));

    try {
      let newValue: unknown = info.value;
      let newCursor = info.valueCursor;
      let newOffset = info.valueOffset;
      let newHasMore = false;

      const detail = await RedisGetKeyDetail({
        assetId: tab.assetId,
        db,
        key,
        cursor: info.valueCursor,
        offset: info.valueOffset,
        count: REDIS_PAGE_SIZE,
      });
      const next = toRedisKeyInfo(detail as RedisKeyDetailResult);
      newValue =
        info.type === "hash" || info.type === "zset"
          ? [...(info.value as [string, string][]), ...(next.value as [string, string][])]
          : info.type === "stream"
            ? [...(info.value as RedisStreamEntry[]), ...(next.value as RedisStreamEntry[])]
            : [...(info.value as string[]), ...((next.value as string[]) || [])];
      newCursor = next.valueCursor;
      newOffset = next.valueOffset;
      newHasMore = next.hasMoreValues;

      set((s) => {
        const current = s.redisStates[tabId];
        if (
          !current ||
          current.keyDetailRequestId !== requestId ||
          current.selectedKey !== key ||
          current.currentDb !== db
        ) {
          return { redisStates: s.redisStates };
        }
        return {
          redisStates: {
            ...s.redisStates,
            [tabId]: {
              ...current,
              keyInfo: {
                ...info,
                value: newValue,
                valueCursor: newCursor,
                valueOffset: newOffset,
                hasMoreValues: newHasMore,
                loadingMore: false,
              },
            },
          },
        };
      });
    } catch {
      set((s) => {
        const current = s.redisStates[tabId];
        if (
          !current ||
          current.keyDetailRequestId !== requestId ||
          current.selectedKey !== key ||
          current.currentDb !== db
        ) {
          return { redisStates: s.redisStates };
        }
        return {
          redisStates: {
            ...s.redisStates,
            [tabId]: { ...current, keyInfo: { ...info, loadingMore: false } },
          },
        };
      });
    }
  },

  setKeyFilter: (tabId, pattern) => {
    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...s.redisStates[tabId], keyFilter: pattern || "*" },
      },
    }));
  },

  loadDbKeyCounts: async (tabId) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    try {
      const databases = await RedisListDatabases(tab.assetId);
      const counts: Record<number, number> = {};
      for (const db of databases || []) {
        counts[db.db] = db.keys;
      }
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: { ...s.redisStates[tabId], dbKeyCounts: counts },
        },
      }));
    } catch (err) {
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: { ...s.redisStates[tabId], error: s.redisStates[tabId]?.error || String(err) },
        },
      }));
    }
  },

  clearSelectedKey: (tabId, key) => {
    const state = get().redisStates[tabId];
    if (!state || (key && state.selectedKey !== key)) return;
    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: {
          ...s.redisStates[tabId],
          selectedKey: null,
          keyInfo: null,
          keyDetailRequestId: (s.redisStates[tabId].keyDetailRequestId || 0) + 1,
        },
      },
    }));
  },

  removeKey: (tabId, key) => {
    const state = get().redisStates[tabId];
    if (!state) return;
    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: {
          ...s.redisStates[tabId],
          keys: s.redisStates[tabId].keys.filter((k) => k !== key),
          selectedKey: s.redisStates[tabId].selectedKey === key ? null : s.redisStates[tabId].selectedKey,
          keyInfo: s.redisStates[tabId].selectedKey === key ? null : s.redisStates[tabId].keyInfo,
          keyDetailRequestId:
            s.redisStates[tabId].selectedKey === key
              ? (s.redisStates[tabId].keyDetailRequestId || 0) + 1
              : s.redisStates[tabId].keyDetailRequestId,
          removedKey: key,
          removedKeySeq: (s.redisStates[tabId].removedKeySeq || 0) + 1,
        },
      },
    }));
  },

  // --- MongoDB ---

  loadMongoDatabases: async (tabId) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    try {
      const result = await ListMongoDatabases(tab.assetId);
      const databases: string[] = JSON.parse(result);
      set((s) => ({
        mongoStates: {
          ...s.mongoStates,
          [tabId]: { ...s.mongoStates[tabId], databases, error: null },
        },
      }));
    } catch (err) {
      set((s) => ({
        mongoStates: {
          ...s.mongoStates,
          [tabId]: { ...s.mongoStates[tabId], error: s.mongoStates[tabId]?.error || String(err) },
        },
      }));
    }
  },

  loadMongoCollections: async (tabId, database) => {
    const tab = getQueryTabFromTabStore(tabId);
    if (!tab) return;

    try {
      const result = await ListMongoCollections(tab.assetId, database);
      const collections: string[] = JSON.parse(result);
      set((s) => ({
        mongoStates: {
          ...s.mongoStates,
          [tabId]: {
            ...s.mongoStates[tabId],
            collections: { ...s.mongoStates[tabId].collections, [database]: collections },
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        mongoStates: {
          ...s.mongoStates,
          [tabId]: { ...s.mongoStates[tabId], error: s.mongoStates[tabId]?.error || String(err) },
        },
      }));
    }
  },

  toggleMongoDbExpand: (tabId, database) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    const isExpanded = state.expandedDbs.includes(database);
    const expanded = isExpanded ? state.expandedDbs.filter((d) => d !== database) : [...state.expandedDbs, database];
    if (!isExpanded && !state.collections[database]) {
      get().loadMongoCollections(tabId, database);
    }
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: {
          ...s.mongoStates[tabId],
          expandedDbs: expanded,
          // 展开某个库时把它当作"当前库"，方便新开 Query Tab 时继承
          activeDatabase: !isExpanded ? database : s.mongoStates[tabId].activeDatabase,
        },
      },
    }));
  },

  openCollectionTab: (tabId, database, collection) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    const innerId = `collection:${database}.${collection}`;
    if (state.innerTabs.some((t) => t.id === innerId)) {
      set((s) => ({
        mongoStates: {
          ...s.mongoStates,
          [tabId]: { ...s.mongoStates[tabId], activeInnerTabId: innerId },
        },
      }));
      return;
    }
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: {
          ...s.mongoStates[tabId],
          innerTabs: [...state.innerTabs, { id: innerId, type: "collection", database, collection }],
          activeInnerTabId: innerId,
        },
      },
    }));
  },

  openMongoQueryTab: (tabId, database?, collection?) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    const count = state.innerTabs.filter((t) => t.type === "query").length + 1;
    const innerId = `mongo-query:${Date.now()}`;
    const resolvedDb = database ?? state.activeDatabase ?? undefined;
    const queryText = collection ? `db.${collection}.find({})` : "";
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: {
          ...s.mongoStates[tabId],
          activeDatabase: resolvedDb ?? s.mongoStates[tabId].activeDatabase,
          innerTabs: [
            ...state.innerTabs,
            {
              id: innerId,
              type: "query",
              title: `Query ${count}`,
              database: resolvedDb,
              collection,
              queryText,
            },
          ],
          activeInnerTabId: innerId,
        },
      },
    }));
  },

  closeMongoInnerTab: (tabId, innerTabId) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    const idx = state.innerTabs.findIndex((t) => t.id === innerTabId);
    const newTabs = state.innerTabs.filter((t) => t.id !== innerTabId);
    let newActive = state.activeInnerTabId;
    if (newActive === innerTabId) {
      const neighbor = state.innerTabs[idx + 1] || state.innerTabs[idx - 1];
      newActive = neighbor?.id || null;
    }
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: { ...s.mongoStates[tabId], innerTabs: newTabs, activeInnerTabId: newActive },
      },
    }));
  },

  setActiveMongoInnerTab: (tabId, innerTabId) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: { ...s.mongoStates[tabId], activeInnerTabId: innerTabId },
      },
    }));
  },

  updateMongoInnerTab: (tabId, innerTabId, patch) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: {
          ...state,
          innerTabs: state.innerTabs.map((t) => (t.id === innerTabId ? ({ ...t, ...patch } as MongoInnerTab) : t)),
        },
      },
    }));
  },

  markMongoCollectionTabLoaded: (tabId, innerTabId) => {
    const state = get().mongoStates[tabId];
    if (!state) return;
    set((s) => ({
      mongoStates: {
        ...s.mongoStates,
        [tabId]: {
          ...state,
          innerTabs: state.innerTabs.map((t) =>
            t.id === innerTabId && t.type === "collection" ? { ...t, pendingLoad: false } : t
          ),
        },
      },
    }));
  },
}));

// === Persistence ===
//
// Caches sidebar metadata (database / table / collection lists, expanded
// state, inner tabs, sql history, editor height) so the sidebar is ready
// immediately on reload. Query results are NOT cached; table / collection
// inner tabs are restored with pendingLoad = true so the user must click
// to re-fetch the current page — avoiding a burst of queries on startup.

const QUERY_STORE_KEY = "query_store_v1";

interface PersistedDbState {
  databases: string[];
  tables: Record<string, string[]>;
  expandedDbs: string[];
  innerTabs: InnerTab[];
  activeInnerTabId: string | null;
}

interface PersistedMongoState {
  databases: string[];
  collections: Record<string, string[]>;
  expandedDbs: string[];
  innerTabs: MongoInnerTab[];
  activeInnerTabId: string | null;
}

interface PersistedQueryStore {
  dbStates: Record<string, PersistedDbState>;
  mongoStates: Record<string, PersistedMongoState>;
}

function stripDbState(s: DatabaseTabState): PersistedDbState {
  return {
    databases: s.databases,
    tables: s.tables,
    expandedDbs: s.expandedDbs,
    innerTabs: s.innerTabs,
    activeInnerTabId: s.activeInnerTabId,
  };
}

function stripMongoState(s: MongoDBTabState): PersistedMongoState {
  return {
    databases: s.databases,
    collections: s.collections,
    expandedDbs: s.expandedDbs,
    innerTabs: s.innerTabs,
    activeInnerTabId: s.activeInnerTabId,
  };
}

function loadPersistedQueryStore(): PersistedQueryStore {
  try {
    const raw = localStorage.getItem(QUERY_STORE_KEY);
    if (!raw) return { dbStates: {}, mongoStates: {} };
    const parsed = JSON.parse(raw) as Partial<PersistedQueryStore>;
    return {
      dbStates: parsed.dbStates || {},
      mongoStates: parsed.mongoStates || {},
    };
  } catch {
    return { dbStates: {}, mongoStates: {} };
  }
}

function savePersistedQueryStore() {
  const state = useQueryStore.getState();
  const data: PersistedQueryStore = {
    dbStates: {},
    mongoStates: {},
  };
  for (const [tabId, s] of Object.entries(state.dbStates)) {
    data.dbStates[tabId] = stripDbState(s);
  }
  for (const [tabId, s] of Object.entries(state.mongoStates)) {
    data.mongoStates[tabId] = stripMongoState(s);
  }
  try {
    localStorage.setItem(QUERY_STORE_KEY, JSON.stringify(data));
  } catch {
    /* storage full — ignore */
  }
}

let _persistReady = false;
let _saveTimer: ReturnType<typeof setTimeout> | null = null;

useQueryStore.subscribe((state, prevState) => {
  if (!_persistReady) return;
  if (state.dbStates === prevState.dbStates && state.mongoStates === prevState.mongoStates) return;
  // Debounce: SQL editor / MongoDB query editor writes on every keystroke.
  if (_saveTimer) clearTimeout(_saveTimer);
  _saveTimer = setTimeout(() => {
    _saveTimer = null;
    savePersistedQueryStore();
  }, 300);
});

// === Close Hook: clean up when tabStore closes a query tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "query") return;
  useQueryStore.setState((s) => {
    const newDbStates = { ...s.dbStates };
    delete newDbStates[tab.id];
    const newRedisStates = { ...s.redisStates };
    delete newRedisStates[tab.id];
    const newMongoStates = { ...s.mongoStates };
    delete newMongoStates[tab.id];
    return { dbStates: newDbStates, redisStates: newRedisStates, mongoStates: newMongoStates };
  });
});

// === Restore Hook: initialize query tab states ===

registerTabRestoreHook("query", (tabs) => {
  const persisted = loadPersistedQueryStore();
  // Drop persisted entries whose tab is no longer open
  const openIds = new Set(tabs.map((t) => t.id));

  const dbStates: Record<string, DatabaseTabState> = {};
  const redisStates: Record<string, RedisTabState> = {};
  const mongoStates: Record<string, MongoDBTabState> = {};

  for (const tab of tabs) {
    const m = tab.meta as QueryTabMeta;
    if (m.assetType === "database") {
      const saved = persisted.dbStates[tab.id];
      if (saved) {
        dbStates[tab.id] = {
          ...defaultDbState(),
          databases: saved.databases || [],
          tables: saved.tables || {},
          expandedDbs: saved.expandedDbs || [],
          innerTabs: (saved.innerTabs || []).map((it) => (it.type === "table" ? { ...it, pendingLoad: true } : it)),
          activeInnerTabId: saved.activeInnerTabId ?? null,
        };
      } else {
        dbStates[tab.id] = defaultDbState();
      }
    } else if (m.assetType === "mongodb") {
      const saved = persisted.mongoStates[tab.id];
      if (saved) {
        mongoStates[tab.id] = {
          ...defaultMongoState(),
          databases: saved.databases || [],
          collections: saved.collections || {},
          expandedDbs: saved.expandedDbs || [],
          innerTabs: (saved.innerTabs || []).map((it) =>
            it.type === "collection" ? { ...it, pendingLoad: true } : it
          ),
          activeInnerTabId: saved.activeInnerTabId ?? null,
        };
      } else {
        mongoStates[tab.id] = defaultMongoState();
      }
    } else if (m.assetType === "redis") {
      redisStates[tab.id] = defaultRedisState({ database: m.redisDatabase });
    }
  }

  useQueryStore.setState({ dbStates, redisStates, mongoStates });
  _persistReady = true;

  // Drop stale persisted entries (tabs no longer open) by writing the current
  // trimmed state back.
  const hasStale =
    Object.keys(persisted.dbStates).some((id) => !openIds.has(id)) ||
    Object.keys(persisted.mongoStates).some((id) => !openIds.has(id));
  if (hasStale) savePersistedQueryStore();
});
