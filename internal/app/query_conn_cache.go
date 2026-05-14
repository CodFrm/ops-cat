package app

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// panelConnCache 持久化 query 面板的连接,按 key("<assetID>:<database>") 缓存。
//
// 与 internal/ai/ConnCache 的差别:
//   - 生命周期:那个是每次 AI Send 用完即弃;这个是 App 生命周期 + 空闲 TTL。
//   - 后台 evictor 周期扫描,超过 idleTTL 未使用的项目自动 Close + 移除。
//   - key 用 string,可以同时区分 assetID 和 database。
//
// 并发模型:mu 保护 map;sf 保证同 key 并发首拨只发生一次。
type panelConnCache[C io.Closer] struct {
	mu      sync.Mutex
	entries map[string]*panelConnEntry[C]
	sf      singleflight.Group
	idleTTL time.Duration
	name    string // 仅用于日志区分,如 "database"、"redis"、"mongodb"
}

type panelConnEntry[C io.Closer] struct {
	client   C
	tunnel   io.Closer
	lastUsed atomic.Int64 // unix nano
}

func newPanelConnCache[C io.Closer](name string, idleTTL time.Duration) *panelConnCache[C] {
	return &panelConnCache[C]{
		entries: make(map[string]*panelConnEntry[C]),
		idleTTL: idleTTL,
		name:    name,
	}
}

// GetOrDial 从缓存取连接,不存在则调 dial 并入缓存。
// 返回的 client 由 cache 持有,调用方不应主动 Close —— 由 Drop/Close/evictor 统一释放。
// 第二个返回值始终为 nil(为兼容现有 connpool.DialDatabase 三返值签名)。
func (c *panelConnCache[C]) GetOrDial(key string, dial func() (C, io.Closer, error)) (C, io.Closer, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		e.lastUsed.Store(time.Now().UnixNano())
		client := e.client
		c.mu.Unlock()
		return client, nil, nil
	}
	c.mu.Unlock()

	v, err, _ := c.sf.Do(key, func() (any, error) {
		// 进入 dial 前再查一次,避免 first dialer 释放 singleflight 后、
		// 第二批 waiter 才到达时重复拨号。
		c.mu.Lock()
		if e, ok := c.entries[key]; ok {
			e.lastUsed.Store(time.Now().UnixNano())
			client := e.client
			c.mu.Unlock()
			return client, nil
		}
		c.mu.Unlock()

		client, closer, derr := dial()
		if derr != nil {
			return *new(C), derr
		}
		e := &panelConnEntry[C]{client: client, tunnel: closer}
		e.lastUsed.Store(time.Now().UnixNano())
		c.mu.Lock()
		c.entries[key] = e
		c.mu.Unlock()
		return client, nil
	})
	if err != nil {
		var zero C
		return zero, nil, err
	}
	return v.(C), nil, nil
}

// Touch 显式刷新 lastUsed。GetOrDial 已经隐式 Touch 命中分支,但成功执行查询后
// 调用方再 Touch 一次,可以避免"长查询期间被 evictor 误判空闲"。
func (c *panelConnCache[C]) Touch(key string) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		e.lastUsed.Store(time.Now().UnixNano())
	}
	c.mu.Unlock()
}

// Drop 关闭并移除指定 key 的连接。
func (c *panelConnCache[C]) Drop(key string) {
	c.mu.Lock()
	e, ok := c.entries[key]
	delete(c.entries, key)
	c.mu.Unlock()
	if !ok {
		return
	}
	c.closeEntry(key, e)
}

// Close 关闭并移除所有连接。可重复调用,二次调用为空操作。
func (c *panelConnCache[C]) Close() error {
	c.mu.Lock()
	entries := c.entries
	c.entries = make(map[string]*panelConnEntry[C])
	c.mu.Unlock()
	for key, e := range entries {
		c.closeEntry(key, e)
	}
	return nil
}

// startEvictor 启动周期驱逐 goroutine。每 evictInterval 扫一次,
// 把超过 idleTTL 未使用的项关闭并移除。ctx 取消时退出。
func (c *panelConnCache[C]) startEvictor(ctx context.Context, evictInterval time.Duration) {
	ticker := time.NewTicker(evictInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.evictOnce(now)
		}
	}
}

// evictOnce 扫一次缓存,把 lastUsed 早于 (now - idleTTL) 的项目驱逐。
// 拆出为方法是为了让测试能在不依赖真实时钟的情况下触发驱逐。
func (c *panelConnCache[C]) evictOnce(now time.Time) {
	threshold := now.Add(-c.idleTTL).UnixNano()
	c.mu.Lock()
	var stale map[string]*panelConnEntry[C]
	for key, e := range c.entries {
		if e.lastUsed.Load() < threshold {
			if stale == nil {
				stale = make(map[string]*panelConnEntry[C])
			}
			stale[key] = e
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
	for key, e := range stale {
		c.closeEntry(key, e)
	}
}

// size 仅用于测试断言。
func (c *panelConnCache[C]) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *panelConnCache[C]) closeEntry(key string, e *panelConnEntry[C]) {
	if err := e.client.Close(); err != nil && !isExpectedPanelCloseErr(err) {
		logger.Default().Warn("close cached "+c.name+" connection", zap.String("key", key), zap.Error(err))
	}
	if e.tunnel != nil {
		if err := e.tunnel.Close(); err != nil && !isExpectedPanelCloseErr(err) {
			logger.Default().Warn("close "+c.name+" tunnel", zap.String("key", key), zap.Error(err))
		}
	}
}

func isExpectedPanelCloseErr(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}
