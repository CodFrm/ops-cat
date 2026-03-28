package extension

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Manifest 扩展清单
type Manifest struct {
	Name          string         `json:"name"`
	DisplayName   string         `json:"displayName"`
	DisplayNameEn string         `json:"displayName_en"`
	Version       string         `json:"version"`
	Icon          string         `json:"icon"`
	Description   string         `json:"description"`
	MinAppVersion string         `json:"minAppVersion"`
	Backend       BackendConfig  `json:"backend"`
	AssetTypes    []AssetTypeDef `json:"assetTypes"`
	Tools         []ToolDef      `json:"tools"`
	Policies      PolicyDef      `json:"policies"`
	Frontend      FrontendConfig `json:"frontend"`
}

// BackendConfig 后端配置
type BackendConfig struct {
	Runtime string `json:"runtime"` // "wasm"
	Binary  string `json:"binary"`  // e.g. "main.wasm"
}

// AssetTypeDef 扩展声明的资产类型
type AssetTypeDef struct {
	Type         string          `json:"type"`
	Name         string          `json:"name"`
	NameEn       string          `json:"name_en"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

// ToolDef 扩展声明的工具
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// PolicyDef 扩展声明的策略类型
type PolicyDef struct {
	Type    string   `json:"type"`
	Actions []string `json:"actions"`
}

// FrontendConfig 前端配置
type FrontendConfig struct {
	Entry string    `json:"entry"`
	Pages []PageDef `json:"pages"`
}

// PageDef 前端页面声明
type PageDef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Component string `json:"component"`
}

// ParseManifest 解析并校验 manifest JSON
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest JSON: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate 校验必填字段
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if m.Backend.Runtime == "" {
		return fmt.Errorf("manifest: backend.runtime is required")
	}
	if m.Backend.Binary == "" {
		return fmt.Errorf("manifest: backend.binary is required")
	}
	if _, ok := parseSemanticVersion(m.Version); !ok {
		return fmt.Errorf("manifest: version %q is not a valid semver (expected x.y.z)", m.Version)
	}
	if m.MinAppVersion != "" {
		if _, ok := parseSemanticVersion(m.MinAppVersion); !ok {
			return fmt.Errorf("manifest: minAppVersion %q is not a valid semver (expected x.y.z)", m.MinAppVersion)
		}
	}
	return nil
}

// CheckAppVersionCompatibility 检查当前 App 版本是否 >= minAppVersion
func CheckAppVersionCompatibility(appVersion, minAppVersion string) bool {
	if minAppVersion == "" {
		return true
	}
	appParts, ok := parseSemanticVersion(appVersion)
	if !ok {
		return false
	}
	minParts, ok := parseSemanticVersion(minAppVersion)
	if !ok {
		return false
	}
	for i := range 3 {
		if appParts[i] > minParts[i] {
			return true
		}
		if appParts[i] < minParts[i] {
			return false
		}
	}
	return true // equal
}

func parseSemanticVersion(v string) ([3]int, bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		result[i] = n
	}
	return result, true
}
