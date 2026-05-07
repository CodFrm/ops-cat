package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opskat/opskat/internal/service/sftp_svc"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// --- SFTP 文件传输 ---

// SFTPGetwd 获取远程工作目录（用户 home）
func (a *App) SFTPGetwd(sessionID string) (string, error) {
	return a.sftpService.Getwd(sessionID)
}

// SFTPListDir 列出远程目录内容
func (a *App) SFTPListDir(sessionID, dirPath string) ([]sftp_svc.FileEntry, error) {
	return a.sftpService.ListDir(sessionID, dirPath)
}

// SFTPUpload 上传文件：弹出本地文件选择 → 上传到 remotePath
func (a *App) SFTPUpload(sessionID, remotePath string) (string, error) {
	localPath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件",
	})
	if err != nil {
		return "", fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil // 用户取消
	}

	// 如果 remotePath 以 / 结尾，则拼接本地文件名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localPath)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Upload(a.ctx, transferID, sessionID, localPath, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPUploadDir 上传目录：弹出本地目录选择 → 上传到 remotePath
func (a *App) SFTPUploadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件夹",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// remotePath 拼接本地目录名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localDir)
	} else {
		remotePath += "/" + filepath.Base(localDir)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.UploadDir(a.ctx, transferID, sessionID, localDir, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownload 下载文件：remotePath → 弹出本地保存对话框
func (a *App) SFTPDownload(sessionID, remotePath string) (string, error) {
	// 以远程文件名作为默认文件名
	defaultName := filepath.Base(remotePath)
	localPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "保存到本地",
	})
	if err != nil {
		return "", fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Download(a.ctx, transferID, sessionID, remotePath, localPath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownloadDir 下载目录：remotePath → 弹出本地目录选择
func (a *App) SFTPDownloadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择保存目录",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// 本地目录 + 远程目录名
	localDir = filepath.Join(localDir, filepath.Base(remotePath))

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.DownloadDir(a.ctx, transferID, sessionID, remotePath, localDir, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPUploadFile 直接上传本地文件或目录（不弹对话框，用于拖拽上传）
func (a *App) SFTPUploadFile(sessionID, localPath, remotePath string) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}

	transferID := a.sftpService.GenerateTransferID()
	emitProgress := func(p sftp_svc.TransferProgress) {
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
	}
	emitDone := func(err error) {
		if err != nil {
			emitProgress(sftp_svc.TransferProgress{TransferID: transferID, Status: "error", Error: err.Error()})
			return
		}
		emitProgress(sftp_svc.TransferProgress{TransferID: transferID, Status: "done"})
	}

	if info.IsDir() {
		dirRemotePath := remotePath
		if strings.HasSuffix(dirRemotePath, "/") {
			dirRemotePath += filepath.Base(localPath)
		} else {
			dirRemotePath += "/" + filepath.Base(localPath)
		}
		go func() {
			emitDone(a.sftpService.UploadDir(a.ctx, transferID, sessionID, localPath, dirRemotePath, emitProgress))
		}()
	} else {
		fileRemotePath := remotePath
		if strings.HasSuffix(fileRemotePath, "/") {
			fileRemotePath += filepath.Base(localPath)
		}
		go func() {
			emitDone(a.sftpService.Upload(a.ctx, transferID, sessionID, localPath, fileRemotePath, emitProgress))
		}()
	}

	return transferID, nil
}

// SFTPCancelTransfer 取消传输
func (a *App) SFTPCancelTransfer(transferID string) {
	a.sftpService.Cancel(transferID)
}

// SFTPDelete 删除远程文件或目录
func (a *App) SFTPDelete(sessionID, remotePath string, isDir bool) error {
	if isDir {
		return a.sftpService.RemoveDir(sessionID, remotePath)
	}
	return a.sftpService.Remove(sessionID, remotePath)
}

// --- 本地 SSH 密钥发现 ---

// LocalSSHKeyInfo 本地 SSH 密钥信息
type LocalSSHKeyInfo struct {
	Path        string `json:"path"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	IsEncrypted bool   `json:"isEncrypted"` // 密钥是否需要 passphrase
}

// ListLocalSSHKeys 扫描 ~/.ssh 目录，返回有效的私钥列表
func (a *App) ListLocalSSHKeys() ([]LocalSSHKeyInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %w", err)
	}
	sshDir := filepath.Join(homeDir, ".ssh")

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		// ~/.ssh 不存在时返回空列表
		if os.IsNotExist(err) {
			return []LocalSSHKeyInfo{}, nil
		}
		return nil, fmt.Errorf("读取 .ssh 目录失败: %w", err)
	}

	// 需要跳过的文件
	skipFiles := map[string]bool{
		"known_hosts":     true,
		"known_hosts.old": true,
		"config":          true,
		"authorized_keys": true,
		"environment":     true,
	}

	var keys []LocalSSHKeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过公钥、已知文件和隐藏文件
		if strings.HasSuffix(name, ".pub") || skipFiles[name] || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".sock") {
			continue
		}

		fullPath := filepath.Join(sshDir, name)
		info, err := parseLocalSSHKey(fullPath)
		if err != nil {
			continue // 不是有效私钥，跳过
		}
		keys = append(keys, *info)
	}

	if keys == nil {
		keys = []LocalSSHKeyInfo{}
	}
	return keys, nil
}

// SelectSSHKeyFile 打开文件选择框选择密钥文件，默认定位到 ~/.ssh
func (a *App) SelectSSHKeyFile() (*LocalSSHKeyInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Ctx(a.ctx).Warn("get user home dir", zap.Error(err))
	}
	defaultDir := filepath.Join(homeDir, ".ssh")

	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:            "选择 SSH 私钥文件",
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil // 用户取消
	}

	info, err := parseLocalSSHKey(filePath)
	if err != nil {
		return nil, fmt.Errorf("所选文件不是有效的 SSH 私钥: %w", err)
	}
	return info, nil
}

// parseLocalSSHKey 解析本地私钥文件，返回密钥信息
// 如果密钥有 passphrase 保护，返回 IsEncrypted=true
func parseLocalSSHKey(path string) (*LocalSSHKeyInfo, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from user file dialog
	if err != nil {
		return nil, err
	}
	// 快速检查：私钥文件通常以 "-----BEGIN" 开头或是 OpenSSH 格式
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		// 检查是否是加密密钥
		errStr := err.Error()
		if strings.Contains(errStr, "password protected") ||
			strings.Contains(errStr, "encrypted") ||
			strings.Contains(errStr, "passphrase") {
			// 尝试解析 PEM 格式获取密钥类型
			keyType := "unknown"
			if strings.Contains(string(data), "OPENSSH PRIVATE KEY") {
				keyType = "ssh-ed25519" // OpenSSH 格式默认 ED25519，实际类型需要 passphrase 后才能确定
			} else if strings.Contains(string(data), "RSA PRIVATE KEY") {
				keyType = "ssh-rsa"
			} else if strings.Contains(string(data), "EC PRIVATE KEY") {
				keyType = "ecdsa-sha2-nistp256"
			} else if strings.Contains(string(data), "DSA PRIVATE KEY") {
				keyType = "ssh-dss"
			}
			return &LocalSSHKeyInfo{
				Path:        path,
				KeyType:     keyType,
				Fingerprint: "",
				IsEncrypted: true,
			}, nil
		}
		return nil, err
	}

	pubKey := signer.PublicKey()
	fingerprint := ssh.FingerprintSHA256(pubKey)
	keyType := pubKey.Type()

	return &LocalSSHKeyInfo{
		Path:        path,
		KeyType:     keyType,
		Fingerprint: fingerprint,
		IsEncrypted: false,
	}, nil
}
