package connpool

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/opskat/opskat/internal/sshpool"
)

// tunnelConn 包装 SSH channel，在连接关闭时自动释放 SSH 池引用。
// 同时忽略 SetDeadline 调用（SSH channel 不支持 deadline，但数据库驱动会调用）。
type tunnelConn struct {
	net.Conn
	pool    *sshpool.Pool
	assetID int64
	once    sync.Once
}

func (c *tunnelConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.pool.Release(c.assetID) })
	return err
}

func (c *tunnelConn) SetDeadline(_ time.Time) error      { return nil }
func (c *tunnelConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *tunnelConn) SetWriteDeadline(_ time.Time) error { return nil }

// SSHTunnel 管理通过 SSH 资产建立的 TCP 隧道
type SSHTunnel struct {
	sshAssetID int64
	targetAddr string
	pool       *sshpool.Pool
}

// NewSSHTunnel 创建 SSH 隧道
func NewSSHTunnel(sshAssetID int64, host string, port int, pool *sshpool.Pool) *SSHTunnel {
	return &SSHTunnel{
		sshAssetID: sshAssetID,
		targetAddr: fmt.Sprintf("%s:%d", host, port),
		pool:       pool,
	}
}

// Dial 通过 SSH 转发获得到目标地址的 net.Conn。
// 每条连接独立持有 SSH 池引用，连接关闭时自动释放。
func (t *SSHTunnel) Dial(ctx context.Context) (net.Conn, error) {
	sshClient, err := t.pool.Get(ctx, t.sshAssetID)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	conn, err := sshClient.Dial("tcp", t.targetAddr)
	if err != nil {
		t.pool.Release(t.sshAssetID)
		return nil, fmt.Errorf("SSH 隧道建立失败: %w", err)
	}
	return &tunnelConn{Conn: conn, pool: t.pool, assetID: t.sshAssetID}, nil
}

// Close 是一个空操作。每条通过 Dial 创建的连接会在自身关闭时释放 SSH 池引用，
// 无需由 SSHTunnel 统一释放。保留此方法以满足 io.Closer 接口。
func (t *SSHTunnel) Close() error {
	return nil
}
