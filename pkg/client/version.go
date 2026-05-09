// Package client is the public Go API for embedding opskat capabilities into
// other binaries (mobile clients via gomobile, future SDK consumers, etc.).
//
// Anything mobile-specific (gomobile bind targets, Dart-bridge debug helpers)
// belongs in the opskat-mobile repository, not here.
package client

import "github.com/opskat/opskat/internal/buildinfo"

// Version returns the opskat build's version string.
func Version() string {
	commit := buildinfo.ShortCommitID()
	if commit == "" {
		commit = "dev"
	}
	return "opskat " + commit
}
