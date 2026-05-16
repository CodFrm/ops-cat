/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { createRef } from "react";
import { MentionList, type MentionListRef, type MentionItem } from "@/components/ai/MentionList";
import { useAssetStore } from "@/stores/assetStore";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore } from "@/stores/tabStore";

function seed(assets: any[], groups: any[] = []) {
  useAssetStore.setState({
    assets,
    groups,
  } as any);
}

describe("MentionList", () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabId: null });
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} } as any);
    seed(
      [
        { ID: 42, Name: "prod-db", Type: "database", Icon: "mysql", GroupID: 0 },
        { ID: 43, Name: "prod-web", Type: "ssh", GroupID: 1 },
        { ID: 44, Name: "cache-1", Type: "redis", GroupID: 0 },
      ],
      [{ ID: 1, Name: "生产", ParentID: 0 }]
    );
  });

  it("按资产名过滤", async () => {
    const selected: MentionItem[] = [];
    render(<MentionList query="prod" command={(item) => selected.push(item)} />);
    const items = screen.getAllByRole("option");
    expect(items.map((el) => el.textContent)).toEqual(
      expect.arrayContaining([expect.stringContaining("prod-db"), expect.stringContaining("prod-web")])
    );
    expect(items).toHaveLength(2);
  });

  it("按分组路径过滤", async () => {
    render(<MentionList query="生产" command={() => {}} />);
    const items = screen.getAllByRole("option");
    expect(items).toHaveLength(1);
    expect(items[0].textContent).toContain("prod-web");
  });

  it("无匹配显示未找到", () => {
    render(<MentionList query="nope" command={() => {}} />);
    expect(screen.getByText("ai.mentionNotFound")).toBeInTheDocument();
  });

  it("Enter 触发 command 提交选中项", async () => {
    const ref = createRef<MentionListRef>();
    const received: MentionItem[] = [];
    render(<MentionList ref={ref} query="prod" command={(item) => received.push(item)} />);
    ref.current?.onKeyDown({ event: { key: "Enter" } as any });
    expect(received).toHaveLength(1);
    expect(received[0].id).toBe(42); // 前缀匹配 + 排序后第一项
  });

  it("ArrowDown 移动 selectedIndex", () => {
    const ref = createRef<MentionListRef>();
    render(<MentionList ref={ref} query="prod" command={() => {}} />);
    ref.current?.onKeyDown({ event: { key: "ArrowDown" } as any });
    const items = screen.getAllByRole("option");
    expect(items[1]).toHaveAttribute("aria-selected", "true");
  });

  it("当前 tab 是数据库时，保持单列样式并按上下文、默认库、资产、表排序", () => {
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          icon: "mysql",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app", "audit"],
          tables: { app: ["users", "orders"], audit: [] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [{ id: "table:app.users", type: "table", database: "app", table: "users" }],
          activeInnerTabId: "table:app.users",
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList query="" command={() => {}} />);

    expect(screen.queryByText("ai.mentionGroupContext")).not.toBeInTheDocument();
    const items = screen.getAllByRole("option");
    expect(items[0]).toHaveTextContent("prod-db/app.users");
    expect(items[1]).toHaveTextContent("prod-db/app");
    expect(items[2]).toHaveTextContent("prod-db");
    expect(items.at(-1)).toHaveTextContent("prod-db/app.orders");
    expect(items.map((item) => item.textContent)).not.toContain("prod-db/audit");
  });

  it("空查询时大量表排在资产之后，避免表名挤掉资产结果", () => {
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app"],
          tables: { app: ["users", "orders", "sessions", "events", "audit_logs", "user_profiles"] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList query="" command={() => {}} />);

    const itemTexts = screen.getAllByRole("option").map((item) => item.textContent ?? "");
    expect(itemTexts).toHaveLength(8);
    expect(itemTexts).toEqual(expect.arrayContaining(["prod-db", "生产/prod-web", "cache-1"]));
    expect(itemTexts.indexOf("prod-db")).toBeLessThan(itemTexts.findIndex((text) => text.includes("app.users")));
  });

  it("键盘选择按单列排序结果移动", () => {
    const ref = createRef<MentionListRef>();
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app"],
          tables: { app: ["users"] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [{ id: "table:app.users", type: "table", database: "app", table: "users" }],
          activeInnerTabId: "table:app.users",
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList ref={ref} query="" command={() => {}} />);

    ref.current?.onKeyDown({ event: { key: "ArrowDown" } as any });
    const items = screen.getAllByRole("option");
    expect(items[1]).toHaveAttribute("aria-selected", "true");
    expect(items[1]).toHaveTextContent("prod-db/app");
  });

  it("查询表名时，表级主字段匹配优先于同名资产", () => {
    seed([
      { ID: 42, Name: "prod-db", Type: "database", Icon: "mysql", GroupID: 0 },
      { ID: 45, Name: "users-api", Type: "ssh", GroupID: 0 },
    ]);
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app"],
          tables: { app: ["users"] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList query="users" command={() => {}} />);

    const items = screen.getAllByRole("option");
    expect(items[0]).toHaveTextContent("prod-db/app.users");
    expect(items[1]).toHaveTextContent("users-api");
  });

  it("查询库名时，库匹配优先于该库下的表", () => {
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app"],
          tables: { app: ["users"] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList query="app" command={() => {}} />);

    const items = screen.getAllByRole("option");
    expect(items[0]).toHaveTextContent("prod-db/app");
    expect(items[1]).toHaveTextContent("prod-db/app.users");
  });

  it("查询非默认的已加载库名时，不把库列表项作为候选", () => {
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
            defaultDatabase: "app",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app", "audit"],
          tables: { app: ["users"], audit: [] },
          loadingTables: {},
          expandedDbs: ["app", "audit"],
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList query="audit" command={() => {}} />);

    expect(screen.queryByRole("option")).not.toBeInTheDocument();
    expect(screen.getByText("ai.mentionNotFound")).toBeInTheDocument();
  });

  it("选择表 mention 时返回资产 ID 和库表上下文", () => {
    const ref = createRef<MentionListRef>();
    const received: MentionItem[] = [];
    useTabStore.setState({
      activeTabId: "query-42",
      tabs: [
        {
          id: "query-42",
          type: "query",
          label: "prod-db",
          meta: {
            type: "query",
            assetId: 42,
            assetName: "prod-db",
            assetIcon: "mysql",
            assetType: "database",
            driver: "mysql",
          },
        },
      ],
    } as any);
    useQueryStore.setState({
      dbStates: {
        "query-42": {
          databases: ["app"],
          tables: { app: ["users"] },
          loadingTables: {},
          expandedDbs: ["app"],
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    } as any);

    render(<MentionList ref={ref} query="users" command={(item) => received.push(item)} />);
    ref.current?.onKeyDown({ event: { key: "Enter" } as any });

    expect(received[0]).toMatchObject({
      id: 42,
      kind: "table",
      label: "app.users",
      type: "database",
      database: "app",
      table: "users",
      driver: "mysql",
    });
  });
});
