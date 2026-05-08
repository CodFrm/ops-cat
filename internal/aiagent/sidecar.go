package aiagent

import (
	"sync"

	"github.com/opskat/opskat/internal/ai"
)

// sidecar threads *ai.CheckResult from PreToolUse to PostToolUse keyed by
// cago's ToolEvent.ID. cago HookInput has no per-call sidecar field; this
// fills the gap. One sidecar instance per *coding.System.
type sidecar struct {
	m sync.Map // map[string]*ai.CheckResult
}

func newSidecar() *sidecar { return &sidecar{} }

func (s *sidecar) put(toolID string, r *ai.CheckResult) {
	if toolID == "" || r == nil {
		return
	}
	s.m.Store(toolID, r)
}

func (s *sidecar) drain(toolID string) *ai.CheckResult {
	if toolID == "" {
		return nil
	}
	v, ok := s.m.LoadAndDelete(toolID)
	if !ok {
		return nil
	}
	return v.(*ai.CheckResult)
}
