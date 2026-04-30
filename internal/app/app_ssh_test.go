package app

import (
	"testing"

	"github.com/opskat/opskat/internal/pkg/dirsync"
	"github.com/opskat/opskat/internal/service/ssh_svc"
)

func TestChangeSSHDirectoryReturnsSessionNotFoundForUnknownID(t *testing.T) {
	a := &App{sshManager: ssh_svc.NewManager()}
	err := a.ChangeSSHDirectory("nonexistent", "/tmp")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != dirsync.CodeSessionNotFound {
		t.Fatalf("expected SessionNotFound code, got %q", err.Error())
	}
}

func TestEnableSSHSyncReturnsSessionNotFoundForUnknownID(t *testing.T) {
	a := &App{sshManager: ssh_svc.NewManager()}
	err := a.EnableSSHSync("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != dirsync.CodeSessionNotFound {
		t.Fatalf("expected SessionNotFound code, got %q", err.Error())
	}
}
