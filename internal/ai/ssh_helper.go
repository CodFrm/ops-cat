package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/service/credential_resolver"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// isExpectedCloseErr 判断 SSH/网络连接关闭时的预期错误。
// 取消路径会主动 Close session/client 打断阻塞，随后的 defer 关闭就会返回这些错误；
// 归类为预期错误后，上层可以跳过 warn 日志，避免噪音。
func isExpectedCloseErr(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed)
}

// closeOnCancel 启动 watcher goroutine，ctx 取消时调用所有 closers。
// 用于打断 SFTP io.Copy 等不感知 ctx 的阻塞操作 —— 关闭底层连接后，
// Copy 会立即因 net.ErrClosed 返回。
// 返回的 stop 函数必须 defer 调用，确保正常路径下 watcher 退出，不泄漏 goroutine。
// Close 错误忽略：connection 可能已被正常路径关闭，Close 是幂等的。
func closeOnCancel(ctx context.Context, closers ...io.Closer) func() {
	if ctx == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			for _, c := range closers {
				_ = c.Close()
			}
		case <-done:
		}
	}()
	return func() { close(done) }
}

// dialAssetSSH 建立到资产的 SSH 连接，返回 client 与一次性清理函数。
// 底层委托给 credential_resolver.DialAssetSSH，统一支持 SOCKS5/HTTP 代理与跳板机链。
// 调用方必须在使用结束后调用返回的 cleanup，一并关闭 client 与跳板机链上的中间连接。
func dialAssetSSH(ctx context.Context, assetID int64) (*ssh.Client, func(), error) {
	client, extraClosers, err := credential_resolver.Default().DialAssetSSH(ctx, assetID)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close SSH client", zap.Error(err))
		}
		closeExtras(extraClosers)
	}
	return client, cleanup, nil
}

// closeExtras 关闭跳板机链等附加资源，预期关闭错误静默跳过。
func closeExtras(closers []io.Closer) {
	for _, c := range closers {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close SSH chain resource", zap.Error(err))
		}
	}
}

// closersAsOne 将多个 closer 打包成单个 io.Closer（用于只接受单 closer 的 API，如 ConnCache）。
func closersAsOne(closers []io.Closer) io.Closer {
	if len(closers) == 0 {
		return nil
	}
	return closerFunc(func() error {
		closeExtras(closers)
		return nil
	})
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// executeSSHCommand 执行一次性 SSH 命令并返回输出（每次新建连接）
func executeSSHCommand(ctx context.Context, assetID int64, command string) (string, error) {
	client, cleanup, err := dialAssetSSH(ctx, assetID)
	if err != nil {
		return "", err
	}
	defer cleanup()
	return runSSHCommand(ctx, client, command)
}

// runSSHCommand 在已有的 SSH 客户端上执行命令
func runSSHCommand(ctx context.Context, client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		// ctx 取消路径下 session 已经被主动关闭，defer 再次 Close 会拿到已关闭错误，静默跳过。
		if err := session.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close SSH session", zap.Error(err))
		}
	}()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	runCh := make(chan error, 1)
	go func() {
		runCh <- session.Run(command)
	}()

	select {
	case err := <-runCh:
		if err != nil {
			if stderr.Len() > 0 {
				return "", fmt.Errorf("command failed: %s", stderr.String())
			}
			return "", fmt.Errorf("command failed: %w", err)
		}
	case <-ctx.Done():
		// 仅关闭 session 可能不足以唤醒底层 Run/Wait，这里连 client 一并关闭来打断阻塞。
		// 上层 defer 会再次 Close，已通过 isExpectedCloseErr 过滤预期错误。
		if err := session.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close SSH session on cancel", zap.Error(err))
		}
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close SSH client on cancel", zap.Error(err))
		}
		return "", ctx.Err()
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}
	return output, nil
}

// executeWithSFTP 创建临时 SSH+SFTP 连接并执行操作。
// ctx 取消时主动关闭底层连接以打断 fn 内部可能的 io.Copy 阻塞，
// 从而让 AI 停止会话能立即生效（否则大文件传输会挂住 runner.Stop）。
func executeWithSFTP(ctx context.Context, assetID int64, fn func(*sftp.Client) error) error {
	client, cleanup, err := dialAssetSSH(ctx, assetID)
	if err != nil {
		return err
	}
	defer cleanup()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer func() {
		if err := sftpClient.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close SFTP client", zap.Error(err))
		}
	}()

	// 顺序：先关 sftpClient 结束 SFTP 会话，再关 SSH client 打断底层 TCP。
	stopWatch := closeOnCancel(ctx, sftpClient, client)
	defer stopWatch()

	if err := fn(sftpClient); err != nil {
		// ctx 已取消时，优先返回 ctx.Err()，避免把底层 EOF/closed 暴露给上层。
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

// DialSSHClient 创建 SSH 客户端连接，自动解析凭据、代理、跳板机链。
// 调用者必须调用返回的 cleanup 关闭 client 与链路资源。
func DialSSHClient(ctx context.Context, assetID int64) (*ssh.Client, func(), error) {
	return dialAssetSSH(ctx, assetID)
}

// ExecWithStdio 在远程服务器执行命令，直接连接 stdio（支持管道）
func ExecWithStdio(ctx context.Context, assetID int64, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	client, cleanup, err := dialAssetSSH(ctx, assetID)
	if err != nil {
		return err
	}
	defer cleanup()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			logger.Ctx(ctx).Warn("close ExecWithStdio SSH session", zap.Error(err))
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
	srcClient, srcCleanup, err := dialAssetSSH(ctx, srcAssetID)
	if err != nil {
		return fmt.Errorf("source asset SSH connection failed: %w", err)
	}
	defer srcCleanup()

	dstClient, dstCleanup, err := dialAssetSSH(ctx, dstAssetID)
	if err != nil {
		return fmt.Errorf("destination asset SSH connection failed: %w", err)
	}
	defer dstCleanup()

	srcSFTP, err := sftp.NewClient(srcClient)
	if err != nil {
		return fmt.Errorf("source asset SFTP connection failed: %w", err)
	}
	defer func() {
		if err := srcSFTP.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close source SFTP client", zap.Error(err))
		}
	}()

	dstSFTP, err := sftp.NewClient(dstClient)
	if err != nil {
		return fmt.Errorf("destination asset SFTP connection failed: %w", err)
	}
	defer func() {
		if err := dstSFTP.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close destination SFTP client", zap.Error(err))
		}
	}()

	// ctx 取消时关闭两端 SFTP + SSH，打断 io.Copy 的 SFTP 读写阻塞。
	stopWatch := closeOnCancel(ctx, srcSFTP, dstSFTP, srcClient, dstClient)
	defer stopWatch()

	// 流式传输
	srcFile, err := srcSFTP.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close source file", zap.String("path", srcPath), zap.Error(err))
		}
	}()

	dstFile, err := dstSFTP.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		if err := dstFile.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Ctx(ctx).Warn("close destination file", zap.String("path", dstPath), zap.Error(err))
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("file transfer failed: %w", err)
	}

	return nil
}

// AIPoolDialer 实现 sshpool.PoolDialer，委托给 credential_resolver 统一 dial
type AIPoolDialer struct{}

func (d *AIPoolDialer) DialAsset(ctx context.Context, assetID int64) (*ssh.Client, []io.Closer, error) {
	return credential_resolver.Default().DialAssetSSH(ctx, assetID)
}
