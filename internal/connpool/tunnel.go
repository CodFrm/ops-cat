package connpool

import (
	"context"
	"fmt"
	"net"

	"ops-cat/internal/sshpool"
)

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

// Dial 通过 SSH 转发获得到目标地址的 net.Conn
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
	return conn, nil
}

// Close 释放 SSH 连接引用
func (t *SSHTunnel) Close() error {
	if t.pool != nil {
		t.pool.Release(t.sshAssetID)
	}
	return nil
}
