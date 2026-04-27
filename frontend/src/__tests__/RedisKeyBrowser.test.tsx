import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { RedisKeyBrowser } from "../components/query/RedisKeyBrowser";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import { RedisListDatabases, RedisScanKeys, RedisSetStringValue } from "../../wailsjs/go/app/App";

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

    render(<RedisKeyBrowser tabId="query-10" />);

    fireEvent.click(screen.getByTitle("query.createRedisKey"));
    fireEvent.change(screen.getByPlaceholderText("query.redisKeyNamePlaceholder"), {
      target: { value: "new:key" },
    });
    fireEvent.change(screen.getByPlaceholderText("query.newValue"), {
      target: { value: "hello" },
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
});
