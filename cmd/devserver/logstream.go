package main

import (
	"encoding/json"
	"sync"
	"time"
)

// LogEntry represents a single log event from host function calls.
type LogEntry struct {
	Time      time.Time      `json:"time"`
	Type      string         `json:"type"` // log, http, kv_get, kv_set, credential_get, asset_config, event
	Extension string         `json:"extension,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// LogStream is a thread-safe ring buffer with SSE broadcast support.
type LogStream struct {
	entries []LogEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex

	// SSE subscribers
	subs   map[chan LogEntry]struct{}
	subsMu sync.RWMutex
}

func NewLogStream(size int) *LogStream {
	return &LogStream{
		entries: make([]LogEntry, size),
		size:    size,
		subs:    make(map[chan LogEntry]struct{}),
	}
}

// Push adds a log entry and broadcasts to SSE subscribers.
func (s *LogStream) Push(entry LogEntry) {
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	s.mu.Lock()
	s.entries[s.head] = entry
	s.head = (s.head + 1) % s.size
	if s.count < s.size {
		s.count++
	}
	s.mu.Unlock()

	s.subsMu.RLock()
	for ch := range s.subs {
		select {
		case ch <- entry:
		default: // drop if subscriber is slow
		}
	}
	s.subsMu.RUnlock()
}

// Recent returns the most recent entries.
func (s *LogStream) Recent(n int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if n > s.count {
		n = s.count
	}
	result := make([]LogEntry, n)
	start := (s.head - n + s.size) % s.size
	for i := range n {
		result[i] = s.entries[(start+i)%s.size]
	}
	return result
}

// Subscribe returns a channel that receives new log entries. Call Unsubscribe when done.
func (s *LogStream) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 64)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (s *LogStream) Unsubscribe(ch chan LogEntry) {
	s.subsMu.Lock()
	delete(s.subs, ch)
	s.subsMu.Unlock()
	close(ch)
}

func (e LogEntry) JSON() []byte {
	data, _ := json.Marshal(e)
	return data
}
