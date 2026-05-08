package aiagent

import (
	"testing"
)

func TestNewDeps_PopulatesPerConversationCachesAndKafka(t *testing.T) {
	d := NewDeps(nil, nil)
	if d == nil {
		t.Fatal("NewDeps returned nil")
	}
	if d.SSHCache == nil {
		t.Error("SSHCache not initialized")
	}
	if d.MongoCache == nil {
		t.Error("MongoCache not initialized")
	}
	if d.KafkaService == nil {
		t.Error("KafkaService not initialized")
	}
}

// TestDeps_CloseIsIdempotent guards the per-Conversation lifecycle promise:
// System.Close calls Deps.Close exactly once, but a defensive double-close
// (e.g. errored construction path) must not panic or fail.
func TestDeps_CloseIsIdempotent(t *testing.T) {
	d := NewDeps(nil, nil)
	if err := d.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if d.KafkaService != nil || d.MongoCache != nil || d.SSHCache != nil {
		t.Errorf("Close left non-nil fields: kafka=%v mongo=%v ssh=%v",
			d.KafkaService != nil, d.MongoCache != nil, d.SSHCache != nil)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDeps_CloseOnZeroValueIsSafe(t *testing.T) {
	var d Deps
	if err := d.Close(); err != nil {
		t.Fatalf("zero-value Close should be safe, got %v", err)
	}
}
