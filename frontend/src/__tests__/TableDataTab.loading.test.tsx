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

function setupStores() {
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
          driver: "mysql",
        },
      },
    ],
    activeTabId: "query-1",
  });
  useQueryStore.setState({
    dbStates: {
      "query-1": {
        databases: ["appdb"],
        tables: { appdb: ["users"] },
        expandedDbs: ["appdb"],
        loadingDbs: false,
        innerTabs: [{ id: "table-1", type: "table", database: "appdb", table: "users" }],
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

    await user.click(screen.getAllByTitle("query.stopLoading")[0]);
    firstPk.resolve(JSON.stringify({ rows: [] }));
    firstColumns.resolve(JSON.stringify({ rows: [] }));
    firstCount.resolve(JSON.stringify({ rows: [{ cnt: 1 }] }));
    firstRows.resolve(JSON.stringify({ columns: ["id", "name"], rows: [{ id: 1, name: "old" }] }));

    await user.click(screen.getAllByTitle("query.refreshTable")[0]);
    secondRows.resolve(JSON.stringify({ columns: ["id", "name"], rows: [{ id: 2, name: "new" }] }));
    secondCount.resolve(JSON.stringify({ rows: [{ cnt: 1 }] }));

    await waitFor(() => expect(screen.getByText("new")).toBeInTheDocument());
    expect(screen.queryByText("old")).not.toBeInTheDocument();
  });
});
