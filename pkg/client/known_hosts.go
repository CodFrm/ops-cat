package client

import (
	"errors"
	"fmt"
	"sync"
)

// ErrHostKeyNotFound is returned by KnownHosts.Lookup when the host has not
// been seen before. Callers MUST treat this as "ask the user" (TOFU prompt),
// not as "connect anyway".
var ErrHostKeyNotFound = errors.New("client: host key not in known_hosts")

// ErrHostKeyMismatch is returned when a stored host key does not match the
// key offered by the server. This is a hard failure — the connection MUST be
// aborted.
var ErrHostKeyMismatch = errors.New("client: host key mismatch")

// KnownHosts persists previously-accepted SSH host public keys for trust-on-
// first-use (TOFU) verification. Implementations are caller-supplied: the
// desktop uses an on-disk known_hosts file, the mobile client stores entries
// in SQLite. Implementations MUST be safe for concurrent use.
type KnownHosts interface {
	// Lookup returns the stored key for the given host:port, or
	// ErrHostKeyNotFound if no entry exists. The returned slice is the raw
	// SSH wire-format public key (ssh.PublicKey.Marshal()), not the base64
	// authorized_keys form.
	Lookup(host string, port int) ([]byte, error)

	// Save records the host key as accepted. Overwrites any existing entry
	// for the same host:port — callers should only call this after the user
	// has explicitly approved the fingerprint.
	Save(host string, port int, publicKey []byte) error
}

// NewInMemoryKnownHosts returns an in-memory KnownHosts. Suitable for tests
// and for the mobile bridge's first iteration before SQLite is wired up.
// Entries are lost on process restart.
func NewInMemoryKnownHosts() KnownHosts {
	return &inMemoryKnownHosts{entries: make(map[string][]byte)}
}

type inMemoryKnownHosts struct {
	mu      sync.RWMutex
	entries map[string][]byte
}

func (m *inMemoryKnownHosts) Lookup(host string, port int) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key, ok := m.entries[knownHostsKey(host, port)]
	if !ok {
		return nil, ErrHostKeyNotFound
	}
	out := make([]byte, len(key))
	copy(out, key)
	return out, nil
}

func (m *inMemoryKnownHosts) Save(host string, port int, publicKey []byte) error {
	if len(publicKey) == 0 {
		return errors.New("client: refuse to save empty host key")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := make([]byte, len(publicKey))
	copy(stored, publicKey)
	m.entries[knownHostsKey(host, port)] = stored
	return nil
}

func knownHostsKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
