package extension

import (
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
			defer serverConn.Close()
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

// Package-level test helper: Reader+Writer+Closer without deadline methods.
type plainNoDeadline struct{}

func (plainNoDeadline) Read(p []byte) (int, error)  { return 0, nil }
func (plainNoDeadline) Write(p []byte) (int, error) { return len(p), nil }
func (plainNoDeadline) Close() error                { return nil }
