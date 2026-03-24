package main

import (
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/sshpool"
)

// getSSHProxyClient 检测 sshpool.sock 是否可用，返回 client 或 nil（fallback 直连）
func getSSHProxyClient() *sshpool.Client {
	sockPath := sshpool.SocketPath(bootstrap.AppDataDir())
	client := sshpool.NewClient(sockPath)
	if client.IsAvailable() {
		return client
	}
	return nil
}
