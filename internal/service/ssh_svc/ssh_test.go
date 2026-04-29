package ssh_svc

import (
	"strings"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/pkg/dirsync"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

func newTestSyncSession(token string) *Session {
	sess := &Session{syncToken: token}
	sess.initSyncState("/bin/bash", shellTypeBash, true)
	return sess
}

func buildTestSyncSequence(token, payload string) []byte {
	return []byte(syncSequencePrefix + token + ":" + payload + syncSequenceTerm)
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
	sess := newTestSyncSession("real-token")

	raw := []byte("hello" + syncSequencePrefix + "real-token:init:pid:4242" + syncSequenceTerm + "world")
	filtered := sess.filterOutput(raw)

	assert.Equal(t, "helloworld", string(filtered))
	assert.Equal(t, 4242, sess.shellPID)
	assert.True(t, sess.syncDirty)
}

func TestSession_FilterOutputAcceptsValidatedPromptProof(t *testing.T) {
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
	sess.notePrompt("/srv/app")
	sess.markUserInput([]byte("ls"))

	_, err := sess.prepareDirectoryChange("/srv/logs", "/srv/logs", make(chan error, 1))
	assert.EqualError(t, err, "DIRSYNC_BUSY")
}

func TestSession_FilterOutputIgnoresSpoofedMarkers(t *testing.T) {
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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
	sess := newTestSyncSession("real-token")
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

func TestBuildInteractiveShellCommand_BashDoesNotSourceProfiles(t *testing.T) {
	command := buildInteractiveShellCommand("/bin/bash", shellTypeBash, "token", "nonce")

	assert.NotContains(t, command, ".bash_profile")
	assert.NotContains(t, command, ".bash_login")
	assert.NotContains(t, command, ".profile")
	assert.Contains(t, command, ".bashrc")
}

func TestSession_PendingDirectoryChangeAcceptsCanonicalCwd(t *testing.T) {
	sess := newTestSyncSession("real-token")
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

	sess := newTestSyncSession("real-token")
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

	sess := newTestSyncSession("real-token")
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
