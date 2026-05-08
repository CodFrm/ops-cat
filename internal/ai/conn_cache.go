package ai

import (
	"io"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// ConnCache 泛型连接缓存，在同一次 AI Chat 中复用连接。
//
// 并发安全：cago 默认 toolConcurrency=8，模型一轮可以并行触发多个 run_command/exec_*；
// 同一 assetID 的并发 GetOrDial 要么 race 在 map 上（runtime panic：concurrent map writes），
// 要么各自 dial 一份连接 + 互相覆盖 map 项 → 多余连接泄漏。
//
// 这里用一把 mu 保护两张 map + 一个 inflight 通道做 per-asset single-flight：
// 第一路 miss 时占位、跑 dial、写回；之后到的同 assetID 调用 wait inflight，dial 完成后
// 直接复用缓存。dial 阶段持有的是 inflight chan 而不是 mu，所以一个 asset 在 dial 中
// 不会卡住其它 asset 的查询。
type ConnCache[C io.Closer] struct {
	mu       sync.Mutex
	clients  map[int64]C
	closers  map[int64]io.Closer
	inflight map[int64]chan struct{}
	name     string // 用于日志标识，如 "database"、"Redis"
}

// NewConnCache 创建连接缓存
func NewConnCache[C io.Closer](name string) *ConnCache[C] {
	return &ConnCache[C]{
		clients:  make(map[int64]C),
		closers:  make(map[int64]io.Closer),
		inflight: make(map[int64]chan struct{}),
		name:     name,
	}
}

// Close 关闭所有缓存的连接
func (c *ConnCache[C]) Close() error {
	c.mu.Lock()
	clients := c.clients
	closers := c.closers
	c.clients = make(map[int64]C)
	c.closers = make(map[int64]io.Closer)
	c.mu.Unlock()

	for id, client := range clients {
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close cached "+c.name+" connection", zap.Int64("assetID", id), zap.Error(err))
		}
	}
	for id, closer := range closers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close "+c.name+" tunnel", zap.Int64("assetID", id), zap.Error(err))
		}
	}
	return nil
}

// GetOrDial 从缓存获取连接，不存在则通过 dial 创建并缓存。
// 返回的 closer 为 nil 表示连接来自缓存（调用方无需关闭）。
//
// 同一 assetID 的并发调用会被 single-flight 化：第一路 dial，其余 wait inflight，
// dial 完成后直接拿缓存，不会重复发 SSH/Redis/DB 握手。
func (c *ConnCache[C]) GetOrDial(assetID int64, dial func() (C, io.Closer, error)) (C, io.Closer, error) {
	for {
		c.mu.Lock()
		if client, ok := c.clients[assetID]; ok {
			c.mu.Unlock()
			var zero io.Closer
			return client, zero, nil
		}
		if wait, ok := c.inflight[assetID]; ok {
			// 别的 goroutine 正在 dial，松锁等它完成后回到 for 顶重新查缓存。
			c.mu.Unlock()
			<-wait
			continue
		}
		// 占位：宣告自己在 dial。
		wait := make(chan struct{})
		c.inflight[assetID] = wait
		c.mu.Unlock()

		client, closer, err := dial()

		c.mu.Lock()
		delete(c.inflight, assetID)
		if err == nil {
			c.clients[assetID] = client
			c.closers[assetID] = closer
		}
		c.mu.Unlock()
		close(wait) // 唤醒所有 waiter

		if err != nil {
			var zero C
			return zero, nil, err
		}
		return client, nil, nil
	}
}

// Remove 关闭并移除指定 assetID 的缓存连接
func (c *ConnCache[C]) Remove(assetID int64) {
	c.mu.Lock()
	client, hasClient := c.clients[assetID]
	closer, hasCloser := c.closers[assetID]
	delete(c.clients, assetID)
	delete(c.closers, assetID)
	c.mu.Unlock()

	if hasClient {
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close "+c.name+" connection", zap.Int64("assetID", assetID), zap.Error(err))
		}
	}
	if hasCloser && closer != nil {
		if err := closer.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close "+c.name+" tunnel", zap.Int64("assetID", assetID), zap.Error(err))
		}
	}
}

// Forget 仅把 assetID 对应的连接从缓存中摘除，但不主动调用 Close。
// 用于 client 已被上游提前关闭（例如 runSSHCommand 在 ctx 取消时已 Close）的场景，
// 避免 Remove 触发二次 Close 及伴随的预期关闭错误日志。
func (c *ConnCache[C]) Forget(assetID int64) {
	c.mu.Lock()
	delete(c.clients, assetID)
	delete(c.closers, assetID)
	c.mu.Unlock()
}
