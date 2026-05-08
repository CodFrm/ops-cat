package aiagent

import (
	"testing"
)

func TestEnabled_DefaultFalse(t *testing.T) {
	t.Setenv("OPSKAT_AI_BACKEND", "")
	if Enabled() {
		t.Fatal("default should be false during Phase 1")
	}
}

func TestEnabled_TrueWhenEnvCago(t *testing.T) {
	t.Setenv("OPSKAT_AI_BACKEND", "cago")
	if !Enabled() {
		t.Fatal("OPSKAT_AI_BACKEND=cago should enable")
	}
}
