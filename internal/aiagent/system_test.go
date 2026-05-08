package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/provider/providertest"

	"github.com/opskat/opskat/internal/ai"
)

func TestSystem_NewClose_Smoke(t *testing.T) {
	mock := providertest.New()

	sys, err := NewSystem(context.Background(), SystemOptions{
		Provider:      mock,
		Cwd:           t.TempDir(), // empty tmpdir avoids host CLAUDE.md / skill pollution
		ConvID:        1,
		Lang:          "en",
		Deps:          &Deps{},
		Emitter:       EmitterFunc(func(int64, ai.StreamEvent) {}),
		PolicyChecker: nil, // smoke does not exercise tool dispatch
	})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	if sys == nil {
		t.Fatal("NewSystem returned nil System")
	}
	if err := sys.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Idempotency: Close again is a no-op (cago coding.System uses sync.Once).
	if err := sys.Close(context.Background()); err != nil {
		t.Fatalf("Close (second): %v", err)
	}
}
