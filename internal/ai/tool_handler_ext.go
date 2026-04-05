package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opskat/opskat/pkg/extension"
)

// ExtensionToolExecutor provides extension tool execution to the AI system.
type ExtensionToolExecutor interface {
	FindExtensionByTool(extName, toolName string) *extension.Extension
	GetExtensionPolicyGroups(extName, assetType string, assetID int64) []string
}

var execToolExecutor ExtensionToolExecutor

// SetExecToolExecutor wires the extension executor into the exec_tool handler.
func SetExecToolExecutor(executor ExtensionToolExecutor) {
	execToolExecutor = executor
}

func handleExecTool(ctx context.Context, args map[string]any) (string, error) {
	extName := argString(args, "extension")
	if extName == "" {
		return "", fmt.Errorf("exec_tool: extension name is required")
	}
	toolName := argString(args, "tool")
	if toolName == "" {
		return "", fmt.Errorf("exec_tool: tool name is required")
	}

	if execToolExecutor == nil {
		return "", fmt.Errorf("exec_tool: extension %q not found (no extensions loaded)", extName)
	}

	ext := execToolExecutor.FindExtensionByTool(extName, toolName)
	if ext == nil {
		return "", fmt.Errorf("exec_tool: tool %q not found in extension %q", toolName, extName)
	}

	toolArgs, _ := args["args"].(map[string]any)
	argsJSON, err := json.Marshal(toolArgs)
	if err != nil {
		return "", fmt.Errorf("exec_tool: marshal args: %w", err)
	}

	assetID := argInt64(args, "asset_id")
	if ext.Manifest.Policies.Type != "" {
		if assetID <= 0 {
			return "", fmt.Errorf("exec_tool: %s.%s requires asset_id (extension declares policy type %q)",
				extName, toolName, ext.Manifest.Policies.Type)
		}
		action, resource, err := ext.Plugin.CheckPolicy(ctx, toolName, argsJSON)
		if err == nil && action != "" {
			policyGroups := execToolExecutor.GetExtensionPolicyGroups(
				extName, ext.Manifest.Policies.Type, assetID,
			)
			result := checkExtensionPolicy(ctx, policyGroups, action, resource)
			switch result.Decision {
			case Deny:
				return "", fmt.Errorf("exec_tool: policy denied: %s", result.Message)
			case NeedConfirm:
				checker := GetPolicyChecker(ctx)
				if checker != nil {
					confirmResult := checker.handleConfirm(ctx, assetID, ext.Manifest.Policies.Type, extName+"."+toolName)
					if confirmResult.Decision != Allow {
						return "", fmt.Errorf("exec_tool: user denied: %s.%s", extName, toolName)
					}
				}
			}
		}
	}

	result, err := ext.Plugin.CallTool(ctx, toolName, argsJSON)
	if err != nil {
		return "", fmt.Errorf("exec_tool: %s.%s failed: %w", extName, toolName, err)
	}

	return string(result), nil
}
