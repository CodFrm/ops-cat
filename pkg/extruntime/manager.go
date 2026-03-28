package extruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Logger is a minimal logging interface for the Manager.
type Logger interface {
	Warn(msg string, keysAndValues ...any)
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithLogger sets the logger for the Manager. When nil, warnings are silently dropped.
func WithLogger(l Logger) ManagerOption {
	return func(m *Manager) { m.logger = l }
}

// WithAppVersion sets the app version for compatibility checking.
// Extensions with minAppVersion higher than this will be skipped.
func WithAppVersion(v string) ManagerOption {
	return func(m *Manager) { m.appVersion = v }
}

// Manager 扩展管理器，负责发现、加载、安装/卸载扩展
type Manager struct {
	extensionsDir string
	appVersion    string
	logger        Logger
	mu            sync.RWMutex
	extensions    []*ExtensionInfo
}

// NewManager 创建扩展管理器
func NewManager(extensionsDir string, opts ...ManagerOption) *Manager {
	m := &Manager{extensionsDir: extensionsDir}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Scan 扫描 extensions 目录，加载所有合法扩展的 manifest
func (m *Manager) Scan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
			if m.logger != nil {
				m.logger.Warn("skip extension", "dir", extDir, "error", loadErr)
			}
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
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}

	if err := copyFile(manifestPath, filepath.Join(targetDir, "manifest.json")); err != nil {
		return nil, fmt.Errorf("copy manifest: %w", err)
	}

	if manifest.PromptFile != "" {
		promptSrc := filepath.Clean(filepath.Join(sourceDir, manifest.PromptFile))
		if _, statErr := os.Stat(promptSrc); statErr == nil {
			if err := copyFile(promptSrc, filepath.Join(targetDir, manifest.PromptFile)); err != nil {
				return nil, fmt.Errorf("copy prompt file: %w", err)
			}
		}
	}

	distSrc := filepath.Join(sourceDir, "dist")
	if _, statErr := os.Stat(distSrc); statErr == nil {
		if err := copyDir(distSrc, filepath.Join(targetDir, "dist")); err != nil {
			return nil, fmt.Errorf("copy dist: %w", err)
		}
	}

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
	if m.appVersion != "" && !CheckAppVersionCompatibility(m.appVersion, manifest.MinAppVersion) {
		return nil, fmt.Errorf("requires app version >= %s (current: %s)", manifest.MinAppVersion, m.appVersion)
	}
	return &ExtensionInfo{Manifest: manifest, Dir: dir}, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(filepath.Clean(src))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644) //nolint:gosec
}

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
		data, readErr := os.ReadFile(path) //nolint:gosec
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, info.Mode()) //nolint:gosec
	})
}
