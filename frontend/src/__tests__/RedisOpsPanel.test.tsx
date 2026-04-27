import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { RedisOpsPanel } from "../components/query/RedisOpsPanel";
import { useTabStore } from "../stores/tabStore";
import { useQueryStore } from "../stores/queryStore";
import { ExecuteRedis } from "../../wailsjs/go/app/App";

describe("RedisOpsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
          dbKeyCounts: {},
          error: null,
        },
      },
    });
  });

  it("renders redis info details, keyspace stats, and searchable info rows", async () => {
    vi.mocked(ExecuteRedis).mockResolvedValue(
      JSON.stringify({
        type: "string",
        value:
          "# Server\r\nredis_version:7.4.8\r\nos:Linux6.8.7.2-microsoft-standard-WSL2x86_64\r\nprocess_id:1\r\nredis_git_sha1:00000000\r\nredis_build_id:dc3fdca8addf42ba\r\n# Clients\r\nconnected_clients:12\r\ntotal_connections_received:6684\r\n# Memory\r\nused_memory_human:79.33M\r\nused_memory_peak_human:83.89M\r\nused_memory_lua_human:43K\r\n# Stats\r\ntotal_commands_processed:32863384\r\n# Keyspace\r\ndb0:keys=8795,expires=8771,avg_ttl=253391607\r\n",
      })
    );

    render(<RedisOpsPanel tabId="query-10" />);

    await waitFor(() => {
      expect(ExecuteRedis).toHaveBeenCalledWith(10, "INFO", 0);
    });
    expect(screen.getByText("query.redisServer")).toBeInTheDocument();
    expect(screen.getByText("query.redisVersion:")).toBeInTheDocument();
    expect(screen.getAllByText("7.4.8").length).toBeGreaterThan(0);
    expect(screen.getAllByText("db0").length).toBeGreaterThan(0);
    expect(screen.getByText("8,795")).toBeInTheDocument();
    expect(screen.getByText("253,391,607")).toBeInTheDocument();
    expect(screen.getByText("redis_build_id")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("query.redisInfoSearch"), { target: { value: "git" } });

    expect(screen.getByText("redis_git_sha1")).toBeInTheDocument();
    expect(screen.queryByText("redis_build_id")).not.toBeInTheDocument();
    expect(screen.queryByText("process_id")).not.toBeInTheDocument();
  });
});
