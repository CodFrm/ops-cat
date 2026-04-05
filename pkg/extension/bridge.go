package extension

import (
	"encoding/json"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
)

// ExtAssetType represents an extension-provided asset type.
type ExtAssetType struct {
	Type          string
	ExtensionName string
	ConfigSchema  map[string]any
	I18n          I18nName
}

// ExtPolicyGroup represents an extension-provided policy group.
type ExtPolicyGroup struct {
	ID            string
	ExtensionName string
	PolicyType    string
	I18n          I18nNameDesc
	Policy        map[string]any
}

// SkillMDWithExtension pairs SKILL.md content with its originating extension name.
type SkillMDWithExtension struct {
	ExtensionName string
	Content       string
}

// Bridge connects loaded extensions to the main app's tool, policy, and frontend systems.
type Bridge struct {
	mu              sync.RWMutex
	extensions      map[string]*Extension
	assetTypes      []ExtAssetType
	policyGroups    []ExtPolicyGroup
	defaultPolicies map[string][]string                  // asset type → default policy group IDs
	skillMDs        map[string]SkillMDWithExtension       // asset type → SKILL.md content + ext name
	toolIndex       map[string]map[string]*Extension      // extName → toolName → Extension
}

func NewBridge() *Bridge {
	return &Bridge{
		extensions:      make(map[string]*Extension),
		defaultPolicies: make(map[string][]string),
		skillMDs:        make(map[string]SkillMDWithExtension),
		toolIndex:       make(map[string]map[string]*Extension),
	}
}

func (b *Bridge) Register(ext *Extension) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m := ext.Manifest
	b.extensions[ext.Name] = ext

	for _, at := range m.AssetTypes {
		b.assetTypes = append(b.assetTypes, ExtAssetType{
			Type:          at.Type,
			ExtensionName: ext.Name,
			ConfigSchema:  at.ConfigSchema,
			I18n:          at.I18n,
		})
		if ext.SkillMD != "" {
			if existing, ok := b.skillMDs[at.Type]; ok && existing.ExtensionName != ext.Name {
				// Collision: another extension already registered SKILL.md for this asset type.
				// Prefer the first registration; silently skip the duplicate.
				continue
			}
			b.skillMDs[at.Type] = SkillMDWithExtension{
				ExtensionName: ext.Name,
				Content:       ext.SkillMD,
			}
		}
		if len(m.Policies.Default) > 0 {
			b.defaultPolicies[at.Type] = m.Policies.Default
			groups := m.Policies.Default // capture for closure
			policy.RegisterDefaultPolicy(at.Type, func() any {
				return &policy.CommandPolicy{Groups: groups}
			})
		}
	}

	for _, pg := range m.Policies.Groups {
		b.policyGroups = append(b.policyGroups, ExtPolicyGroup{
			ID:            pg.ID,
			ExtensionName: ext.Name,
			PolicyType:    m.Policies.Type,
			I18n:          pg.I18n,
			Policy:        pg.Policy,
		})
		// 注册到全局 entity，使 ListPolicyGroups 可以返回扩展权限组
		policyJSON, _ := json.Marshal(pg.Policy)
		policy_group_entity.RegisterExtensionGroup(&policy_group_entity.PolicyGroup{
			BuiltinID:     pg.ID,
			Name:          pg.I18n.Name,
			Description:   pg.I18n.Description,
			PolicyType:    m.Policies.Type,
			Policy:        string(policyJSON),
			ExtensionName: ext.Name,
		})
	}

	b.toolIndex[ext.Name] = make(map[string]*Extension)
	for _, tool := range m.Tools {
		b.toolIndex[ext.Name][tool.Name] = ext
	}
}

func (b *Bridge) Unregister(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.extensions, name)

	filtered := b.assetTypes[:0]
	for _, at := range b.assetTypes {
		if at.ExtensionName != name {
			filtered = append(filtered, at)
		}
	}
	b.assetTypes = filtered

	filteredPG := b.policyGroups[:0]
	for _, pg := range b.policyGroups {
		if pg.ExtensionName != name {
			filteredPG = append(filteredPG, pg)
		}
	}
	b.policyGroups = filteredPG

	// 清理全局 entity 中的扩展权限组
	policy_group_entity.UnregisterExtensionGroupsByExtension(name)

	delete(b.toolIndex, name)

	for key, entry := range b.skillMDs {
		if entry.ExtensionName == name {
			delete(b.skillMDs, key)
			delete(b.defaultPolicies, key)
			policy.UnregisterDefaultPolicy(key)
		}
	}
}

func (b *Bridge) ListNames() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	names := make([]string, 0, len(b.extensions))
	for name := range b.extensions {
		names = append(names, name)
	}
	return names
}

func (b *Bridge) GetAssetTypes() []ExtAssetType {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]ExtAssetType, len(b.assetTypes))
	copy(result, b.assetTypes)
	return result
}

func (b *Bridge) GetPolicyGroups() []ExtPolicyGroup {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]ExtPolicyGroup, len(b.policyGroups))
	copy(result, b.policyGroups)
	return result
}

func (b *Bridge) GetDefaultPolicyGroups(assetType string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.defaultPolicies[assetType]
}

func (b *Bridge) GetSkillMD(assetType string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.skillMDs[assetType].Content
}

// GetSkillMDWithExtension returns the SKILL.md content and extension name for an asset type.
// Returns empty struct if no SKILL.md is registered for the type.
func (b *Bridge) GetSkillMDWithExtension(assetType string) SkillMDWithExtension {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entry, ok := b.skillMDs[assetType]
	if !ok {
		return SkillMDWithExtension{}
	}
	return entry
}

func (b *Bridge) GetExtensionPolicyGroups(extName, assetType string, assetID int64) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if groups, ok := b.defaultPolicies[assetType]; ok {
		return groups
	}
	return nil
}

func (b *Bridge) FindExtensionByTool(extName, toolName string) *Extension {
	b.mu.RLock()
	defer b.mu.RUnlock()
	tools, ok := b.toolIndex[extName]
	if !ok {
		return nil
	}
	return tools[toolName]
}

// GetExtensionByAssetType returns the Extension that registered the given asset type,
// or nil if no extension owns that type.
func (b *Bridge) GetExtensionByAssetType(assetType string) *Extension {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, at := range b.assetTypes {
		if at.Type == assetType {
			return b.extensions[at.ExtensionName]
		}
	}
	return nil
}
