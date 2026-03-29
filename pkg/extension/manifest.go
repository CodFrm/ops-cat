package extension

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type Manifest struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Icon          string          `json:"icon"`
	MinAppVersion string          `json:"minAppVersion"`
	I18n          ManifestI18n    `json:"i18n"`
	Backend       ManifestBackend `json:"backend"`
	AssetTypes    []AssetTypeDef  `json:"assetTypes"`
	Tools         []ToolDef       `json:"tools"`
	Policies      PoliciesDef     `json:"policies"`
	Frontend      FrontendDef     `json:"frontend"`
}

type ManifestI18n struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type ManifestBackend struct {
	Runtime string `json:"runtime"`
	Binary  string `json:"binary"`
}

type AssetTypeDef struct {
	Type         string          `json:"type"`
	I18n         I18nName        `json:"i18n"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

type I18nName struct {
	Name string `json:"name"`
}

type I18nNameDesc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolDef struct {
	Name       string          `json:"name"`
	I18n       I18nDesc        `json:"i18n"`
	Parameters json.RawMessage `json:"parameters"`
}

type I18nDesc struct {
	Description string `json:"description"`
}

type PoliciesDef struct {
	Type    string           `json:"type"`
	Actions []string         `json:"actions"`
	Groups  []PolicyGroupDef `json:"groups"`
	Default []string         `json:"default"`
}

type PolicyGroupDef struct {
	ID     string          `json:"id"`
	I18n   I18nNameDesc    `json:"i18n"`
	Policy json.RawMessage `json:"policy"`
}

type FrontendDef struct {
	Entry  string    `json:"entry"`
	Styles string    `json:"styles"`
	Pages  []PageDef `json:"pages"`
}

type PageDef struct {
	ID        string   `json:"id"`
	I18n      I18nName `json:"i18n"`
	Component string   `json:"component"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version must be semver (got %q)", m.Version)
	}
	if m.MinAppVersion != "" && !semverRe.MatchString(m.MinAppVersion) {
		return fmt.Errorf("manifest: minAppVersion must be semver (got %q)", m.MinAppVersion)
	}
	for _, g := range m.Policies.Groups {
		if !strings.HasPrefix(g.ID, "ext:") {
			return fmt.Errorf("manifest: policy group ID must start with ext: (got %q)", g.ID)
		}
	}
	return nil
}
