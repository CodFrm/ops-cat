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

func TestSchemaRegistryService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer schema-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.schemaregistry.v1+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/subjects":
			_ = json.NewEncoder(w).Encode([]string{"orders-value"})
		case r.Method == http.MethodGet && r.URL.Path == "/subjects/orders-value/versions":
			_ = json.NewEncoder(w).Encode([]int{1, 2})
		case r.Method == http.MethodGet && r.URL.Path == "/subjects/orders-value/versions/latest":
			_ = json.NewEncoder(w).Encode(SchemaVersionDetail{
				Subject:    "orders-value",
				ID:         11,
				Version:    2,
				SchemaType: "AVRO",
				Schema:     `{"type":"record","name":"Order","fields":[]}`,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/compatibility/subjects/orders-value/versions/latest":
			var payload schemaRegistryPayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Contains(t, payload.Schema, "Order")
			_ = json.NewEncoder(w).Encode(schemaRegistryCompatibilityResponse{IsCompatible: true})
		case r.Method == http.MethodPost && r.URL.Path == "/subjects/orders-value/versions":
			var payload schemaRegistryPayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, "AVRO", payload.SchemaType)
			_ = json.NewEncoder(w).Encode(schemaRegistryIDResponse{ID: 12})
		case r.Method == http.MethodDelete && r.URL.Path == "/subjects/orders-value/versions/2":
			assert.Equal(t, "true", r.URL.Query().Get("permanent"))
			_ = json.NewEncoder(w).Encode(2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	asset := &asset_entity.Asset{ID: 9101, Name: "schema-registry", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(&asset_entity.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		SchemaRegistry: asset_entity.KafkaSchemaRegistryConfig{
			Enabled:  true,
			URL:      server.URL,
			AuthType: "bearer",
			Username: "schema-token",
		},
	}))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9101)).Return(asset, nil).AnyTimes()
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

	subjects, err := svc.ListSchemaSubjects(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"orders-value"}, subjects)

	versions, err := svc.GetSchemaSubjectVersions(ctx, asset.ID, "orders-value")
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, versions.Versions)

	detail, err := svc.GetSchema(ctx, asset.ID, "orders-value", "")
	require.NoError(t, err)
	assert.Equal(t, 2, detail.Version)

	compatibility, err := svc.CheckSchemaCompatibility(ctx, CheckSchemaCompatibilityRequest{
		AssetID:    asset.ID,
		Subject:    "orders-value",
		SchemaType: "AVRO",
		Schema:     `{"type":"record","name":"Order","fields":[]}`,
	})
	require.NoError(t, err)
	assert.True(t, compatibility.Compatible)

	registered, err := svc.RegisterSchema(ctx, RegisterSchemaRequest{
		AssetID:    asset.ID,
		Subject:    "orders-value",
		SchemaType: "AVRO",
		Schema:     `{"type":"record","name":"Order","fields":[]}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 12, registered.ID)

	deleted, err := svc.DeleteSchema(ctx, DeleteSchemaRequest{
		AssetID:   asset.ID,
		Subject:   "orders-value",
		Version:   "2",
		Permanent: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, deleted.DeletedVersion)
}

func TestSchemaRegistryClientHelpers(t *testing.T) {
	assert.Equal(t, "/subjects/orders-value/versions/latest", schemaRegistryPath("subjects", "orders-value", "versions", "latest"))
	assert.Equal(t, "/subjects/a%2Fb/versions/1", schemaRegistryPath("subjects", "a/b", "versions", "1"))
	assert.Equal(t, "latest", normalizeSchemaVersion(""))
	assert.Equal(t, "3", normalizeSchemaVersion(" 3 "))

	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	require.NoError(t, err)
	client := &schemaRegistryClient{authType: "bearer", username: "token"}
	require.NoError(t, client.applyAuth(req))
	assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
}
