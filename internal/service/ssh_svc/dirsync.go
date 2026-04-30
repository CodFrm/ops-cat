package ssh_svc

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/opskat/opskat/internal/pkg/dirsync"
	"go.uber.org/zap"
)

// DirectorySyncState 表示终端目录同步状态。
type DirectorySyncState struct {
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd,omitempty"`
	CwdKnown    bool   `json:"cwdKnown"`
	Shell       string `json:"shell,omitempty"`
	ShellType   string `json:"shellType,omitempty"`
	Supported   bool   `json:"supported"`
	PromptReady bool   `json:"promptReady"`
	PromptClean bool   `json:"promptClean"`
	Busy        bool   `json:"busy"`
	Status      string `json:"status"` // "initializing" | "ready" | "unsupported"
	LastError   string `json:"lastError,omitempty"`
}

const (
	shellTypeUnsupported = "unsupported"
	shellTypeBash        = "bash"
	shellTypeZsh         = "zsh"
	shellTypeKsh         = "ksh"
	shellTypeMksh        = "mksh"

	directorySyncInitializing = "initializing"
	directorySyncReady        = "ready"
	directorySyncUnsupported  = "unsupported"

	syncSequencePrefix          = "\x1b]1337;opskat:"
	syncSequenceTerm            = "\a"
	syncSequenceParserMaxBytes  = 8 * 1024
	syncSequenceTokenBytes      = 16
	directorySyncMarkerOverflow = dirsync.CodeMarkerOverflow

	dirSyncErrInvalidTarget    = dirsync.CodeInvalidTarget
	dirSyncErrSessionClosed    = dirsync.CodeSessionClosed
	dirSyncErrTimeout          = dirsync.CodeTimeout
	dirSyncErrUnsupported      = dirsync.CodeUnsupported
	dirSyncErrCwdUnknown       = dirsync.CodeCwdUnknown
	dirSyncErrPending          = dirsync.CodePending
	dirSyncErrBusy             = dirsync.CodeBusy
	dirSyncErrNonceFailed      = dirsync.CodeNonceFailed
	dirSyncErrProbeUnsupported = dirsync.CodeProbeUnsupported
)

var (
	syncProbeInterval           = 250 * time.Millisecond
	syncProbeMaxUnusableResults = 12
)

func (s *Session) GetSyncState() DirectorySyncState {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncState
}

// ChangeDirectory 在终端提示符可用时切换目录，并等待 shell 确认结果。
func (s *Session) ChangeDirectory(targetPath string) error {
	return s.ChangeDirectoryTo(targetPath, targetPath)
}

// ChangeDirectoryTo switches the terminal to targetPath and treats expectedPath
// as the canonical cwd reported by the remote shell after the change.
func (s *Session) ChangeDirectoryTo(targetPath, expectedPath string) error {
	if targetPath == "" {
		return dirsync.Error(dirSyncErrInvalidTarget)
	}
	if expectedPath == "" {
		expectedPath = targetPath
	}

	resultCh := make(chan error, 1)
	command, err := s.prepareDirectoryChange(targetPath, expectedPath, resultCh)
	if err != nil {
		return err
	}

	if err := s.writeInternal([]byte(command)); err != nil {
		s.failPendingDirectoryChange(err)
		return err
	}
	s.ensureSyncProbe()

	select {
	case result := <-resultCh:
		return result
	case <-time.After(4 * time.Second):
		s.failPendingDirectoryChange(dirsync.Error(dirSyncErrTimeout))
		return dirsync.Error(dirSyncErrTimeout)
	}
}

func (s *Session) initSyncState(shellPath, shellType string, supported bool) {
	state := DirectorySyncState{
		SessionID:   s.ID,
		Shell:       shellPath,
		ShellType:   shellType,
		Supported:   supported,
		PromptReady: false,
		PromptClean: true,
		Status:      directorySyncUnsupported,
	}
	if supported {
		state.Status = directorySyncInitializing
	}
	state.Busy = !state.PromptReady || !state.PromptClean

	s.syncMu.Lock()
	s.syncState = state
	s.syncDirty = supported
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) markUserInput(data []byte) {
	if len(data) == 0 {
		return
	}

	s.syncMu.Lock()
	if !s.syncState.Supported {
		s.syncMu.Unlock()
		return
	}

	hasNewline := bytes.ContainsAny(data, "\r\n")
	changed := false
	if s.syncState.PromptReady {
		if s.syncState.PromptClean {
			s.syncState.PromptClean = false
			changed = true
		}
		if hasNewline {
			s.syncState.PromptReady = false
			s.syncState.CwdKnown = false
			s.syncState.Cwd = ""
			s.syncState.Status = directorySyncInitializing
			s.syncDirty = true
			changed = true
		}
	}
	if changed {
		s.syncState.Busy = !s.syncState.PromptReady || !s.syncState.PromptClean
		state := s.syncState
		go s.emitSyncState(state)
	}
	s.syncMu.Unlock()
}

func (s *Session) notePrompt(cwd string) {
	s.syncMu.Lock()
	s.syncState.Cwd = strings.TrimRight(cwd, "\r\n")
	s.syncState.CwdKnown = s.syncState.Cwd != ""
	s.syncState.PromptReady = true
	s.syncState.PromptClean = true
	s.syncState.Busy = false
	s.syncState.Status = directorySyncReady
	s.syncState.LastError = ""
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) noteObservedCwd(cwd string) {
	cleaned := strings.TrimRight(cwd, "\r\n")
	if cleaned == "" {
		return
	}

	s.syncMu.Lock()
	s.syncState.Cwd = cleaned
	s.syncState.CwdKnown = true
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

func (s *Session) prepareDirectoryChange(targetPath, expectedPath string, resultCh chan error) (string, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	switch {
	case !s.syncState.Supported:
		return "", dirsync.Error(dirSyncErrUnsupported)
	case !s.syncState.CwdKnown:
		return "", dirsync.Error(dirSyncErrCwdUnknown)
	case s.pendingDirChange != nil:
		return "", dirsync.Error(dirSyncErrPending)
	case !s.syncState.PromptReady || !s.syncState.PromptClean:
		return "", dirsync.Error(dirSyncErrBusy)
	}

	nonce, err := generateSyncToken()
	if err != nil {
		return "", dirsync.Error(dirSyncErrNonceFailed)
	}
	s.pendingDirChange = resultCh
	s.pendingDirNonce = nonce
	s.pendingDirTarget = targetPath
	s.pendingDirExpected = expectedPath
	s.syncState.PromptReady = false
	s.syncState.PromptClean = false
	s.syncState.CwdKnown = false
	s.syncState.Cwd = ""
	s.syncState.Busy = true
	s.syncState.Status = directorySyncInitializing
	s.syncState.LastError = ""
	s.syncDirty = true
	state := s.syncState

	go s.emitSyncState(state)
	return buildDirectoryChangeCommand(targetPath), nil
}

func (s *Session) finishDirectoryChange(err error, cwd string) {
	s.syncMu.Lock()
	ch := s.pendingDirChange
	s.pendingDirChange = nil
	s.pendingDirNonce = ""
	s.pendingDirTarget = ""
	s.pendingDirExpected = ""
	if cwd != "" {
		s.syncState.Cwd = strings.TrimRight(cwd, "\r\n")
		s.syncState.CwdKnown = s.syncState.Cwd != ""
		s.syncState.PromptReady = true
		s.syncState.PromptClean = true
		s.syncState.Busy = false
		s.syncState.Status = directorySyncReady
	}
	if err != nil {
		s.syncState.LastError = err.Error()
	} else {
		s.syncState.LastError = ""
	}
	s.syncDirty = false
	state := s.syncState
	s.syncMu.Unlock()

	if ch != nil {
		ch <- err
		close(ch)
	}
	s.emitSyncState(state)
}

func (s *Session) failPendingDirectoryChange(err error) {
	s.syncMu.Lock()
	ch := s.pendingDirChange
	s.pendingDirChange = nil
	s.pendingDirNonce = ""
	s.pendingDirTarget = ""
	s.pendingDirExpected = ""
	if err != nil {
		s.syncState.LastError = err.Error()
	}
	state := s.syncState
	s.syncMu.Unlock()

	if ch != nil {
		ch <- err
		close(ch)
	}
	s.emitSyncState(state)
}

func (s *Session) disableDirectorySync(reason string) {
	s.syncMu.Lock()
	ch := s.pendingDirChange
	s.pendingDirChange = nil
	s.pendingDirNonce = ""
	s.pendingDirTarget = ""
	s.pendingDirExpected = ""
	s.syncProbeActive = false
	s.syncDirty = false
	s.syncState.Supported = false
	s.syncState.Cwd = ""
	s.syncState.CwdKnown = false
	s.syncState.PromptReady = false
	s.syncState.PromptClean = true
	s.syncState.Busy = false
	s.syncState.Status = directorySyncUnsupported
	s.syncState.LastError = reason
	state := s.syncState
	s.syncMu.Unlock()

	err := dirsync.Error(reason)
	if ch != nil {
		ch <- err
		close(ch)
	}
	s.emitSyncState(state)
}

func (s *Session) emitSyncState(state DirectorySyncState) {
	if s.onSync == nil {
		return
	}
	s.onSync(s.ID, state)
}

func (s *Session) noteParserOverflow() {
	s.syncMu.Lock()
	if s.syncState.LastError == directorySyncMarkerOverflow {
		s.syncMu.Unlock()
		return
	}
	s.syncState.LastError = directorySyncMarkerOverflow
	state := s.syncState
	s.syncMu.Unlock()
	s.emitSyncState(state)
}

type shellProbeResult struct {
	cwd         string
	promptReady bool
}

func (s *Session) ensureSyncProbe() {
	s.syncMu.Lock()
	if s.syncProbeActive || !s.syncState.Supported || s.shellPID <= 0 || s.shared == nil || s.shared.client == nil {
		s.syncMu.Unlock()
		return
	}
	s.syncProbeActive = true
	s.syncMu.Unlock()

	go s.runSyncProbeLoop()
}

func (s *Session) runSyncProbeLoop() {
	ticker := time.NewTicker(syncProbeInterval)
	defer ticker.Stop()

	unusableResults := 0
	for {
		if s.IsClosed() {
			s.syncMu.Lock()
			s.syncProbeActive = false
			s.syncMu.Unlock()
			return
		}

		s.syncMu.Lock()
		if !s.syncState.Supported || s.shellPID <= 0 || s.shared == nil || s.shared.client == nil {
			s.syncProbeActive = false
			s.syncMu.Unlock()
			return
		}
		shouldProbe := s.syncDirty || s.pendingDirChange != nil
		pid := s.shellPID
		pending := s.pendingDirChange != nil
		pendingNonce := s.pendingDirNonce
		pendingTarget := s.pendingDirTarget
		s.syncMu.Unlock()

		if !shouldProbe {
			s.syncMu.Lock()
			s.syncProbeActive = false
			s.syncMu.Unlock()
			return
		}

		result, err := s.probeShellState(pid)
		if err != nil || result.cwd == "" {
			unusableResults++
			if unusableResults >= syncProbeMaxUnusableResults {
				s.disableDirectorySync(dirSyncErrProbeUnsupported)
				return
			}
		} else {
			unusableResults = 0
			if pending {
				s.finishPendingDirectoryChangeProbe(pendingNonce, pendingTarget, result.cwd)
			} else if result.cwd != "" {
				s.noteObservedCwd(result.cwd)
			}
		}

		<-ticker.C
	}
}

func (s *Session) finishPendingDirectoryChangeProbe(nonce, targetPath, cwd string) {
	s.syncMu.Lock()
	if s.pendingDirChange == nil || s.pendingDirNonce == "" || s.pendingDirNonce != nonce {
		s.syncMu.Unlock()
		return
	}
	expectedPath := s.pendingDirExpected
	s.syncMu.Unlock()

	if cwd == "" {
		return
	}
	if expectedPath == "" {
		expectedPath = targetPath
	}
	if path.Clean(cwd) == path.Clean(expectedPath) {
		s.finishDirectoryChange(nil, cwd)
	}
}

func (s *Session) probeShellState(shellPID int) (shellProbeResult, error) {
	if s.probeShellStateFn != nil {
		return s.probeShellStateFn(shellPID)
	}
	session, err := s.shared.client.NewSession()
	if err != nil {
		return shellProbeResult{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && closeErr != io.EOF {
			logger.Default().Warn("close shell probe session", zap.Error(closeErr))
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = io.Discard
	if err := session.Run(buildShellStateProbeCommand(shellPID)); err != nil {
		return shellProbeResult{}, err
	}
	return parseShellProbeOutput(out.Bytes())
}

func buildShellStateProbeCommand(shellPID int) string {
	return fmt.Sprintf(`sh -lc 'pid=%d
cwd=""
prompt=0
if kill -0 "$pid" 2>/dev/null; then
  cwd=$(readlink "/proc/$pid/cwd" 2>/dev/null || printf "")
  if [ -z "$cwd" ] && command -v pwdx >/dev/null 2>&1; then
    cwd=$(pwdx "$pid" 2>/dev/null | sed "s/^[^ ]* //")
  fi
  pgid=$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d " ")
  tpgid=$(ps -o tpgid= -p "$pid" 2>/dev/null | tr -d " ")
  tty_path=$(readlink "/proc/$pid/fd/0" 2>/dev/null || printf "")
  if [ -n "$tty_path" ]; then
    stty_state=$(stty -a < "$tty_path" 2>/dev/null || printf "")
    case "$stty_state" in
      *"-icanon"*"-echo"*)
        if [ -n "$pgid" ] && [ "$pgid" = "$tpgid" ]; then
          prompt=1
        fi
        ;;
    esac
  fi
fi
printf "cwd=%%s\0prompt=%%s\0" "$cwd" "$prompt"'`, shellPID)
}

func parseShellProbeOutput(raw []byte) (shellProbeResult, error) {
	result := shellProbeResult{}
	fields := bytes.Split(raw, []byte{0})
	for _, field := range fields {
		if len(field) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(field, []byte{'='})
		if !ok {
			return shellProbeResult{}, fmt.Errorf("invalid probe field")
		}
		switch string(key) {
		case "cwd":
			result.cwd = string(value)
		case "prompt":
			result.promptReady = string(value) == "1"
		}
	}
	return result, nil
}

func (s *Session) filterOutput(chunk []byte) []byte {
	data := chunk
	if len(s.parserRemainder) > 0 {
		data = append(append([]byte(nil), s.parserRemainder...), chunk...)
		s.parserRemainder = nil
	}

	prefix := []byte(syncSequencePrefix)
	out := make([]byte, 0, len(data))

	for len(data) > 0 {
		idx := bytes.Index(data, prefix)
		if idx < 0 {
			break
		}
		out = append(out, data[:idx]...)
		remainder := data[idx+len(prefix):]
		end := bytes.IndexByte(remainder, syncSequenceTerm[0])
		if end < 0 {
			tail := append([]byte(nil), data[idx:]...)
			if len(tail) > syncSequenceParserMaxBytes {
				s.noteParserOverflow()
				out = append(out, tail...)
				return out
			}
			s.parserRemainder = tail
			return out
		}
		rawEnd := idx + len(prefix) + end + 1
		raw := data[idx:rawEnd]
		if !s.handleSyncPayload(string(remainder[:end])) {
			out = append(out, raw...)
		}
		data = data[rawEnd:]
	}

	if len(data) == 0 {
		return out
	}

	if keep := trailingPrefixLength(data, prefix); keep > 0 {
		out = append(out, data[:len(data)-keep]...)
		s.parserRemainder = append([]byte(nil), data[len(data)-keep:]...)
		return out
	}

	out = append(out, data...)
	return out
}

func trailingPrefixLength(data, prefix []byte) int {
	maxSize := len(prefix) - 1
	if maxSize > len(data) {
		maxSize = len(data)
	}
	for size := maxSize; size > 0; size-- {
		if bytes.Equal(data[len(data)-size:], prefix[:size]) {
			return size
		}
	}
	return 0
}

func (s *Session) handleSyncPayload(payload string) bool {
	token, body, ok := strings.Cut(payload, ":")
	if !ok || token == "" || token != s.syncToken {
		return false
	}

	switch {
	case strings.HasPrefix(body, "init:pid:"):
		pidText := strings.TrimPrefix(body, "init:pid:")
		pid, err := strconv.Atoi(strings.TrimSpace(pidText))
		if err != nil || pid <= 0 {
			return false
		}
		s.syncMu.Lock()
		if s.shellPID != 0 {
			s.syncMu.Unlock()
			return false
		}
		s.shellPID = pid
		s.syncDirty = true
		s.syncMu.Unlock()
		s.ensureSyncProbe()
		return true
	case strings.HasPrefix(body, "prompt:"):
		remainder := strings.TrimPrefix(body, "prompt:")
		currentNonce, nextPayload, ok := strings.Cut(remainder, ":")
		if !ok || currentNonce == "" {
			return false
		}
		nextNonce, cwd, ok := strings.Cut(nextPayload, ":")
		if !ok || nextNonce == "" {
			return false
		}
		s.syncMu.Lock()
		promptNonce := s.promptNonce
		promptPendingNonce := s.promptPendingNonce
		shellPID := s.shellPID
		s.syncMu.Unlock()
		validCurrent := currentNonce == promptNonce || (promptPendingNonce != "" && currentNonce == promptPendingNonce)
		if promptNonce == "" || !validCurrent || shellPID <= 0 {
			return false
		}
		probe, err := s.probeShellState(shellPID)
		if err != nil || !probe.promptReady {
			s.syncMu.Lock()
			if currentNonce == s.promptNonce || (s.promptPendingNonce != "" && currentNonce == s.promptPendingNonce) {
				s.promptPendingNonce = nextNonce
			}
			s.syncMu.Unlock()
			return false
		}
		resolvedCwd := probe.cwd
		if resolvedCwd == "" {
			resolvedCwd = cwd
		}
		if resolvedCwd == "" {
			return false
		}
		s.syncMu.Lock()
		if currentNonce != s.promptNonce && (s.promptPendingNonce == "" || currentNonce != s.promptPendingNonce) {
			s.syncMu.Unlock()
			return false
		}
		s.promptNonce = nextNonce
		s.promptPendingNonce = ""
		s.syncMu.Unlock()
		s.notePrompt(resolvedCwd)
		return true
	}
	return false
}
