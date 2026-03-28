package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opskat/opskat/internal/ai"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// Bridge 将扩展系统桥接到 AI 工具系统
// 通过 ext_exec 内置工具统一路由，不再为每个扩展工具单独注册 AI ToolDef
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

// ExecuteTool 统一执行入口：校验工具存在 → 调用 check_policy 获取 action → 检查策略 → 调用 execute_tool
func (b *Bridge) ExecuteTool(ctx context.Context, extName, toolName string, argsJSON json.RawMessage) (string, error) {
	// 查找扩展
	var ext *ExtensionInfo
	for _, e := range b.extensions {
		if e.Manifest.Name == extName {
			ext = e
			break
		}
	}
	if ext == nil {
		return "", fmt.Errorf("extension %q not registered", extName)
	}

	// 校验工具存在
	if !ext.HasTool(toolName) {
		return "", fmt.Errorf("tool %q not found in extension %q", toolName, extName)
	}

	// 获取 WASM 插件
	if b.host == nil {
		return "", fmt.Errorf("extension host not initialized")
	}
	plugin := b.host.GetPlugin(extName)
	if plugin == nil {
		return "", fmt.Errorf("extension %q plugin not loaded", extName)
	}

	// 调用 check_policy 获取 action 分类
	policyResult, err := plugin.CallCheckPolicy(ctx, toolName, argsJSON)
	if err != nil {
		logger.Default().Warn("extension check_policy failed, treating as NeedConfirm",
			zap.String("extension", extName),
			zap.String("tool", toolName),
			zap.Error(err),
		)
	}

	// 如果 check_policy 返回了 action，检查资产策略
	if policyResult != nil && policyResult.Action != "" {
		checkResult := b.checkExtensionToolPolicy(ctx, extName, argsJSON, policyResult.Action, policyResult.Resource)
		ai.SetCheckResult(ctx, checkResult)
		if checkResult.Decision == ai.Deny {
			return checkResult.Message, nil
		}
		// NeedConfirm 时走用户确认流程
		if checkResult.Decision == ai.NeedConfirm {
			checker := ai.GetPolicyChecker(ctx)
			if checker != nil && checker.ConfirmFunc() != nil {
				qualifiedName := extName + "." + toolName
				confirmResult := checker.CheckForAsset(ctx, 0, "extension", qualifiedName)
				ai.SetCheckResult(ctx, confirmResult)
				if confirmResult.Decision != ai.Allow {
					return confirmResult.Message, nil
				}
			}
		}
	}

	// 调用 execute_tool
	result, err := plugin.CallTool(ctx, toolName, argsJSON)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// checkExtensionToolPolicy 检查扩展工具的策略
func (b *Bridge) checkExtensionToolPolicy(ctx context.Context, extName string, argsJSON json.RawMessage, action, _ string) ai.CheckResult {
	// 从参数中尝试提取 asset_id
	var args struct {
		AssetID int64 `json:"asset_id"`
	}
	if json.Unmarshal(argsJSON, &args) != nil || args.AssetID == 0 {
		// 无 asset_id，无法检查资产策略，返回 NeedConfirm
		return ai.CheckResult{Decision: ai.NeedConfirm}
	}

	// 查找扩展声明的资产类型，使用实际类型检查权限
	assetType := b.findExtensionAssetType(extName)
	result := ai.CheckPermission(ctx, assetType, args.AssetID, action)
	return result
}

// findExtensionAssetType 查找扩展声明的第一个资产类型
func (b *Bridge) findExtensionAssetType(extName string) string {
	for _, ext := range b.extensions {
		if ext.Manifest.Name == extName {
			if len(ext.Manifest.AssetTypes) > 0 {
				return ext.Manifest.AssetTypes[0].Type
			}
			break
		}
	}
	return extName
}

// GetExtensionPrompts 收集所有扩展的 prompt 文本，用于注入 AI 系统消息
func (b *Bridge) GetExtensionPrompts() string {
	if len(b.extensions) == 0 {
		return ""
	}

	var parts []string
	for _, ext := range b.extensions {
		prompt := b.loadExtensionPrompt(ext)
		if prompt != "" {
			parts = append(parts, prompt)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "\n\n--- Extensions ---\n" +
		"The following extensions are available. Use the ext_exec tool to execute extension tools.\n\n" +
		strings.Join(parts, "\n\n")
}

// loadExtensionPrompt 加载单个扩展的 prompt 文本
// 优先从 prompt_file 读取，无 prompt_file 时自动从工具定义生成
func (b *Bridge) loadExtensionPrompt(ext *ExtensionInfo) string {
	// 尝试从 prompt_file 加载
	if ext.Manifest.PromptFile != "" {
		promptPath := filepath.Clean(filepath.Join(ext.Dir, ext.Manifest.PromptFile))
		data, err := os.ReadFile(promptPath)
		if err != nil {
			logger.Default().Warn("read extension prompt file",
				zap.String("extension", ext.Manifest.Name),
				zap.String("path", promptPath),
				zap.Error(err),
			)
		} else if len(data) > 0 {
			return fmt.Sprintf("### Extension: %s\n%s", ext.Manifest.Name, strings.TrimSpace(string(data)))
		}
	}

	// 自动从工具定义生成
	return b.autoGeneratePrompt(ext)
}

// autoGeneratePrompt 从工具定义自动生成 prompt 文本
func (b *Bridge) autoGeneratePrompt(ext *ExtensionInfo) string {
	if len(ext.Manifest.Tools) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "### Extension: %s\n", ext.Manifest.Name)
	if ext.Manifest.Description != "" {
		sb.WriteString(ext.Manifest.Description)
		sb.WriteString("\n")
	}
	sb.WriteString("\nAvailable tools:\n")
	for _, tool := range ext.Manifest.Tools {
		fmt.Fprintf(&sb, "- **%s**: %s\n", tool.Name, tool.Description)
		if tool.Parameters != nil {
			fmt.Fprintf(&sb, "  Parameters: %s\n", string(tool.Parameters))
		}
	}
	return sb.String()
}

// ParseExtensionToolName 解析带命名空间的工具名，返回 (extName, toolName, ok)
func ParseExtensionToolName(qualifiedName string) (string, string, bool) {
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
