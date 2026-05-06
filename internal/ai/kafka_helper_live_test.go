package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func TestKafkaToolLiveClusterOverview(t *testing.T) {
	brokersText := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if brokersText == "" {
		t.Skip("set OPSKAT_KAFKA_TEST_BROKERS to run live Kafka AI tool test")
	}

	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{ID: 1, Name: "live-kafka", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(&asset_entity.KafkaConfig{
		Brokers:               splitKafkaLiveBrokers(brokersText),
		SASLMechanism:         asset_entity.KafkaSASLNone,
		RequestTimeoutSeconds: 5,
	}))
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	result, err := handleKafkaCluster(ctx, map[string]any{
		"asset_id":  float64(1),
		"operation": "overview",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "broker_count")
}

func splitKafkaLiveBrokers(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
