package sshpool

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// PoolDialer 创建 SSH 连接的接口，由调用方实现以解耦凭据解析和跳板机逻辑
type PoolDialer interface {
	DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error)
}

// poolEntry 连接池条目
type poolEntry struct {
	client   *ssh.Client
	closers  []io.Closer // 跳板机等中间连接
	assetID  int64
	lastUsed time.Time
	refCount int
	mu       sync.Mutex
	closed   bool
}

// acquire 增加引用计数
func (e *poolEntry) acquire() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refCount++
	e.lastUsed = time.Now()
}

// release 减少引用计数，返回是否可以被清理（refCount == 0）
func (e *poolEntry) release() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refCount--
	e.lastUsed = time.Now()
	return e.refCount <= 0
}

// isIdle 检查是否空闲超时
func (e *poolEntry) isIdle(timeout time.Duration) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.refCount <= 0 && time.Since(e.lastUsed) > timeout
}

// isAlive 检测连接是否存活
func (e *poolEntry) isAlive() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false
	}
	_, _, err := e.client.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

// close 关闭连接及所有中间连接
func (e *poolEntry) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	if err := e.client.Close(); err != nil {
		logger.Default().Warn("close ssh client", zap.Int64("assetID", e.assetID), zap.Error(err))
	}
	for _, c := range e.closers {
		if err := c.Close(); err != nil {
			logger.Default().Warn("close intermediate connection", zap.Int64("assetID", e.assetID), zap.Error(err))
		}
	}
}

// PoolEntryInfo 连接池条目信息（用于 UI 展示）
type PoolEntryInfo struct {
	AssetID  int64 `json:"asset_id"`
	RefCount int   `json:"ref_count"`
	LastUsed int64 `json:"last_used"` // Unix timestamp
}

// Pool SSH 连接池
type Pool struct {
	mu        sync.RWMutex
	entries   map[int64]*poolEntry
	dialer    PoolDialer
	idleTime  time.Duration
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewPool 创建连接池
func NewPool(dialer PoolDialer, idleTimeout time.Duration) *Pool {
	p := &Pool{
		entries:  make(map[int64]*poolEntry),
		dialer:   dialer,
		idleTime: idleTimeout,
		done:     make(chan struct{}),
	}
	p.wg.Add(1)
	go p.cleanupLoop()
	return p
}

// Get 获取或创建一个 SSH 连接，调用方用完后必须调用 Release
func (p *Pool) Get(ctx context.Context, assetID int64) (*ssh.Client, error) {
	// 尝试从缓存获取
	p.mu.RLock()
	entry, ok := p.entries[assetID]
	p.mu.RUnlock()

	if ok {
		if entry.isAlive() {
			entry.acquire()
			return entry.client, nil
		}
		// 连接已死，移除
		p.Remove(assetID)
	}

	// 创建新连接
	client, closers, err := p.dialer.DialAsset(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("dial asset %d: %w", assetID, err)
	}

	entry = &poolEntry{
		client:   client,
		closers:  closers,
		assetID:  assetID,
		lastUsed: time.Now(),
		refCount: 1,
	}

	p.mu.Lock()
	// 可能在拨号期间有其他 goroutine 已创建了连接
	if existing, ok := p.entries[assetID]; ok {
		p.mu.Unlock()
		// 关闭我们刚创建的，使用已存在的
		if err := client.Close(); err != nil {
			logger.Ctx(ctx).Warn("close duplicate ssh client", zap.Int64("assetID", assetID), zap.Error(err))
		}
		for _, c := range closers {
			if err := c.Close(); err != nil {
				logger.Ctx(ctx).Warn("close duplicate intermediate connection", zap.Int64("assetID", assetID), zap.Error(err))
			}
		}
		if existing.isAlive() {
			existing.acquire()
			return existing.client, nil
		}
		// 已存在的也死了，递归重试
		p.Remove(assetID)
		return p.Get(ctx, assetID)
	}
	p.entries[assetID] = entry
	p.mu.Unlock()

	return client, nil
}

// Release 释放连接引用
func (p *Pool) Release(assetID int64) {
	p.mu.RLock()
	entry, ok := p.entries[assetID]
	p.mu.RUnlock()
	if ok {
		entry.release()
	}
}

// Remove 强制移除并关闭连接
func (p *Pool) Remove(assetID int64) {
	p.mu.Lock()
	entry, ok := p.entries[assetID]
	if ok {
		delete(p.entries, assetID)
	}
	p.mu.Unlock()
	if ok {
		entry.close()
	}
}

// List 返回所有连接池条目信息
func (p *Pool) List() []PoolEntryInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	infos := make([]PoolEntryInfo, 0, len(p.entries))
	for _, entry := range p.entries {
		entry.mu.Lock()
		infos = append(infos, PoolEntryInfo{
			AssetID:  entry.assetID,
			RefCount: entry.refCount,
			LastUsed: entry.lastUsed.Unix(),
		})
		entry.mu.Unlock()
	}
	return infos
}

// Close 关闭连接池（可安全多次调用）
func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		close(p.done)
		p.wg.Wait()

		p.mu.Lock()
		for id, entry := range p.entries {
			entry.close()
			delete(p.entries, id)
		}
		p.mu.Unlock()
	})
}

// cleanupLoop 后台清理空闲连接
func (p *Pool) cleanupLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.cleanupIdle()
		}
	}
}

func (p *Pool) cleanupIdle() {
	p.mu.Lock()
	var toRemove []int64
	for id, entry := range p.entries {
		if entry.isIdle(p.idleTime) {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		if entry, ok := p.entries[id]; ok {
			delete(p.entries, id)
			entry.close()
			logger.Default().Info("closed idle connection", zap.Int64("assetID", id))
		}
	}
	p.mu.Unlock()
}
