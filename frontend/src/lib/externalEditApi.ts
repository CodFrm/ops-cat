type MaybePromise<T> = Promise<T>;

export interface ExternalEditEditorConfig {
  id: string;
  name: string;
  path: string;
  args?: string[];
}

export interface ExternalEditEditor {
  id: string;
  name: string;
  path: string;
  args?: string[];
  builtIn: boolean;
  available: boolean;
  default: boolean;
}

export interface ExternalEditSettings {
  defaultEditorId: string;
  workspaceRoot: string;
  cleanupRetentionDays: number;
  maxReadFileSizeMB: number;
  editors: ExternalEditEditor[];
  customEditors: ExternalEditEditorConfig[];
}

export interface ExternalEditSettingsInput {
  defaultEditorId: string;
  workspaceRoot: string;
  cleanupRetentionDays: number;
  maxReadFileSizeMB: number;
  customEditors: ExternalEditEditorConfig[];
}

export interface ExternalEditOpenRequest {
  assetId: number;
  sessionId: string;
  remotePath: string;
  editorId?: string;
}

export interface ExternalEditSession {
  id: string;
  assetId: number;
  assetName: string;
  documentKey: string;
  sessionId: string;
  remotePath: string;
  remoteRealPath: string;
  localPath: string;
  workspaceRoot: string;
  workspaceDir: string;
  editorId: string;
  editorName: string;
  editorPath: string;
  editorArgs?: string[];
  // `originalSha256` 保留现有 IPC 字段名，语义上等同于当前 document 的 baseHash。
  originalSha256: string;
  originalSize: number;
  originalModTime: number;
  originalEncoding: string;
  originalBom?: string;
  originalByteSample?: string;
  // `lastLocalSha256` 同样是兼容字段名，语义上等同于最近一次落盘的 localHash。
  lastLocalSha256: string;
  dirty: boolean;
  state: string;
  recordState?: "active" | "conflict" | "error" | "completed" | "abandoned";
  saveMode?: "auto_live" | "manual_restored";
  pendingReview?: boolean;
  hidden: boolean;
  expired: boolean;
  lastError?: {
    step: string;
    summary: string;
    suggestion: string;
    at: number;
  };
  resumeRequired?: boolean;
  mergeRemoteSha256?: string;
  sourceSessionId?: string;
  supersededBySessionId?: string;
  createdAt: number;
  updatedAt: number;
  lastLaunchedAt: number;
  lastSyncedAt: number;
}

export interface ExternalEditSaveResult {
  status: string;
  message?: string;
  session?: ExternalEditSession;
  conflict?: {
    documentKey: string;
    primaryDraftSessionId: string;
    latestSnapshotSessionId?: string;
  };
  automatic?: boolean;
}

export interface ExternalEditDeleteResult {
  status: string;
  message?: string;
  session?: ExternalEditSession;
}

export interface ExternalEditEvent {
  type: string;
  session?: ExternalEditSession;
  saveResult?: ExternalEditSaveResult;
  autoSave?: {
    documentKey: string;
    sessionId?: string;
    phase: "pending" | "running" | "idle";
  };
}

export interface ExternalEditCompareResult {
  documentKey: string;
  primaryDraftSessionId: string;
  latestSnapshotSessionId?: string;
  fileName: string;
  remotePath: string;
  localContent: string;
  remoteContent: string;
  readOnly: boolean;
  status?: string;
  message?: string;
  session?: ExternalEditSession;
  conflict?: {
    documentKey: string;
    primaryDraftSessionId: string;
    latestSnapshotSessionId?: string;
  };
}

export interface ExternalEditMergePrepareResult {
  documentKey: string;
  primaryDraftSessionId: string;
  fileName: string;
  remotePath: string;
  localContent: string;
  remoteContent: string;
  finalContent: string;
  remoteHash: string;
  session?: ExternalEditSession;
}

export interface ExternalEditMergeApplyRequest {
  sessionId: string;
  finalContent: string;
  remoteHash: string;
}

declare global {
  interface Window {
    go?: {
      app?: {
        App?: {
          GetExternalEditSettings?: () => MaybePromise<ExternalEditSettings>;
          SaveExternalEditSettings?: (input: ExternalEditSettingsInput) => MaybePromise<ExternalEditSettings>;
          SelectExternalEditorExecutable?: () => MaybePromise<string>;
          SelectExternalEditWorkspaceRoot?: () => MaybePromise<string>;
          OpenExternalEdit?: (req: ExternalEditOpenRequest) => MaybePromise<ExternalEditSession>;
          ListExternalEditSessions?: () => MaybePromise<ExternalEditSession[]>;
          SaveExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditSaveResult>;
          RefreshExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditSession>;
          ResolveExternalEditConflict?: (sessionId: string, resolution: string) => MaybePromise<ExternalEditSaveResult>;
          CompareExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditCompareResult>;
          PrepareExternalEditMerge?: (sessionId: string) => MaybePromise<ExternalEditMergePrepareResult>;
          ApplyExternalEditMerge?: (req: ExternalEditMergeApplyRequest) => MaybePromise<ExternalEditSaveResult>;
          RecoverExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditSession>;
          ContinueExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditSession>;
          DeleteExternalEditSession?: (
            sessionId: string,
            removeLocal: boolean
          ) => MaybePromise<ExternalEditDeleteResult>;
        };
      };
    };
  }
}

function appBindings() {
  const bindings = window.go?.app?.App;
  if (!bindings) {
    throw new Error("Wails app bindings unavailable");
  }
  return bindings;
}

// 这里只保留最薄的一层调用封装，让 store / 组件共享同一批 IPC 名称，
// 同时把 Wails 运行时缺失的报错集中在一个边界里处理。
export function getExternalEditSettings() {
  return appBindings().GetExternalEditSettings!();
}

export function saveExternalEditSettings(input: ExternalEditSettingsInput) {
  return appBindings().SaveExternalEditSettings!(input);
}

export function selectExternalEditorExecutable() {
  return appBindings().SelectExternalEditorExecutable!();
}

export function selectExternalEditWorkspaceRoot() {
  return appBindings().SelectExternalEditWorkspaceRoot!();
}

export function openExternalEdit(req: ExternalEditOpenRequest) {
  return appBindings().OpenExternalEdit!(req);
}

export function listExternalEditSessions() {
  return appBindings().ListExternalEditSessions!();
}

export function saveExternalEditSession(sessionId: string) {
  return appBindings().SaveExternalEditSession!(sessionId);
}

export function refreshExternalEditSession(sessionId: string) {
  return appBindings().RefreshExternalEditSession!(sessionId);
}

export function resolveExternalEditConflict(sessionId: string, resolution: string) {
  return appBindings().ResolveExternalEditConflict!(sessionId, resolution);
}

export function compareExternalEditSession(sessionId: string) {
  return appBindings().CompareExternalEditSession!(sessionId);
}

export function prepareExternalEditMerge(sessionId: string) {
  return appBindings().PrepareExternalEditMerge!(sessionId);
}

export function applyExternalEditMerge(req: ExternalEditMergeApplyRequest) {
  return appBindings().ApplyExternalEditMerge!(req);
}

export function recoverExternalEditSession(sessionId: string) {
  return appBindings().RecoverExternalEditSession!(sessionId);
}

export function continueExternalEditSession(sessionId: string) {
  return appBindings().ContinueExternalEditSession!(sessionId);
}

export function deleteExternalEditSession(sessionId: string, removeLocal: boolean) {
  return appBindings().DeleteExternalEditSession!(sessionId, removeLocal);
}
