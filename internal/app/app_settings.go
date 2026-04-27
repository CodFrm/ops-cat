package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/buildinfo"
	"github.com/opskat/opskat/internal/embedded"
	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/pkg/executil"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/backup_svc"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/internal/service/import_svc"
	"github.com/opskat/opskat/internal/service/update_svc"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// --- GitHub Token ---

// SaveGitHubToken 加密保存 GitHub token
func (a *App) SaveGitHubToken(token, user string) error {
	cfg := bootstrap.GetConfig()
	if token == "" {
		cfg.GitHubToken = ""
		cfg.GitHubUser = ""
	} else {
		encrypted, err := credential_svc.Default().Encrypt(token)
		if err != nil {
			return fmt.Errorf("加密 GitHub Token 失败: %w", err)
		}
		cfg.GitHubToken = encrypted
		cfg.GitHubUser = user
	}
	return bootstrap.SaveConfig(cfg)
}

// GetGitHubToken 获取解密后的 GitHub token
func (a *App) GetGitHubToken() (string, error) {
	cfg := bootstrap.GetConfig()
	if cfg.GitHubToken == "" {
		return "", nil
	}
	return credential_svc.Default().Decrypt(cfg.GitHubToken)
}

// GetStoredGitHubUser 获取保存的 GitHub 用户名
func (a *App) GetStoredGitHubUser() string {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return ""
	}
	return cfg.GitHubUser
}

// ClearGitHubToken 清除保存的 GitHub token
func (a *App) ClearGitHubToken() error {
	return a.SaveGitHubToken("", "")
}

// --- 导入导出 ---

// PreviewTabbyConfig 预览 Tabby 配置（不写入数据库）
// 自动检测默认路径，找不到则弹出文件选择框
func (a *App) PreviewTabbyConfig() (*import_svc.PreviewResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.PreviewTabbyConfig(a.langCtx(), data)
}

// ImportTabbySelected 导入用户选中的 Tabby 连接
func (a *App) ImportTabbySelected(selectedIndexes []int, passphrase string, overwrite bool) (*import_svc.ImportResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.ImportTabbySelected(a.langCtx(), data, selectedIndexes, import_svc.ImportOptions{
		Passphrase: passphrase,
		Overwrite:  overwrite,
	})
}

// PreviewSSHConfig 预览 SSH Config 文件（不写入数据库）
// 自动检测 ~/.ssh/config，找不到则弹出文件选择框
func (a *App) PreviewSSHConfig() (*import_svc.PreviewResult, error) {
	data, err := a.readSSHConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.PreviewSSHConfig(a.langCtx(), data)
}

// ImportSSHConfigSelected 导入用户选中的 SSH Config 连接
func (a *App) ImportSSHConfigSelected(selectedIndexes []int, overwrite bool) (*import_svc.ImportResult, error) {
	data, err := a.readSSHConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.ImportSSHConfigSelected(a.langCtx(), data, selectedIndexes, import_svc.ImportOptions{
		Overwrite: overwrite,
	})
}

// readSSHConfig 读取 SSH Config 文件
func (a *App) readSSHConfig() ([]byte, error) {
	filePath := import_svc.DetectSSHConfigPath()
	if filePath == "" {
		var err error
		filePath, err = wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择 SSH Config 文件",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "All Files", Pattern: "*"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("打开文件对话框失败: %w", err)
		}
		if filePath == "" {
			return nil, nil
		}
	}
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog or known config path
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return data, nil
}

// readTabbyConfig 读取 Tabby 配置文件内容
func (a *App) readTabbyConfig() ([]byte, error) {
	filePath := detectTabbyConfigPath()
	if filePath == "" {
		var err error
		filePath, err = wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择 Tabby 配置文件",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
				{DisplayName: "All Files", Pattern: "*"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("打开文件对话框失败: %w", err)
		}
		if filePath == "" {
			return nil, nil
		}
	}
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog or known config path
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return data, nil
}

// detectTabbyConfigPath 检测 Tabby 配置文件默认路径
func detectTabbyConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(homeDir, "Library", "Application Support", "tabby", "config.yaml"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			candidates = []string{
				filepath.Join(appData, "Tabby", "config.yaml"),
			}
		}
	case "linux":
		candidates = []string{
			filepath.Join(homeDir, ".config", "tabby", "config.yaml"),
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil { //nolint:gosec // path is from known config locations
			return path
		}
	}
	return ""
}

// ExportData 导出所有资产和分组为 JSON（剪贴板用，不含凭据）
func (a *App) ExportData() (string, error) {
	opts := &backup_svc.ExportOptions{}
	data, err := backup_svc.Export(a.langCtx(), opts, nil)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// --- 备份操作 ---

// ExportToFile 导出备份到文件
func (a *App) ExportToFile(password string, opts backup_svc.ExportOptions) error {
	if opts.IncludeCredentials && password == "" {
		return fmt.Errorf("包含凭据时必须设置备份密码")
	}

	var crypto backup_svc.CredentialCrypto
	if opts.IncludeCredentials {
		crypto = credential_svc.Default()
	}

	data, err := backup_svc.Export(a.langCtx(), &opts, crypto)
	if err != nil {
		return err
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	var output []byte
	var defaultName string
	if password != "" {
		output, err = backup_svc.EncryptBackup(jsonData, password)
		if err != nil {
			return err
		}
		defaultName = fmt.Sprintf("opskat-backup-%s.encrypted.json", time.Now().Format("20060102"))
	} else {
		output = jsonData
		defaultName = fmt.Sprintf("opskat-backup-%s.json", time.Now().Format("20060102"))
	}

	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil
	}

	return os.WriteFile(filePath, output, 0644)
}

// ImportFileInfo 导入文件信息
type ImportFileInfo struct {
	FilePath  string                    `json:"filePath"`
	Encrypted bool                      `json:"encrypted"`
	Summary   *backup_svc.BackupSummary `json:"summary,omitempty"`
}

// SelectImportFile 选择备份文件并检测是否加密，返回概览信息
func (a *App) SelectImportFile() (*ImportFileInfo, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "导入备份",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}

	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from file dialog
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	info := &ImportFileInfo{
		FilePath:  filePath,
		Encrypted: backup_svc.IsEncryptedBackup(fileData),
	}
	// 非加密备份可直接解析概览
	if !info.Encrypted {
		var data backup_svc.BackupData
		if err := json.Unmarshal(fileData, &data); err == nil {
			info.Summary = data.Summary()
		}
	}
	return info, nil
}

// PreviewImportFile 解密并预览备份文件概览
func (a *App) PreviewImportFile(filePath, password string) (*backup_svc.BackupSummary, error) {
	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from previous file dialog
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return nil, err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}
	summary := data.Summary()
	summary.Encrypted = backup_svc.IsEncryptedBackup(fileData)
	return summary, nil
}

// ExecuteImportFile 执行文件导入
func (a *App) ExecuteImportFile(filePath, password string, opts backup_svc.ImportOptions) (*backup_svc.ImportResult, error) {
	fileData, err := os.ReadFile(filePath) //nolint:gosec // filePath is from previous file dialog selection
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return nil, err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data, &opts, credential_svc.Default())
}

// --- GitHub 认证 ---

// StartGitHubDeviceFlow 发起 GitHub Device Flow 认证
func (a *App) StartGitHubDeviceFlow() (*backup_svc.DeviceFlowInfo, error) {
	return backup_svc.StartDeviceFlow()
}

// WaitGitHubDeviceAuth 等待用户完成 GitHub 授权，返回 access_token
func (a *App) WaitGitHubDeviceAuth(deviceCode string, interval int) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	a.githubAuthCancel = cancel
	defer func() {
		cancel()
		a.githubAuthCancel = nil
	}()
	return backup_svc.PollDeviceAuth(ctx, deviceCode, interval)
}

// CancelGitHubAuth 取消 GitHub 授权等待
func (a *App) CancelGitHubAuth() {
	if a.githubAuthCancel != nil {
		a.githubAuthCancel()
	}
}

// GetGitHubUser 获取 GitHub 用户信息
func (a *App) GetGitHubUser(token string) (*backup_svc.GitHubUser, error) {
	return backup_svc.GetGitHubUser(token)
}

// WebDAVStoredConfig 是前端可读取的 WebDAV 配置，不包含明文密码。
type WebDAVStoredConfig struct {
	URL         string `json:"url"`
	Username    string `json:"username,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
	Configured  bool   `json:"configured"`
}

// --- Gist 备份 ---

// ExportToGist 加密并上传备份到 Gist
func (a *App) ExportToGist(password, token, gistID string, opts backup_svc.ExportOptions) (*backup_svc.GistInfo, error) {
	var crypto backup_svc.CredentialCrypto
	if opts.IncludeCredentials {
		crypto = credential_svc.Default()
	}

	data, err := backup_svc.Export(a.langCtx(), &opts, crypto)
	if err != nil {
		return nil, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := backup_svc.EncryptBackup(jsonData, password)
	if err != nil {
		return nil, err
	}

	return backup_svc.CreateOrUpdateGist(token, gistID, encrypted)
}

// ListBackupGists 列出用户的备份 Gist
func (a *App) ListBackupGists(token string) ([]*backup_svc.GistInfo, error) {
	return backup_svc.ListBackupGists(token)
}

// PreviewGistBackup 预览 Gist 备份概览
func (a *App) PreviewGistBackup(gistID, password, token string) (*backup_svc.BackupSummary, error) {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return nil, err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return nil, err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}
	summary := data.Summary()
	summary.Encrypted = true
	return summary, nil
}

// ImportFromGist 从 Gist 导入备份
func (a *App) ImportFromGist(gistID, password, token string, opts backup_svc.ImportOptions) (*backup_svc.ImportResult, error) {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return nil, err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return nil, err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data, &opts, credential_svc.Default())
}

// --- WebDAV 备份 ---

// SaveWebDAVConfig 保存 WebDAV 备份配置。密码为空时保留已有密码。
func (a *App) SaveWebDAVConfig(rawURL, username, password string) error {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	rawURL = strings.TrimSpace(rawURL)
	username = strings.TrimSpace(username)
	if rawURL == "" {
		return fmt.Errorf("WebDAV URL 不能为空")
	}
	// 校验 URL：拒绝形如 https://user:pass@host/path 的 URL，避免把明文凭据写进 config.json。
	if err := backup_svc.ValidateWebDAVURL(rawURL); err != nil {
		return err
	}
	cfg.WebDAVURL = rawURL
	cfg.WebDAVUsername = username
	if password != "" {
		encrypted, err := credential_svc.Default().Encrypt(password)
		if err != nil {
			return fmt.Errorf("加密 WebDAV 密码失败: %w", err)
		}
		cfg.WebDAVPassword = encrypted
	}
	return bootstrap.SaveConfig(cfg)
}

// GetWebDAVConfig 获取已保存的 WebDAV 配置状态。
func (a *App) GetWebDAVConfig() (*WebDAVStoredConfig, error) {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return &WebDAVStoredConfig{}, nil
	}
	return &WebDAVStoredConfig{
		URL:         cfg.WebDAVURL,
		Username:    cfg.WebDAVUsername,
		PasswordSet: cfg.WebDAVPassword != "",
		Configured:  strings.TrimSpace(cfg.WebDAVURL) != "",
	}, nil
}

// ClearWebDAVConfig 清除 WebDAV 备份配置。
func (a *App) ClearWebDAVConfig() error {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	cfg.WebDAVURL = ""
	cfg.WebDAVUsername = ""
	cfg.WebDAVPassword = ""
	return bootstrap.SaveConfig(cfg)
}

// TestWebDAVConfig 测试 WebDAV 目录是否可访问。
func (a *App) TestWebDAVConfig(rawURL, username, password string) error {
	rawURL = strings.TrimSpace(rawURL)
	username = strings.TrimSpace(username)
	cfg := backup_svc.WebDAVConfig{URL: rawURL, Username: username, Password: password}

	stored := bootstrap.GetConfig()
	if password == "" && stored != nil &&
		strings.TrimSpace(stored.WebDAVURL) == rawURL &&
		strings.TrimSpace(stored.WebDAVUsername) == username &&
		stored.WebDAVPassword != "" {
		decrypted, err := credential_svc.Default().Decrypt(stored.WebDAVPassword)
		if err != nil {
			return fmt.Errorf("解密 WebDAV 密码失败: %w", err)
		}
		cfg.Password = decrypted
	}

	return backup_svc.TestWebDAVConnection(cfg)
}

// ListWebDAVBackups 列出 WebDAV 目录中的 OpsKat 备份。
func (a *App) ListWebDAVBackups() ([]*backup_svc.WebDAVBackupInfo, error) {
	cfg, err := a.webDAVConfigFromStorage()
	if err != nil {
		return nil, err
	}
	return backup_svc.ListWebDAVBackups(cfg)
}

// ExportToWebDAV 加密并上传备份到 WebDAV。
func (a *App) ExportToWebDAV(password string, opts backup_svc.ExportOptions) (*backup_svc.WebDAVBackupInfo, error) {
	if password == "" {
		return nil, fmt.Errorf("WebDAV 备份必须设置备份密码")
	}
	cfg, err := a.webDAVConfigFromStorage()
	if err != nil {
		return nil, err
	}

	var crypto backup_svc.CredentialCrypto
	if opts.IncludeCredentials {
		crypto = credential_svc.Default()
	}

	data, err := backup_svc.Export(a.langCtx(), &opts, crypto)
	if err != nil {
		return nil, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := backup_svc.EncryptBackup(jsonData, password)
	if err != nil {
		return nil, err
	}

	return backup_svc.CreateOrUpdateWebDAVBackup(cfg, encrypted)
}

// ImportFromWebDAV 从 WebDAV 导入备份。
func (a *App) ImportFromWebDAV(name, password string, opts backup_svc.ImportOptions) (*backup_svc.ImportResult, error) {
	cfg, err := a.webDAVConfigFromStorage()
	if err != nil {
		return nil, err
	}
	content, err := backup_svc.GetWebDAVBackupContent(cfg, name)
	if err != nil {
		return nil, err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return nil, err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data, &opts, credential_svc.Default())
}

func (a *App) webDAVConfigFromStorage() (backup_svc.WebDAVConfig, error) {
	cfg := bootstrap.GetConfig()
	if cfg == nil || strings.TrimSpace(cfg.WebDAVURL) == "" {
		return backup_svc.WebDAVConfig{}, fmt.Errorf("WebDAV 未配置")
	}

	password := ""
	if cfg.WebDAVPassword != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.WebDAVPassword)
		if err != nil {
			return backup_svc.WebDAVConfig{}, fmt.Errorf("解密 WebDAV 密码失败: %w", err)
		}
		password = decrypted
	}

	return backup_svc.WebDAVConfig{
		URL:      cfg.WebDAVURL,
		Username: cfg.WebDAVUsername,
		Password: password,
	}, nil
}

// GetDataDir 返回应用数据目录
func (a *App) GetDataDir() string {
	return bootstrap.AppDataDir()
}

// OpenDirectory 在系统文件管理器中打开指定目录
func (a *App) OpenDirectory(path string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{path}
	case "windows":
		cmd = "explorer"
		args = []string{path}
	default: // linux
		cmd = "xdg-open"
		args = []string{path}
	}
	c := exec.Command(cmd, args...) //nolint:gosec
	return c.Start()
}

// --- Opsctl 安装 ---

// OpsctlInfo opsctl CLI 检测结果
type OpsctlInfo struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Embedded  bool   `json:"embedded"` // 桌面端是否内嵌了 opsctl 二进制
}

// DetectOpsctl 检测 opsctl CLI 是否已安装
func (a *App) DetectOpsctl() OpsctlInfo {
	info := OpsctlInfo{
		Embedded: embedded.HasEmbeddedOpsctl(),
	}
	opsctlPath, err := exec.LookPath("opsctl")
	if err != nil {
		// LookPath 用的是进程启动时的 PATH，安装后当前进程感知不到
		// 直接检查默认安装路径
		binName := "opsctl"
		if runtime.GOOS == "windows" {
			binName = "opsctl.exe"
		}
		candidate := filepath.Join(embedded.DefaultInstallDir(), binName)
		if _, statErr := os.Stat(candidate); statErr != nil {
			return info
		}
		opsctlPath = candidate
	}
	info.Installed = true
	info.Path = opsctlPath
	versionCmd := exec.Command(opsctlPath, "version") //nolint:gosec
	executil.HideWindow(versionCmd)
	out, err := versionCmd.Output()
	if err == nil {
		info.Version = strings.TrimSpace(string(out))
	}
	return info
}

// GetOpsctlInstallDir 返回默认安装目录
func (a *App) GetOpsctlInstallDir() string {
	return embedded.DefaultInstallDir()
}

// InstallOpsctl 将内嵌的 opsctl 二进制安装到指定目录
func (a *App) InstallOpsctl(targetDir string) (string, error) {
	if targetDir == "" {
		targetDir = embedded.DefaultInstallDir()
	}
	return embedded.InstallOpsctl(targetDir)
}

// --- Skills / Plugin ---

// SkillTarget AI Skill 安装目标
type SkillTarget struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
}

// skillInstallType 安装格式类型
type skillInstallType int

const (
	installClaude skillInstallType = iota // Claude Code 插件格式
	installSkill                          // 普通 SKILL.md 格式（Codex/OpenCode）
	installGemini                         // Gemini CLI 扩展格式
)

// skillTargetDefs 支持的 Skill 安装目标，添加新 CLI 只需在此追加
var skillTargetDefs = []struct {
	Name     string                   // 显示名称
	Type     skillInstallType         // 安装格式
	SkillFn  func(home string) string // 返回安装目录
	DetectFn func(path string) bool   // 检测是否已安装
}{
	{
		"Claude Code", installClaude,
		func(home string) string { return claudePluginDir(home) },
		func(path string) bool {
			_, err := os.Stat(filepath.Join(path, ".claude-plugin", "plugin.json"))
			return err == nil
		},
	},
	{
		"Codex", installSkill,
		func(home string) string { return filepath.Join(home, ".codex", "skills", "opsctl") },
		func(path string) bool {
			_, err := os.Stat(filepath.Join(path, "SKILL.md"))
			return err == nil
		},
	},
	{
		"OpenCode", installSkill,
		func(home string) string { return filepath.Join(home, ".config", "opencode", "skills", "opsctl") },
		func(path string) bool {
			_, err := os.Stat(filepath.Join(path, "SKILL.md"))
			return err == nil
		},
	},
	{
		"Gemini CLI", installGemini,
		func(home string) string { return filepath.Join(home, ".gemini", "extensions", "opsctl") },
		func(path string) bool {
			_, err := os.Stat(filepath.Join(path, "gemini-extension.json"))
			return err == nil
		},
	},
}

const pluginRegistryName = "opskat"
const pluginName = "opsctl"
const pluginVersion = "1.0.0"

// claudePluginDir 返回 Claude Code 插件目录（marketplace 内的插件根目录）
func claudePluginDir(home string) string {
	return filepath.Join(home, ".claude", "plugins", "marketplaces", pluginRegistryName, pluginName)
}

// claudeMarketplaceDir 返回 Claude Code 市场目录
func claudeMarketplaceDir(home string) string {
	return filepath.Join(home, ".claude", "plugins", "marketplaces", pluginRegistryName)
}

// DetectSkills 检测所有 AI 工具的 Skill 安装状态
func (a *App) DetectSkills() []SkillTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	targets := make([]SkillTarget, 0, len(skillTargetDefs))
	for _, def := range skillTargetDefs {
		path := def.SkillFn(home)
		targets = append(targets, SkillTarget{
			Name:      def.Name,
			Installed: def.DetectFn(path),
			Path:      path,
		})
	}
	return targets
}

// skillMDWithDataDir 返回注入数据目录后的 SKILL.md 内容
func (a *App) skillMDWithDataDir() string {
	dataDir := bootstrap.AppDataDir()
	insertion := "## Data Directory\n\n" + dataDir + "\n\n"
	return strings.Replace(a.skillContent.SkillMD, "## Global Flags", insertion+"## Global Flags", 1)
}

// installPluginTo 将 Skill 以插件格式安装到 Claude Code
// pluginDir 是 marketplace 内的插件根目录（marketplaces/opskat/opsctl/）
func (a *App) installPluginTo(pluginDir, home string) error {
	// 创建插件目录结构（插件和市场 manifest 都在 marketplace 目录树内）
	mktDir := claudeMarketplaceDir(home)
	dirs := []string{
		filepath.Join(pluginDir, ".claude-plugin"),
		filepath.Join(pluginDir, "skills", "opsctl", "references"),
		filepath.Join(pluginDir, "commands"),
		filepath.Join(mktDir, ".claude-plugin"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s failed: %w", d, err)
		}
	}

	// 写入插件文件
	files := map[string]string{
		filepath.Join(pluginDir, ".claude-plugin", "plugin.json"):                 a.skillContent.PluginJSON,
		filepath.Join(pluginDir, ".claude-plugin", "marketplace.json"):            a.skillContent.PluginMarketplaceJSON,
		filepath.Join(pluginDir, "skills", "opsctl", "SKILL.md"):                  a.skillMDWithDataDir(),
		filepath.Join(pluginDir, "skills", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
		filepath.Join(pluginDir, "commands", "init.md"):                           a.skillContent.InitMD,
		// 市场根目录 manifest
		filepath.Join(mktDir, ".claude-plugin", "marketplace.json"): a.skillContent.MarketplaceJSON,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s failed: %w", filepath.Base(path), err)
		}
	}

	// 注册到 installed_plugins.json + known_marketplaces.json + settings.json
	if err := a.registerPlugin(home); err != nil {
		return fmt.Errorf("register plugin failed: %w", err)
	}

	return nil
}

// registerPlugin 注册插件到 installed_plugins.json + known_marketplaces.json + settings.json
func (a *App) registerPlugin(home string) error {
	pluginsDir := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	key := pluginName + "@" + pluginRegistryName
	pluginPath := claudePluginDir(home) // marketplaces/opskat/opsctl
	mktPath := claudeMarketplaceDir(home)

	// 1. installed_plugins.json
	type pluginEntry struct {
		Scope       string `json:"scope"`
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
		InstalledAt string `json:"installedAt"`
		LastUpdated string `json:"lastUpdated"`
	}
	type pluginsConfig struct {
		Version int                      `json:"version"`
		Plugins map[string][]pluginEntry `json:"plugins"`
	}

	pluginsFile := filepath.Join(pluginsDir, "installed_plugins.json")
	cfg := pluginsConfig{Version: 2, Plugins: make(map[string][]pluginEntry)}
	if data, err := os.ReadFile(pluginsFile); err == nil { //nolint:gosec // path from app data dir
		if err := json.Unmarshal(data, &cfg); err != nil {
			logger.Default().Warn("parse installed_plugins.json failed, will overwrite", zap.Error(err))
			cfg = pluginsConfig{Version: 2, Plugins: make(map[string][]pluginEntry)}
		}
	}

	entries := cfg.Plugins[key]
	found := false
	for i, e := range entries {
		if e.Scope == "user" {
			entries[i].InstallPath = pluginPath
			entries[i].Version = pluginVersion
			entries[i].LastUpdated = now
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, pluginEntry{
			Scope:       "user",
			InstallPath: pluginPath,
			Version:     pluginVersion,
			InstalledAt: now,
			LastUpdated: now,
		})
	}
	cfg.Plugins[key] = entries
	if err := writeJSON(pluginsFile, cfg); err != nil {
		return fmt.Errorf("write installed_plugins.json: %w", err)
	}

	// 2. known_marketplaces.json
	kmFile := filepath.Join(pluginsDir, "known_marketplaces.json")
	km := make(map[string]any)
	if data, err := os.ReadFile(kmFile); err == nil { //nolint:gosec // path from app data dir
		json.Unmarshal(data, &km) //nolint:errcheck,gosec // best-effort merge
	}
	km[pluginRegistryName] = map[string]any{
		"source":          map[string]any{"source": "directory", "path": mktPath},
		"installLocation": mktPath,
		"lastUpdated":     now,
	}
	if err := writeJSON(kmFile, km); err != nil {
		return fmt.Errorf("write known_marketplaces.json: %w", err)
	}

	// 3. settings.json — enabledPlugins + extraKnownMarketplaces
	settingsFile := filepath.Join(home, ".claude", "settings.json")
	sc := make(map[string]any)
	if data, err := os.ReadFile(settingsFile); err == nil { //nolint:gosec // path from app data dir
		json.Unmarshal(data, &sc) //nolint:errcheck,gosec // best-effort merge
	}
	ep, _ := sc["enabledPlugins"].(map[string]any)
	if ep == nil {
		ep = make(map[string]any)
	}
	ep[key] = true
	sc["enabledPlugins"] = ep

	ekm, _ := sc["extraKnownMarketplaces"].(map[string]any)
	if ekm == nil {
		ekm = make(map[string]any)
	}
	ekm[pluginRegistryName] = map[string]any{
		"source": map[string]any{"source": "directory", "path": mktPath},
	}
	sc["extraKnownMarketplaces"] = ekm
	if err := writeJSON(settingsFile, sc); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}

	return nil
}

// writeJSON 将数据写入 JSON 文件
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// installSkillTo 将 Skill 文件以普通格式安装到目标目录（Codex/OpenCode）
// Codex/OpenCode 没有 commands/ 机制，init.md 作为 references 供自动加载
func (a *App) installSkillTo(skillDir string) error {
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	files := map[string]string{
		filepath.Join(skillDir, "SKILL.md"):   a.skillMDWithDataDir(),
		filepath.Join(refsDir, "commands.md"): a.skillContent.CommandsMD,
		filepath.Join(refsDir, "init.md"):     a.skillContent.InitMD,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s failed: %w", filepath.Base(path), err)
		}
	}

	return nil
}

// installGeminiExtension 将 Skill 以 Gemini CLI 扩展格式安装
// extDir = ~/.gemini/extensions/opsctl/
func (a *App) installGeminiExtension(extDir string) error {
	dirs := []string{
		filepath.Join(extDir, "skills", "opsctl", "references"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s failed: %w", d, err)
		}
	}

	// 扩展清单（version 为必填字段）
	manifest := `{"name":"opsctl","version":"` + pluginVersion + `"}` + "\n"

	files := map[string]string{
		filepath.Join(extDir, "gemini-extension.json"):                         manifest,
		filepath.Join(extDir, "GEMINI.md"):                                     "See the opsctl skill in skills/opsctl/ for asset management instructions.\n",
		filepath.Join(extDir, "skills", "opsctl", "SKILL.md"):                  a.skillMDWithDataDir(),
		filepath.Join(extDir, "skills", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
		filepath.Join(extDir, "skills", "opsctl", "references", "init.md"):     a.skillContent.InitMD,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s failed: %w", filepath.Base(path), err)
		}
	}

	return nil
}

// installTarget 根据安装类型分发到对应安装方法
func (a *App) installTarget(def struct {
	Name     string
	Type     skillInstallType
	SkillFn  func(home string) string
	DetectFn func(path string) bool
}, home string) error {
	path := def.SkillFn(home)
	switch def.Type {
	case installClaude:
		return a.installPluginTo(path, home)
	case installSkill:
		return a.installSkillTo(path)
	case installGemini:
		return a.installGeminiExtension(path)
	default:
		return fmt.Errorf("unknown install type: %d", def.Type)
	}
}

// InstallSkills 安装 Skill 文件到所有支持的 AI 工具
func (a *App) InstallSkills() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory failed: %w", err)
	}

	for _, def := range skillTargetDefs {
		if err := a.installTarget(def, home); err != nil {
			return fmt.Errorf("install %s failed: %w", def.Name, err)
		}
	}

	// 在应用数据目录写一份各工具的插件结构，方便用户手动拷贝
	if err := a.writePluginReference(); err != nil {
		logger.Default().Warn("write plugin reference failed", zap.Error(err))
	}

	return nil
}

// GetPluginReferenceDir 返回应用数据目录下的插件参考目录
func (a *App) GetPluginReferenceDir() string {
	return filepath.Join(bootstrap.AppDataDir(), "plugins")
}

// writePluginReference 在数据目录写各工具的插件目录结构
func (a *App) writePluginReference() error {
	base := a.GetPluginReferenceDir()
	skillMD := a.skillMDWithDataDir()

	structures := []struct {
		files map[string]string
	}{
		// Claude Code
		{files: map[string]string{
			filepath.Join(base, "claude-code", ".claude-plugin", "marketplace.json"):                      a.skillContent.MarketplaceJSON,
			filepath.Join(base, "claude-code", "opsctl", ".claude-plugin", "plugin.json"):                 a.skillContent.PluginJSON,
			filepath.Join(base, "claude-code", "opsctl", ".claude-plugin", "marketplace.json"):            a.skillContent.PluginMarketplaceJSON,
			filepath.Join(base, "claude-code", "opsctl", "skills", "opsctl", "SKILL.md"):                  skillMD,
			filepath.Join(base, "claude-code", "opsctl", "skills", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
			filepath.Join(base, "claude-code", "opsctl", "commands", "init.md"):                           a.skillContent.InitMD,
		}},
		// Codex
		{files: map[string]string{
			filepath.Join(base, "codex", "opsctl", "SKILL.md"):                  skillMD,
			filepath.Join(base, "codex", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
			filepath.Join(base, "codex", "opsctl", "references", "init.md"):     a.skillContent.InitMD,
		}},
		// OpenCode
		{files: map[string]string{
			filepath.Join(base, "opencode", "opsctl", "SKILL.md"):                  skillMD,
			filepath.Join(base, "opencode", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
			filepath.Join(base, "opencode", "opsctl", "references", "init.md"):     a.skillContent.InitMD,
		}},
		// Gemini CLI
		{files: map[string]string{
			filepath.Join(base, "gemini", "opsctl", "gemini-extension.json"):                         `{"name":"opsctl","version":"` + pluginVersion + `"}` + "\n",
			filepath.Join(base, "gemini", "opsctl", "GEMINI.md"):                                     "See the opsctl skill in skills/opsctl/ for asset management instructions.\n",
			filepath.Join(base, "gemini", "opsctl", "skills", "opsctl", "SKILL.md"):                  skillMD,
			filepath.Join(base, "gemini", "opsctl", "skills", "opsctl", "references", "commands.md"): a.skillContent.CommandsMD,
			filepath.Join(base, "gemini", "opsctl", "skills", "opsctl", "references", "init.md"):     a.skillContent.InitMD,
		}},
	}

	for _, s := range structures {
		for p, content := range s.files {
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(p, []byte(content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetSkillPreview 获取 Skill 文件内容预览
func (a *App) GetSkillPreview() string {
	return "--- skills/opsctl/SKILL.md ---\n\n" + a.skillMDWithDataDir() +
		"\n\n--- commands/init.md ---\n\n" + a.skillContent.InitMD +
		"\n\n--- skills/opsctl/references/commands.md ---\n\n" + a.skillContent.CommandsMD
}

// --- 审计日志 ---

// AuditLogListResult 审计日志列表结果
type AuditLogListResult struct {
	Items []*audit_entity.AuditLog `json:"items"`
	Total int64                    `json:"total"`
}

// ListAuditLogs 查询审计日志
func (a *App) ListAuditLogs(source string, assetID int64, startTime, endTime int64, offset, limit int, sessionID string) (*AuditLogListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	items, total, err := audit_repo.Audit().List(a.langCtx(), audit_repo.ListOptions{
		Source:    source,
		AssetID:   assetID,
		SessionID: sessionID,
		StartTime: startTime,
		EndTime:   endTime,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return &AuditLogListResult{Items: items, Total: total}, nil
}

// ListAuditSessions 查询审计日志中的会话列表
func (a *App) ListAuditSessions(startTime int64) ([]audit_repo.SessionInfo, error) {
	return audit_repo.Audit().ListSessions(a.langCtx(), startTime)
}

// --- 更新 ---

// startAutoUpdateCheck 启动时自动检查更新（每天一次）
func (a *App) startAutoUpdateCheck() {
	go func() {
		// 延迟 5 秒，等前端就绪
		time.Sleep(5 * time.Second)

		cfg := bootstrap.GetConfig()
		if cfg == nil {
			return
		}
		now := time.Now().Unix()
		if now-cfg.LastUpdateCheck < 86400 {
			return
		}

		info, err := update_svc.CheckForUpdate(a.GetUpdateChannel(), a.GetDownloadMirror())
		if err != nil {
			logger.Default().Warn("auto check update failed", zap.Error(err))
			return
		}

		cfg.LastUpdateCheck = now
		if err := bootstrap.SaveConfig(cfg); err != nil {
			logger.Default().Warn("save last update check time", zap.Error(err))
		}

		if info.HasUpdate {
			wailsRuntime.EventsEmit(a.ctx, "update:available", info)
		}
	}()
}

// GetAppVersion 返回当前应用版本
func (a *App) GetAppVersion() string {
	v := configs.Version
	if c := buildinfo.ShortCommitID(); c != "" {
		v += " (" + c + ")"
	}
	return v
}

// BugReportInfo 用于前端拼接 GitHub Issue 预填 URL 的诊断信息。
type BugReportInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	OSLabel string `json:"osLabel"`
}

// GetDebugMode 返回当前是否开启 debug 日志
func (a *App) GetDebugMode() bool {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return false
	}
	return cfg.DebugMode
}

// SetDebugMode 开启/关闭 debug 日志，写入配置并重建全局 logger
func (a *App) SetDebugMode(enabled bool) error {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	cfg.DebugMode = enabled
	if err := bootstrap.SaveConfig(cfg); err != nil {
		return err
	}
	return bootstrap.InitLogger()
}

// OpenLogsDir 在系统文件管理器中打开日志目录
func (a *App) OpenLogsDir() error {
	dir := bootstrap.GetLogsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return a.OpenDirectory(dir)
}

// GetBugReportInfo 返回用于 Bug 反馈模板预填的系统信息。
func (a *App) GetBugReportInfo() BugReportInfo {
	osVer := detectOSVersion()
	archSuffix := runtime.GOARCH
	osLabel := ""
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			archSuffix = "Apple Silicon"
		} else {
			archSuffix = "Intel"
		}
		if osVer != "" {
			osLabel = fmt.Sprintf("macOS %s (%s)", osVer, archSuffix)
		} else {
			osLabel = fmt.Sprintf("macOS (%s)", archSuffix)
		}
	case "windows":
		if osVer != "" {
			osLabel = fmt.Sprintf("Windows %s (%s)", osVer, archSuffix)
		} else {
			osLabel = fmt.Sprintf("Windows (%s)", archSuffix)
		}
	case "linux":
		if osVer != "" {
			osLabel = fmt.Sprintf("%s (%s)", osVer, archSuffix)
		} else {
			osLabel = fmt.Sprintf("Linux (%s)", archSuffix)
		}
	default:
		osLabel = fmt.Sprintf("%s (%s)", runtime.GOOS, archSuffix)
	}
	return BugReportInfo{
		Version: configs.Version,
		Commit:  buildinfo.ShortCommitID(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		OSLabel: osLabel,
	}
}

// detectOSVersion 尝试获取当前操作系统版本号/发行版名称，失败返回空串。
func detectOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	case "linux":
		data, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return ""
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			if name, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				return strings.Trim(strings.TrimSpace(name), `"`)
			}
		}
		return ""
	case "windows":
		out, err := exec.Command("cmd", "/c", "ver").Output()
		if err != nil {
			return ""
		}
		s := strings.TrimSpace(string(out))
		if i := strings.Index(s, "[Version "); i >= 0 {
			if j := strings.Index(s[i:], "]"); j > 0 {
				return strings.TrimPrefix(s[i:i+j], "[Version ")
			}
		}
		return ""
	}
	return ""
}

// GetUpdateChannel 获取当前更新通道
func (a *App) GetUpdateChannel() string {
	cfg := bootstrap.GetConfig()
	if cfg == nil || cfg.UpdateChannel == "" {
		return update_svc.ChannelStable
	}
	return cfg.UpdateChannel
}

// SetUpdateChannel 设置更新通道
func (a *App) SetUpdateChannel(channel string) error {
	cfg := bootstrap.GetConfig()
	cfg.UpdateChannel = channel
	return bootstrap.SaveConfig(cfg)
}

// GetDownloadMirror 获取当前下载镜像 URL 前缀
func (a *App) GetDownloadMirror() string {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return ""
	}
	return cfg.DownloadMirror
}

// SetDownloadMirror 设置下载镜像
// mirror 为镜像 URL 前缀（如 "https://ghfast.top/"），空字符串表示直连 GitHub
func (a *App) SetDownloadMirror(mirror string) error {
	cfg := bootstrap.GetConfig()
	cfg.DownloadMirror = mirror
	return bootstrap.SaveConfig(cfg)
}

// GetAvailableMirrors 返回可用的下载镜像列表
func (a *App) GetAvailableMirrors() []update_svc.MirrorInfo {
	return update_svc.GetAvailableMirrors()
}

// CheckForUpdate 检查是否有新版本
func (a *App) CheckForUpdate() (*update_svc.UpdateInfo, error) {
	return update_svc.CheckForUpdate(a.GetUpdateChannel(), a.GetDownloadMirror())
}

// DownloadAndInstallUpdate 下载并安装更新
// 更新完成后需要用户重启应用
func (a *App) DownloadAndInstallUpdate(skipChecksum bool) error {
	err := update_svc.DownloadAndUpdate(a.GetUpdateChannel(), a.GetDownloadMirror(), skipChecksum, func(downloaded, total int64) {
		wailsRuntime.EventsEmit(a.ctx, "update:progress", map[string]int64{
			"downloaded": downloaded,
			"total":      total,
		})
	})
	if err != nil {
		return err
	}

	// 更新后重新安装 opsctl（如果已安装）
	opsctlInfo := a.DetectOpsctl()
	if opsctlInfo.Installed && embedded.HasEmbeddedOpsctl() {
		installDir := filepath.Dir(opsctlInfo.Path)
		if _, err := embedded.InstallOpsctl(installDir); err != nil {
			// opsctl 更新失败不阻塞主更新
			wailsRuntime.EventsEmit(a.ctx, "update:opsctl-error", err.Error())
		}
	}

	// 更新后重新安装 Skills/Plugin（如果已安装）
	home, _ := os.UserHomeDir()
	skills := a.DetectSkills()
	for i, s := range skills {
		if !s.Installed {
			continue
		}
		if installErr := a.installTarget(skillTargetDefs[i], home); installErr != nil {
			wailsRuntime.EventsEmit(a.ctx, "update:skill-error", installErr.Error())
		}
	}

	return nil
}
