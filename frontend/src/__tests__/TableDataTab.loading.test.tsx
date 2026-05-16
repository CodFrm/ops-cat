import { describe, expect, it, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TableDataTab } from "@/components/query/TableDataTab";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore } from "@/stores/tabStore";
import { ExecuteSQL, OpenTable } from "../../wailsjs/go/app/App";

// 构造 OpenTable 返回的 JSON 字符串(与后端 query_svc.OpenTableResult 对齐)。
function openTablePayload(opts: {
  columns?: string[];
  rows?: Record<string, unknown>[];
  totalCount?: number;
  primaryKeys?: string[];
}) {
  return JSON.stringify({
    columns: opts.columns ?? [],
    columnTypes: {},
    columnRules: [],
    primaryKeys: opts.primaryKeys ?? [],
    totalCount: opts.totalCount ?? 0,
    firstPage: opts.rows ?? [],
    pageSize: 1000,
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function setupStores(driver = "mysql", table = "users") {
  useTabStore.setState({
    tabs: [
      {
        id: "query-1",
        type: "query",
        label: "db",
        meta: {
          type: "query",
          assetId: 1,
          assetName: "db",
          assetIcon: "",
          assetType: "database",
          driver,
        },
      },
    ],
    activeTabId: "query-1",
  });
  useQueryStore.setState({
    dbStates: {
      "query-1": {
        databases: ["appdb"],
        tables: { appdb: [table] },
        loadingTables: {},
        expandedDbs: ["appdb"],
        loadingDbs: false,
        innerTabs: [{ id: "table-1", type: "table", database: "appdb", table }],
        activeInnerTabId: "table-1",
        error: null,
      },
    },
  });
}

describe("TableDataTab loading cancellation", () => {
  beforeEach(() => {
    vi.mocked(ExecuteSQL).mockReset();
    vi.mocked(OpenTable).mockReset();
    setupStores();
  });

  it("does not let a stopped request overwrite the next refresh result", async () => {
    const user = userEvent.setup();
    const firstOpen = deferred<string>();
    const secondRows = deferred<string>();
    const secondCount = deferred<string>();

    vi.mocked(OpenTable).mockReturnValueOnce(firstOpen.promise);
    vi.mocked(ExecuteSQL).mockReturnValueOnce(secondRows.promise).mockReturnValueOnce(secondCount.promise);

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table="users" />);

    // 首次 OpenTable 还在 pending 时点击 stop:把当前 requestId 标记为取消。
    await user.click(screen.getByTitle("query.stopLoading"));
    firstOpen.resolve(openTablePayload({ columns: ["id", "name"], rows: [{ id: 1, name: "old" }], totalCount: 1 }));

    // 刷新走 fetchData + fetchCount 两个 ExecuteSQL(顺序与 useEffect 触发顺序相关)。
    await user.click(screen.getByTitle(/^query\.refreshTable/));
    secondRows.resolve(JSON.stringify({ columns: ["id", "name"], rows: [{ id: 2, name: "new" }] }));
    secondCount.resolve(JSON.stringify({ rows: [{ cnt: 1 }] }));

    await waitFor(() => expect(screen.getByText("new")).toBeInTheDocument());
    expect(screen.queryByText("old")).not.toBeInTheDocument();
  });

  it("keeps filter and sort controls collapsed until the toolbar button is clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(ExecuteSQL).mockResolvedValue(
      JSON.stringify({ columns: ["id", "name"], rows: [{ id: 1, name: "ada" }] })
    );

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table="users" />);

    expect(screen.queryByText("query.filterBuilderTitle")).not.toBeInTheDocument();
    expect(screen.queryByText("query.sortBuilderTitle")).not.toBeInTheDocument();

    await user.click(screen.getByTitle("query.filterSort"));

    expect(screen.getByText("query.filterBuilderTitle")).toBeInTheDocument();
    expect(screen.getByText("query.sortBuilderTitle")).toBeInTheDocument();
  });

  it("uses a default table limit of 1000 and refetches when the footer limit changes", async () => {
    const user = userEvent.setup();
    // 首屏走 OpenTable;footer 改 pageSize 后才走 ExecuteSQL 重取。
    vi.mocked(OpenTable).mockResolvedValue(
      openTablePayload({ columns: ["id", "name"], rows: [{ id: 1, name: "ada" }], totalCount: 1 })
    );
    vi.mocked(ExecuteSQL).mockResolvedValue(
      JSON.stringify({ columns: ["id", "name"], rows: [{ id: 1, name: "ada" }] })
    );

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table="users" />);

    await waitFor(() =>
      expect(vi.mocked(OpenTable).mock.calls.some(([, , , size]) => Number(size) === 1000)).toBe(true)
    );

    await user.click(screen.getByTitle("query.tableFooterSettings"));
    const limitInput = screen.getByLabelText("query.pageSize");
    await user.clear(limitInput);
    await user.type(limitInput, "250{Enter}");

    await waitFor(() =>
      expect(vi.mocked(ExecuteSQL).mock.calls.some(([, sql]) => String(sql).includes("LIMIT 250 OFFSET 0"))).toBe(true)
    );
  });

  it("forwards postgresql table identifiers to OpenTable verbatim (backend quotes)", async () => {
    setupStores("postgresql", 'audit"logs');
    // PG quote/escape 现在由 backend 的 query_svc.QuoteTableRef 处理,前端只透传 table 名。
    vi.mocked(OpenTable).mockResolvedValue(
      openTablePayload({ columns: ['id"part', "name"], rows: [{ 'id"part': 1, name: "ada" }] })
    );

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table={'audit"logs'} />);

    await waitFor(() =>
      expect(
        vi
          .mocked(OpenTable)
          .mock.calls.some(([assetId, db, table]) => assetId === 1 && db === "appdb" && table === 'audit"logs')
      ).toBe(true)
    );
  });
});
