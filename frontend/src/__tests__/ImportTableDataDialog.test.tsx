import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ImportTableDataDialog } from "@/components/query/ImportTableDataDialog";
import * as App from "../../wailsjs/go/app/App";

type MockTableImportResult = {
  processed: number;
  added: number;
  updated: number;
  deleted: number;
  error: number;
  errors: Array<{ index: number; statement?: string; message: string }>;
};

describe("ImportTableDataDialog", () => {
  function mockExecuteTableImport(
    result: MockTableImportResult = { processed: 1, added: 1, updated: 0, deleted: 0, error: 0, errors: [] }
  ) {
    const executeTableImport = vi.fn().mockResolvedValue(result);
    Object.assign(window, {
      go: {
        app: {
          App: {
            ExecuteTableImport: executeTableImport,
          },
        },
      },
    });
    return executeTableImport;
  }

  afterEach(() => {
    vi.mocked(App.ExecuteSQL).mockReset();
    delete (window as unknown as { go?: unknown }).go;
  });

  async function walkCsvWizardToMode(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByLabelText("query.importTypeCsv"));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    const file = new File(["id,name\n1,Alice"], "users.csv", { type: "text/csv" });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, file);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
  }

  async function walkCsvWizardToSummary(user: ReturnType<typeof userEvent.setup>) {
    await walkCsvWizardToMode(user);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
  }

  async function walkCsvWizardWithFileToMode(user: ReturnType<typeof userEvent.setup>, content: string) {
    await user.click(screen.getByLabelText("query.importTypeCsv"));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    const file = new File([content], "users.csv", { type: "text/csv" });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, file);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
  }

  it("shows an explicit warning when no uploaded columns map to table columns", async () => {
    const user = userEvent.setup();

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await user.click(screen.getByLabelText("query.importTypeCsv"));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    const file = new File(["external_id,full_name\n1,Alice"], "users.csv", { type: "text/csv" });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, file);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    expect(await screen.findByText("query.importNoMappedColumns")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "query.importWizardNext" })).toBeDisabled();
  });

  it("does not show enterprise-only import format placeholders", () => {
    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    expect(screen.queryByText("query.importTypeExcel")).not.toBeInTheDocument();
    expect(screen.queryByText("query.importTypeAccess")).not.toBeInTheDocument();
    expect(screen.queryByText("Ent")).not.toBeInTheDocument();
  });

  it("rejects files that do not match the selected import type", async () => {
    const user = userEvent.setup();

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await user.click(screen.getByLabelText("query.importTypeJson"));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [new File(["id,name\n1,Alice"], "users.csv", { type: "text/csv" })] },
    });

    await waitFor(() => expect(screen.queryByText("users.csv")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "query.importWizardNext" })).toBeDisabled();
  });

  it("does not expose Save Profile as a clickable no-op on the final import step", async () => {
    const user = userEvent.setup();

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await walkCsvWizardToSummary(user);

    expect(screen.getByRole("button", { name: "query.importSaveProfile" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "query.importWizardStart" })).toBeEnabled();
  });

  it("shows import mode before summary and exposes advanced settings", async () => {
    const user = userEvent.setup();

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        primaryKeys={["id"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await walkCsvWizardToMode(user);

    expect(screen.getByText("query.importModeIntro")).toBeInTheDocument();
    expect(screen.getByLabelText("query.importModeAppend")).toBeChecked();

    await user.click(screen.getByRole("button", { name: "query.importAdvancedSettings" }));

    expect(screen.getByText("query.importAdvancedTitle")).toBeInTheDocument();
    expect(screen.getByLabelText("query.importAdvancedExtendedInsert")).toBeChecked();
    expect(screen.getByLabelText("query.importAdvancedContinueOnError")).toBeChecked();

    await user.click(screen.getByRole("button", { name: "query.importAdvancedOk" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    expect(screen.getByText("query.importSummaryIntro")).toBeInTheDocument();
  });

  it("walks through JSON import wizard and starts importing mapped rows", async () => {
    const user = userEvent.setup();
    vi.mocked(App.ExecuteSQL).mockResolvedValue(JSON.stringify({ affected_rows: 1 }));

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name", "email"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await user.click(screen.getByLabelText("query.importTypeJson"));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    const file = new File([JSON.stringify([{ id: 1, name: "Alice", email: "alice@example.test" }])], "users.json", {
      type: "application/json",
    });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, file);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    expect(screen.getByText("query.importOptionsTitle")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    expect(await screen.findByText("query.importMappingIntro")).toBeInTheDocument();
    expect(screen.getAllByText("id").length).toBeGreaterThan(0);
    expect(screen.getAllByText("email").length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));

    await user.click(screen.getByRole("button", { name: "query.importWizardStart" }));

    expect(App.ExecuteSQL).toHaveBeenCalledWith(
      1,
      "INSERT INTO `appdb`.`users` (`id`, `name`, `email`) VALUES ('1', 'Alice', 'alice@example.test');",
      "appdb"
    );
  });

  it("shows import execution errors in the summary log", async () => {
    const user = userEvent.setup();
    vi.mocked(App.ExecuteSQL).mockRejectedValueOnce(new Error("Incorrect datetime value"));

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await walkCsvWizardToSummary(user);
    await user.click(screen.getByRole("button", { name: "query.importWizardStart" }));

    expect(await screen.findByText(/^\[ERR\].*Incorrect datetime value/)).toBeInTheDocument();
    expect(screen.getByText("query.importError")).toBeInTheDocument();
    expect(screen.getByText("[IMP] Processed: 1, Added: 0, Updated: 0, Deleted: 0, Errors: 1")).toBeInTheDocument();
  });

  it("keeps simple append imports cancellable between statements", async () => {
    const user = userEvent.setup();
    let cancelled = false;
    vi.mocked(App.ExecuteSQL).mockImplementation(async () => {
      cancelled = true;
      return JSON.stringify({ affected_rows: 1 });
    });

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        isSubmitCancelled={() => cancelled}
        onSuccess={vi.fn()}
      />
    );

    await walkCsvWizardWithFileToMode(user, "id,name\n1,Alice\n2,Bob");
    await user.click(screen.getByRole("button", { name: "query.importAdvancedSettings" }));
    await user.click(screen.getByLabelText("query.importAdvancedExtendedInsert"));
    await user.click(screen.getByRole("button", { name: "query.importAdvancedOk" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardStart" }));

    await waitFor(() => expect(App.ExecuteSQL).toHaveBeenCalledTimes(1));
    expect(
      (window as unknown as { go?: { app?: { App?: { ExecuteTableImport?: unknown } } } }).go?.app?.App
        ?.ExecuteTableImport
    ).toBeUndefined();
  });

  it("sends copy imports as one backend batch with foreign-key restore handled by the backend", async () => {
    const user = userEvent.setup();
    const executeTableImport = mockExecuteTableImport({
      processed: 2,
      added: 1,
      updated: 0,
      deleted: 1,
      error: 0,
      errors: [],
    });

    render(
      <ImportTableDataDialog
        open
        onOpenChange={vi.fn()}
        assetId={1}
        database="appdb"
        table="users"
        columns={["id", "name"]}
        driver="mysql"
        onSuccess={vi.fn()}
      />
    );

    await walkCsvWizardToMode(user);
    await user.click(screen.getByLabelText("query.importModeCopy"));
    await user.click(screen.getByRole("button", { name: "query.importAdvancedSettings" }));
    await user.click(screen.getByLabelText("query.importAdvancedIgnoreForeignKey"));
    await user.click(screen.getByRole("button", { name: "query.importAdvancedOk" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardNext" }));
    await user.click(screen.getByRole("button", { name: "query.importWizardStart" }));

    expect(executeTableImport).toHaveBeenCalledWith(1, "appdb", {
      statements: ["DELETE FROM `appdb`.`users`;", "INSERT INTO `appdb`.`users` (`id`, `name`) VALUES ('1', 'Alice');"],
      mode: "copy",
      continueOnError: true,
      disableForeignKeyChecks: true,
    });
    expect(App.ExecuteSQL).not.toHaveBeenCalled();
  });
});
