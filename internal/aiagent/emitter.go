package aiagent

import "github.com/opskat/opskat/internal/ai"

// EventEmitter is the abstraction over Wails event delivery used by the hooks
// and the bridge. internal/app implements it by calling wailsRuntime.EventsEmit.
// Tests substitute a recording fake.
type EventEmitter interface {
	Emit(convID int64, event ai.StreamEvent)
}

// EmitterFunc adapts a function to the EventEmitter interface.
type EmitterFunc func(convID int64, event ai.StreamEvent)

func (f EmitterFunc) Emit(convID int64, event ai.StreamEvent) { f(convID, event) }
