package dirsync

import "errors"

const (
	CodeMarkerOverflow    = "DIRSYNC_MARKER_OVERFLOW"
	CodeInvalidTarget     = "DIRSYNC_INVALID_TARGET"
	CodeSessionClosed     = "DIRSYNC_SESSION_CLOSED"
	CodeSessionNotFound   = "DIRSYNC_SESSION_NOT_FOUND"
	CodeTimeout           = "DIRSYNC_TIMEOUT"
	CodeUnsupported       = "DIRSYNC_UNSUPPORTED"
	CodeCwdUnknown        = "DIRSYNC_CWD_UNKNOWN"
	CodePending           = "DIRSYNC_PENDING"
	CodeBusy              = "DIRSYNC_BUSY"
	CodeNonceFailed       = "DIRSYNC_NONCE_FAILED"
	CodeProbeUnsupported  = "DIRSYNC_PROBE_UNSUPPORTED"
	CodeNotFound          = "DIRSYNC_NOT_FOUND"
	CodeNotDirectory      = "DIRSYNC_NOT_DIRECTORY"
	CodeAccessDenied      = "DIRSYNC_ACCESS_DENIED"
	CodeRemoteChdirFailed = "DIRSYNC_REMOTE_CHDIR_FAILED"
)

func Error(code string) error {
	return errors.New(code)
}
