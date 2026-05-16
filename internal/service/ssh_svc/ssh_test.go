package ssh_svc

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/pkg/dirsync"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

func newTestSyncSession() *Session {
	sess := &Session{syncToken: "real-token"}
	sess.initSyncState("/bin/bash", shellTypeBash, true)
	return sess
}

func buildTestSyncSequence(token, payload string) []byte {
	return []byte(syncSequencePrefix + token + ":" + payload + syncSequenceTerm)
}

type recordingWriteCloser struct {
	mu      sync.Mutex
	writes  [][]byte
	err     error
	onWrite func([]byte)
}

func (w *recordingWriteCloser) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	copied := append([]byte(nil), data...)
	w.mu.Lock()
	w.writes = append(w.writes, copied)
	w.mu.Unlock()
	if w.onWrite != nil {
		w.onWrite(copied)
	}
	return len(data), nil
}

func (w *recordingWriteCloser) Close() error {
	return nil
}

func (w *recordingWriteCloser) writeCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.writes)
}

func (w *recordingWriteCloser) lastWrite() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.writes) == 0 {
		return nil
	}
	return append([]byte(nil), w.writes[len(w.writes)-1]...)
}

func extractInitTokenFromEnableCommand(t *testing.T, cmd []byte) string {
	t.Helper()
	const marker = "1337;opskat:"
	text := string(cmd)
	idx := strings.LastIndex(text, marker)
	if idx < 0 {
		t.Fatalf("enable command missing init marker: %q", text)
	}
	remainder := text[idx+len(marker):]
	token, _, ok := strings.Cut(remainder, ":init:pid:")
	if !ok || token == "" {
		t.Fatalf("enable command missing init token: %q", text)
	}
	return token
}

func TestManager_Basic(t *testing.T) {
	convey.Convey("SSH Manager 基础功能", t, func() {
		m := NewManager()

		convey.Convey("新创建的 Manager 无活跃会话", func() {
			assert.Equal(t, 0, m.ActiveSessions())
		})

		convey.Convey("获取不存在的会话返回 false", func() {
			_, ok := m.GetSession("nonexistent")
			assert.False(t, ok)
		})

		convey.Convey("断开不存在的会话不 panic", func() {
			assert.NotPanics(t, func() {
				m.Disconnect("nonexistent")
			})
		})

		convey.Convey("DisconnectAll 空管理器不 panic", func() {
			assert.NotPanics(t, func() {
				m.DisconnectAll()
			})
		})
	})
}

func TestManager_ConnectInvalidAuth(t *testing.T) {
	convey.Convey("SSH 连接无效参数", t, func() {
		m := NewManager()

		convey.Convey("不支持的认证方式返回错误", func() {
			_, err := m.Connect(ConnectConfig{
				Host:     "127.0.0.1",
				Port:     22,
				Username: "root",
				AuthType: "unsupported",
				OnData:   func(string, []byte) {},
				OnClosed: func(string) {},
			})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "不支持的认证方式")
		})

		convey.Convey("无效密钥返回错误", func() {
			_, err := m.Connect(ConnectConfig{
				Host:     "127.0.0.1",
				Port:     22,
				Username: "root",
				AuthType: "key",
				Key:      "invalid-key-content",
				OnData:   func(string, []byte) {},
				OnClosed: func(string) {},
			})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "解析密钥失败")
		})
	})
}

func TestManager_GetSessionSyncStateReturnsDirSyncCodeWhenSessionMissing(t *testing.T) {
	m := NewManager()

	_, err := m.GetSessionSyncState("missing-session")

	assert.EqualError(t, err, dirsync.CodeSessionNotFound)
}

func TestSession_ClosedBehavior(t *testing.T) {
	convey.Convey("Session 关闭后的行为", t, func() {
		// 创建一个模拟的 closed session 来测试
		sess := &Session{
			ID:     "test-1",
			closed: true,
		}

		convey.Convey("关闭的 session Write 返回错误", func() {
			err := sess.Write([]byte("test"))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "closed")
		})

		convey.Convey("关闭的 session Resize 返回错误", func() {
			err := sess.Resize(80, 24)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "closed")
		})

		convey.Convey("IsClosed 返回 true", func() {
			assert.True(t, sess.IsClosed())
		})

		convey.Convey("重复 Close 不 panic", func() {
			assert.NotPanics(t, func() {
				sess.Close()
			})
		})
	})
}

func TestNormalizeShellType(t *testing.T) {
	assert.Equal(t, shellTypeBash, normalizeShellType("/bin/bash"))
	assert.Equal(t, shellTypeZsh, normalizeShellType("/usr/bin/zsh"))
	assert.Equal(t, shellTypeUnsupported, normalizeShellType("/usr/bin/fish"))
}

func TestSession_FilterOutputCapturesInitMarker(t *testing.T) {
	sess := newTestSyncSession()
	sess.syncBootstrapCh = make(chan struct{})

	raw := []byte("hello" + syncSequencePrefix + "real-token:init:pid:4242" + syncSequenceTerm + "world")
	filtered := sess.filterOutput(raw)

	assert.Equal(t, "helloworld", string(filtered))
	assert.Equal(t, 4242, sess.shellPID)
	assert.True(t, sess.syncDirty)
	assert.Nil(t, sess.syncBootstrapCh)
}

func TestSession_FilterOutputAcceptsValidatedPromptProof(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-once"
	sess.shellPID = 4242
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{cwd: "/srv/app", promptReady: true}, nil
	}

	raw := buildTestSyncSequence("real-token", "prompt:prompt-once:prompt-next:/srv/app")
	filtered := sess.filterOutput(raw)

	assert.Empty(t, filtered)
	state := sess.GetSyncState()
	assert.Equal(t, "/srv/app", state.Cwd)
	assert.True(t, state.CwdKnown)
	assert.True(t, state.PromptReady)
	assert.Equal(t, "prompt-next", sess.promptNonce)
}

func TestSession_ProbeMissDoesNotBreakNextPromptProof(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-one"
	sess.shellPID = 4242
	promptReady := false
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{cwd: "/srv/app", promptReady: promptReady}, nil
	}

	firstProof := buildTestSyncSequence("real-token", "prompt:prompt-one:prompt-two:/srv/app")
	replayedFirst := sess.filterOutput(firstProof)
	assert.Equal(t, string(firstProof), string(replayedFirst))
	assert.Equal(t, "prompt-one", sess.promptNonce)
	assert.Equal(t, "prompt-two", sess.promptPendingNonce)

	promptReady = true
	secondProof := buildTestSyncSequence("real-token", "prompt:prompt-two:prompt-three:/srv/app")
	filteredSecond := sess.filterOutput(secondProof)
	assert.Empty(t, filteredSecond)

	state := sess.GetSyncState()
	assert.True(t, state.PromptReady)
	assert.True(t, state.CwdKnown)
	assert.Equal(t, "/srv/app", state.Cwd)
	assert.Equal(t, "prompt-three", sess.promptNonce)
	assert.Empty(t, sess.promptPendingNonce)
}

func TestSession_OldPromptProofReplayFailsAfterConsumption(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-once"
	sess.shellPID = 4242
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{cwd: "/srv/app", promptReady: true}, nil
	}

	raw := buildTestSyncSequence("real-token", "prompt:prompt-once:prompt-next:/srv/app")
	assert.Empty(t, sess.filterOutput(raw))

	replayed := sess.filterOutput(raw)
	assert.Equal(t, string(raw), string(replayed))

	state := sess.GetSyncState()
	assert.True(t, state.PromptReady)
	assert.Equal(t, "/srv/app", state.Cwd)
	assert.Equal(t, "prompt-next", sess.promptNonce)
}

func TestSession_PrepareDirectoryChangeRequiresCleanPrompt(t *testing.T) {
	sess := newTestSyncSession()
	sess.notePrompt("/srv/app")
	sess.markUserInput([]byte("ls"))

	_, err := sess.prepareDirectoryChange("/srv/logs", "/srv/logs", make(chan error, 1))
	assert.EqualError(t, err, "DIRSYNC_BUSY")
}

func TestSession_FilterOutputIgnoresSpoofedMarkers(t *testing.T) {
	sess := newTestSyncSession()
	sess.notePrompt("/srv/app")
	sess.markUserInput([]byte("ls\r"))

	fakeCwd := buildTestSyncSequence("fake-token", "cwd:/srv/fake")
	filtered := sess.filterOutput(fakeCwd)

	assert.Equal(t, string(fakeCwd), string(filtered))

	state := sess.GetSyncState()
	assert.Empty(t, state.Cwd)
	assert.False(t, state.CwdKnown)
	assert.False(t, state.PromptReady)
	assert.False(t, state.PromptClean)
	assert.True(t, state.Busy)
	assert.Empty(t, state.LastError)
}

func TestSession_ReplayedReadableMarkerCannotFinishPendingDirectoryChange(t *testing.T) {
	sess := newTestSyncSession()
	sess.notePrompt("/srv/app")

	resultCh := make(chan error, 1)
	_, err := sess.prepareDirectoryChange("/srv/logs", "/srv/logs", resultCh)
	assert.NoError(t, err)

	replayedChdir := buildTestSyncSequence("real-token", "chdir:ok:/srv/logs")
	filtered := sess.filterOutput(replayedChdir)

	assert.Equal(t, string(replayedChdir), string(filtered))
	assert.NotNil(t, sess.pendingDirChange)
	assert.NotEmpty(t, sess.pendingDirNonce)

	select {
	case result := <-resultCh:
		t.Fatalf("expected pending dir change to remain unresolved, got %v", result)
	default:
	}

	state := sess.GetSyncState()
	assert.False(t, state.PromptReady)
	assert.True(t, state.Busy)
	assert.False(t, state.CwdKnown)
	assert.Empty(t, state.LastError)
}

func TestSession_ProbeCwdDoesNotRestoreReadyDuringBuiltinWait(t *testing.T) {
	sess := newTestSyncSession()
	sess.notePrompt("/srv/app")
	sess.markUserInput([]byte("read foo\r"))
	sess.noteObservedCwd("/srv/app")

	state := sess.GetSyncState()
	assert.False(t, state.PromptReady)
	assert.False(t, state.PromptClean)
	assert.True(t, state.CwdKnown)
	assert.Equal(t, "/srv/app", state.Cwd)
	assert.True(t, state.Busy)
	assert.Equal(t, directorySyncInitializing, state.Status)
}

func TestSession_OrdinaryCommandCanRestoreReadyAgain(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-once"
	sess.shellPID = 4242
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{cwd: "/srv/app", promptReady: true}, nil
	}
	assert.Empty(t, sess.filterOutput(buildTestSyncSequence("real-token", "prompt:prompt-once:prompt-two:/srv/app")))
	sess.markUserInput([]byte("ls\r"))

	replayedOld := sess.filterOutput(buildTestSyncSequence("real-token", "prompt:prompt-once:prompt-two:/srv/app"))
	assert.NotEmpty(t, replayedOld)

	filtered := sess.filterOutput(buildTestSyncSequence("real-token", "prompt:prompt-two:prompt-three:/srv/app"))
	assert.Empty(t, filtered)

	state := sess.GetSyncState()
	assert.True(t, state.PromptReady)
	assert.True(t, state.PromptClean)
	assert.True(t, state.CwdKnown)
	assert.Equal(t, "/srv/app", state.Cwd)
	assert.False(t, state.Busy)
	assert.Equal(t, "prompt-three", sess.promptNonce)
}

func TestSession_OrdinaryOutputCannotSpoofReady(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-once"
	sess.shellPID = 4242
	promptReady := true
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{cwd: "/srv/app", promptReady: promptReady}, nil
	}
	assert.Empty(t, sess.filterOutput(buildTestSyncSequence("real-token", "prompt:prompt-once:prompt-two:/srv/app")))
	sess.markUserInput([]byte("read foo\r"))
	promptReady = false

	replayedPrompt := buildTestSyncSequence("real-token", "prompt:prompt-two:prompt-three:/srv/fake")
	filtered := sess.filterOutput(replayedPrompt)

	assert.Equal(t, string(replayedPrompt), string(filtered))

	state := sess.GetSyncState()
	assert.False(t, state.PromptReady)
	assert.False(t, state.CwdKnown)
	assert.True(t, state.Busy)
	assert.Equal(t, directorySyncInitializing, state.Status)
	assert.Equal(t, "prompt-two", sess.promptNonce)
}

func TestSession_FilterOutputBoundsParserRemainder(t *testing.T) {
	sess := newTestSyncSession()
	firstChunk := []byte(syncSequencePrefix + "real-token:cwd:" + strings.Repeat("x", syncSequenceParserMaxBytes/2))
	secondChunk := []byte(strings.Repeat("y", syncSequenceParserMaxBytes))

	filteredFirst := sess.filterOutput(firstChunk)
	assert.Empty(t, filteredFirst)
	assert.LessOrEqual(t, len(sess.parserRemainder), syncSequenceParserMaxBytes)

	filteredSecond := sess.filterOutput(secondChunk)
	assert.Equal(t, string(append(append([]byte(nil), firstChunk...), secondChunk...)), string(filteredSecond))
	assert.Len(t, sess.parserRemainder, 0)

	state := sess.GetSyncState()
	assert.Equal(t, directorySyncMarkerOverflow, state.LastError)
	assert.False(t, state.PromptReady)
	assert.True(t, state.Busy)
}

func TestParseShellProbeOutput(t *testing.T) {
	result, err := parseShellProbeOutput([]byte("cwd=/srv/app\x00prompt=1\x00"))
	assert.NoError(t, err)
	assert.Equal(t, "/srv/app", result.cwd)
	assert.True(t, result.promptReady)
}

func TestSession_PendingDirectoryChangeAcceptsCanonicalCwd(t *testing.T) {
	sess := newTestSyncSession()
	sess.notePrompt("/home/me")

	resultCh := make(chan error, 1)
	_, err := sess.prepareDirectoryChange("/home/me/current", "/srv/releases/2026", resultCh)
	assert.NoError(t, err)

	sess.finishPendingDirectoryChangeProbe(sess.pendingDirNonce, sess.pendingDirTarget, "/srv/releases/2026")

	select {
	case result := <-resultCh:
		assert.NoError(t, result)
	default:
		t.Fatal("expected canonical cwd to complete pending directory change")
	}

	state := sess.GetSyncState()
	assert.Equal(t, "/srv/releases/2026", state.Cwd)
	assert.True(t, state.CwdKnown)
	assert.True(t, state.PromptReady)
}

func TestSession_ProbeLoopDisablesSyncAfterRepeatedUnusableResults(t *testing.T) {
	oldInterval := syncProbeInterval
	oldMax := syncProbeMaxUnusableResults
	syncProbeInterval = 5 * time.Millisecond
	syncProbeMaxUnusableResults = 3
	defer func() {
		syncProbeInterval = oldInterval
		syncProbeMaxUnusableResults = oldMax
	}()

	sess := newTestSyncSession()
	sess.shellPID = 4242
	sess.shared = &sharedClient{client: &ssh.Client{}}
	sess.syncProbeActive = true
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{}, nil
	}

	go sess.runSyncProbeLoop()

	assert.Eventually(t, func() bool {
		state := sess.GetSyncState()
		return !state.Supported &&
			state.Status == directorySyncUnsupported &&
			state.LastError == dirSyncErrProbeUnsupported &&
			!state.Busy
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func TestSession_ProbeLoopDisablesSyncWhenPromptProbeHasNoCwd(t *testing.T) {
	oldInterval := syncProbeInterval
	oldMax := syncProbeMaxUnusableResults
	syncProbeInterval = 5 * time.Millisecond
	syncProbeMaxUnusableResults = 3
	defer func() {
		syncProbeInterval = oldInterval
		syncProbeMaxUnusableResults = oldMax
	}()

	sess := newTestSyncSession()
	sess.shellPID = 4242
	sess.shared = &sharedClient{client: &ssh.Client{}}
	sess.syncProbeActive = true
	sess.probeShellStateFn = func(_ int) (shellProbeResult, error) {
		return shellProbeResult{promptReady: true}, nil
	}

	go sess.runSyncProbeLoop()

	assert.Eventually(t, func() bool {
		state := sess.GetSyncState()
		return !state.Supported &&
			state.Status == directorySyncUnsupported &&
			state.LastError == dirSyncErrProbeUnsupported
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func TestSessionStartsWithSupportedFalse(t *testing.T) {
	sess := &Session{ID: "test"}
	sess.initSyncState("/bin/bash", shellTypeBash, false)

	state := sess.GetSyncState()
	if state.Supported {
		t.Fatal("Supported must be false until EnableSync is invoked")
	}
	if state.ShellType != shellTypeBash {
		t.Fatalf("ShellType not retained: %q", state.ShellType)
	}
	if state.Status != directorySyncUnsupported {
		t.Fatalf("Status should be unsupported initially, got %q", state.Status)
	}
}

func TestEnableSyncTimesOutWhenNoMarker(t *testing.T) {
	stdin := &recordingWriteCloser{}
	sess := &Session{
		ID:        "test-timeout",
		stdin:     stdin,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)

	prev := syncEnableTimeout
	syncEnableTimeout = 100 * time.Millisecond
	defer func() { syncEnableTimeout = prev }()

	err := sess.EnableSync()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if state := sess.GetSyncState(); state.Supported {
		t.Fatalf("Supported must remain false on timeout, got %#v", state)
	}
	if sess.syncToken != "" || sess.promptNonce != "" || sess.promptPendingNonce != "" {
		t.Fatal("timeout must invalidate sync tokens")
	}
	if sess.shellPID != 0 {
		t.Fatalf("timeout must clear shellPID, got %d", sess.shellPID)
	}
}

func TestEnableSyncIgnoresLateInitMarkerAfterTimeout(t *testing.T) {
	stdin := &recordingWriteCloser{}
	sess := &Session{
		ID:        "test-late-marker",
		stdin:     stdin,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)

	prevTimeout := syncEnableTimeout
	syncEnableTimeout = 20 * time.Millisecond
	defer func() { syncEnableTimeout = prevTimeout }()

	err := sess.EnableSync()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	token := extractInitTokenFromEnableCommand(t, stdin.lastWrite())
	filtered := sess.filterOutput(buildTestSyncSequence(token, "init:pid:999"))
	assert.NotEmpty(t, filtered, "late marker should be replayed as ordinary output")

	state := sess.GetSyncState()
	assert.False(t, state.Supported)
	assert.Equal(t, directorySyncUnsupported, state.Status)
	assert.Equal(t, 0, sess.shellPID)
}

func TestSessionIgnoresPromptAfterDisable(t *testing.T) {
	sess := newTestSyncSession()
	sess.promptNonce = "prompt-one"
	sess.shellPID = 4242
	sess.disableDirectorySync(dirSyncErrUnsupported)

	filtered := sess.filterOutput(buildTestSyncSequence("real-token", "prompt:prompt-one:prompt-two:/srv/app"))
	assert.NotEmpty(t, filtered, "stale prompt marker should not be consumed after disable")

	state := sess.GetSyncState()
	assert.False(t, state.Supported)
	assert.False(t, state.CwdKnown)
	assert.Equal(t, 0, sess.shellPID)
}

func TestEnableSyncUnsupportedShellReturnsError(t *testing.T) {
	sess := &Session{ID: "u", shellType: shellTypeUnsupported}
	sess.initSyncState("/bin/sh", shellTypeUnsupported, false)
	if err := sess.EnableSync(); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestEnableSyncIdempotent(t *testing.T) {
	sess := &Session{ID: "i", shellType: shellTypeBash, shellPath: "/bin/bash"}
	sess.initSyncState(sess.shellPath, sess.shellType, true)
	sess.syncMu.Lock()
	sess.syncState.Supported = true
	sess.syncState.Status = directorySyncReady
	sess.shellPID = 12345
	sess.syncMu.Unlock()

	if err := sess.EnableSync(); err != nil {
		t.Fatalf("idempotent enable should return nil, got %v", err)
	}
}

func TestEnableSyncSerializesConcurrentFirstEnable(t *testing.T) {
	stdin := &recordingWriteCloser{}
	sess := &Session{
		ID:        "test-concurrent",
		stdin:     stdin,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)
	stdin.onWrite = func(data []byte) {
		token := extractInitTokenFromEnableCommand(t, data)
		_ = sess.filterOutput(buildTestSyncSequence(token, "init:pid:4242"))
	}

	prevGrace := syncFirstCwdGrace
	syncFirstCwdGrace = time.Millisecond
	defer func() { syncFirstCwdGrace = prevGrace }()

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- sess.EnableSync()
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, stdin.writeCount(), "concurrent first enable should write one bootstrap command")
	state := sess.GetSyncState()
	assert.True(t, state.Supported)
	assert.Equal(t, 4242, sess.shellPID)
}

func TestEnableSyncWriteFailureRollsBackBootstrap(t *testing.T) {
	stdin := &recordingWriteCloser{err: errors.New("broken pipe")}
	sess := &Session{
		ID:        "test-write-failure",
		stdin:     stdin,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)

	err := sess.EnableSync()
	assert.EqualError(t, err, dirsync.CodeSessionClosed)

	state := sess.GetSyncState()
	assert.False(t, state.Supported)
	assert.False(t, state.Busy)
	assert.Equal(t, directorySyncUnsupported, state.Status)
	assert.Empty(t, sess.syncToken)
	assert.Empty(t, sess.promptNonce)
	assert.Nil(t, sess.syncBootstrapCh)
}

func TestEnableSyncReturnsErrorIfDisableRacesIn(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	sess := &Session{
		ID:        "test-race",
		stdin:     pw,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)
	go func() { _, _ = io.Copy(io.Discard, pr) }()

	prev := syncEnableTimeout
	syncEnableTimeout = 500 * time.Millisecond
	defer func() { syncEnableTimeout = prev }()

	errCh := make(chan error, 1)
	go func() { errCh <- sess.EnableSync() }()

	assert.Eventually(t, func() bool {
		sess.syncMu.Lock()
		bootstrapping := sess.syncBootstrapCh != nil
		state := sess.syncState
		sess.syncMu.Unlock()
		return bootstrapping &&
			state.Supported &&
			state.Busy &&
			state.Status == directorySyncInitializing &&
			!state.CwdKnown
	}, 500*time.Millisecond, 10*time.Millisecond, "EnableSync should enter busy initializing state")

	// Simulate a disable racing in before init:pid arrives.
	sess.disableDirectorySync(dirSyncErrUnsupported)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("EnableSync must report error when DisableSync races in")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EnableSync did not return after DisableSync; bootstrap channel not closed?")
	}

	if state := sess.GetSyncState(); state.Supported {
		t.Fatalf("Supported must be false after disable race, got %#v", state)
	}
}

// generateTestPrivateKeyPEM 生成 ed25519 测试私钥的 PKCS8 PEM 编码。
func generateTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成 ed25519 密钥失败: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("PKCS8 编码失败: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// TestBuildAuthMethods 校验各 authType 分支返回正确的 ssh.AuthMethod 序列。
// 重点回归 issue #77：authType=key 分支必须在末尾追加 keyboard-interactive，
// 以支持 publickey + MFA/OTP 链路（JumpServer 等堡垒机场景）。
func TestBuildAuthMethods(t *testing.T) {
	convey.Convey("buildAuthMethods 按 authType 返回正确的认证方法序列", t, func() {
		pemKey := generateTestPrivateKeyPEM(t)
		keyPath := filepath.Join(t.TempDir(), "id_test")
		if err := os.WriteFile(keyPath, []byte(pemKey), 0o600); err != nil {
			t.Fatalf("写入测试密钥文件失败: %v", err)
		}

		// crypto/ssh 包内的具体类型是私有的，但 %T 格式化能拿到稳定的类型名称，
		// 足够用来断言 method 序列的组成与顺序。
		const (
			tPassword = "ssh.passwordCallback"
			tPubKey   = "ssh.publicKeyCallback"
			tKBI      = "ssh.KeyboardInteractiveChallenge"
		)
		typeNames := func(ms []ssh.AuthMethod) []string {
			out := make([]string, len(ms))
			for i, m := range ms {
				out[i] = fmt.Sprintf("%T", m)
			}
			return out
		}

		convey.Convey("authType=password 返回 [password, keyboard-interactive]", func() {
			ms, err := buildAuthMethods("password", "hunter2", "", "", nil, nil)
			assert.NoError(t, err)
			assert.Equal(t, []string{tPassword, tKBI}, typeNames(ms))
		})

		convey.Convey("authType=key + inline key 返回 [publickey, keyboard-interactive]（issue #77）", func() {
			ms, err := buildAuthMethods("key", "", pemKey, "", nil, nil)
			assert.NoError(t, err)
			assert.Equal(t, []string{tPubKey, tKBI}, typeNames(ms))
		})

		convey.Convey("authType=key + 多个 file paths 返回 [publickey...publickey, keyboard-interactive]", func() {
			ms, err := buildAuthMethods("key", "", "", "", []string{keyPath, keyPath}, nil)
			assert.NoError(t, err)
			assert.Equal(t, []string{tPubKey, tPubKey, tKBI}, typeNames(ms))
		})

		convey.Convey("authType=key + inline + file paths 同时存在", func() {
			ms, err := buildAuthMethods("key", "", pemKey, "", []string{keyPath}, nil)
			assert.NoError(t, err)
			assert.Equal(t, []string{tPubKey, tPubKey, tKBI}, typeNames(ms))
		})

		convey.Convey("authType=key 但未提供任何密钥返回错误", func() {
			_, err := buildAuthMethods("key", "", "", "", nil, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "需要提供私钥")
		})

		convey.Convey("authType=keyboard-interactive 仅返回 [keyboard-interactive]", func() {
			ms, err := buildAuthMethods("keyboard-interactive", "", "", "", nil, nil)
			assert.NoError(t, err)
			assert.Equal(t, []string{tKBI}, typeNames(ms))
		})

		convey.Convey("未知 authType 返回错误", func() {
			_, err := buildAuthMethods("magic", "", "", "", nil, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "不支持的认证方式")
		})
	})
}
