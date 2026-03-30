package extension

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

// Extension represents a loaded extension.
type Extension struct {
	Name     string
	Dir      string
	Manifest *Manifest
	Plugin   *Plugin
	SkillMD  string // Contents of SKILL.md
}

// Manager handles extension discovery, loading, and lifecycle.
type Manager struct {
	dir        string
	newHost    func(extName string) HostProvider
	logger     *zap.Logger
	mu         sync.RWMutex
	extensions map[string]*Extension
}

func NewManager(dir string, newHost func(extName string) HostProvider, logger *zap.Logger) *Manager {
	return &Manager{
		dir:        dir,
		newHost:    newHost,
		logger:     logger,
		extensions: make(map[string]*Extension),
	}
}

// Scan discovers and loads extensions from the extensions directory.
func (m *Manager) Scan(ctx context.Context) ([]*Manifest, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read extensions dir: %w", err)
	}

	var manifests []*Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extDir := filepath.Join(m.dir, entry.Name())
		manifest, err := m.loadExtension(ctx, extDir)
		if err != nil {
			m.logger.Warn("skip extension", zap.String("dir", entry.Name()), zap.Error(err))
			continue
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func (m *Manager) GetExtension(name string) *Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.extensions[name]
}

func (m *Manager) ListExtensions() []*Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exts := make([]*Extension, 0, len(m.extensions))
	for _, ext := range m.extensions {
		exts = append(exts, ext)
	}
	return exts
}

func (m *Manager) Unload(ctx context.Context, name string) error {
	m.mu.Lock()
	ext, ok := m.extensions[name]
	if ok {
		delete(m.extensions, name)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("extension %q not loaded", name)
	}
	if ext.Plugin != nil {
		return ext.Plugin.Close(ctx)
	}
	return nil
}

func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	exts := m.extensions
	m.extensions = make(map[string]*Extension)
	m.mu.Unlock()
	for _, ext := range exts {
		if ext.Plugin != nil {
			ext.Plugin.Close(ctx)
		}
	}
}

func (m *Manager) loadExtension(ctx context.Context, dir string) (*Manifest, error) {
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	wasmPath := filepath.Join(dir, manifest.Backend.Binary)
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read wasm binary: %w", err)
	}

	skillMD := ""
	if skillData, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		skillMD = string(skillData)
	}

	host := m.newHost(manifest.Name)
	plugin, err := LoadPlugin(ctx, manifest, wasmBytes, host)
	if err != nil {
		host.CloseAll()
		return nil, fmt.Errorf("load plugin: %w", err)
	}

	ext := &Extension{
		Name:     manifest.Name,
		Dir:      dir,
		Manifest: manifest,
		Plugin:   plugin,
		SkillMD:  skillMD,
	}

	m.mu.Lock()
	m.extensions[manifest.Name] = ext
	m.mu.Unlock()

	m.logger.Info("loaded extension", zap.String("name", manifest.Name), zap.String("version", manifest.Version))
	return manifest, nil
}
