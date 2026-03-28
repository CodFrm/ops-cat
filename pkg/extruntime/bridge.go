package extruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ExecutionPolicy is an optional hook for policy enforcement before tool execution.
// When nil on the Bridge, tools execute without policy checks.
type ExecutionPolicy interface {
	// CheckToolExecution checks if a tool call should proceed.
	// Returns (proceed, denyMessage).
	CheckToolExecution(ctx context.Context, extName, toolName string,
		argsJSON json.RawMessage, policyResult *PolicyCheckResult) (proceed bool, denyMessage string)
}

// BridgeOption configures a Bridge.
type BridgeOption func(*Bridge)

// WithExecutionPolicy sets the policy enforcement hook.
func WithExecutionPolicy(p ExecutionPolicy) BridgeOption {
	return func(b *Bridge) { b.policy = p }
}

// WithBridgeLogger sets the logger for the Bridge.
func WithBridgeLogger(l Logger) BridgeOption {
	return func(b *Bridge) { b.logger = l }
}

// Bridge routes tool calls to extension WASM plugins with optional policy enforcement.
type Bridge struct {
	host       *ExtensionHost
	extensions []*ExtensionInfo
	policy     ExecutionPolicy
	logger     Logger
}

// NewBridge creates a Bridge.
func NewBridge(host *ExtensionHost, opts ...BridgeOption) *Bridge {
	b := &Bridge{host: host}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// RegisterExtension registers an extension to the Bridge.
func (b *Bridge) RegisterExtension(ext *ExtensionInfo) {
	b.extensions = append(b.extensions, ext)
}

// ExtensionNames returns all registered extension names.
func (b *Bridge) ExtensionNames() []string {
	names := make([]string, 0, len(b.extensions))
	for _, ext := range b.extensions {
		names = append(names, ext.Manifest.Name)
	}
	return names
}

// GetExtension returns the extension info by name, or nil.
func (b *Bridge) GetExtension(name string) *ExtensionInfo {
	for _, ext := range b.extensions {
		if ext.Manifest.Name == name {
			return ext
		}
	}
	return nil
}

// ExecuteTool executes a tool with the full pipeline:
// validate → check_policy(WASM) → ExecutionPolicy hook → execute_tool(WASM)
func (b *Bridge) ExecuteTool(ctx context.Context, extName, toolName string, argsJSON json.RawMessage) (string, error) {
	ext := b.GetExtension(extName)
	if ext == nil {
		return "", fmt.Errorf("extension %q not registered", extName)
	}

	if !ext.HasTool(toolName) {
		return "", fmt.Errorf("tool %q not found in extension %q", toolName, extName)
	}

	if b.host == nil {
		return "", fmt.Errorf("extension host not initialized")
	}
	plugin := b.host.GetPlugin(extName)
	if plugin == nil {
		return "", fmt.Errorf("extension %q plugin not loaded", extName)
	}

	// Call check_policy on WASM
	policyResult, err := plugin.CallCheckPolicy(ctx, toolName, argsJSON)
	if err != nil {
		slog.Warn("extension check_policy failed",
			"extension", extName, "tool", toolName, "error", err)
	}

	// Run ExecutionPolicy hook if configured
	if b.policy != nil && policyResult != nil && policyResult.Action != "" {
		proceed, denyMsg := b.policy.CheckToolExecution(ctx, extName, toolName, argsJSON, policyResult)
		if !proceed {
			return denyMsg, nil
		}
	}

	// Execute the tool
	result, err := plugin.CallTool(ctx, toolName, argsJSON)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// GetExtensionPrompts collects prompt text from all registered extensions.
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

func (b *Bridge) loadExtensionPrompt(ext *ExtensionInfo) string {
	if ext.Manifest.PromptFile != "" {
		promptPath := filepath.Clean(filepath.Join(ext.Dir, ext.Manifest.PromptFile))
		data, err := os.ReadFile(promptPath)
		if err != nil {
			if b.logger != nil {
				b.logger.Warn("read extension prompt file",
					"extension", ext.Manifest.Name, "path", promptPath, "error", err)
			}
		} else if len(data) > 0 {
			return fmt.Sprintf("### Extension: %s\n%s", ext.Manifest.Name, strings.TrimSpace(string(data)))
		}
	}

	return b.autoGeneratePrompt(ext)
}

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

// ParseToolName parses a qualified tool name "ext.tool" into (extName, toolName, ok).
func ParseToolName(qualifiedName string) (string, string, bool) {
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
