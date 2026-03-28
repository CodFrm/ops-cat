package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/ai"
)

// Bridge 将扩展的 tools/policies 注册到主 App 系统
type Bridge struct {
	host       *ExtensionHost
	extensions []*ExtensionInfo
}

// NewBridge 创建 Bridge
func NewBridge(host *ExtensionHost) *Bridge {
	return &Bridge{
		host: host,
	}
}

// RegisterExtension 注册扩展到 Bridge
func (b *Bridge) RegisterExtension(ext *ExtensionInfo) {
	b.extensions = append(b.extensions, ext)
}

// ExtensionNames 返回所有已注册的扩展名
func (b *Bridge) ExtensionNames() []string {
	names := make([]string, 0, len(b.extensions))
	for _, ext := range b.extensions {
		names = append(names, ext.Manifest.Name)
	}
	return names
}

// MergeToolDefs 将扩展工具合并到内置工具列表
// 扩展工具名自动加 "{ext}." 前缀，如 oss.list_buckets
func (b *Bridge) MergeToolDefs(builtinTools []ai.ToolDef) []ai.ToolDef {
	all := append([]ai.ToolDef{}, builtinTools...)
	for _, ext := range b.extensions {
		for _, tool := range ext.Manifest.Tools {
			qualifiedName := ext.Manifest.Name + "." + tool.Name
			all = append(all, ai.ToolDef{
				Name:        qualifiedName,
				Description: tool.Description,
				Params:      convertJSONSchemaToParams(tool.Parameters),
				Handler:     b.makeWASMHandler(ext.Manifest.Name, tool.Name),
				CommandExtractor: func(args map[string]any) string {
					return qualifiedName
				},
			})
		}
	}
	return all
}

// ParseExtensionToolName 解析带命名空间的工具名，返回 (extName, toolName, ok)
func ParseExtensionToolName(qualifiedName string) (string, string, bool) {
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// makeWASMHandler 创建将工具调用路由到 WASM 的 handler
func (b *Bridge) makeWASMHandler(extName, toolName string) ai.ToolHandlerFunc {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if b.host == nil {
			return "", fmt.Errorf("extension host not initialized")
		}
		plugin := b.host.GetPlugin(extName)
		if plugin == nil {
			return "", fmt.Errorf("extension %q not loaded", extName)
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return "", fmt.Errorf("marshal args: %w", err)
		}
		result, err := plugin.CallTool(ctx, toolName, argsJSON)
		if err != nil {
			return "", err
		}
		return string(result), nil
	}
}

// convertJSONSchemaToParams 将 JSON Schema 转换为 ai.ParamDef 列表
func convertJSONSchemaToParams(schema json.RawMessage) []ai.ParamDef {
	if schema == nil {
		return nil
	}
	var s struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil || s.Properties == nil {
		return nil
	}

	requiredSet := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	var params []ai.ParamDef
	for name, prop := range s.Properties {
		paramType := ai.ParamString
		if prop.Type == "number" || prop.Type == "integer" {
			paramType = ai.ParamNumber
		}
		params = append(params, ai.ParamDef{
			Name:        name,
			Type:        paramType,
			Description: prop.Description,
			Required:    requiredSet[name],
		})
	}
	return params
}
