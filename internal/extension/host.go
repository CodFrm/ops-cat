package extension

import "github.com/opskat/opskat/pkg/extruntime"

// Type aliases — runtime types live in pkg/extruntime now.
type (
	HostServices      = extruntime.HostServices
	ExtensionHost     = extruntime.ExtensionHost
	LoadedPlugin      = extruntime.LoadedPlugin
	PolicyCheckResult = extruntime.PolicyCheckResult
	ExtensionInfo     = extruntime.ExtensionInfo
)

// Function re-exports
var NewExtensionHost = extruntime.NewExtensionHost
