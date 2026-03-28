package extension

import "github.com/opskat/opskat/pkg/extruntime"

// Type aliases for backward compatibility.
type (
	ExtDecision     = extruntime.ExtDecision
	ExtPolicyResult = extruntime.ExtPolicyResult
	ExtensionPolicy = extruntime.ExtensionPolicy
)

// Constant re-exports.
const (
	ExtAllow       = extruntime.ExtAllow
	ExtDeny        = extruntime.ExtDeny
	ExtNeedConfirm = extruntime.ExtNeedConfirm
)

// Function re-export.
var CheckExtensionPolicy = extruntime.CheckExtensionPolicy
