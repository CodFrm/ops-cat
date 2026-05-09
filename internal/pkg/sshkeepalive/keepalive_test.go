package sshkeepalive

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type fakePinger struct {
	mu       sync.Mutex
	count    int32
	returnFn func() error
}

func (f *fakePinger) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	atomic.AddInt32(&f.count, 1)
	f.mu.Lock()
	fn := f.returnFn
	f.mu.Unlock()
	if fn != nil {
		return false, nil, fn()
	}
	return true, nil, nil
}

func (f *fakePinger) calls() int32 {
	return atomic.LoadInt32(&f.count)
}

func TestStart(t *testing.T) {
	Convey("Start sends keepalive ticks", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		defer stop()

		time.Sleep(55 * time.Millisecond)
		So(fp.calls(), ShouldBeGreaterThanOrEqualTo, 3)
	})

	Convey("Start does not fire before the first interval", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 100*time.Millisecond)
		defer stop()

		time.Sleep(20 * time.Millisecond)
		So(fp.calls(), ShouldEqual, 0)
	})

	Convey("stop halts the ticker", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		time.Sleep(35 * time.Millisecond)
		stop()
		baseline := fp.calls()

		time.Sleep(50 * time.Millisecond)
		So(fp.calls(), ShouldEqual, baseline)
	})

	Convey("stop is idempotent", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		stop()
		stop() // must not panic
		stop()
		So(true, ShouldBeTrue)
	})

	Convey("ping error stops the goroutine", t, func() {
		fp := &fakePinger{returnFn: func() error { return errors.New("boom") }}
		stop := Start(fp, 5*time.Millisecond)
		defer stop()

		time.Sleep(40 * time.Millisecond)
		// First failing call exits the goroutine; allow at most a couple
		// scheduled ticks before the error is observed.
		So(fp.calls(), ShouldBeLessThanOrEqualTo, 2)
	})
}
