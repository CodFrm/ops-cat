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
	Type         string         `json:"type"`
	I18n         I18nName       `json:"i18n"`
	ConfigSchema map[string]any `json:"configSchema"`
}

type I18nName struct {
	Name string `json:"name"`
}

type I18nNameDesc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolDef struct {
	Name       string         `json:"name"`
	I18n       I18nDesc       `json:"i18n"`
	Parameters map[string]any `json:"parameters"`
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
	ID     string         `json:"id"`
	I18n   I18nNameDesc   `json:"i18n"`
	Policy map[string]any `json:"policy"`
}

type FrontendDef struct {
	Entry  string    `json:"entry"`
	Styles string    `json:"styles"`
	Pages  []PageDef `json:"pages"`
}

type PageDef struct {
	ID        string   `json:"id"`
	Slot      string   `json:"slot,omitempty"`
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

// Localized returns a shallow copy of the manifest with all i18n string fields
// resolved using the provided translate function.
func (m *Manifest) Localized(tr func(key string) string) *Manifest {
	out := *m
	out.I18n = ManifestI18n{
		DisplayName: tr(m.I18n.DisplayName),
		Description: tr(m.I18n.Description),
	}

	if len(m.AssetTypes) > 0 {
		out.AssetTypes = make([]AssetTypeDef, len(m.AssetTypes))
		for i, at := range m.AssetTypes {
			at.I18n = I18nName{Name: tr(at.I18n.Name)}
			at.ConfigSchema = localizeConfigSchema(at.ConfigSchema, tr)
			out.AssetTypes[i] = at
		}
	}

	if len(m.Tools) > 0 {
		out.Tools = make([]ToolDef, len(m.Tools))
		for i, t := range m.Tools {
			t.I18n = I18nDesc{Description: tr(t.I18n.Description)}
			out.Tools[i] = t
		}
	}

	if len(m.Policies.Groups) > 0 {
		out.Policies.Groups = make([]PolicyGroupDef, len(m.Policies.Groups))
		for i, pg := range m.Policies.Groups {
			pg.I18n = I18nNameDesc{
				Name:        tr(pg.I18n.Name),
				Description: tr(pg.I18n.Description),
			}
			out.Policies.Groups[i] = pg
		}
	}

	if len(m.Frontend.Pages) > 0 {
		out.Frontend.Pages = make([]PageDef, len(m.Frontend.Pages))
		for i, p := range m.Frontend.Pages {
			p.I18n = I18nName{Name: tr(p.I18n.Name)}
			out.Frontend.Pages[i] = p
		}
	}

	return &out
}

// localizeConfigSchema translates title, placeholder, description fields in a JSON Schema.
func localizeConfigSchema(schema map[string]any, tr func(string) string) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		out[k] = v
	}
	// Translate top-level title/placeholder/description
	for _, field := range []string{"title", "placeholder", "description"} {
		if s, ok := out[field].(string); ok && s != "" {
			out[field] = tr(s)
		}
	}
	// Recurse into properties
	if props, ok := out["properties"].(map[string]any); ok {
		newProps := make(map[string]any, len(props))
		for name, propVal := range props {
			if propMap, ok := propVal.(map[string]any); ok {
				newProps[name] = localizeConfigSchema(propMap, tr)
			} else {
				newProps[name] = propVal
			}
		}
		out["properties"] = newProps
	}
	return out
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
