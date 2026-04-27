import { describe, it, expect, beforeEach, vi } from "vitest";
import { useTabStore } from "../stores/tabStore";
import { useQueryStore } from "../stores/queryStore";
import { useAssetStore } from "../stores/assetStore";
import { asset_entity } from "../../wailsjs/go/models";
import { RedisGetKeyDetail, RedisListDatabases, RedisScanKeys } from "../../wailsjs/go/app/App";

function makeDatabaseAsset(id: number, name = `DB ${id}`): asset_entity.Asset {
  return {
    ID: id,
    Name: name,
    Type: "database",
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    Config: JSON.stringify({ driver: "mysql", database: "testdb", host: "10.0.0.1", port: 3306 }),
    CmdPolicy: "",
    SortOrder: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  } as asset_entity.Asset;
}

function makeRedisAsset(id: number, name = `Redis ${id}`): asset_entity.Asset {
  return {
    ID: id,
    Name: name,
    Type: "redis",
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    Config: JSON.stringify({ host: "10.0.0.1", port: 6379 }),
    CmdPolicy: "",
    SortOrder: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  } as asset_entity.Asset;
}

function makeMongoAsset(id: number, name = `Mongo ${id}`): asset_entity.Asset {
  return {
    ID: id,
    Name: name,
    Type: "mongodb",
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    Config: JSON.stringify({ host: "10.0.0.1", port: 27017 }),
    CmdPolicy: "",
    SortOrder: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  } as asset_entity.Asset;
}

describe("queryStore.openQueryTab", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} });
    vi.spyOn(useAssetStore.getState(), "getAssetPath").mockReturnValue("Test/DB");
  });

  it("should open a new database query tab", () => {
    const asset = makeDatabaseAsset(1);
    useQueryStore.getState().openQueryTab(asset);

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("query-1");
    expect(tabs[0].type).toBe("query");

    const meta = tabs[0].meta as { assetType: string; driver: string };
    expect(meta.assetType).toBe("database");
    expect(meta.driver).toBe("mysql");

    // Should initialize dbStates
    expect(useQueryStore.getState().dbStates["query-1"]).toBeDefined();
    expect(useQueryStore.getState().redisStates["query-1"]).toBeUndefined();
  });

  it("should open a new redis query tab", () => {
    const asset = makeRedisAsset(10);
    useQueryStore.getState().openQueryTab(asset);

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("query-10");

    // Should initialize redisStates
    expect(useQueryStore.getState().redisStates["query-10"]).toBeDefined();
    expect(useQueryStore.getState().dbStates["query-10"]).toBeUndefined();
  });

  it("should reuse existing tab for same asset", () => {
    const asset = makeDatabaseAsset(1);
    useQueryStore.getState().openQueryTab(asset);
    useQueryStore.getState().openQueryTab(asset);

    // Only one tab should exist
    expect(useTabStore.getState().tabs).toHaveLength(1);
    // Should activate it
    expect(useTabStore.getState().activeTabId).toBe("query-1");
  });

  it("should activate existing tab instead of creating duplicate", () => {
    const db1 = makeDatabaseAsset(1);
    const db2 = makeDatabaseAsset(2);

    useQueryStore.getState().openQueryTab(db1);
    useQueryStore.getState().openQueryTab(db2);
    expect(useTabStore.getState().activeTabId).toBe("query-2");

    // Open db1 again — should switch to it, not create new
    useQueryStore.getState().openQueryTab(db1);
    expect(useTabStore.getState().activeTabId).toBe("query-1");
    expect(useTabStore.getState().tabs).toHaveLength(2);
  });

  it("should allow different assets to open separate tabs", () => {
    useQueryStore.getState().openQueryTab(makeDatabaseAsset(1));
    useQueryStore.getState().openQueryTab(makeDatabaseAsset(2));
    useQueryStore.getState().openQueryTab(makeRedisAsset(3));

    expect(useTabStore.getState().tabs).toHaveLength(3);
    expect(useQueryStore.getState().dbStates["query-1"]).toBeDefined();
    expect(useQueryStore.getState().dbStates["query-2"]).toBeDefined();
    expect(useQueryStore.getState().redisStates["query-3"]).toBeDefined();
  });

  it("should open a new mongodb query tab", () => {
    const asset = makeMongoAsset(20);
    useQueryStore.getState().openQueryTab(asset);

    const tabs = useTabStore.getState().tabs;
    expect(tabs).toHaveLength(1);
    expect(tabs[0].id).toBe("query-20");
    expect(tabs[0].type).toBe("query");

    const meta = tabs[0].meta as { assetType: string };
    expect(meta.assetType).toBe("mongodb");

    // Should initialize mongoStates, not db/redis
    expect(useQueryStore.getState().mongoStates["query-20"]).toBeDefined();
    expect(useQueryStore.getState().dbStates["query-20"]).toBeUndefined();
    expect(useQueryStore.getState().redisStates["query-20"]).toBeUndefined();
  });

  it("should reuse existing mongodb query tab", () => {
    const asset = makeMongoAsset(20);
    useQueryStore.getState().openQueryTab(asset);
    useQueryStore.getState().openQueryTab(asset);

    // Only one tab should exist
    expect(useTabStore.getState().tabs).toHaveLength(1);
    // Should activate it
    expect(useTabStore.getState().activeTabId).toBe("query-20");
  });
});

describe("queryStore redis actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useTabStore.setState({ tabs: [], activeTabId: null });
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} });
    vi.spyOn(useAssetStore.getState(), "getAssetPath").mockReturnValue("Test/Redis");
    useQueryStore.getState().openQueryTab(makeRedisAsset(10));
  });

  it("scans redis keys through typed binding", async () => {
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "5", keys: ["a", "b"], hasMore: true });

    await useQueryStore.getState().scanKeys("query-10", true);

    expect(RedisScanKeys).toHaveBeenCalledWith({
      assetId: 10,
      db: 0,
      cursor: "0",
      match: "*",
      type: "",
      count: 200,
      exact: false,
    });
    const state = useQueryStore.getState().redisStates["query-10"];
    expect(state.keys).toEqual(["a", "b"]);
    expect(state.scanCursor).toBe("5");
    expect(state.hasMore).toBe(true);
  });

  it("uses contains matching for plain redis key search", async () => {
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "0", keys: [], hasMore: false });

    useQueryStore.getState().setKeyFilter("query-10", "2fe43136-1b38-43c3-b4bf-82b19c66c7bf");
    await useQueryStore.getState().scanKeys("query-10", true);

    expect(RedisScanKeys).toHaveBeenCalledWith(
      expect.objectContaining({
        match: "*2fe43136-1b38-43c3-b4bf-82b19c66c7bf*",
      })
    );
  });

  it("preserves explicit redis match patterns", async () => {
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "0", keys: [], hasMore: false });

    useQueryStore.getState().setKeyFilter("query-10", "common:*");
    await useQueryStore.getState().scanKeys("query-10", true);

    expect(RedisScanKeys).toHaveBeenCalledWith(expect.objectContaining({ match: "common:*" }));
  });

  it("loads selected key detail through typed binding", async () => {
    vi.mocked(RedisGetKeyDetail).mockResolvedValue({
      key: "user:1",
      type: "hash",
      ttl: 120,
      size: 42,
      total: 1,
      value: [{ field: "name", value: "Ada" }],
      valueCursor: "0",
      valueOffset: 0,
      hasMoreValues: false,
    });

    await useQueryStore.getState().selectKey("query-10", "user:1");

    expect(RedisGetKeyDetail).toHaveBeenCalledWith({
      assetId: 10,
      db: 0,
      key: "user:1",
      cursor: "",
      offset: 0,
      count: 100,
    });
    const info = useQueryStore.getState().redisStates["query-10"].keyInfo;
    expect(info?.type).toBe("hash");
    expect(info?.ttl).toBe(120);
    expect(info?.value).toEqual([["name", "Ada"]]);
  });

  it("loads db key counts through typed binding", async () => {
    vi.mocked(RedisListDatabases).mockResolvedValue([
      { db: 0, keys: 2, expires: 1, avgTtl: 10 },
      { db: 3, keys: 7, expires: 0, avgTtl: 0 },
    ]);

    await useQueryStore.getState().loadDbKeyCounts("query-10");

    expect(RedisListDatabases).toHaveBeenCalledWith(10);
    expect(useQueryStore.getState().redisStates["query-10"].dbKeyCounts).toEqual({ 0: 2, 3: 7 });
  });

  it("uses redis browser options from asset config", async () => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} });
    const asset = makeRedisAsset(11);
    asset.Config = JSON.stringify({ host: "10.0.0.1", port: 6379, database: 3, scan_page_size: 500 });
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "0", keys: [], hasMore: false });

    useQueryStore.getState().openQueryTab(asset);
    await useQueryStore.getState().scanKeys("query-11", true);

    expect(useQueryStore.getState().redisStates["query-11"].currentDb).toBe(3);
    expect(RedisScanKeys).toHaveBeenCalledWith(
      expect.objectContaining({
        assetId: 11,
        db: 3,
        count: 500,
      })
    );
  });
});
