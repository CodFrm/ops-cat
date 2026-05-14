import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FileManagerPanel } from "../components/terminal/FileManagerPanel";
import { useTerminalStore, type TerminalDirectorySyncState } from "../stores/terminalStore";
import { useSFTPStore } from "../stores/sftpStore";
import { useExternalEditStore } from "../stores/externalEditStore";
import type { ExternalEditSession } from "../lib/externalEditApi";
import {
  ChangeSSHDirectory,
  PrepareExternalEditMerge,
  SFTPListDir,
} from "../../wailsjs/go/app/App";

const { toastError } = vi.hoisted(() => ({
  toastError: vi.fn(),
}));
const { codeDiffViewerMock, codeEditorMountMock } = vi.hoisted(() => ({
  codeDiffViewerMock: vi.fn(),
  codeEditorMountMock: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    error: toastError,
    success: vi.fn(),
  },
}));

vi.mock("@/components/CodeDiffViewer", () => ({
  CodeDiffViewer: (props: {
    activeBlockIndex?: number;
    modified?: string;
    navigationToken?: number;
    onDiffStatsChange?: (stats: { total: number; blocks: unknown[] }) => void;
    original?: string;
    testId?: string;
  }) => {
    codeDiffViewerMock(props);
    window.setTimeout(() => props.onDiffStatsChange?.({ total: 2, blocks: [{ id: "a" }, { id: "b" }] }), 0);
    return (
      <div data-active-block-index={props.activeBlockIndex} data-navigation-token={props.navigationToken} data-testid={props.testId}>
        externalEdit.compare.readOnly
        externalEdit.compare.remoteSnapshot
        externalEdit.compare.localDraft
        {props.original}
        {props.modified}
      </div>
    );
  },
}));

vi.mock("@/components/CodeEditor", () => ({
  CodeEditor: ({
    value,
    onChange,
    onMount,
    readOnly,
    testId,
  }: {
    value?: string;
    onChange?: (value: string) => void;
    onMount?: (editor: unknown, monaco: unknown) => void;
    readOnly?: boolean;
    testId?: string;
  }) => {
    const editor = {
      createDecorationsCollection: vi.fn((decorations: unknown[]) => ({ clear: vi.fn(), decorations })),
      revealLineInCenter: vi.fn(),
      setPosition: vi.fn(),
    };
    const monaco = {
      Range: vi.fn(function Range(this: unknown, startLine: number, startColumn: number, endLine: number, endColumn: number) {
        return { startLineNumber: startLine, startColumn, endLineNumber: endLine, endColumn };
      }),
      editor: { OverviewRulerLane: { Full: 7 } },
    };
    if (onMount) {
      codeEditorMountMock({ testId, readOnly });
      onMount(editor, monaco);
    }
    return readOnly ? (
      <pre data-testid={testId}>{value}</pre>
    ) : (
      <textarea data-testid={testId} value={value || ""} onChange={(event) => onChange?.(event.target.value)} />
    );
  },
}));

function makeSyncState(partial: Partial<TerminalDirectorySyncState> = {}): TerminalDirectorySyncState {
  return {
    sessionId: "s1",
    cwd: "/srv/app",
    cwdKnown: true,
    shell: "/bin/bash",
    shellType: "bash",
    supported: true,
    promptReady: true,
    promptClean: true,
    busy: false,
    status: "ready",
    ...partial,
  };
}

function makeExternalEditSession(partial: Partial<ExternalEditSession> & { id: string }): ExternalEditSession {
  return {
    assetId: 101,
    assetName: "asset-101",
    documentKey: "101:/srv/app/demo.txt",
    sessionId: "ssh-b",
    remotePath: "/srv/app/demo.txt",
    remoteRealPath: "/srv/app/demo.txt",
    localPath: "/tmp/opskat-sensitive/demo.txt",
    workspaceRoot: "/tmp/opskat-sensitive",
    workspaceDir: "/tmp/opskat-sensitive/demo",
    editorId: "system-text",
    editorName: "System Text Editor",
    editorPath: "/opt/sensitive/editor",
    originalSha256: "a",
    originalSize: 1,
    originalModTime: 1,
    originalEncoding: "utf-8",
    lastLocalSha256: "b",
    dirty: true,
    state: "dirty",
    hidden: false,
    expired: false,
    createdAt: 1,
    updatedAt: 10,
    lastLaunchedAt: 10,
    lastSyncedAt: 1,
    ...partial,
    id: partial.id,
  };
}

const realExternalEditPrepareMerge = useExternalEditStore.getState().prepareMerge;

describe("FileManagerPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.go ??= { app: { App: {} } };
    window.go.app ??= { App: {} };
    window.go.app.App ??= {};
    Object.assign(window.go.app.App, {
      PrepareExternalEditMerge,
    });
    useTerminalStore.setState({
      tabData: {
        tab1: {
          splitTree: { type: "terminal", sessionId: "s1" },
          activePaneId: "s1",
          panes: { s1: { sessionId: "s1", connected: true, connectedAt: Date.now() } },
          directoryFollowMode: "off",
        },
      },
      sessionSync: {
        s1: makeSyncState(),
      },
      connections: {},
      connectingAssetIds: new Set(),
    });
    useSFTPStore.setState({
      transfers: {},
      fileManagerOpenTabs: { tab1: true },
      fileManagerPaths: { tab1: "/srv/app" },
      fileManagerWidth: 280,
    });
    useExternalEditStore.setState({
      sessions: {},
      loading: false,
      savingSessionId: null,
      autoSavePhases: {},
      pendingConflict: null,
      compareResult: null,
      mergeResult: null,
      selectedError: null,
      fetchSessions: vi.fn(),
      saveSession: vi.fn(),
      refreshSession: vi.fn(),
      compareSession: vi.fn(),
      prepareMerge: realExternalEditPrepareMerge,
      applyMerge: vi.fn(),
      resolveConflict: vi.fn(),
      dismissConflict: vi.fn(),
      dismissCompare: vi.fn(),
      dismissMerge: vi.fn(),
      openErrorDetail: vi.fn((sessionId: string) => {
        useExternalEditStore.setState((state) => ({ selectedError: state.sessions[sessionId] || null }));
      }),
      dismissErrorDetail: vi.fn(),
      applyEvent: vi.fn(),
    });
    codeDiffViewerMock.mockClear();
    codeEditorMountMock.mockClear();
    vi.mocked(SFTPListDir).mockResolvedValue([]);
  });

  it("syncs the file manager to the active terminal cwd", async () => {
    const user = userEvent.setup();
    render(<FileManagerPanel tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app"));
    vi.clearAllMocks();

    useTerminalStore.getState().setSessionSyncState("s1", makeSyncState({ cwd: "/srv/releases" }));

    await user.click(screen.getByRole("button", { name: "sftp.sync.panelFromTerminal" }));

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/releases"));
    expect(useSFTPStore.getState().fileManagerPaths.tab1).toBe("/srv/releases");
  });

  it("changes the active terminal directory to the current file manager path", async () => {
    const user = userEvent.setup();
    render(<FileManagerPanel tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app"));
    vi.clearAllMocks();

    await user.click(screen.getByRole("button", { name: "sftp.sync.terminalFromPanel" }));

    expect(ChangeSSHDirectory).toHaveBeenCalledWith("s1", "/srv/app");
  });

  it("keeps panel navigation aligned with the terminal when follow mode is enabled", async () => {
    const user = userEvent.setup();
    vi.mocked(SFTPListDir)
      .mockResolvedValueOnce([{ name: "logs", isDir: true, size: 0, modTime: 0 }])
      .mockResolvedValueOnce([]);

    useTerminalStore.getState().setDirectoryFollowMode("tab1", "always");

    render(<FileManagerPanel tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(screen.getByText("logs")).toBeInTheDocument());
    vi.clearAllMocks();

    await user.dblClick(screen.getByText("logs"));

    await waitFor(() => {
      expect(ChangeSSHDirectory).toHaveBeenCalledWith("s1", "/srv/app/logs");
      expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app/logs");
    });
  });

  it("does not enable follow mode while the active pane is busy", async () => {
    const user = userEvent.setup();
    useTerminalStore.getState().setSessionSyncState("s1", makeSyncState({ busy: true, promptClean: false }));

    render(<FileManagerPanel tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "sftp.sync.followToggle" }));

    expect(toastError).toHaveBeenCalledWith("sftp.sync.busy");
    expect(useTerminalStore.getState().tabData.tab1.directoryFollowMode).toBe("off");
  });

  it("removes the bottom external edit panel and exposes only the unified pending entry", async () => {
    useExternalEditStore.setState({
      sessions: {
        draft: makeExternalEditSession({ id: "draft", state: "conflict", recordState: "conflict", updatedAt: 30 }),
        stale: makeExternalEditSession({ id: "stale", state: "stale", supersededBySessionId: "draft", updatedAt: 20 }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(await screen.findByTestId("external-edit-pending-entry")).toHaveTextContent("externalEdit.pending.entry");
    expect(screen.queryByText("externalEdit.panel.conflicts")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.panel.errors")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.panel.recoveries")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-main-draft")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-retained-drafts")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "externalEdit.actions.compare" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "externalEdit.actions.merge" })).not.toBeInTheDocument();
  });

  it("keeps clean reread drafts and retained stale drafts out of the main file manager surface", async () => {
    useExternalEditStore.setState({
      sessions: {
        stale: makeExternalEditSession({ id: "stale", state: "stale", supersededBySessionId: "snapshot", updatedAt: 30 }),
        snapshot: makeExternalEditSession({
          id: "snapshot",
          state: "clean",
          dirty: false,
          sourceSessionId: "stale",
          updatedAt: 20,
        }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app"));
    expect(screen.queryByTestId("external-edit-pending-entry")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-active-reread-draft")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-retained-drafts")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.panel.rereadBaselineHint")).not.toBeInTheDocument();
  });

  it("renders compare as a dedicated IDEA-style read-only workbench", async () => {
    useExternalEditStore.setState({
      compareResult: {
        documentKey: "101:/srv/app/demo.txt",
        primaryDraftSessionId: "draft",
        latestSnapshotSessionId: "snapshot",
        fileName: "demo.txt",
        remotePath: "/srv/app/demo.txt",
        remoteContent: "remote\n",
        localContent: "local\n",
        readOnly: true,
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    const workbench = await screen.findByTestId("external-edit-compare-workbench");
    expect(workbench).toHaveClass("fixed");
    expect(screen.queryByTestId("external-edit-compare-dialog")).not.toBeInTheDocument();
    expect(screen.getByTestId("external-edit-compare-idea-layout")).toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-compare-resize-top")).not.toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.compare.projectView")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.compare.status")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.compare.remoteLeftLocalRight")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-compare-diff-editor")).toHaveTextContent("remote");
    expect(screen.getByTestId("external-edit-compare-diff-editor")).toHaveTextContent("local");
    expect(codeDiffViewerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        activeBlockIndex: 0,
        modified: "local\n",
        navigationToken: expect.any(Number),
        original: "remote\n",
      })
    );
    await waitFor(() => expect(screen.getByTestId("external-edit-compare-diff-count")).toHaveTextContent("1 / 2"));
    expect(within(workbench).getByRole("button", { name: "externalEdit.compare.previous" })).toBeDisabled();
    expect(within(workbench).getByRole("button", { name: "externalEdit.compare.next" })).toBeEnabled();
    expect(screen.getByTestId("external-edit-compare-diff-editor")).toHaveTextContent("externalEdit.compare.readOnly");
    expect(screen.getByTestId("external-edit-compare-diff-editor")).toHaveTextContent(
      "externalEdit.compare.remoteSnapshot"
    );
    expect(screen.getByTestId("external-edit-compare-diff-editor")).toHaveTextContent("externalEdit.compare.localDraft");
    expect(screen.getByText("externalEdit.compare.helper")).toBeInTheDocument();
  });

  it("handles remote-missing recreate only from the unified pending dialog", async () => {
    const user = userEvent.setup();
    const resolveConflict = vi.fn();
    useExternalEditStore.setState({
      sessions: {
        missing: makeExternalEditSession({ id: "missing", state: "remote_missing", recordState: "conflict" }),
      },
      resolveConflict,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "externalEdit.actions.saveAgain" })).not.toBeInTheDocument();
    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.saveAgain" }));

    expect(resolveConflict).toHaveBeenCalledWith("missing", "recreate");
  });

  it("shows only merge, accept-remote, and overwrite as conflict main actions", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        conflict: makeExternalEditSession({ id: "conflict", state: "conflict", recordState: "conflict", updatedAt: 30 }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.merge" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.acceptRemote" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.compare" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.reread" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.reopenLocal" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.hideRecord" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.deleteLocal" })).not.toBeInTheDocument();
  });

  it("shows continue, reread, and overwrite for recovery pending records", async () => {
    const user = userEvent.setup();
    const continuePendingSession = vi.fn(async () => null);
    const resolveConflict = vi.fn();
    useExternalEditStore.setState({
      sessions: {
        recovery: makeExternalEditSession({
          id: "recovery",
          documentKey: "101:/srv/app/recovery.txt",
          remotePath: "/srv/app/recovery.txt",
          remoteRealPath: "/srv/app/recovery.txt",
          state: "dirty",
          resumeRequired: true,
          updatedAt: 20,
        }),
      },
      continuePendingSession,
      resolveConflict,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(within(pendingDialog).getByRole("button", { name: /继续修改|externalEdit\.actions\.continueEdit/ })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.reread" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.merge" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.reopenLocal" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.hideRecord" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.deleteLocal" })).not.toBeInTheDocument();

    await user.click(within(pendingDialog).getByRole("button", { name: /继续修改|externalEdit\.actions\.continueEdit/ }));
    await user.click(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.reread" }));
    await user.click(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.overwrite" }));

    expect(continuePendingSession).toHaveBeenCalledWith("recovery", "recovery");
    expect(resolveConflict).toHaveBeenCalledWith("recovery", "reread");
    expect(resolveConflict).toHaveBeenCalledWith("recovery", "overwrite");
  });

  it("uses a three-section pending dialog layout and keeps action buttons in a dedicated wrapping row", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        conflict: makeExternalEditSession({
          id: "conflict",
          state: "conflict",
          recordState: "conflict",
          documentKey: "101:/srv/app/projects/very/deep/path/demo-long-name.txt",
          remotePath: "/srv/app/projects/very/deep/path/with-a-very-long-segment/and-another-segment/demo-long-name.txt",
          remoteRealPath: "/srv/app/projects/very/deep/path/with-a-very-long-segment/and-another-segment/demo-long-name.txt",
          updatedAt: 30,
        }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const dialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(dialog.className).toContain("grid-rows-[auto,minmax(0,1fr),auto]");
    expect(dialog.className).toContain("gap-0");

    expect(screen.getByTestId("external-edit-pending-dialog-header")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-pending-dialog-body")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-pending-dialog-footer")).toBeInTheDocument();

    const content = screen.getByTestId("external-edit-pending-content-conflict");
    const actions = screen.getByTestId("external-edit-pending-actions-conflict");
    expect(content.className).toContain("space-y-1.5");
    expect(actions.className).toContain("w-full");
    expect(actions.className).toContain("flex-wrap");

    const file = screen.getByTestId("external-edit-pending-file-conflict");
    const path = screen.getByTestId("external-edit-pending-path-conflict");
    const summary = screen.getByTestId("external-edit-pending-summary-conflict");
    expect(file.className).toContain("break-words");
    expect(path.className).toContain("break-all");
    expect(path.className).toContain("whitespace-normal");
    expect(summary.className).toContain("whitespace-normal");

    const merge = within(actions).getByRole("button", { name: "externalEdit.actions.merge" });
    const overwrite = within(actions).getByRole("button", { name: "externalEdit.actions.overwrite" });
    expect(merge.className).toContain("!whitespace-normal");
    expect(merge.className).toContain("break-words");
    expect(overwrite.className).toContain("!whitespace-normal");

    const footerClose = within(screen.getByTestId("external-edit-pending-dialog-footer")).getByRole("button", { name: "action.close" });
    expect(footerClose).toBeInTheDocument();
  });

  it("opens a three-way editor-based merge dialog from the unified pending dialog for conflict decisions", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        conflict: makeExternalEditSession({ id: "conflict", state: "conflict", recordState: "conflict", updatedAt: 30 }),
      },
      prepareMerge: vi.fn(async () => {
        const result = {
          documentKey: "101:/srv/app/demo.txt",
          primaryDraftSessionId: "conflict",
          fileName: "demo.txt",
          remotePath: "/srv/app/demo.txt",
          localContent: "local\n",
          remoteContent: "remote\n",
          finalContent: "local\n",
          remoteHash: "remote-hash",
        };
        useExternalEditStore.setState({ mergeResult: result });
        return result;
      }),
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    await user.click(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.merge" }));

    const workbench = await screen.findByTestId("external-edit-merge-workbench");
    expect(workbench).toHaveClass("fixed");
    expect(screen.queryByTestId("external-edit-merge-dialog")).not.toBeInTheDocument();
    expect(screen.getByTestId("external-edit-merge-idea-layout")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.merge.changelist")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.merge.status")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.merge.localCenterRemote")).toBeInTheDocument();
    expect(within(workbench).getByText("externalEdit.merge.editableCenter")).toBeInTheDocument();
    expect(within(workbench).getAllByText("externalEdit.merge.readOnlySide").length).toBe(2);
    expect(screen.getByTestId("external-edit-merge-conflict-count")).toHaveTextContent("1 / 1");
    expect(within(workbench).getByRole("button", { name: "externalEdit.merge.previous" })).toBeDisabled();
    expect(within(workbench).getByRole("button", { name: "externalEdit.merge.next" })).toBeDisabled();
    expect(codeEditorMountMock).toHaveBeenCalledWith(expect.objectContaining({ testId: "external-edit-merge-local" }));
    expect(codeEditorMountMock).toHaveBeenCalledWith(expect.objectContaining({ testId: "external-edit-merge-final" }));
    expect(codeEditorMountMock).toHaveBeenCalledWith(expect.objectContaining({ testId: "external-edit-merge-remote" }));
    expect(within(workbench).getByTestId("external-edit-merge-local")).toHaveTextContent("local");
    expect(within(workbench).getByTestId("external-edit-merge-remote")).toHaveTextContent("remote");
    fireEvent.change(within(workbench).getByTestId("external-edit-merge-final"), { target: { value: "merged" } });
    await user.click(within(workbench).getByRole("button", { name: "action.cancel" }));

    expect(await screen.findByText("externalEdit.merge.closeDirtyTitle")).toBeInTheDocument();
  });

  it("removes the recovery detail dialog shell from the delivery surface", async () => {
    useExternalEditStore.setState({
      sessions: {
        recovery: makeExternalEditSession({
          id: "recovery",
          documentKey: "101:/srv/app/recovery.txt",
          remotePath: "/srv/app/recovery.txt",
          remoteRealPath: "/srv/app/recovery.txt",
          localPath: "C:\\Users\\owner\\AppData\\Local\\OpsKat\\tmp\\secret.txt",
          state: "dirty",
          saveMode: "manual_restored",
          resumeRequired: true,
        }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app"));
    expect(screen.queryByText("externalEdit.recovery.title")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.recovery.description")).not.toBeInTheDocument();
  });

  it("opens merge from a pending remote-changed conflict without a fallback list action", async () => {
    const user = userEvent.setup();
    const conflict = makeExternalEditSession({
      id: "conflict",
      documentKey: "101:/srv/app/ee68_c_conflict.txt",
      remotePath: "/srv/app/ee68_c_conflict.txt",
      remoteRealPath: "/srv/app/ee68_c_conflict.txt",
      state: "conflict",
      recordState: "conflict",
      updatedAt: 30,
    });
    const prepareMerge = vi.fn(async () => {
      const result = {
        documentKey: conflict.documentKey,
        primaryDraftSessionId: conflict.id,
        fileName: "ee68_c_conflict.txt",
        remotePath: conflict.remotePath,
        localContent: "CASE68-C-LOCAL-EDIT-1\n",
        remoteContent: "CASE68-C-REMOTE-EDIT-1\n",
        finalContent: "CASE68-C-LOCAL-EDIT-1\n",
        remoteHash: "remote-hash",
        session: conflict,
      };
      useExternalEditStore.setState({ mergeResult: result });
      return result;
    });
    useExternalEditStore.setState({
      sessions: { conflict },
      pendingConflict: {
        status: "conflict_remote_changed",
        message: "remote changed",
        session: conflict,
        conflict: { documentKey: conflict.documentKey, primaryDraftSessionId: conflict.id },
      },
      prepareMerge,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(within(pendingDialog).queryByText("externalEdit.panel.reviewInList")).not.toBeInTheDocument();
    await user.click(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.merge" }));

    expect(prepareMerge).toHaveBeenCalledWith("conflict");
    const workbench = await screen.findByTestId("external-edit-merge-workbench");
    expect(screen.queryByTestId("external-edit-merge-dialog")).not.toBeInTheDocument();
    expect(screen.getByTestId("external-edit-merge-idea-layout")).toBeInTheDocument();
    expect(within(workbench).getByTestId("external-edit-merge-local")).toHaveTextContent("CASE68-C-LOCAL-EDIT-1");
    expect(within(workbench).getByTestId("external-edit-merge-remote")).toHaveTextContent("CASE68-C-REMOTE-EDIT-1");
  });

  it("opens the merge dialog from the unified pending dialog through the real store action", async () => {
    const user = userEvent.setup();
    const conflict = makeExternalEditSession({
      id: "conflict",
      documentKey: "101:/srv/app/ee68_c_conflict.txt",
      remotePath: "/srv/app/ee68_c_conflict.txt",
      remoteRealPath: "/srv/app/ee68_c_conflict.txt",
      state: "conflict",
      recordState: "conflict",
      updatedAt: 30,
    });
    vi.mocked(PrepareExternalEditMerge).mockResolvedValueOnce({
      documentKey: conflict.documentKey,
      primaryDraftSessionId: conflict.id,
      fileName: "ee68_c_conflict.txt",
      remotePath: conflict.remotePath,
      localContent: "CASE68-C-LOCAL-EDIT-1\n",
      remoteContent: "CASE68-C-REMOTE-EDIT-1\n",
      finalContent: "CASE68-C-LOCAL-EDIT-1\n",
      remoteHash: "remote-hash",
    } as Awaited<ReturnType<typeof PrepareExternalEditMerge>>);
    useExternalEditStore.setState({ sessions: { conflict } });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.merge" }));

    expect(PrepareExternalEditMerge).toHaveBeenCalledWith("conflict");
    const workbench = await screen.findByTestId("external-edit-merge-workbench");
    expect(screen.queryByTestId("external-edit-merge-dialog")).not.toBeInTheDocument();
    expect(screen.getByTestId("external-edit-merge-idea-layout")).toBeInTheDocument();
    expect(within(workbench).getByTestId("external-edit-merge-local")).toHaveTextContent("CASE68-C-LOCAL-EDIT-1");
    expect(within(workbench).getByTestId("external-edit-merge-remote")).toHaveTextContent("CASE68-C-REMOTE-EDIT-1");
  });

  it("sanitizes prepare merge failures inside the pending dialog", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        conflict: makeExternalEditSession({ id: "conflict", state: "conflict", recordState: "conflict", updatedAt: 30 }),
      },
      prepareMerge: vi.fn(async () => {
        throw new Error("SSH session ssh-b failed at C:\\Users\\owner\\AppData\\Local\\OpsKat\\tmp\\draft.txt");
      }),
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.merge" }));

    await waitFor(() => expect(screen.getAllByText("externalEdit.error.safeActionFailed").length).toBeGreaterThan(0));
    expect(screen.queryByText(/ssh-b/)).not.toBeInTheDocument();
    expect(screen.queryByText(/AppData\\Local\\OpsKat/)).not.toBeInTheDocument();
  });

  it("sanitizes apply merge failures before showing them in the file view", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      mergeResult: {
        documentKey: "101:/srv/app/demo.txt",
        primaryDraftSessionId: "conflict",
        fileName: "demo.txt",
        remotePath: "/srv/app/demo.txt",
        localContent: "local\n",
        remoteContent: "remote\n",
        finalContent: "local\n",
        remoteHash: "remote-hash",
      },
      applyMerge: vi.fn(async () => {
        throw new Error("editor path /Applications/SecretEditor.app failed for session ssh-b");
      }),
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.saveMerge" }));

    expect(await screen.findByText("externalEdit.error.safeActionFailed")).toBeInTheDocument();
    expect(screen.queryByText(/SecretEditor/)).not.toBeInTheDocument();
    expect(screen.queryByText(/ssh-b/)).not.toBeInTheDocument();
  });

  it("sanitizes reread failures inside the pending dialog for recovery decisions", async () => {
    const user = userEvent.setup();
    const resolveConflict = vi.fn(async () => {
      throw new Error("cannot launch C:\\Tools\\SecretEditor.exe for C:\\Users\\owner\\draft.txt");
    });
    useExternalEditStore.setState({
      sessions: {
        recovery: makeExternalEditSession({
          id: "recovery",
          documentKey: "101:/srv/app/recovery.txt",
          remotePath: "/srv/app/recovery.txt",
          remoteRealPath: "/srv/app/recovery.txt",
          state: "dirty",
          saveMode: "manual_restored",
          resumeRequired: true,
        }),
      },
      resolveConflict,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.reread" }));

    expect(await screen.findByText("externalEdit.error.safeActionFailed")).toBeInTheDocument();
    expect(screen.queryByText(/SecretEditor/)).not.toBeInTheDocument();
    expect(screen.queryByText(/owner\\draft/)).not.toBeInTheDocument();
  });

  it("shows runtime conflict in the same three-action matrix", async () => {
    const conflict = makeExternalEditSession({
      id: "runtime-conflict",
      documentKey: "101:/srv/app/runtime-conflict.txt",
      remotePath: "/srv/app/runtime-conflict.txt",
      remoteRealPath: "/srv/app/runtime-conflict.txt",
      state: "conflict",
      recordState: "conflict",
      updatedAt: 30,
    });
    useExternalEditStore.setState({
      sessions: {},
      pendingConflict: {
        status: "conflict_remote_changed",
        session: conflict,
        conflict: { documentKey: conflict.documentKey, primaryDraftSessionId: conflict.id },
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.merge" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.acceptRemote" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.reread" })).not.toBeInTheDocument();
  });

  it("shows runtime non-conflict pending in the same three-action matrix", async () => {
    const user = userEvent.setup();
    const pending = makeExternalEditSession({
      id: "runtime-pending",
      documentKey: "101:/srv/app/runtime-pending.txt",
      remotePath: "/srv/app/runtime-pending.txt",
      remoteRealPath: "/srv/app/runtime-pending.txt",
      state: "dirty",
      recordState: "active",
      saveMode: "auto_live",
      pendingReview: true,
      updatedAt: 30,
    });
    const continuePendingSession = vi.fn(async () => ({ ...pending, pendingReview: false, updatedAt: 40 }));
    useExternalEditStore.setState({
      sessions: { [pending.id]: pending },
      continuePendingSession,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    await user.click(within(pendingDialog).getByRole("button", { name: /继续修改|externalEdit\.actions\.continueEdit/ }));
    expect(continuePendingSession).toHaveBeenCalledWith("runtime-pending", "runtime");
    expect(within(pendingDialog).getByRole("button", { name: /继续修改|externalEdit\.actions\.continueEdit/ })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.reread" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.merge" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.acceptRemote" })).not.toBeInTheDocument();
  });

  it("keeps remote-missing and error actions outside the unified 2/3 action matrix", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        missing: makeExternalEditSession({
          id: "missing",
          documentKey: "101:/srv/app/missing.txt",
          remotePath: "/srv/app/missing.txt",
          remoteRealPath: "/srv/app/missing.txt",
          state: "remote_missing",
          recordState: "conflict",
          updatedAt: 30,
        }),
        error: makeExternalEditSession({
          id: "error",
          documentKey: "101:/srv/app/error.txt",
          remotePath: "/srv/app/error.txt",
          remoteRealPath: "/srv/app/error.txt",
          recordState: "error",
          lastError: { step: "write_remote_file", summary: "failed", suggestion: "retry", at: 1 },
          updatedAt: 20,
        }),
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByTestId("external-edit-pending-entry"));
    const pendingDialog = await screen.findByTestId("external-edit-pending-dialog");
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.saveAgain" })).toBeInTheDocument();
    expect(within(pendingDialog).getByRole("button", { name: "externalEdit.actions.viewError" })).toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.merge" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.acceptRemote" })).not.toBeInTheDocument();
    expect(within(pendingDialog).queryByRole("button", { name: "externalEdit.actions.reread" })).not.toBeInTheDocument();
  });

  it("keeps clipboard residue hidden from file manager runtime surfaces", async () => {
    const clipboard = makeExternalEditSession({
      id: "clipboard",
      documentKey: "101:/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
      remotePath: "/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
      remoteRealPath: "/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
      localPath:
        "C:\\Users\\asus\\AppData\\Local\\com.golutra\\clipboard-images\\clipboard-6607ba08467079f385199f18c460e71b33008a531841e07a90f9b4b613629f88.png",
      workspaceDir: "C:\\Users\\asus\\AppData\\Local\\com.golutra\\clipboard-images",
      state: "conflict",
      recordState: "error",
      resumeRequired: true,
      lastError: { step: "write_remote_file", summary: "failed", suggestion: "retry", at: 1 },
      updatedAt: 50,
    });
    const active = makeExternalEditSession({
      id: "active",
      documentKey: "101:/srv/app/demo.txt",
      remotePath: "/srv/app/demo.txt",
      remoteRealPath: "/srv/app/demo.txt",
      state: "clean",
      dirty: false,
      updatedAt: 40,
    });
    const retainedClipboard = makeExternalEditSession({
      ...clipboard,
      id: "retained-clipboard",
      state: "stale",
      recordState: "conflict",
      supersededBySessionId: "active",
    });
    useExternalEditStore.setState({
      sessions: { active, clipboard, retainedClipboard },
      pendingConflict: {
        status: "conflict_remote_changed",
        message: "clipboard conflict",
        session: clipboard,
        conflict: { documentKey: clipboard.documentKey, primaryDraftSessionId: clipboard.id },
      },
      compareResult: {
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        readOnly: true,
        session: clipboard,
      },
      mergeResult: {
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        finalContent: "local",
        remoteHash: "remote-hash",
        session: clipboard,
      },
      selectedError: clipboard,
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await waitFor(() => expect(SFTPListDir).toHaveBeenCalledWith("s1", "/srv/app"));
    expect(screen.queryByText(/clipboard-d29e2e94/)).not.toBeInTheDocument();
    expect(screen.queryByText(/clipboard-images/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-pending-entry")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-pending-dialog")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-retained-drafts")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-compare-dialog")).not.toBeInTheDocument();
    expect(screen.queryByTestId("external-edit-merge-dialog")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.error.title")).not.toBeInTheDocument();
    expect(screen.queryByText("externalEdit.recovery.title")).not.toBeInTheDocument();
  });
});
