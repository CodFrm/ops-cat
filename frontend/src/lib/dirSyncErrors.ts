export const DIRSYNC_ERROR_CODES = {
  MARKER_OVERFLOW: "DIRSYNC_MARKER_OVERFLOW",
  INVALID_TARGET: "DIRSYNC_INVALID_TARGET",
  SESSION_CLOSED: "DIRSYNC_SESSION_CLOSED",
  SESSION_NOT_FOUND: "DIRSYNC_SESSION_NOT_FOUND",
  UNSUPPORTED: "DIRSYNC_UNSUPPORTED",
  PROBE_UNSUPPORTED: "DIRSYNC_PROBE_UNSUPPORTED",
  CWD_UNKNOWN: "DIRSYNC_CWD_UNKNOWN",
  BUSY: "DIRSYNC_BUSY",
  PENDING: "DIRSYNC_PENDING",
  TIMEOUT: "DIRSYNC_TIMEOUT",
  NOT_FOUND: "DIRSYNC_NOT_FOUND",
  NOT_DIRECTORY: "DIRSYNC_NOT_DIRECTORY",
  ACCESS_DENIED: "DIRSYNC_ACCESS_DENIED",
  REMOTE_CHDIR_FAILED: "DIRSYNC_REMOTE_CHDIR_FAILED",
  NONCE_FAILED: "DIRSYNC_NONCE_FAILED",
} as const;

export type DirSyncErrorCode = (typeof DIRSYNC_ERROR_CODES)[keyof typeof DIRSYNC_ERROR_CODES];

export const DIRSYNC_ERROR_MESSAGE_KEYS = {
  [DIRSYNC_ERROR_CODES.MARKER_OVERFLOW]: "sftp.sync.markerOverflow",
  [DIRSYNC_ERROR_CODES.INVALID_TARGET]: "sftp.sync.invalidTarget",
  [DIRSYNC_ERROR_CODES.SESSION_CLOSED]: "sftp.sync.sessionClosed",
  [DIRSYNC_ERROR_CODES.SESSION_NOT_FOUND]: "sftp.sync.sessionNotFound",
  [DIRSYNC_ERROR_CODES.UNSUPPORTED]: "sftp.sync.unsupported",
  [DIRSYNC_ERROR_CODES.PROBE_UNSUPPORTED]: "sftp.sync.probeUnsupported",
  [DIRSYNC_ERROR_CODES.CWD_UNKNOWN]: "sftp.sync.cwdUnknown",
  [DIRSYNC_ERROR_CODES.BUSY]: "sftp.sync.busy",
  [DIRSYNC_ERROR_CODES.PENDING]: "sftp.sync.pending",
  [DIRSYNC_ERROR_CODES.TIMEOUT]: "sftp.sync.timeout",
  [DIRSYNC_ERROR_CODES.NOT_FOUND]: "sftp.sync.notFound",
  [DIRSYNC_ERROR_CODES.NOT_DIRECTORY]: "sftp.sync.notDirectory",
  [DIRSYNC_ERROR_CODES.ACCESS_DENIED]: "sftp.sync.accessDenied",
  [DIRSYNC_ERROR_CODES.REMOTE_CHDIR_FAILED]: "sftp.sync.remoteFailed",
  [DIRSYNC_ERROR_CODES.NONCE_FAILED]: "sftp.sync.nonceFailed",
} as const satisfies Record<DirSyncErrorCode, string>;

export type DirSyncErrorMessageKey = (typeof DIRSYNC_ERROR_MESSAGE_KEYS)[DirSyncErrorCode];

function hasDirSyncErrorCode(code: string): code is DirSyncErrorCode {
  return Object.prototype.hasOwnProperty.call(DIRSYNC_ERROR_MESSAGE_KEYS, code);
}

export function getDirSyncErrorMessageKey(error: unknown): DirSyncErrorMessageKey | null {
  const code = String(error);
  return hasDirSyncErrorCode(code) ? DIRSYNC_ERROR_MESSAGE_KEYS[code] : null;
}

export function formatDirSyncError(t: (key: DirSyncErrorMessageKey) => string, error: unknown): string {
  const key = getDirSyncErrorMessageKey(error);
  return key ? t(key) : String(error);
}
