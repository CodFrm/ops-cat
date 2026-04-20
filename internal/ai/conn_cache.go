package ai

import (
	"io"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// ConnCache 泛型连接缓存，在同一次 AI Chat 中复用连接
type ConnCache[C io.Closer] struct {
	clients map[int64]C
	closers map[int64]io.Closer
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
	for id, client := range c.clients {
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close cached "+c.name+" connection", zap.Int64("assetID", id), zap.Error(err))
		}
		delete(c.clients, id)
	}
	for id, closer := range c.closers {
		if closer != nil {
			if err := closer.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close "+c.name+" tunnel", zap.Int64("assetID", id), zap.Error(err))
			}
		}
		delete(c.closers, id)
	}
	return nil
}

// GetOrDial 从缓存获取连接，不存在则通过 dial 创建并缓存
// 返回的 closer 为 nil 表示连接来自缓存（调用方无需关闭）
func (c *ConnCache[C]) GetOrDial(assetID int64, dial func() (C, io.Closer, error)) (C, io.Closer, error) {
	if client, ok := c.clients[assetID]; ok {
		var zero io.Closer
		return client, zero, nil
	}
	client, closer, err := dial()
	if err != nil {
		var zero C
		return zero, nil, err
	}
	c.clients[assetID] = client
	c.closers[assetID] = closer
	return client, nil, nil
}

// Remove 关闭并移除指定 assetID 的缓存连接
func (c *ConnCache[C]) Remove(assetID int64) {
	if client, ok := c.clients[assetID]; ok {
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close "+c.name+" connection", zap.Int64("assetID", assetID), zap.Error(err))
		}
		delete(c.clients, assetID)
	}
	if closer, ok := c.closers[assetID]; ok {
		if closer != nil {
			if err := closer.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close "+c.name+" tunnel", zap.Int64("assetID", assetID), zap.Error(err))
			}
		}
		delete(c.closers, assetID)
	}
}

// Forget 仅把 assetID 对应的连接从缓存中摘除，但不主动调用 Close。
// 用于 client 已被上游提前关闭（例如 runSSHCommand 在 ctx 取消时已 Close）的场景，
// 避免 Remove 触发二次 Close 及伴随的预期关闭错误日志。
func (c *ConnCache[C]) Forget(assetID int64) {
	delete(c.clients, assetID)
	delete(c.closers, assetID)
}
