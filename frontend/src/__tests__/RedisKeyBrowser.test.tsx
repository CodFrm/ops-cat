import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { RedisKeyBrowser } from "../components/query/RedisKeyBrowser";
import { buildKeyTree, flattenTree, makeLocalKeyMatcher } from "../lib/redisKeyTree";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import {
  RedisHashSet,
  RedisListDatabases,
  RedisListPush,
  RedisScanKeys,
  RedisSetKeyTTL,
  RedisSetStringValue,
} from "../../wailsjs/go/app/App";

describe("RedisKeyBrowser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(RedisScanKeys).mockResolvedValue({
      cursor: "0",
      keys: ["common:user:1", "common:user:2", "dispatcher:task:1"],
      hasMore: false,
    });
    vi.mocked(RedisListDatabases).mockResolvedValue([
      { db: 0, keys: 7767, expires: 0, avgTtl: 0 },
      { db: 1, keys: 12, expires: 0, avgTtl: 0 },
    ]);
    useTabStore.setState({
      activeTabId: "query-10",
      tabs: [
        {
          id: "query-10",
          type: "query",
          label: "Redis",
          meta: { type: "query", assetId: 10, assetName: "Redis", assetIcon: "", assetType: "redis" },
        },
      ],
    });
    useQueryStore.setState({
      redisStates: {
        "query-10": {
          currentDb: 0,
          keys: ["common:user:1", "common:user:2", "dispatcher:task:1"],
          loadingKeys: false,
          keyFilter: "*",
          scanCursor: "23",
          hasMore: true,
          selectedKey: null,
          keyInfo: null,
          dbKeyCounts: { 0: 7767, 1: 12 },
          error: null,
        },
      },
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("defaults to tree view and keeps database selection in the footer", () => {
    render(<RedisKeyBrowser tabId="query-10" />);

    expect(screen.getByTitle("query.listView")).toBeInTheDocument();
    expect(screen.getByTitle("query.createRedisKey")).toBeInTheDocument();
    expect(screen.queryByText("query.loadMore")).not.toBeInTheDocument();
    expect(screen.getByTestId("redis-key-tree")).toHaveAttribute("data-counts-incomplete", "true");
    expect(screen.getByTestId("redis-db-footer")).toHaveTextContent("db0");
  });

  it("creates a string key from the add key dialog", async () => {
    vi.mocked(RedisSetStringValue).mockResolvedValue(undefined);
    vi.mocked(RedisSetKeyTTL).mockResolvedValue(undefined);

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByTitle("query.createRedisKey"));
    fireEvent.change(screen.getByTestId("redis-create-key-input"), {
      target: { value: "new:key" },
    });
    fireEvent.change(screen.getByTestId("redis-create-string-value"), {
      target: { value: "hello" },
    });
    fireEvent.change(screen.getByTestId("redis-create-ttl-input"), {
      target: { value: "60" },
    });
    fireEvent.click(screen.getByText("query.createRedisKeySubmit"));

    await waitFor(() => {
      expect(RedisSetStringValue).toHaveBeenCalledWith({
        assetId: 10,
        db: 0,
        key: "new:key",
        value: "hello",
        format: "raw",
      });
    });
    expect(RedisSetKeyTTL).toHaveBeenCalledWith(10, 0, "new:key", 60);
  });

  it("creates a hash key with multiple initial fields", async () => {
    vi.mocked(RedisHashSet).mockResolvedValue(undefined);

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByTitle("query.createRedisKey"));
    fireEvent.change(screen.getByTestId("redis-create-key-input"), {
      target: { value: "profile:1" },
    });
    fireEvent.click(screen.getByTestId("redis-create-type-trigger"));
    fireEvent.click(await screen.findByRole("option", { name: "hash" }));
    fireEvent.change(screen.getByTestId("redis-create-hash-field-0"), {
      target: { value: "name" },
    });
    fireEvent.change(screen.getByTestId("redis-create-hash-value-0"), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByTestId("redis-create-add-row"));
    fireEvent.change(screen.getByTestId("redis-create-hash-field-1"), {
      target: { value: "role" },
    });
    fireEvent.change(screen.getByTestId("redis-create-hash-value-1"), {
      target: { value: "admin" },
    });
    fireEvent.click(screen.getByText("query.createRedisKeySubmit"));

    await waitFor(() => {
      expect(RedisHashSet).toHaveBeenCalledTimes(2);
    });
    expect(RedisHashSet).toHaveBeenNthCalledWith(1, 10, 0, "profile:1", "name", "Ada");
    expect(RedisHashSet).toHaveBeenNthCalledWith(2, 10, 0, "profile:1", "role", "admin");
  });

  it("creates a list key in the same order as the initial values", async () => {
    vi.mocked(RedisListPush).mockResolvedValue(undefined);

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByTitle("query.createRedisKey"));
    fireEvent.change(screen.getByTestId("redis-create-key-input"), {
      target: { value: "queue:1" },
    });
    fireEvent.click(screen.getByTestId("redis-create-type-trigger"));
    fireEvent.click(await screen.findByRole("option", { name: "list" }));

    fireEvent.change(screen.getAllByPlaceholderText("query.newValue")[0], {
      target: { value: "first" },
    });
    fireEvent.click(screen.getByTestId("redis-create-add-row"));
    fireEvent.change(screen.getAllByPlaceholderText("query.newValue")[1], {
      target: { value: "second" },
    });
    fireEvent.click(screen.getByText("query.createRedisKeySubmit"));

    await waitFor(() => {
      expect(RedisListPush).toHaveBeenCalledTimes(2);
    });
    expect(RedisListPush).toHaveBeenNthCalledWith(1, 10, 0, "queue:1", "first");
    expect(RedisListPush).toHaveBeenNthCalledWith(2, 10, 0, "queue:1", "second");
  });

  it("opens a lightweight database menu and selects a db", async () => {
    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByRole("button", { name: /db0/ }));

    const menu = screen.getByTestId("redis-db-menu");
    expect(menu).toHaveClass("overflow-y-auto");
    expect(menu).toHaveStyle({ maxHeight: "320px" });

    fireEvent.click(screen.getByRole("option", { name: /^db1\b/ }));

    await waitFor(() => {
      expect(RedisScanKeys).toHaveBeenCalledWith(
        expect.objectContaining({
          assetId: 10,
          db: 1,
          cursor: "0",
        })
      );
    });
    expect(screen.queryByTestId("redis-db-menu")).not.toBeInTheDocument();
  });

  it("includes non-empty databases beyond the default range in the db menu", async () => {
    useQueryStore.setState((s) => ({
      redisStates: {
        ...s.redisStates,
        "query-10": {
          ...s.redisStates["query-10"],
          dbKeyCounts: { 0: 7767, 20: 9 },
        },
      },
    }));

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByRole("button", { name: /db0/ }));

    expect(screen.getByRole("option", { name: /^db20\b/ })).toBeInTheDocument();
  });

  it("keeps prefix keys expandable when a key also has children", () => {
    const tree = buildKeyTree(["root", "root:session"], ":");
    const collapsed = flattenTree(tree, new Set(), ":");
    const expanded = flattenTree(tree, new Set(["root"]), ":");

    expect(collapsed[0]).toEqual(
      expect.objectContaining({
        name: "root",
        fullKey: "root",
        hasChildren: true,
        keyCount: 2,
      })
    );
    expect(expanded.map((row) => row.name)).toEqual(["root", "session"]);
  });

  it("opens a prefix key and expands its child keys from tree mode", async () => {
    vi.mocked(RedisScanKeys).mockResolvedValueOnce({
      cursor: "0",
      keys: ["root", "root:session"],
      hasMore: false,
    });

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(await screen.findByRole("button", { name: /^root$/ }));

    await waitFor(() => {
      expect(useQueryStore.getState().redisStates["query-10"].selectedKey).toBe("root");
    });

    fireEvent.click(screen.getByTitle("query.expandFolder root"));

    expect(await screen.findByRole("button", { name: /^session$/ })).toBeInTheDocument();
  });

  it("does not overwrite an existing key from the add key dialog", async () => {
    vi.mocked(RedisSetStringValue).mockResolvedValue(undefined);

    render(<RedisKeyBrowser tabId="query-10" />);
    vi.mocked(RedisScanKeys).mockClear();
    vi.mocked(RedisScanKeys).mockResolvedValueOnce({
      cursor: "0",
      keys: ["new:key"],
      hasMore: false,
    });

    fireEvent.click(screen.getByTitle("query.createRedisKey"));
    fireEvent.change(screen.getByTestId("redis-create-key-input"), {
      target: { value: "new:key" },
    });
    fireEvent.change(screen.getByTestId("redis-create-string-value"), {
      target: { value: "hello" },
    });
    fireEvent.click(screen.getByText("query.createRedisKeySubmit"));

    await waitFor(() => {
      expect(RedisScanKeys).toHaveBeenCalledWith(
        expect.objectContaining({
          assetId: 10,
          db: 0,
          match: "new:key",
          exact: true,
        })
      );
    });
    expect(RedisSetStringValue).not.toHaveBeenCalled();
  });

  it("filters locally while typing and searches Redis on Enter", async () => {
    const matcher = makeLocalKeyMatcher("dispatcher");
    expect(["common:user:1", "dispatcher:task:1"].filter(matcher)).toEqual(["dispatcher:task:1"]);

    vi.useFakeTimers();
    render(<RedisKeyBrowser tabId="query-10" />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(RedisScanKeys).toHaveBeenCalled();
    vi.mocked(RedisScanKeys).mockClear();

    fireEvent.change(screen.getByPlaceholderText("query.filterKeys"), { target: { value: "dispatcher" } });
    await act(async () => {
      vi.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(RedisScanKeys).not.toHaveBeenCalled();

    await act(async () => {
      fireEvent.keyDown(screen.getByPlaceholderText("query.filterKeys"), { key: "Enter" });
      await Promise.resolve();
    });

    expect(RedisScanKeys).toHaveBeenCalledWith(expect.objectContaining({ match: "*dispatcher*" }));
  });
});
