package kafka_svc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type schemaRegistryClient struct {
	baseURL  string
	authType string
	username string
	password string
	client   *http.Client
}

type schemaRegistryPayload struct {
	Schema     string            `json:"schema"`
	SchemaType string            `json:"schemaType,omitempty"`
	References []SchemaReference `json:"references,omitempty"`
}

type schemaRegistryIDResponse struct {
	ID int `json:"id"`
}

type schemaRegistryCompatibilityResponse struct {
	IsCompatible bool     `json:"is_compatible"`
	Messages     []string `json:"messages,omitempty"`
}

type schemaRegistryErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	Message   string `json:"message"`
}

func (s *Service) ListSchemaSubjects(ctx context.Context, assetID int64) ([]string, error) {
	var out []string
	err := s.withSchemaRegistry(ctx, assetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		if err := client.do(ctx, http.MethodGet, "/subjects", nil, nil, &out); err != nil {
			return fmt.Errorf("读取 Schema Registry subjects 失败: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *Service) GetSchemaSubjectVersions(ctx context.Context, assetID int64, subject string) (SchemaSubjectVersions, error) {
	var out SchemaSubjectVersions
	err := s.withSchemaRegistry(ctx, assetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return fmt.Errorf("subject 不能为空")
		}
		var versions []int
		if err := client.do(ctx, http.MethodGet, schemaRegistryPath("subjects", subject, "versions"), nil, nil, &versions); err != nil {
			return fmt.Errorf("读取 Schema Registry subject versions 失败: %w", err)
		}
		out = SchemaSubjectVersions{Subject: subject, Versions: versions}
		return nil
	})
	return out, err
}

func (s *Service) GetSchema(ctx context.Context, assetID int64, subject string, version string) (SchemaVersionDetail, error) {
	var out SchemaVersionDetail
	err := s.withSchemaRegistry(ctx, assetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return fmt.Errorf("subject 不能为空")
		}
		version = normalizeSchemaVersion(version)
		if err := client.do(ctx, http.MethodGet, schemaRegistryPath("subjects", subject, "versions", version), nil, nil, &out); err != nil {
			return fmt.Errorf("读取 Schema Registry schema 失败: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *Service) CheckSchemaCompatibility(ctx context.Context, req CheckSchemaCompatibilityRequest) (CheckSchemaCompatibilityResponse, error) {
	var out CheckSchemaCompatibilityResponse
	err := s.withSchemaRegistry(ctx, req.AssetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		subject := strings.TrimSpace(req.Subject)
		if subject == "" {
			return fmt.Errorf("subject 不能为空")
		}
		if strings.TrimSpace(req.Schema) == "" {
			return fmt.Errorf("schema 不能为空")
		}
		version := normalizeSchemaVersion(req.Version)
		payload := schemaRegistryPayload{Schema: req.Schema, SchemaType: strings.TrimSpace(req.SchemaType), References: req.References}
		var response schemaRegistryCompatibilityResponse
		if err := client.do(ctx, http.MethodPost, schemaRegistryPath("compatibility", "subjects", subject, "versions", version), nil, payload, &response); err != nil {
			return fmt.Errorf("检查 Schema Registry 兼容性失败: %w", err)
		}
		out = CheckSchemaCompatibilityResponse{
			Subject:    subject,
			Version:    version,
			Compatible: response.IsCompatible,
			Messages:   response.Messages,
		}
		return nil
	})
	return out, err
}

func (s *Service) RegisterSchema(ctx context.Context, req RegisterSchemaRequest) (RegisterSchemaResponse, error) {
	var out RegisterSchemaResponse
	err := s.withSchemaRegistry(ctx, req.AssetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		subject := strings.TrimSpace(req.Subject)
		if subject == "" {
			return fmt.Errorf("subject 不能为空")
		}
		if strings.TrimSpace(req.Schema) == "" {
			return fmt.Errorf("schema 不能为空")
		}
		payload := schemaRegistryPayload{Schema: req.Schema, SchemaType: strings.TrimSpace(req.SchemaType), References: req.References}
		var response schemaRegistryIDResponse
		if err := client.do(ctx, http.MethodPost, schemaRegistryPath("subjects", subject, "versions"), nil, payload, &response); err != nil {
			return fmt.Errorf("注册 Schema Registry schema 失败: %w", err)
		}
		out = RegisterSchemaResponse{Subject: subject, ID: response.ID}
		return nil
	})
	return out, err
}

func (s *Service) DeleteSchema(ctx context.Context, req DeleteSchemaRequest) (DeleteSchemaResponse, error) {
	var out DeleteSchemaResponse
	err := s.withSchemaRegistry(ctx, req.AssetID, func(ctx context.Context, client *schemaRegistryClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		subject := strings.TrimSpace(req.Subject)
		if subject == "" {
			return fmt.Errorf("subject 不能为空")
		}
		query := url.Values{}
		if req.Permanent {
			query.Set("permanent", "true")
		}
		version := strings.TrimSpace(req.Version)
		if version == "" {
			var versions []int
			if err := client.do(ctx, http.MethodDelete, schemaRegistryPath("subjects", subject), query, nil, &versions); err != nil {
				return fmt.Errorf("删除 Schema Registry subject 失败: %w", err)
			}
			out = DeleteSchemaResponse{Subject: subject, Versions: versions}
			return nil
		}
		var deletedVersion int
		if err := client.do(ctx, http.MethodDelete, schemaRegistryPath("subjects", subject, "versions", version), query, nil, &deletedVersion); err != nil {
			return fmt.Errorf("删除 Schema Registry version 失败: %w", err)
		}
		out = DeleteSchemaResponse{Subject: subject, Version: version, DeletedVersion: deletedVersion}
		return nil
	})
	return out, err
}

func (s *Service) withSchemaRegistry(ctx context.Context, assetID int64, fn func(context.Context, *schemaRegistryClient, *asset_entity.Asset, *asset_entity.KafkaConfig) error) error {
	asset, cfg, err := resolveKafkaAssetConfig(ctx, assetID)
	if err != nil {
		return err
	}
	schemaCfg := cfg.SchemaRegistry
	if !schemaCfg.Enabled {
		return fmt.Errorf("schema registry 未启用")
	}
	if strings.TrimSpace(schemaCfg.URL) == "" {
		return fmt.Errorf("schema registry URL 不能为空")
	}
	password, err := credential_resolver.Default().ResolvePasswordGeneric(ctx, &schemaCfg)
	if err != nil {
		return fmt.Errorf("解析 Schema Registry 凭据失败: %w", err)
	}
	client, err := newSchemaRegistryClient(&schemaCfg, password, kafkaTimeout(cfg, defaultKafkaOperationTimeout))
	if err != nil {
		return err
	}
	return fn(ctx, client, asset, cfg)
}

func newSchemaRegistryClient(cfg *asset_entity.KafkaSchemaRegistryConfig, password string, timeout time.Duration) (*schemaRegistryClient, error) {
	httpClient, err := schemaRegistryHTTPClient(cfg, timeout)
	if err != nil {
		return nil, err
	}
	return &schemaRegistryClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		authType: strings.ToLower(strings.TrimSpace(cfg.AuthType)),
		username: strings.TrimSpace(cfg.Username),
		password: password,
		client:   httpClient,
	}, nil
}

func schemaRegistryHTTPClient(cfg *asset_entity.KafkaSchemaRegistryConfig, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig, err := schemaRegistryTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func schemaRegistryTLSConfig(cfg *asset_entity.KafkaSchemaRegistryConfig) (*tls.Config, error) {
	if !cfg.TLSInsecure && cfg.TLSServerName == "" && cfg.TLSCAFile == "" && cfg.TLSCertFile == "" && cfg.TLSKeyFile == "" {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.TLSServerName,
		InsecureSkipVerify: cfg.TLSInsecure,
	}
	if cfg.TLSCAFile != "" {
		ca, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("读取 Schema Registry TLS CA 证书失败: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("解析 Schema Registry TLS CA 证书失败")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
			return nil, fmt.Errorf("schema registry TLS 客户端证书和私钥必须同时配置")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载 Schema Registry TLS 客户端证书失败: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func (c *schemaRegistryClient) do(ctx context.Context, method string, path string, query url.Values, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	target, err := c.url(path, query)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.schemaregistry.v1+json, application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.schemaregistry.v1+json")
	}
	if err := c.applyAuth(req); err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schemaRegistryHTTPError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析 Schema Registry 响应失败: %w", err)
	}
	return nil
}

func (c *schemaRegistryClient) url(path string, query url.Values) (string, error) {
	target, err := url.Parse(c.baseURL + path)
	if err != nil {
		return "", err
	}
	if len(query) > 0 {
		target.RawQuery = query.Encode()
	}
	return target.String(), nil
}

func (c *schemaRegistryClient) applyAuth(req *http.Request) error {
	switch c.authType {
	case "", "none":
		return nil
	case "basic":
		req.SetBasicAuth(c.username, c.password)
		return nil
	case "bearer":
		token := strings.TrimSpace(c.password)
		if token == "" {
			token = c.username
		}
		if token == "" {
			return fmt.Errorf("bearer token 不能为空")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	default:
		return fmt.Errorf("不支持的 Schema Registry AuthType: %s", c.authType)
	}
}

func schemaRegistryHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body schemaRegistryErrorResponse
	if err := json.Unmarshal(data, &body); err == nil && body.Message != "" {
		return fmt.Errorf("http %d schema_registry_code=%d: %s", resp.StatusCode, body.ErrorCode, body.Message)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("http %d: %s", resp.StatusCode, text)
}

func schemaRegistryPath(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/" + strings.Join(escaped, "/")
}

func normalizeSchemaVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "latest"
	}
	return version
}
