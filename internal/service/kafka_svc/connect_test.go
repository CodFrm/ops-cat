package kafka_svc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
)

func TestKafkaConnectService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer connect-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/connectors" && r.URL.Query().Get("expand") == "status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sink-orders": map[string]any{
					"status": map[string]any{
						"name": "sink-orders",
						"type": "sink",
						"connector": map[string]string{
							"state":     "RUNNING",
							"worker_id": "worker-1",
						},
						"tasks": []map[string]any{{"id": 0, "state": "RUNNING", "worker_id": "worker-1"}},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/connectors":
			_ = json.NewEncoder(w).Encode([]string{"sink-orders"})
		case r.Method == http.MethodGet && r.URL.Path == "/connectors/sink-orders":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "sink-orders",
				"type": "sink",
				"config": map[string]string{
					"name":            "sink-orders",
					"connector.class": "FileStreamSink",
				},
				"tasks": []map[string]any{{"connector": "sink-orders", "task": 0}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/connectors/sink-orders/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "sink-orders",
				"type": "sink",
				"connector": map[string]string{
					"state":     "RUNNING",
					"worker_id": "worker-1",
				},
				"tasks": []map[string]any{{"id": 0, "state": "RUNNING", "worker_id": "worker-1"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/connectors":
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, "sink-orders", payload["name"])
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPut && r.URL.Path == "/connectors/sink-orders/config":
			var payload map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, "sink-orders", payload["name"])
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPut && r.URL.Path == "/connectors/sink-orders/pause":
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && r.URL.Path == "/connectors/sink-orders/resume":
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPost && r.URL.Path == "/connectors/sink-orders/restart":
			assert.Equal(t, "true", r.URL.Query().Get("includeTasks"))
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodDelete && r.URL.Path == "/connectors/sink-orders":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	asset := &asset_entity.Asset{ID: 9201, Name: "connect", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(&asset_entity.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Connect: asset_entity.KafkaConnectConfig{
			Enabled: true,
			Clusters: []asset_entity.KafkaConnectClusterConfig{
				{Name: "local", URL: server.URL, AuthType: "bearer", Username: "connect-token"},
			},
		},
	}))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9201)).Return(asset, nil).AnyTimes()
	origRepo := asset_repo.Asset()
	asset_repo.RegisterAsset(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			asset_repo.RegisterAsset(origRepo)
		}
	})

	svc := New(nil)
	defer svc.Close()
	ctx := context.Background()

	clusters, err := svc.ListConnectClusters(ctx, asset.ID)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, "local", clusters[0].Name)

	connectors, err := svc.ListConnectors(ctx, ListConnectorsRequest{AssetID: asset.ID, Cluster: "local"})
	require.NoError(t, err)
	assert.Equal(t, "sink-orders", connectors[0].Name)
	assert.Equal(t, "RUNNING", connectors[0].Status)
	assert.Equal(t, "sink", connectors[0].Type)
	assert.Equal(t, 1, connectors[0].TaskCount)

	detail, err := svc.GetConnector(ctx, asset.ID, "local", "sink-orders")
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", detail.Status.Connector.State)
	assert.Equal(t, "worker-1", detail.Status.Connector.WorkerID)

	config := map[string]string{"connector.class": "FileStreamSink"}
	created, err := svc.CreateConnector(ctx, ConnectorConfigRequest{AssetID: asset.ID, Cluster: "local", Name: "sink-orders", Config: config})
	require.NoError(t, err)
	assert.Equal(t, "sink-orders", created.Name)

	updated, err := svc.UpdateConnectorConfig(ctx, ConnectorConfigRequest{AssetID: asset.ID, Cluster: "local", Name: "sink-orders", Config: config})
	require.NoError(t, err)
	assert.Equal(t, "local", updated.Cluster)

	_, err = svc.PauseConnector(ctx, asset.ID, "local", "sink-orders")
	require.NoError(t, err)
	_, err = svc.ResumeConnector(ctx, asset.ID, "local", "sink-orders")
	require.NoError(t, err)
	_, err = svc.RestartConnector(ctx, RestartConnectorRequest{AssetID: asset.ID, Cluster: "local", Name: "sink-orders", IncludeTasks: true})
	require.NoError(t, err)
	_, err = svc.DeleteConnector(ctx, asset.ID, "local", "sink-orders")
	require.NoError(t, err)
}

func TestKafkaConnectListConnectorsFallsBackOn4xx(t *testing.T) {
	var sawExpanded bool
	var sawFallback bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/connectors" && r.URL.Query().Get("expand") == "status":
			sawExpanded = true
			http.Error(w, "expand unsupported", http.StatusMethodNotAllowed)
		case r.Method == http.MethodGet && r.URL.Path == "/connectors":
			sawFallback = true
			_ = json.NewEncoder(w).Encode([]string{"legacy-sink"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	asset := &asset_entity.Asset{ID: 9202, Name: "connect-legacy", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(&asset_entity.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Connect: asset_entity.KafkaConnectConfig{
			Enabled: true,
			Clusters: []asset_entity.KafkaConnectClusterConfig{
				{Name: "local", URL: server.URL},
			},
		},
	}))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9202)).Return(asset, nil).AnyTimes()
	origRepo := asset_repo.Asset()
	asset_repo.RegisterAsset(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			asset_repo.RegisterAsset(origRepo)
		}
	})

	svc := New(nil)
	defer svc.Close()
	connectors, err := svc.ListConnectors(context.Background(), ListConnectorsRequest{AssetID: asset.ID, Cluster: "local"})
	require.NoError(t, err)
	require.Len(t, connectors, 1)
	assert.True(t, sawExpanded)
	assert.True(t, sawFallback)
	assert.Equal(t, "legacy-sink", connectors[0].Name)
	assert.Empty(t, connectors[0].Status)
}

func TestKafkaConnectHelpers(t *testing.T) {
	assert.Equal(t, "/connectors/a%2Fb/status", connectPath("connectors", "a/b", "status"))

	name, cfg, err := normalizeConnectorConfig("sink", map[string]string{"connector.class": "FileStreamSink"})
	require.NoError(t, err)
	assert.Equal(t, "sink", name)
	assert.Equal(t, "sink", cfg["name"])

	_, _, err = normalizeConnectorConfig("", map[string]string{"connector.class": "FileStreamSink"})
	assert.Error(t, err)

	_, _, err = selectKafkaConnectCluster(&asset_entity.KafkaConfig{
		Connect: asset_entity.KafkaConnectConfig{
			Enabled: true,
			Clusters: []asset_entity.KafkaConnectClusterConfig{
				{Name: "primary", URL: "http://connect-a:8083"},
				{Name: "backup", URL: "http://connect-b:8083"},
			},
		},
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "请指定 cluster 名称")
}
