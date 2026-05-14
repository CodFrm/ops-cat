package app

import (
	"path/filepath"
	"testing"
)

func TestResolveRelaunchTargetFromExecutablePath(t *testing.T) {
	t.Parallel()

	t.Run("mac app bundle", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("darwin", "/Applications/OpsKat.app/Contents/MacOS/opskat")
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if got, want := filepath.ToSlash(target.appBundlePath), "/Applications/OpsKat.app"; got != want {
			t.Fatalf("appBundlePath = %q, want %q", got, want)
		}
	})

	t.Run("mac app backup bundle", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("darwin", "/Applications/OpsKat.app.backup/Contents/MacOS/opskat")
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if got, want := filepath.ToSlash(target.appBundlePath), "/Applications/OpsKat.app"; got != want {
			t.Fatalf("appBundlePath = %q, want %q", got, want)
		}
	})

	t.Run("mac non bundle", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("darwin", "/tmp/opskat")
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if target.appBundlePath != "" {
			t.Fatalf("appBundlePath = %q, want empty", target.appBundlePath)
		}
		if target.executablePath != "/tmp/opskat" {
			t.Fatalf("executablePath = %q, want /tmp/opskat", target.executablePath)
		}
	})

	t.Run("mac non bundle backup executable", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("darwin", "/tmp/opskat.backup")
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if target.executablePath != "/tmp/opskat" {
			t.Fatalf("executablePath = %q, want /tmp/opskat", target.executablePath)
		}
	})

	t.Run("windows old executable", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("windows", `C:\Users\me\AppData\Local\OpsKat\opskat.exe.old`)
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if want := `C:\Users\me\AppData\Local\OpsKat\opskat.exe`; target.executablePath != want {
			t.Fatalf("executablePath = %q, want %q", target.executablePath, want)
		}
	})

	t.Run("linux deleted backup executable", func(t *testing.T) {
		t.Parallel()

		target, err := resolveRelaunchTargetFromExecutablePath("linux", "/opt/opskat/opskat.backup (deleted)")
		if err != nil {
			t.Fatalf("resolveRelaunchTargetFromExecutablePath() error = %v", err)
		}
		if target.executablePath != "/opt/opskat/opskat" {
			t.Fatalf("executablePath = %q, want /opt/opskat/opskat", target.executablePath)
		}
	})
}
