package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/credential_svc"
)

type kafkaHandler struct{}

func init() {
	Register(&kafkaHandler{})
	policy.RegisterDefaultPolicy("kafka", func() any { return asset_entity.DefaultKafkaPolicy() })
}

func (h *kafkaHandler) Type() string     { return asset_entity.AssetTypeKafka }
func (h *kafkaHandler) DefaultPort() int { return 9092 }

func (h *kafkaHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetKafkaConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"brokers":        cfg.Brokers,
		"client_id":      cfg.ClientID,
		"sasl_mechanism": cfg.SASLMechanism,
		"username":       cfg.Username,
		"tls":            cfg.TLS,
	}
}

func (h *kafkaHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetKafkaConfig()
	if err != nil {
		return "", fmt.Errorf("get Kafka config failed: %w", err)
	}
	return credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
}

func (h *kafkaHandler) DefaultPolicy() any { return asset_entity.DefaultKafkaPolicy() }

func (h *kafkaHandler) ValidateCreateArgs(args map[string]any) error {
	if len(ArgStringSlice(args, "brokers")) == 0 {
		host := ArgString(args, "host")
		port := ArgInt(args, "port")
		if host == "" || port <= 0 {
			return fmt.Errorf("missing required parameter: brokers (or host+port) for kafka type")
		}
	}
	return nil
}

func (h *kafkaHandler) ApplyCreateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	cfg := &asset_entity.KafkaConfig{
		Brokers:               ArgStringSlice(args, "brokers"),
		ClientID:              ArgString(args, "client_id"),
		SASLMechanism:         ArgString(args, "sasl_mechanism"),
		Username:              ArgString(args, "username"),
		TLS:                   ArgBool(args, "tls"),
		TLSInsecure:           ArgBool(args, "tls_insecure"),
		TLSServerName:         ArgString(args, "tls_server_name"),
		TLSCAFile:             ArgString(args, "tls_ca_file"),
		TLSCertFile:           ArgString(args, "tls_cert_file"),
		TLSKeyFile:            ArgString(args, "tls_key_file"),
		RequestTimeoutSeconds: ArgInt(args, "request_timeout_seconds"),
		MessagePreviewBytes:   ArgInt(args, "message_preview_bytes"),
		MessageFetchLimit:     ArgInt(args, "message_fetch_limit"),
		SSHAssetID:            a.SSHTunnelID,
	}
	if len(cfg.Brokers) == 0 {
		host := ArgString(args, "host")
		port := ArgInt(args, "port")
		if host != "" && port > 0 {
			cfg.Brokers = []string{fmt.Sprintf("%s:%d", host, port)}
		}
	}
	if cfg.SASLMechanism == "" {
		cfg.SASLMechanism = asset_entity.KafkaSASLNone
	}
	if password := ArgString(args, "password"); password != "" {
		encrypted, err := credential_svc.Default().Encrypt(password)
		if err != nil {
			return fmt.Errorf("encrypt Kafka password: %w", err)
		}
		cfg.Password = encrypted
	}
	return a.SetKafkaConfig(cfg)
}

func (h *kafkaHandler) ApplyUpdateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetKafkaConfig()
	if err != nil || cfg == nil {
		return err
	}
	if brokers := ArgStringSlice(args, "brokers"); len(brokers) > 0 {
		cfg.Brokers = brokers
	}
	if v := ArgString(args, "client_id"); v != "" {
		cfg.ClientID = v
	}
	if _, ok := args["sasl_mechanism"]; ok {
		cfg.SASLMechanism = ArgString(args, "sasl_mechanism")
	}
	if _, ok := args["username"]; ok {
		cfg.Username = ArgString(args, "username")
	}
	if _, ok := args["tls"]; ok {
		cfg.TLS = ArgBool(args, "tls")
	}
	if _, ok := args["tls_insecure"]; ok {
		cfg.TLSInsecure = ArgBool(args, "tls_insecure")
	}
	if _, ok := args["tls_server_name"]; ok {
		cfg.TLSServerName = ArgString(args, "tls_server_name")
	}
	if _, ok := args["tls_ca_file"]; ok {
		cfg.TLSCAFile = ArgString(args, "tls_ca_file")
	}
	if _, ok := args["tls_cert_file"]; ok {
		cfg.TLSCertFile = ArgString(args, "tls_cert_file")
	}
	if _, ok := args["tls_key_file"]; ok {
		cfg.TLSKeyFile = ArgString(args, "tls_key_file")
	}
	if _, ok := args["request_timeout_seconds"]; ok {
		cfg.RequestTimeoutSeconds = ArgInt(args, "request_timeout_seconds")
	}
	if _, ok := args["message_preview_bytes"]; ok {
		cfg.MessagePreviewBytes = ArgInt(args, "message_preview_bytes")
	}
	if _, ok := args["message_fetch_limit"]; ok {
		cfg.MessageFetchLimit = ArgInt(args, "message_fetch_limit")
	}
	if _, ok := args["ssh_asset_id"]; ok {
		a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
		cfg.SSHAssetID = a.SSHTunnelID
	}
	if password := ArgString(args, "password"); password != "" {
		encrypted, err := credential_svc.Default().Encrypt(password)
		if err != nil {
			return fmt.Errorf("encrypt Kafka password: %w", err)
		}
		cfg.Password = encrypted
		cfg.CredentialID = 0
	}
	return a.SetKafkaConfig(cfg)
}
