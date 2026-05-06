import { describe, expect, it, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TableDataTab } from "@/components/query/TableDataTab";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore } from "@/stores/tabStore";
import { ExecuteSQL } from "../../wailsjs/go/app/App";

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
    setupStores();
  });

  it("does not let a stopped request overwrite the next refresh result", async () => {
    const user = userEvent.setup();
    const firstPk = deferred<string>();
    const firstColumns = deferred<string>();
    const firstCount = deferred<string>();
    const firstRows = deferred<string>();
    const secondCount = deferred<string>();
    const secondRows = deferred<string>();

    vi.mocked(ExecuteSQL)
      .mockReturnValueOnce(firstPk.promise)
      .mockReturnValueOnce(firstColumns.promise)
      .mockReturnValueOnce(firstCount.promise)
      .mockReturnValueOnce(firstRows.promise)
      .mockReturnValueOnce(secondRows.promise)
      .mockReturnValueOnce(secondCount.promise);

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table="users" />);

    await user.click(screen.getByTitle("query.stopLoading"));
    firstPk.resolve(JSON.stringify({ rows: [] }));
    firstColumns.resolve(JSON.stringify({ rows: [] }));
    firstCount.resolve(JSON.stringify({ rows: [{ cnt: 1 }] }));
    firstRows.resolve(JSON.stringify({ columns: ["id", "name"], rows: [{ id: 1, name: "old" }] }));

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
    vi.mocked(ExecuteSQL).mockResolvedValue(
      JSON.stringify({ columns: ["id", "name"], rows: [{ id: 1, name: "ada" }] })
    );

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table="users" />);

    await waitFor(() =>
      expect(vi.mocked(ExecuteSQL).mock.calls.some(([, sql]) => String(sql).includes("LIMIT 1000 OFFSET 0"))).toBe(true)
    );

    await user.click(screen.getByTitle("query.tableFooterSettings"));
    const limitInput = screen.getByLabelText("query.pageSize");
    await user.clear(limitInput);
    await user.type(limitInput, "250{Enter}");

    await waitFor(() =>
      expect(vi.mocked(ExecuteSQL).mock.calls.some(([, sql]) => String(sql).includes("LIMIT 250 OFFSET 0"))).toBe(true)
    );
  });

  it("escapes postgresql table identifiers when loading data", async () => {
    setupStores("postgresql", 'audit"logs');
    vi.mocked(ExecuteSQL).mockResolvedValue(
      JSON.stringify({ columns: ['id"part', "name"], rows: [{ 'id"part': 1, name: "ada" }] })
    );

    render(<TableDataTab tabId="query-1" innerTabId="table-1" database="appdb" table={'audit"logs'} />);

    await waitFor(() =>
      expect(
        vi
          .mocked(ExecuteSQL)
          .mock.calls.some(([, sql]) => String(sql).includes(`SELECT * FROM "audit""logs" LIMIT 1000 OFFSET 0`))
      ).toBe(true)
    );
  });
});
