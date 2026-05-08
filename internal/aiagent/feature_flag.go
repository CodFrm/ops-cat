package aiagent

import "os"

// Enabled reports whether the cago-backed agent should be used.
// Phase 1 default: opt-in via OPSKAT_AI_BACKEND=cago.
// Phase 2 default: enabled unconditionally; flag becomes a no-op.
func Enabled() bool {
	return os.Getenv("OPSKAT_AI_BACKEND") == "cago"
}
