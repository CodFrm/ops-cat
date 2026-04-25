import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TableDataStatusBar, TableEditorToolbar } from "@/components/query/TableEditorToolbar";

describe("TableDataTab toolbar", () => {
  it("renders fixed edit toolbar buttons with the expected disabled states", () => {
    render(
      <TableEditorToolbar
        hasEdits={false}
        hasSelectedRow={false}
        loading={false}
        submitting={false}
        canExport
        canImport
        onAddRow={vi.fn()}
        onDeleteRow={vi.fn()}
        onSubmit={vi.fn()}
        onDiscard={vi.fn()}
        onRefresh={vi.fn()}
        onStopLoading={vi.fn()}
        onImport={vi.fn()}
        onExport={vi.fn()}
        onPreviewSql={vi.fn()}
      />
    );

    expect(screen.getByTitle("query.addRow")).toBeEnabled();
    expect(screen.getByTitle("query.deleteRecord")).toBeDisabled();
    expect(screen.getByTitle("query.submitEdits")).toBeDisabled();
    expect(screen.getByTitle("query.discardEdits")).toBeDisabled();
    expect(screen.getByTitle("query.previewSql")).toBeDisabled();
    expect(screen.getByTitle("query.refreshTable")).toBeEnabled();
    expect(screen.getByTitle("query.stopLoading")).toBeDisabled();
    expect(screen.getByTitle("query.importData")).toBeEnabled();
    expect(screen.getByTitle("query.exportData")).toBeEnabled();
  });

  it("enables selected-row and pending-edit actions and forwards toolbar handlers", async () => {
    const user = userEvent.setup();
    const onDeleteRow = vi.fn();
    const onSubmit = vi.fn();
    const onDiscard = vi.fn();
    const onPreviewSql = vi.fn();

    render(
      <TableEditorToolbar
        hasEdits
        hasSelectedRow
        loading={false}
        submitting={false}
        canExport
        canImport
        onAddRow={vi.fn()}
        onDeleteRow={onDeleteRow}
        onSubmit={onSubmit}
        onDiscard={onDiscard}
        onRefresh={vi.fn()}
        onStopLoading={vi.fn()}
        onImport={vi.fn()}
        onExport={vi.fn()}
        onPreviewSql={onPreviewSql}
      />
    );

    await user.click(screen.getByTitle("query.deleteRecord"));
    await user.click(screen.getByTitle("query.submitEdits"));
    await user.click(screen.getByTitle("query.discardEdits"));
    await user.click(screen.getByTitle("query.previewSql"));

    expect(onDeleteRow).toHaveBeenCalledOnce();
    expect(onSubmit).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
    expect(onPreviewSql).toHaveBeenCalledOnce();
  });

  it("shows the bottom status summary and keeps refresh/stop controls near pagination", async () => {
    const user = userEvent.setup();
    const onRefresh = vi.fn();

    render(
      <TableDataStatusBar
        pendingEditCount={2}
        sqlSummary="UPDATE `appdb`.`users` SET `name` = 'ally' WHERE `id` = '1' LIMIT 1;"
        totalRows={12}
        page={0}
        totalPages={3}
        pageInput="1"
        pageSize={100}
        pageSizes={[50, 100]}
        hasPrev={false}
        hasNext
        loading={false}
        refreshTitle="query.refreshTable"
        onRefresh={onRefresh}
        onStopLoading={vi.fn()}
        onPageInputChange={vi.fn()}
        onPageInputConfirm={vi.fn()}
        onPageSizeChange={vi.fn()}
        onFirstPage={vi.fn()}
        onPreviousPage={vi.fn()}
        onNextPage={vi.fn()}
        onLastPage={vi.fn()}
      />
    );

    expect(screen.getByText("query.pendingEdits")).toBeInTheDocument();
    expect(
      screen.getByText("UPDATE `appdb`.`users` SET `name` = 'ally' WHERE `id` = '1' LIMIT 1;")
    ).toBeInTheDocument();
    expect(screen.getByTitle("query.stopLoading")).toBeDisabled();

    await user.click(screen.getByTitle("query.refreshTable"));

    expect(onRefresh).toHaveBeenCalledOnce();
  });
});
