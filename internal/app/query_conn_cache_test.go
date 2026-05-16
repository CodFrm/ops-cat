package app

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// --- 测试桩 ---

type fakeClient struct {
	id     int64
	closed atomic.Bool
}

func (f *fakeClient) Close() error {
	if f.closed.Swap(true) {
		return errors.New("already closed")
	}
	return nil
}

type fakeCloser struct {
	closed atomic.Bool
}

func (f *fakeCloser) Close() error {
	f.closed.Store(true)
	return nil
}

// --- 测试 ---

func TestPanelConnCache_GetOrDial_CachesAndReuses(t *testing.T) {
	Convey("GetOrDial 命中缓存时不重复 dial", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)
		defer func() { _ = c.Close() }()

		var dialCount atomic.Int32
		dial := func() (*fakeClient, io.Closer, error) {
			dialCount.Add(1)
			return &fakeClient{id: 1}, &fakeCloser{}, nil
		}

		client1, _, err := c.GetOrDial("1:db", dial)
		So(err, ShouldBeNil)
		So(client1, ShouldNotBeNil)
		So(dialCount.Load(), ShouldEqual, 1)

		client2, _, err := c.GetOrDial("1:db", dial)
		So(err, ShouldBeNil)
		So(client2, ShouldEqual, client1)
		So(dialCount.Load(), ShouldEqual, 1)
	})
}
func TestPanelConnCache_GetOrDial_ConcurrentSameKeyDialsOnce(t *testing.T) {
	Convey("同 key 并发首拨经 singleflight 合流为一次", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)
		defer func() { _ = c.Close() }()

		var dialCount atomic.Int32
		var dialErr error
		start := make(chan struct{})
		dial := func() (*fakeClient, io.Closer, error) {
			dialCount.Add(1)
			time.Sleep(20 * time.Millisecond)
			return &fakeClient{id: 1}, &fakeCloser{}, dialErr
		}

		var wg sync.WaitGroup
		var firstClient *fakeClient
		var mu sync.Mutex
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				client, _, err := c.GetOrDial("1:db", dial)
				assert.NoError(t, err)
				mu.Lock()
				if firstClient == nil {
					firstClient = client
				} else {
					assert.Equal(t, firstClient, client)
				}
				mu.Unlock()
			}()
		}
		close(start)
		wg.Wait()
		So(dialCount.Load(), ShouldEqual, 1)
	})
}

func TestPanelConnCache_DifferentKeysIndependent(t *testing.T) {
	Convey("不同 key 独立缓存", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)
		defer func() { _ = c.Close() }()

		dialA := func() (*fakeClient, io.Closer, error) {
			return &fakeClient{id: 1}, nil, nil
		}
		dialB := func() (*fakeClient, io.Closer, error) {
			return &fakeClient{id: 2}, nil, nil
		}

		a, _, _ := c.GetOrDial("1:db1", dialA)
		b, _, _ := c.GetOrDial("2:db2", dialB)
		So(a.id, ShouldEqual, 1)
		So(b.id, ShouldEqual, 2)
		So(a, ShouldNotEqual, b)
	})
}

func TestPanelConnCache_DialError_NotCached(t *testing.T) {
	Convey("dial 失败时不进缓存,下次重试", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)
		defer func() { _ = c.Close() }()

		var attempts atomic.Int32
		dial := func() (*fakeClient, io.Closer, error) {
			n := attempts.Add(1)
			if n == 1 {
				return nil, nil, errors.New("boom")
			}
			return &fakeClient{id: 1}, nil, nil
		}

		_, _, err := c.GetOrDial("1:db", dial)
		So(err, ShouldNotBeNil)

		client, _, err := c.GetOrDial("1:db", dial)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)
		So(attempts.Load(), ShouldEqual, 2)
	})
}

func TestPanelConnCache_Drop_ClosesAndRemoves(t *testing.T) {
	Convey("Drop 关闭并移除指定 key", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)
		defer func() { _ = c.Close() }()

		client := &fakeClient{id: 1}
		tunnel := &fakeCloser{}
		dial := func() (*fakeClient, io.Closer, error) {
			return client, tunnel, nil
		}
		_, _, err := c.GetOrDial("1:db", dial)
		So(err, ShouldBeNil)

		c.Drop("1:db")
		So(client.closed.Load(), ShouldBeTrue)
		So(tunnel.closed.Load(), ShouldBeTrue)

		// 下次 GetOrDial 应该重新 dial
		newClient := &fakeClient{id: 2}
		dial2 := func() (*fakeClient, io.Closer, error) {
			return newClient, nil, nil
		}
		got, _, err := c.GetOrDial("1:db", dial2)
		So(err, ShouldBeNil)
		So(got, ShouldEqual, newClient)
	})
}

func TestPanelConnCache_Close_ReleasesAll(t *testing.T) {
	Convey("Close 释放所有缓存的 client 和 tunnel", t, func() {
		c := newPanelConnCache[*fakeClient]("test", time.Minute)

		clientA := &fakeClient{id: 1}
		clientB := &fakeClient{id: 2}
		tunnelA := &fakeCloser{}
		tunnelB := &fakeCloser{}

		_, _, _ = c.GetOrDial("1:db", func() (*fakeClient, io.Closer, error) {
			return clientA, tunnelA, nil
		})
		_, _, _ = c.GetOrDial("2:db", func() (*fakeClient, io.Closer, error) {
			return clientB, tunnelB, nil
		})

		err := c.Close()
		So(err, ShouldBeNil)
		So(clientA.closed.Load(), ShouldBeTrue)
		So(clientB.closed.Load(), ShouldBeTrue)
		So(tunnelA.closed.Load(), ShouldBeTrue)
		So(tunnelB.closed.Load(), ShouldBeTrue)
	})
}

func TestPanelConnCache_Evictor_EvictsIdle(t *testing.T) {
	Convey("idle 超过 TTL 后被 evictor 驱逐", t, func() {
		c := newPanelConnCache[*fakeClient]("test", 50*time.Millisecond)
		defer func() { _ = c.Close() }()

		client := &fakeClient{id: 1}
		_, _, _ = c.GetOrDial("1:db", func() (*fakeClient, io.Closer, error) {
			return client, nil, nil
		})

		// 触发一次驱逐扫描,模拟时间过去
		c.evictOnce(time.Now().Add(time.Second))

		So(client.closed.Load(), ShouldBeTrue)

		// 缓存里应该已经没有这个 key 了
		So(c.size(), ShouldEqual, 0)
	})
}
