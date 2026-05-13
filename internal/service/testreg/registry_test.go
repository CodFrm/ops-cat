package testreg

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCancelTriggersDerivedContext(t *testing.T) {
	ctx, release := Begin(context.Background(), "abc")
	defer release()

	Cancel("abc")

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Cancel did not cancel derived context")
	}
}

func TestReleaseRemovesFromRegistry(t *testing.T) {
	_, release := Begin(context.Background(), "xyz")
	release()
	// Second Cancel must be a no-op (item removed).
	Cancel("xyz")
	mu.Lock()
	_, exists := items["xyz"]
	mu.Unlock()
	if exists {
		t.Fatal("release should have removed id from registry")
	}
}

func TestCancelUnknownIDIsNoop(t *testing.T) {
	Cancel("does-not-exist")
	Cancel("")
}

func TestEmptyIDDoesNotRegister(t *testing.T) {
	_, release := Begin(context.Background(), "")
	defer release()
	mu.Lock()
	n := len(items)
	mu.Unlock()
	// Should be unaffected; we only assert that empty id wasn't added.
	if _, ok := items[""]; ok {
		t.Fatalf("empty id should not be registered, registry size=%d", n)
	}
}

func TestParentCancelPropagates(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, release := Begin(parent, "parent-test")
	defer release()
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancel should propagate to derived ctx")
	}
}

func TestConcurrentBeginCancel(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		id := "id-" + itoa(i)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, release := Begin(context.Background(), id)
			release()
		}()
		go func() {
			defer wg.Done()
			Cancel(id)
		}()
	}
	wg.Wait()
	mu.Lock()
	leftover := len(items)
	mu.Unlock()
	if leftover != 0 {
		t.Fatalf("expected registry empty after concurrent run, got %d entries", leftover)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
