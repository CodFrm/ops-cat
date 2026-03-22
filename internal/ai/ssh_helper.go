package ai

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/internal/service/ssh_key_svc"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// resolveAssetCredentials 从资产配置中解析凭据（自动解密密码/获取密钥）
func resolveAssetCredentials(ctx context.Context, cfg *asset_entity.SSHConfig) (password, key string, err error) {
	if cfg.AuthType == "password" && cfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
		if err != nil {
			return "", "", fmt.Errorf("解密密码失败: %w", err)
		}
		return decrypted, "", nil
	}
	if cfg.AuthType == "key" && cfg.KeySource == "managed" && cfg.KeyID > 0 {
		privKey, err := ssh_key_svc.GetPrivateKey(ctx, cfg.KeyID)
		if err != nil {
			return "", "", fmt.Errorf("获取密钥失败: %w", err)
		}
		return "", privKey, nil
	}
	return "", "", nil
}

// createSSHClient 创建 SSH 客户端，支持 password 和 key 认证
func createSSHClient(cfg *asset_entity.SSHConfig, password, key string) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod
	switch cfg.AuthType {
	case "password":
		if password != "" {
			authMethods = []ssh.AuthMethod{ssh.Password(password)}
		}
	case "key":
		if key != "" {
			signer, err := ssh.ParsePrivateKey([]byte(key))
			if err != nil {
				return nil, fmt.Errorf("解析私钥失败: %w", err)
			}
			authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		}
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("没有可用的认证方式")
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
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	return client, nil
}

// executeSSHCommand 执行一次性 SSH 命令并返回输出（每次新建连接）
func executeSSHCommand(cfg *asset_entity.SSHConfig, password, key string, command string) (string, error) {
	client, err := createSSHClient(cfg, password, key)
	if err != nil {
		return "", err
	}
	defer client.Close()

	return runSSHCommand(client, command)
}

// runSSHCommand 在已有的 SSH 客户端上执行命令
func runSSHCommand(client *ssh.Client, command string) (string, error) {
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

// executeWithSFTP 创建临时 SSH+SFTP 连接并执行操作
func executeWithSFTP(cfg *asset_entity.SSHConfig, password, key string, fn func(*sftp.Client) error) error {
	client, err := createSSHClient(cfg, password, key)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("创建SFTP客户端失败: %w", err)
	}
	defer sftpClient.Close()

	return fn(sftpClient)
}
