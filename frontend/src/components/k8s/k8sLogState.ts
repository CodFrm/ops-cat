export const MAX_LOG_CHUNKS = 2000;

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
