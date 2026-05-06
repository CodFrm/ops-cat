package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
)

type k8sHandler struct{}

func init() {
	Register(&k8sHandler{})
	policy.RegisterDefaultPolicy("k8s", func() any { return asset_entity.DefaultK8sPolicy() })
}

func (h *k8sHandler) Type() string     { return asset_entity.AssetTypeK8s }
func (h *k8sHandler) DefaultPort() int { return 0 }

func (h *k8sHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetK8sConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"namespace":     cfg.Namespace,
		"context":       cfg.Context,
		"ssh_tunnel_id": a.SSHTunnelID,
	}
}

func (h *k8sHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	_, err := a.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config failed: %w", err)
	}
	return "", nil
}

func (h *k8sHandler) DefaultPolicy() any { return asset_entity.DefaultK8sPolicy() }

func (h *k8sHandler) ApplyCreateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	cfg := &asset_entity.K8sConfig{
		Namespace:  ArgString(args, "namespace"),
		Context:    ArgString(args, "context"),
		Kubeconfig: ArgString(args, "kubeconfig"),
	}
	return a.SetK8sConfig(cfg)
}

func (h *k8sHandler) ApplyUpdateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetK8sConfig()
	if err != nil || cfg == nil {
		return err
	}
	if v := ArgString(args, "kubeconfig"); v != "" {
		cfg.Kubeconfig = v
	}
	if v := ArgString(args, "namespace"); v != "" {
		cfg.Namespace = v
	}
	if v := ArgString(args, "context"); v != "" {
		cfg.Context = v
	}
	if _, ok := args["ssh_asset_id"]; ok {
		a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	}
	return a.SetK8sConfig(cfg)
}
