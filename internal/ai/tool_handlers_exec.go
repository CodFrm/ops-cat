package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/service/credential_resolver"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func handleRequestGrant(ctx context.Context, args map[string]any) (string, error) {
	itemsJSON := argString(args, "items")
	reason := argString(args, "reason")
	if itemsJSON == "" {
		return "", fmt.Errorf("missing required parameter: items")
	}

	var rawItems []struct {
		AssetID         int64  `json:"asset_id"`
		CommandPatterns string `json:"command_patterns"`
	}
	if err := json.Unmarshal([]byte(itemsJSON), &rawItems); err != nil {
		return "", fmt.Errorf("invalid items JSON: %w", err)
	}
	if len(rawItems) == 0 {
		return "", fmt.Errorf("items must not be empty")
	}

	var grantItems []GrantItem
	for _, raw := range rawItems {
		if raw.AssetID == 0 {
			return "", fmt.Errorf("each item must have a non-zero asset_id")
		}
		var patterns []string
		for _, line := range strings.Split(raw.CommandPatterns, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				patterns = append(patterns, line)
			}
		}
		if len(patterns) == 0 {
			continue
		}
		grantItems = append(grantItems, GrantItem{AssetID: raw.AssetID, Patterns: patterns})
	}
	if len(grantItems) == 0 {
		return "", fmt.Errorf("no valid command patterns provided")
	}

	checker := GetPolicyChecker(ctx)
	if checker == nil {
		return "", fmt.Errorf("permission checker not available")
	}

	result := checker.SubmitGrantMulti(ctx, grantItems, reason)
	RecordDecision(ctx, result)
	return result.Message, nil
}

func handleRunCommand(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	command := argString(args, "command")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if command == "" {
		return "", fmt.Errorf("missing required parameter: command")
	}

	// 权限检查（两条路径共用）
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.Check(ctx, assetID, command)
		RecordDecision(ctx, result)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	// 如果 context 注入了 SSH 缓存，复用同一资产的连接
	if cache := getSSHCache(ctx); cache != nil {
		return runCommandWithCache(ctx, cache, assetID, command)
	}

	// 无缓存，创建一次性连接
	return executeSSHCommand(ctx, assetID, command)
}

func runCommandWithCache(ctx context.Context, cache *SSHClientCache, assetID int64, command string) (string, error) {
	dial := func() (*ssh.Client, io.Closer, error) {
		client, extras, err := credential_resolver.Default().DialAssetSSH(ctx, assetID)
		if err != nil {
			return nil, nil, err
		}
		return client, closersAsOne(extras), nil
	}

	client, _, err := cache.GetOrDial(assetID, dial)
	if err != nil {
		return "", err
	}
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		// 当前会话已经取消时，runSSHCommand 已主动关闭 client 以打断阻塞；
		// 这里只需把条目从缓存中摘除（避免下次复用半失效连接），不能再次 Close。
		if ctx.Err() != nil {
			cache.Forget(assetID)
			return "", ctx.Err()
		}
		// 非取消错误优先按连接失效处理，删除缓存后只重试一次，避免重复执行
		cache.Remove(assetID)
		client, _, err = cache.GetOrDial(assetID, dial)
		if err != nil {
			return "", err
		}
		output, err = runSSHCommand(ctx, client, command)
		if err != nil {
			cache.Remove(assetID)
			return "", err
		}
	}
	return output, nil
}

func handleUploadFile(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	localPath := argString(args, "local_path")
	remotePath := argString(args, "remote_path")
	if assetID == 0 || localPath == "" || remotePath == "" {
		return "", fmt.Errorf("missing required parameters: asset_id, local_path, remote_path")
	}

	err := executeWithSFTP(ctx, assetID, func(client *sftp.Client) error {
		srcFile, err := os.Open(localPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("failed to open local file: %w", err)
		}
		defer func() {
			if err := srcFile.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close local file", zap.String("path", localPath), zap.Error(err))
			}
		}()

		dstFile, err := client.Create(remotePath)
		if err != nil {
			return fmt.Errorf("failed to create remote file: %w", err)
		}
		defer func() {
			if err := dstFile.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close remote file", zap.String("path", remotePath), zap.Error(err))
			}
		}()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"message":"file uploaded successfully","remote_path":"%s"}`, remotePath), nil
}

func handleDownloadFile(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	remotePath := argString(args, "remote_path")
	localPath := argString(args, "local_path")
	if assetID == 0 || remotePath == "" || localPath == "" {
		return "", fmt.Errorf("missing required parameters: asset_id, remote_path, local_path")
	}

	err := executeWithSFTP(ctx, assetID, func(client *sftp.Client) error {
		srcFile, err := client.Open(remotePath)
		if err != nil {
			return fmt.Errorf("failed to open remote file: %w", err)
		}
		defer func() {
			if err := srcFile.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close remote file", zap.String("path", remotePath), zap.Error(err))
			}
		}()

		dstFile, err := os.Create(localPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("failed to create local file: %w", err)
		}
		defer func() {
			if err := dstFile.Close(); err != nil && !isExpectedCloseErr(err) {
				logger.Default().Warn("close local file", zap.String("path", localPath), zap.Error(err))
			}
		}()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"message":"file downloaded successfully","local_path":"%s"}`, localPath), nil
}
