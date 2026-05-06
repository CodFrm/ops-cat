package assettype

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/credential_svc"
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

// ResolvePassword 返回解密后的 kubeconfig 内容（K8s 没有传统密码，复用 PasswordSource 通道）。
func (h *k8sHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config failed: %w", err)
	}
	return credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
}

func (h *k8sHandler) DefaultPolicy() any { return asset_entity.DefaultK8sPolicy() }

func (h *k8sHandler) ValidateCreateArgs(args map[string]any) error {
	if ArgString(args, "kubeconfig") == "" {
		return fmt.Errorf("missing required parameter: kubeconfig for k8s type")
	}
	return nil
}

func (h *k8sHandler) ApplyCreateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	namespace := ArgString(args, "namespace")
	contextName := ArgString(args, "context")
	if err := validateK8sIdentifier("namespace", namespace); err != nil {
		return err
	}
	if err := validateK8sIdentifier("context", contextName); err != nil {
		return err
	}
	a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	cfg := &asset_entity.K8sConfig{
		Namespace: namespace,
		Context:   contextName,
	}
	if kubeconfig := ArgString(args, "kubeconfig"); kubeconfig != "" {
		encrypted, err := credential_svc.Default().Encrypt(kubeconfig)
		if err != nil {
			return fmt.Errorf("encrypt kubeconfig: %w", err)
		}
		cfg.Kubeconfig = encrypted
	}
	return a.SetK8sConfig(cfg)
}

func (h *k8sHandler) ApplyUpdateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetK8sConfig()
	if err != nil || cfg == nil {
		return err
	}
	if v := ArgString(args, "kubeconfig"); v != "" {
		encrypted, encErr := credential_svc.Default().Encrypt(v)
		if encErr != nil {
			return fmt.Errorf("encrypt kubeconfig: %w", encErr)
		}
		cfg.Kubeconfig = encrypted
	}
	if v := ArgString(args, "namespace"); v != "" {
		if err := validateK8sIdentifier("namespace", v); err != nil {
			return err
		}
		cfg.Namespace = v
	}
	if v := ArgString(args, "context"); v != "" {
		if err := validateK8sIdentifier("context", v); err != nil {
			return err
		}
		cfg.Context = v
	}
	if _, ok := args["ssh_asset_id"]; ok {
		a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	}
	return a.SetK8sConfig(cfg)
}

// validateK8sIdentifier 拒绝含空白或 -- 前缀的 namespace/context，避免它们被原样
// 拼到 kubectl 参数中时被解析成额外的 flag 或 argv 切分点。
func validateK8sIdentifier(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("invalid %s %q: must not start with '-'", field, value)
	}
	for _, r := range value {
		if unicode.IsSpace(r) {
			return fmt.Errorf("invalid %s %q: must not contain whitespace", field, value)
		}
	}
	return nil
}
