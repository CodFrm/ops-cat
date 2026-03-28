package extruntime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Manifest 扩展清单
// i18n: English is the default (no suffix), other languages use suffix (e.g. _zh)
type Manifest struct {
	Name          string         `json:"name"`
	DisplayName   string         `json:"displayName"`
	DisplayNameZh string         `json:"displayName_zh"`
	Version       string         `json:"version"`
	Icon          string         `json:"icon"`
	Description   string         `json:"description"`
	DescriptionZh string         `json:"description_zh"`
	MinAppVersion string         `json:"minAppVersion"`
	Backend       BackendConfig  `json:"backend"`
	AssetTypes    []AssetTypeDef `json:"assetTypes"`
	Tools         []ToolDef      `json:"tools"`
	Policies      PolicyDef      `json:"policies"`
	Frontend      FrontendConfig `json:"frontend"`
	PromptFile    string         `json:"prompt_file"` // relative path to prompt file, e.g. "prompt.md"
}

// BackendConfig 后端配置
type BackendConfig struct {
	Runtime string `json:"runtime"` // "wasm"
	Binary  string `json:"binary"`  // e.g. "main.wasm"
}

// AssetTypeDef 扩展声明的资产类型
type AssetTypeDef struct {
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	NameZh          string          `json:"name_zh"`
	NamePlaceholder string          `json:"namePlaceholder"` // e.g. "oss-1", shown as placeholder when creating asset
	TestConnection  bool            `json:"testConnection"`  // whether extension supports test_connection WASM export
	ConfigSchema    json.RawMessage `json:"configSchema"`
}

// ToolDef 扩展声明的工具
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// PolicyDef 扩展声明的策略类型
type PolicyDef struct {
	Type          string           `json:"type"`
	Actions       []string         `json:"actions"`
	DefaultGroups []PolicyGroupDef `json:"defaultGroups"`
}

// PolicyGroupDef 扩展声明的默认权限组
type PolicyGroupDef struct {
	Slug          string               `json:"slug"`           // e.g. "readonly", 需唯一
	Name          string               `json:"name"`           // 英文名称
	NameZh        string               `json:"name_zh"`        // 中文名称
	Description   string               `json:"description"`    // 英文描述
	DescriptionZh string               `json:"description_zh"` // 中文描述
	Policy        ExtensionPolicyRules `json:"policy"`
}

// ExtensionPolicyRules 扩展策略规则
type ExtensionPolicyRules struct {
	AllowList []string `json:"allow_list,omitempty"`
	DenyList  []string `json:"deny_list,omitempty"`
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
	NameZh    string `json:"name_zh"`
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
	// 校验 defaultGroups slug
	if err := m.validatePolicyGroups(); err != nil {
		return err
	}
	return nil
}

var slugRegexp = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (m *Manifest) validatePolicyGroups() error {
	if len(m.Policies.DefaultGroups) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	for _, g := range m.Policies.DefaultGroups {
		if g.Slug == "" {
			return fmt.Errorf("manifest: policy group slug is required")
		}
		if !slugRegexp.MatchString(g.Slug) {
			return fmt.Errorf("manifest: policy group slug %q must match ^[a-z][a-z0-9_]*$", g.Slug)
		}
		if seen[g.Slug] {
			return fmt.Errorf("manifest: duplicate policy group slug %q", g.Slug)
		}
		seen[g.Slug] = true
		if g.Name == "" {
			return fmt.Errorf("manifest: policy group %q name is required", g.Slug)
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
