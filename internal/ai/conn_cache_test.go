package ai

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

// mockCloser 用于测试的简单 Closer
type mockCloser struct {
	closed bool
}

func (m *mockCloser) Close() error {
	m.closed = true
	return nil
}

func TestConnCache(t *testing.T) {
	Convey("ConnCache 泛型连接缓存", t, func() {
		cache := NewConnCache[*mockCloser]("test")

		Convey("GetOrDial - 第一次调用执行 dial 并缓存，返回 nil closer", func() {
			client := &mockCloser{}
			dialCalled := 0
			dial := func() (*mockCloser, io.Closer, error) {
				dialCalled++
				return client, nil, nil
			}

			got, closer, err := cache.GetOrDial(1, dial)

			assert.NoError(t, err)
			assert.Equal(t, client, got)
			assert.Nil(t, closer)
			assert.Equal(t, 1, dialCalled)
		})

		Convey("GetOrDial - 相同 assetID 第二次调用返回缓存，不再执行 dial", func() {
			client := &mockCloser{}
			dialCalled := 0
			dial := func() (*mockCloser, io.Closer, error) {
				dialCalled++
				return client, nil, nil
			}

			got1, _, err1 := cache.GetOrDial(1, dial)
			assert.NoError(t, err1)
			assert.Equal(t, client, got1)
			assert.Equal(t, 1, dialCalled)

			got2, closer2, err2 := cache.GetOrDial(1, dial)
			assert.NoError(t, err2)
			assert.Equal(t, client, got2)
			assert.Nil(t, closer2)
			assert.Equal(t, 1, dialCalled) // dial 不应再次被调用
		})

		Convey("GetOrDial - 不同 assetID 分别缓存", func() {
			client1 := &mockCloser{}
			client2 := &mockCloser{}
			dial1 := func() (*mockCloser, io.Closer, error) {
				return client1, nil, nil
			}
			dial2 := func() (*mockCloser, io.Closer, error) {
				return client2, nil, nil
			}

			got1, _, err1 := cache.GetOrDial(1, dial1)
			got2, _, err2 := cache.GetOrDial(2, dial2)

			assert.NoError(t, err1)
			assert.NoError(t, err2)
			assert.Equal(t, client1, got1)
			assert.Equal(t, client2, got2)
			assert.NotSame(t, got1, got2)
		})

		Convey("GetOrDial - dial 返回错误时传播错误", func() {
			dialErr := errors.New("connection refused")
			dial := func() (*mockCloser, io.Closer, error) {
				return nil, nil, dialErr
			}

			got, closer, err := cache.GetOrDial(1, dial)

			assert.Error(t, err)
			assert.Equal(t, dialErr, err)
			assert.Nil(t, got)
			assert.Nil(t, closer)
		})

		Convey("Close - 关闭所有缓存的客户端和 tunnel closer", func() {
			client1 := &mockCloser{}
			client2 := &mockCloser{}
			tunnel1 := &mockCloser{}

			dial1 := func() (*mockCloser, io.Closer, error) {
				return client1, tunnel1, nil
			}
			dial2 := func() (*mockCloser, io.Closer, error) {
				return client2, nil, nil
			}

			_, _, err1 := cache.GetOrDial(1, dial1)
			_, _, err2 := cache.GetOrDial(2, dial2)
			assert.NoError(t, err1)
			assert.NoError(t, err2)

			err := cache.Close()
			assert.NoError(t, err)
			assert.True(t, client1.closed)
			assert.True(t, client2.closed)
			assert.True(t, tunnel1.closed)
		})

		Convey("Remove - 关闭并移除指定 assetID 的缓存连接", func() {
			client1 := &mockCloser{}
			client2 := &mockCloser{}
			tunnel1 := &mockCloser{}

			dial1 := func() (*mockCloser, io.Closer, error) {
				return client1, tunnel1, nil
			}
			dial2 := func() (*mockCloser, io.Closer, error) {
				return client2, nil, nil
			}

			_, _, err1 := cache.GetOrDial(1, dial1)
			_, _, err2 := cache.GetOrDial(2, dial2)
			assert.NoError(t, err1)
			assert.NoError(t, err2)

			// Remove assetID=1，只关闭 client1 和 tunnel1
			cache.Remove(1)
			assert.True(t, client1.closed)
			assert.True(t, tunnel1.closed)
			assert.False(t, client2.closed)

			// 再次 GetOrDial(1) 应重新调用 dial
			newClient := &mockCloser{}
			dialCalled := 0
			dialNew := func() (*mockCloser, io.Closer, error) {
				dialCalled++
				return newClient, nil, nil
			}
			got, _, err := cache.GetOrDial(1, dialNew)
			assert.NoError(t, err)
			assert.Equal(t, newClient, got)
			assert.Equal(t, 1, dialCalled)
		})

		Convey("Remove - 对不存在的 assetID 无副作用", func() {
			cache.Remove(999) // 不应 panic
		})
	})
}

// TestConnCache_ConcurrentSameAsset 复刻线上场景：cago 把同一轮里的两个 run_command
// 并行派发到同一 assetID。曾经 GetOrDial 的两张 map 没锁，会同时 read/write 触发
// "fatal error: concurrent map writes"，或者两路都 miss 各 dial 一份连接 + 互相覆盖
// map → 多余连接泄漏。现在加 mu + per-asset single-flight 保证：
//
//   - 全部并发调用只触发 1 次 dial（second goroutine 等 inflight 后命中缓存）；
//   - 拿到的 *Closer 全是同一份；
//   - 不会发生 race（go test -race 必过）。
func TestConnCache_ConcurrentSameAsset(t *testing.T) {
	cache := NewConnCache[*mockCloser]("test")

	const N = 32
	var dialCalls atomic.Int32
	dial := func() (*mockCloser, io.Closer, error) {
		dialCalls.Add(1)
		// 模拟一段网络握手；如果没有 single-flight，多个 goroutine 会在这里
		// 同时跑完然后竞争 map 写。
		time.Sleep(20 * time.Millisecond)
		return &mockCloser{}, nil, nil
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		clients = make([]*mockCloser, 0, N)
		errs    = make([]error, 0, N)
	)
	wg.Add(N)
	start := make(chan struct{})
	for range N {
		go func() {
			defer wg.Done()
			<-start
			c, _, err := cache.GetOrDial(42, dial)
			mu.Lock()
			clients = append(clients, c)
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if got := dialCalls.Load(); got != 1 {
		t.Fatalf("dial 被调用了 %d 次，期望 1（single-flight 失效，并发会话各自 dial 一份连接 → 连接泄漏）", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d 返回错误: %v", i, err)
		}
	}
	first := clients[0]
	for i, c := range clients {
		if c != first {
			t.Fatalf("goroutine %d 拿到不同连接 %p，期望全部复用 %p", i, c, first)
		}
	}
}

// TestConnCache_DialErrorReleasesInflight 保证 dial 失败也会清空 inflight，否则后续
// 同 assetID 调用会永远 wait。
func TestConnCache_DialErrorReleasesInflight(t *testing.T) {
	cache := NewConnCache[*mockCloser]("test")

	dialErr := errors.New("dial fail")
	failOnce := func() (*mockCloser, io.Closer, error) {
		return nil, nil, dialErr
	}
	if _, _, err := cache.GetOrDial(7, failOnce); err != dialErr {
		t.Fatalf("first dial err=%v, want %v", err, dialErr)
	}

	// 第二次必须能继续进入 dial（说明 inflight 已被清掉），不能死锁也不能永远复用旧失败。
	done := make(chan error, 1)
	want := &mockCloser{}
	go func() {
		_, _, err := cache.GetOrDial(7, func() (*mockCloser, io.Closer, error) {
			return want, nil, nil
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second dial err=%v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second GetOrDial 卡住 → inflight 没在 dial 失败时释放")
	}
}
