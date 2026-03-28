package main

import (
	"context"
	"encoding/json"

	"github.com/opskat/opskat/pkg/extruntime"
)

// DevPolicy implements extruntime.ExecutionPolicy for the dev server.
// In dev mode, NeedConfirm is treated as Allow.
type DevPolicy struct {
	config *DevConfig
}

func NewDevPolicy(config *DevConfig) *DevPolicy {
	return &DevPolicy{config: config}
}

func (p *DevPolicy) CheckToolExecution(_ context.Context, _, _ string,
	_ json.RawMessage, policyResult *extruntime.PolicyCheckResult) (bool, string) {
	if p.config == nil || p.config.Policy == nil {
		return true, "" // no policy configured → allow all
	}

	policyJSON, err := json.Marshal(p.config.Policy)
	if err != nil {
		return true, ""
	}

	result := extruntime.CheckExtensionPolicy(string(policyJSON), policyResult.Action, policyResult.Resource)
	if result.Decision == extruntime.ExtDeny {
		return false, result.Message
	}
	// Allow + NeedConfirm both proceed in dev mode
	return true, ""
}
