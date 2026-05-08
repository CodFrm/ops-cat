package aiagent

import (
	"sync"

	"github.com/opskat/opskat/internal/ai"
)

// sidecar is a concurrent map that stores the CheckResult produced by the
// PreToolUse policy hook so the PostToolUse audit hook can retrieve it.
// It is keyed by ToolCallID. Each entry is consumed exactly once (drain).
type sidecar struct {
	mu    sync.Mutex
	items map[string]*ai.CheckResult
}

func newSidecar() *sidecar {
	return &sidecar{items: make(map[string]*ai.CheckResult)}
}

// put stores (or replaces) the CheckResult for the given toolCallID.
func (s *sidecar) put(toolCallID string, r *ai.CheckResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[toolCallID] = r
}

// drain retrieves and removes the CheckResult for toolCallID.
// Returns nil when not found.
func (s *sidecar) drain(toolCallID string) *ai.CheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[toolCallID]
	delete(s.items, toolCallID)
	return r
}
