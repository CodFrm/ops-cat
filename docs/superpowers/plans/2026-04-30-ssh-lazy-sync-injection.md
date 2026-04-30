# SSH Lazy Sync Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the server-native `Last login` / motd / banner output on SSH terminal connect by removing the unconditional shell wrapper. Move directory-sync hook setup from session start to on-demand injection triggered by the user's first sync action (F→T / T→F / always-follow toggle).

**Architecture:** `createSession` always opens via `session.Shell()` (SSH `shell` request, sshd emits banner natively). `Session` records the detected shell type but starts with `Supported=false`. New `Session.EnableSync()` writes a single-line hook installer to stdin (function definitions + `PROMPT_COMMAND`/`precmd`/`PS1` hook + `init:pid` marker), then waits for the marker and the first prompt nonce within a timeout to confirm bootstrap. `App.ChangeSSHDirectory` auto-enables sync if not yet supported. Frontend F→T flow lazy-enables on first click; T→F flow relies on backend auto-enable.

**Tech Stack:** Go 1.25, `golang.org/x/crypto/ssh`, Wails v2 IPC, React 19, Zustand, goconvey/testify, Vitest.

---

## File Structure

- Modify `internal/service/ssh_svc/dirsync_shell.go`: split `buildInteractiveShellCommand` into `buildEnableSyncCommand` (single-line stdin command, not a wrapping shell) + `buildDisableSyncCommand`.
- Modify `internal/service/ssh_svc/ssh.go`:
  - `Session` struct gains `shellPath` / `shellType` fields persisted at session creation.
  - `createSession` removes the `Start(wrapperCmd)` branch — always calls `session.Shell()`. Drops eager `syncToken` / `promptNonce` generation.
  - Adds `(*Manager).enableSyncForSession` shim (or move logic onto Session).
- Modify `internal/service/ssh_svc/dirsync.go`: add `(*Session).EnableSync(ctx) error` and `(*Session).DisableSync()` methods. `EnableSync` is idempotent and serialized via `syncMu`.
- Modify `internal/app/app_ssh.go`:
  - `ChangeSSHDirectory` auto-calls `EnableSync` when `!Supported`.
  - Add `EnableSSHSync(sessionID) error` Wails binding for explicit F→T.
- Modify `internal/service/ssh_svc/ssh_test.go` and add `dirsync_shell_test.go`: cover new builders and bootstrap flow.
- Regenerate `frontend/wailsjs/go/app/App.{d.ts,js}` via `make dev` once binding is added.
- Modify `frontend/src/components/terminal/file-manager/useTerminalDirectorySync.ts`: F→T (`syncPanelFromTerminal`) lazy-calls `EnableSSHSync` when state is `unsupported`/missing.
- Optionally modify `frontend/src/components/terminal/TerminalToolbar.tsx` (or wherever the sync indicator lives) to surface "正在启用目录同步…" / "请退出当前程序后重试" toasts.

## Tasks

### Task 1: Split shell command builders

**Files:**
- Modify: `internal/service/ssh_svc/dirsync_shell.go`
- Test: `internal/service/ssh_svc/dirsync_shell_test.go` (new)

The new `buildEnableSyncCommand` returns a one-line stdin payload. It is **not** a wrapping `exec` — it runs inside the user's already-running interactive shell, so it must be a single logical command terminated by `\r` (so the shell executes it immediately). It defines functions, sets the prompt hook, emits the `init:pid` marker, all separated by `;`. Heredocs are unsuitable inside a single line; we inline function bodies via `;` (multi-statement function body in bash/zsh syntax: `f() { stmt1; stmt2; }`).

- [ ] **Step 1: Add failing test for `buildEnableSyncCommand` bash variant**

```go
// internal/service/ssh_svc/dirsync_shell_test.go
package ssh_svc

import (
	"strings"
	"testing"
)

func TestBuildEnableSyncCommandBash(t *testing.T) {
	cmd := buildEnableSyncCommand(shellTypeBash, "TOK", "NONCE")
	if !strings.Contains(cmd, "opskat_prompt_proof") {
		t.Fatalf("missing function name: %s", cmd)
	}
	if !strings.Contains(cmd, "OPSKAT_PROMPT_NONCE=NONCE") {
		t.Fatalf("missing nonce assignment: %s", cmd)
	}
	if !strings.Contains(cmd, `PROMPT_COMMAND="opskat_prompt_proof`) {
		t.Fatalf("missing PROMPT_COMMAND wiring: %s", cmd)
	}
	if !strings.Contains(cmd, `1337;opskat:TOK:init:pid:`) {
		t.Fatalf("missing init marker: %s", cmd)
	}
	if strings.ContainsAny(cmd, "\n") {
		t.Fatalf("command must be single-line, got newline: %q", cmd)
	}
	if !strings.HasSuffix(cmd, "\r") {
		t.Fatalf("command must end with carriage return: %q", cmd)
	}
}
```

- [ ] **Step 2: Run test to confirm failure**

Run: `go test ./internal/service/ssh_svc -run TestBuildEnableSyncCommandBash -count=1`
Expected: FAIL with `undefined: buildEnableSyncCommand`.

- [ ] **Step 3: Replace `buildInteractiveShellCommand` with the split builders**

Edit `internal/service/ssh_svc/dirsync_shell.go`. Remove `buildInteractiveShellCommand`. Add:

```go
// buildEnableSyncCommand returns a single-line shell statement to be written
// to the running interactive shell's stdin. It installs the prompt-proof
// hook, sets the initial nonce, and emits the init:pid marker.
// shellType MUST be one of shellTypeBash/shellTypeZsh/shellTypeKsh/shellTypeMksh.
func buildEnableSyncCommand(shellType, syncToken, promptNonce string) string {
	switch shellType {
	case shellTypeBash:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ local n r;n=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);r=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$r" "$n";};`+
				`opskat_prompt_proof(){ local p c x;c=${OPSKAT_PROMPT_NONCE:-};[ -n "$c" ]||return;x=$(opskat_next_prompt_nonce);p=$(builtin pwd -P 2>/dev/null||builtin pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$c" "$x" "$p";OPSKAT_PROMPT_NONCE=$x;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`PROMPT_COMMAND="opskat_prompt_proof${PROMPT_COMMAND:+;$PROMPT_COMMAND}";`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	case shellTypeZsh:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ local n r;n=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);r=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$r" "$n";};`+
				`opskat_prompt_proof(){ local p c x;c=${OPSKAT_PROMPT_NONCE:-};[[ -n "$c" ]]||return;x=$(opskat_next_prompt_nonce);p=$(pwd -P 2>/dev/null||pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$c" "$x" "$p";OPSKAT_PROMPT_NONCE=$x;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`autoload -Uz add-zsh-hook;add-zsh-hook precmd opskat_prompt_proof;`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	case shellTypeKsh, shellTypeMksh:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ OPSKAT_NOW=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);OPSKAT_RAND=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$OPSKAT_RAND" "$OPSKAT_NOW";};`+
				`opskat_prompt_proof(){ OPSKAT_CURRENT=${OPSKAT_PROMPT_NONCE:-};[ -n "$OPSKAT_CURRENT" ]||return;OPSKAT_NEXT=$(opskat_next_prompt_nonce);OPSKAT_PWD=$(pwd -P 2>/dev/null||pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$OPSKAT_CURRENT" "$OPSKAT_NEXT" "$OPSKAT_PWD";OPSKAT_PROMPT_NONCE=$OPSKAT_NEXT;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`OPSKAT_ORIG_PS1=${OPSKAT_ORIG_PS1:-$PS1};PS1='$(opskat_prompt_proof)'"$OPSKAT_ORIG_PS1";`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	default:
		return ""
	}
}

// buildDisableSyncCommand returns a one-line statement that removes the hook
// and unsets helper functions. Safe to send even if EnableSync was never run.
func buildDisableSyncCommand(shellType string) string {
	switch shellType {
	case shellTypeBash:
		return `PROMPT_COMMAND=${PROMPT_COMMAND#opskat_prompt_proof};PROMPT_COMMAND=${PROMPT_COMMAND#;};unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE` + "\r"
	case shellTypeZsh:
		return `add-zsh-hook -d precmd opskat_prompt_proof 2>/dev/null;unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE` + "\r"
	case shellTypeKsh, shellTypeMksh:
		return `[ -n "$OPSKAT_ORIG_PS1" ] && PS1=$OPSKAT_ORIG_PS1;unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE OPSKAT_ORIG_PS1` + "\r"
	default:
		return ""
	}
}
```

Update or delete the deprecated test in `ssh_test.go` that references `buildInteractiveShellCommand` (search first, then either rename to point at `buildEnableSyncCommand` or remove if it was checking the wrapping form specifically). Use grep:

```bash
grep -n "buildInteractiveShellCommand" internal/service/ssh_svc/*.go
```

Replace usages or delete the test. Update `ssh_test.go:356` and any neighboring asserts to the new builder semantics (no wrapping `exec`, single line, ends in `\r`).

- [ ] **Step 4: Add zsh + ksh variant tests in the same test file**

```go
func TestBuildEnableSyncCommandZsh(t *testing.T) {
	cmd := buildEnableSyncCommand(shellTypeZsh, "TOK", "NONCE")
	if !strings.Contains(cmd, "add-zsh-hook precmd opskat_prompt_proof") {
		t.Fatalf("missing add-zsh-hook: %s", cmd)
	}
	if !strings.Contains(cmd, "1337;opskat:TOK:init:pid:") {
		t.Fatalf("missing init marker: %s", cmd)
	}
	if !strings.HasSuffix(cmd, "\r") {
		t.Fatalf("must end with \\r: %q", cmd)
	}
}

func TestBuildEnableSyncCommandKsh(t *testing.T) {
	cmd := buildEnableSyncCommand(shellTypeKsh, "TOK", "NONCE")
	if !strings.Contains(cmd, `PS1='$(opskat_prompt_proof)'`) {
		t.Fatalf("missing PS1 wiring: %s", cmd)
	}
	if !strings.HasSuffix(cmd, "\r") {
		t.Fatalf("must end with \\r: %q", cmd)
	}
}

func TestBuildEnableSyncCommandUnsupported(t *testing.T) {
	if got := buildEnableSyncCommand(shellTypeUnsupported, "T", "N"); got != "" {
		t.Fatalf("unsupported shell must return empty, got: %q", got)
	}
}

func TestBuildDisableSyncCommand(t *testing.T) {
	for _, sh := range []string{shellTypeBash, shellTypeZsh, shellTypeKsh, shellTypeMksh} {
		got := buildDisableSyncCommand(sh)
		if got == "" || !strings.HasSuffix(got, "\r") {
			t.Fatalf("disable cmd for %s should end in \\r, got %q", sh, got)
		}
	}
	if buildDisableSyncCommand(shellTypeUnsupported) != "" {
		t.Fatal("unsupported should return empty")
	}
}
```

- [ ] **Step 5: Run all tests in package, fix any compile errors from the removal**

Run: `go test ./internal/service/ssh_svc -count=1`
Expected: all new tests PASS. Pre-existing tests that referenced `buildInteractiveShellCommand` either updated or deleted.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ssh_svc/dirsync_shell.go internal/service/ssh_svc/dirsync_shell_test.go internal/service/ssh_svc/ssh_test.go
git commit -m "♻️ 拆分 SSH 目录同步钩子为按需启用/关闭命令"
```

---

### Task 2: Persist shell info on Session and remove wrapper from createSession

**Files:**
- Modify: `internal/service/ssh_svc/ssh.go` (Session struct, createSession)
- Test: `internal/service/ssh_svc/ssh_test.go`

- [ ] **Step 1: Add failing test asserting createSession does not eagerly mark Supported**

Append to `ssh_test.go`:

```go
func TestSessionStartsWithSupportedFalse(t *testing.T) {
	// Construct a Session via initSyncState directly (no real SSH needed).
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
```

- [ ] **Step 2: Run test, observe current state**

Run: `go test ./internal/service/ssh_svc -run TestSessionStartsWithSupportedFalse -count=1`
Expected: PASS already (initSyncState with `supported=false` already sets `Status=unsupported`). This is a regression guard.

- [ ] **Step 3: Add `shellPath` / `shellType` fields to Session struct**

Edit `internal/service/ssh_svc/ssh.go`. Locate the `Session` struct (around line 62). Add two fields:

```go
type Session struct {
	ID       string
	AssetID  int64
	shared   *sharedClient
	session  *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	mu       sync.Mutex
	closed   bool
	onData   func(data []byte)
	onClosed func(sessionID string)
	onSync   func(sessionID string, state DirectorySyncState)

	shellPath string // detected user shell, e.g. /bin/bash — set in createSession
	shellType string // shellTypeBash / shellTypeZsh / shellTypeKsh / shellTypeMksh / shellTypeUnsupported

	syncMu             sync.Mutex
	syncState          DirectorySyncState
	// ... existing fields below ...
```

- [ ] **Step 4: Rewrite createSession to always use Shell() and never inject at start**

Replace the block in `createSession` from the `shellPath, shellType := detectRemoteShell(...)` line through the `if supported { ... } else if err := session.Shell(); ...` branch (currently around lines 353–377) with:

```go
shellPath, shellType := detectRemoteShell(shared.client)
sess := &Session{
	ID:        sessionID,
	AssetID:   assetID,
	shared:    shared,
	session:   session,
	stdin:     stdin,
	stdout:    stdout,
	shellPath: shellPath,
	shellType: shellType,
	onData:    func(data []byte) { onData(sessionID, data) },
	onClosed:  onClosed,
}
if onSync != nil {
	sess.onSync = func(_ string, state DirectorySyncState) { onSync(sessionID, state) }
}

// Always use the SSH "shell" request so sshd emits Last login / motd / banner
// natively. Directory-sync hooks are injected on demand via EnableSync().
if err := session.Shell(); err != nil {
	if closeErr := session.Close(); closeErr != nil {
		logger.Default().Warn("close session after shell start failure", zap.Error(closeErr))
	}
	return "", fmt.Errorf("启动shell失败: %w", err)
}

sess.initSyncState(shellPath, shellType, false)

m.sessions.Store(sessionID, sess)
go m.readOutput(sess)

return sessionID, nil
```

Also delete the now-unused early `syncToken` / `promptNonce` generation block (currently lines 322–335 in `ssh.go`). The Session no longer has `syncToken` / `promptNonce` populated at creation — those are generated on EnableSync. The struct fields stay (they're used during the sync lifecycle).

- [ ] **Step 5: Run package tests + build**

```bash
go build ./...
go test ./internal/service/ssh_svc -count=1
```

Expected: PASS. Compile failures from missing fields → fix locally.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ssh_svc/ssh.go internal/service/ssh_svc/ssh_test.go
git commit -m "♻️ SSH 会话默认走 Shell 请求，保留服务端原生登录提示"
```

---

### Task 3: Implement Session.EnableSync / DisableSync

**Files:**
- Modify: `internal/service/ssh_svc/dirsync.go`
- Test: `internal/service/ssh_svc/ssh_test.go`

`EnableSync` is idempotent: if already supported it returns nil. Otherwise it generates fresh tokens, writes the enable command via `writeInternal`, and waits up to a bootstrap timeout for `handleSyncPayload` to flip `shellPID`. The wait is implemented as a channel signaled by `handleSyncPayload` when `init:pid` arrives.

The bootstrap window: 3 seconds is generous. If it expires, we surface an error so the frontend can toast "请退出当前程序后重试".

- [ ] **Step 1: Failing test for EnableSync timeout path**

Append to `ssh_test.go`:

```go
func TestEnableSyncTimesOutWhenNoMarker(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	sess := &Session{
		ID:        "test-timeout",
		stdin:     pw,
		shellPath: "/bin/bash",
		shellType: shellTypeBash,
	}
	sess.initSyncState(sess.shellPath, sess.shellType, false)

	// Drain whatever the enable command writes so the pipe doesn't block.
	go func() { _, _ = io.Copy(io.Discard, pr) }()

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
	sess.initSyncState(sess.shellPath, sess.shellType, true) // pretend already enabled
	sess.syncMu.Lock()
	sess.syncState.Supported = true
	sess.syncState.Status = directorySyncReady
	sess.shellPID = 12345
	sess.syncMu.Unlock()

	if err := sess.EnableSync(); err != nil {
		t.Fatalf("idempotent enable should return nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests, observe undefined symbols**

Run: `go test ./internal/service/ssh_svc -run TestEnableSync -count=1`
Expected: FAIL with `sess.EnableSync undefined` and `syncEnableTimeout undefined`.

- [ ] **Step 3: Add EnableSync, DisableSync, and bootstrap signaling**

Edit `internal/service/ssh_svc/dirsync.go`. Add near the top of the file (after the existing `var (...)` block):

```go
var syncEnableTimeout = 3 * time.Second
```

Add two fields to `Session` (in `ssh.go`, the struct definition from Task 2):

```go
syncBootstrapCh chan struct{} // closed when EnableSync receives init:pid; nil when not bootstrapping
```

Add the methods at the end of `dirsync.go`:

```go
// EnableSync injects the directory-sync hooks into the running interactive
// shell and waits for the init:pid marker. Idempotent: returns nil immediately
// if already supported with a known shell PID.
func (s *Session) EnableSync() error {
	s.syncMu.Lock()
	if s.syncState.Supported && s.shellPID > 0 {
		s.syncMu.Unlock()
		return nil
	}
	if s.shellType == shellTypeUnsupported || s.shellType == "" {
		s.syncMu.Unlock()
		return dirsync.Error(dirSyncErrUnsupported)
	}
	if s.syncBootstrapCh != nil {
		// Another EnableSync is in flight; wait on the same channel.
		ch := s.syncBootstrapCh
		s.syncMu.Unlock()
		return waitForBootstrap(ch)
	}

	token, err := generateSyncToken()
	if err != nil {
		s.syncMu.Unlock()
		return dirsync.Error(dirSyncErrNonceFailed)
	}
	promptNonce, err := generateSyncToken()
	if err != nil {
		s.syncMu.Unlock()
		return dirsync.Error(dirSyncErrNonceFailed)
	}
	s.syncToken = token
	s.promptNonce = promptNonce
	s.shellPID = 0
	s.syncState.Supported = true
	s.syncState.Status = directorySyncInitializing
	s.syncState.LastError = ""
	s.syncDirty = true
	bootstrapCh := make(chan struct{})
	s.syncBootstrapCh = bootstrapCh
	state := s.syncState
	cmd := buildEnableSyncCommand(s.shellType, token, promptNonce)
	s.syncMu.Unlock()

	go s.emitSyncState(state)

	if cmd == "" {
		s.clearBootstrap(bootstrapCh)
		return dirsync.Error(dirSyncErrUnsupported)
	}
	if err := s.writeInternal([]byte(cmd)); err != nil {
		s.clearBootstrap(bootstrapCh)
		s.syncMu.Lock()
		s.syncState.Supported = false
		s.syncState.Status = directorySyncUnsupported
		s.syncState.LastError = err.Error()
		st := s.syncState
		s.syncMu.Unlock()
		s.emitSyncState(st)
		return err
	}

	if err := waitForBootstrap(bootstrapCh); err != nil {
		s.syncMu.Lock()
		// Only roll back if we still own the bootstrap (no one already promoted state).
		if s.syncBootstrapCh == bootstrapCh {
			s.syncBootstrapCh = nil
			s.syncState.Supported = false
			s.syncState.Status = directorySyncUnsupported
			s.syncState.LastError = err.Error()
			s.shellPID = 0
		}
		st := s.syncState
		s.syncMu.Unlock()
		s.emitSyncState(st)
		return err
	}
	return nil
}

// DisableSync removes hooks from the running shell and flips state back to
// unsupported. Best-effort: if the stdin write fails, state is still cleared.
func (s *Session) DisableSync() {
	s.syncMu.Lock()
	if !s.syncState.Supported {
		s.syncMu.Unlock()
		return
	}
	cmd := buildDisableSyncCommand(s.shellType)
	s.syncMu.Unlock()

	if cmd != "" {
		if err := s.writeInternal([]byte(cmd)); err != nil {
			logger.Default().Warn("write disable-sync command", zap.String("sessionID", s.ID), zap.Error(err))
		}
	}
	s.disableDirectorySync(dirSyncErrUnsupported)
}

func (s *Session) clearBootstrap(ch chan struct{}) {
	s.syncMu.Lock()
	if s.syncBootstrapCh == ch {
		s.syncBootstrapCh = nil
	}
	s.syncMu.Unlock()
}

func waitForBootstrap(ch chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-time.After(syncEnableTimeout):
		return dirsync.Error(dirSyncErrTimeout)
	}
}
```

Modify `handleSyncPayload` in `dirsync.go`. In the `init:pid` branch, after `s.shellPID = pid`, signal the bootstrap channel:

```go
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
	bootstrap := s.syncBootstrapCh
	s.syncBootstrapCh = nil
	s.syncMu.Unlock()
	if bootstrap != nil {
		close(bootstrap)
	}
	s.ensureSyncProbe()
	return true
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ssh_svc -count=1`
Expected: All Task-3 tests PASS. Existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ssh_svc/dirsync.go internal/service/ssh_svc/ssh.go internal/service/ssh_svc/ssh_test.go
git commit -m "✨ Session.EnableSync / DisableSync 实现按需注入目录同步钩子"
```

---

### Task 4: Auto-enable in ChangeSSHDirectory + add EnableSSHSync binding

**Files:**
- Modify: `internal/app/app_ssh.go`
- Test: `internal/app/app_ssh_test.go` (create if absent — otherwise extend)

- [ ] **Step 1: Failing test ensuring ChangeSSHDirectory auto-enables**

Search first:

```bash
ls internal/app/app_ssh_test.go 2>/dev/null || echo "missing"
```

If missing, create it. Otherwise extend. Test:

```go
// internal/app/app_ssh_test.go (extend or create)
package app

import (
	"errors"
	"testing"

	"github.com/opskat/opskat/internal/pkg/dirsync"
)

func TestChangeSSHDirectoryFailsCleanlyWhenSessionMissing(t *testing.T) {
	a := &App{sshManager: nil}
	err := a.ChangeSSHDirectory("nonexistent", "/tmp")
	if !errors.Is(err, dirsync.Error(dirsync.CodeSessionNotFound)) {
		t.Fatalf("expected SessionNotFound, got %v", err)
	}
}
```

Note: full integration test of auto-enable requires a real SSH server. The E2E suite (`make test-e2e`) covers that. Here we only assert the binding exists and surfaces the right error code on missing session.

- [ ] **Step 2: Run test, observe failure or pass**

Run: `go test ./internal/app -run TestChangeSSHDirectory -count=1`
Expected: PASS once the binding signature matches. (May already pass — this is a guard.)

- [ ] **Step 3: Modify ChangeSSHDirectory to auto-enable**

Edit `internal/app/app_ssh.go`. Replace the `ChangeSSHDirectory` body (currently around lines 408–435):

```go
// ChangeSSHDirectory 请求当前终端切换到指定目录。
// 若目录同步尚未启用，会自动注入钩子（一次性，会话内后续切换不再注入）。
func (a *App) ChangeSSHDirectory(sessionID, targetPath string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return dirsync.Error(dirsync.CodeSessionNotFound)
	}

	state := sess.GetSyncState()
	if !state.Supported {
		if err := sess.EnableSync(); err != nil {
			return err
		}
		state = sess.GetSyncState()
	}
	if !state.CwdKnown {
		// EnableSync resolved init:pid; first prompt may not have arrived yet.
		// Probe loop will fill cwd shortly. Surface a typed retry error so the
		// frontend can debounce + retry.
		return dirsync.Error(dirsync.CodeCwdUnknown)
	}

	resolvedPath := targetPath
	if !strings.HasPrefix(resolvedPath, "/") {
		resolvedPath = path.Join(state.Cwd, resolvedPath)
	}
	resolvedPath = path.Clean(resolvedPath)

	expectedPath, err := a.sftpService.ResolveDirectory(sessionID, resolvedPath)
	if err != nil {
		return err
	}
	return sess.ChangeDirectoryTo(resolvedPath, expectedPath)
}

// EnableSSHSync 显式启用目录同步（用于面板"跟随终端"按钮的首次点击）。
func (a *App) EnableSSHSync(sessionID string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return dirsync.Error(dirsync.CodeSessionNotFound)
	}
	return sess.EnableSync()
}
```

- [ ] **Step 4: Build + test + regenerate Wails bindings**

```bash
go build ./...
go test ./internal/app -count=1
```

Then regenerate the frontend Wails bindings. The project's standard regenerate path is via `make dev` — but for plan execution we want a non-interactive form. Use:

```bash
make dev
```

…and Ctrl-C once the dev server prints "wails: bindings generated" (or equivalent). Confirm `frontend/wailsjs/go/app/App.d.ts` now contains `EnableSSHSync`.

If `make dev` is too heavy for CI, run `wails generate module` directly (consult `Makefile` for the canonical command — search for `bindings`):

```bash
grep -n "binding\|generate" Makefile
```

Use whatever the project's documented regenerate command is.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app_ssh.go internal/app/app_ssh_test.go frontend/wailsjs/go/app/App.d.ts frontend/wailsjs/go/app/App.js
git commit -m "✨ ChangeSSHDirectory 自动启用目录同步并暴露 EnableSSHSync 绑定"
```

---

### Task 5: Frontend lazy-enable in F→T flow

**Files:**
- Modify: `frontend/src/components/terminal/file-manager/useTerminalDirectorySync.ts`
- Test: extend an existing relevant frontend test, or add `frontend/src/__tests__/useTerminalDirectorySync.test.tsx` (mock Wails bindings as in `frontend/src/__tests__/setup.ts`)

- [ ] **Step 1: Failing test — F→T calls EnableSSHSync when sessionSync is missing**

Locate setup mocks:

```bash
grep -rn "EnableSSHSync\|ChangeSSHDirectory" frontend/src/__tests__/ 2>/dev/null
grep -n "vi.mock" frontend/src/__tests__/setup.ts
```

Create `frontend/src/__tests__/useTerminalDirectorySync.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTerminalDirectorySync } from "@/components/terminal/file-manager/useTerminalDirectorySync";

// Mock Wails bindings used by the hook.
const enableSpy = vi.fn().mockResolvedValue(undefined);
const changeSpy = vi.fn().mockResolvedValue(undefined);
vi.mock("../../wailsjs/go/app/App", () => ({
  EnableSSHSync: (...args: unknown[]) => enableSpy(...args),
  ChangeSSHDirectory: (...args: unknown[]) => changeSpy(...args),
}));

// Minimal store shim — mirror the real store's keys touched by the hook.
vi.mock("@/stores/terminalStore", () => {
  const state = {
    tabData: { tab1: { directoryFollowMode: "off", panes: { sess1: { connected: true } } } },
    sessionSync: {} as Record<string, unknown>,
    setDirectoryFollowMode: vi.fn(),
  };
  return {
    useTerminalStore: (selector: (s: typeof state) => unknown) => selector(state),
  };
});

describe("useTerminalDirectorySync lazy enable", () => {
  beforeEach(() => {
    enableSpy.mockClear();
    changeSpy.mockClear();
  });

  it("calls EnableSSHSync before reading cwd when state is missing", async () => {
    const loadDir = vi.fn().mockResolvedValue(true);
    const currentPathRef = { current: "/" };
    const { result } = renderHook(() =>
      useTerminalDirectorySync({ currentPathRef, loadDir, sessionId: "sess1", tabId: "tab1" })
    );
    await act(async () => {
      await result.current.syncPanelFromTerminal();
    });
    expect(enableSpy).toHaveBeenCalledWith("sess1");
  });
});
```

- [ ] **Step 2: Run test, expect failure**

Run: `cd frontend && pnpm test -- useTerminalDirectorySync`
Expected: FAIL — current hook does not call `EnableSSHSync`.

- [ ] **Step 3: Modify the hook to lazy-enable**

Edit `frontend/src/components/terminal/file-manager/useTerminalDirectorySync.ts`. Add the import:

```ts
import { ChangeSSHDirectory, EnableSSHSync } from "../../../../wailsjs/go/app/App";
```

Replace `syncPanelFromTerminal`:

```ts
const syncPanelFromTerminal = useCallback(async () => {
  let sync = sessionSync;
  if (!sync || !sync.supported) {
    try {
      await EnableSSHSync(sessionId);
    } catch (e) {
      showSyncError(e);
      return false;
    }
    sync = useTerminalStore.getState().sessionSync[sessionId];
  }
  if (!sync) {
    showSyncCode(DIRSYNC_ERROR_CODES.CWD_UNKNOWN);
    return false;
  }
  if (!sync.supported) {
    showSyncCode(DIRSYNC_ERROR_CODES.UNSUPPORTED);
    return false;
  }
  if (!sync.cwdKnown || !sync.cwd) {
    showSyncCode(DIRSYNC_ERROR_CODES.CWD_UNKNOWN);
    return false;
  }
  return loadDir(sync.cwd);
}, [loadDir, sessionId, sessionSync, showSyncCode, showSyncError]);
```

Also add `useTerminalStore` direct access import at top:

```ts
import { useTerminalStore } from "@/stores/terminalStore";
```

(already imported — confirm it exposes `getState`. If selector-only, swap to `useTerminalStore.getState`.)

- [ ] **Step 4: Run tests**

```bash
cd frontend && pnpm test -- useTerminalDirectorySync
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/terminal/file-manager/useTerminalDirectorySync.ts frontend/src/__tests__/useTerminalDirectorySync.test.tsx
git commit -m "✨ 终端面板 F→T 首次点击时懒启用目录同步"
```

---

### Task 6: Manual smoke test

**Files:** none (verification only)

- [ ] **Step 1: Build and launch dev**

```bash
make dev
```

- [ ] **Step 2: Connect to a Linux SSH host configured with `PrintLastLog yes` (default)**

In a fresh tab, open a terminal session to any Linux server. Confirm visually:

- "Last login: ..." line appears immediately after the auth banner.
- `/etc/motd` (if present) is displayed.
- Custom banners/issue.net (if configured) display.

- [ ] **Step 3: First F→T click triggers enable**

Click the "panel follows terminal" toggle. Expected:

- A short visible flash of the injection command (this is the known UX cost — accepted for v1).
- After ~100ms, panel loads the current cwd.
- Subsequent F→T clicks have no command flash (already supported).

- [ ] **Step 4: T→F via clicking a folder in the file manager**

Navigate the panel into a subdirectory with auto-follow off, then enable always-follow. Expected: terminal `cd`s to the panel path. If sync was not previously enabled, expect a one-time injection flash followed by the cd.

- [ ] **Step 5: Edge case — user is in `vim` when clicking F→T**

In an active SSH session, run `vim`. While vim is foregrounded, click F→T. Expected:

- The injection bytes are interpreted by vim (cosmetic damage to vim's UI).
- After 3s, frontend toast: "请退出当前程序后重试" (mapped from `dirSyncErrTimeout`).
- Session remains open and usable; sync state remains unsupported.
- Pressing Esc + `:q!` recovers vim. Clicking F→T again works.

- [ ] **Step 6: Disable + reconnect**

Reconnect the session (via the toolbar's reconnect). Expected: full Last login + motd shown again, sync resets to unsupported. Re-enable works.

- [ ] **Step 7: Lint + final commit**

```bash
make lint
cd frontend && pnpm lint
```

If anything is flagged, fix and commit:

```bash
git add -p
git commit -m "🎨 lint pass for SSH lazy sync injection"
```

---

## Out of Scope (call out, do not implement)

- Hiding the injection command echo via cursor-up + clear-line. Could be a v2 polish PR.
- Showing a synthesized banner if `PrintLastLog no` on the server — keep behavior aligned with native sshd (i.e., don't show what the server chose to suppress).
- Caching the shell-type detection across reconnects — a single `sh -lc 'printf %s $SHELL'` is cheap.
- Restoring the previous wrapper as a fallback for "always-on sync from connect" — design choice is to defer hook installation.

## Risks

- **Bracketed paste mode**: some servers' `~/.inputrc` enables bracketed paste in readline. The Ctrl-U + command + `\r` injection is plain text, not paste-bracketed, so it should execute as a typed command. If observed broken on a specific shell config, prepend `\033[?2004l` and append `\033[?2004h` to the enable command.
- **Localized error toasts**: `dirSyncErrTimeout` may not currently map to a user-friendly message. Confirm `frontend/src/lib/dirSyncErrors.ts` includes a mapping; add if missing during Task 5.
- **Probe loop relies on `shellPID`**: with lazy enable, probe doesn't start until init marker arrives. If user never enables sync, probe never runs — desired. If user disables sync, probe stops on next tick (existing logic checks `Supported`).
