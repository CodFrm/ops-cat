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
	"os/exec"
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

	resp, err := client.Do(req)
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

	resp, err := client.Do(req)
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

// isNightlyVersion 判断是否为 nightly 版本
func isNightlyVersion(version string) bool {
	return strings.Contains(version, "nightly.") || strings.HasPrefix(version, "nightly-")
}

// hasUpdate 判断是否有更新
func hasUpdate(channel, currentVersion, latestVersion string) bool {
	if currentVersion == "dev" || currentVersion == "" {
		return true
	}

	isCurrentNightly := isNightlyVersion(currentVersion)

	if channel == ChannelNightly {
		if !isCurrentNightly {
			return true // 从 stable/beta 切换到 nightly
		}
		// 旧格式 nightly-YYYYMMDD-SHA 直接字符串比较
		if strings.HasPrefix(currentVersion, "nightly-") {
			return currentVersion != latestVersion
		}
		// 新格式使用语义化版本比较
		cv := strings.TrimPrefix(currentVersion, "v")
		lv := strings.TrimPrefix(latestVersion, "v")
		return compareVersions(lv, cv) > 0
	}

	// stable 或 beta 通道
	if isCurrentNightly {
		return true // 从 nightly 切换到 stable/beta
	}

	cv := strings.TrimPrefix(currentVersion, "v")
	lv := strings.TrimPrefix(latestVersion, "v")
	return compareVersions(lv, cv) > 0
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

	var downloadURL string
	var assetSize int64
	var assetName string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, platform) {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			assetName = asset.Name
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

	// 下载到临时文件（保留扩展名以便后续判断格式）
	ext := filepath.Ext(assetName)
	tmpFile, err := os.CreateTemp("", "opskat-update-*"+ext)
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

	if strings.HasSuffix(archivePath, ".dmg") {
		return updateMacOSFromDMG(archivePath, appDir)
	}
	return updateMacOSFromTarGz(archivePath, appDir)
}

// updateMacOSFromDMG 从 DMG 文件更新 macOS .app bundle
func updateMacOSFromDMG(dmgPath, appDir string) error {
	mountPoint, err := os.MkdirTemp("", "opskat-mount-*")
	if err != nil {
		return fmt.Errorf("create mount point failed: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(mountPoint); err != nil {
			logger.Default().Warn("remove mount point", zap.String("path", mountPoint), zap.Error(err))
		}
	}()

	// 挂载 DMG
	if output, err := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-nobrowse", "-quiet").CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("mount DMG failed: %s: %w", string(output), err)
	}
	defer func() {
		if output, err := exec.Command("hdiutil", "detach", mountPoint, "-quiet").CombinedOutput(); err != nil { //nolint:gosec
			logger.Default().Warn("unmount DMG", zap.String("output", string(output)), zap.Error(err))
		}
	}()

	newAppPath := filepath.Join(mountPoint, "opskat.app")
	if _, err := os.Stat(newAppPath); err != nil {
		return fmt.Errorf("app not found in DMG: %w", err)
	}

	// 备份旧的 .app
	backupDir := appDir + ".backup"
	if err := os.RemoveAll(backupDir); err != nil {
		logger.Default().Warn("remove old backup dir", zap.String("path", backupDir), zap.Error(err))
	}
	if err := os.Rename(appDir, backupDir); err != nil {
		return fmt.Errorf("backup old app failed: %w", err)
	}

	// 从挂载点复制新的 .app（跨挂载点无法 rename）
	if output, err := exec.Command("cp", "-R", newAppPath, appDir).CombinedOutput(); err != nil { //nolint:gosec
		if renameErr := os.Rename(backupDir, appDir); renameErr != nil {
			logger.Default().Error("restore backup after failed install", zap.Error(renameErr))
		}
		return fmt.Errorf("install new app failed: %s: %w", string(output), err)
	}

	if err := os.RemoveAll(backupDir); err != nil {
		logger.Default().Warn("remove backup dir", zap.String("path", backupDir), zap.Error(err))
	}
	return nil
}

// updateMacOSFromTarGz 从 tar.gz 更新 macOS .app bundle
func updateMacOSFromTarGz(archivePath, appDir string) error {
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

	newAppDir := filepath.Join(tmpExtractDir, "opskat.app")
	if _, err := os.Stat(newAppDir); err != nil {
		return fmt.Errorf("extracted app not found: %w", err)
	}

	backupDir := appDir + ".backup"
	if err := os.RemoveAll(backupDir); err != nil {
		logger.Default().Warn("remove old backup dir", zap.String("path", backupDir), zap.Error(err))
	}
	if err := os.Rename(appDir, backupDir); err != nil {
		return fmt.Errorf("backup old app failed: %w", err)
	}

	if err := os.Rename(newAppDir, appDir); err != nil {
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
	if strings.HasSuffix(archivePath, ".deb") {
		return updateLinuxFromDeb(archivePath, execPath)
	}
	return updateLinuxFromTarGz(archivePath, execPath)
}

// updateLinuxFromDeb 从 deb 包提取二进制并替换
func updateLinuxFromDeb(debPath, execPath string) error {
	tmpExtractDir, err := os.MkdirTemp("", "opskat-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpExtractDir); err != nil {
			logger.Default().Warn("remove temp extract dir", zap.String("path", tmpExtractDir), zap.Error(err))
		}
	}()

	if output, err := exec.Command("dpkg", "-x", debPath, tmpExtractDir).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("extract deb failed: %s: %w", string(output), err)
	}

	newBin := filepath.Join(tmpExtractDir, "usr", "bin", "opskat")
	if _, err := os.Stat(newBin); err != nil {
		return fmt.Errorf("extracted binary not found: %w", err)
	}

	return replaceBinary(newBin, execPath)
}

// updateLinuxFromTarGz 从 tar.gz 提取二进制并替换
func updateLinuxFromTarGz(archivePath, execPath string) error {
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

	return replaceBinary(newBin, execPath)
}

// replaceBinary 备份旧二进制并替换为新二进制
func replaceBinary(newBin, execPath string) error {
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
	if strings.HasSuffix(archivePath, ".exe") {
		return updateWindowsFromInstaller(archivePath, execPath)
	}
	return updateWindowsFromZip(archivePath, execPath)
}

// updateWindowsFromInstaller 运行 NSIS 安装程序静默更新（用户级安装，无需 UAC）
// Windows 不能覆盖正在运行的 exe，需要先 rename 再运行安装程序
func updateWindowsFromInstaller(installerPath, execPath string) error {
	installDir := filepath.Dir(execPath)

	// Windows 允许 rename 正在运行的 exe，但不允许覆盖
	backupPath := execPath + ".old"
	if err := os.Remove(backupPath); err != nil {
		logger.Default().Warn("remove old backup", zap.String("path", backupPath), zap.Error(err))
	}
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup running binary failed: %w", err)
	}

	// 运行 NSIS 安装程序，/D= 必须是最后一个参数且指定安装目录
	if err := runInstaller(installerPath, "/S", "/D="+installDir); err != nil {
		if renameErr := os.Rename(backupPath, execPath); renameErr != nil {
			logger.Default().Error("restore backup after failed install", zap.Error(renameErr))
		}
		return err
	}

	// 验证新 exe 是否已安装到位
	if _, err := os.Stat(execPath); err != nil {
		if renameErr := os.Rename(backupPath, execPath); renameErr != nil {
			logger.Default().Error("restore backup after missing binary", zap.Error(renameErr))
		}
		return fmt.Errorf("installer did not produce binary at %s", execPath)
	}

	return nil
}

// updateWindowsFromZip 从 zip 提取二进制并替换
func updateWindowsFromZip(archivePath, execPath string) error {
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
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
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
			if err := os.MkdirAll(target, 0755); err != nil {
				logger.Default().Warn("create directory", zap.String("path", target), zap.Error(err))
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
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

// parseChecksums 解析 SHA256SUMS.txt 内容
// 格式: "<sha256hex>  <filename>" 或 "<sha256hex> <filename>" (sha256sum 输出格式)
func parseChecksums(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// sha256sum 输出格式: hash + 两个空格 + 文件名，也兼容单空格
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		hash := parts[0]
		filename := parts[1]
		// sha256sum 在二进制模式下文件名前可能有 * 前缀
		filename = strings.TrimPrefix(filename, "*")
		result[filename] = hash
	}
	return result
}

// fetchChecksums 从 release assets 下载并解析 SHA256SUMS.txt
// 如果 release 中没有 SHA256SUMS.txt，返回 nil（跳过校验，兼容旧版本 release）
func fetchChecksums(assets []ReleaseAsset) (map[string]string, error) {
	var checksumURL string
	for _, asset := range assets {
		if asset.Name == "SHA256SUMS.txt" {
			checksumURL = asset.BrowserDownloadURL
			break
		}
	}
	if checksumURL == "" {
		return nil, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return nil, fmt.Errorf("download SHA256SUMS.txt failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Default().Warn("close checksum response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download SHA256SUMS.txt returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SHA256SUMS.txt failed: %w", err)
	}

	return parseChecksums(string(body)), nil
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
