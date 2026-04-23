package extension

import (
	"net"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIOOpenTCP(t *testing.T) {
	Convey("Given a DefaultHostProvider", t, func() {
		// Start a trivial TCP echo server
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		defer ln.Close() //nolint:errcheck

		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() //nolint:errcheck
			buf := make([]byte, 1024)
			n, _ := c.Read(buf)
			_, _ = c.Write(buf[:n]) // echo server; write failure is irrelevant in test
		}()

		h := NewDefaultHostProvider(DefaultHostConfig{})

		Convey("IOOpen(tcp) should succeed with valid addr", func() {
			id, _, err := h.IOOpen(IOOpenParams{Type: "tcp", Addr: ln.Addr().String()})
			So(err, ShouldBeNil)
			So(id, ShouldBeGreaterThan, uint32(0))

			Convey("Write and Read should round-trip", func() {
				n, err := h.IOWrite(id, []byte("ping"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 4)

				data, err := h.IORead(id, 16)
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, "ping")

				So(h.IOClose(id), ShouldBeNil)
			})
		})

		Convey("IOOpen(tcp) with invalid addr should fail", func() {
			_, _, err := h.IOOpen(IOOpenParams{Type: "tcp", Addr: "localhost:1"})
			So(err, ShouldNotBeNil)
		})
	})
}

// fakeTunnelDialer records calls and hands back one side of a TCP loopback
// pair so the rest of the host IO stack works naturally. We use TCP rather
// than net.Pipe because the host stores the returned conn as a deadliner and
// TCP exercises the same interface the real SSH tunnel dialer returns.
type fakeTunnelDialer struct {
	wantID  int64
	calls   atomic.Int32
	gotID   atomic.Int64
	gotAddr atomic.Value // string

	// A pre-accepted TCP pair: client is handed back to the host; peer is kept
	// by the test to drive the other side of the conversation.
	clientConn net.Conn
	peerConn   net.Conn
}

func newFakeTunnelDialer(t *testing.T, wantID int64) *fakeTunnelDialer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close() //nolint:errcheck

	dialDone := make(chan net.Conn, 1)
	go func() {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			dialDone <- nil
			return
		}
		dialDone <- c
	}()
	peer, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	client := <-dialDone
	if client == nil {
		t.Fatal("dial failed")
	}
	return &fakeTunnelDialer{wantID: wantID, clientConn: client, peerConn: peer}
}

func (f *fakeTunnelDialer) Dial(tunnelID int64, addr string) (net.Conn, error) {
	f.calls.Add(1)
	f.gotID.Store(tunnelID)
	f.gotAddr.Store(addr)
	return f.clientConn, nil
}

func (f *fakeTunnelDialer) Close() {
	if f.peerConn != nil {
		_ = f.peerConn.Close()
	}
}

// TestIOOpenTCPTunnelPath verifies that when AssetSSHTunnelID > 0 is set,
// openTCP routes through TunnelDialer instead of net.Dial — this path is
// first-party only (Kafka extension), so a regression would go unnoticed.
func TestIOOpenTCPTunnelPath(t *testing.T) {
	Convey("Given a DefaultHostProvider with a TunnelDialer", t, func() {
		const tunnelID int64 = 42
		fake := newFakeTunnelDialer(t, tunnelID)
		defer fake.Close()

		h := NewDefaultHostProvider(DefaultHostConfig{
			TunnelDialer:     fake,
			AssetSSHTunnelID: tunnelID,
		})

		Convey("IOOpen(tcp) routes through the tunnel dialer", func() {
			id, _, err := h.IOOpen(IOOpenParams{
				Type:    "tcp",
				Addr:    "kafka.internal:9092",
				Timeout: 1, // 1ms — doc says tunnel path ignores this
			})
			So(err, ShouldBeNil)
			So(id, ShouldBeGreaterThan, uint32(0))
			So(fake.calls.Load(), ShouldEqual, int32(1))
			So(fake.gotID.Load(), ShouldEqual, tunnelID)
			So(fake.gotAddr.Load().(string), ShouldEqual, "kafka.internal:9092")

			// Data flows through the returned handle — write on host side,
			// read on peer side.
			n, err := h.IOWrite(id, []byte("hello"))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 5)

			buf := make([]byte, 16)
			rn, err := fake.peerConn.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf[:rn]), ShouldEqual, "hello")

			So(h.IOClose(id), ShouldBeNil)
		})
	})
}
