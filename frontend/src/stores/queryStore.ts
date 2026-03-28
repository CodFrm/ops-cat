import { create } from "zustand";
import { ExecuteSQL, ExecuteRedis, ExecuteRedisArgs } from "../../wailsjs/go/app/App";
import { asset_entity } from "../../wailsjs/go/models";
import { useTabStore, registerTabCloseHook, registerTabRestoreHook, type QueryTabMeta } from "./tabStore";
import { useAssetStore } from "./assetStore";

// --- Types ---

export interface QueryTab {
  id: string; // "query:{assetId}"
  assetId: number;
  assetName: string;
  assetIcon: string;
  assetType: string;
  driver?: string; // "mysql" | "postgresql"
  defaultDatabase?: string;
}

export type InnerTab =
  | { id: string; type: "table"; database: string; table: string }
  | { id: string; type: "sql"; title: string; sql?: string; selectedDb?: string };

export interface DatabaseTabState {
  databases: string[];
  tables: Record<string, string[]>; // db -> table[]
  expandedDbs: Set<string>;
  loadingDbs: boolean;
  innerTabs: InnerTab[];
  activeInnerTabId: string | null;
  error: string | null;
}

const REDIS_PAGE_SIZE = 100;

export interface RedisKeyInfo {
  type: string;
  ttl: number;
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
  error: string | null;
}

interface QueryState {
  dbStates: Record<string, DatabaseTabState>;
  redisStates: Record<string, RedisTabState>;

  openQueryTab: (asset: asset_entity.Asset) => void;

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

  // Redis actions
  scanKeys: (tabId: string, reset?: boolean) => Promise<void>;
  selectRedisDb: (tabId: string, db: number) => Promise<void>;
  selectKey: (tabId: string, key: string) => Promise<void>;
  loadMoreValues: (tabId: string) => Promise<void>;
  setKeyFilter: (tabId: string, pattern: string) => void;
  loadDbKeyCounts: (tabId: string) => Promise<void>;
  removeKey: (tabId: string, key: string) => void;
}

// --- Helpers ---

function makeTabId(assetId: number) {
  return `query-${assetId}`;
}

function defaultDbState(): DatabaseTabState {
  return {
    databases: [],
    tables: {},
    expandedDbs: new Set(),
    loadingDbs: false,
    innerTabs: [],
    activeInnerTabId: null,
    error: null,
  };
}

function defaultRedisState(): RedisTabState {
  return {
    currentDb: 0,
    scanCursor: "0",
    keys: [],
    keyFilter: "*",
    selectedKey: null,
    keyInfo: null,
    loadingKeys: false,
    hasMore: true,
    dbKeyCounts: {},
    error: null,
  };
}

interface SQLResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  count?: number;
  affected_rows?: number;
}

interface RedisResult {
  type: string;
  value: unknown;
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
  };
}

export const useQueryStore = create<QueryState>((set, get) => ({
  dbStates: {},
  redisStates: {},

  openQueryTab: (asset) => {
    const tabId = makeTabId(asset.ID);
    const tabStore = useTabStore.getState();

    // If already open, activate
    if (tabStore.tabs.some((t) => t.id === tabId)) {
      tabStore.activateTab(tabId);
      return;
    }

    let driver: string | undefined;
    let defaultDatabase: string | undefined;
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      driver = cfg.driver;
      defaultDatabase = cfg.database;
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
        assetType: asset.Type,
        driver,
        defaultDatabase,
      },
    });

    if (asset.Type === "database") {
      set((s) => ({
        dbStates: { ...s.dbStates, [tabId]: defaultDbState() },
      }));
    } else {
      set((s) => ({
        redisStates: { ...s.redisStates, [tabId]: defaultRedisState() },
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
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: { ...s.dbStates[tabId], error: s.dbStates[tabId]?.error || String(err) },
        },
      }));
    }
  },

  refreshTables: async (tabId, database) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    // Clear existing tables for this database and reload
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...s.dbStates[tabId],
          tables: { ...s.dbStates[tabId].tables, [database]: undefined as unknown as string[] },
        },
      },
    }));
    await get().loadTables(tabId, database);
  },

  toggleDbExpand: (tabId, database) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const expanded = new Set(state.expandedDbs);
    if (expanded.has(database)) {
      expanded.delete(database);
    } else {
      expanded.add(database);
      // Load tables if not loaded
      if (!state.tables[database]) {
        get().loadTables(tabId, database);
      }
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

  // --- Redis ---

  scanKeys: async (tabId, reset) => {
    const tab = getQueryTabFromTabStore(tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state) return;

    const cursor = reset ? "0" : state.scanCursor;
    if (!reset && cursor === "0" && state.keys.length > 0) return;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...state, loadingKeys: true },
      },
    }));

    try {
      const cmd = `SCAN ${cursor} MATCH ${state.keyFilter || "*"} COUNT 200`;
      const result = await ExecuteRedis(tab.assetId, cmd, state.currentDb);
      const parsed: RedisResult = JSON.parse(result);

      let newCursor = "0";
      let newKeys: string[] = [];
      if (parsed.type === "list" && Array.isArray(parsed.value)) {
        const arr = parsed.value as unknown[];
        newCursor = String(arr[0] || "0");
        if (Array.isArray(arr[1])) {
          newKeys = (arr[1] as unknown[]).map(String);
        }
      }

      const allKeys = reset ? newKeys : [...state.keys, ...newKeys];

      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: {
            ...s.redisStates[tabId],
            scanCursor: newCursor,
            keys: allKeys,
            hasMore: newCursor !== "0",
            loadingKeys: false,
            error: null,
          },
        },
      }));
    } catch (err) {
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: { ...s.redisStates[tabId], loadingKeys: false, error: String(err) },
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

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...s.redisStates[tabId], selectedKey: key, keyInfo: null },
      },
    }));

    try {
      const typeResult = await ExecuteRedis(tab.assetId, `TYPE ${key}`, db);
      const typeParsed: RedisResult = JSON.parse(typeResult);
      const keyType = String(typeParsed.value || "none");

      const ttlResult = await ExecuteRedis(tab.assetId, `TTL ${key}`, db);
      const ttlParsed: RedisResult = JSON.parse(ttlResult);
      const ttl = Number(ttlParsed.value) || -1;

      let value: unknown = null;
      let total = -1;
      let valueCursor = "";
      let valueOffset = 0;
      let hasMoreValues = false;

      switch (keyType) {
        case "string": {
          const r = await ExecuteRedisArgs(tab.assetId, ["GET", key], db);
          value = JSON.parse(r).value;
          break;
        }
        case "list": {
          const [countR, valR] = await Promise.all([
            ExecuteRedisArgs(tab.assetId, ["LLEN", key], db),
            ExecuteRedisArgs(tab.assetId, ["LRANGE", key, "0", String(REDIS_PAGE_SIZE - 1)], db),
          ]);
          total = Number(JSON.parse(countR).value) || 0;
          const items = (JSON.parse(valR).value as string[]) || [];
          value = items;
          valueOffset = items.length;
          hasMoreValues = valueOffset < total;
          break;
        }
        case "hash": {
          const [countR, scanR] = await Promise.all([
            ExecuteRedisArgs(tab.assetId, ["HLEN", key], db),
            ExecuteRedisArgs(tab.assetId, ["HSCAN", key, "0", "COUNT", String(REDIS_PAGE_SIZE)], db),
          ]);
          total = Number(JSON.parse(countR).value) || 0;
          const scanParsed = JSON.parse(scanR);
          if (scanParsed.type === "list" && Array.isArray(scanParsed.value)) {
            const arr = scanParsed.value as unknown[];
            valueCursor = String(arr[0] || "0");
            const flat = (arr[1] as string[]) || [];
            const entries: [string, string][] = [];
            for (let i = 0; i < flat.length; i += 2) {
              entries.push([flat[i], flat[i + 1] || ""]);
            }
            value = entries;
            hasMoreValues = valueCursor !== "0";
          }
          break;
        }
        case "set": {
          const [countR, scanR] = await Promise.all([
            ExecuteRedisArgs(tab.assetId, ["SCARD", key], db),
            ExecuteRedisArgs(tab.assetId, ["SSCAN", key, "0", "COUNT", String(REDIS_PAGE_SIZE)], db),
          ]);
          total = Number(JSON.parse(countR).value) || 0;
          const scanParsed = JSON.parse(scanR);
          if (scanParsed.type === "list" && Array.isArray(scanParsed.value)) {
            const arr = scanParsed.value as unknown[];
            valueCursor = String(arr[0] || "0");
            value = (arr[1] as string[]) || [];
            hasMoreValues = valueCursor !== "0";
          }
          break;
        }
        case "zset": {
          const [countR, valR] = await Promise.all([
            ExecuteRedisArgs(tab.assetId, ["ZCARD", key], db),
            ExecuteRedisArgs(tab.assetId, ["ZRANGE", key, "0", String(REDIS_PAGE_SIZE - 1), "WITHSCORES"], db),
          ]);
          total = Number(JSON.parse(countR).value) || 0;
          const raw = (JSON.parse(valR).value as string[]) || [];
          const pairs: [string, string][] = [];
          for (let i = 0; i < raw.length; i += 2) {
            pairs.push([raw[i], raw[i + 1] || "0"]);
          }
          value = pairs;
          valueOffset = pairs.length;
          hasMoreValues = valueOffset < total;
          break;
        }
      }

      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: {
            ...s.redisStates[tabId],
            keyInfo: {
              type: keyType,
              ttl,
              value,
              total,
              valueCursor,
              valueOffset,
              hasMoreValues,
              loadingMore: false,
            },
          },
        },
      }));
    } catch {
      /* ignore */
    }
  },

  loadMoreValues: async (tabId) => {
    const tab = getQueryTabFromTabStore(tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state?.keyInfo || !state.selectedKey || !state.keyInfo.hasMoreValues) return;

    const key = state.selectedKey;
    const info = state.keyInfo;
    const db = state.currentDb;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...s.redisStates[tabId], keyInfo: { ...info, loadingMore: true } },
      },
    }));

    try {
      let newValue: unknown = info.value;
      let newCursor = info.valueCursor;
      let newOffset = info.valueOffset;
      let newHasMore = false;

      switch (info.type) {
        case "list": {
          const r = await ExecuteRedisArgs(
            tab.assetId,
            ["LRANGE", key, String(newOffset), String(newOffset + REDIS_PAGE_SIZE - 1)],
            db
          );
          const items = (JSON.parse(r).value as string[]) || [];
          newValue = [...(info.value as string[]), ...items];
          newOffset = (newValue as string[]).length;
          newHasMore = newOffset < info.total;
          break;
        }
        case "hash": {
          const r = await ExecuteRedisArgs(
            tab.assetId,
            ["HSCAN", key, newCursor, "COUNT", String(REDIS_PAGE_SIZE)],
            db
          );
          const parsed = JSON.parse(r);
          if (parsed.type === "list" && Array.isArray(parsed.value)) {
            const arr = parsed.value as unknown[];
            newCursor = String(arr[0] || "0");
            const flat = (arr[1] as string[]) || [];
            const entries: [string, string][] = [];
            for (let i = 0; i < flat.length; i += 2) {
              entries.push([flat[i], flat[i + 1] || ""]);
            }
            newValue = [...(info.value as [string, string][]), ...entries];
            newHasMore = newCursor !== "0";
          }
          break;
        }
        case "set": {
          const r = await ExecuteRedisArgs(
            tab.assetId,
            ["SSCAN", key, newCursor, "COUNT", String(REDIS_PAGE_SIZE)],
            db
          );
          const parsed = JSON.parse(r);
          if (parsed.type === "list" && Array.isArray(parsed.value)) {
            const arr = parsed.value as unknown[];
            newCursor = String(arr[0] || "0");
            const items = (arr[1] as string[]) || [];
            newValue = [...(info.value as string[]), ...items];
            newHasMore = newCursor !== "0";
          }
          break;
        }
        case "zset": {
          const r = await ExecuteRedisArgs(
            tab.assetId,
            ["ZRANGE", key, String(newOffset), String(newOffset + REDIS_PAGE_SIZE - 1), "WITHSCORES"],
            db
          );
          const raw = (JSON.parse(r).value as string[]) || [];
          const pairs: [string, string][] = [];
          for (let i = 0; i < raw.length; i += 2) {
            pairs.push([raw[i], raw[i + 1] || "0"]);
          }
          newValue = [...(info.value as [string, string][]), ...pairs];
          newOffset = (newValue as [string, string][]).length;
          newHasMore = newOffset < info.total;
          break;
        }
      }

      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: {
            ...s.redisStates[tabId],
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
      }));
    } catch {
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: { ...s.redisStates[tabId], keyInfo: { ...info, loadingMore: false } },
        },
      }));
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
      const result = await ExecuteRedis(tab.assetId, "INFO keyspace", 0);
      const parsed: RedisResult = JSON.parse(result);
      const text = String(parsed.value || "");
      const counts: Record<number, number> = {};
      for (const line of text.split(/\r?\n/)) {
        const m = line.match(/^db(\d+):keys=(\d+)/);
        if (m) {
          counts[Number(m[1])] = Number(m[2]);
        }
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
        },
      },
    }));
  },
}));

// === Close Hook: clean up when tabStore closes a query tab ===

registerTabCloseHook((tab) => {
  if (tab.type !== "query") return;
  useQueryStore.setState((s) => {
    const newDbStates = { ...s.dbStates };
    delete newDbStates[tab.id];
    const newRedisStates = { ...s.redisStates };
    delete newRedisStates[tab.id];
    return { dbStates: newDbStates, redisStates: newRedisStates };
  });
});

// === Restore Hook: initialize query tab states ===

registerTabRestoreHook("query", (tabs) => {
  if (tabs.length === 0) return;

  const dbStates: Record<string, DatabaseTabState> = {};
  const redisStates: Record<string, RedisTabState> = {};
  for (const tab of tabs) {
    const m = tab.meta as QueryTabMeta;
    if (m.assetType === "database") {
      dbStates[tab.id] = defaultDbState();
    } else {
      redisStates[tab.id] = defaultRedisState();
    }
  }
  useQueryStore.setState({ dbStates, redisStates });
});
