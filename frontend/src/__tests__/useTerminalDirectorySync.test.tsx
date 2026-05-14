import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { ChangeSSHDirectory, EnableSSHSync, GetSSHSyncState } from "../../wailsjs/go/app/App";
import { useTerminalDirectorySync } from "@/components/terminal/file-manager/useTerminalDirectorySync";
import { useTerminalStore, type TerminalDirectorySyncState, type TerminalTabData } from "@/stores/terminalStore";

const sessionId = "ssh-1";
const tabId = "tab-1";

function buildTabData(): TerminalTabData {
  return {
    splitTree: { type: "terminal", sessionId },
    activePaneId: sessionId,
    panes: { [sessionId]: { sessionId, transport: "ssh" as const, connected: true, connectedAt: 0 } },
    directoryFollowMode: "off",
  };
}

function primeStore(sessionSync: Record<string, TerminalDirectorySyncState>) {
  // Reset store to a known shape so each test is isolated.
  useTerminalStore.setState({
    tabData: { [tabId]: buildTabData() },
    sessionSync,
  } as never);
}

function readySyncState(cwd = "/srv/app"): TerminalDirectorySyncState {
  return {
    sessionId,
    supported: true,
    cwd,
    cwdKnown: true,
    shell: "/bin/bash",
    shellType: "bash",
    promptReady: true,
    promptClean: true,
    busy: false,
    status: "ready",
  };
}

describe("useTerminalDirectorySync — lazy enable", () => {
  beforeEach(() => {
    vi.mocked(EnableSSHSync).mockReset().mockResolvedValue(undefined);
    vi.mocked(ChangeSSHDirectory).mockReset().mockResolvedValue(undefined);
    vi.mocked(GetSSHSyncState)
      .mockReset()
      .mockResolvedValue(readySyncState() as never);
  });

  afterEach(() => {
    primeStore({});
  });

  it("calls EnableSSHSync when sessionSync is missing, then fetches latest cwd", async () => {
    primeStore({});

    const loadDir = vi.fn().mockResolvedValue(true);
    const currentPathRef = { current: "/" };

    const { result } = renderHook(() => useTerminalDirectorySync({ currentPathRef, loadDir, sessionId, tabId }));

    await act(async () => {
      await result.current.syncPanelFromTerminal();
    });

    expect(EnableSSHSync).toHaveBeenCalledWith(sessionId);
    expect(GetSSHSyncState).toHaveBeenCalledWith(sessionId);
    expect(loadDir).toHaveBeenCalledWith("/srv/app");
    expect(useTerminalStore.getState().sessionSync[sessionId]).toEqual(readySyncState());
  });

  it("does NOT call EnableSSHSync when sessionSync is already supported", async () => {
    primeStore({
      [sessionId]: readySyncState(),
    });

    const loadDir = vi.fn().mockResolvedValue(true);
    const currentPathRef = { current: "/" };

    const { result } = renderHook(() => useTerminalDirectorySync({ currentPathRef, loadDir, sessionId, tabId }));

    await act(async () => {
      await result.current.syncPanelFromTerminal();
    });

    expect(EnableSSHSync).not.toHaveBeenCalled();
    expect(GetSSHSyncState).not.toHaveBeenCalled();
    expect(loadDir).toHaveBeenCalledWith("/srv/app");
  });
});
