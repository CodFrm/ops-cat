import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { RedisKeyDetail } from "../components/query/RedisKeyDetail";
import { RedisStreamViewer } from "../components/query/RedisStreamViewer";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import { ExecuteRedisArgs, RedisGetKeyDetail, RedisSetKeyTTL, RedisStreamAdd } from "../../wailsjs/go/app/App";

describe("RedisKeyDetail", () => {
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
          currentDb: 2,
          keys: ["user:1"],
          loadingKeys: false,
          keyFilter: "*",
          scanCursor: "0",
          hasMore: false,
          selectedKey: "user:1",
          keyInfo: {
            type: "string",
            ttl: 60,
            size: 8,
            total: -1,
            value: "value",
            valueCursor: "",
            valueOffset: 0,
            hasMoreValues: false,
            loadingMore: false,
          },
          dbKeyCounts: {},
          error: null,
        },
      },
    });
    vi.mocked(RedisGetKeyDetail).mockResolvedValue({
      key: "user:1",
      type: "string",
      ttl: 120,
      size: 8,
      total: -1,
      value: "value",
      valueCursor: "",
      valueOffset: 0,
      hasMoreValues: false,
    });
  });

  it("sets ttl through the typed redis binding", async () => {
    vi.mocked(RedisSetKeyTTL).mockResolvedValue(undefined);

    render(<RedisKeyDetail tabId="query-10" />);

    fireEvent.click(screen.getByText(/query.ttl:/));
    fireEvent.change(screen.getByPlaceholderText("query.ttlInput"), { target: { value: "120" } });
    fireEvent.click(screen.getByText("query.setTtl"));

    await waitFor(() => {
      expect(RedisSetKeyTTL).toHaveBeenCalledWith(10, 2, "user:1", 120);
    });
    expect(ExecuteRedisArgs).not.toHaveBeenCalled();
  });

  it("highlights JSON string values without wrapping long content out of the detail area", () => {
    useQueryStore.setState((s) => ({
      redisStates: {
        ...s.redisStates,
        "query-10": {
          ...s.redisStates["query-10"],
          selectedKey: "json:1",
          keyInfo: {
            type: "string",
            ttl: -1,
            size: 34,
            total: -1,
            value: '{"a":1,"enabled":true,"payload":"x"}',
            valueCursor: "",
            valueOffset: 0,
            hasMoreValues: false,
            loadingMore: false,
          },
        },
      },
    }));

    render(<RedisKeyDetail tabId="query-10" />);

    const valueBox = screen.getByTestId("redis-string-value");
    expect(valueBox).toHaveClass("overflow-auto", "whitespace-pre");
    expect(valueBox).not.toHaveClass("break-all");
    expect(within(valueBox).getByText('"a"')).toHaveClass("text-sky-700");
    expect(within(valueBox).getByText("1")).toHaveClass("text-purple-700");
    expect(within(valueBox).getByText("true")).toHaveClass("text-amber-700");
  });

  it("executes command input with quoted arguments preserved", async () => {
    vi.mocked(ExecuteRedisArgs).mockResolvedValue(JSON.stringify({ type: "string", value: "OK" }));

    render(<RedisKeyDetail tabId="query-10" />);

    fireEvent.change(screen.getByPlaceholderText("query.redisPlaceholder"), {
      target: { value: 'SET "my key" "hello world"' },
    });
    fireEvent.keyDown(screen.getByPlaceholderText("query.redisPlaceholder"), { key: "Enter" });

    await waitFor(() => {
      expect(ExecuteRedisArgs).toHaveBeenCalledWith(10, ["SET", "my key", "hello world"], 2);
    });
  });
});

describe("RedisStreamViewer", () => {
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
          currentDb: 1,
          keys: ["events"],
          loadingKeys: false,
          keyFilter: "*",
          scanCursor: "0",
          hasMore: false,
          selectedKey: "events",
          keyInfo: null,
          dbKeyCounts: {},
          error: null,
        },
      },
    });
    vi.mocked(RedisGetKeyDetail).mockResolvedValue({
      key: "events",
      type: "stream",
      ttl: -1,
      size: 0,
      total: 1,
      value: [{ id: "1-0", fields: { name: "Ada" } }],
      valueCursor: "1-0",
      valueOffset: 1,
      hasMoreValues: false,
    });
  });

  it("adds stream entries through the typed redis binding", async () => {
    vi.mocked(RedisStreamAdd).mockResolvedValue(undefined);

    render(
      <RedisStreamViewer
        tabId="query-10"
        t={(key) => key}
        info={{
          type: "stream",
          ttl: -1,
          size: 0,
          total: 0,
          value: [],
          valueCursor: "",
          valueOffset: 0,
          hasMoreValues: false,
          loadingMore: false,
        }}
      />
    );

    fireEvent.change(screen.getByPlaceholderText("query.streamEntryId"), { target: { value: "*" } });
    fireEvent.change(screen.getByPlaceholderText("query.streamField"), { target: { value: "name" } });
    fireEvent.change(screen.getByPlaceholderText("query.streamValue"), { target: { value: "Ada" } });
    fireEvent.click(screen.getByTitle("query.addEntry"));

    await waitFor(() => {
      expect(RedisStreamAdd).toHaveBeenCalledWith(10, 1, "events", "*", [{ field: "name", value: "Ada" }]);
    });
  });
});
