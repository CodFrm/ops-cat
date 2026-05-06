package kafka_svc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type kafkaConnectClient struct {
	cluster  string
	baseURL  string
	authType string
	username string
	password string
	client   *http.Client
}

type connectConnectorInfo struct {
	Name   string            `json:"name"`
	Config map[string]string `json:"config"`
	Tasks  []struct {
		Connector string `json:"connector"`
		Task      int    `json:"task"`
	} `json:"tasks"`
	Type string `json:"type"`
}

type connectStatusResponse struct {
	Name      string `json:"name"`
	Connector struct {
		State    string `json:"state"`
		WorkerID string `json:"worker_id"`
		Trace    string `json:"trace"`
	} `json:"connector"`
	Tasks []struct {
		ID       int    `json:"id"`
		State    string `json:"state"`
		WorkerID string `json:"worker_id"`
		Trace    string `json:"trace"`
	} `json:"tasks"`
	Type string `json:"type"`
}

type connectExpandedStatusItem struct {
	Status connectStatusResponse `json:"status"`
}

type connectErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	Message   string `json:"message"`
}

type kafkaConnectHTTPStatusError struct {
	StatusCode  int
	ConnectCode int
	Message     string
}

func (e *kafkaConnectHTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	if e.ConnectCode != 0 && e.Message != "" {
		return fmt.Sprintf("HTTP %d connect_code=%d: %s", e.StatusCode, e.ConnectCode, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func isKafkaConnectHTTPClientError(err error) bool {
	var statusErr *kafkaConnectHTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode >= 400 && statusErr.StatusCode < 500
}

func (s *Service) ListConnectClusters(ctx context.Context, assetID int64) ([]KafkaConnectCluster, error) {
	_, cfg, err := resolveKafkaAssetConfig(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if !cfg.Connect.Enabled {
		return nil, fmt.Errorf("kafka connect 未启用")
	}
	out := make([]KafkaConnectCluster, 0, len(cfg.Connect.Clusters))
	for _, cluster := range cfg.Connect.Clusters {
		if strings.TrimSpace(cluster.URL) == "" {
			continue
		}
		name := strings.TrimSpace(cluster.Name)
		if name == "" {
			name = "default"
		}
		out = append(out, KafkaConnectCluster{Name: name, URL: strings.TrimSpace(cluster.URL)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("kafka connect cluster 不能为空")
	}
	return out, nil
}

func (s *Service) ListConnectors(ctx context.Context, req ListConnectorsRequest) ([]KafkaConnectorSummary, error) {
	var out []KafkaConnectorSummary
	err := s.withKafkaConnect(ctx, req.AssetID, req.Cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		var expanded map[string]connectExpandedStatusItem
		query := url.Values{"expand": []string{"status"}}
		if err := client.do(ctx, http.MethodGet, "/connectors", query, nil, &expanded); err == nil {
			out = connectorSummariesFromExpandedStatus(expanded)
			return nil
		} else if !isKafkaConnectHTTPClientError(err) {
			return fmt.Errorf("读取 Kafka Connect connector status 列表失败: %w", err)
		}

		var names []string
		if err := client.do(ctx, http.MethodGet, "/connectors", nil, nil, &names); err != nil {
			return fmt.Errorf("读取 Kafka Connect connectors 失败: %w", err)
		}
		out = make([]KafkaConnectorSummary, 0, len(names))
		for _, name := range names {
			out = append(out, KafkaConnectorSummary{Name: name})
		}
		return nil
	})
	return out, err
}

func (s *Service) GetConnector(ctx context.Context, assetID int64, cluster string, name string) (KafkaConnectorDetail, error) {
	var out KafkaConnectorDetail
	err := s.withKafkaConnect(ctx, assetID, cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("connector 不能为空")
		}
		var info connectConnectorInfo
		if err := client.do(ctx, http.MethodGet, connectPath("connectors", name), nil, nil, &info); err != nil {
			return fmt.Errorf("读取 Kafka Connect connector 失败: %w", err)
		}
		var status connectStatusResponse
		if err := client.do(ctx, http.MethodGet, connectPath("connectors", name, "status"), nil, nil, &status); err != nil {
			return fmt.Errorf("读取 Kafka Connect connector status 失败: %w", err)
		}
		out = connectorDetailFromResponses(info, status)
		return nil
	})
	return out, err
}

func (s *Service) CreateConnector(ctx context.Context, req ConnectorConfigRequest) (ConnectorOperationResponse, error) {
	var out ConnectorOperationResponse
	err := s.withKafkaConnect(ctx, req.AssetID, req.Cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name, config, err := normalizeConnectorConfig(req.Name, req.Config)
		if err != nil {
			return err
		}
		payload := map[string]any{"name": name, "config": config}
		var info connectConnectorInfo
		if err := client.do(ctx, http.MethodPost, "/connectors", nil, payload, &info); err != nil {
			return fmt.Errorf("创建 Kafka Connect connector 失败: %w", err)
		}
		// 服务端可能对 name 做归一化（如去空白），以响应中的 name 为准
		resolvedName := strings.TrimSpace(info.Name)
		if resolvedName == "" {
			resolvedName = name
		}
		out = ConnectorOperationResponse{Cluster: client.cluster, Name: resolvedName}
		return nil
	})
	return out, err
}

func (s *Service) UpdateConnectorConfig(ctx context.Context, req ConnectorConfigRequest) (ConnectorOperationResponse, error) {
	var out ConnectorOperationResponse
	err := s.withKafkaConnect(ctx, req.AssetID, req.Cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name, config, err := normalizeConnectorConfig(req.Name, req.Config)
		if err != nil {
			return err
		}
		if err := client.do(ctx, http.MethodPut, connectPath("connectors", name, "config"), nil, config, nil); err != nil {
			return fmt.Errorf("更新 Kafka Connect connector 配置失败: %w", err)
		}
		out = ConnectorOperationResponse{Cluster: client.cluster, Name: name}
		return nil
	})
	return out, err
}

func (s *Service) PauseConnector(ctx context.Context, assetID int64, cluster string, name string) (ConnectorOperationResponse, error) {
	return s.connectNoBodyOperation(ctx, assetID, cluster, name, http.MethodPut, "pause", "暂停 Kafka Connect connector 失败")
}

func (s *Service) ResumeConnector(ctx context.Context, assetID int64, cluster string, name string) (ConnectorOperationResponse, error) {
	return s.connectNoBodyOperation(ctx, assetID, cluster, name, http.MethodPut, "resume", "恢复 Kafka Connect connector 失败")
}

func (s *Service) RestartConnector(ctx context.Context, req RestartConnectorRequest) (ConnectorOperationResponse, error) {
	var out ConnectorOperationResponse
	err := s.withKafkaConnect(ctx, req.AssetID, req.Cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			return fmt.Errorf("connector 不能为空")
		}
		query := url.Values{}
		if req.IncludeTasks {
			query.Set("includeTasks", "true")
		}
		if req.OnlyFailed {
			query.Set("onlyFailed", "true")
		}
		if err := client.do(ctx, http.MethodPost, connectPath("connectors", name, "restart"), query, nil, nil); err != nil {
			return fmt.Errorf("重启 Kafka Connect connector 失败: %w", err)
		}
		out = ConnectorOperationResponse{Cluster: client.cluster, Name: name}
		return nil
	})
	return out, err
}

func (s *Service) DeleteConnector(ctx context.Context, assetID int64, cluster string, name string) (ConnectorOperationResponse, error) {
	return s.connectNoBodyOperation(ctx, assetID, cluster, name, http.MethodDelete, "", "删除 Kafka Connect connector 失败")
}

func (s *Service) connectNoBodyOperation(ctx context.Context, assetID int64, cluster string, name string, method string, action string, errPrefix string) (ConnectorOperationResponse, error) {
	var out ConnectorOperationResponse
	err := s.withKafkaConnect(ctx, assetID, cluster, func(ctx context.Context, client *kafkaConnectClient, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("connector 不能为空")
		}
		parts := []string{"connectors", name}
		if action != "" {
			parts = append(parts, action)
		}
		if err := client.do(ctx, method, connectPath(parts...), nil, nil, nil); err != nil {
			return fmt.Errorf("%s: %w", errPrefix, err)
		}
		out = ConnectorOperationResponse{Cluster: client.cluster, Name: name}
		return nil
	})
	return out, err
}

func (s *Service) withKafkaConnect(ctx context.Context, assetID int64, clusterName string, fn func(context.Context, *kafkaConnectClient, *asset_entity.Asset, *asset_entity.KafkaConfig) error) error {
	asset, cfg, err := resolveKafkaAssetConfig(ctx, assetID)
	if err != nil {
		return err
	}
	cluster, resolvedName, err := selectKafkaConnectCluster(cfg, clusterName)
	if err != nil {
		return err
	}
	password, err := credential_resolver.Default().ResolvePasswordGeneric(ctx, &cluster)
	if err != nil {
		return fmt.Errorf("解析 Kafka Connect 凭据失败: %w", err)
	}
	client, err := newKafkaConnectClient(&cluster, resolvedName, password, kafkaTimeout(cfg, defaultKafkaOperationTimeout))
	if err != nil {
		return err
	}
	return fn(ctx, client, asset, cfg)
}

func selectKafkaConnectCluster(cfg *asset_entity.KafkaConfig, name string) (asset_entity.KafkaConnectClusterConfig, string, error) {
	if cfg == nil || !cfg.Connect.Enabled {
		return asset_entity.KafkaConnectClusterConfig{}, "", fmt.Errorf("kafka connect 未启用")
	}
	name = strings.TrimSpace(name)
	for _, cluster := range cfg.Connect.Clusters {
		resolvedName := strings.TrimSpace(cluster.Name)
		if resolvedName == "" {
			resolvedName = "default"
		}
		if strings.TrimSpace(cluster.URL) == "" {
			continue
		}
		if name == "" && len(cfg.Connect.Clusters) == 1 {
			return cluster, resolvedName, nil
		}
		if name == resolvedName {
			return cluster, resolvedName, nil
		}
	}
	if name == "" {
		return asset_entity.KafkaConnectClusterConfig{}, "", fmt.Errorf("存在多个 Kafka Connect Cluster，请指定 cluster 名称")
	}
	return asset_entity.KafkaConnectClusterConfig{}, "", fmt.Errorf("kafka connect cluster 不存在: %s", name)
}

func newKafkaConnectClient(cfg *asset_entity.KafkaConnectClusterConfig, cluster string, password string, timeout time.Duration) (*kafkaConnectClient, error) {
	httpClient, err := kafkaConnectHTTPClient(cfg, timeout)
	if err != nil {
		return nil, err
	}
	return &kafkaConnectClient{
		cluster:  cluster,
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		authType: strings.ToLower(strings.TrimSpace(cfg.AuthType)),
		username: strings.TrimSpace(cfg.Username),
		password: password,
		client:   httpClient,
	}, nil
}

func kafkaConnectHTTPClient(cfg *asset_entity.KafkaConnectClusterConfig, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig, err := kafkaConnectTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func kafkaConnectTLSConfig(cfg *asset_entity.KafkaConnectClusterConfig) (*tls.Config, error) {
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
			return nil, fmt.Errorf("读取 Kafka Connect TLS CA 证书失败: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("解析 Kafka Connect TLS CA 证书失败")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
			return nil, fmt.Errorf("kafka connect TLS 客户端证书和私钥必须同时配置")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载 Kafka Connect TLS 客户端证书失败: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func (c *kafkaConnectClient) do(ctx context.Context, method string, path string, query url.Values, body any, out any) error {
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
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
		return kafkaConnectHTTPError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析 Kafka Connect 响应失败: %w", err)
	}
	return nil
}

func (c *kafkaConnectClient) url(path string, query url.Values) (string, error) {
	target, err := url.Parse(c.baseURL + path)
	if err != nil {
		return "", err
	}
	if len(query) > 0 {
		target.RawQuery = query.Encode()
	}
	return target.String(), nil
}

func (c *kafkaConnectClient) applyAuth(req *http.Request) error {
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
		return fmt.Errorf("不支持的 Kafka Connect AuthType: %s", c.authType)
	}
}

func kafkaConnectHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body connectErrorResponse
	if err := json.Unmarshal(data, &body); err == nil && body.Message != "" {
		return &kafkaConnectHTTPStatusError{
			StatusCode:  resp.StatusCode,
			ConnectCode: body.ErrorCode,
			Message:     body.Message,
		}
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = http.StatusText(resp.StatusCode)
	}
	return &kafkaConnectHTTPStatusError{
		StatusCode: resp.StatusCode,
		Message:    text,
	}
}

func connectPath(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/" + strings.Join(escaped, "/")
}

func normalizeConnectorConfig(name string, config map[string]string) (string, map[string]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("connector 不能为空")
	}
	if len(config) == 0 {
		return "", nil, fmt.Errorf("connector 配置不能为空")
	}
	out := make(map[string]string, len(config)+1)
	for key, value := range config {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if out["name"] == "" {
		out["name"] = name
	}
	return name, out, nil
}

func connectorDetailFromResponses(info connectConnectorInfo, status connectStatusResponse) KafkaConnectorDetail {
	out := KafkaConnectorDetail{
		Name:   info.Name,
		Type:   info.Type,
		Config: info.Config,
		Tasks:  make([]KafkaConnectorTask, 0, len(info.Tasks)),
		Status: KafkaConnectorStatus{
			Name: status.Name,
			Type: status.Type,
			Connector: KafkaConnectorWorkerState{
				State:    status.Connector.State,
				WorkerID: status.Connector.WorkerID,
				Trace:    status.Connector.Trace,
			},
			Tasks: make([]KafkaConnectorTaskState, 0, len(status.Tasks)),
		},
	}
	for _, task := range info.Tasks {
		out.Tasks = append(out.Tasks, KafkaConnectorTask{Connector: task.Connector, Task: task.Task})
	}
	for _, task := range status.Tasks {
		out.Status.Tasks = append(out.Status.Tasks, KafkaConnectorTaskState{
			ID:       task.ID,
			State:    task.State,
			WorkerID: task.WorkerID,
			Trace:    task.Trace,
		})
	}
	return out
}

func connectorSummariesFromExpandedStatus(expanded map[string]connectExpandedStatusItem) []KafkaConnectorSummary {
	out := make([]KafkaConnectorSummary, 0, len(expanded))
	for name, item := range expanded {
		status := item.Status
		summary := KafkaConnectorSummary{
			Name:            name,
			Type:            status.Type,
			Status:          status.Connector.State,
			TaskCount:       len(status.Tasks),
			FailedTaskCount: failedConnectTaskCount(status.Tasks),
		}
		if status.Name != "" {
			summary.Name = status.Name
		}
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func failedConnectTaskCount(tasks []struct {
	ID       int    `json:"id"`
	State    string `json:"state"`
	WorkerID string `json:"worker_id"`
	Trace    string `json:"trace"`
}) int {
	var count int
	for _, task := range tasks {
		if strings.EqualFold(task.State, "FAILED") {
			count++
		}
	}
	return count
}
