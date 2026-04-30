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
	if !strings.Contains(cmd, "OPSKAT_PROMPT_NONCE='NONCE'") {
		t.Fatalf("missing nonce assignment (shellQuote'd): %s", cmd)
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

func TestBuildEnableSyncCommandZsh(t *testing.T) {
	cmd := buildEnableSyncCommand(shellTypeZsh, "TOK", "NONCE")
	if !strings.Contains(cmd, "add-zsh-hook precmd opskat_prompt_proof") {
		t.Fatalf("missing add-zsh-hook: %s", cmd)
	}
	if !strings.Contains(cmd, "1337;opskat:TOK:init:pid:") {
		t.Fatalf("missing init marker: %s", cmd)
	}
	if strings.ContainsAny(cmd, "\n") {
		t.Fatalf("must be single line: %q", cmd)
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
	if strings.ContainsAny(cmd, "\n") {
		t.Fatalf("must be single line: %q", cmd)
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
			t.Fatalf("disable cmd for %s should be non-empty and end in \\r, got %q", sh, got)
		}
		if strings.ContainsAny(got, "\n") {
			t.Fatalf("must be single line: %q", got)
		}
	}
	if buildDisableSyncCommand(shellTypeUnsupported) != "" {
		t.Fatal("unsupported should return empty")
	}
}
