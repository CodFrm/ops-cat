import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { ScrollText, Square, Play } from "lucide-react";
import { StartK8sPodLogs, StopK8sPodLogs } from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { K8sSectionCard } from "./K8sSectionCard";
import { K8sLogTerminal, type K8sLogTerminalHandle } from "./K8sLogTerminal";

const MAX_LOG_CHUNKS = 2000;

export interface LogBufferState {
  container: string;
  tailLines: number;
  chunks: string[];
}

export interface LogTabState {
  logStreamID: string | null;
  logContainer: string;
  logTailLines: number;
  logError: string | null;
  currentPod?: string;
  logBuffers?: Record<string, LogBufferState>;
}

export type LogTabStateUpdate = Partial<LogTabState> | ((prev: LogTabState) => LogTabState);

export function buildLogBufferKey(podName: string, container: string, tailLines: number) {
  return `${podName}::${container}::${tailLines}`;
}

function base64ToBytes(base64: string): Uint8Array {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

interface K8sLogsPanelProps {
  assetId: number;
  containers: { name: string }[];
  namespace: string;
  podName: string;
  state: LogTabState;
  onStateChange: (update: LogTabStateUpdate) => void;
  pods?: { name: string }[];
  onSwitchPod?: (podName: string) => void;
}

export function K8sLogsPanel({
  assetId,
  containers,
  namespace,
  podName,
  state,
  onStateChange,
  pods,
  onSwitchPod,
}: K8sLogsPanelProps) {
  const { t } = useTranslation();
  const terminalRef = useRef<K8sLogTerminalHandle>(null);
  const myStreamIDRef = useRef<string | null>(null);
  const eventNamesRef = useRef<{ data: string; err: string; end: string } | null>(null);
  const onStateChangeRef = useRef(onStateChange);
  const activeContainer = state.logContainer || containers[0]?.name || "";
  // eslint-disable-next-line react-hooks/refs
  onStateChangeRef.current = onStateChange;

  const offEvents = useCallback(() => {
    const names = eventNamesRef.current;
    if (!names) return;
    EventsOff(names.data);
    EventsOff(names.err);
    EventsOff(names.end);
    eventNamesRef.current = null;
  }, []);

  const stop = useCallback(() => {
    if (myStreamIDRef.current) {
      StopK8sPodLogs(myStreamIDRef.current);
      myStreamIDRef.current = null;
    }
    offEvents();
    onStateChangeRef.current({ logStreamID: null });
  }, [offEvents]);

  const start = useCallback(() => {
    stop();
    terminalRef.current?.clear();
    const bufferKey = buildLogBufferKey(podName, activeContainer, state.logTailLines);
    onStateChangeRef.current((prev) => ({
      ...prev,
      logError: null,
      logBuffers: {
        ...(prev.logBuffers || {}),
        [bufferKey]: {
          container: activeContainer,
          tailLines: state.logTailLines,
          chunks: [],
        },
      },
    }));

    StartK8sPodLogs(assetId, namespace, podName, activeContainer, state.logTailLines)
      .then((streamID: string) => {
        myStreamIDRef.current = streamID;
        onStateChangeRef.current({ logStreamID: streamID });

        const dataEvent = "k8s:log:" + streamID;
        const errEvent = "k8s:logerr:" + streamID;
        const endEvent = "k8s:logend:" + streamID;
        eventNamesRef.current = { data: dataEvent, err: errEvent, end: endEvent };

        EventsOn(dataEvent, (data: string) => {
          if (myStreamIDRef.current !== streamID) return;
          terminalRef.current?.write(base64ToBytes(data));
          onStateChangeRef.current((prev) => {
            const existing = prev.logBuffers?.[bufferKey];
            const chunks = [...(existing?.chunks || []), data];
            const nextChunks = chunks.length > MAX_LOG_CHUNKS ? chunks.slice(chunks.length - MAX_LOG_CHUNKS) : chunks;
            return {
              ...prev,
              logBuffers: {
                ...(prev.logBuffers || {}),
                [bufferKey]: {
                  container: activeContainer,
                  tailLines: state.logTailLines,
                  chunks: nextChunks,
                },
              },
            };
          });
        });

        EventsOn(errEvent, (err: string) => {
          if (myStreamIDRef.current !== streamID) return;
          if (err === "context canceled" || err.includes("context canceled")) return;
          onStateChangeRef.current({ logError: err });
        });

        EventsOn(endEvent, () => {
          if (myStreamIDRef.current !== streamID) return;
          myStreamIDRef.current = null;
          onStateChangeRef.current({ logStreamID: null });
          offEvents();
        });
      })
      .catch((e: unknown) => {
        onStateChangeRef.current({ logError: String(e) });
      });
  }, [activeContainer, assetId, namespace, podName, state.logTailLines, stop, offEvents]);

  useEffect(() => {
    return () => {
      if (myStreamIDRef.current) {
        StopK8sPodLogs(myStreamIDRef.current);
        myStreamIDRef.current = null;
      }
      offEvents();
    };
  }, [offEvents]);

  useEffect(() => {
    if (myStreamIDRef.current) {
      StopK8sPodLogs(myStreamIDRef.current);
      myStreamIDRef.current = null;
      onStateChangeRef.current({ logStreamID: null });
    }
    terminalRef.current?.clear();
    const bufferKey = buildLogBufferKey(podName, activeContainer, state.logTailLines);
    const chunks = state.logBuffers?.[bufferKey]?.chunks || [];
    for (const chunk of chunks) {
      terminalRef.current?.write(base64ToBytes(chunk));
    }
  }, [activeContainer, podName, state.logTailLines]);

  return (
    <K8sSectionCard className="flex flex-col h-full">
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
          <ScrollText className="h-3.5 w-3.5" />
          {t("asset.k8sPodLogs")}
        </h4>
        <div className="flex items-center gap-2">
          {containers.length > 1 && (
            <select
              className="h-7 rounded-md border bg-background px-2 text-xs"
              value={activeContainer}
              onChange={(e) => {
                const container = e.target.value;
                onStateChange({ logContainer: container });
                if (state.logStreamID) {
                  stop();
                  // 注意：这里不自动 start，让用户手动点击开始
                }
              }}
              disabled={!!state.logStreamID}
            >
              {containers.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>
          )}
          <input
            type="number"
            className="h-7 w-16 rounded-md border bg-background px-2 text-xs"
            value={state.logTailLines}
            onChange={(e) => onStateChange({ logTailLines: Number(e.target.value) })}
            disabled={!!state.logStreamID}
            min={1}
            max={10000}
            title={t("asset.k8sPodLogsTailLines")}
          />
          {state.logStreamID ? (
            <button
              onClick={stop}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/50 px-3 py-1.5 text-xs text-destructive hover:bg-destructive/10"
            >
              <Square className="h-3 w-3" />
              {t("asset.k8sPodLogsStop")}
            </button>
          ) : (
            <button
              onClick={start}
              className="inline-flex items-center gap-1.5 rounded-md border border-primary/50 px-3 py-1.5 text-xs text-primary hover:bg-primary/10"
            >
              <Play className="h-3 w-3" />
              {t("asset.k8sPodLogsStart")}
            </button>
          )}
        </div>
      </div>
      {pods && pods.length > 0 && onSwitchPod && (
        <div className="flex items-center gap-2 mb-3">
          <span className="text-xs text-muted-foreground">Pod:</span>
          <select
            className="h-7 rounded-md border bg-background px-2 text-xs flex-1 min-w-0"
            value={podName}
            onChange={(e) => {
              const newPod = e.target.value;
              if (newPod !== podName) {
                stop();
                onSwitchPod(newPod);
              }
            }}
          >
            {pods.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
      )}
      {state.logError && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive mb-3">
          {t("asset.k8sPodLogsError")}: {state.logError}
        </div>
      )}
      <K8sLogTerminal ref={terminalRef} />
    </K8sSectionCard>
  );
}
