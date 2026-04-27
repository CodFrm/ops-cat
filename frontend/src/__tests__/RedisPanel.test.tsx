import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { RedisPanel } from "../components/query/RedisPanel";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import { ExecuteRedis, RedisClientList, RedisCommandHistory, RedisListDatabases, RedisScanKeys, RedisSlowLog } from "../../wailsjs/go/app/App";

describe("RedisPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "0", keys: [], hasMore: false });
    vi.mocked(RedisListDatabases).mockResolvedValue([{ db: 0, keys: 1, expires: 0, avgTtl: 0 }]);
    vi.mocked(RedisSlowLog).mockResolvedValue([]);
    vi.mocked(RedisClientList).mockResolvedValue("");
    vi.mocked(RedisCommandHistory).mockResolvedValue([]);
    vi.mocked(ExecuteRedis).mockResolvedValue(
      JSON.stringify({
        type: "string",
        value:
          "# Server\r\nredis_version:7.2.4\r\nuptime_in_seconds:7200\r\n# Clients\r\nconnected_clients:2\r\n# Memory\r\nused_memory_human:12.34M\r\n# Stats\r\ntotal_commands_processed:128\r\n# Keyspace\r\ndb0:keys=1,expires=0,avg_ttl=0\r\n",
      })
    );
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
          keys: [],
          loadingKeys: false,
          keyFilter: "*",
          scanCursor: "0",
          hasMore: false,
          selectedKey: null,
          keyInfo: null,
          dbKeyCounts: { 0: 1 },
          error: null,
        },
      },
    });
  });

  it("shows the Redis overview as the default top tab", async () => {
    render(<RedisPanel tabId="query-10" />);

    expect(screen.getByRole("tab", { name: "query.redisOverview" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText("query.noKeySelected")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getAllByText("7.2.4").length).toBeGreaterThan(0);
    });
  });
});
