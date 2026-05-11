import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FileManagerPanel } from "../components/terminal/FileManagerPanel";
import { useTerminalStore, type TerminalDirectorySyncState } from "../stores/terminalStore";
import { useSFTPStore } from "../stores/sftpStore";
import { useExternalEditStore } from "../stores/externalEditStore";
import type { ExternalEditCompareResult } from "../lib/externalEditApi";
import { ChangeSSHDirectory, CompareExternalEditSession, SFTPListDir } from "../../wailsjs/go/app/App";

const { toastError } = vi.hoisted(() => ({
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    error: toastError,
    success: vi.fn(),
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

describe("FileManagerPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
      selectedError: null,
      fetchSessions: vi.fn(),
      saveSession: vi.fn(),
      refreshSession: vi.fn(),
      compareSession: vi.fn(),
      deleteSession: vi.fn(),
      resolveConflict: vi.fn(),
      dismissConflict: vi.fn(),
      dismissCompare: vi.fn(),
      openErrorDetail: vi.fn(),
      dismissErrorDetail: vi.fn(),
      applyEvent: vi.fn(),
    });
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
      .mockResolvedValueOnce([
        {
          name: "logs",
          isDir: true,
          size: 0,
          modTime: 0,
        },
      ])
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

  it("renders a document-level conflict row with visible disabled actions", async () => {
    useExternalEditStore.setState({
      sessions: {
        draft: {
          id: "draft",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "a",
          originalSize: 1,
          originalModTime: 1,
          originalEncoding: "utf-8",
          lastLocalSha256: "b",
          dirty: true,
          state: "conflict",
          hidden: false,
          expired: false,
          createdAt: 1,
          updatedAt: 10,
          lastLaunchedAt: 10,
          lastSyncedAt: 1,
        },
        stale: {
          id: "stale",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo-old.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo-old",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "a",
          originalSize: 1,
          originalModTime: 1,
          originalEncoding: "utf-8",
          lastLocalSha256: "b",
          dirty: true,
          state: "stale",
          hidden: false,
          expired: false,
          createdAt: 1,
          updatedAt: 30,
          lastLaunchedAt: 30,
          lastSyncedAt: 1,
        },
      },
      savingSessionId: "draft",
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(await screen.findByText("externalEdit.panel.conflicts")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-main-draft")).toHaveTextContent("externalEdit.panel.currentDraft");
    expect(screen.getByTestId("external-edit-retained-drafts")).toHaveTextContent("externalEdit.panel.retainedDraft");
    expect(screen.getByRole("button", { name: "externalEdit.actions.compare" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "externalEdit.actions.reread" })).toBeDisabled();
    expect(screen.getByText("externalEdit.panel.rereadBaselineHint")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeDisabled();
  });

  it("shows reread drafts as the main document while keeping the old draft discoverable", async () => {
    useExternalEditStore.setState({
      sessions: {
        stale: {
          id: "stale",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo-old.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo-old",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "a",
          originalSize: 1,
          originalModTime: 1,
          originalEncoding: "utf-8",
          lastLocalSha256: "b",
          dirty: true,
          state: "stale",
          hidden: false,
          expired: false,
          supersededBySessionId: "snapshot",
          createdAt: 1,
          updatedAt: 30,
          lastLaunchedAt: 30,
          lastSyncedAt: 1,
        },
        snapshot: {
          id: "snapshot",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-c",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo-new.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo-new",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "c",
          originalSize: 1,
          originalModTime: 2,
          originalEncoding: "utf-8",
          lastLocalSha256: "c",
          dirty: false,
          state: "clean",
          hidden: false,
          expired: false,
          sourceSessionId: "stale",
          createdAt: 2,
          updatedAt: 20,
          lastLaunchedAt: 20,
          lastSyncedAt: 20,
        },
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect((await screen.findAllByText("externalEdit.panel.rereadDraft")).length).toBeGreaterThan(0);
    expect(screen.getByText("externalEdit.panel.conflicts")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-main-draft")).toHaveTextContent("externalEdit.panel.rereadDraft");
    expect(screen.getByTestId("external-edit-retained-drafts")).toHaveTextContent("externalEdit.panel.retainedDraft");
    expect(screen.getByTestId("external-edit-retained-drafts")).toHaveTextContent("externalEdit.state.stale");
    expect(screen.getByText((content) => content.includes("externalEdit.panel.rereadBaselineHint"))).toBeInTheDocument();
  });

  it("keeps compare remote-missing results in the external edit business panel instead of the file-list error", async () => {
    const user = userEvent.setup();
    useExternalEditStore.setState({
      sessions: {
        draft: {
          id: "draft",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "a",
          originalSize: 1,
          originalModTime: 1,
          originalEncoding: "utf-8",
          lastLocalSha256: "b",
          dirty: true,
          state: "conflict",
          hidden: false,
          expired: false,
          createdAt: 1,
          updatedAt: 10,
          lastLaunchedAt: 10,
          lastSyncedAt: 1,
        },
      },
      compareSession: vi.fn(async (sessionId: string) => {
        const result = (await CompareExternalEditSession(sessionId)) as ExternalEditCompareResult;
        useExternalEditStore.setState((state) => ({
          sessions: result.session ? { ...state.sessions, [result.session.id]: result.session } : state.sessions,
          compareResult: result.status === "remote_missing" ? null : result,
          pendingConflict:
            result.status === "remote_missing"
              ? {
                  status: "remote_missing",
                  message: result.message,
                  session: result.session,
                  conflict: result.conflict,
                  automatic: false,
                }
              : state.pendingConflict,
        }));
        return result;
      }),
    });
    vi.mocked(CompareExternalEditSession).mockResolvedValueOnce({
      documentKey: "101:/srv/app/demo.txt",
      primaryDraftSessionId: "draft",
      fileName: "demo.txt",
      remotePath: "/srv/app/demo.txt",
      localContent: "",
      remoteContent: "",
      readOnly: true,
      status: "remote_missing",
      message: "remote missing",
      session: {
        ...useExternalEditStore.getState().sessions.draft,
        state: "remote_missing",
      },
      conflict: {
        documentKey: "101:/srv/app/demo.txt",
        primaryDraftSessionId: "draft",
      },
    } as Awaited<ReturnType<typeof CompareExternalEditSession>>);

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "externalEdit.actions.compare" }));

    expect(await screen.findByRole("alertdialog", { name: "externalEdit.conflict.remoteMissingTitle" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "externalEdit.panel.reviewInList" })).toBeEnabled();
    expect(await screen.findByText("externalEdit.panel.recreateReadyHint")).toBeInTheDocument();
    expect(screen.queryByText("sftp.loadError")).not.toBeInTheDocument();
    expect(useExternalEditStore.getState().compareResult).toBeNull();
  });

  it("renders the compare dialog as a large read-only dual-pane diff", async () => {
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

    expect(await screen.findByText("externalEdit.compare.title")).toBeInTheDocument();
    expect(screen.getByTestId("external-edit-compare-dialog")).toHaveStyle({
      width: "min(98vw, 1680px)",
      height: "min(96vh, 920px)",
    });
    for (const edge of ["top", "right", "bottom", "left", "top-left", "top-right", "bottom-right", "bottom-left"]) {
      expect(screen.getByTestId(`external-edit-compare-resize-${edge}`)).toBeInTheDocument();
    }
    expect(screen.getAllByText("externalEdit.compare.readOnly").length).toBeGreaterThan(0);
    expect(screen.getAllByText("externalEdit.compare.remoteSnapshot").length).toBeGreaterThan(0);
    expect(screen.getAllByText("externalEdit.compare.localDraft").length).toBeGreaterThan(0);
    expect(screen.getByText("externalEdit.compare.helper")).toBeInTheDocument();
  });

  it("only shows recreate for actionable remote-missing drafts", async () => {
    useExternalEditStore.setState({
      sessions: {
        missing: {
          id: "missing",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
          originalSha256: "a",
          originalSize: 1,
          originalModTime: 1,
          originalEncoding: "utf-8",
          lastLocalSha256: "b",
          dirty: true,
          state: "remote_missing",
          hidden: false,
          expired: false,
          createdAt: 1,
          updatedAt: 10,
          lastLaunchedAt: 10,
          lastSyncedAt: 1,
        },
      },
    });

    const { rerender } = render(
      <FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />
    );

    expect(await screen.findByRole("button", { name: "externalEdit.actions.saveAgain" })).toBeEnabled();
    expect(screen.getByText("externalEdit.panel.recreateReadyHint")).toBeInTheDocument();

    useExternalEditStore.setState({
      savingSessionId: "missing",
    });
    rerender(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "externalEdit.actions.saveAgain" })).not.toBeInTheDocument();
    expect(screen.getByText("externalEdit.panel.recreateUnavailableBusy")).toBeInTheDocument();
  });

  it("shows auto-save pending feedback for active external edit drafts", async () => {
    useExternalEditStore.setState({
      sessions: {
        draft: {
          id: "draft",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
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
        },
      },
      autoSavePhases: {
        "101:/srv/app/demo.txt": "pending",
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(await screen.findByTestId("external-edit-autosave-phase")).toHaveTextContent(
      "externalEdit.panel.autoSavePending"
    );
  });

  it("disables manual sync while auto-save is running", async () => {
    useExternalEditStore.setState({
      sessions: {
        draft: {
          id: "draft",
          assetId: 101,
          assetName: "asset-101",
          documentKey: "101:/srv/app/demo.txt",
          sessionId: "ssh-b",
          remotePath: "/srv/app/demo.txt",
          remoteRealPath: "/srv/app/demo.txt",
          localPath: "/tmp/demo.txt",
          workspaceRoot: "/tmp",
          workspaceDir: "/tmp/demo",
          editorId: "system-text",
          editorName: "System Text Editor",
          editorPath: "/bin/editor",
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
        },
      },
      autoSavePhases: {
        "101:/srv/app/demo.txt": "running",
      },
    });

    render(<FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />);

    expect(await screen.findByTestId("external-edit-autosave-phase")).toHaveTextContent(
      "externalEdit.panel.autoSaveRunning"
    );
    expect(screen.getByRole("button", { name: "externalEdit.actions.sync" })).toBeDisabled();
  });
});
