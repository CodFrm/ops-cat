// Package sshkeepalive runs an OpenSSH-style keepalive heartbeat over an
// ssh.Client (or any compatible Pinger), so long-lived SSH sessions don't
// get reaped by NAT/firewall idle timeouts.
package sshkeepalive

import (
	"sync"
	"time"
)

// Interval is the global SSH keepalive heartbeat interval.
const Interval = 60 * time.Second

// Pinger is the subset of *ssh.Client used to send keepalive global requests.
// Defining it as an interface keeps this package decoupled from net/ssh and
// makes it trivial to test with a fake.
type Pinger interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

// Start launches a goroutine that sends an OpenSSH "keepalive@openssh.com"
// global request on p every interval. It returns a stop function the caller
// MUST invoke when shutting down. stop is idempotent.
//
// If SendRequest returns an error, the goroutine exits silently. Start does
// NOT close the underlying connection — the read loop on the client will
// detect EOF and surface it through the existing close path.
func Start(p Pinger, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	stopFn := func() { once.Do(func() { close(done) }) }

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := p.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					return
				}
			}
		}
	}()

	return stopFn
}
