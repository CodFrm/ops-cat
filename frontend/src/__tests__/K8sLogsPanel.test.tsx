import { forwardRef, useImperativeHandle, useState } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StartK8sPodLogs, StopK8sPodLogs } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import {
  K8sLogsPanel,
  type LogTabState,
  type LogTabStateUpdate,
} from "@/components/k8s/K8sLogsPanel";

const terminalSpies = vi.hoisted(() => ({
  clear: vi.fn(),
  write: vi.fn(),
}));

vi.mock("@/components/k8s/K8sLogTerminal", () => ({
  K8sLogTerminal: forwardRef(function MockK8sLogTerminal(_, ref) {
    useImperativeHandle(ref, () => ({
      clear: terminalSpies.clear,
      write: terminalSpies.write,
    }));
    return <div data-testid="k8s-log-terminal" />;
  }),
}));

function decodeTerminalWrite(data: string | Uint8Array) {
  if (typeof data === "string") return data;
  return new TextDecoder().decode(data);
}

function DeploymentLogPanelHarness() {
  const [podName, setPodName] = useState("pod-a");
  const [state, setState] = useState<LogTabState>({
    logStreamID: null,
    logContainer: "",
    logTailLines: 200,
    logError: null,
    currentPod: "pod-a",
    logBuffers: {},
  });

  const handleStateChange = (update: LogTabStateUpdate) => {
    setState((prev) => (typeof update === "function" ? update(prev) : { ...prev, ...update }));
  };

  return (
    <K8sLogsPanel
      assetId={7}
      namespace="default"
      podName={podName}
      containers={[{ name: "main" }]}
      pods={[{ name: "pod-a" }, { name: "pod-b" }]}
      state={state}
      onStateChange={handleStateChange}
      onSwitchPod={(nextPod) => {
        setPodName(nextPod);
        setState((prev) => ({ ...prev, currentPod: nextPod }));
      }}
    />
  );
}

describe("K8sLogsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    terminalSpies.clear.mockReset();
    terminalSpies.write.mockReset();
  });

  it("restores cached logs when switching away from a pod and back", async () => {
    const user = userEvent.setup();
    const eventHandlers = new Map<string, (payload?: string) => void>();

    vi.mocked(StartK8sPodLogs).mockResolvedValue("stream-1" as never);
    vi.mocked(StopK8sPodLogs).mockResolvedValue(undefined as never);
    vi.mocked(EventsOn).mockImplementation(((event: string, handler: (payload?: string) => void) => {
      eventHandlers.set(event, handler);
      return vi.fn();
    }) as never);

    render(<DeploymentLogPanelHarness />);

    await user.click(screen.getByRole("button", { name: /asset\.k8sPodLogsStart/i }));

    await waitFor(() => {
      expect(StartK8sPodLogs).toHaveBeenCalledWith(7, "default", "pod-a", "main", 200);
    });

    const logChunk = btoa("hello from pod-a\n");
    eventHandlers.get("k8s:log:stream-1")?.(logChunk);

    await waitFor(() => {
      expect(terminalSpies.write).toHaveBeenCalledTimes(1);
    });
    expect(decodeTerminalWrite(terminalSpies.write.mock.calls[0]![0])).toBe("hello from pod-a\n");

    await user.selectOptions(screen.getByRole("combobox"), "pod-b");

    await waitFor(() => {
      expect(StopK8sPodLogs).toHaveBeenCalledWith("stream-1");
    });
    expect(terminalSpies.clear).toHaveBeenCalled();

    await user.selectOptions(screen.getByRole("combobox"), "pod-a");

    await waitFor(() => {
      expect(terminalSpies.write).toHaveBeenCalledTimes(2);
    });
    expect(decodeTerminalWrite(terminalSpies.write.mock.calls[1]![0])).toBe("hello from pod-a\n");
  });
});
