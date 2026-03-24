//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// watchTerminalResize starts a goroutine that watches for SIGWINCH signals
// and sends window-change requests to the SSH session.
// Returns a stop function to clean up.
func watchTerminalResize(session *ssh.Session, fd int) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				w, h, err := term.GetSize(fd)
				if err == nil {
					_ = session.WindowChange(h, w)
				}
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(done)
	}
}
