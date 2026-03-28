package extension

import (
	"context"
	"encoding/json"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/pkg/extruntime"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// Bridge type alias.
type Bridge = extruntime.Bridge

// ParseExtensionToolName is re-exported for backward compatibility.
var ParseExtensionToolName = extruntime.ParseToolName

// NewBridge creates a Bridge with opskat-specific policy enforcement.
func NewBridge(host *ExtensionHost) *Bridge {
	return extruntime.NewBridge(host,
		extruntime.WithExecutionPolicy(&opskatPolicy{}),
		extruntime.WithBridgeLogger(&cagoLoggerAdapter{}),
	)
}

// opskatPolicy implements extruntime.ExecutionPolicy with opskat AI integration.
type opskatPolicy struct{}

func (p *opskatPolicy) CheckToolExecution(ctx context.Context, extName, toolName string,
	argsJSON json.RawMessage, policyResult *extruntime.PolicyCheckResult) (bool, string) {

	// Extract asset_id from args
	var args struct {
		AssetID int64 `json:"asset_id"`
	}
	if json.Unmarshal(argsJSON, &args) != nil || args.AssetID == 0 {
		// No asset_id → NeedConfirm → run user confirmation flow
		return p.handleNeedConfirm(ctx, extName, toolName)
	}

	// Find extension's asset type for policy lookup
	assetType := extName // fallback
	checkResult := ai.CheckPermission(ctx, assetType, args.AssetID, policyResult.Action)
	ai.SetCheckResult(ctx, checkResult)

	if checkResult.Decision == ai.Deny {
		return false, checkResult.Message
	}

	if checkResult.Decision == ai.NeedConfirm {
		return p.handleNeedConfirm(ctx, extName, toolName)
	}

	return true, ""
}

func (p *opskatPolicy) handleNeedConfirm(ctx context.Context, extName, toolName string) (bool, string) {
	checker := ai.GetPolicyChecker(ctx)
	if checker == nil || checker.ConfirmFunc() == nil {
		return true, "" // no checker available, allow
	}
	qualifiedName := extName + "." + toolName
	confirmResult := checker.CheckForAsset(ctx, 0, "extension", qualifiedName)
	ai.SetCheckResult(ctx, confirmResult)
	if confirmResult.Decision != ai.Allow {
		return false, confirmResult.Message
	}
	return true, ""
}

func init() {
	// Silence unused import warnings for logger — used by cagoLoggerAdapter in manager.go
	_ = logger.Default
	_ = zap.String
}
