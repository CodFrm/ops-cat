import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { RedisPanel } from "../components/query/RedisPanel";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import {
  ExecuteRedis,
  RedisClientList,
  RedisCommandHistory,
  RedisGetKeyDetail,
  RedisListDatabases,
  RedisScanKeys,
  RedisSlowLog,
} from "../../wailsjs/go/app/App";

describe("RedisPanel", () => {
  const selectPanelKey = async (key: string) => {
    await act(async () => {
      await useQueryStore.getState().selectKey("query-10", key);
    });
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(RedisScanKeys).mockResolvedValue({ cursor: "0", keys: [], hasMore: false });
    vi.mocked(RedisListDatabases).mockResolvedValue([{ db: 0, keys: 1, expires: 0, avgTtl: 0 }]);
    vi.mocked(RedisSlowLog).mockResolvedValue([]);
    vi.mocked(RedisClientList).mockResolvedValue("");
    vi.mocked(RedisCommandHistory).mockResolvedValue([]);
    vi.mocked(RedisGetKeyDetail).mockImplementation(async ({ key }) => ({
      key,
      type: "string",
      ttl: -1,
      size: String(key).length,
      total: -1,
      value: key,
      valueCursor: "0",
      valueOffset: 0,
      hasMoreValues: false,
    }));
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

  it("keeps multiple key detail tabs open and lets users close a tab", async () => {
    render(<RedisPanel tabId="query-10" />);

    await selectPanelKey("common:user:1");

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /common:user:1/ })).toHaveAttribute("aria-selected", "true");
    });

    await selectPanelKey("dispatcher:task:2");

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /common:user:1/ })).toBeInTheDocument();
      expect(screen.getByRole("tab", { name: /dispatcher:task:2/ })).toHaveAttribute("aria-selected", "true");
    });

    fireEvent.click(screen.getByLabelText("query.closeRedisKeyTab common:user:1"));

    expect(screen.queryByRole("tab", { name: /common:user:1/ })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /dispatcher:task:2/ })).toHaveAttribute("aria-selected", "true");
  });

  it("clears selected key when closing the last detail tab so the same key can reopen", async () => {
    render(<RedisPanel tabId="query-10" />);

    await selectPanelKey("common:user:1");

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /common:user:1/ })).toHaveAttribute("aria-selected", "true");
    });

    fireEvent.click(screen.getByLabelText("query.closeRedisKeyTab common:user:1"));

    await waitFor(() => {
      expect(screen.queryByRole("tab", { name: /common:user:1/ })).not.toBeInTheDocument();
      expect(screen.getByRole("tab", { name: "query.redisOverview" })).toHaveAttribute("aria-selected", "true");
    });
    expect(useQueryStore.getState().redisStates["query-10"].selectedKey).toBeNull();
    expect(useQueryStore.getState().redisStates["query-10"].keyInfo).toBeNull();

    await selectPanelKey("common:user:1");

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /common:user:1/ })).toHaveAttribute("aria-selected", "true");
    });
  });

  it("closes an open key tab when the key is deleted elsewhere in the browser", async () => {
    render(<RedisPanel tabId="query-10" />);

    act(() => {
      useQueryStore.setState((s) => ({
        redisStates: {
          ...s.redisStates,
          "query-10": {
            ...s.redisStates["query-10"],
            keys: ["common:user:1"],
          },
        },
      }));
    });

    await selectPanelKey("common:user:1");

    expect(screen.getByRole("tab", { name: /common:user:1/ })).toHaveAttribute("aria-selected", "true");

    act(() => {
      useQueryStore.getState().removeKey("query-10", "common:user:1");
    });

    await waitFor(() => {
      expect(screen.queryByRole("tab", { name: /common:user:1/ })).not.toBeInTheDocument();
      expect(screen.getByRole("tab", { name: "query.redisOverview" })).toHaveAttribute("aria-selected", "true");
    });
  });

  it("keeps key tabs readable with full-name tooltips and mouse-wheel horizontal scrolling", async () => {
    const longKey = "dispatcher:dispatch_task_map:5efab087-2cd6-4dc6-b4a9-3ab25";
    render(<RedisPanel tabId="query-10" />);

    await selectPanelKey(longKey);

    const tab = await screen.findByRole("tab", { name: longKey });
    expect(tab).toHaveAttribute("title", longKey);

    const tabStrip = screen.getByTestId("redis-key-tab-strip");
    expect(tabStrip).toHaveClass("h-9", "overflow-x-auto", "overflow-y-hidden");

    tabStrip.scrollLeft = 0;
    fireEvent.wheel(tabStrip, { deltaY: 96, deltaX: 0 });

    expect(tabStrip.scrollLeft).toBe(96);
  });
});
