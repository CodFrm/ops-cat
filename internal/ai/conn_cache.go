package ai

import (
	"io"
	"strconv"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// ConnCache 泛型连接缓存，在同一次 AI Send 中复用连接。
// 并发模型：mu 保护 maps；sf 保证同一 assetID 并发首拨只发生一次（second waiter
// 拿到 first dialer 的结果），batch_command 多条命令并发命中同资产时只握手一次。
type ConnCache[C io.Closer] struct {
	mu      sync.Mutex
	clients map[int64]C
	closers map[int64]io.Closer
	sf      singleflight.Group
	name    string // 用于日志标识，如 "database"、"Redis"
}

// NewConnCache 创建连接缓存
func NewConnCache[C io.Closer](name string) *ConnCache[C] {
	return &ConnCache[C]{
		clients: make(map[int64]C),
		closers: make(map[int64]io.Closer),
		name:    name,
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
		if closer != nil {
			if err := closer.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close "+c.name+" tunnel", zap.Int64("assetID", id), zap.Error(err))
			}
		}
	}
	return nil
}

// GetOrDial 从缓存获取连接，不存在则通过 dial 创建并缓存。
// 返回的 closer 始终为 nil —— 缓存命中或新拨号都由 cache 持有 closer，
// 调用方不需要也不应该 Close 这个连接（应通过 Remove/Forget/Close 释放）。
//
// 并发安全：同一 assetID 的并发首拨经 singleflight 合流为一次 dial，second
// waiter 直接拿到 first dialer 的 client；不同 assetID 完全并行。
func (c *ConnCache[C]) GetOrDial(assetID int64, dial func() (C, io.Closer, error)) (C, io.Closer, error) {
	c.mu.Lock()
	if client, ok := c.clients[assetID]; ok {
		c.mu.Unlock()
		return client, nil, nil
	}
	c.mu.Unlock()

	key := strconv.FormatInt(assetID, 10)
	v, err, _ := c.sf.Do(key, func() (any, error) {
		// 进入 dial 前再查一次，避免 first dialer 释放 singleflight 后、
		// 第二批 waiter 才到达时重复拨号。
		c.mu.Lock()
		if client, ok := c.clients[assetID]; ok {
			c.mu.Unlock()
			return client, nil
		}
		c.mu.Unlock()

		client, closer, derr := dial()
		if derr != nil {
			return *new(C), derr
		}
		c.mu.Lock()
		c.clients[assetID] = client
		c.closers[assetID] = closer
		c.mu.Unlock()
		return client, nil
	})
	if err != nil {
		var zero C
		return zero, nil, err
	}
	return v.(C), nil, nil
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
