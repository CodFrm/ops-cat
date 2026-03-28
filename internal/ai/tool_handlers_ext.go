package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExtBridge 扩展系统桥接接口，由 extension.Bridge 实现
type ExtBridge interface {
	ExecuteTool(ctx context.Context, extName, toolName string, argsJSON json.RawMessage) (string, error)
	GetExtensionPrompts() string
}

type extBridgeKeyType struct{}

// WithExtBridge 将 ExtBridge 注入 context
func WithExtBridge(ctx context.Context, bridge ExtBridge) context.Context {
	return context.WithValue(ctx, extBridgeKeyType{}, bridge)
}

// GetExtBridge 从 context 中获取 ExtBridge
func GetExtBridge(ctx context.Context) ExtBridge {
	if v := ctx.Value(extBridgeKeyType{}); v != nil {
		return v.(ExtBridge)
	}
	return nil
}

func handleExtExec(ctx context.Context, args map[string]any) (string, error) {
	extName := argString(args, "extension")
	toolName := argString(args, "tool")
	argsJSONStr := argString(args, "args")

	if extName == "" {
		return "", fmt.Errorf("missing required parameter: extension")
	}
	if toolName == "" {
		return "", fmt.Errorf("missing required parameter: tool")
	}

	bridge := GetExtBridge(ctx)
	if bridge == nil {
		return "", fmt.Errorf("extension system not available")
	}

	var argsJSON json.RawMessage
	if argsJSONStr != "" {
		argsJSON = json.RawMessage(argsJSONStr)
	} else {
		argsJSON = json.RawMessage("{}")
	}

	return bridge.ExecuteTool(ctx, extName, toolName, argsJSON)
}
