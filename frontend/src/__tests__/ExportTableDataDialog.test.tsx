import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ExportTableDataDialog } from "@/components/query/ExportTableDataDialog";
import * as App from "../../wailsjs/go/app/App";

const baseProps = {
  open: true,
  onOpenChange: vi.fn(),
  assetId: 1,
  database: "appdb",
  table: "users",
  driver: "mysql",
  columns: ["id", "name"],
  rows: [{ id: 1, name: "Alice" }],
  totalRows: 1,
  page: 0,
  pageSize: 50,
  whereClause: "",
  orderByClause: "",
  sortColumn: null,
  sortDir: null,
  initialFormat: "csv" as const,
  onFormatChange: vi.fn(),
};

describe("ExportTableDataDialog", () => {
  afterEach(() => {
    vi.mocked(App.SelectTableExportFile).mockReset();
    vi.mocked(App.ExecuteSQL).mockReset();
    vi.mocked(App.OpenDirectory).mockReset();
  });

  it("opens the exported file or its containing folder after a successful export", async () => {
    const user = userEvent.setup();
    vi.mocked(App.SelectTableExportFile).mockResolvedValue("/tmp/opskat/users.csv");
    const writeFile = vi.fn().mockResolvedValue(undefined);
    Object.assign(window, {
      go: {
        app: {
          App: {
            WriteTableExportFile: writeFile,
          },
        },
      },
    });

    render(<ExportTableDataDialog {...baseProps} />);

    await user.click(screen.getByRole("button", { name: /query.exportChooseFile/ }));
    await user.click(screen.getByRole("button", { name: /query.exportStart/ }));

    await waitFor(() => expect(writeFile).toHaveBeenCalled());
    await user.click(await screen.findByRole("button", { name: /query.openExport/ }));
    await user.click(await screen.findByText("query.openExportFile"));
    expect(App.OpenDirectory).toHaveBeenCalledWith("/tmp/opskat/users.csv");

    await user.click(screen.getByRole("button", { name: /query.openExport/ }));
    await user.click(await screen.findByText("query.openExportFolder"));
    expect(App.OpenDirectory).toHaveBeenCalledWith("/tmp/opskat");
  });
});
