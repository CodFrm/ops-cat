import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { ChangeSSHDirectory, EnableSSHSync } from "../../wailsjs/go/app/App";
import { useTerminalDirectorySync } from "@/components/terminal/file-manager/useTerminalDirectorySync";
import { useTerminalStore, type TerminalDirectorySyncState, type TerminalTabData } from "@/stores/terminalStore";

const sessionId = "ssh-1";
const tabId = "tab-1";

function buildTabData(): TerminalTabData {
  return {
    splitTree: { type: "terminal", sessionId },
    activePaneId: sessionId,
    panes: { [sessionId]: { sessionId, connected: true, connectedAt: 0 } },
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

describe("useTerminalDirectorySync — lazy enable", () => {
  beforeEach(() => {
    vi.mocked(EnableSSHSync).mockReset().mockResolvedValue(undefined);
    vi.mocked(ChangeSSHDirectory).mockReset().mockResolvedValue(undefined);
  });

  afterEach(() => {
    primeStore({});
  });

  it("calls EnableSSHSync when sessionSync is missing, then reads cwd from store", async () => {
    primeStore({});

    // Simulate the backend pushing sync state after EnableSync resolves.
    vi.mocked(EnableSSHSync).mockImplementationOnce(async () => {
      useTerminalStore.setState((state) => ({
        sessionSync: {
          ...state.sessionSync,
          [sessionId]: {
            sessionId,
            supported: true,
            cwd: "/srv/app",
            cwdKnown: true,
            shell: "/bin/bash",
            shellType: "bash",
            promptReady: true,
            promptClean: true,
            busy: false,
            status: "ready",
          },
        },
      }));
      return undefined;
    });

    const loadDir = vi.fn().mockResolvedValue(true);
    const currentPathRef = { current: "/" };

    const { result } = renderHook(() => useTerminalDirectorySync({ currentPathRef, loadDir, sessionId, tabId }));

    await act(async () => {
      await result.current.syncPanelFromTerminal();
    });

    expect(EnableSSHSync).toHaveBeenCalledWith(sessionId);
    expect(loadDir).toHaveBeenCalledWith("/srv/app");
  });

  it("does NOT call EnableSSHSync when sessionSync is already supported", async () => {
    primeStore({
      [sessionId]: {
        sessionId,
        supported: true,
        cwd: "/srv/app",
        cwdKnown: true,
        shell: "/bin/bash",
        shellType: "bash",
        promptReady: true,
        promptClean: true,
        busy: false,
        status: "ready",
      },
    });

    const loadDir = vi.fn().mockResolvedValue(true);
    const currentPathRef = { current: "/" };

    const { result } = renderHook(() => useTerminalDirectorySync({ currentPathRef, loadDir, sessionId, tabId }));

    await act(async () => {
      await result.current.syncPanelFromTerminal();
    });

    expect(EnableSSHSync).not.toHaveBeenCalled();
    expect(loadDir).toHaveBeenCalledWith("/srv/app");
  });
});
