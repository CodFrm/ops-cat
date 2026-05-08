package aiagent

import (
	"testing"

	"github.com/opskat/opskat/internal/ai"
)

func TestSidecar_PutDrain(t *testing.T) {
	s := newSidecar()
	r := &ai.CheckResult{Decision: ai.Allow, MatchedPattern: "ls *"}
	s.put("tool_id_1", r)
	got := s.drain("tool_id_1")
	if got == nil || got.MatchedPattern != "ls *" {
		t.Fatalf("drained = %+v", got)
	}
	// second drain returns nil
	if s.drain("tool_id_1") != nil {
		t.Fatal("drain must remove the entry")
	}
}

func TestSidecar_DrainMissingIsNil(t *testing.T) {
	s := newSidecar()
	if s.drain("missing") != nil {
		t.Fatal("missing key should drain to nil")
	}
}
