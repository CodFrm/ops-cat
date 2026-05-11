import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FileManagerPanel } from "../components/terminal/FileManagerPanel";
import { useTerminalStore, type TerminalDirectorySyncState } from "../stores/terminalStore";
import { useSFTPStore } from "../stores/sftpStore";
import { useExternalEditStore } from "../stores/externalEditStore";
import { ChangeSSHDirectory, SFTPListDir } from "../../wailsjs/go/app/App";

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
      pendingConflict: null,
      compareResult: null,
      fetchSessions: vi.fn(),
      saveSession: vi.fn(),
      refreshSession: vi.fn(),
      compareSession: vi.fn(),
      resolveConflict: vi.fn(),
      dismissConflict: vi.fn(),
      dismissCompare: vi.fn(),
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
          sourceSessionId: "draft",
          createdAt: 2,
          updatedAt: 20,
          lastLaunchedAt: 20,
          lastSyncedAt: 20,
        },
      },
      savingSessionId: "draft",
    });

    render(
      <FileManagerPanel assetId={101} tabId="tab1" sessionId="s1" isOpen width={280} onWidthChange={vi.fn()} />
    );

    expect(await screen.findByText("externalEdit.panel.conflicts")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "externalEdit.actions.compare" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "externalEdit.actions.reread" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "externalEdit.actions.overwrite" })).toBeDisabled();
  });
});
