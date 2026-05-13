// Package testreg 维护资产「测试连接」操作的可取消上下文注册表。
//
// 前端为每次测试生成一个 testID，调用 App 层 Test*Connection(testID, ...)
// 时通过 Begin 注册一个派生 ctx；用户点击「取消测试」时，前端调用
// CancelTest(testID)，App 层调用 Cancel(testID) 触发取消。
package testreg

import (
	"context"
	"sync"
)

var (
	mu    sync.Mutex
	items = make(map[string]context.CancelFunc)
)

// Begin 注册 id → cancel，返回派生 ctx 和 release 函数。
// 调用方必须 defer release()：它会从注册表删除 id 并调用 cancel，
// 既保证不泄漏 goroutine，又使得 release 后再来的 Cancel(id) 变为 no-op。
// 空 id 视为不注册（仅返回派生 ctx 与一个只 cancel 的 release）。
func Begin(parent context.Context, id string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	if id == "" {
		return ctx, cancel
	}
	mu.Lock()
	items[id] = cancel
	mu.Unlock()
	return ctx, func() {
		mu.Lock()
		delete(items, id)
		mu.Unlock()
		cancel()
	}
}

// Cancel 触发指定 testID 的取消。未知 id 静默忽略。
func Cancel(id string) {
	if id == "" {
		return
	}
	mu.Lock()
	fn := items[id]
	delete(items, id)
	mu.Unlock()
	if fn != nil {
		fn()
	}
}
