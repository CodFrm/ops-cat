import { create } from "zustand";
import { ExecuteSQL, ExecuteRedis } from "../../wailsjs/go/main/App";
import { asset_entity } from "../../wailsjs/go/models";

// --- Types ---

export interface QueryTab {
  id: string; // "query:{assetId}"
  assetId: number;
  assetName: string;
  assetIcon: string;
  assetType: "database" | "redis";
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
}

const REDIS_PAGE_SIZE = 100;

export interface RedisKeyInfo {
  type: string;
  ttl: number;
  value: unknown;
  total: number;       // LLEN/HLEN/SCARD/ZCARD, -1 for string
  valueCursor: string;  // HSCAN/SSCAN cursor
  valueOffset: number;  // LRANGE/ZRANGE next offset
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
}

interface QueryState {
  openTabs: QueryTab[];
  dbStates: Record<string, DatabaseTabState>;
  redisStates: Record<string, RedisTabState>;

  openQueryTab: (asset: asset_entity.Asset) => void;
  closeQueryTab: (tabId: string) => void;
  reorderTabs: (fromIdx: number, toIdx: number) => void;

  // Database actions
  loadDatabases: (tabId: string) => Promise<void>;
  loadTables: (tabId: string, database: string) => Promise<void>;
  toggleDbExpand: (tabId: string, database: string) => void;
  openTableTab: (tabId: string, database: string, table: string) => void;
  openSqlTab: (tabId: string) => void;
  closeInnerTab: (tabId: string, innerTabId: string) => void;
  setActiveInnerTab: (tabId: string, innerTabId: string) => void;
  updateInnerTab: (tabId: string, innerTabId: string, patch: Record<string, unknown>) => void;

  // Redis actions
  scanKeys: (tabId: string, reset?: boolean) => Promise<void>;
  selectRedisDb: (tabId: string, db: number) => Promise<void>;
  selectKey: (tabId: string, key: string) => Promise<void>;
  loadMoreValues: (tabId: string) => Promise<void>;
  setKeyFilter: (tabId: string, pattern: string) => void;
}

// --- Helpers ---

function makeTabId(assetId: number) {
  return `query:${assetId}`;
}

function defaultDbState(): DatabaseTabState {
  return {
    databases: [],
    tables: {},
    expandedDbs: new Set(),
    loadingDbs: false,
    innerTabs: [],
    activeInnerTabId: null,
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

export const useQueryStore = create<QueryState>((set, get) => ({
  openTabs: [],
  dbStates: {},
  redisStates: {},

  openQueryTab: (asset) => {
    const tabId = makeTabId(asset.ID);
    const { openTabs } = get();
    if (openTabs.some((t) => t.id === tabId)) return;

    let driver: string | undefined;
    let defaultDatabase: string | undefined;
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      driver = cfg.driver;
      defaultDatabase = cfg.database;
    } catch { /* ignore */ }

    const tab: QueryTab = {
      id: tabId,
      assetId: asset.ID,
      assetName: asset.Name,
      assetIcon: asset.Icon || "",
      assetType: asset.Type as "database" | "redis",
      driver,
      defaultDatabase,
    };

    if (asset.Type === "database") {
      set((s) => ({
        openTabs: [...s.openTabs, tab],
        dbStates: { ...s.dbStates, [tabId]: defaultDbState() },
      }));
    } else {
      set((s) => ({
        openTabs: [...s.openTabs, tab],
        redisStates: { ...s.redisStates, [tabId]: defaultRedisState() },
      }));
    }
  },

  closeQueryTab: (tabId) => {
    set((s) => {
      const newDbStates = { ...s.dbStates };
      delete newDbStates[tabId];
      const newRedisStates = { ...s.redisStates };
      delete newRedisStates[tabId];
      return {
        openTabs: s.openTabs.filter((t) => t.id !== tabId),
        dbStates: newDbStates,
        redisStates: newRedisStates,
      };
    });
  },

  reorderTabs: (fromIdx, toIdx) => {
    set((s) => {
      const tabs = [...s.openTabs];
      const [moved] = tabs.splice(fromIdx, 1);
      tabs.splice(toIdx, 0, moved);
      return { openTabs: tabs };
    });
  },

  // --- Database ---

  loadDatabases: async (tabId) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
    if (!tab) return;

    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: { ...s.dbStates[tabId], loadingDbs: true },
      },
    }));

    try {
      const sql = tab.driver === "postgresql"
        ? "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"
        : "SHOW DATABASES";
      const result = await ExecuteSQL(tab.assetId, sql, "");
      const parsed: SQLResult = JSON.parse(result);
      const databases = (parsed.rows || []).map((r) => {
        const vals = Object.values(r);
        return String(vals[0] || "");
      }).filter(Boolean);

      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: { ...s.dbStates[tabId], databases, loadingDbs: false },
        },
      }));
    } catch {
      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: { ...s.dbStates[tabId], loadingDbs: false },
        },
      }));
    }
  },

  loadTables: async (tabId, database) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
    if (!tab) return;

    try {
      const sql = tab.driver === "postgresql"
        ? `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`
        : `SHOW TABLES FROM \`${database}\``;
      const result = await ExecuteSQL(tab.assetId, sql, database);
      const parsed: SQLResult = JSON.parse(result);
      const tables = (parsed.rows || []).map((r) => {
        const vals = Object.values(r);
        return String(vals[0] || "");
      }).filter(Boolean);

      set((s) => ({
        dbStates: {
          ...s.dbStates,
          [tabId]: {
            ...s.dbStates[tabId],
            tables: { ...s.dbStates[tabId].tables, [database]: tables },
          },
        },
      }));
    } catch { /* ignore */ }
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

  openSqlTab: (tabId) => {
    const state = get().dbStates[tabId];
    if (!state) return;
    const count = state.innerTabs.filter((t) => t.type === "sql").length + 1;
    const innerId = `sql:${Date.now()}`;
    set((s) => ({
      dbStates: {
        ...s.dbStates,
        [tabId]: {
          ...state,
          innerTabs: [...state.innerTabs, { id: innerId, type: "sql", title: `SQL ${count}` }],
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
          innerTabs: state.innerTabs.map((t) =>
            t.id === innerTabId ? ({ ...t, ...patch } as InnerTab) : t
          ),
        },
      },
    }));
  },

  // --- Redis ---

  scanKeys: async (tabId, reset) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
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
      const result = await ExecuteRedis(tab.assetId, cmd);
      const parsed: RedisResult = JSON.parse(result);

      // SCAN returns [cursor, [keys...]]
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
          },
        },
      }));
    } catch {
      set((s) => ({
        redisStates: {
          ...s.redisStates,
          [tabId]: { ...s.redisStates[tabId], loadingKeys: false },
        },
      }));
    }
  },

  selectRedisDb: async (tabId, db) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
    if (!tab) return;

    try {
      await ExecuteRedis(tab.assetId, `SELECT ${db}`);
    } catch { /* ignore */ }

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: {
          ...defaultRedisState(),
          currentDb: db,
          keyFilter: s.redisStates[tabId]?.keyFilter || "*",
        },
      },
    }));

    // Rescan with new DB
    get().scanKeys(tabId, true);
  },

  selectKey: async (tabId, key) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
    if (!tab) return;

    set((s) => ({
      redisStates: {
        ...s.redisStates,
        [tabId]: { ...s.redisStates[tabId], selectedKey: key, keyInfo: null },
      },
    }));

    try {
      // Get type + TTL
      const typeResult = await ExecuteRedis(tab.assetId, `TYPE ${key}`);
      const typeParsed: RedisResult = JSON.parse(typeResult);
      const keyType = String(typeParsed.value || "none");

      const ttlResult = await ExecuteRedis(tab.assetId, `TTL ${key}`);
      const ttlParsed: RedisResult = JSON.parse(ttlResult);
      const ttl = Number(ttlParsed.value) || -1;

      let value: unknown = null;
      let total = -1;
      let valueCursor = "";
      let valueOffset = 0;
      let hasMoreValues = false;

      switch (keyType) {
        case "string": {
          const r = await ExecuteRedis(tab.assetId, `GET ${key}`);
          value = JSON.parse(r).value;
          break;
        }
        case "list": {
          const [countR, valR] = await Promise.all([
            ExecuteRedis(tab.assetId, `LLEN ${key}`),
            ExecuteRedis(tab.assetId, `LRANGE ${key} 0 ${REDIS_PAGE_SIZE - 1}`),
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
            ExecuteRedis(tab.assetId, `HLEN ${key}`),
            ExecuteRedis(tab.assetId, `HSCAN ${key} 0 COUNT ${REDIS_PAGE_SIZE}`),
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
            ExecuteRedis(tab.assetId, `SCARD ${key}`),
            ExecuteRedis(tab.assetId, `SSCAN ${key} 0 COUNT ${REDIS_PAGE_SIZE}`),
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
            ExecuteRedis(tab.assetId, `ZCARD ${key}`),
            ExecuteRedis(tab.assetId, `ZRANGE ${key} 0 ${REDIS_PAGE_SIZE - 1} WITHSCORES`),
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
              type: keyType, ttl, value, total,
              valueCursor, valueOffset, hasMoreValues, loadingMore: false,
            },
          },
        },
      }));
    } catch { /* ignore */ }
  },

  loadMoreValues: async (tabId) => {
    const tab = get().openTabs.find((t) => t.id === tabId);
    const state = get().redisStates[tabId];
    if (!tab || !state?.keyInfo || !state.selectedKey || !state.keyInfo.hasMoreValues) return;

    const key = state.selectedKey;
    const info = state.keyInfo;

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
          const r = await ExecuteRedis(tab.assetId, `LRANGE ${key} ${newOffset} ${newOffset + REDIS_PAGE_SIZE - 1}`);
          const items = (JSON.parse(r).value as string[]) || [];
          newValue = [...(info.value as string[]), ...items];
          newOffset = (newValue as string[]).length;
          newHasMore = newOffset < info.total;
          break;
        }
        case "hash": {
          const r = await ExecuteRedis(tab.assetId, `HSCAN ${key} ${newCursor} COUNT ${REDIS_PAGE_SIZE}`);
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
          const r = await ExecuteRedis(tab.assetId, `SSCAN ${key} ${newCursor} COUNT ${REDIS_PAGE_SIZE}`);
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
          const r = await ExecuteRedis(tab.assetId, `ZRANGE ${key} ${newOffset} ${newOffset + REDIS_PAGE_SIZE - 1} WITHSCORES`);
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
              ...info, value: newValue,
              valueCursor: newCursor, valueOffset: newOffset,
              hasMoreValues: newHasMore, loadingMore: false,
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
}));
