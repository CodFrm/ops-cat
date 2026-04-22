import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DatabaseTree } from "../components/query/DatabaseTree";
import { useQueryStore } from "../stores/queryStore";
import { useTabStore } from "../stores/tabStore";
import { ExecuteSQL } from "../../wailsjs/go/app/App";

function makeDatabaseTab(id = "query-1"): void {
  useTabStore.setState({
    tabs: [
      {
        id,
        type: "query",
        label: "test",
        meta: {
          type: "query",
          assetId: 1,
          assetName: "test-db",
          assetIcon: "",
          assetType: "database",
          driver: "mysql",
          defaultDatabase: "appdb",
        },
      },
    ],
    activeTabId: id,
  });
}

describe("DatabaseTree", () => {
  beforeEach(() => {
    useQueryStore.setState({
      dbStates: {
        "query-1": {
          databases: ["appdb"],
          tables: {},
          expandedDbs: new Set(),
          loadingDbs: false,
          innerTabs: [],
          activeInnerTabId: null,
          error: null,
        },
      },
      redisStates: {},
      mongoStates: {},
    });
    makeDatabaseTab();
    vi.mocked(ExecuteSQL).mockResolvedValue(JSON.stringify({ rows: [] }));
  });

  it("opens create database dialog from toolbar", async () => {
    render(<DatabaseTree tabId="query-1" />);

    fireEvent.click(screen.getByLabelText("query.createDatabase"));
    expect(screen.getByText("query.createDatabaseDialogTitle")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("query.databaseNamePlaceholder"), {
      target: { value: "reports" },
    });
    fireEvent.change(screen.getByPlaceholderText("query.charsetPlaceholder"), {
      target: { value: "utf8mb4" },
    });
    fireEvent.change(screen.getByPlaceholderText("query.collationPlaceholder"), {
      target: { value: "utf8mb4_0900_ai_ci" },
    });
    fireEvent.click(screen.getByText("query.createDatabaseSubmit"));

    await waitFor(() => {
      expect(ExecuteSQL).toHaveBeenCalledWith(1, "CREATE DATABASE `reports` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", "appdb");
    });
  });
});
