package extension

import "sync/atomic"

// ActionCancellation is a flag polled by long-running actions via
// host_action_should_stop. Set by the host when the frontend requests cancel.
type ActionCancellation struct {
	stop atomic.Bool
}

func NewActionCancellation() *ActionCancellation {
	return &ActionCancellation{}
}

// Cancel marks the action as canceled. Idempotent.
func (c *ActionCancellation) Cancel() {
	c.stop.Store(true)
}

// ShouldStop returns true if Cancel has been called.
func (c *ActionCancellation) ShouldStop() bool {
	return c.stop.Load()
}
