package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// ExtensionInfo 已加载的扩展信息
type ExtensionInfo struct {
	Manifest *Manifest
	Dir      string // 扩展目录的绝对路径
}

// HasTool 检查扩展是否声明了指定的工具
func (e *ExtensionInfo) HasTool(name string) bool {
	for _, t := range e.Manifest.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// Manager 扩展管理器，负责发现、加载、安装/卸载扩展
type Manager struct {
	extensionsDir string
	appVersion    string
	mu            sync.RWMutex
	extensions    []*ExtensionInfo
}

// NewManager 创建扩展管理器
func NewManager(extensionsDir, appVersion string) *Manager {
	return &Manager{
		extensionsDir: extensionsDir,
		appVersion:    appVersion,
	}
}

// Scan 扫描 extensions 目录，加载所有合法扩展的 manifest
func (m *Manager) Scan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 目录不存在时创建
	if err := os.MkdirAll(m.extensionsDir, 0o755); err != nil {
		return fmt.Errorf("create extensions dir: %w", err)
	}

	entries, err := os.ReadDir(m.extensionsDir)
	if err != nil {
		return fmt.Errorf("read extensions dir: %w", err)
	}

	var loaded []*ExtensionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extDir := filepath.Join(m.extensionsDir, entry.Name())
		info, loadErr := m.loadExtension(extDir)
		if loadErr != nil {
			logger.Default().Warn("skip extension",
				zap.String("dir", extDir),
				zap.Error(loadErr),
			)
			continue
		}
		loaded = append(loaded, info)
	}
	m.extensions = loaded
	return nil
}

// Extensions 返回已加载的扩展列表
func (m *Manager) Extensions() []*ExtensionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.extensions
}

// GetExtension 按名称查找扩展
func (m *Manager) GetExtension(name string) *ExtensionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ext := range m.extensions {
		if ext.Manifest.Name == name {
			return ext
		}
	}
	return nil
}

// Install 从 sourceDir 安装扩展到 extensions 目录
func (m *Manager) Install(sourceDir string) (*ExtensionInfo, error) {
	manifestPath := filepath.Clean(filepath.Join(sourceDir, "manifest.json"))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	targetDir := filepath.Join(m.extensionsDir, manifest.Name)
	if err := copyDir(sourceDir, targetDir); err != nil {
		return nil, fmt.Errorf("copy extension: %w", err)
	}

	// 重新扫描
	if err := m.Scan(); err != nil {
		return nil, err
	}
	return m.GetExtension(manifest.Name), nil
}

// Remove 卸载扩展
func (m *Manager) Remove(name string) error {
	targetDir := filepath.Join(m.extensionsDir, name)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("extension %q not found", name)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("remove extension: %w", err)
	}
	return m.Scan()
}

func (m *Manager) loadExtension(dir string) (*ExtensionInfo, error) {
	manifestPath := filepath.Clean(filepath.Join(dir, "manifest.json"))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest.json: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}
	if !CheckAppVersionCompatibility(m.appVersion, manifest.MinAppVersion) {
		return nil, fmt.Errorf("requires app version >= %s (current: %s)", manifest.MinAppVersion, m.appVersion)
	}
	return &ExtensionInfo{Manifest: manifest, Dir: dir}, nil
}

// copyDir 递归复制目录
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // filepath.Walk 提供的路径
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, info.Mode()) //nolint:gosec // 目标路径由 filepath.Join 构造
	})
}
