package ai

import (
	"bytes"
	"fmt"
	"time"

	"ops-cat/internal/model/entity/asset_entity"

	"golang.org/x/crypto/ssh"
)

// executeSSHCommand 执行一次性 SSH 命令并返回输出
func executeSSHCommand(cfg *asset_entity.SSHConfig, password string, command string) (string, error) {
	var authMethods []ssh.AuthMethod
	switch cfg.AuthType {
	case "password":
		authMethods = []ssh.AuthMethod{ssh.Password(password)}
	default:
		return "", fmt.Errorf("AI run_command 暂仅支持密码认证")
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return "", fmt.Errorf("SSH连接失败: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("命令执行失败: %s", stderr.String())
		}
		return "", fmt.Errorf("命令执行失败: %w", err)
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}
	return output, nil
}
