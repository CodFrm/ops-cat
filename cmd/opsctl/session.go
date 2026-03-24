package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sessionMaxAge = 24 * time.Hour

const opscatDir = ".opskat"

// sessionScope returns a short hash identifying the current terminal/session context.
// Uses terminal session env vars to differentiate concurrent sessions in the same directory.
func sessionScope() string {
	// Check well-known terminal session env vars
	candidates := []string{
		"OPSKAT_SESSION_ID", // our own (desktop app injects this)
		"TERM_SESSION_ID",    // macOS Terminal.app
		"ITERM_SESSION_ID",   // iTerm2
		"WT_SESSION",         // Windows Terminal
		"WINDOWID",           // X11
	}
	for _, key := range candidates {
		if v := os.Getenv(key); v != "" {
			h := sha256.Sum256([]byte(key + "=" + v))
			return fmt.Sprintf("%x", h[:8])
		}
	}
	// Fallback: use "default" scope (single shared session)
	return "default"
}

// sessionFilePath returns the path to the session file for the current scope.
// e.g. .opscat/sessions/a1b2c3d4e5f6
func sessionFilePath(opscatPath string) string {
	return filepath.Join(opscatPath, "sessions", sessionScope())
}

// findOpscatDir walks up from CWD looking for .opscat/ directory.
// Returns empty string if not found.
func findOpscatDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		path := filepath.Join(dir, opscatDir)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// readActiveSession reads the session ID for the current scope.
// Returns empty string if file doesn't exist, is invalid, or has expired (24h).
func readActiveSession() string {
	dir := findOpscatDir()
	if dir == "" {
		return ""
	}
	path := sessionFilePath(dir)
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	// Check expiry by file modification time
	if time.Since(info.ModTime()) > sessionMaxAge {
		_ = os.Remove(path)
		cleanupSessionsDir(dir)
		return ""
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from known .opscat dir
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	if len(id) < 8 {
		return ""
	}
	return id
}

// writeActiveSession writes the session ID for the current scope in CWD.
func writeActiveSession(id string) error {
	sessDir := filepath.Join(opscatDir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessDir, sessionScope()), []byte(id+"\n"), 0644)
}

// resolveSessionID resolves the session ID from flag, env, or session file.
func resolveSessionID(flagSession string) string {
	if flagSession != "" {
		return flagSession
	}
	if env := os.Getenv("OPSKAT_SESSION_ID"); env != "" {
		return env
	}
	return readActiveSession()
}

// cmdSession handles the "session" verb.
func cmdSession(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printSessionUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	switch args[0] {
	case "start":
		return cmdSessionStart()
	case "end":
		return cmdSessionEnd()
	case "status":
		return cmdSessionStatus()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown session subcommand %q\n\nRun 'opsctl session --help' for usage.\n", args[0]) //nolint:gosec // args[0] is from CLI args
		return 1
	}
}

func cmdSessionStart() int {
	id := uuid.New().String()
	if err := writeActiveSession(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write session file: %v\n", err)
		return 1
	}
	fmt.Println(id)
	return 0
}

func cmdSessionEnd() int {
	dir := findOpscatDir()
	if dir == "" {
		fmt.Fprintln(os.Stderr, "No active session.")
		return 0
	}
	path := sessionFilePath(dir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	cleanupSessionsDir(dir)
	fmt.Fprintln(os.Stderr, "Session ended.")
	return 0
}

// cleanupSessionsDir removes sessions/ if empty.
func cleanupSessionsDir(opscatPath string) {
	_ = os.Remove(filepath.Join(opscatPath, "sessions"))
}

func cmdSessionStatus() int {
	id := readActiveSession()
	if id == "" {
		fmt.Fprintln(os.Stderr, "No active session.")
		return 0
	}
	fmt.Println(id)
	return 0
}

func printSessionUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl session <subcommand>

Subcommands:
  start     Create a session and print its ID
  end       End the current session (remove session file)
  status    Show the current session ID

Sessions allow batch approval of write operations. When the desktop app user
approves with "Allow Session", all subsequent operations in the same session
are auto-approved without further dialogs.

Note: Sessions are auto-created on the first write operation if none exists.
You only need 'session start' if you want to explicitly manage the lifecycle.

Storage:
  Session files are stored in .opscat/sessions/<scope> in the current directory.
  The <scope> is derived from terminal env vars (TERM_SESSION_ID, ITERM_SESSION_ID,
  WT_SESSION, WINDOWID) so that different terminal windows in the same directory
  get separate sessions. Sessions expire after 24 hours.

Session ID resolution priority:
  1. --session <id> global flag (explicit)
  2. OPSKAT_SESSION_ID environment variable (desktop app injects this)
  3. .opscat/sessions/<scope> file (auto-created, walks up directory tree)

Examples:
  # Explicit session management
  opsctl session start
  opsctl exec web-01 -- uptime       # reads session from .opscat/sessions/
  opsctl exec web-02 -- df -h        # same session, auto-approved after first allow
  opsctl session end

  # Auto session (no manual steps needed)
  opsctl exec web-01 -- uptime       # auto-creates session on first call
  opsctl exec web-02 -- df -h        # reuses same session

  # Check current session
  opsctl session status
`)
}
