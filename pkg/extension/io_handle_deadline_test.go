package extension

import (
	"errors"
	"net"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIOHandleManagerSetDeadline(t *testing.T) {
	Convey("Given an IOHandleManager", t, func() {
		m := NewIOHandleManager()

		Convey("When registering a net.Conn", func() {
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close() //nolint:errcheck
			defer clientConn.Close() //nolint:errcheck
			id, err := m.Register(clientConn, clientConn, clientConn, IOMeta{})
			So(err, ShouldBeNil)

			Convey("SetDeadline with kind=both should succeed", func() {
				err := m.SetDeadline(id, "both", time.Now().Add(5*time.Second))
				So(err, ShouldBeNil)
			})
			Convey("SetDeadline with kind=read should succeed", func() {
				err := m.SetDeadline(id, "read", time.Now().Add(5*time.Second))
				So(err, ShouldBeNil)
			})
			Convey("SetDeadline with kind=unknown should error", func() {
				err := m.SetDeadline(id, "bogus", time.Time{})
				So(err, ShouldNotBeNil)
			})

			Convey("A read deadline in the past causes Read to time out", func() {
				err := m.SetDeadline(id, "read", time.Now().Add(-1*time.Second))
				So(err, ShouldBeNil)

				buf := make([]byte, 8)
				_, readErr := m.Read(id, buf)
				So(readErr, ShouldNotBeNil)
				var netErr net.Error
				So(errors.As(readErr, &netErr), ShouldBeTrue)
				So(netErr.Timeout(), ShouldBeTrue)
			})

			Convey("Clearing deadline (zero time) re-enables blocking Read", func() {
				// Arm a past deadline, then clear it. After clearing, a read should
				// block until data arrives rather than returning immediately.
				So(m.SetDeadline(id, "read", time.Now().Add(-1*time.Second)), ShouldBeNil)
				buf := make([]byte, 8)
				_, readErr := m.Read(id, buf)
				So(readErr, ShouldNotBeNil) // timed out
				// Clear it.
				So(m.SetDeadline(id, "read", time.Time{}), ShouldBeNil)
				// Send data from the server side and verify Read gets it.
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = serverConn.Write([]byte("hi"))
				}()
				n, err := m.Read(id, buf)
				So(err, ShouldBeNil)
				So(string(buf[:n]), ShouldEqual, "hi")
				<-done
			})
		})

		Convey("When registering a plain reader without deadline support", func() {
			pc := plainNoDeadline{}
			id, err := m.Register(pc, pc, pc, IOMeta{})
			So(err, ShouldBeNil)

			Convey("SetDeadline should return deadline-unsupported error", func() {
				err := m.SetDeadline(id, "both", time.Time{})
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "deadlines")
			})
		})
	})
}

// TestDefaultHostProviderSetDeadline exercises the unixNanos=0 → clear
// behavior at the HostProvider level (the WASM ABI passes a zero-nanos
// sentinel which must become a zero time.Time).
func TestDefaultHostProviderSetDeadline(t *testing.T) {
	Convey("Given a DefaultHostProvider with a TCP handle", t, func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		defer ln.Close() //nolint:errcheck

		accepted := make(chan net.Conn, 1)
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}()

		h := NewDefaultHostProvider(DefaultHostConfig{})
		id, _, err := h.IOOpen(IOOpenParams{Type: "tcp", Addr: ln.Addr().String()})
		So(err, ShouldBeNil)
		serverConn := <-accepted
		defer serverConn.Close() //nolint:errcheck

		Convey("IOSetDeadline with unixNanos=0 clears any existing deadline", func() {
			// Arm a past deadline via the host interface.
			past := time.Now().Add(-1 * time.Second).UnixNano()
			So(h.IOSetDeadline(id, "read", past), ShouldBeNil)
			_, readErr := h.IORead(id, 8)
			So(readErr, ShouldNotBeNil)

			// Clear by passing 0.
			So(h.IOSetDeadline(id, "read", 0), ShouldBeNil)
			// After clearing, a subsequent read with data available must succeed.
			_, _ = serverConn.Write([]byte("ok"))
			data, err := h.IORead(id, 8)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "ok")
		})

		Convey("IOSetDeadline with unknown kind returns error", func() {
			So(h.IOSetDeadline(id, "bogus", 0), ShouldNotBeNil)
		})

		So(h.IOClose(id), ShouldBeNil)
	})
}

// Package-level test helper: Reader+Writer+Closer without deadline methods.
type plainNoDeadline struct{}

func (plainNoDeadline) Read(p []byte) (int, error)  { return 0, nil }
func (plainNoDeadline) Write(p []byte) (int, error) { return len(p), nil }
func (plainNoDeadline) Close() error                { return nil }
