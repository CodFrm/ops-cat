package update_svc

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

const (
	githubRepo = "opskat/opskat"
	apiBaseURL = "https://api.github.com/repos/" + githubRepo

	// ChannelStable 稳定版更新通道
	ChannelStable = "stable"
	// ChannelBeta 测试版更新通道
	ChannelBeta = "beta"
	// ChannelNightly 每日构建更新通道
	ChannelNightly = "nightly"
)

// ReleaseAsset GitHub release 资产
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// ReleaseInfo GitHub release 信息
type ReleaseInfo struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	HTMLURL     string         `json:"html_url"`
	PublishedAt string         `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// UpdateInfo 更新检查结果
type UpdateInfo struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes"`
	ReleaseURL     string `json:"releaseURL"`
	PublishedAt    string `json:"publishedAt"`
}

// fetchRelease 根据通道获取对应的 release 信息
func fetchRelease(channel string) (*ReleaseInfo, error) {
	switch channel {
	case ChannelNightly:
		return fetchReleaseFromURL(apiBaseURL + "/releases/tags/nightly")
	case ChannelBeta:
		return fetchLatestBetaRelease()
	default:
		return fetchReleaseFromURL(apiBaseURL + "/releases/latest")
	}
}

// fetchReleaseFromURL 从指定 URL 获取单个 release
func fetchReleaseFromURL(url string) (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req) //nolint:gosec // request to GitHub API URL
	if err != nil {
		return nil, fmt.Errorf("request GitHub API failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Default().Warn("close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}
	return &release, nil
}

// fetchLatestBetaRelease 获取最新的 beta 或 stable release（排除 nightly）
func fetchLatestBetaRelease() (*ReleaseInfo, error) {
	url := apiBaseURL + "/releases?per_page=20"
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req) //nolint:gosec // request to GitHub API URL
	if err != nil {
		return nil, fmt.Errorf("request GitHub API failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Default().Warn("close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	for i := range releases {
		if releases[i].TagName != "nightly" {
			return &releases[i], nil
		}
	}
	return nil, fmt.Errorf("no beta or stable release found")
}

// CheckForUpdate 检查指定通道的最新版本
func CheckForUpdate(channel string) (*UpdateInfo, error) {
	if channel == "" {
		channel = ChannelStable
	}

	release, err := fetchRelease(channel)
	if err != nil {
		return nil, err
	}

	currentVersion := configs.Version
	latestVersion := release.TagName
	if channel == ChannelNightly {
		latestVersion = release.Name // nightly 用 release title 作为版本号
	}

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		ReleaseNotes:   release.Body,
		ReleaseURL:     release.HTMLURL,
		PublishedAt:    release.PublishedAt,
	}

	info.HasUpdate = hasUpdate(channel, currentVersion, latestVersion)
	return info, nil
}

// hasUpdate 判断是否有更新
func hasUpdate(channel, currentVersion, latestVersion string) bool {
	if currentVersion == "dev" || currentVersion == "" {
		return true
	}

	isCurrentNightly := strings.HasPrefix(currentVersion, "nightly-")

	if channel == ChannelNightly {
		if !isCurrentNightly {
			return true // 从 stable/beta 切换到 nightly
		}
		return currentVersion != latestVersion
	}

	// stable 或 beta 通道
	if isCurrentNightly {
		return true // 从 nightly 切换到 stable/beta
	}

	cv := strings.TrimPrefix(currentVersion, "v")
	lv := strings.TrimPrefix(latestVersion, "v")
	return lv != cv && compareVersions(lv, cv) > 0
}

// DownloadAndUpdate 下载指定通道的最新版本并替换当前二进制
func DownloadAndUpdate(channel string, onProgress func(downloaded, total int64)) error {
	if channel == "" {
		channel = ChannelStable
	}

	release, err := fetchRelease(channel)
	if err != nil {
		return err
	}

	// 找到当前平台的桌面端资产
	platform := runtime.GOOS + "-" + runtime.GOARCH
	assetName := fmt.Sprintf("opskat-%s", platform)

	var downloadURL string
	var assetSize int64
	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, assetName) {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for platform %s", platform)
	}

	// 下载资产
	dlClient := &http.Client{Timeout: 30 * time.Minute}
	dlResp, err := dlClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() {
		if err := dlResp.Body.Close(); err != nil {
			logger.Default().Warn("close download response body", zap.Error(err))
		}
	}()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", dlResp.StatusCode)
	}

	if assetSize == 0 {
		assetSize = dlResp.ContentLength
	}

	// 下载到临时文件
	tmpFile, err := os.CreateTemp("", "opskat-update-*")
	if err != nil {
		return fmt.Errorf("create temp file failed: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			logger.Default().Warn("remove temp file", zap.String("path", tmpPath), zap.Error(err))
		}
	}()

	var reader io.Reader = dlResp.Body
	if onProgress != nil {
		reader = &progressReader{r: dlResp.Body, total: assetSize, onProgress: onProgress}
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			logger.Default().Warn("close temp file after write error", zap.Error(closeErr))
		}
		return fmt.Errorf("download write failed: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		logger.Default().Warn("close temp file", zap.Error(err))
	}

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path failed: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve executable path failed: %w", err)
	}

	// 解压并替换
	switch runtime.GOOS {
	case "darwin":
		return updateMacOS(tmpPath, execPath)
	case "windows":
		return updateWindows(tmpPath, execPath)
	default:
		return updateLinux(tmpPath, execPath)
	}
}

// updateMacOS 更新 macOS .app bundle
func updateMacOS(archivePath, execPath string) error {
	// execPath 类似 /path/to/opskat.app/Contents/MacOS/opskat
	// 需要找到 .app 目录
	appDir := execPath
	for !strings.HasSuffix(appDir, ".app") && appDir != "/" {
		appDir = filepath.Dir(appDir)
	}
	if !strings.HasSuffix(appDir, ".app") {
		// 非 .app bundle，按 Linux 方式处理
		return updateLinux(archivePath, execPath)
	}

	parentDir := filepath.Dir(appDir)

	// 解压 tar.gz 到临时目录
	tmpExtractDir, err := os.MkdirTemp("", "opskat-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpExtractDir); err != nil {
			logger.Default().Warn("remove temp extract dir", zap.String("path", tmpExtractDir), zap.Error(err))
		}
	}()

	if err := extractTarGz(archivePath, tmpExtractDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// 找到解压出的 .app 目录
	newAppDir := filepath.Join(tmpExtractDir, "opskat.app")
	if _, err := os.Stat(newAppDir); err != nil {
		return fmt.Errorf("extracted app not found: %w", err)
	}

	// 备份旧的 .app
	backupDir := appDir + ".backup"
	if err := os.RemoveAll(backupDir); err != nil {
		logger.Default().Warn("remove old backup dir", zap.String("path", backupDir), zap.Error(err))
	}
	if err := os.Rename(appDir, backupDir); err != nil {
		return fmt.Errorf("backup old app failed: %w", err)
	}

	// 移动新的 .app 到原位置
	if err := os.Rename(newAppDir, filepath.Join(parentDir, "opskat.app")); err != nil {
		// 恢复备份
		if renameErr := os.Rename(backupDir, appDir); renameErr != nil {
			logger.Default().Error("restore backup after failed install", zap.Error(renameErr))
		}
		return fmt.Errorf("install new app failed: %w", err)
	}

	if err := os.RemoveAll(backupDir); err != nil {
		logger.Default().Warn("remove backup dir", zap.String("path", backupDir), zap.Error(err))
	}
	return nil
}

// updateLinux 更新 Linux 二进制
func updateLinux(archivePath, execPath string) error {
	// 解压 tar.gz
	tmpExtractDir, err := os.MkdirTemp("", "opskat-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpExtractDir); err != nil {
			logger.Default().Warn("remove temp extract dir", zap.String("path", tmpExtractDir), zap.Error(err))
		}
	}()

	if err := extractTarGz(archivePath, tmpExtractDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	newBin := filepath.Join(tmpExtractDir, "opskat")
	if _, err := os.Stat(newBin); err != nil {
		return fmt.Errorf("extracted binary not found: %w", err)
	}

	// 备份旧文件，替换新文件
	backupPath := execPath + ".backup"
	if err := os.Remove(backupPath); err != nil {
		logger.Default().Warn("remove old backup", zap.String("path", backupPath), zap.Error(err))
	}
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup old binary failed: %w", err)
	}

	if err := copyFile(newBin, execPath, 0755); err != nil {
		if renameErr := os.Rename(backupPath, execPath); renameErr != nil {
			logger.Default().Error("restore backup after failed install", zap.Error(renameErr))
		}
		return fmt.Errorf("install new binary failed: %w", err)
	}

	if err := os.Remove(backupPath); err != nil {
		logger.Default().Warn("remove backup", zap.String("path", backupPath), zap.Error(err))
	}
	return nil
}

// updateWindows 更新 Windows 二进制
func updateWindows(archivePath, execPath string) error {
	// 解压 zip
	tmpExtractDir, err := os.MkdirTemp("", "opskat-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpExtractDir); err != nil {
			logger.Default().Warn("remove temp extract dir", zap.String("path", tmpExtractDir), zap.Error(err))
		}
	}()

	if err := extractZip(archivePath, tmpExtractDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	newBin := filepath.Join(tmpExtractDir, "opskat.exe")
	if _, err := os.Stat(newBin); err != nil {
		return fmt.Errorf("extracted binary not found: %w", err)
	}

	// Windows 不能替换正在运行的 exe，重命名旧文件后复制新文件
	backupPath := execPath + ".old"
	if err := os.Remove(backupPath); err != nil {
		logger.Default().Warn("remove old backup", zap.String("path", backupPath), zap.Error(err))
	}
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup old binary failed: %w", err)
	}

	if err := copyFile(newBin, execPath, 0755); err != nil {
		if renameErr := os.Rename(backupPath, execPath); renameErr != nil {
			logger.Default().Error("restore backup after failed install", zap.Error(renameErr))
		}
		return fmt.Errorf("install new binary failed: %w", err)
	}

	// 旧的 .old 文件留着，下次启动时可以清理
	return nil
}

// extractTarGz 解压 tar.gz 到指定目录
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath) //nolint:gosec // extracting trusted archive
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Default().Warn("close archive file", zap.Error(err))
		}
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		if err := gz.Close(); err != nil {
			logger.Default().Warn("close gzip reader", zap.Error(err))
		}
	}()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// 安全检查: 防止路径遍历
		target := filepath.Join(destDir, header.Name) //nolint:gosec // extracting trusted archive
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil { //nolint:gosec // extracting trusted archive
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { //nolint:gosec // extracting trusted archive
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)) //nolint:gosec // extracting trusted archive
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil { //nolint:gosec // trusted archive source
				if closeErr := outFile.Close(); closeErr != nil {
					logger.Default().Warn("close extracted file after copy error", zap.Error(closeErr))
				}
				return err
			}
			if err := outFile.Close(); err != nil {
				logger.Default().Warn("close extracted file", zap.Error(err))
			}
		}
	}
	return nil
}

// extractZip 解压 zip 到指定目录
func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			logger.Default().Warn("close zip reader", zap.Error(err))
		}
	}()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name) //nolint:gosec // extracting trusted archive
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil { //nolint:gosec // extracting trusted archive
				logger.Default().Warn("create directory", zap.String("path", target), zap.Error(err))
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { //nolint:gosec // extracting trusted archive
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()) //nolint:gosec // extracting trusted archive
		if err != nil {
			if closeErr := rc.Close(); closeErr != nil {
				logger.Default().Warn("close zip entry after open error", zap.Error(closeErr))
			}
			return err
		}
		_, err = io.Copy(outFile, rc) //nolint:gosec // trusted archive source
		if closeErr := outFile.Close(); closeErr != nil {
			logger.Default().Warn("close extracted file", zap.Error(closeErr))
		}
		if closeErr := rc.Close(); closeErr != nil {
			logger.Default().Warn("close zip entry", zap.Error(closeErr))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制文件
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // copying trusted file
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			logger.Default().Warn("close source file", zap.String("path", src), zap.Error(err))
		}
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm) //nolint:gosec // copying trusted file
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			logger.Default().Warn("close destination file", zap.String("path", dst), zap.Error(err))
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

// progressReader 带进度回调的 reader
type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	onProgress func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.downloaded += int64(n)
	pr.onProgress(pr.downloaded, pr.total)
	return n, err
}

// compareVersions 比较两个版本号，支持预发布后缀
// 如 "1.0.0" vs "1.0.0-beta.1"，"1.0.0-beta.1" vs "1.0.0-beta.2"
// 返回: >0 表示 a 更新, <0 表示 b 更新, 0 表示相同
func compareVersions(a, b string) int {
	aBase, aPre := splitPreRelease(a)
	bBase, bPre := splitPreRelease(b)

	result := compareBase(aBase, bBase)
	if result != 0 {
		return result
	}

	// 同基础版本: 无预发布 > 有预发布 (stable > beta)
	if aPre == "" && bPre != "" {
		return 1
	}
	if aPre != "" && bPre == "" {
		return -1
	}
	if aPre == "" && bPre == "" {
		return 0
	}

	return comparePreRelease(aPre, bPre)
}

// splitPreRelease 分离基础版本和预发布后缀
// "1.0.0-beta.1" -> ("1.0.0", "beta.1")
func splitPreRelease(v string) (string, string) {
	idx := strings.Index(v, "-")
	if idx < 0 {
		return v, ""
	}
	return v[:idx], v[idx+1:]
}

// compareBase 比较基础版本号 (如 "1.0.0" vs "0.2.0")
func compareBase(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			aNum, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bNum, _ = strconv.Atoi(bParts[i])
		}
		if aNum != bNum {
			return aNum - bNum
		}
	}
	return 0
}

// comparePreRelease 比较预发布标识符
// "beta.1" vs "beta.2", "beta.1" vs "rc.1"
func comparePreRelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var ap, bp string
		if i < len(aParts) {
			ap = aParts[i]
		}
		if i < len(bParts) {
			bp = bParts[i]
		}

		aNum, aErr := strconv.Atoi(ap)
		bNum, bErr := strconv.Atoi(bp)
		if aErr == nil && bErr == nil {
			if aNum != bNum {
				return aNum - bNum
			}
		} else if ap != bp {
			return strings.Compare(ap, bp)
		}
	}
	return 0
}
