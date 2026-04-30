package sftp_svc

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opskat/opskat/internal/pkg/dirsync"
	"github.com/opskat/opskat/internal/service/ssh_svc"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/pkg/sftp"
	"go.uber.org/zap"
)

// TransferProgress 传输进度事件
type TransferProgress struct {
	TransferID     string `json:"transferId"`
	Status         string `json:"status"` // "progress" | "done" | "error"
	CurrentFile    string `json:"currentFile"`
	FilesCompleted int    `json:"filesCompleted"`
	FilesTotal     int    `json:"filesTotal"`
	BytesDone      int64  `json:"bytesDone"`
	BytesTotal     int64  `json:"bytesTotal"`
	Speed          int64  `json:"speed"` // bytes/sec
	Error          string `json:"error,omitempty"`
}

// Service SFTP 文件传输服务
type Service struct {
	sshManager *ssh_svc.Manager
	clients    sync.Map // sessionID -> *sftp.Client
	cancels    sync.Map // transferID -> context.CancelFunc
	counter    atomic.Int64
}

// NewService 创建 SFTP 服务
func NewService(sshManager *ssh_svc.Manager) *Service {
	return &Service{sshManager: sshManager}
}

// GenerateTransferID 生成唯一传输 ID
func (s *Service) GenerateTransferID() string {
	return fmt.Sprintf("sftp-%d-%d", time.Now().UnixNano(), s.counter.Add(1))
}

// getSFTPClient 获取或创建 SFTP 客户端（懒加载）
func (s *Service) getSFTPClient(sessionID string) (*sftp.Client, error) {
	if v, ok := s.clients.Load(sessionID); ok {
		client := v.(*sftp.Client)
		// 检查是否仍然可用
		if _, err := client.Getwd(); err == nil {
			return client, nil
		}
		// 已失效，移除
		s.clients.Delete(sessionID)
		if err := client.Close(); err != nil {
			logger.Default().Warn("close stale client", zap.String("sessionID", sessionID), zap.Error(err))
		}
	}

	sess, ok := s.sshManager.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("SSH 会话不存在: %s", sessionID)
	}
	if sess.IsClosed() {
		return nil, fmt.Errorf("SSH 会话已关闭: %s", sessionID)
	}

	client, err := sftp.NewClient(sess.Client())
	if err != nil {
		return nil, fmt.Errorf("创建 SFTP 客户端失败: %w", err)
	}

	s.clients.Store(sessionID, client)
	return client, nil
}

// Getwd 获取远程工作目录（用户 home）
func (s *Service) Getwd(sessionID string) (string, error) {
	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return "", err
	}
	return sftpClient.Getwd()
}

// ResolveDirectory validates that a remote directory exists and returns its canonical path.
func (s *Service) ResolveDirectory(sessionID, dirPath string) (string, error) {
	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return "", err
	}

	info, err := sftpClient.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", dirsync.Error(dirsync.CodeNotFound)
		}
		return "", dirsync.Error(dirsync.CodeAccessDenied)
	}
	if !info.IsDir() {
		return "", dirsync.Error(dirsync.CodeNotDirectory)
	}

	if realPath, realPathErr := sftpClient.RealPath(dirPath); realPathErr == nil && realPath != "" {
		return realPath, nil
	}
	return dirPath, nil
}

// ValidateDirectory 校验远程目录存在且可访问。
func (s *Service) ValidateDirectory(sessionID, dirPath string) error {
	_, err := s.ResolveDirectory(sessionID, dirPath)
	return err
}

// FileEntry 远程文件/目录条目
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"` // Unix timestamp
}

// ListDir 列出远程目录内容
func (s *Service) ListDir(sessionID, dirPath string) ([]FileEntry, error) {
	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return nil, err
	}

	infos, err := sftpClient.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("读取远程目录失败: %w", err)
	}

	// 排序：目录在前，文件在后，各自按名称排序
	var dirs, files []FileEntry
	for _, info := range infos {
		entry := FileEntry{
			Name:    info.Name(),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		}
		if info.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	result := make([]FileEntry, 0, len(dirs)+len(files))
	result = append(result, dirs...)
	result = append(result, files...)
	return result, nil
}

// Upload 上传单个文件
func (s *Service) Upload(ctx context.Context, transferID, sessionID, localPath, remotePath string, onProgress func(TransferProgress)) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancels.Store(transferID, cancel)
	defer func() {
		s.cancels.Delete(transferID)
		cancel()
	}()

	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}

	localFile, err := os.Open(localPath) //nolint:gosec // file path from user config
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() {
		if err := localFile.Close(); err != nil {
			logger.Default().Warn("close local file", zap.String("path", localPath), zap.Error(err))
		}
	}()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer func() {
		if err := remoteFile.Close(); err != nil {
			logger.Default().Warn("close remote file", zap.String("path", remotePath), zap.Error(err))
		}
	}()

	return s.copyWithProgress(ctx, transferID, remoteFile, localFile, stat.Size(), 1, filepath.Base(remotePath), onProgress)
}

// Download 下载单个文件
func (s *Service) Download(ctx context.Context, transferID, sessionID, remotePath, localPath string, onProgress func(TransferProgress)) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancels.Store(transferID, cancel)
	defer func() {
		s.cancels.Delete(transferID)
		cancel()
	}()

	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer func() {
		if err := remoteFile.Close(); err != nil {
			logger.Default().Warn("close remote file", zap.String("path", remotePath), zap.Error(err))
		}
	}()

	stat, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("获取远程文件信息失败: %w", err)
	}

	localFile, err := os.Create(localPath) //nolint:gosec // file path from user config
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer func() {
		if err := localFile.Close(); err != nil {
			logger.Default().Warn("close local file", zap.String("path", localPath), zap.Error(err))
		}
	}()

	return s.copyWithProgress(ctx, transferID, localFile, remoteFile, stat.Size(), 1, filepath.Base(remotePath), onProgress)
}

// UploadDir 上传目录
func (s *Service) UploadDir(ctx context.Context, transferID, sessionID, localDir, remoteDir string, onProgress func(TransferProgress)) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancels.Store(transferID, cancel)
	defer func() {
		s.cancels.Delete(transferID)
		cancel()
	}()

	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}

	// 扫描阶段：统计文件数和总大小
	var filesTotal int
	var bytesTotal int64
	if err := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.IsDir() {
			filesTotal++
			info, err := d.Info()
			if err != nil {
				return err
			}
			bytesTotal += info.Size()
		}
		return nil
	}); err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}

	// 传输阶段
	var filesCompleted int
	var bytesDone int64
	startTime := time.Now()
	lastEmit := time.Now()

	return filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			logger.Default().Warn("compute relative path", zap.String("base", localDir), zap.String("path", path), zap.Error(err))
			return err
		}
		remoteFull := remoteDir + "/" + filepath.ToSlash(relPath)

		if d.IsDir() {
			return sftpClient.MkdirAll(remoteFull)
		}

		// 上传文件
		localFile, err := os.Open(path) //nolint:gosec // file path from user config
		if err != nil {
			return err
		}
		defer func() {
			if err := localFile.Close(); err != nil {
				logger.Default().Warn("close local file", zap.String("path", path), zap.Error(err))
			}
		}()

		remoteFile, err := sftpClient.Create(remoteFull)
		if err != nil {
			return err
		}
		defer func() {
			if err := remoteFile.Close(); err != nil {
				logger.Default().Warn("close remote file", zap.String("path", remoteFull), zap.Error(err))
			}
		}()

		buf := make([]byte, 32*1024)
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n, readErr := localFile.Read(buf)
			if n > 0 {
				if _, writeErr := remoteFile.Write(buf[:n]); writeErr != nil {
					return writeErr
				}
				bytesDone += int64(n)

				if time.Since(lastEmit) >= 100*time.Millisecond {
					elapsed := time.Since(startTime).Seconds()
					speed := int64(0)
					if elapsed > 0 {
						speed = int64(float64(bytesDone) / elapsed)
					}
					onProgress(TransferProgress{
						TransferID:     transferID,
						Status:         "progress",
						CurrentFile:    relPath,
						FilesCompleted: filesCompleted,
						FilesTotal:     filesTotal,
						BytesDone:      bytesDone,
						BytesTotal:     bytesTotal,
						Speed:          speed,
					})
					lastEmit = time.Now()
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}

		filesCompleted++
		return nil
	})
}

// DownloadDir 下载目录
func (s *Service) DownloadDir(ctx context.Context, transferID, sessionID, remoteDir, localDir string, onProgress func(TransferProgress)) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancels.Store(transferID, cancel)
	defer func() {
		s.cancels.Delete(transferID)
		cancel()
	}()

	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}

	// 扫描阶段：递归统计远程目录
	type fileEntry struct {
		remotePath string
		size       int64
		isDir      bool
	}
	var entries []fileEntry
	var bytesTotal int64
	var filesTotal int

	var walk func(dir string) error
	walk = func(dir string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		infos, err := sftpClient.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, info := range infos {
			fullPath := dir + "/" + info.Name()
			if info.IsDir() {
				entries = append(entries, fileEntry{remotePath: fullPath, isDir: true})
				if err := walk(fullPath); err != nil {
					return err
				}
			} else {
				entries = append(entries, fileEntry{remotePath: fullPath, size: info.Size()})
				bytesTotal += info.Size()
				filesTotal++
			}
		}
		return nil
	}

	// 先创建根目录
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}
	if err := walk(remoteDir); err != nil {
		return fmt.Errorf("扫描远程目录失败: %w", err)
	}

	// 传输阶段
	var filesCompleted int
	var bytesDone int64
	startTime := time.Now()
	lastEmit := time.Now()

	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 计算相对路径
		relPath := entry.remotePath[len(remoteDir):]
		localFull := filepath.Join(localDir, filepath.FromSlash(relPath))

		if entry.isDir {
			if err := os.MkdirAll(localFull, 0755); err != nil {
				return err
			}
			continue
		}

		// 下载文件
		remoteFile, err := sftpClient.Open(entry.remotePath)
		if err != nil {
			return err
		}

		localFile, err := os.Create(localFull) //nolint:gosec // file path from user config
		if err != nil {
			if closeErr := remoteFile.Close(); closeErr != nil {
				logger.Default().Warn("close remote file", zap.String("path", entry.remotePath), zap.Error(closeErr))
			}
			return err
		}

		buf := make([]byte, 32*1024)
		for {
			if ctx.Err() != nil {
				if closeErr := localFile.Close(); closeErr != nil {
					logger.Default().Warn("close local file", zap.String("path", localFull), zap.Error(closeErr))
				}
				if closeErr := remoteFile.Close(); closeErr != nil {
					logger.Default().Warn("close remote file", zap.String("path", entry.remotePath), zap.Error(closeErr))
				}
				return ctx.Err()
			}
			n, readErr := remoteFile.Read(buf)
			if n > 0 {
				if _, writeErr := localFile.Write(buf[:n]); writeErr != nil {
					if closeErr := localFile.Close(); closeErr != nil {
						logger.Default().Warn("close local file", zap.String("path", localFull), zap.Error(closeErr))
					}
					if closeErr := remoteFile.Close(); closeErr != nil {
						logger.Default().Warn("close remote file", zap.String("path", entry.remotePath), zap.Error(closeErr))
					}
					return writeErr
				}
				bytesDone += int64(n)

				if time.Since(lastEmit) >= 100*time.Millisecond {
					elapsed := time.Since(startTime).Seconds()
					speed := int64(0)
					if elapsed > 0 {
						speed = int64(float64(bytesDone) / elapsed)
					}
					onProgress(TransferProgress{
						TransferID:     transferID,
						Status:         "progress",
						CurrentFile:    relPath,
						FilesCompleted: filesCompleted,
						FilesTotal:     filesTotal,
						BytesDone:      bytesDone,
						BytesTotal:     bytesTotal,
						Speed:          speed,
					})
					lastEmit = time.Now()
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				if closeErr := localFile.Close(); closeErr != nil {
					logger.Default().Warn("close local file", zap.String("path", localFull), zap.Error(closeErr))
				}
				if closeErr := remoteFile.Close(); closeErr != nil {
					logger.Default().Warn("close remote file", zap.String("path", entry.remotePath), zap.Error(closeErr))
				}
				return readErr
			}
		}

		if closeErr := localFile.Close(); closeErr != nil {
			logger.Default().Warn("close local file", zap.String("path", localFull), zap.Error(closeErr))
		}
		if closeErr := remoteFile.Close(); closeErr != nil {
			logger.Default().Warn("close remote file", zap.String("path", entry.remotePath), zap.Error(closeErr))
		}
		filesCompleted++
	}

	return nil
}

// Cancel 取消传输
func (s *Service) Cancel(transferID string) {
	if v, ok := s.cancels.Load(transferID); ok {
		v.(context.CancelFunc)()
	}
}

// CleanupSession 清理 SSH 会话关联的 SFTP 客户端
func (s *Service) CleanupSession(sessionID string) {
	if v, ok := s.clients.LoadAndDelete(sessionID); ok {
		if err := v.(*sftp.Client).Close(); err != nil {
			logger.Default().Warn("close client", zap.String("sessionID", sessionID), zap.Error(err))
		}
	}
}

// Remove 删除单个文件
func (s *Service) Remove(sessionID, path string) error {
	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}
	return sftpClient.Remove(path)
}

// RemoveDir 递归删除目录
func (s *Service) RemoveDir(sessionID, path string) error {
	sftpClient, err := s.getSFTPClient(sessionID)
	if err != nil {
		return err
	}
	return s.removeDirRecursive(sftpClient, path)
}

func (s *Service) removeDirRecursive(client *sftp.Client, path string) error {
	entries, err := client.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fullPath := path + "/" + entry.Name()
		if entry.IsDir() {
			if err := s.removeDirRecursive(client, fullPath); err != nil {
				return err
			}
		} else {
			if err := client.Remove(fullPath); err != nil {
				return err
			}
		}
	}
	return client.RemoveDirectory(path)
}

// copyWithProgress 带进度的文件拷贝
func (s *Service) copyWithProgress(ctx context.Context, transferID string, dst io.Writer, src io.Reader, totalBytes int64, filesTotal int, currentFile string, onProgress func(TransferProgress)) error {
	buf := make([]byte, 32*1024)
	var bytesDone int64
	startTime := time.Now()
	lastEmit := time.Now()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			bytesDone += int64(n)

			if time.Since(lastEmit) >= 100*time.Millisecond {
				elapsed := time.Since(startTime).Seconds()
				speed := int64(0)
				if elapsed > 0 {
					speed = int64(float64(bytesDone) / elapsed)
				}
				onProgress(TransferProgress{
					TransferID:     transferID,
					Status:         "progress",
					CurrentFile:    currentFile,
					FilesCompleted: 0,
					FilesTotal:     filesTotal,
					BytesDone:      bytesDone,
					BytesTotal:     totalBytes,
					Speed:          speed,
				})
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
