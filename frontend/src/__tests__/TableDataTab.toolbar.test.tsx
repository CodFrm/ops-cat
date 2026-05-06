import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TableDataStatusBar, TableEditorToolbar } from "@/components/query/TableEditorToolbar";

describe("TableDataTab toolbar", () => {
  it("renders fixed edit toolbar buttons with the expected disabled states", () => {
    render(
      <TableEditorToolbar
        hasEdits={false}
        submitting={false}
        canExport
        canImport
        filterSortOpen={false}
        onToggleFilterSort={vi.fn()}
        onSubmit={vi.fn()}
        onDiscard={vi.fn()}
        onImport={vi.fn()}
        onExport={vi.fn()}
        onPreviewSql={vi.fn()}
      />
    );

    expect(screen.getByTitle("query.filterSort")).toBeEnabled();
    expect(screen.queryByTitle("query.addRow")).not.toBeInTheDocument();
    expect(screen.queryByTitle("query.deleteRecord")).not.toBeInTheDocument();
    expect(screen.getByTitle("query.submitEdits")).toBeDisabled();
    expect(screen.getByTitle("query.discardEdits")).toBeDisabled();
    expect(screen.getByTitle("query.previewSql")).toBeDisabled();
    expect(screen.queryByTitle("query.refreshTable")).not.toBeInTheDocument();
    expect(screen.queryByTitle("query.stopLoading")).not.toBeInTheDocument();
    expect(screen.getByTitle("query.importData")).toBeEnabled();
    expect(screen.getByTitle("query.exportData")).toBeEnabled();
  });

  it("enables pending-edit actions and forwards toolbar handlers", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    const onDiscard = vi.fn();
    const onPreviewSql = vi.fn();
    const onToggleFilterSort = vi.fn();

    render(
      <TableEditorToolbar
        hasEdits
        submitting={false}
        canExport
        canImport
        filterSortOpen={false}
        onToggleFilterSort={onToggleFilterSort}
        onSubmit={onSubmit}
        onDiscard={onDiscard}
        onImport={vi.fn()}
        onExport={vi.fn()}
        onPreviewSql={onPreviewSql}
      />
    );

    await user.click(screen.getByTitle("query.filterSort"));
    await user.click(screen.getByTitle("query.submitEdits"));
    await user.click(screen.getByTitle("query.discardEdits"));
    await user.click(screen.getByTitle("query.previewSql"));

    expect(onToggleFilterSort).toHaveBeenCalledOnce();
    expect(onSubmit).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
    expect(onPreviewSql).toHaveBeenCalledOnce();
  });

  it("shows the bottom status summary and keeps refresh/stop controls near pagination", async () => {
    const user = userEvent.setup();
    const onRefresh = vi.fn();
    const onAddRow = vi.fn();

    render(
      <TableDataStatusBar
        pendingEditCount={2}
        sqlSummary="UPDATE `appdb`.`users` SET `name` = 'ally' WHERE `id` = '1' LIMIT 1;"
        totalRows={12}
        page={0}
        totalPages={3}
        pageSize={1000}
        pageInput="1"
        hasPrev={false}
        hasNext
        hasSelectedRow={false}
        submitting={false}
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
        onAddRow={onAddRow}
        onDeleteRow={vi.fn()}
        onApplyChanges={vi.fn()}
        onDiscardChanges={vi.fn()}
      />
    );

    expect(
      screen.getByText("UPDATE `appdb`.`users` SET `name` = 'ally' WHERE `id` = '1' LIMIT 1;")
    ).toBeInTheDocument();
    expect(screen.getByTitle("query.stopLoading")).toBeDisabled();

    await user.click(screen.getByTitle("query.addRow"));
    await user.click(screen.getByTitle("query.refreshTable"));

    expect(onAddRow).toHaveBeenCalledOnce();
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("shows bottom apply and discard actions when edits are pending", async () => {
    const user = userEvent.setup();
    const onApplyChanges = vi.fn();
    const onDiscardChanges = vi.fn();

    render(
      <TableDataStatusBar
        pendingEditCount={1}
        sqlSummary="UPDATE `users` SET `name` = 'ally' WHERE `id` = '1' LIMIT 1;"
        totalRows={12}
        page={0}
        totalPages={3}
        pageSize={1000}
        pageInput="1"
        hasPrev={false}
        hasNext
        hasSelectedRow
        submitting={false}
        loading={false}
        refreshTitle="query.refreshTable"
        onRefresh={vi.fn()}
        onStopLoading={vi.fn()}
        onPageInputChange={vi.fn()}
        onPageInputConfirm={vi.fn()}
        onPageSizeChange={vi.fn()}
        onFirstPage={vi.fn()}
        onPreviousPage={vi.fn()}
        onNextPage={vi.fn()}
        onLastPage={vi.fn()}
        onAddRow={vi.fn()}
        onDeleteRow={vi.fn()}
        onApplyChanges={onApplyChanges}
        onDiscardChanges={onDiscardChanges}
      />
    );

    await user.click(screen.getByTitle("query.applyChanges"));
    await user.click(screen.getByTitle("query.discardChanges"));

    expect(onApplyChanges).toHaveBeenCalledOnce();
    expect(onDiscardChanges).toHaveBeenCalledOnce();
  });

  it("changes the footer page limit from the settings menu", async () => {
    const user = userEvent.setup();
    const onPageSizeChange = vi.fn();

    render(
      <TableDataStatusBar
        pendingEditCount={0}
        sqlSummary=""
        totalRows={1200}
        page={0}
        totalPages={2}
        pageSize={1000}
        pageInput="1"
        hasPrev={false}
        hasNext
        hasSelectedRow={false}
        submitting={false}
        loading={false}
        refreshTitle="query.refreshTable"
        onRefresh={vi.fn()}
        onStopLoading={vi.fn()}
        onPageInputChange={vi.fn()}
        onPageInputConfirm={vi.fn()}
        onPageSizeChange={onPageSizeChange}
        onFirstPage={vi.fn()}
        onPreviousPage={vi.fn()}
        onNextPage={vi.fn()}
        onLastPage={vi.fn()}
        onAddRow={vi.fn()}
        onDeleteRow={vi.fn()}
        onApplyChanges={vi.fn()}
        onDiscardChanges={vi.fn()}
      />
    );

    await user.click(screen.getByTitle("query.tableFooterSettings"));
    const limitInput = screen.getByLabelText("query.pageSize");
    await user.clear(limitInput);
    await user.type(limitInput, "500{Enter}");

    expect(onPageSizeChange).toHaveBeenCalledWith(500);
  });
});
