package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

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

// resolveAssetSSH 根据资产 ID 解析 SSH 连接所需信息（内部使用 credential_resolver）
func resolveAssetSSH(ctx context.Context, assetID int64) (*asset_entity.Asset, *asset_entity.SSHConfig, string, string, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return nil, nil, "", "", fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("获取SSH配置失败: %w", err)
	}
	password, key, err := credential_resolver.Default().ResolveSSHCredentials(ctx, sshCfg)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("解析凭据失败: %w", err)
	}
	return asset, sshCfg, password, key, nil
}

// executeSSHCommand 执行一次性 SSH 命令并返回输出（每次新建连接）
func executeSSHCommand(cfg *asset_entity.SSHConfig, password, key string, command string) (string, error) {
	client, err := createSSHClient(cfg, password, key)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close SSH client", zap.Error(err))
		}
	}()

	return runSSHCommand(client, command)
}

// runSSHCommand 在已有的 SSH 客户端上执行命令
func runSSHCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			logger.Default().Warn("close SSH session", zap.Error(err))
		}
	}()

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
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close SFTP SSH client", zap.Error(err))
		}
	}()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("创建SFTP客户端失败: %w", err)
	}
	defer func() {
		if err := sftpClient.Close(); err != nil {
			logger.Default().Warn("close SFTP client", zap.Error(err))
		}
	}()

	return fn(sftpClient)
}

// DialSSHClient 创建 SSH 客户端连接，自动解析凭据。调用者需要关闭 client。
func DialSSHClient(ctx context.Context, assetID int64) (*ssh.Client, error) {
	_, sshCfg, password, key, err := resolveAssetSSH(ctx, assetID)
	if err != nil {
		return nil, err
	}
	return createSSHClient(sshCfg, password, key)
}

// ExecWithStdio 在远程服务器执行命令，直接连接 stdio（支持管道）
func ExecWithStdio(ctx context.Context, assetID int64, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	_, sshCfg, password, key, err := resolveAssetSSH(ctx, assetID)
	if err != nil {
		return err
	}

	client, err := createSSHClient(sshCfg, password, key)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close ExecWithStdio SSH client", zap.Error(err))
		}
	}()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("创建会话失败: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			logger.Default().Warn("close ExecWithStdio SSH session", zap.Error(err))
		}
	}()

	if stdin != nil {
		session.Stdin = stdin
	}
	session.Stdout = stdout
	session.Stderr = stderr

	return session.Run(command)
}

// CopyBetweenAssets 在两个资产间直接传输文件（SFTP 流式，不经本地磁盘）
func CopyBetweenAssets(ctx context.Context, srcAssetID int64, srcPath string, dstAssetID int64, dstPath string) error {
	// 解析源资产凭证
	_, srcCfg, srcPassword, srcKey, err := resolveAssetSSH(ctx, srcAssetID)
	if err != nil {
		return fmt.Errorf("源资产解析失败: %w", err)
	}

	// 解析目标资产凭证
	_, dstCfg, dstPassword, dstKey, err := resolveAssetSSH(ctx, dstAssetID)
	if err != nil {
		return fmt.Errorf("目标资产解析失败: %w", err)
	}

	// 创建 SSH 客户端
	srcClient, err := createSSHClient(srcCfg, srcPassword, srcKey)
	if err != nil {
		return fmt.Errorf("源资产SSH连接失败: %w", err)
	}
	defer func() {
		if err := srcClient.Close(); err != nil {
			logger.Default().Warn("close source SSH client", zap.Error(err))
		}
	}()

	dstClient, err := createSSHClient(dstCfg, dstPassword, dstKey)
	if err != nil {
		return fmt.Errorf("目标资产SSH连接失败: %w", err)
	}
	defer func() {
		if err := dstClient.Close(); err != nil {
			logger.Default().Warn("close destination SSH client", zap.Error(err))
		}
	}()

	// 创建 SFTP 客户端
	srcSFTP, err := sftp.NewClient(srcClient)
	if err != nil {
		return fmt.Errorf("源资产SFTP连接失败: %w", err)
	}
	defer func() {
		if err := srcSFTP.Close(); err != nil {
			logger.Default().Warn("close source SFTP client", zap.Error(err))
		}
	}()

	dstSFTP, err := sftp.NewClient(dstClient)
	if err != nil {
		return fmt.Errorf("目标资产SFTP连接失败: %w", err)
	}
	defer func() {
		if err := dstSFTP.Close(); err != nil {
			logger.Default().Warn("close destination SFTP client", zap.Error(err))
		}
	}()

	// 流式传输
	srcFile, err := srcSFTP.Open(srcPath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			logger.Default().Warn("close source file", zap.String("path", srcPath), zap.Error(err))
		}
	}()

	dstFile, err := dstSFTP.Create(dstPath)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			logger.Default().Warn("close destination file", zap.String("path", dstPath), zap.Error(err))
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("文件传输失败: %w", err)
	}

	return nil
}

// AIPoolDialer 实现 sshpool.PoolDialer，使用 credential_resolver 解析凭据
type AIPoolDialer struct{}

func (d *AIPoolDialer) DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error) {
	sshCfg, password, key, _, err := credential_resolver.Default().ResolveSSHConnectConfig(ctx, assetID)
	if err != nil {
		return nil, nil, err
	}
	client, err := createSSHClient(sshCfg, password, key)
	if err != nil {
		return nil, nil, err
	}
	return client, nil, nil
}
