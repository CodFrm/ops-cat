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
  editors: ExternalEditEditor[];
  customEditors: ExternalEditEditorConfig[];
}

export interface ExternalEditSettingsInput {
  defaultEditorId: string;
  workspaceRoot: string;
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
  originalSha256: string;
  originalSize: number;
  originalModTime: number;
  originalEncoding: string;
  originalBom?: string;
  originalByteSample?: string;
  lastLocalSha256: string;
  dirty: boolean;
  state: string;
  expired: boolean;
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

export interface ExternalEditEvent {
  type: string;
  session?: ExternalEditSession;
  saveResult?: ExternalEditSaveResult;
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
          ResolveExternalEditConflict?: (sessionId: string, resolution: string) => MaybePromise<ExternalEditSaveResult>;
          CompareExternalEditSession?: (sessionId: string) => MaybePromise<ExternalEditCompareResult>;
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

export function resolveExternalEditConflict(sessionId: string, resolution: string) {
  return appBindings().ResolveExternalEditConflict!(sessionId, resolution);
}

export function compareExternalEditSession(sessionId: string) {
  return appBindings().CompareExternalEditSession!(sessionId);
}
