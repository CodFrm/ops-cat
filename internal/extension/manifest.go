package extension

import "github.com/opskat/opskat/pkg/extruntime"

// Type aliases — all manifest types live in pkg/extruntime now.
type (
	Manifest             = extruntime.Manifest
	BackendConfig        = extruntime.BackendConfig
	AssetTypeDef         = extruntime.AssetTypeDef
	ToolDef              = extruntime.ToolDef
	PolicyDef            = extruntime.PolicyDef
	PolicyGroupDef       = extruntime.PolicyGroupDef
	ExtensionPolicyRules = extruntime.ExtensionPolicyRules
	FrontendConfig       = extruntime.FrontendConfig
	PageDef              = extruntime.PageDef
)

// Function re-exports
var (
	ParseManifest               = extruntime.ParseManifest
	CheckAppVersionCompatibility = extruntime.CheckAppVersionCompatibility
)
